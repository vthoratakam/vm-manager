package utils

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// GetPIDforVMID reads the PID file for a given VM and returns the process ID.
func GetPIDforVMID(vmID string) (int, error) {
	pidFilePath := fmt.Sprintf("/linodes/%s/run/qemu.pid", vmID)
	data, err := ioutil.ReadFile(pidFilePath)
	if err != nil {
		return 0, fmt.Errorf("failed to read PID file %s: %v", pidFilePath, err)
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file %s: %v", pidFilePath, err)
	}

	return pid, nil
}

// IsProcessActive checks if a process is running by sending signal 0.
func IsProcessActive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// IsZombie checks if a process is in zombie (Z) state.
func IsZombie(pid int) bool {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := ioutil.ReadFile(statPath)
	if err != nil {
		return false
	}
	parts := strings.Split(string(data), " ")
	if len(parts) > 2 {
		return parts[2] == "Z"
	}
	return false
}

// IsStopped checks if a process is in stopped (T) state.
func IsStopped(pid int) bool {
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := ioutil.ReadFile(statPath)
	if err != nil {
		return false
	}
	parts := strings.Split(string(data), " ")
	if len(parts) > 2 {
		return parts[2] == "T"
	}
	return false
}

// CmdlineMatchesPattern checks if the process cmdline contains a given pattern.
func CmdlineMatchesPattern(pid int, pattern string) bool {
	cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
	data, err := ioutil.ReadFile(cmdlinePath)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), pattern)
}

// IsZombieLinodeOff matches Perl logic to check zombie + cmdline mismatch.
func IsZombieLinodeOff(vmID string, pid int, pattern string) bool {
	if !CmdlineMatchesPattern(pid, pattern) {
		log.Printf("[VM %s] Possible zombie. Cmdline didn't match expected pattern.", vmID)

		for zombieWait := 0; zombieWait < 5 && IsZombie(pid); zombieWait++ {
			log.Printf("[VM %s] Waiting %ds for zombie reaping...", vmID, zombieWait+1)
			time.Sleep(time.Duration(zombieWait+1) * time.Second)
		}

		if IsZombie(pid) {
			log.Printf("[VM %s] Still in zombie state after wait.", vmID)
			return true
		}
	}
	return false
}
