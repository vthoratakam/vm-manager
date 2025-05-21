package vmmanager

import (
	"fmt"
	"log"
)

func (v *VMManager) reconcileVM(vmID string, event ReconcileEvent, _ map[string]interface{}) (map[string]interface{}, error) {

	vm := v.vmList[vmID]

	log.Printf("[VM %s] Reconciling due to: %d", vmID, event)

	// Prevent duplicate
	if vm.reconcileEvent == event || (event == QMP_EVENT_SOCKET_CLOSED && vm.reconcileEvent == QMP_EVENT_SHUTDOWN) {
		log.Printf("[VM %s] Skipping reconcile (already processing %d)", vmID, event)
		return map[string]interface{}{"return": "Already processing this event"}, nil
	}

	vm.reconcileEvent = event

	var response map[string]interface{}
	var err error

	switch event {
	case VM_MANAGER_START:
		if isQemuRunning(vmID) {
			go v.StartVM(vmID)
		}
		response = map[string]interface{}{"return": "VM started"}

	case QMP_EVENT_SHUTDOWN, QMP_EVENT_SOCKET_CLOSED:
		v.StopVM(vmID)
		response = map[string]interface{}{"return": "VM stopped due to QMP event"}
	case CONTROL_EVENT_DELETE_VM:
		v.DeleteVM(vmID)
		response = map[string]interface{}{"return": "VM deleted"}

	case CONTROL_EVENT_START_VM:
		if vm.State != StateRunning {
			go v.StartVM(vmID)
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

	vm.reconcileEvent = EVENT_UNKNOWN
	return response, err
}
