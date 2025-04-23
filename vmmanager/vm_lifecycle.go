package vmmanager

import (
	"fmt"
	"log"
	"time"
)

func (v *VMManager) StartVM(vmID string) error {
	vm, exists := v.vmList[vmID]
	if !exists {
		return fmt.Errorf("VM %s not found", vmID)
	}

	// VM reboot time
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

	v.qmp.RemoveVM(vmID)
	return nil
}

// ForceStopVM forcefully stops the VM (e.g., kill the process)
func (v *VMManager) ForceStopVM(vmID string) error {
	// Implement logic to force stop a VM
	return nil
}

// CreateVM Create a VM instance from the manager
func (v *VMManager) CreateVM(vmID string) error {
	// Implement logic to Create a VM
	v.vmList[vmID] = &VMInstance{}
	v.StartVM(vmID)
	return nil
}

// DeleteVM deletes a VM instance from the manager
func (v *VMManager) DeleteVM(vmID string) error {
	// Implement logic to remove a VM
	return nil
}

func (v *VMManager) GetStatus(vmID string) (string, error) {
	vm, exists := v.vmList[vmID]
	if !exists {
		return "", fmt.Errorf("VM %s not found", vmID)
	}
	return string(vm.State), nil
}

// ListVMs lists all the VMs under management and queries their status
func (v *VMManager) ListVMs() []*VMInstance {
	return nil
}
