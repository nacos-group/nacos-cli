package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var skillSyncStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync state of all added skills",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		state, opts, err := loadSyncStateForStatus()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		printSyncStatusSummaryWithOptions(state, opts)
	},
}

type syncStatusPrintOptions struct {
	refreshRemote   bool
	refreshLocal    bool
	activeProfile   string
	showingProfile  string
	inactive        bool
	followedActive  bool
	originalProfile string
}

func loadSyncStateForStatus() (*skill.SyncState, syncStatusPrintOptions, error) {
	opts := syncStatusPrintOptions{refreshRemote: true, refreshLocal: true}
	current := skill.CurrentSyncProfile()
	active, err := skill.LoadActiveSyncProfile()
	if err != nil {
		return nil, opts, err
	}
	opts.activeProfile = active
	opts.showingProfile = current
	if active != "" && active != current {
		if profileName == "" {
			skill.SetCurrentSyncProfile(active)
			state, err := skill.LoadSyncStateForProfile(active)
			opts.refreshRemote = false
			opts.followedActive = true
			opts.originalProfile = current
			opts.showingProfile = active
			return state, opts, err
		}
		opts.inactive = true
		opts.refreshRemote = false
		opts.refreshLocal = false
	}

	state, err := skill.LoadSyncState()
	return state, opts, err
}

func printSyncStatusSummary(state *skill.SyncState) {
	printSyncStatusSummaryWithOptions(state, syncStatusPrintOptions{refreshRemote: true, refreshLocal: true})
}

func printSyncStatusSummaryWithOptions(state *skill.SyncState, opts syncStatusPrintOptions) {
	printSyncStatusProfileContext(state, opts)

	mode := state.Mode
	if mode == skill.SyncModeUnset {
		mode = "(unset)"
	}
	fmt.Printf("Mode: %s\n", mode)
	if state.Profile != "" && !opts.inactive {
		fmt.Printf("Profile: %s\n", state.Profile)
	}
	if state.Repo != "" {
		fmt.Printf("Repository: %s\n", state.Repo)
	}
	if state.Mode == skill.SyncModeNacos {
		fmt.Printf("Tracking label: %s\n", state.Label)
		if state.Config.AutoUpload {
			fmt.Printf("Auto-upload: enabled\n")
		} else {
			fmt.Printf("Auto-upload: disabled\n")
		}
	}
	printSyncDaemonStatusWithOptions(opts)
	fmt.Println()

	if len(state.Skills) == 0 {
		fmt.Println("No skills added.")
		fmt.Println("Use 'nacos-cli skill-sync add <skill>' to add a skill.")
		return
	}

	// Refresh local hashes for accurate status
	if opts.refreshLocal {
		refreshLocalHashes(state)
		refreshLocalAgentConflicts(state)
	}
	if opts.refreshRemote {
		refreshNacosVersionsForStatus(state)
	}

	// Sort skill names
	names := make([]string, 0, len(state.Skills))
	for name := range state.Skills {
		names = append(names, name)
	}
	sort.Strings(names)

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if state.Mode == skill.SyncModeLocal {
		fmt.Fprintf(w, "SKILL\tSTATUS\tAGENTS\tNEXT\n")
		fmt.Fprintf(w, "-----\t------\t------\t----\n")
		for _, name := range names {
			entry := state.Skills[name]
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				name, entry.Status.DisplayString(),
				agentsDisplay(name, entry, state),
				nextAction(name, entry, state))
		}
	} else {
		fmt.Fprintf(w, "SKILL\tSTATUS\tVERSION\tAGENTS\tNEXT\n")
		fmt.Fprintf(w, "-----\t------\t-------\t------\t----\n")
		for _, name := range names {
			entry := state.Skills[name]
			version := entry.ResolvedVersion
			if version == "" {
				version = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				name, entry.Status.DisplayString(), version,
				agentsDisplay(name, entry, state),
				nextAction(name, entry, state))
		}
	}
	w.Flush()

	fmt.Printf("\nTotal: %d skills\n", len(state.Skills))
	if len(state.Agents) > 0 {
		agentNames := make([]string, 0, len(state.Agents))
		for _, a := range state.Agents {
			agentNames = append(agentNames, a.Name)
		}
		fmt.Printf("Agents: %v\n", agentNames)
	}
}

func printSyncStatusProfileContext(state *skill.SyncState, opts syncStatusPrintOptions) {
	if opts.inactive {
		fmt.Printf("Active profile: %s\n", opts.activeProfile)
		showing := opts.showingProfile
		if showing == "" {
			showing = state.Profile
		}
		fmt.Printf("Showing profile: %s (inactive)\n", showing)
		fmt.Println("This profile is not currently linked to agent directories.")
		return
	}
	if opts.followedActive {
		fmt.Printf("Active profile: %s\n", opts.showingProfile)
		fmt.Printf("Showing profile: %s\n", opts.showingProfile)
		if opts.originalProfile != "" {
			fmt.Printf("Current CLI profile: %s\n", opts.originalProfile)
		}
	}
}

