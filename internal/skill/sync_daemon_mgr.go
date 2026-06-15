package skill

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// LoadSyncDaemonPID reads the sync daemon PID from the global PID file.
// Returns 0, nil if no PID file exists.
func LoadSyncDaemonPID() (int, error) {
	pidPath, err := GetSyncDaemonPIDPath()
	if err != nil {
		return 0, err
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return 0, nil
	}
	pid, err := strconv.Atoi(raw)
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid sync daemon pid %q", raw)
	}
	return pid, nil
}

// SaveSyncDaemonPID writes the sync daemon PID to the global PID file.
func SaveSyncDaemonPID(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid sync daemon pid %d", pid)
	}
	pidPath, err := GetSyncDaemonPIDPath()
	if err != nil {
		return err
	}
	return os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", pid)), 0644)
}

// ClearSyncDaemonPID removes the sync daemon PID file.
// If expectedPID > 0, only clears if the PID matches.
func ClearSyncDaemonPID(expectedPID int) error {
	if expectedPID > 0 {
		pid, err := LoadSyncDaemonPID()
		if err != nil {
			return err
		}
		if pid != 0 && pid != expectedPID {
			return nil // Different PID, don't clear
		}
	}

	pidPath, err := GetSyncDaemonPIDPath()
	if err != nil {
		return err
	}

	if err := os.Remove(pidPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// IsSyncDaemonRunning checks if the sync daemon is currently running.
// Returns (running, pid).
func IsSyncDaemonRunning() (bool, int) {
	pid, err := LoadSyncDaemonPID()
	if err != nil || pid == 0 {
		return false, 0
	}
	return IsSyncDaemonProcess(pid), pid
}
