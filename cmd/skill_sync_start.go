package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nacos-group/nacos-cli/internal/config"
	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var (
	syncStartForeground bool
	syncStartInterval   string
)

var skillSyncStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the background sync daemon",
	Long: `Start the background sync daemon for continuous skill synchronization.

The daemon monitors Nacos for version changes on all subscribed skills,
automatically pulls updates when local state is Synced, and detects conflicts.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// In --foreground mode, this process IS the daemon. Skip the
		// running-daemon check to avoid the false-positive where the
		// parent has already written our own PID into the PID file.
		if !syncStartForeground {
			running, existingPID := skill.IsSyncDaemonRunning()
			if running {
				fmt.Printf("Skill sync daemon is already running (pid: %d).\n", existingPID)
				return
			}
		}

		// Load state to check subscriptions
		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		if len(state.Skills) == 0 {
			fmt.Println("No skills subscribed yet. Use 'skill-sync add' to subscribe.")
			fmt.Println("The daemon will start and pick up new subscriptions automatically.")
			fmt.Println()
		}

		// Parse interval
		interval, err := time.ParseDuration(syncStartInterval)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid interval %q: %v\n", syncStartInterval, err)
			os.Exit(1)
		}
		if interval < 5*time.Second {
			fmt.Fprintf(os.Stderr, "Error: interval must be at least 5s\n")
			os.Exit(1)
		}

		if syncStartForeground {
			runSyncDaemonForeground(state, interval)
			return
		}

		// Start background process
		pid, logPath, err := startSyncDaemonBackground()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to start sync daemon: %v\n", err)
			os.Exit(1)
		}

		// Display info consistent with status output
		fmt.Printf("Sync list source: local\n")
		fmt.Printf("Tracking label: %s\n", state.Label)
		fmt.Printf("Sync daemon: running (pid: %d)\n", pid)
		fmt.Println()

		if len(state.Skills) > 0 {
			fmt.Printf("Subscriptions: %s\n", strings.Join(state.GetSubscribedSkillNames(), ", "))
		}

		fmt.Printf("\n  Log: %s\n", logPath)
		fmt.Printf("  Stop: nacos-cli skill-sync stop\n")
	},
}

func runSyncDaemonForeground(state *skill.SyncState, interval time.Duration) {
	currentPID := os.Getpid()

	// Check for stale PID
	if existingPID, _ := skill.LoadSyncDaemonPID(); existingPID != 0 && existingPID != currentPID {
		if skill.IsProcessRunning(existingPID) {
			fmt.Fprintf(os.Stderr, "Error: sync daemon already running (pid: %d)\n", existingPID)
			os.Exit(1)
		}
	}

	// Save our PID
	if err := skill.SaveSyncDaemonPID(currentPID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save PID: %v\n", err)
	}
	defer func() {
		_ = skill.ClearSyncDaemonPID(currentPID)
	}()

	fmt.Printf("Tracking label: %s\n", state.Label)
	fmt.Printf("Subscriptions: %s\n", strings.Join(state.GetSubscribedSkillNames(), ", "))
	fmt.Printf("\nSync daemon running (foreground, interval: %s)...\n", interval)
	fmt.Printf("Press Ctrl+C to stop.\n\n")

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Printf("\nShutting down sync daemon...\n")
		cancel()
	}()

	// Create Nacos client
	nacosClient := mustNewNacosClient()
	skillService := skill.NewSkillService(nacosClient)

	// Initial poll
	syncPollOnce(skillService)

	// Poll loop
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Sync daemon stopped.")
			return
		case <-ticker.C:
			syncPollOnce(skillService)
		}
	}
}

func syncPollOnce(skillService *skill.SkillService) {
	state, err := skill.LoadSyncState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] Error: failed to load state: %v\n", timeNow(), err)
		return
	}

	if len(state.Skills) == 0 || len(state.Agents) == 0 {
		return
	}

	primaryDir := state.Agents[0].Path
	changed := false

	for name, entry := range state.Skills {
		// Ensure token is valid
		if err := skillService.Client().EnsureTokenValid(); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] Auth error for %s: %v\n", timeNow(), name, err)
			continue
		}

		// IMPORTANT: compute local hash BEFORE QuerySkill, because QuerySkill
		// will overwrite the local directory on a 200 response. We need to know
		// the pre-pull state to detect whether the user had local modifications.
		skillDir := filepath.Join(primaryDir, name)
		preLocalHash, _ := skill.ComputeDirectoryHash(skillDir)
		localChanged := preLocalHash != "" && entry.SyncedHash != "" && preLocalHash != entry.SyncedHash

		// Query with current MD5 for conditional download
		result, err := skillService.QuerySkill(name, primaryDir, "", state.Label, entry.RemoteMd5)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] Error polling %s: %v\n", timeNow(), name, err)
			continue
		}

		fmt.Printf("[%s] Poll: %s label=%s sentMd5=%s updated=%v deleted=%v newMd5=%s newVersion=%s\n",
			timeNow(), name, state.Label, shortHash(entry.RemoteMd5), result.Updated, result.Deleted,
			shortHash(result.Md5), result.ResolvedVersion)

		if result.Deleted {
			fmt.Printf("[%s] Deleted: %s (removed from server)\n", timeNow(), name)
			delete(state.Skills, name)
			changed = true
			continue
		}

		if result.Updated {
			// Remote changed (local has been overwritten by QuerySkill at this point)
			if localChanged {
				// Conflict: both sides changed before pull
				entry.RemoteMd5 = result.Md5
				entry.ResolvedVersion = result.ResolvedVersion
				entry.Status = skill.SyncStatusConflict
				fmt.Printf("[%s] Conflict: %s (local modified + remote updated)\n", timeNow(), name)
			} else {
				// Safe to auto-pull: copy to all other agents
				sourceDir := filepath.Join(primaryDir, name)
				if len(state.Agents) > 1 {
					_ = skill.EnsureSkillInAllAgents(name, sourceDir, state.Agents[1:])
				}

				newHash, _ := skill.ComputeDirectoryHash(sourceDir)
				entry.RemoteMd5 = result.Md5
				entry.ResolvedVersion = result.ResolvedVersion
				entry.LocalHash = newHash
				entry.SyncedHash = newHash
				entry.Status = skill.SyncStatusSynced
				fmt.Printf("[%s] Updated: %s → %s\n", timeNow(), name, result.ResolvedVersion)
			}

			state.Skills[name] = entry
			changed = true
		} else {
			// Not modified on remote. Update local status based on local changes.
			if localChanged && entry.Status != skill.SyncStatusLocalChanges &&
				entry.Status != skill.SyncStatusUploaded {
				entry.LocalHash = preLocalHash
				entry.Status = skill.SyncStatusLocalChanges
				state.Skills[name] = entry
				changed = true
			}

			// Check lifecycle for "uploaded" skills (Phase 3)
			if entry.Status == skill.SyncStatusUploaded {
				if skill.TryAutoTransitionToSynced(state, name, skillService) {
					fmt.Printf("[%s] Published: %s (auto-synced)\n", timeNow(), name)
					changed = true
				}
			}
		}
	}

	if changed {
		if err := skill.SaveSyncState(state); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] Error: failed to save state: %v\n", timeNow(), err)
		}
	}
}

func timeNow() string {
	return time.Now().Format("15:04:05")
}

func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	if h == "" {
		return "-"
	}
	return h
}

func startSyncDaemonBackground() (int, string, error) {
	// Clear stale PID if any
	if existingPID, _ := skill.LoadSyncDaemonPID(); existingPID != 0 {
		if !skill.IsProcessRunning(existingPID) {
			_ = skill.ClearSyncDaemonPID(existingPID)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		return 0, "", err
	}

	logPath, err := skill.GetSyncDaemonLogPath()
	if err != nil {
		return 0, "", err
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, "", err
	}
	defer logFile.Close()

	// Build args: rerun with --foreground
	args := []string{"skill-sync", "start", "--foreground", "--interval", syncStartInterval}

	// Pass through connection config so the child can authenticate.
	// The child process has no stdin, so it cannot prompt for missing config.
	// We must ensure it gets a complete config path or profile.
	if configFile != "" {
		args = append(args, "--config", configFile)
	} else {
		// Resolve the effective profile name (explicit or current default)
		effectiveProfile := profileName
		if effectiveProfile == "" {
			if current, err := config.GetCurrentProfile(); err == nil {
				effectiveProfile = current
			} else {
				effectiveProfile = config.DefaultProfile
			}
		}
		args = append(args, "--profile", effectiveProfile)
	}

	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	if scheme != "" && scheme != "http" {
		args = append(args, "--scheme", scheme)
	}
	if verbose {
		args = append(args, "--verbose")
	}

	cmd := exec.Command(executable, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return 0, "", err
	}

	pid := cmd.Process.Pid
	if err := skill.SaveSyncDaemonPID(pid); err != nil {
		_ = cmd.Process.Kill()
		return 0, "", err
	}
	if err := cmd.Process.Release(); err != nil {
		return 0, "", err
	}
	return pid, logPath, nil
}

func init() {
	skillSyncStartCmd.Flags().BoolVar(&syncStartForeground, "foreground", false, "Run in foreground instead of background")
	skillSyncStartCmd.Flags().StringVar(&syncStartInterval, "interval", "30s", "Poll interval (e.g. 10s, 1m)")
	skillSyncCmd.AddCommand(skillSyncStartCmd)
}

