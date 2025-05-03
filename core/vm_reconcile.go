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
		return nil, fmt.Errorf("VM not found and event is not CREATE")
	}

	// Special handling for create — skip locking as VM is not yet in the list
	if event == CONTROL_EVENT_CREATE_VM {
		v.CreateVM(vmID)
		return map[string]interface{}{"info": "VM created"}, nil
	}

	vm.vmMutex.Lock()
	defer func() {
		vm.reconcileEvent = EVENT_UNKNOWN
		vm.vmMutex.Unlock()
	}()

	// Prevent duplicate or conflicting reconciliations
	if vm.reconcileEvent == event || (event == QMP_EVENT_SOCKET_CLOSED && vm.reconcileEvent == QMP_EVENT_SHUTDOWN) {
		log.Printf("[VM %s] Skipping reconcile (already processing %d)", vmID, event)
		return map[string]interface{}{"info": "already processing"}, nil
	}

	vm.reconcileEvent = event
	log.Printf("[VM %s] Reconciling due to: %d", vmID, event)

	switch event {
	case VM_MANAGER_START:
		v.StartVM(vmID)
		return map[string]interface{}{"info": "VM created and started"}, nil

	case QMP_EVENT_SHUTDOWN, QMP_EVENT_SOCKET_CLOSED:
		vm.State = StateStopped
		vm.qmpState = QMPState(QMPDisconnected)
		v.StopVM(vmID)
		return map[string]interface{}{"info": "VM stopped due to QMP event"}, nil

	case CONTROL_EVENT_DELETE_VM:
		v.DeleteVM(vmID)
		return map[string]interface{}{"info": "VM deleted"}, nil

	case CONTROL_EVENT_START_VM:
		v.StartVM(vmID)
		return map[string]interface{}{"info": "VM started"}, nil

	case CONTROL_EVENT_STOP_VM:
		v.StopVM(vmID)
		return map[string]interface{}{"info": "VM stopped"}, nil

	default:
		log.Printf("[VM %s] Unknown event enum: %d", vmID, event)
		return nil, fmt.Errorf("unknown event: %d", event)
	}
}
