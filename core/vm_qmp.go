package vmmanager

import (
	"errors"
	"fmt"
	"log"
	"math"
	"time"
	"vmmanager/utils"
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

// SendQMPCommand wraps the QMP command send for a VM.
func (v *VMManager) SendQMPCommand(vmID string, context map[string]interface{}) (interface{}, error) {
	// Optional tracing/logging
	start := time.Now()

	vm, exists := v.vmList[vmID]

	// Handle VM not found
	if !exists {
		log.Printf("[VM %s] Not found in VM list", vmID)
		return nil, fmt.Errorf("VM not found")
	}

	// Check if QMP is connected
	if vm.qmpState != QMPConnected {
		log.Printf("[VM %s] QMP is not connected. Cannot execute command.", vmID)
		return nil, fmt.Errorf("QMP not connected for VM %s", vmID)
	}

	// Ensure the command is in the context
	cmd, ok := context["command"].(map[string]interface{})
	if !ok {
		log.Printf("[VM %s] Invalid 'command' in context", vmID)
		return nil, fmt.Errorf("invalid or missing QMP command in context")
	}

	// Send the command via QMP
	resp, err := v.qmp.SendCommand(vmID, cmd)
	if err != nil {
		log.Printf("[VM %s] QMP command error: %v", vmID, err)
		return nil, err
	}

	// Log the command duration
	duration := time.Since(start)
	log.Printf("[VM %s] QMP command succeeded in %s", vmID, duration)

	// Convert response based on type
	switch val := resp.(type) {
	case map[string]interface{}:
		// If the response is a map, return it directly
		return val, nil
	case []interface{}:
		// If the response is a list, wrap it in a map with the "return" key
		return map[string]interface{}{"list": val}, nil
	default:
		return nil, fmt.Errorf("unexpected response type: %T", val)
	}
}
