package vmmanager

type VMInstance struct {
	Pid            int
	State          VMState // running, stopped, paused, respawn
	qmpState       QMPState
	reconcileEvent string
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

var validReconcileEvents = map[string]bool{
	"VM_MANAGER_START":     true,
	"SHUTDOWN":             true,
	"QMP_SOCKET_CLOSED":    true,
	"CONTROL_PLANE_CREATE": true,
	"CONTROL_PLANE_DELETE": true,
	"CONTROL_PLANE_START":  true,
	"CONTROL_PLANE_STOP":   true,
	"CONTROL_GET_STATUS":   true,
}

type VMStatus struct {
	VMID     string
	State    VMState  // e.g., "running", "stopped"
	QMPState QMPState // e.g., "connected", "disconnected"
}
