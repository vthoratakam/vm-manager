package grpcserver

import (
	"context"
	"fmt"
	"log"

	"govbin/grpcapi" // Replace with your correct import path
	"govbin/vmmanager"
)

// VMManagerServer is the server that implements the VMManager service defined in the .proto file
type VMManagerServer struct {
	grpcapi.UnimplementedVMManagerServer
	vmmanager *vmmanager.VMManager
}

func NewVMManagerServer(mgr *vmmanager.VMManager) *VMManagerServer {
	return &VMManagerServer{
		vmmanager: mgr,
	}
}

// StartVM starts a VM based on vm_id
func (s *VMManagerServer) StartVM(ctx context.Context, req *grpcapi.VMRequest) (*grpcapi.VMResponse, error) {
	// Your logic to start the VM goes here
	log.Printf("Starting VM: %s", req.GetVmId())
	return &grpcapi.VMResponse{
		Message: fmt.Sprintf("VM %s started", req.GetVmId()),
		Success: true,
	}, nil
}

// StopVM stops a VM based on vm_id
func (s *VMManagerServer) StopVM(ctx context.Context, req *grpcapi.VMRequest) (*grpcapi.VMResponse, error) {
	// Your logic to stop the VM goes here
	log.Printf("Stopping VM: %s", req.GetVmId())
	return &grpcapi.VMResponse{
		Message: fmt.Sprintf("VM %s stopped", req.GetVmId()),
		Success: true,
	}, nil
}

func (s *VMManagerServer) GetVMStatus(ctx context.Context, req *grpcapi.VMRequest) (*grpcapi.VMStatus, error) {
	vmID := req.GetVmId()
	log.Printf("Getting status of VM: %s", vmID)

	status, err := s.vmmanager.GetStatus(vmID)
	if err != nil {
		log.Printf("Error getting status of VM %s: %v", vmID, err)
		return nil, err
	}

	return &grpcapi.VMStatus{
		VmId:  vmID,
		State: status,
	}, nil
}

func (s *VMManagerServer) SendQMPCommand(ctx context.Context, req *grpcapi.QMPCommandRequest) (*grpcapi.QMPCommandResponse, error) {
	vmID := req.GetVmId()

	//check the vmstate qmp state
	cmdMap := make(map[string]interface{})

	for key, val := range req.GetCommand() {
		cmdMap[key] = val
	}

	resp, err := s.vmmanager.SendQMPCommand(vmID, cmdMap)
	if err != nil {
		log.Printf("Failed to send QMP command to VM %s: %v", vmID, err)
		return &grpcapi.QMPCommandResponse{
			Success: false,
			Response: map[string]string{
				"error_class": "QMPManagerFailure",
				"error_desc":  err.Error(),
			},
		}, nil
	}

	if qmpErr, ok := resp["error"]; ok {
		// Attempt to extract class and desc if it's a map
		if errMap, ok := qmpErr.(map[string]interface{}); ok {
			errClass := fmt.Sprintf("%v", errMap["class"])
			errDesc := fmt.Sprintf("%v", errMap["desc"])
			return &grpcapi.QMPCommandResponse{
				Success: false,
				Response: map[string]string{
					"error_class": errClass,
					"error_desc":  errDesc,
				},
			}, nil
		}

		// Fallback: just return the whole error as string
		return &grpcapi.QMPCommandResponse{
			Success:  false,
			Response: map[string]string{"error": fmt.Sprintf("%v", qmpErr)},
		}, nil
	}

	// Return response as map[string]string
	responseMap := make(map[string]string)
	for k, v := range resp {
		responseMap[k] = fmt.Sprintf("%v", v)
	}

	return &grpcapi.QMPCommandResponse{
		Success:  true,
		Response: responseMap,
	}, nil
}
