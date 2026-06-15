package skill

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

const (
	// SkillsWatcherPIDFile records the background subscription watcher process ID.
	SkillsWatcherPIDFile = "skills-watcher.pid"
	// SkillsWatcherLogFile records background subscription watcher output.
	SkillsWatcherLogFile = "skills-watcher.log"
)

// WatcherPIDPath returns the subscription watcher PID file path for a skill output directory.
func WatcherPIDPath(dir string) string {
	return filepath.Join(dir, SkillsWatcherPIDFile)
}

// WatcherLogPath returns the subscription watcher log file path for a skill output directory.
func WatcherLogPath(dir string) string {
	return filepath.Join(dir, SkillsWatcherLogFile)
}

// LoadWatcherPID reads the background watcher PID from disk.
// A missing PID file means no watcher is recorded and returns 0, nil.
func LoadWatcherPID(dir string) (int, error) {
	data, err := os.ReadFile(WatcherPIDPath(dir))
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
		return 0, fmt.Errorf("invalid watcher pid %q", raw)
	}
	return pid, nil
}

// SaveWatcherPID writes the background watcher PID to disk.
func SaveWatcherPID(dir string, pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid watcher pid %d", pid)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(WatcherPIDPath(dir), []byte(fmt.Sprintf("%d\n", pid)), 0644)
}

// ClearWatcherPID removes the watcher PID file.
// If expectedPID is greater than zero, the file is removed only when it still
// points at that PID.
func ClearWatcherPID(dir string, expectedPID int) error {
	if expectedPID > 0 {
		pid, err := LoadWatcherPID(dir)
		if err != nil {
			return err
		}
		if pid != 0 && pid != expectedPID {
			return nil
		}
	}

	err := os.Remove(WatcherPIDPath(dir))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// IsProcessRunning reports whether a process is currently reachable.
func IsProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, os.ErrPermission)
}

// IsSyncDaemonProcess reports whether pid points to the skill-sync daemon,
// guarding against stale PID files after the OS reuses a process ID.
func IsSyncDaemonProcess(pid int) bool {
	if !IsProcessRunning(pid) {
		return false
	}
	cmdline, err := processCommandLine(pid)
	if err != nil {
		return false
	}
	return isSyncDaemonCommand(cmdline)
}

func processCommandLine(pid int) (string, error) {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
		if err == nil && len(data) > 0 {
			return strings.ReplaceAll(string(data), "\x00", " "), nil
		}
	}

	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func isSyncDaemonCommand(cmdline string) bool {
	fields := strings.Fields(cmdline)
	hasSkillSync := false
	hasStart := false
	hasForeground := false
	for _, field := range fields {
		switch field {
		case "skill-sync":
			hasSkillSync = true
		case "start":
			hasStart = true
		case "--foreground":
			hasForeground = true
		}
		if strings.HasPrefix(field, "--foreground=") {
			hasForeground = true
		}
	}
	return hasSkillSync && hasStart && hasForeground
}
