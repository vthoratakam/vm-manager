package vmmanager

import (
	"govbin/qmpmanager"
	"log"
	"path/filepath"
	"strings"
	"sync"
)

type VMManager struct {
	vmMutex map[string]*sync.Mutex
	vmList  map[string]*VMInstance
	qmp     *qmpmanager.QMPManager
}

var (
	vmmanager   *VMManager
	managerOnce sync.Once
)

// Singleton
func GetVMManagerInstance() *VMManager {
	managerOnce.Do(func() {
		vmmanager = &VMManager{
			vmMutex: make(map[string]*sync.Mutex),
			vmList:  make(map[string]*VMInstance),
			qmp:     qmpmanager.GetManager(),
		}
	})

	vmmanager.qmp.SetHandler(vmmanager)
	vmmanager.detectExistingVMs()
	return vmmanager
}

// Auto-detect already running VMs based on QMP monitor socket
func (v *VMManager) detectExistingVMs() {
	matches, err := filepath.Glob("/linodes/*/run/qemu.monitor")
	if err != nil {
		log.Printf("Failed to scan QMP sockets: %v", err)
		return
	}

	for _, path := range matches {
		parts := strings.Split(path, "/")
		if len(parts) < 3 {
			continue
		}
		vmID := parts[2]
		v.vmMutex[vmID] = &sync.Mutex{}
		v.vmList[vmID] = &VMInstance{}
		go v.reconcileVM(vmID, "VM_MANAGER_START")
	}
}