// agentsDisplay returns a comma-joined list of agents that have the skill linked,
// noting any agent in conflict with the central repo as `name≠`.
func agentsDisplay(skillName string, entry skill.SyncSkillEntry, state *skill.SyncState) string {
	conflict := make(map[string]bool, len(entry.ConflictAgents))
	for _, name := range entry.ConflictAgents {
		conflict[name] = true
	}
	parts := make([]string, 0, len(state.Agents))
	for _, a := range state.Agents {
		if conflict[a.Name] {
			parts = append(parts, a.Name+"\u2260")
		} else {
			parts = append(parts, a.Name)
		}
	}
	return strings.Join(parts, ",")
}

func printSyncDaemonStatusWithOptions(opts syncStatusPrintOptions) {
	if opts.inactive {
		active := opts.activeProfile
		if active == "" {
			active = "-"
		}
		fmt.Printf("Sync daemon: inactive for this profile (active profile: %s)\n", active)
		return
	}
	running, pid := skill.IsSyncDaemonRunning()
	if running {
		fmt.Printf("Sync daemon: running (pid: %d)\n", pid)
	} else {
		fmt.Printf("Sync daemon: stopped\n")
	}
}

func refreshLocalHashes(state *skill.SyncState) {
	if len(state.Agents) == 0 {
		return
	}
	primaryDir := state.Agents[0].Path

	changed := false
	for name, entry := range state.Skills {
		skillDir := filepath.Join(primaryDir, name)
		localHash, err := skill.ComputeDirectoryHash(skillDir)
		if err != nil || localHash == "" {
			continue
		}

		if localHash != entry.LocalHash {
			entry.LocalHash = localHash
			// Recompute status
			newStatus := skill.DetermineStatus(entry, localHash, entry.RemoteMd5)
			if newStatus != entry.Status {
				entry.Status = newStatus
			}
			state.Skills[name] = entry
			changed = true
		}
	}

	if changed {
		_ = skill.SaveSyncState(state)
	}
}

func refreshLocalAgentConflicts(state *skill.SyncState) {
	if state.Mode != skill.SyncModeLocal || len(state.Agents) == 0 {
		return
	}

	repoPath, err := skill.GetSkillRepoPath()
	if err != nil {
		return
	}

	changed := false
	for name, entry := range state.Skills {
		if _, err := os.Stat(filepath.Join(repoPath, name)); err != nil {
			continue
		}
		repoHash, err := skill.ComputeDirectoryHash(filepath.Join(repoPath, name))
		if err != nil {
			continue
		}

		conflictAgents := make([]string, 0)
		for _, agent := range state.Agents {
			status, err := skill.InspectAgentSkill(agent.Path, name, repoPath)
			if err != nil {
				continue
			}
			if status == skill.AgentSkillConflict || status == skill.AgentSkillBroken {
				conflictAgents = append(conflictAgents, agent.Name)
			}
		}

		desiredStatus := skill.SyncStatusLinked
		if len(conflictAgents) > 0 {
			desiredStatus = skill.SyncStatusConflict
		}

		if !sameStringSlice(entry.ConflictAgents, conflictAgents) ||
			entry.Status != desiredStatus ||
			entry.LocalHash != repoHash ||
			entry.SyncedHash != repoHash {
			entry.LocalHash = repoHash
			entry.SyncedHash = repoHash
			entry.ConflictAgents = conflictAgents
			entry.Status = desiredStatus
			entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			state.Skills[name] = entry
			changed = true
		}
	}

	if changed {
		_ = skill.SaveSyncState(state)
	}
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func refreshNacosVersionsForStatus(state *skill.SyncState) {
	if state.Mode != skill.SyncModeNacos {
		return
	}
	if _, _, mismatch := skillSyncProfileMismatch(); mismatch {
		return
	}

	needsRefresh := false
	for _, entry := range state.Skills {
		if entry.ResolvedVersion == "" {
			needsRefresh = true
			break
		}
	}
	if !needsRefresh {
		return
	}

	svc := skill.NewSkillService(mustNewNacosClient())
	if refreshMissingNacosVersions(state, svc) {
		_ = skill.SaveSyncState(state)
	}
}

func nextAction(name string, entry skill.SyncSkillEntry, state *skill.SyncState) string {
	switch entry.Status {
	case skill.SyncStatusSynced:
		return "-"
	case skill.SyncStatusLocalChanges:
		if state.Mode == skill.SyncModeNacos && state.Config.AutoUpload {
			return "auto-upload pending"
		}
		if len(state.Agents) > 0 {
			return fmt.Sprintf("skill-upload %s", filepath.Join(state.Agents[0].Path, name))
		}
		return "skill-upload"
	case skill.SyncStatusUploaded:
		return "waiting publish"
	case skill.SyncStatusUploadBlocked:
		if entry.BlockedDraftVersion != "" {
			return fmt.Sprintf("Nacos draft %s exists; review/clear it, auto-upload will retry", entry.BlockedDraftVersion)
		}
		if entry.BlockedReviewVersion != "" {
			return fmt.Sprintf("Nacos reviewing %s exists; wait/review it, auto-upload will retry", entry.BlockedReviewVersion)
		}
		return "Nacos draft exists; review/clear it, auto-upload will retry"
	case skill.SyncStatusRemoteChanges:
		return "auto-pull pending"
	case skill.SyncStatusConflict:
		return fmt.Sprintf("skill-sync resolve %s", name)
	default:
		return "-"
	}
}

func init() {
	skillSyncCmd.AddCommand(skillSyncStatusCmd)
}
