package vmcore

type VMConfig struct {
	ID            string
	Name          string
	CPUs          int
	Memory        int // in MB
	Disk          string
	MonitorSocket string
	NetBridge     string
}

type VMInstance struct {
	ID     string
	Config VMConfig
	Pid    int
	State  string // running, stopped, paused
}
