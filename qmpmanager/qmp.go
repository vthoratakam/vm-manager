package qmpmanager

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

const maxEpollEvents = 32

type QMPManager struct {
	cmdResponses map[string]chan map[string]interface{}
	mutex        sync.Mutex
	ln_mutex     map[string]*sync.Mutex
	sockets      map[string]net.Conn
	fdToLinode   map[int]string
	linodeToFd   map[string]int
	epfd         int
	events       [maxEpollEvents]syscall.EpollEvent
	handler      QMPEventHandler
}

var (
	manager *QMPManager
	once    sync.Once
)

type QMPEventHandler interface {
	OnQMPEvent(vmID, event string, data map[string]interface{})
}

func (qmp *QMPManager) SetHandler(handler QMPEventHandler) {
	qmp.handler = handler
}

func GetManager() *QMPManager {
	once.Do(func() {
		manager = newQMPManager()
		go manager.StartSocketLoop()
	})
	return manager
}

func newQMPManager() *QMPManager {
	epfd, err := syscall.EpollCreate1(0)
	if err != nil {
		log.Fatalf("epoll_create1 failed: %v", err)
	}
	return &QMPManager{
		cmdResponses: make(map[string]chan map[string]interface{}),
		ln_mutex:     make(map[string]*sync.Mutex),
		sockets:      make(map[string]net.Conn),
		fdToLinode:   make(map[int]string),
		linodeToFd:   make(map[string]int),
		epfd:         epfd,
	}
}

func sendDataQMP(conn net.Conn, cmd map[string]interface{}) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = conn.Write(data)
	return err
}

func (qm *QMPManager) SendCommand(linodeID string, cmd map[string]interface{}) (map[string]interface{}, error) {
	qm.mutex.Lock()
	conn, ok := qm.sockets[linodeID]
	qm.mutex.Unlock()

	if !ok {
		return nil, fmt.Errorf("no socket for linode %s", linodeID)
	}

	id := uuid.New().String()
	cmd["id"] = id
	ch := make(chan map[string]interface{}, 1)

	qm.mutex.Lock()
	qm.cmdResponses[id] = ch
	qm.mutex.Unlock()

	if err := sendDataQMP(conn, cmd); err != nil {
		return nil, err
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout waiting for response")
	}
}

func (qm *QMPManager) AddVM(linodeID string) error {
	qm.ln_mutex[linodeID] = &sync.Mutex{}
	qm.ln_mutex[linodeID].Lock()
	path := fmt.Sprintf("/linodes/%s/run/qemu.monitor", linodeID)

	conn, err := net.Dial("unix", path)
	if err != nil {
		return fmt.Errorf("connect failed: %v", err)
	}

	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		conn.Close()
		return fmt.Errorf("failed to cast to *net.UnixConn")
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		conn.Close()
		return fmt.Errorf("greeting read failed: %v", err)
	}

	var greeting map[string]interface{}
	if err := json.Unmarshal(line, &greeting); err != nil {
		conn.Close()
		return fmt.Errorf("invalid greeting: %v", err)
	}

	cmd := map[string]interface{}{
		"execute": "qmp_capabilities",
		"id":      uuid.New().String(),
	}
	if err := sendDataQMP(conn, cmd); err != nil {
		conn.Close()
		return err
	}

	line, err = reader.ReadBytes('\n')
	if err != nil {
		conn.Close()
		return fmt.Errorf("capabilities response read failed: %v", err)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(line, &resp); err != nil {
		conn.Close()
		return fmt.Errorf("invalid capabilities response: %v", err)
	}
	if _, ok := resp["return"]; !ok {
		conn.Close()
		return fmt.Errorf("capabilities failed: %v", resp)
	}

	file, err := unixConn.File()
	if err != nil {
		conn.Close()
		return fmt.Errorf("fd extraction failed: %v", err)
	}
	fd := int(file.Fd())
	defer file.Close()

	event := syscall.EpollEvent{Events: syscall.EPOLLIN, Fd: int32(fd)}
	if err := syscall.EpollCtl(qm.epfd, syscall.EPOLL_CTL_ADD, fd, &event); err != nil {
		conn.Close()
		return fmt.Errorf("epoll_ctl error: %v", err)
	}

	qm.sockets[linodeID] = conn
	qm.fdToLinode[fd] = linodeID
	qm.linodeToFd[linodeID] = fd

	fmt.Printf("VM %s is added to QMP Manager\n", linodeID)
	qm.ln_mutex[linodeID].Unlock()

	return nil
}

func (qm *QMPManager) RemoveVM(linodeID string) {
	qm.ln_mutex[linodeID].Lock()
	if conn, ok := qm.sockets[linodeID]; ok {
		fd := qm.linodeToFd[linodeID]
		syscall.EpollCtl(qm.epfd, syscall.EPOLL_CTL_DEL, fd, nil)
		conn.Close()
		delete(qm.sockets, linodeID)
		delete(qm.fdToLinode, fd)
		delete(qm.linodeToFd, linodeID)
		qm.handler.OnQMPEvent(linodeID, "QMPSOCKREMOVE", nil)
	}
	fmt.Printf("VM %s is removed from QMP Manager\n", linodeID)
	qm.ln_mutex[linodeID].Unlock()
	delete(qm.ln_mutex, linodeID)
}

func (qm *QMPManager) handleMessage(linodeID string, msg map[string]interface{}) {
	if id, ok := msg["id"].(string); ok {
		if ch, exists := qm.cmdResponses[id]; exists {
			ch <- msg
			delete(qm.cmdResponses, id)
		}
	} else if event, ok := msg["event"].(string); ok {
		qm.handler.OnQMPEvent(linodeID, event, msg)
	}
}

func (qm *QMPManager) StartSocketLoop() {
	for {
		nevents, err := syscall.EpollWait(qm.epfd, qm.events[:], -1)
		if err != nil {
			log.Printf("epoll_wait error: %v", err)
			continue
		}
		for i := 0; i < nevents; i++ {
			fd := int(qm.events[i].Fd)

			linodeID := qm.fdToLinode[fd]
			conn := qm.sockets[linodeID]

			qm.ln_mutex[linodeID].Lock()
			reader := bufio.NewReader(conn)
			line, err := reader.ReadBytes('\n')
			qm.ln_mutex[linodeID].Unlock()
			if err != nil {
				log.Printf("read error (%s): %v", linodeID, err)
				qm.RemoveVM(linodeID)
				continue
			}

			var msg map[string]interface{}
			if err := json.Unmarshal(line, &msg); err != nil {
				log.Printf("invalid JSON (%s): %v\n", linodeID, err)
				continue
			}
			qm.handleMessage(linodeID, msg)
		}
	}
}
