package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var skillSyncCmd = &cobra.Command{
	Use:   "skill-sync",
	Short: "Manage skill synchronization across agent directories",
	Long: `Skill synchronization between Nacos (or a local repo) and one or more
agent skill directories.

Subcommands:
  add         Subscribe to a skill and link it to all agents
  remove      Unsubscribe and unlink from all agents
  start       Initial sync (default: subscribed only) and start the daemon/watcher
  stop        Stop the background daemon/watcher
  status      Show sync state and per-agent linkage
  resolve     Resolve a conflict (repo vs Nacos, or repo vs agent)
  agent       Manage agent directories
  set-label   Set the global tracking label for Nacos mode`,
}

// --- skill-sync add ---

var (
	addOptFromAgent   string
	addOptUpload      bool
	addOptDryRun      bool
	addOptNonInteract bool
)

var skillSyncAddCmd = &cobra.Command{
	Use:   "add [skill...]",
	Short: "Subscribe to skills and link them to all agents",
	Long: `Add one or more skills to the subscription list and link them to every agent.

Behavior is safe by default: any agent that already holds a different version
of the skill is left untouched and reported as a conflict (resolve later with
'skill-sync resolve <skill>').

Nacos mode:
  - If the skill exists on Nacos and local content does not conflict, pull it
    into the central repo and link.
  - If local content also exists, choose Nacos or one local agent version.
  - Choosing a local version records Local changes; auto-upload decides whether
    it is uploaded as draft.

Local mode:
  - If the central repo has the skill, link it to all agents.
  - Otherwise reverse-import from an agent (single match auto-imports;
    multiple different versions trigger a source picker, override with --from).

Non-interactive:
  - Use --non-interactive to disable prompts.
  - Nacos mode defaults to Nacos when a Nacos version is available.
  - Use --from <agent> or --from latest to choose a local source.
  - Ambiguous local-only sources fail instead of being skipped silently.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		override := skill.ModeOverrideNone
		profileHint := ""
		if profileName != "" {
			override = skill.ModeOverrideNacos
			profileHint = profileName
		}

		res, err := skill.ResolveSyncMode(state, skill.ResolveModeOptions{
			Override:    override,
			ProfileHint: profileHint,
			Interactive: !addOptNonInteract,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to resolve mode: %v\n", err)
			os.Exit(1)
		}

		opts := addOptions{
			fromAgent:   addOptFromAgent,
			dryRun:      addOptDryRun,
			nonInteract: addOptNonInteract,
		}

		switch res.Mode {
		case skill.SyncModeLocal:
			if err := runSkillSyncAddLocal(args, opts); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		case skill.SyncModeNacos:
			if err := runSkillSyncAddNacos(args, opts); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

// --- skill-sync remove ---

var removeOptKeepSource bool

var skillSyncRemoveCmd = &cobra.Command{
	Use:   "remove [skill...]",
	Short: "Unsubscribe from skills and unlink them from all agents",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		for _, skillName := range args {
			if _, ok := state.Skills[skillName]; !ok {
				fmt.Printf("Skill %q not in subscriptions, skipping.\n", skillName)
				continue
			}
			fmt.Printf("Removing %s...\n", skillName)
			if err := skill.UnlinkSkillFromAllAgents(skillName, state.Agents, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
			state.RemoveSkill(skillName)
			if removeOptKeepSource {
				fmt.Printf("Unsubscribed: %s (repo source preserved)\n", skillName)
			} else {
				fmt.Printf("Unsubscribed: %s\n", skillName)
			}
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

		fmt.Printf("Tracking label: %s -> %s\n", oldLabel, label)
		fmt.Printf("All subscribed skills will now track the '%s' label.\n", label)
	},
}

func init() {
	skillSyncAddCmd.Flags().StringVar(&addOptFromAgent, "from", "", "Reverse-import source agent name (or 'latest')")
	skillSyncAddCmd.Flags().BoolVar(&addOptUpload, "upload", false, "Deprecated: auto-upload controls draft uploads")
	_ = skillSyncAddCmd.Flags().MarkHidden("upload")
	skillSyncAddCmd.Flags().BoolVar(&addOptDryRun, "dry-run", false, "Show planned actions without executing")
	skillSyncAddCmd.Flags().BoolVar(&addOptNonInteract, "non-interactive", false, "Run without prompts")

	skillSyncRemoveCmd.Flags().BoolVar(&removeOptKeepSource, "keep-source", false, "Keep the skill in the central repo after unsubscribing")

	skillSyncCmd.AddCommand(skillSyncAddCmd)
	skillSyncCmd.AddCommand(skillSyncRemoveCmd)
	skillSyncCmd.AddCommand(skillSyncSetLabelCmd)
	rootCmd.AddCommand(skillSyncCmd)
}
