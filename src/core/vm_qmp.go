package vmmanager

import (
	"errors"
	"fmt"
	"log"
	"math"
	"time"
	"vmmanager/src/utils"
)

func isQemuRunning(vmID string) bool {
	pid, err := utils.GetPIDforVMID(vmID)
	if err != nil || pid <= 0 {
		log.Printf("[VM %s] Not active. PID not found.", vmID)
		return false
	}

	if !utils.IsProcessActive(pid) {
		log.Printf("[VM %s] Not active. Process not running.", vmID)
		return false
	}

	if utils.IsStopped(pid) {
		return false
	}

	return true
}

func (v *VMManager) attachToQMPWithRetry(vmID string, maxRetries int, baseDelay time.Duration) error {

	var err error
	v.vmList[vmID].qmpState = QMPDisconnected
	for i := 0; i < maxRetries; i++ {
		err = v.qmp.AddVM(vmID)
		if err == nil {
			v.vmList[vmID].qmpState = QMPConnected
			log.Printf("[VM %s] Attached to QMP successfully (attempt %d)", vmID, i+1)
			return nil
		}

		delay := time.Duration(math.Pow(1.5, float64(i))) * baseDelay
		log.Printf("[VM %s] QMP attach failed (attempt %d/%d): %v. Retrying in %v...", vmID, i+1, maxRetries, err, delay)
		time.Sleep(delay)
	}
	return errors.New("QMP attach failed after max retries: " + err.Error())
}

func FromQMPEvent(name string) ReconcileEvent {
	switch name {
	case "SHUTDOWN":
		return QMP_EVENT_SHUTDOWN
	case "QMP_SOCKET_CLOSED":
		return QMP_EVENT_SOCKET_CLOSED
	default:
		return EVENT_UNKNOWN
	}
}

// QMP event handler callback
func (v *VMManager) OnQMPEvent(vmID string, event string, data map[string]interface{}) {
	log.Printf("[VM %s] QMP event received: %s", vmID, event)
	go v.reconcileVM(vmID, FromQMPEvent(event), data)

}

func (v *VMManager) SendQMPCommand(vmID string, context map[string]interface{}) (map[string]interface{}, error) {

	vm := v.vmList[vmID]

	// Check if QMP is connected
	if vm.qmpState != QMPConnected {
		return nil, fmt.Errorf("QMP not connected for VM %s", vmID)
	}

	resp, err := v.qmp.SendQMPCommand(vmID, context)

	// Handle error from QMP/HMP command
	if err != nil {
		log.Printf("[VM %s] QMP/HMP command error: %v", vmID, err)
		return nil, err
	}

	return resp.(map[string]interface{}), nil
}
