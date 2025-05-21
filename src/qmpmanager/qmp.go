package qmpmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

const (
	maxEpollEvents       = 32
	qmpReadBufferSize    = 262144
	maxConnectionRetries = 3
	retryDelay           = 500 * time.Millisecond
	commandTimeout       = 10 * time.Second
)

type QMPEventHandler interface {
	OnQMPEvent(vmID, event string, data map[string]interface{})
}

type QMPManager struct {
	mu              sync.Mutex
	qmpcmdResponses map[string]chan map[string]interface{}
	vmToFD          map[string]int
	fdToVM          map[int]string
	epfd            int
	events          [maxEpollEvents]syscall.EpollEvent
	handler         QMPEventHandler
	bufPool         sync.Pool
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

var (
	manager *QMPManager
	once    sync.Once
)

func GetManager() *QMPManager {
	once.Do(func() {
		manager = newQMPManager()
		manager.start()
	})
	return manager
}

func newQMPManager() *QMPManager {
	ctx, cancel := context.WithCancel(context.Background())
	epfd, err := syscall.EpollCreate1(0)
	if err != nil {
		log.Fatalf("epoll_create1 failed: %v", err)
	}

	m := &QMPManager{
		qmpcmdResponses: make(map[string]chan map[string]interface{}),
		vmToFD:          make(map[string]int),
		fdToVM:          make(map[int]string),
		epfd:            epfd,
		ctx:             ctx,
		cancel:          cancel,
		bufPool: sync.Pool{
			New: func() interface{} {
				b := make([]byte, qmpReadBufferSize)
				return &b
			},
		},
	}

	return m
}

func (m *QMPManager) start() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.eventLoop()
	}()
}

func (m *QMPManager) Shutdown() {
	m.cancel()
	m.mu.Lock()
	for vmID := range m.vmToFD {
		m.removeVMUnlocked(vmID)
	}
	m.mu.Unlock()
	syscall.Close(m.epfd)
	m.wg.Wait()
}

func (m *QMPManager) SetHandler(handler QMPEventHandler) {
	m.mu.Lock()
	m.handler = handler
	m.mu.Unlock()
}

func (m *QMPManager) AddVM(vmID string) error {
	var lastErr error
	for attempt := 0; attempt < maxConnectionRetries; attempt++ {
		fd, err := m.connectVM(vmID)
		if err == nil {
			if err := m.registerVM(vmID, fd); err == nil {
				log.Printf("[QMP %s] connected (fd=%d, attempt=%d)", vmID, fd, attempt+1)
				return nil
			}
			syscall.Close(fd)
		}
		lastErr = err
		time.Sleep(retryDelay)
	}
	return fmt.Errorf("failed to add VM %s after %d attempts: %w", vmID, maxConnectionRetries, lastErr)
}

func (m *QMPManager) connectVM(vmID string) (int, error) {
	socketPath := fmt.Sprintf("/linodes/%s/run/qemu.monitor", vmID)
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return 0, fmt.Errorf("socket creation failed: %w", err)
	}

	addr := &syscall.SockaddrUnix{Name: socketPath}
	if err := syscall.Connect(fd, addr); err != nil {
		syscall.Close(fd)
		return 0, fmt.Errorf("connect failed: %w", err)
	}

	if err := m.initializeQMPSession(fd); err != nil {
		syscall.Close(fd)
		return 0, err
	}

	return fd, nil
}

func (m *QMPManager) initializeQMPSession(fd int) error {
	greeting, err := readQMPLine(fd)
	if err != nil {
		return fmt.Errorf("greeting read failed: %w", err)
	}

	var msg map[string]interface{}
	if err := json.Unmarshal(greeting, &msg); err != nil {
		return fmt.Errorf("invalid QMP greeting: %w", err)
	}

	capCmd := map[string]interface{}{
		"execute": "qmp_capabilities",
	}
	if err := writeQMPMapCommand(fd, capCmd); err != nil {
		return fmt.Errorf("sending capabilities failed: %w", err)
	}

	capResp, err := readQMPLine(fd)
	if err != nil || !jsonContainsReturn(capResp) {
		return fmt.Errorf("capabilities response invalid or missing: %s", string(capResp))
	}

	return nil
}

func (m *QMPManager) registerVM(vmID string, fd int) error {
	event := syscall.EpollEvent{Events: syscall.EPOLLIN | syscall.EPOLLHUP | syscall.EPOLLERR, Fd: int32(fd)}
	if err := syscall.EpollCtl(m.epfd, syscall.EPOLL_CTL_ADD, fd, &event); err != nil {
		return fmt.Errorf("epoll_ctl add failed: %w", err)
	}

	m.mu.Lock()
	m.vmToFD[vmID] = fd
	m.fdToVM[fd] = vmID
	m.mu.Unlock()
	return nil
}

