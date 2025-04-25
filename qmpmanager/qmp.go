package qmpmanager

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

const maxEpollEvents = 32
const qmpReadBufferSize = 4096

type QMPEventHandler interface {
	OnQMPEvent(vmID, event string, data map[string]interface{})
}

type QMPManager struct {
	mu           sync.Mutex
	cmdResponses map[string]chan map[string]interface{}
	vmToFD       map[string]int
	fdToVM       map[int]string
	epfd         int
	events       [maxEpollEvents]syscall.EpollEvent
	handler      QMPEventHandler
	bufPool      sync.Pool
}

var (
	manager *QMPManager
	once    sync.Once
)

func GetManager() *QMPManager {
	once.Do(func() {
		manager = newQMPManager()
		go manager.eventLoop()
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
		vmToFD:       make(map[string]int),
		fdToVM:       make(map[int]string),
		epfd:         epfd,
		bufPool: sync.Pool{
			New: func() interface{} {
				b := make([]byte, qmpReadBufferSize)
				return &b
			},
		},
	}
}

func (m *QMPManager) SetHandler(handler QMPEventHandler) {
	m.handler = handler
}

func (m *QMPManager) AddVM(vmID string) error {
	socketPath := fmt.Sprintf("/linodes/%s/run/qemu.monitor", vmID)

	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return fmt.Errorf("socket creation failed: %w", err)
	}
	addr := &syscall.SockaddrUnix{Name: socketPath}
	if err := syscall.Connect(fd, addr); err != nil {
		syscall.Close(fd)
		return fmt.Errorf("connect failed: %w", err)
	}

	// Read QMP greeting
	greeting, err := readQMPLine(fd)
	if err != nil {
		syscall.Close(fd)
		return fmt.Errorf("greeting read failed: %w", err)
	}
	var msg map[string]interface{}
	if err := json.Unmarshal(greeting, &msg); err != nil {
		syscall.Close(fd)
		return fmt.Errorf("invalid QMP greeting: %w", err)
	}

	// Send qmp_capabilities
	capCmd := map[string]interface{}{
		"execute": "qmp_capabilities",
	}
	if err := writeQMPMapCommand(fd, capCmd); err != nil {
		syscall.Close(fd)
		return fmt.Errorf("sending capabilities failed: %w", err)
	}
	capResp, err := readQMPLine(fd)
	if err != nil || !jsonContainsReturn(capResp) {
		syscall.Close(fd)
		return fmt.Errorf("capabilities response invalid or missing: %s", capResp)
	}

	// Register FD to epoll
	event := syscall.EpollEvent{Events: syscall.EPOLLIN, Fd: int32(fd)}
	if err := syscall.EpollCtl(m.epfd, syscall.EPOLL_CTL_ADD, fd, &event); err != nil {
		syscall.Close(fd)
		return fmt.Errorf("epoll_ctl add failed: %w", err)
	}

	m.mu.Lock()
	m.vmToFD[vmID] = fd
	m.fdToVM[fd] = vmID
	m.mu.Unlock()

	log.Printf("[QMP] VM %s connected and registered (fd=%d)", vmID, fd)
	return nil
}

func (m *QMPManager) RemoveVM(vmID string) {
	m.mu.Lock()
	fd, ok := m.vmToFD[vmID]
	if !ok {
		m.mu.Unlock()
		log.Printf("[QMP] RemoveVM: VM %s not found", vmID)
		return
	}
	delete(m.vmToFD, vmID)
	delete(m.fdToVM, fd)
	m.mu.Unlock()

	syscall.EpollCtl(m.epfd, syscall.EPOLL_CTL_DEL, fd, nil)
	syscall.Close(fd)

	log.Printf("[QMP] VM %s removed and socket closed", vmID)
}

func (m *QMPManager) SendCommand(vmID string, cmd map[string]interface{}) (map[string]interface{}, error) {
	m.mu.Lock()
	fd, ok := m.vmToFD[vmID]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("VM %s not found", vmID)
	}

	id := uuid.New().String()
	cmd["id"] = id
	ch := make(chan map[string]interface{}, 1)

	m.mu.Lock()
	m.cmdResponses[id] = ch
	m.mu.Unlock()

	data, _ := json.Marshal(cmd)
	data = append(data, '\n')
	if _, err := syscall.Write(fd, data); err != nil {
		return nil, fmt.Errorf("write failed: %w", err)
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout waiting for response")
	}
}

func (m *QMPManager) eventLoop() {
	for {
		n, err := syscall.EpollWait(m.epfd, m.events[:], -1)
		if err != nil {
			log.Printf("[QMP] epoll_wait error: %v", err)
			continue
		}

		for i := range n {
			fd := int(m.events[i].Fd)
			ev := m.events[i].Events

			m.mu.Lock()
			vmID, ok := m.fdToVM[fd]
			m.mu.Unlock()

			if !ok {
				log.Printf("[QMP] event received for unknown fd: %d", fd)
				continue
			}

			if ev&(syscall.EPOLLERR|syscall.EPOLLHUP) != 0 {
				// Do not remove the VM just yet — read() might still return buffered messages.
				log.Printf("[QMP] EPOLLERR | EPOLLHUP on VM %s (fd=%d), checking socket...", vmID, fd)
			}

			bufPtr := m.bufPool.Get().(*[]byte)
			buf := *bufPtr
			n, err := syscall.Read(fd, buf)
			if err != nil || n <= 0 {
				log.Printf("[QMP] read error or socket closed on VM %s: %v", vmID, err)

				m.RemoveVM(vmID)
				if m.handler != nil {
					go m.handler.OnQMPEvent(vmID, "QMP_SOCKET_CLOSED", nil)
				}
				m.bufPool.Put(bufPtr)
				continue
			}

			var msg map[string]interface{}
			if err := json.Unmarshal(buf[:n], &msg); err != nil {
				log.Printf("[QMP] invalid JSON from VM %s: %v", vmID, err)
				m.bufPool.Put(bufPtr)
				continue
			}

			m.bufPool.Put(bufPtr)
			m.dispatchRawMessage(vmID, msg)
		}
	}
}

func (m *QMPManager) dispatchRawMessage(vmID string, msg map[string]interface{}) {
	if idVal, ok := msg["id"]; ok {
		id := fmt.Sprintf("%v", idVal)

		m.mu.Lock()
		ch, found := m.cmdResponses[id]
		if found {
			ch <- msg
			delete(m.cmdResponses, id)
		}
		m.mu.Unlock()
		return
	}

	if event, ok := msg["event"].(string); ok {
		if event == "SHUTDOWN" {
			m.RemoveVM(vmID)
		}
		data := map[string]interface{}{}
		if d, ok := msg["data"].(map[string]interface{}); ok {
			data = d
		}
		if m.handler != nil {
			go m.handler.OnQMPEvent(vmID, event, data)
		}
	}
}

func writeQMPMapCommand(fd int, cmd map[string]interface{}) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = syscall.Write(fd, data)
	return err
}

func readQMPLine(fd int) ([]byte, error) {
	var data []byte
	buf := make([]byte, 1024)

	for {
		n, err := syscall.Read(fd, buf)
		if err != nil {
			return nil, err
		}
		data = append(data, buf[:n]...)
		if n > 0 && buf[n-1] == '\n' {
			break
		}
	}
	return data, nil
}

func jsonContainsReturn(data []byte) bool {
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return false
	}
	_, ok := result["return"]
	return ok
}
