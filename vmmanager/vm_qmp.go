package vmmanager

import (
	"errors"
	"fmt"
	"govbin/utils"
	"log"
	"math"
	"time"
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

	pattern := fmt.Sprintf("qemu-system/linodes/%s/", vmID)
	if utils.IsZombieLinodeOff(vmID, pid, pattern) {
		return false // 'Z' state
	}

	if utils.IsStopped(pid) {
		return false
	}

	// TODO: Call QMP to check status (e.g. "query-status")
	// If running/paused/inmigrate → return 1
	// If shutdown → return 0
	// If unknown → return 0

	return true
}

// Retry QMP attach with backoff
func (v *VMManager) attachToQMPWithRetry(vmID string, maxRetries int, baseDelay time.Duration) error {

	var err error
	v.vmList[vmID].qmpState = QMPState(QMPDisconnected)
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

// QMP event handler callback
func (v *VMManager) OnQMPEvent(vmID string, event string, data map[string]interface{}) {
	log.Printf("[VM %s] QMP event received: %s", vmID, event)
	if validReconcileEvents[event] {
		go v.reconcileVM(vmID, event)
	}
}

// SendQMPCommand wraps the QMP command send for a VM.
func (v *VMManager) SendQMPCommand(vmID string, cmd map[string]interface{}) (map[string]interface{}, error) {
	// Optional tracing/logging
	start := time.Now()
	resp, err := v.qmp.SendCommand(vmID, cmd)
	duration := time.Since(start)

	if err != nil {
		log.Printf("[VM] VM %s command failed in %s: %v", vmID, duration, err)
		return nil, err
	}

	log.Printf("[VM] VM %s command succeeded in %s", vmID, duration)
	return resp, nil
}
