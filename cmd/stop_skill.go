package cmd

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var stopSkillOutput string

var stopSkillCmd = &cobra.Command{
	Use:        "skill-stop",
	Short:      "Stop the background skill subscription watcher",
	Long:       help.SkillStop.FormatForCLI("nacos-cli"),
	Deprecated: "use 'nacos-cli skill-sync stop' instead",
	Args:       cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		outputDir := resolveSkillOutputDir(stopSkillOutput)

		pid, err := skill.LoadWatcherPID(outputDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load %s: %v\n", skill.SkillsWatcherPIDFile, err)
			os.Exit(1)
		}
		if pid == 0 {
			fmt.Println("No background skill watcher is recorded.")
			return
		}
		if !skill.IsProcessRunning(pid) {
			if err := skill.ClearWatcherPID(outputDir, pid); err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to clear stale %s: %v\n", skill.SkillsWatcherPIDFile, err)
				os.Exit(1)
			}
			fmt.Printf("No running watcher found. Removed stale pid file (pid: %d).\n", pid)
			return
		}

		proc, err := os.FindProcess(pid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to find watcher process %d: %v\n", pid, err)
			os.Exit(1)
		}
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to stop watcher process %d: %v\n", pid, err)
			os.Exit(1)
		}

		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if !skill.IsProcessRunning(pid) {
				if err := skill.ClearWatcherPID(outputDir, pid); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to clear %s: %v\n", skill.SkillsWatcherPIDFile, err)
				}
				fmt.Printf("Stopped background skill watcher (pid: %d).\n", pid)
				return
			}
			time.Sleep(200 * time.Millisecond)
		}

		fmt.Printf("Stop signal sent to background skill watcher (pid: %d).\n", pid)
		fmt.Printf("If it is still running, check log: %s\n", skill.WatcherLogPath(outputDir))
	},
}

func init() {
	stopSkillCmd.Flags().StringVarP(&stopSkillOutput, "output", "o", "", "Output directory (default: ~/.skills)")
	_ = stopSkillCmd.MarkFlagDirname("output")
	rootCmd.AddCommand(stopSkillCmd)
}
