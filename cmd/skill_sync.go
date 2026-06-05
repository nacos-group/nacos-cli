package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var skillSyncCmd = &cobra.Command{
	Use:   "skill-sync",
	Short: "Manage skill synchronization across agent directories",
	Long: `Unified skill synchronization management.

Subcommands:
  add         Subscribe to a skill and perform initial pull
  remove      Unsubscribe from a skill (keeps local files)
  status      Show sync state of all subscribed skills
  resolve     Resolve conflicts between local and remote
  start       Start the background sync daemon
  stop        Stop the background sync daemon
  set-label   Set the global tracking label
  agent       Manage agent directories`,
}

// --- skill-sync add ---

var skillSyncAddCmd = &cobra.Command{
	Use:   "add [skill...]",
	Short: "Subscribe to skills and perform initial pull",
	Long: `Subscribe to one or more skills and perform the initial download.

This command:
  - Adds skills to the local subscription list
  - Auto-discovers agent directories on first run
  - Downloads the skill (using the global tracking label) to all agent dirs
  - Does NOT start the background sync daemon (use 'skill-sync start' for that)`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		skillNames := args

		// Load sync state
		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		// Auto-discover agents on first run
		if len(state.Agents) == 0 {
			discovered, err := skill.DiscoverAgents()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: agent discovery failed: %v\n", err)
			}
			if len(discovered) > 0 {
				state.Agents = discovered
				fmt.Println("Detected agents:")
				for _, agent := range discovered {
					fmt.Printf("  %-10s %s\n", agent.Name, agent.Path)
				}
				fmt.Println()
			} else {
				// Create default ~/.skills directory
				homeDir, _ := os.UserHomeDir()
				defaultPath := filepath.Join(homeDir, ".skills")
				if err := os.MkdirAll(defaultPath, 0755); err == nil {
					state.Agents = []skill.AgentDir{{Name: "default", Path: defaultPath, AutoFound: true}}
					fmt.Printf("Created default agent directory: %s\n\n", defaultPath)
				}
			}
		}

		if len(state.Agents) == 0 {
			fmt.Fprintf(os.Stderr, "Error: no agent directories found. Use 'skill-sync agent add' to add one.\n")
			os.Exit(1)
		}

		fmt.Printf("Tracking label: %s\n", state.Label)

		// Create Nacos client and skill service
		nacosClient := mustNewNacosClient()
		skillService := skill.NewSkillService(nacosClient)

		var addedCount int
		for _, skillName := range skillNames {
			// Check if already subscribed
			if existing, ok := state.Skills[skillName]; ok {
				fmt.Printf("Skill %q already subscribed (version: %s, status: %s)\n",
					skillName, existing.ResolvedVersion, existing.Status.DisplayString())
				continue
			}

			fmt.Printf("Adding subscription: %s...\n", skillName)

			// Download to first agent directory
			primaryDir := state.Agents[0].Path
			result, err := skillService.QuerySkill(skillName, primaryDir, "", state.Label, "")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to download skill %q: %v\n", skillName, err)
				os.Exit(1)
			}
			if result.Deleted {
				fmt.Fprintf(os.Stderr, "Error: skill %q not found on server\n", skillName)
				os.Exit(1)
			}

			// Copy to remaining agents
			sourceDir := filepath.Join(primaryDir, skillName)
			if len(state.Agents) > 1 {
				if err := skill.EnsureSkillInAllAgents(skillName, sourceDir, state.Agents[1:]); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to sync to all agents: %v\n", err)
				}
			}

			// Compute content hash
			localHash, err := skill.ComputeDirectoryHash(sourceDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to compute hash: %v\n", err)
			}

			// Add to state
			state.AddSkill(skillName, state.Label, result.ResolvedVersion, result.Md5, localHash)
			addedCount++

			fmt.Printf("  Subscribed: %s (version: %s)\n", skillName, result.ResolvedVersion)
			fmt.Printf("  Synced to %d agent(s)\n", len(state.Agents))
		}

		// Save state
		if err := skill.SaveSyncState(state); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to save sync state: %v\n", err)
			os.Exit(1)
		}

		// Hint about starting daemon (only when new skills were added)
		if addedCount > 0 {
			running, _ := skill.IsSyncDaemonRunning()
			if !running {
				fmt.Printf("\nNote: sync daemon is not running. Use 'nacos-cli skill-sync start' to enable background sync.\n")
			}
		}
	},
}

