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
  add         Add a skill and link it to all agents
  remove      Remove a skill from sync management and keep local copies in agents
  start       Initial sync; start the daemon in Nacos mode
  stop        Stop the background daemon/watcher
  status      Show sync state and per-agent linkage
  resolve     Resolve a conflict (repo vs Nacos, or repo vs agent)
  agent       Manage agent directories
  set-label   Set the global tracking label for Nacos mode`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			fmt.Fprintf(os.Stderr, "Error: unknown skill-sync command %q\n\n", args[0])
			_ = cmd.Help()
			os.Exit(1)
		}
		_ = cmd.Help()
	},
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
	Short: "Add skills and link them to all agents",
	Long: `Add one or more skills and link them to every agent.

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
		if err := runSkillSyncAdd(args, currentAddOptions()); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

// --- skill-sync remove ---

var skillSyncRemoveCmd = &cobra.Command{
	Use:   "remove [skill...]",
	Short: "Remove skills from sync management and keep local copies in agents",
	Long: `Remove one or more skills from skill-sync management while keeping them usable in each agent.

For each managed symlink, the command copies the repo skill into the agent
directory before removing the tracking state. Existing real directories are left
untouched.`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runSkillSyncRemove(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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
		fmt.Printf("All added skills will now track the '%s' label.\n", label)
	},
}

func init() {
	registerSkillSyncAddFlags(skillSyncAddCmd)

	skillSyncCmd.AddCommand(skillSyncAddCmd)
	skillSyncCmd.AddCommand(skillSyncRemoveCmd)
	skillSyncCmd.AddCommand(skillSyncSetLabelCmd)
	rootCmd.AddCommand(skillSyncCmd)
}

func registerSkillSyncAddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&addOptFromAgent, "from", "", "Reverse-import source agent name (or 'latest')")
	cmd.Flags().BoolVar(&addOptUpload, "upload", false, "Deprecated: auto-upload controls draft uploads")
	_ = cmd.Flags().MarkHidden("upload")
	cmd.Flags().BoolVar(&addOptDryRun, "dry-run", false, "Show planned actions without executing")
	cmd.Flags().BoolVar(&addOptNonInteract, "non-interactive", false, "Run without prompts")
}

func currentAddOptions() addOptions {
	return addOptions{
		fromAgent:   addOptFromAgent,
		dryRun:      addOptDryRun,
		nonInteract: addOptNonInteract,
	}
}

func runSkillSyncAdd(skillNames []string, opts addOptions) error {
	state, err := skill.LoadSyncState()
	if err != nil {
		return fmt.Errorf("failed to load sync state: %w", err)
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
		Interactive: !opts.nonInteract,
	})
	if err != nil {
		return fmt.Errorf("failed to resolve mode: %w", err)
	}

	switch res.Mode {
	case skill.SyncModeLocal:
		return runSkillSyncAddLocal(skillNames, opts)
	case skill.SyncModeNacos:
		return runSkillSyncAddNacos(skillNames, opts)
	default:
		return fmt.Errorf("unsupported sync mode: %s", res.Mode)
	}
}

func runSkillSyncRemove(skillNames []string) error {
	state, err := skill.LoadSyncState()
	if err != nil {
		return fmt.Errorf("failed to load sync state: %w", err)
	}

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		return fmt.Errorf("ensure skill repo: %w", err)
	}
	state.Repo = repoPath

	var failures []string
	for _, skillName := range skillNames {
		if _, ok := state.Skills[skillName]; !ok {
			fmt.Printf("Skill %q is not managed by skill-sync, skipping.\n", skillName)
			continue
		}
		fmt.Printf("Removing %s from skill-sync...\n", skillName)
		if err := skill.DetachSkillFromAllAgents(repoPath, skillName, state.Agents, os.Stdout); err != nil {
			failures = append(failures, err.Error())
			continue
		}
		state.RemoveSkill(skillName)
		fmt.Printf("Removed: %s (agent copies preserved)\n", skillName)
	}

	if len(failures) == 0 {
		return skill.SaveSyncState(state)
	}
	if saveErr := skill.SaveSyncState(state); saveErr != nil {
		return fmt.Errorf("%s; additionally failed to save sync state: %w", strings.Join(failures, "; "), saveErr)
	}
	return fmt.Errorf("%s", strings.Join(failures, "; "))
}
