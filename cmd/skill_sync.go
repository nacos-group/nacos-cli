package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var skillSyncCmd = &cobra.Command{
	Use:   "skill-sync",
	Short: "Manage skill synchronization across agent directories",
	Long: `Skill synchronization between Nacos (or a local repo) and one or more
agent skill directories.

Each profile has its own sync state and skill repo. Agent directories are shared.
When switching profiles, mutating commands prompt to detach the active profile
first; scripts can pass --switch-profile to make that switch explicit.

Subcommands:
  add         Add a skill and link it to all agents
  remove      Remove a skill from sync management and keep local copies in agents
  mode        Set local or Nacos sync mode
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
	addOptAll         bool
)

type removeOptions struct {
	all bool
}

var removeOptAll bool

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
  - Use --all to add every skill from the current local repo.

Non-interactive:
  - Use --non-interactive to disable prompts.
  - Nacos mode defaults to Nacos when a Nacos version is available.
  - Use --from <agent> or --from latest to choose a local source.
  - Ambiguous local-only sources fail instead of being skipped silently.

Bulk add:
  - In Nacos mode, --all adds Nacos skills not yet managed by this profile.
  - In local mode, --all links all repo skills and may import unambiguous
    unmanaged agent skills.`,
	Args: validateSkillSyncAddArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := ensureSkillSyncProfileReady(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := runSkillSyncAdd(args, currentAddOptions(cmd)); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

// --- skill-sync mode ---

var skillSyncModeCmd = &cobra.Command{
	Use:   "mode [local|nacos]",
	Short: "Set local or Nacos sync mode",
	Long: `Set the persisted skill-sync mode for the current sync profile.

Use local mode when you only want to sync local agent directories and avoid
Nacos access. Use Nacos mode when the current or specified profile should pull
from Nacos.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := ensureSkillSyncProfileReady(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := runSkillSyncMode(args[0]); err != nil {
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
untouched.

Use --all to remove every skill from sync management.`,
	Args: validateSkillSyncRemoveArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if err := ensureSkillSyncProfileReady(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := runSkillSyncRemove(args, currentRemoveOptions(cmd)); err != nil {
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
		if err := ensureSkillSyncProfileReady(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
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
	registerSkillSyncProfileFlags(skillSyncCmd)
	registerSkillSyncAddFlags(skillSyncAddCmd)
	registerSkillSyncRemoveFlags(skillSyncRemoveCmd)

	skillSyncCmd.AddCommand(skillSyncAddCmd)
	skillSyncCmd.AddCommand(skillSyncRemoveCmd)
	skillSyncCmd.AddCommand(skillSyncModeCmd)
	skillSyncCmd.AddCommand(skillSyncSetLabelCmd)
	rootCmd.AddCommand(skillSyncCmd)
}

func registerSkillSyncAddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&addOptFromAgent, "from", "", "Reverse-import source agent name (or 'latest')")
	cmd.Flags().BoolVar(&addOptUpload, "upload", false, "Deprecated: auto-upload controls draft uploads")
	_ = cmd.Flags().MarkHidden("upload")
	cmd.Flags().BoolVar(&addOptDryRun, "dry-run", false, "Show planned actions without executing")
	cmd.Flags().BoolVar(&addOptNonInteract, "non-interactive", false, "Run without prompts")
	cmd.Flags().BoolVar(&addOptAll, "all", false, "Add all unmanaged skills from Nacos or the local repo")
}

func currentAddOptions(cmd *cobra.Command) addOptions {
	all, _ := cmd.Flags().GetBool("all")
	return addOptions{
		fromAgent:   addOptFromAgent,
		dryRun:      addOptDryRun,
		nonInteract: addOptNonInteract,
		all:         all,
	}
}

func registerSkillSyncRemoveFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&removeOptAll, "all", false, "Remove all skills from sync management")
}

func currentRemoveOptions(cmd *cobra.Command) removeOptions {
	all, _ := cmd.Flags().GetBool("all")
	return removeOptions{all: all}
}

func validateSkillSyncAddArgs(cmd *cobra.Command, args []string) error {
	all, _ := cmd.Flags().GetBool("all")
	if all {
		if len(args) > 0 {
			return fmt.Errorf("--all cannot be combined with skill names")
		}
		return nil
	}
	return cobra.MinimumNArgs(1)(cmd, args)
}

func validateSkillSyncRemoveArgs(cmd *cobra.Command, args []string) error {
	all, _ := cmd.Flags().GetBool("all")
	if all {
		if len(args) > 0 {
			return fmt.Errorf("--all cannot be combined with skill names")
		}
		return nil
	}
	return cobra.MinimumNArgs(1)(cmd, args)
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

func runSkillSyncMode(rawMode string) error {
	state, err := skill.LoadSyncState()
	if err != nil {
		return fmt.Errorf("failed to load sync state: %w", err)
	}

	var mode skill.SyncMode
	switch strings.ToLower(strings.TrimSpace(rawMode)) {
	case string(skill.SyncModeLocal):
		mode = skill.SyncModeLocal
	case string(skill.SyncModeNacos):
		mode = skill.SyncModeNacos
	default:
		return fmt.Errorf("invalid mode %q (must be local or nacos)", rawMode)
	}

	if mode == skill.SyncModeLocal {
		if err := stopSyncDaemonForProfileSwitch(os.Stdout); err != nil {
			return err
		}
	}

	profile := ""
	if mode == skill.SyncModeNacos {
		profile = profileName
	}
	if err := skill.SetMode(state, mode, profile); err != nil {
		return err
	}

	if mode == skill.SyncModeLocal {
		fmt.Println("Skill sync mode: local")
		fmt.Println("Nacos access is disabled for skill-sync add/start unless you switch back to Nacos mode.")
		return nil
	}
	fmt.Printf("Skill sync mode: nacos (profile: %s)\n", state.Profile)
	fmt.Printf("Run 'nacos-cli skill-sync start --profile %s' to start the Nacos sync daemon.\n", state.Profile)
	return nil
}

func runSkillSyncRemove(skillNames []string, opts removeOptions) error {
	state, err := skill.LoadSyncState()
	if err != nil {
		return fmt.Errorf("failed to load sync state: %w", err)
	}

	if opts.all {
		skillNames = state.GetSubscribedSkillNames()
		sort.Strings(skillNames)
		if len(skillNames) == 0 {
			fmt.Println("No skills managed by skill-sync.")
			return nil
		}
	} else if len(skillNames) == 0 {
		return fmt.Errorf("skill name required")
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
