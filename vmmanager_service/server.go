package grpcserver

import (
	"context"
	"log"

	vmmanager "vmmanager/core"
	"vmmanager/grpcapi"
)

type VMManagerServer struct {
	grpcapi.UnimplementedVMManagerServer
	vmmanager *vmmanager.VMManager
}

func NewVMManagerServer(mgr *vmmanager.VMManager) *VMManagerServer {
	return &VMManagerServer{
		vmmanager: mgr,
	}
}

// HandleControlEvents routes control events based on the enum and context
func (s *VMManagerServer) HandleControlEvents(ctx context.Context, req *grpcapi.VMRequest) (*grpcapi.VMResponse, error) {
	vmID := req.GetVmId()
	event := req.GetControlEvent()
	contextMap := req.GetControlContext().AsMap()

	log.Printf("[VM %s] Received control event: %s", vmID, event.String())

	// Call core logic in your manager with enum
	return s.vmmanager.HandleEvent(vmID, event, contextMap)

}
