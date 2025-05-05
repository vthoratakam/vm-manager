package vmmanager

import "sync"

type VMInstance struct {
	vmMutex        sync.Mutex
	State          VMState // running, stopped, paused, respawn
	qmpState       QMPState
	reconcileEvent ReconcileEvent
}

type VMState string

const (
	StateRunning VMState = "running"
	StateStopped VMState = "stopped"
	StateCrashed VMState = "crashed"
	StatePaused  VMState = "paused"
	StateRespawn VMState = "respawn"
)

type DesiredState string

const (
	PowerOn  DesiredState = "PowerOn"
	PowerOff DesiredState = "PowerOff"
	Deleted  DesiredState = "Deleted"
)

type QMPState string

const (
	QMPConnected    QMPState     = "connected"
	QMPDisconnected DesiredState = "disconnected"
)

type ReconcileEvent int

const (
	EVENT_UNKNOWN ReconcileEvent = iota
	VM_MANAGER_START
	QMP_EVENT_SHUTDOWN
	QMP_EVENT_SOCKET_CLOSED
	CONTROL_EVENT_CREATE_VM
	CONTROL_EVENT_DELETE_VM
	CONTROL_EVENT_START_VM
	CONTROL_EVENT_STOP_VM
	CONTROL_EVENT_EXECUTE_QMP_CMD
	CONTROL_EVENT_GET_VM_STATUS
)
