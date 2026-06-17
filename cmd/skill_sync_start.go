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
	syncStartUseRemoteOnConflict bool
	syncStartRefresh             bool
	syncStartLabel               string
	syncStartNoAutoUpload        bool
	syncStartNonInteract         bool
)

const (
	syncDaemonLogMaxBytes = 10 * 1024 * 1024
	syncDaemonLogBackups  = 3
)

var skillSyncStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Initial sync; start the daemon in Nacos mode",
	Long: `Run the first-time sync. In Nacos mode, also start the background sync process.
In local mode, only link repo skills into agent directories.

In Nacos mode: pulls every added skill from Nacos and links it to the
agents. Conflicts (local content differs from Nacos) are skipped and reported;
resolve them with 'skill-sync resolve <skill>'. Use --use-remote-on-conflict
to overwrite local on conflict (with backup).

Use --non-interactive in scripts or Agent calls. It disables prompts; start
records and skips conflicts unless --use-remote-on-conflict is provided.

In local mode: links every skill in ~/.nacos-cli/skill-repo to the agents.
Local mode does not start a background daemon; symlinks keep agents pointed at
the central repo after the initial link step.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := ensureSkillSyncProfileReady(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

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
			Interactive: !syncStartForeground && !syncStartNonInteract,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		state.Mode = modeRes.Mode
		state.Profile = skill.CurrentSyncProfile()

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
			UseRemoteOnConflict: syncStartUseRemoteOnConflict,
			Refresh:             syncStartRefresh,
		}

		// Initial sync based on mode (first-run logic)
		if modeRes.Mode == skill.SyncModeLocal {
			if !runLocalInitialSync(state, initOpts) {
				return
			}
			printSyncStatusSummary(state)
			return
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
		if skill.IsSyncDaemonProcess(existingPID) {
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
	fmt.Printf("Added skills: %s\n", strings.Join(state.GetSubscribedSkillNames(), ", "))
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

		// Query with current MD5 for conditional download. If the server does
		// not provide MD5s, use the resolved label version as a cheap guard to
		// avoid downloading and re-applying immutable content every cycle.
		result, skippedByVersion := skipFetchWhenVersionUnchanged(skillService, name, state.Label, entry)
		if !skippedByVersion {
			result, err = skillService.FetchSkill(name, "", state.Label, entry.RemoteMd5)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] Error polling %s: %v\n", timeNow(), name, err)
				continue
			}
			ensureFetchedResolvedVersion(skillService, name, state.Label, result)
		}
		logPollResult(name, state.Label, entry.RemoteMd5, result, skippedByVersion)

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
			ensureFetchedResolvedVersion(skillService, name, state.Label, result)
		}

		if result.Deleted {
			stopManaging := true
			if remoteMissingResolutionForStatus(entry.Status) == remoteMissingKeepManaged {
				kept, keepErr := preserveLocalRepoWhenRemoteMissing(state, repoPath, name, entry)
				if keepErr != nil {
					fmt.Fprintf(os.Stderr, "[%s] Error preserving local %s after remote delete: %v\n", timeNow(), name, keepErr)
					continue
				}
				if kept {
					fmt.Printf("[%s] Remote missing: %s (kept local repo, linked agents)\n", timeNow(), name)
					changed = true
					entry = state.Skills[name]
					stopManaging = false
				}
			}
			if stopManaging {
				if err := detachLocalCopiesAfterRemoteDelete(state, repoPath, name); err != nil {
					fmt.Fprintf(os.Stderr, "[%s] Error preserving agent copies for deleted %s: %v\n", timeNow(), name, err)
					continue
				}
				if localRepoSkillExists(repoPath, name) {
					fmt.Printf("[%s] Remote deleted: %s (agent copies preserved; removed from sync)\n", timeNow(), name)
				} else {
					fmt.Printf("[%s] Deleted: %s (removed from server and no local repo copy remains)\n", timeNow(), name)
				}
				delete(state.Skills, name)
				changed = true
				continue
			}
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

			if fetchedContentMatchesSynced(entry, newHash) {
				cleanup()
				entryChanged := mergeFetchedMetadata(&entry, result)
				if localChanged && entry.Status != skill.SyncStatusLocalChanges &&
					entry.Status != skill.SyncStatusUploaded {
					entry.LocalHash = preLocalHash
					entry.Status = skill.SyncStatusLocalChanges
					entryChanged = true
				}
				if entryChanged {
					entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
					state.Skills[name] = entry
				}
				if verbose {
					fmt.Printf("[%s] Unchanged: %s (content hash matched)\n", timeNow(), name)
				}
			} else if shouldProtectLocalFromRemote(entry.Status) {
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

type remoteMissingResolution int

const (
	remoteMissingKeepManaged remoteMissingResolution = iota
	remoteMissingStopManaging
)

func remoteMissingResolutionForStatus(status skill.SyncStatus) remoteMissingResolution {
	switch status {
	case skill.SyncStatusLocalChanges, skill.SyncStatusUploadBlocked, skill.SyncStatusUploaded:
		return remoteMissingKeepManaged
	default:
		return remoteMissingStopManaging
	}
}

func shouldProtectLocalFromRemote(status skill.SyncStatus) bool {
	return status == skill.SyncStatusLocalChanges ||
		status == skill.SyncStatusUploaded ||
		status == skill.SyncStatusUploadBlocked
}

func preserveLocalRepoWhenRemoteMissing(state *skill.SyncState, repoPath, name string, entry skill.SyncSkillEntry) (bool, error) {
	localHash, err := skill.ComputeDirectoryHash(filepath.Join(repoPath, name))
	if err != nil {
		return false, err
	}
	if localHash == "" {
		return false, nil
	}
	if _, err := skill.LinkSkillForce(repoPath, name, state.Agents, nil); err != nil {
		return false, err
	}

	if entry.Status != skill.SyncStatusUploaded {
		entry.RemoteMd5 = ""
		entry.ResolvedVersion = ""
	}
	entry.LocalHash = localHash
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	state.Skills[name] = entry
	return true, nil
}

func detachLocalCopiesAfterRemoteDelete(state *skill.SyncState, repoPath, name string) error {
	if !localRepoSkillExists(repoPath, name) {
		return nil
	}
	return skill.DetachSkillFromAllAgents(repoPath, name, state.Agents, nil)
}

func localRepoSkillExists(repoPath, name string) bool {
	info, err := os.Stat(filepath.Join(repoPath, name))
	return err == nil && info.IsDir()
}

func keepLocalAfterRemoteUpdate(entry skill.SyncSkillEntry, localHash string, result *skill.SkillQueryResult) skill.SyncSkillEntry {
	mergeFetchedMetadata(&entry, result)
	if localHash != "" {
		entry.LocalHash = localHash
	}
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return entry
}

func skipFetchWhenVersionUnchanged(skillService *skill.SkillService, name, label string, entry skill.SyncSkillEntry) (*skill.SkillQueryResult, bool) {
	if entry.RemoteMd5 != "" || entry.ResolvedVersion == "" {
		return nil, false
	}
	version := resolveSkillLabelVersion(skillService, name, label)
	if version == "" || version != entry.ResolvedVersion {
		return nil, false
	}
	return &skill.SkillQueryResult{
		Md5:             entry.RemoteMd5,
		ResolvedVersion: version,
		Updated:         false,
	}, true
}

func logPollResult(name, label, sentMd5 string, result *skill.SkillQueryResult, skippedByVersion bool) {
	if !verbose || result == nil {
		return
	}
	if skippedByVersion {
		fmt.Printf("[%s] Poll: %s label=%s sentMd5=%s updated=false skipped=version newVersion=%s\n",
			timeNow(), name, label, shortHash(sentMd5), result.ResolvedVersion)
		return
	}
	fmt.Printf("[%s] Poll: %s label=%s sentMd5=%s updated=%v deleted=%v newMd5=%s newVersion=%s\n",
		timeNow(), name, label, shortHash(sentMd5), result.Updated, result.Deleted,
		shortHash(result.Md5), result.ResolvedVersion)
}

func fetchedContentMatchesSynced(entry skill.SyncSkillEntry, fetchedHash string) bool {
	if fetchedHash == "" {
		return false
	}
	syncedHash := entry.SyncedHash
	if syncedHash == "" {
		syncedHash = entry.LocalHash
	}
	return syncedHash != "" && fetchedHash == syncedHash
}

func mergeFetchedMetadata(entry *skill.SyncSkillEntry, result *skill.SkillQueryResult) bool {
	if entry == nil || result == nil {
		return false
	}
	changed := false
	if result.Md5 != "" && entry.RemoteMd5 != result.Md5 {
		entry.RemoteMd5 = result.Md5
		changed = true
	}
	if result.ResolvedVersion != "" && entry.ResolvedVersion != result.ResolvedVersion {
		entry.ResolvedVersion = result.ResolvedVersion
		changed = true
	}
	return changed
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
		if !skill.IsSyncDaemonProcess(existingPID) {
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
	if err := rotateSyncDaemonLogIfNeeded(logPath); err != nil {
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
	cmd.SysProcAttr = backgroundSysProcAttr()

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

func rotateSyncDaemonLogIfNeeded(logPath string) error {
	return rotateLogIfNeeded(logPath, syncDaemonLogMaxBytes, syncDaemonLogBackups)
}

func rotateLogIfNeeded(logPath string, maxBytes int64, backups int) error {
	if backups <= 0 {
		return nil
	}
	info, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect sync daemon log: %w", err)
	}
	if info.Size() <= maxBytes {
		return nil
	}

	oldest := fmt.Sprintf("%s.%d", logPath, backups)
	if err := os.Remove(oldest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old sync daemon log: %w", err)
	}
	for i := backups - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", logPath, i)
		dst := fmt.Sprintf("%s.%d", logPath, i+1)
		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("inspect rotated sync daemon log: %w", err)
		}
		if err := os.Rename(src, dst); err != nil {
			return fmt.Errorf("rotate sync daemon log: %w", err)
		}
	}
	if err := os.Rename(logPath, logPath+".1"); err != nil {
		return fmt.Errorf("rotate sync daemon log: %w", err)
	}
	return nil
}

func init() {
	skillSyncStartCmd.Flags().BoolVar(&syncStartForeground, "foreground", false, "Run in foreground instead of background")
	skillSyncStartCmd.Flags().StringVar(&syncStartInterval, "interval", "30s", "Poll interval (e.g. 10s, 1m)")
	skillSyncStartCmd.Flags().BoolVar(&syncStartUseRemoteOnConflict, "use-remote-on-conflict", false, "Overwrite local content with remote on conflict (backup first)")
	skillSyncStartCmd.Flags().BoolVar(&syncStartRefresh, "refresh", false, "Force re-pull every added skill (alias for treating local as out-of-date)")
	skillSyncStartCmd.Flags().StringVar(&syncStartLabel, "label", "", "Override the tracking label for this invocation (Nacos mode only)")
	skillSyncStartCmd.Flags().BoolVar(&syncStartNoAutoUpload, "no-auto-upload", false, "Disable daemon-driven auto-upload")
	skillSyncStartCmd.Flags().BoolVar(&syncStartNonInteract, "non-interactive", false, "Run without prompts; record and skip conflicts")
	skillSyncCmd.AddCommand(skillSyncStartCmd)
}
