package vmmanager

import (
	"log"
)

// Main reconcile logic
func (v *VMManager) reconcileVM(vmID string, event string) {

	vm, ok := v.vmList[vmID]

	if !ok && vm.reconcileEvent != "CONTROL_PLANE_CREATE" {
		log.Printf("[VM %s] Not found in VM list", vmID)
		return
	}

	if vm.reconcileEvent == event || (event == "QMP_SOCKET_CLOSED" && vm.reconcileEvent == "SHUTDOWN") {
		log.Printf("[VM %s] Skipping reconcile (already processing %s)", vmID, vm.reconcileEvent)
		return
	} else {
		log.Printf("[VM %s] Reconciling due to: %s", vmID, event)
	}

	v.vmMutex[vmID].Lock()
	vm.reconcileEvent = event

	switch event {

	//coming from VM manager
	//handles both VMmanager and host restart
	case "VM_MANAGER_START":
		v.StartVM(vmID)
	//coming from QMP manager
	//handles shutdown and reboot
	case "SHUTDOWN":
		vm.State = StateStopped
		vm.qmpState = QMPState(QMPDisconnected)

	//coming from QMP manager
	//handles qmp socket closed and also vm dead case
	case "QMP_SOCKET_CLOSED":
		vm.State = StateStopped
		vm.qmpState = QMPState(QMPDisconnected)

	case "CONTROL_PLANE_CREATE":
		//start VM
		v.CreateVM(vmID)

	case "CONTROL_PLANE_DELETE":
		//stop VM
		v.DeleteVM(vmID)
	case "CONTROL_PLANE_START":
		//start VM
		v.StartVM(vmID)

	case "CONTROL_PLANE_STOP":
		//stop VM
		v.StopVM(vmID)

	}

	vm.reconcileEvent = ""
	v.vmMutex[vmID].Unlock()
}
