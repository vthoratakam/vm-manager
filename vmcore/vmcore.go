package vmcore

import (
	"fmt"
	"log"
	"path/filepath"
	"qemu-socket-test/qmpmanager"
	"strings"
	"sync"
	"time"
)

type VMManager struct {
	mutex                sync.Mutex
	offlineVMscond       *sync.Cond
	offlineVMscond_mutex sync.Mutex
	onlineVMs            map[string]*VMInstance
	offlineVMs           map[string]*VMInstance
	qmp                  *qmpmanager.QMPManager
}

var (
	manager     *VMManager
	managerOnce sync.Once
)

func GetInstance() *VMManager {
	managerOnce.Do(func() {
		manager = &VMManager{
			onlineVMs:  make(map[string]*VMInstance),
			offlineVMs: make(map[string]*VMInstance),
			qmp:        qmpmanager.GetManager(),
		}
	})
	manager.offlineVMscond = sync.NewCond(&manager.offlineVMscond_mutex)
	manager.qmp.SetHandler(manager)
	manager.autoDetectVMs()
	go manager.retryOfflineVMs()
	return manager
}

func (v *VMManager) autoDetectVMs() {
	matches, err := filepath.Glob("/linodes/*/run/qemu.monitor")
	if err != nil {
		log.Printf("Failed to scan QMP sockets: %v", err)
		return
	}

	for _, path := range matches {
		// Extract VM ID from path
		parts := strings.Split(path, "/")
		if len(parts) < 3 {
			continue
		}
		vmID := parts[2]

		if err := v.qmp.AddVM(vmID); err != nil {
			v.offlineVMs[vmID] = &VMInstance{ID: vmID}
			delete(v.onlineVMs, vmID)
		} else {
			v.onlineVMs[vmID] = &VMInstance{ID: vmID}
			delete(v.offlineVMs, vmID)
			log.Printf("Detected and connected to VM: %s", vmID)
		}

	}
}

func (v *VMManager) retryOfflineVMs() {
	for {
		v.offlineVMscond_mutex.Lock()

		// Wait until there is at least one offline VM
		for len(v.offlineVMs) == 0 {
			v.offlineVMscond.Wait()
		}

		for vmID := range v.offlineVMs {
			v.offlineVMscond_mutex.Unlock() // unlock during dial attempt
			err := v.qmp.AddVM(vmID)
			v.offlineVMscond_mutex.Lock()

			if err == nil {
				v.onlineVMs[vmID] = v.offlineVMs[vmID]
				delete(v.offlineVMs, vmID)
				fmt.Printf("VM %s is now online\n", vmID)
			}
		}

		v.offlineVMscond_mutex.Unlock()
		time.Sleep(10 * time.Second)
	}
}

func (v *VMManager) markVMOffline(vmID string) {
	fmt.Printf("marking vm offline %s\n", vmID)
	if _, exists := v.offlineVMs[vmID]; !exists {
		v.offlineVMs[vmID] = &VMInstance{ID: vmID}
		v.offlineVMscond.Signal() // wake the retry goroutine
	}
}

func (m *VMManager) OnQMPEvent(linodeID string, event string, data map[string]interface{}) {
	// Handle the event here
	fmt.Printf("Received QMPManager event for VM %s: %s\n", linodeID, event)
	if event == "SHUTDOWN" {
		//this could be reboot or powerdown VM remove old qmp connection
		m.qmp.RemoveVM(linodeID)
	}
	if event == "QMPSOCKREMOVE" {
		m.markVMOffline(linodeID)
	}
}

// Create a new VM instance
func (v *VMManager) CreateVM(config VMConfig) error {
	//based on config create new vm
	//try to add this VM to qmpmanager
	//accordingly put it inside online offline map
	return nil
}

// Start an existing VM
func (v *VMManager) StartVM(id string) error {
	return nil
}

// Stop a running VM
func (v *VMManager) StopVM(id string) error {
	return nil
}

// Forcefully stop a VM (kill process)
func (v *VMManager) ForceStopVM(id string) error {
	return nil
}

// Update an existing VM's configuration
func (v *VMManager) UpdateVM(id string, newConfig VMConfig) error {
	return nil
}

// Get details about a specific VM
func (v *VMManager) GetVM(id string) (*VMInstance, error) {
	return nil, nil
}

// List all known VMs
func (v *VMManager) ListVMs() []*VMInstance {

	cmd := map[string]interface{}{"execute": "query-block"}
	resp, err := v.qmp.SendCommand("linode506948", cmd)
	if err != nil {
		log.Printf("Command error: %v", err)
	} else {
		log.Printf("Response: %v", resp)
	}

	return nil

}

// Delete a VM from the system
func (v *VMManager) DeleteVM(id string) error {
	return nil
}

// Check and refresh VM status (from QEMU or PID)
func (v *VMManager) RefreshStatus(id string) error {
	return nil
}
