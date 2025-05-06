package vmmanager

import (
	"fmt"
	"log"
	"time"
)

// CreateVM Create a VM instance from the manager
func (v *VMManager) CreateVM(vmID string) error {
	vm, exists := v.vmList[vmID]
	if exists {
		// Optionally, check VM state before deciding to start
		log.Printf("VM %s already exists, skipping creation", vmID)
		return fmt.Errorf("VM %s already exists", vmID)
	}
	// Create
	newVM := &VMInstance{}
	v.vmList[vmID] = newVM
	vm.State = StateRunning
	vm.qmpState = QMPState(QMPDisconnected)

	return nil

}

// DeleteVM deletes a VM instance from the manager
func (v *VMManager) DeleteVM(vmID string) error {
	v.qmp.RemoveVM(vmID)
	delete(v.vmList, vmID)
	return nil
}

func (v *VMManager) GetStatus(vmID string) (string, error) {
	vm, exists := v.vmList[vmID]
	if !exists {
		return "", fmt.Errorf("VM %s not found", vmID)
	}
	vm.vmMutex.Lock()
	state := string(vm.State)
	vm.vmMutex.Lock()
	return state, nil
}

func (v *VMManager) StartVM(vmID string) error {
	vm, exists := v.vmList[vmID]
	if !exists {
		return fmt.Errorf("VM %s not found", vmID)
	}

	if vm.State == StateRespawn {
		log.Printf("[VM %s] Respawn in progress. No operation performed.", vmID)
		return nil
	}

	vm.State = StateRespawn

	// Wait for QEMU process to be detected
	for {
		if isQemuRunning(vmID) {
			break
		}
		log.Printf("[VM %s] Waiting for VM to be ready...", vmID)
		time.Sleep(10 * time.Second)
	}

	// Update state to running
	vm.State = StateRunning

	// Attempt QMP attachment with retries
	err := v.attachToQMPWithRetry(vmID, 10, 1*time.Second)
	if err != nil {
		log.Printf("[VM %s] QMP attach failed: %v", vmID, err)
		return err
	}

	log.Printf("[VM %s] Started and QMP attached", vmID)
	return nil
}

// StopVM gracefully stops the VM
func (v *VMManager) StopVM(vmID string) error {

	vm, exists := v.vmList[vmID]
	if !exists {
		return fmt.Errorf("VM %s not found", vmID)
	}
	vm.qmpState = QMPState(QMPDisconnected)
	vm.State = StateStopped
	v.qmp.RemoveVM(vmID)
	return nil
}