// --- skill-sync remove ---

var skillSyncRemoveCmd = &cobra.Command{
	Use:   "remove [skill...]",
	Short: "Unsubscribe from skills (keeps local files)",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		for _, skillName := range args {
			if _, ok := state.Skills[skillName]; !ok {
				fmt.Printf("Skill %q not found in subscriptions, skipping.\n", skillName)
				continue
			}

			state.RemoveSkill(skillName)
			fmt.Printf("Removed subscription: %s\n", skillName)
			fmt.Printf("  Note: local files are preserved. Delete manually if needed.\n")
		}

		if err := skill.SaveSyncState(state); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to save sync state: %v\n", err)
			os.Exit(1)
		}
	},
}

// --- skill-sync set-label ---

var skillSyncSetLabelCmd = &cobra.Command{
	Use:   "set-label [label]",
	Short: "Set the global tracking label (default: latest)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		label := strings.TrimSpace(args[0])
		if label == "" {
			fmt.Fprintf(os.Stderr, "Error: label cannot be empty\n")
			os.Exit(1)
		}

		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		oldLabel := state.Label
		state.SetLabel(label)

		if err := skill.SaveSyncState(state); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to save sync state: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Tracking label: %s → %s\n", oldLabel, label)
		fmt.Printf("All subscribed skills will now track the '%s' label.\n", label)
	},
}

// --- skill-sync pull ---

var skillSyncPullCmd = &cobra.Command{
	Use:   "pull [skill]",
	Short: "Manually pull latest version for a skill",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		skillName := args[0]

		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		entry, ok := state.Skills[skillName]
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: skill %q not found in subscriptions\n", skillName)
			os.Exit(1)
		}

		if len(state.Agents) == 0 {
			fmt.Fprintf(os.Stderr, "Error: no agent directories configured\n")
			os.Exit(1)
		}

		nacosClient := mustNewNacosClient()
		skillService := skill.NewSkillService(nacosClient)

		primaryDir := state.Agents[0].Path
		result, err := skillService.QuerySkill(skillName, primaryDir, "", state.Label, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to pull skill %q: %v\n", skillName, err)
			os.Exit(1)
		}
		if result.Deleted {
			fmt.Fprintf(os.Stderr, "Error: skill %q not found on server\n", skillName)
			os.Exit(1)
		}

		// Copy to remaining agents
		sourceDir := filepath.Join(primaryDir, skillName)
		if len(state.Agents) > 1 {
			if err := skill.EnsureSkillInAllAgents(skillName, sourceDir, state.Agents[1:]); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to sync to all agents: %v\n", err)
			}
		}

		// Compute hash and update state
		localHash, _ := skill.ComputeDirectoryHash(sourceDir)
		entry.RemoteMd5 = result.Md5
		entry.ResolvedVersion = result.ResolvedVersion
		entry.LocalHash = localHash
		entry.SyncedHash = localHash
		entry.Status = skill.SyncStatusSynced
		state.Skills[skillName] = entry

		if err := skill.SaveSyncState(state); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to save sync state: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Pulled: %s (version: %s)\n", skillName, result.ResolvedVersion)
	},
}

func init() {
	skillSyncCmd.AddCommand(skillSyncAddCmd)
	skillSyncCmd.AddCommand(skillSyncRemoveCmd)
	skillSyncCmd.AddCommand(skillSyncSetLabelCmd)
	skillSyncCmd.AddCommand(skillSyncPullCmd)
	rootCmd.AddCommand(skillSyncCmd)
}
