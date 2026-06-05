package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var skillSyncStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync state of all subscribed skills",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		if len(state.Skills) == 0 {
			fmt.Println("No skills subscribed.")
			fmt.Println("Use 'nacos-cli skill-sync add <skill>' to subscribe.")
			printSyncDaemonStatus()
			return
		}

		printSyncStatusSummary(state)
	},
}

func printSyncStatusSummary(state *skill.SyncState) {
	fmt.Printf("Sync list source: local\n")
	fmt.Printf("Tracking label: %s\n", state.Label)
	printSyncDaemonStatus()
	fmt.Println()

	if len(state.Skills) == 0 {
		fmt.Println("No skills subscribed.")
		fmt.Println("Use 'nacos-cli skill-sync add <skill>' to subscribe.")
		return
	}

	// Refresh local hashes for accurate status
	refreshLocalHashes(state)

	// Sort skill names
	names := make([]string, 0, len(state.Skills))
	for name := range state.Skills {
		names = append(names, name)
	}
	sort.Strings(names)

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "SKILL\tSTATUS\tVERSION\tMD5\tUPDATED\tNEXT\n")
	fmt.Fprintf(w, "-----\t------\t-------\t---\t-------\t----\n")

	for _, name := range names {
		entry := state.Skills[name]

		version := entry.ResolvedVersion
		if version == "" {
			version = "-"
		}

		md5Display := shortMd5(entry.RemoteMd5)
		if md5Display == "" {
			md5Display = "-"
		}

		updatedAt := entry.UpdatedAt
		if updatedAt != "" {
			if idx := len(updatedAt); idx > 19 {
				updatedAt = updatedAt[:19]
			}
		} else {
			updatedAt = "-"
		}

		next := nextAction(name, entry, state)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			name, entry.Status.DisplayString(), version, md5Display, updatedAt, next)
	}
	w.Flush()

	// Summary
	fmt.Printf("\nTotal: %d skills\n", len(state.Skills))
	if len(state.Agents) > 0 {
		agentNames := make([]string, 0, len(state.Agents))
		for _, a := range state.Agents {
			agentNames = append(agentNames, a.Name)
		}
		fmt.Printf("Agents: %v\n", agentNames)
	}
}

func printSyncDaemonStatus() {
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

func nextAction(name string, entry skill.SyncSkillEntry, state *skill.SyncState) string {
	switch entry.Status {
	case skill.SyncStatusSynced:
		return "-"
	case skill.SyncStatusLocalChanges:
		if len(state.Agents) > 0 {
			return fmt.Sprintf("skill-upload %s", filepath.Join(state.Agents[0].Path, name))
		}
		return "skill-upload"
	case skill.SyncStatusUploaded:
		return "waiting publish"
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
