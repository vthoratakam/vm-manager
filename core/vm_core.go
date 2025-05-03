package vmmanager

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"vmmanager/grpcapi"
	"vmmanager/qmpmanager"

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

func (v *VMManager) HandleEvent(vmID string, event grpcapi.ControlEventType, context map[string]interface{}) (*grpcapi.VMResponse, error) {
	reconcileEvent := FromProtoEvent(event)

	// Declare result and err
	var result interface{}
	var err error

	// Call the reconcile function
	if reconcileEvent == CONTROL_EVENT_EXECUTE_QMP_CMD {
		// For QMP execution, use SendQMPCommand
		result, err = v.SendQMPCommand(vmID, context)
	} else {
		// For other events, use reconcileVM
		result, err = v.reconcileVM(vmID, reconcileEvent, context)
	}

	// Error handling: set Success to false and handle the error properly
	if err != nil {
		log.Printf("HandleEvent error: %v", err)

		// Create an error Struct for the response
		errorStruct, _ := structpb.NewStruct(map[string]interface{}{
			"error": err.Error(),
		})
		return &grpcapi.VMResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to handle event %s for VM %s", event.String(), vmID),
			Result:  errorStruct,
		}, nil
	}

	// Convert result (map) to protobuf Struct
	resultStruct, err := structpb.NewStruct(result.(map[string]any))
	if err != nil {
		log.Printf("Error converting result to struct: %v", err)
		errorStruct, _ := structpb.NewStruct(map[string]interface{}{
			"error": "Failed to convert result to Struct",
		})
		return &grpcapi.VMResponse{
			Success: false,
			Message: "Error processing result",
			Result:  errorStruct,
		}, nil
	}

	// Return successful response with Success = true
	return &grpcapi.VMResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully handled %s for VM %s", event.String(), vmID),
		Result:  resultStruct,
	}, nil
}
