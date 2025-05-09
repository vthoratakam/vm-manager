# VMManager

VMManager is a daemon service for managing the full lifecycle of virtual machines (VMs). It provides a control interface via a Unix domain socket (`/run/vmmanager.sock`) and will supports operations like creating, deleting, starting, and stopping VMs. Built with a finite state machine (FSM)-based reconciliation loop, it ensures robust event handling and state management.

## Features
- **Unix Socket Interface**: Communicates over `/run/vmmanager.sock` using gRPC.
- **Supported Operations**:
  - Create, delete, start, and stop VMs.
  - Execute QEMU Machine Protocol (QMP) commands.
  - Query VM status.
- **Respawnable Daemon**: Reconstructs VM state from `/linodes` directory on restart.
- **QMPManager**: Handles QEMU interactions via QMP, adhering to the Single Responsibility Principle (SRP).
  - Manages QMP socket connections, command execution, and asynchronous event notifications.
  - Uses an epoll-based event loop for efficient monitoring.

## Architecture
- **VMManager**:
  - Centralized FSM-driven reconciliation loop processes all events (from clients or QMP).
  - Coordinates with QMPManager for QEMU interactions.
- **QMPManager**:
  - Establishes QMP socket connections and negotiates capabilities.
  - Sends asynchronous QMP commands and processes responses/events.
  - Notifies VMManager of events (e.g., device hotplug, shutdown) via callbacks.

## Cometlab devenv Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/vthoratakam/vm-manager.git
   cd vmmanager
   ```
2. Install dependencies (requires Go 1.20+):
   ```bash
   go mod tidy
   ```
3. Generate Proto:
   ```bash
   
   protoc --go_out=../ --go-grpc_out=../ proto/vmmanager.proto
   
    ```
4. Build the binary static binary cometlab:
   ```bash
   
   docker run --rm -v "$PWD":/app -w /app golang:1.24-alpine sh -c "\
   apk add --no-cache musl-dev gcc && \
   go build -ldflags '-linkmode external -extldflags \"-static\"' -o vmmanager cmd/main.go"
   
   ```
5. Copy Binary to /vbin 
   ```bash
   
   cp vmmanager cometlab-setup/devenv/repos/vbin
   
   ```

## Usage
1. Start the VMManager daemon:
   ```bash
   ./vmmanager
   ```
   - Runs as a daemon, listening on `/run/vmmanager.sock`.
   - Scans `/linodes` to restore VM states on startup.
     
2. Interact using a gRPC client (e.g., VBIN):
   
   ```bash
   Linode::VMManager::Client->vmmanager_send_rpc_event($user, "CONTROL_EXECUTE_QMP_CMD", $command);
   
   Linode::VMManager::Client->vmmanager_send_rpc_event($user, "CONTROL_GET_VM_STATUS", {});
   ```
   - Supported commands:
   `
    CONTROL_EVENT_UNKNOWN   => 0,
    CONTROL_EVENT_CREATE_VM => 1,
    CONTROL_EVENT_DELETE_VM => 2,
    CONTROL_EVENT_START_VM  => 3,
    CONTROL_EVENT_STOP_VM   => 4,
    CONTROL_EXECUTE_QMP_CMD => 5,
    CONTROL_GET_VM_STATUS   => 6
   `.