func (m *QMPManager) RemoveVM(vmID string) {
	m.mu.Lock()
	m.removeVMUnlocked(vmID)
	m.mu.Unlock()
}

func (m *QMPManager) removeVMUnlocked(vmID string) {
	fd, ok := m.vmToFD[vmID]
	if !ok {
		log.Printf("[QMP %s] RemoveVM: not found", vmID)
		return
	}
	delete(m.vmToFD, vmID)
	delete(m.fdToVM, fd)

	syscall.EpollCtl(m.epfd, syscall.EPOLL_CTL_DEL, fd, nil)
	syscall.Close(fd)
	log.Printf("[QMP %s] removed and socket closed", vmID)
}

func (m *QMPManager) SendQMPCommand(vmID string, cmd map[string]interface{}) (interface{}, error) {
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
	m.qmpcmdResponses[id] = ch
	m.mu.Unlock()

	if err := writeQMPMapCommand(fd, cmd); err != nil {
		m.mu.Lock()
		delete(m.qmpcmdResponses, id)
		m.mu.Unlock()
		return nil, fmt.Errorf("write failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(m.ctx, commandTimeout)
	defer cancel()

	select {
	case resp := <-ch:

		if errInfo, ok := resp["error"].(map[string]interface{}); ok {
			return nil, fmt.Errorf("%v: %v", errInfo["class"], errInfo["desc"])
		}
		if _, ok := resp["return"]; ok {
			delete(resp, "id")
			return resp, nil
		}
		return nil, fmt.Errorf("invalid response format: missing 'return'")
	case <-ctx.Done():
		m.mu.Lock()
		delete(m.qmpcmdResponses, id)
		m.mu.Unlock()
		return nil, fmt.Errorf("timeout waiting for response")
	}
}

func (m *QMPManager) eventLoop() {
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
			n, err := syscall.EpollWait(m.epfd, m.events[:], -1)

			if err == syscall.EINTR {
				continue // retry on EINTR
			}

			if err != nil {
				log.Printf("[QMP] epoll_wait error: %v", err)
				continue
			}

			for i := 0; i < n; i++ {
				m.handleEvent(int(m.events[i].Fd), m.events[i].Events)
			}
		}
	}
}

func (m *QMPManager) handleEvent(fd int, events uint32) {
	m.mu.Lock()
	vmID, ok := m.fdToVM[fd]
	m.mu.Unlock()
	if !ok {
		log.Printf("[QMP] event received for unknown fd: %d", fd)
		return
	}

	if events&(syscall.EPOLLERR|syscall.EPOLLHUP) != 0 {
		log.Printf("[QMP %s] EPOLLERR | EPOLLHUP (fd=%d)", vmID, fd)
		m.RemoveVM(vmID)
		if m.handler != nil {
			go m.handler.OnQMPEvent(vmID, "QMP_SOCKET_CLOSED", nil)
		}
		return
	}

	bufPtr := m.bufPool.Get().(*[]byte)
	defer m.bufPool.Put(bufPtr)
	buf := *bufPtr

	n, err := syscall.Read(fd, buf)
	if err != nil || n <= 0 {
		log.Printf("[QMP %s] read error or socket closed on %v", vmID, err)
		m.RemoveVM(vmID)
		if m.handler != nil {
			go m.handler.OnQMPEvent(vmID, "QMP_SOCKET_CLOSED", nil)
		}
		return
	}

	var msg map[string]interface{}
	if err := json.Unmarshal(buf[:n], &msg); err != nil {
		log.Printf("[QMP %s] invalid JSON %s", vmID, err)
		return
	}

	m.dispatchRawMessage(vmID, msg)
}

func (m *QMPManager) dispatchRawMessage(vmID string, msg map[string]interface{}) {
	if idVal, ok := msg["id"]; ok {
		id := fmt.Sprintf("%v", idVal)
		m.mu.Lock()
		if ch, found := m.qmpcmdResponses[id]; found {
			select {
			case ch <- msg:
			default:
				log.Printf("[QMP %s] response channel for command %s is full", vmID, id)
			}
			delete(m.qmpcmdResponses, id)
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
		return fmt.Errorf("json marshal failed: %w", err)
	}
	data = append(data, '\n')
	if _, err := syscall.Write(fd, data); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	return nil
}

func readQMPLine(fd int) ([]byte, error) {
	var data []byte
	buf := make([]byte, 1024)
	for {
		n, err := syscall.Read(fd, buf)
		if err != nil {
			return nil, fmt.Errorf("read failed: %w", err)
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
