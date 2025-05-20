package vmmanager

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	grpcapi "vmmanager/proto"
	"vmmanager/src/qmpmanager"

	"google.golang.org/protobuf/types/known/structpb"
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
		v.vmList[vmID] = &VMInstance{}
		v.vmList[vmID].State = StateStopped
		v.vmList[vmID].qmpState = QMPDisconnected
		go v.reconcileVM(vmID, VM_MANAGER_START, nil)
	}
}

func FromProtoEvent(evt grpcapi.ControlEventType) ReconcileEvent {
	switch evt {
	case grpcapi.ControlEventType_CONTROL_EVENT_CREATE_VM:
		return CONTROL_EVENT_CREATE_VM
	case grpcapi.ControlEventType_CONTROL_EVENT_DELETE_VM:
		return CONTROL_EVENT_DELETE_VM
	case grpcapi.ControlEventType_CONTROL_EVENT_START_VM:
		return CONTROL_EVENT_START_VM
	case grpcapi.ControlEventType_CONTROL_EVENT_STOP_VM:
		return CONTROL_EVENT_STOP_VM
	case grpcapi.ControlEventType_CONTROL_EXECUTE_QMP_CMD:
		return CONTROL_EVENT_EXECUTE_QMP_CMD
	case grpcapi.ControlEventType_CONTROL_GET_VM_STATUS:
		return CONTROL_EVENT_GET_VM_STATUS
	default:
		return EVENT_UNKNOWN
	}
}

func buildErrorResponse(err error) (*grpcapi.VMResponse, error) {
	log.Printf("HandleEvent error: %v", err)

	errorStruct, _ := structpb.NewStruct(map[string]interface{}{
		"error": err.Error(),
	})
	return &grpcapi.VMResponse{
		Success: false,
		Result:  errorStruct,
	}, nil
}

func (v *VMManager) HandleEvent(vmID string, event grpcapi.ControlEventType, context []byte) (*grpcapi.VMResponse, error) {

	reconcileEvent := FromProtoEvent(event)

	var rawResult map[string]interface{}
	var err error

	log.Printf("Raw context: %s", string(context))
	var jsonContext map[string]interface{}
	if err := json.Unmarshal(context, &jsonContext); err != nil {

		return buildErrorResponse(fmt.Errorf("invalid JSON context: %s", jsonContext))
	}

	if reconcileEvent == CONTROL_EVENT_EXECUTE_QMP_CMD {
		rawResult, err = v.SendQMPCommand(vmID, jsonContext)
	} else if reconcileEvent == CONTROL_EVENT_GET_VM_STATUS {
		vm, exist := v.vmList[vmID]
		if !exist {
			return buildErrorResponse(fmt.Errorf("VM Not Found: %s", vmID))
		}
		status := vm.State
		log.Printf("VM %s Status %s", vmID, status)
		resultStruct, err := structpb.NewStruct(map[string]interface{}{"text": string(status)})
		if err != nil {
			return buildErrorResponse(fmt.Errorf("failed to convert result to Struct: %v", err))
		}
		return &grpcapi.VMResponse{
			Success: true,
			Result:  resultStruct,
		}, nil
	} else {
		rawResult, err = v.reconcileVM(vmID, reconcileEvent, jsonContext)
	}

	if err != nil {
		return buildErrorResponse(err)
	}

	resultVal, ok := rawResult["return"]
	if !ok {
		return buildErrorResponse(fmt.Errorf("missing 'return' in result"))
	}

	var resultMap map[string]interface{}

	switch val := resultVal.(type) {
	case map[string]interface{}:
		resultMap = val
	case []interface{}:
		resultMap = map[string]interface{}{"list": val}
	case string:
		resultMap = map[string]interface{}{"text": val}
	default:
		return buildErrorResponse(fmt.Errorf("unexpected 'return' type: %T", val))
	}

	resultStruct, err := structpb.NewStruct(resultMap)
	if err != nil {
		return buildErrorResponse(fmt.Errorf("failed to convert result to Struct: %v", err))
	}

	return &grpcapi.VMResponse{
		Success: true,
		Result:  resultStruct,
	}, nil
}
