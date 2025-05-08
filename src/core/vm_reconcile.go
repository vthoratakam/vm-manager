package vmmanager

import (
	"fmt"
	"log"
)

func (v *VMManager) reconcileVM(vmID string, event ReconcileEvent, context map[string]interface{}) (map[string]interface{}, error) {
	vm, exists := v.vmList[vmID]

	// Handle VM not found (except for create event)
	if !exists && event != CONTROL_EVENT_CREATE_VM {
		log.Printf("[VM %s] Not found in VM list", vmID)
		return map[string]interface{}{"return": "VM not found and event is not CREATE"}, fmt.Errorf("VM not found and event is not CREATE")
	}

	// Special handling for create — skip locking as VM is not yet in the list
	if event == CONTROL_EVENT_CREATE_VM {
		v.CreateVM(vmID)
		return map[string]interface{}{"return": "VM created"}, nil
	}

	// Prevent duplicate or conflicting reconciliations
	if vm.reconcileEvent == event || (event == QMP_EVENT_SOCKET_CLOSED && vm.reconcileEvent == QMP_EVENT_SHUTDOWN) {
		log.Printf("[VM %s] Skipping reconcile (already processing %d)", vmID, event)
		return map[string]interface{}{"return": "Already processing this event"}, nil
	}

	vm.reconcileEvent = event
	log.Printf("[VM %s] Reconciling due to: %d", vmID, event)

	var response map[string]interface{}
	var err error

	switch event {
	case VM_MANAGER_START:
		if isQemuRunning(vmID) {
			v.StartVM(vmID)
		}
		response = map[string]interface{}{"return": "VM started"}

	case QMP_EVENT_SHUTDOWN, QMP_EVENT_SOCKET_CLOSED:
		v.StopVM(vmID)
		response = map[string]interface{}{"return": "VM stopped due to QMP event"}

	case CONTROL_EVENT_DELETE_VM:
		v.DeleteVM(vmID)
		response = map[string]interface{}{"return": "VM deleted"}

	case CONTROL_EVENT_START_VM:
		if vm.State == StateStopped {
			v.StartVM(vmID)
		}
		response = map[string]interface{}{"return": "VM started"}

	case CONTROL_EVENT_STOP_VM:
		v.StopVM(vmID)
		response = map[string]interface{}{"return": "VM stopped"}

	default:
		log.Printf("[VM %s] Unknown event enum: %d", vmID, event)
		response = map[string]interface{}{"return": fmt.Sprintf("Unknown event: %d", event)}
		err = fmt.Errorf("unknown event: %d", event)
	}

	// Always return the response and the error (if any) at the end
	vm.reconcileEvent = EVENT_UNKNOWN
	return response, err
}
