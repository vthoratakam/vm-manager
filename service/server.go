package grpcserver

import (
	"context"
	"encoding/json"
	"log"
	"vmmanager/proto"
	vmmanager "vmmanager/src/core"
)

type VMManagerServer struct {
	proto.UnimplementedVMManagerServer
	vmmanager *vmmanager.VMManager
}

func NewVMManagerServer(mgr *vmmanager.VMManager) *VMManagerServer {
	return &VMManagerServer{
		vmmanager: mgr,
	}
}

func buildErrorResponse(err error) (*proto.VMResponse, error) {
	log.Printf("[HandleEvent error]: %v", err)
	return &proto.VMResponse{
		Success: false,
		Result:  []byte(err.Error()),
	}, nil
}

func (s *VMManagerServer) HandleControlEvents(ctx context.Context, req *proto.VMRequest) (*proto.VMResponse, error) {
	vmID := req.GetVmId()
	event := req.GetControlEvent()
	context := req.GetControlContext()

	log.Printf("[VM %s] Received control event: %s, context: %s", vmID, event.String(), string(context))

	var jsonContext map[string]interface{}
	if err := json.Unmarshal(context, &jsonContext); err != nil {

		return buildErrorResponse(err)
	}

	result, err := s.vmmanager.HandleEvent(vmID, event, jsonContext)

	if err != nil {
		return buildErrorResponse(err)
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return buildErrorResponse(err)
	}
	if event != proto.ControlEventType_CONTROL_EXECUTE_QMP_CMD {
		log.Printf("[VM %s] Response for control event: %s, result: %s \n", vmID, event.String(), string(jsonBytes))
	}

	return &proto.VMResponse{
		Success: true,
		Result:  jsonBytes,
	}, nil

}
