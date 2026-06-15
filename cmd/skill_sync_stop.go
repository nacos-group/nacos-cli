package cmd

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var skillSyncStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background sync daemon",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		pid, err := skill.LoadSyncDaemonPID()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load daemon PID: %v\n", err)
			os.Exit(1)
		}
		if pid == 0 {
			fmt.Println("No sync daemon is recorded.")
			return
		}
		if !skill.IsSyncDaemonProcess(pid) {
			if err := skill.ClearSyncDaemonPID(pid); err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to clear stale PID: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("No running daemon found. Removed stale PID file (pid: %d).\n", pid)
			return
		}

		proc, err := os.FindProcess(pid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to find process %d: %v\n", pid, err)
			os.Exit(1)
		}
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to stop daemon (pid: %d): %v\n", pid, err)
			os.Exit(1)
		}

		// Wait for process to exit
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if !skill.IsProcessRunning(pid) {
				if err := skill.ClearSyncDaemonPID(pid); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to clear PID file: %v\n", err)
				}
				fmt.Printf("Stopped sync daemon (pid: %d).\n", pid)
				return
			}
			time.Sleep(200 * time.Millisecond)
		}

		fmt.Printf("Stop signal sent to sync daemon (pid: %d).\n", pid)
		logPath, _ := skill.GetSyncDaemonLogPath()
		if logPath != "" {
			fmt.Printf("Check log: %s\n", logPath)
		}
	},
}

func init() {
	skillSyncCmd.AddCommand(skillSyncStopCmd)
}
