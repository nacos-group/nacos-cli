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
	syncStartForeground          bool
	syncStartInterval            string
	syncStartAll                 bool
	syncStartUseRemoteOnConflict bool
	syncStartRefresh             bool
	syncStartLabel               string
	syncStartNoAutoUpload        bool
)

var skillSyncStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Initial sync and start the background daemon/watcher",
	Long: `Run the first-time sync and start the background sync process.

In Nacos mode: pulls every subscribed skill from Nacos and links it to the
agents. Conflicts (local content differs from Nacos) are skipped and reported;
resolve them with 'skill-sync resolve <skill>'. Use --all to also subscribe
to every skill on Nacos. Use --use-remote-on-conflict to overwrite local on
conflict (with backup).

In local mode: links every skill in ~/.nacos-cli/skill-repo to the agents.
Use --all to also reverse-import unmanaged skills found in agent directories.`,
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

		// Load state
		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		// Mode resolution
		override := skill.ModeOverrideNone
		if profileName != "" {
			override = skill.ModeOverrideNacos
		}
		modeRes, err := skill.ResolveSyncMode(state, skill.ResolveModeOptions{
			Override:    override,
			ProfileHint: profileName,
			Interactive: !syncStartForeground,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Ensure agents
		if err := ensureAgents(state); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Apply --label override
		if syncStartLabel != "" {
			state.Label = syncStartLabel
		}
		// Apply --no-auto-upload toggle
		if syncStartNoAutoUpload {
			state.Config.AutoUpload = false
		}

		initOpts := startInitOptions{
			All:                 syncStartAll,
			UseRemoteOnConflict: syncStartUseRemoteOnConflict,
			Refresh:             syncStartRefresh,
		}

		// Initial sync based on mode (first-run logic)
		if modeRes.Mode == skill.SyncModeLocal {
			if !runLocalInitialSync(state, initOpts) {
				return
			}
		} else if modeRes.Mode == skill.SyncModeNacos && !syncStartForeground {
			if !runNacosInitialSync(state, initOpts) {
				return
			}
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
		_, _, err = startSyncDaemonBackground()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to start sync daemon: %v\n", err)
			os.Exit(1)
		}

		printSyncStatusSummary(state)
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

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] Error: failed to ensure skill repo: %v\n", timeNow(), err)
		return
	}

	changed := false

	for name, entry := range state.Skills {
		// Ensure token is valid
		if err := skillService.Client().EnsureTokenValid(); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] Auth error for %s: %v\n", timeNow(), name, err)
			continue
		}

		localChanged, preLocalHash, dirtyAgents := detectLocalChanges(name, state.Agents, entry.SyncedHash)

		// Query with current MD5 for conditional download
		result, err := skillService.FetchSkill(name, "", state.Label, entry.RemoteMd5)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] Error polling %s: %v\n", timeNow(), name, err)
			continue
		}

		fmt.Printf("[%s] Poll: %s label=%s sentMd5=%s updated=%v deleted=%v newMd5=%s newVersion=%s\n",
			timeNow(), name, state.Label, shortHash(entry.RemoteMd5), result.Updated, result.Deleted,
			shortHash(result.Md5), result.ResolvedVersion)

		// Workaround: when server returns 304 but the resolved version differs from
		// what we have, the label has been pointed to a new version. Re-query without
		// md5 to force a full download.
		if !result.Updated && !result.Deleted && result.ResolvedVersion != "" &&
			entry.ResolvedVersion != "" && result.ResolvedVersion != entry.ResolvedVersion {
			fmt.Printf("[%s] Label moved (%s → %s), forcing re-pull for %s\n",
				timeNow(), entry.ResolvedVersion, result.ResolvedVersion, name)
			result, err = skillService.FetchSkill(name, "", state.Label, "")
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] Error force-pulling %s: %v\n", timeNow(), name, err)
				continue
			}
		}

		if result.Deleted {
			fmt.Printf("[%s] Deleted: %s (removed from server)\n", timeNow(), name)
			delete(state.Skills, name)
			changed = true
			continue
		}

		if result.Updated {
			sourceDir, cleanup, err := stageFetchedSkill(name, result.ZipBytes)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] Error staging %s: %v\n", timeNow(), name, err)
				continue
			}

			newHash, err := skill.ComputeDirectoryHash(sourceDir)
			if err != nil {
				cleanup()
				fmt.Fprintf(os.Stderr, "[%s] Error hashing staged %s: %v\n", timeNow(), name, err)
				continue
			}

			if shouldProtectLocalFromRemote(entry.Status) {
				cleanup()
				entry = keepLocalAfterRemoteUpdate(entry, preLocalHash, result)
				fmt.Printf("[%s] Remote update pending: %s (kept %s)\n",
					timeNow(), name, entry.Status.DisplayString())
			} else if localChanged {
				// Conflict: both sides changed before pull
				cleanup()
				entry.RemoteMd5 = result.Md5
				entry.ResolvedVersion = result.ResolvedVersion
				entry.LocalHash = preLocalHash
				entry.Status = skill.SyncStatusConflict
				entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				fmt.Printf("[%s] Conflict: %s (local modified in %s + remote updated)\n",
					timeNow(), name, strings.Join(dirtyAgents, ", "))
			} else {
				// Safe to auto-pull: update the central repo, then link agents to it.
				if err := applyRemoteUpdateToRepoAndAgents(repoPath, name, sourceDir, state.Agents); err != nil {
					cleanup()
					fmt.Fprintf(os.Stderr, "[%s] Error applying %s: %v\n", timeNow(), name, err)
					continue
				}
				cleanup()

				entry.RemoteMd5 = result.Md5
				entry.ResolvedVersion = result.ResolvedVersion
				entry.LocalHash = newHash
				entry.SyncedHash = newHash
				entry.Status = skill.SyncStatusSynced
				entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
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

		}

		// Check lifecycle for uploaded skills after remote polling. This also
		// handles the case where latest moved while local Uploaded content is
		// protected from auto-pull.
		if state.Skills[name].Status == skill.SyncStatusUploaded {
			if skill.TryAutoTransitionToSynced(state, name, skillService) {
				if state.Skills[name].Status == skill.SyncStatusSynced {
					fmt.Printf("[%s] Published: %s (auto-synced)\n", timeNow(), name)
				}
				changed = true
			}
		}

		// Auto-upload evaluation: only when in Nacos mode and a repo path exists
		if state.Mode == skill.SyncModeNacos && repoPath != "" {
			current := state.Skills[name]
			eval, err := skill.EvaluateAutoUpload(state, &current, repoPath, skillService)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] auto-upload eval error for %s: %v\n", timeNow(), name, err)
				continue
			}
			switch eval.Decision {
			case skill.AutoUploadShouldUpload:
				fmt.Printf("[%s] Auto-upload: %s (uploading...)\n", timeNow(), name)
				if err := skill.PerformAutoUpload(state, &current, repoPath, skillService, eval.CurrentHash); err != nil {
					fmt.Fprintf(os.Stderr, "[%s] auto-upload failed for %s: %v\n", timeNow(), name, err)
					continue
				}
				changed = true
			case skill.AutoUploadDebouncing:
				// debounce: persist pending hash but don't yet upload
				state.Skills[name] = current
				changed = true
			case skill.AutoUploadBlockedReviewing, skill.AutoUploadBlockedForeignDraft:
				blockedChanged := current.Status != skill.SyncStatusUploadBlocked ||
					current.BlockedDraftVersion != eval.RemoteEditing ||
					current.BlockedReviewVersion != eval.RemoteReviewing
				if blockedChanged {
					current.Status = skill.SyncStatusUploadBlocked
					current.BlockedDraftVersion = eval.RemoteEditing
					current.BlockedReviewVersion = eval.RemoteReviewing
					current.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
					state.Skills[name] = current
					changed = true
					fmt.Printf("[%s] Upload blocked: %s (%s)\n", timeNow(), name, eval.Reason)
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

func shouldProtectLocalFromRemote(status skill.SyncStatus) bool {
	return status == skill.SyncStatusLocalChanges ||
		status == skill.SyncStatusUploaded ||
		status == skill.SyncStatusUploadBlocked
}

func keepLocalAfterRemoteUpdate(entry skill.SyncSkillEntry, localHash string, result *skill.SkillQueryResult) skill.SyncSkillEntry {
	if result.Md5 != "" {
		entry.RemoteMd5 = result.Md5
	}
	if result.ResolvedVersion != "" {
		entry.ResolvedVersion = result.ResolvedVersion
	}
	if localHash != "" {
		entry.LocalHash = localHash
	}
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return entry
}

func applyRemoteUpdateToRepoAndAgents(repoPath, name, sourceDir string, agents []skill.AgentDir) error {
	if err := backupRepoDir(repoPath, name); err != nil {
		return fmt.Errorf("backup repo dir: %w", err)
	}
	if err := skill.ImportAgentSkillToRepo(repoPath, sourceDir, name); err != nil {
		return fmt.Errorf("write remote skill to repo: %w", err)
	}
	if _, err := skill.LinkSkillForce(repoPath, name, agents, nil); err != nil {
		return fmt.Errorf("link remote skill: %w", err)
	}
	return nil
}

func detectLocalChanges(skillName string, agents []skill.AgentDir, syncedHash string) (bool, string, []string) {
	var dirtyAgents []string
	primaryHash := ""
	for idx, agent := range agents {
		skillDir := filepath.Join(agent.Path, skillName)
		localHash, _ := skill.ComputeDirectoryHash(skillDir)
		if idx == 0 {
			primaryHash = localHash
		}
		if localHash != "" && syncedHash != "" && localHash != syncedHash {
			dirtyAgents = append(dirtyAgents, agent.Name)
		}
	}
	return len(dirtyAgents) > 0, primaryHash, dirtyAgents
}

func stageFetchedSkill(skillName string, zipBytes []byte) (string, func(), error) {
	if len(zipBytes) == 0 {
		return "", nil, fmt.Errorf("empty skill ZIP")
	}
	stageRoot, err := os.MkdirTemp("", "nacos-skill-sync-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(stageRoot)
	}
	if err := skill.ExtractSkillZip(zipBytes, stageRoot); err != nil {
		cleanup()
		return "", nil, err
	}
	sourceDir := filepath.Join(stageRoot, skillName)
	info, err := os.Stat(sourceDir)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("staged skill directory not found: %w", err)
	}
	if !info.IsDir() {
		cleanup()
		return "", nil, fmt.Errorf("staged skill path is not a directory: %s", sourceDir)
	}
	return sourceDir, cleanup, nil
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
	skillSyncStartCmd.Flags().BoolVar(&syncStartAll, "all", false, "Pull every available skill (Nacos: namespace-wide; Local: also reverse-import unmanaged)")
	skillSyncStartCmd.Flags().BoolVar(&syncStartUseRemoteOnConflict, "use-remote-on-conflict", false, "Overwrite local content with remote on conflict (backup first)")
	skillSyncStartCmd.Flags().BoolVar(&syncStartRefresh, "refresh", false, "Force re-pull every subscribed skill (alias for treating local as out-of-date)")
	skillSyncStartCmd.Flags().StringVar(&syncStartLabel, "label", "", "Override the tracking label for this invocation (Nacos mode only)")
	skillSyncStartCmd.Flags().BoolVar(&syncStartNoAutoUpload, "no-auto-upload", false, "Disable daemon-driven auto-upload")
	skillSyncCmd.AddCommand(skillSyncStartCmd)
}
