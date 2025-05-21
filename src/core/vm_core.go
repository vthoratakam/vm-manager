package vmmanager

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"vmmanager/proto"
	"vmmanager/src/qmpmanager"
)

type VMManager struct {
	vmList map[string]*VMInstance
	qmp    *qmpmanager.QMPManager
}

var (
	vmmanager   *VMManager
	managerOnce sync.Once
)

// Singleton
func GetVMManagerInstance() *VMManager {
	managerOnce.Do(func() {
		vmmanager = &VMManager{
			vmList: make(map[string]*VMInstance),
			qmp:    qmpmanager.GetManager(),
		}
	})

	vmmanager.qmp.SetHandler(vmmanager)
	vmmanager.detectExistingVMs()
	//go monitorGoroutines()
	return vmmanager
}

// Auto-detect already running VMs based on QMP monitor socket
func (v *VMManager) detectExistingVMs() {
	log.Printf("detectExistingVMs \n")
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
		v.vmList[vmID] = &VMInstance{State: StateStopped, qmpState: QMPDisconnected}
		go v.reconcileVM(vmID, VM_MANAGER_START, nil)
	}
}

func FromProtoEvent(evt proto.ControlEventType) ReconcileEvent {
	switch evt {
	case proto.ControlEventType_CONTROL_EVENT_CREATE_VM:
		return CONTROL_EVENT_CREATE_VM
	case proto.ControlEventType_CONTROL_EVENT_DELETE_VM:
		return CONTROL_EVENT_DELETE_VM
	case proto.ControlEventType_CONTROL_EVENT_START_VM:
		return CONTROL_EVENT_START_VM
	case proto.ControlEventType_CONTROL_EVENT_STOP_VM:
		return CONTROL_EVENT_STOP_VM
	case proto.ControlEventType_CONTROL_EXECUTE_QMP_CMD:
		return CONTROL_EVENT_EXECUTE_QMP_CMD
	case proto.ControlEventType_CONTROL_GET_VM_STATUS:
		return CONTROL_EVENT_GET_VM_STATUS
	default:
		return EVENT_UNKNOWN
	}
}

func (v *VMManager) HandleEvent(vmID string, event proto.ControlEventType, context map[string]interface{}) (map[string]interface{}, error) {

	reconcileEvent := FromProtoEvent(event)

	vm, exist := v.vmList[vmID]
	if !exist && reconcileEvent != CONTROL_EVENT_CREATE_VM {

		return nil, fmt.Errorf("VM Not Exist %s", vmID)

	}

	var rawResult map[string]interface{}
	var err error

	if reconcileEvent == CONTROL_EVENT_EXECUTE_QMP_CMD {
		rawResult, err = v.SendQMPCommand(vmID, context)
	} else if reconcileEvent == CONTROL_EVENT_GET_VM_STATUS {
		rawResult = map[string]interface{}{"return": string(vm.State)}
	} else if reconcileEvent == CONTROL_EVENT_CREATE_VM {
		err = v.CreateVM(vmID)
		if err == nil {
			rawResult = map[string]interface{}{"return": "VM created"}
		}
	} else {
		rawResult, err = v.reconcileVM(vmID, reconcileEvent, context)
	}

	if err != nil {
		return nil, err
	}

	return rawResult, nil
}
