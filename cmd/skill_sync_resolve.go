package cmd

import (
	"fmt"
	"os"

	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var (
	resolveUseNacos    bool
	resolveUseLocal    bool
	resolveUseRemote   bool
	resolveUseAgent    string
	resolveAll         bool
	resolveNonInteract bool
)

var skillSyncResolveCmd = &cobra.Command{
	Use:   "resolve [skill]",
	Short: "Resolve a sync conflict",
	Long: `Resolve a sync conflict.

There are two flavors of conflict:

Resolution options are intentionally the same as add:
  [1] Use Nacos version
  [2] Use one local agent version
  [3] Exit

Non-interactive flags:
  --use-nacos          use Nacos version
  --use-agent NAME     use a local agent version and mark Local changes
  --all                resolve every conflicted skill with the chosen flag
  --non-interactive    fail if --use-nacos or --use-agent is not provided`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		useNacos := resolveUseNacos || resolveUseRemote
		if (useNacos && resolveUseAgent != "") || (resolveUseLocal && resolveUseAgent != "") ||
			(useNacos && resolveUseLocal) {
			fmt.Fprintf(os.Stderr, "Error: choose only one of --use-nacos, --use-agent, --use-local, or --use-remote\n")
			os.Exit(1)
		}

		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}
		if len(state.Agents) == 0 {
			fmt.Fprintf(os.Stderr, "Error: no agent directories configured\n")
			os.Exit(1)
		}

		var targets []string
		if resolveAll {
			for name, entry := range state.Skills {
				if entry.Status == skill.SyncStatusConflict {
					targets = append(targets, name)
				}
			}
			if len(targets) == 0 {
				fmt.Println("No skills in conflict state.")
				return
			}
		} else {
			if len(args) == 0 {
				fmt.Fprintf(os.Stderr, "Error: skill name required (or use --all)\n")
				os.Exit(1)
			}
			name := args[0]
			entry, ok := state.Skills[name]
			if !ok {
				fmt.Fprintf(os.Stderr, "Error: skill %q not in subscriptions\n", name)
				os.Exit(1)
			}
			if entry.Status != skill.SyncStatusConflict {
				fmt.Fprintf(os.Stderr, "Error: skill %q is not in conflict state (current: %s)\n",
					name, entry.Status.DisplayString())
				os.Exit(1)
			}
			targets = []string{name}
		}

		var skillService *skill.SkillService
		if state.Mode == skill.SyncModeNacos {
			nacosClient := mustNewNacosClient()
			skillService = skill.NewSkillService(nacosClient)
		}

		var failures []string
		for _, name := range targets {
			entry := state.Skills[name]
			if err := resolveOne(state, name, entry, skillService); err != nil {
				appendSkillFailure(&failures, name, err)
			}
		}

		if err := saveSyncStateAfterBatch(state, failures); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

// resolveOne dispatches to the right conflict handler.
func resolveOne(state *skill.SyncState, name string, entry skill.SyncSkillEntry, svc *skill.SkillService) error {
	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		return err
	}

	if resolveUseNacos || resolveUseRemote {
		if state.Mode != skill.SyncModeNacos {
			fmt.Printf("Resolving %s: using repo version\n", name)
			return skill.ResolveAgentConflictUseRepo(state, name)
		}
		if svc == nil {
			return fmt.Errorf("nacos service unavailable; check profile/config")
		}
		fmt.Printf("Resolving %s: using Nacos version\n", name)
		return skill.ResolveUseRemote(state, name, svc, state.Agents)
	}

	sources := collectLocalSkillSources(state, repoPath, name, true)
	if resolveUseAgent != "" {
		source := findLocalSource(sources, resolveUseAgent)
		if source == nil {
			return fmt.Errorf("agent %q does not have a usable local version", resolveUseAgent)
		}
		if err := promoteLocalSourceToRepo(state, repoPath, name, *source); err != nil {
			return err
		}
		printLocalSourceSelected(name, source.Name, state.Config.AutoUpload)
		return nil
	}
	if resolveUseLocal {
		source := firstNonRepoSource(sources)
		if source == nil && len(sources) > 0 {
			source = &sources[0]
		}
		if source == nil {
			return fmt.Errorf("no usable local source")
		}
		if err := promoteLocalSourceToRepo(state, repoPath, name, *source); err != nil {
			return err
		}
		printLocalSourceSelected(name, source.Name, state.Config.AutoUpload)
		return nil
	}
	if resolveNonInteract {
		return fmt.Errorf("conflict requires interaction; use --use-nacos or --use-agent")
	}

	if state.Mode == skill.SyncModeNacos {
		if svc == nil {
			return fmt.Errorf("nacos service unavailable; check profile/config")
		}
		fmt.Printf("\n%s has conflicts", name)
		if entry.ResolvedVersion != "" {
			fmt.Printf(" (Nacos %s)", entry.ResolvedVersion)
		}
		fmt.Println(".")
		choice, source, err := chooseNacosOrLocalSource(name, sources, state.Config.AutoUpload, addOptions{})
		if err != nil {
			return err
		}
		switch choice {
		case skillSourceChoiceNacos:
			fmt.Printf("Resolving %s: using Nacos version\n", name)
			return skill.ResolveUseRemote(state, name, svc, state.Agents)
		case skillSourceChoiceLocal:
			if source == nil {
				return nil
			}
			if err := promoteLocalSourceToRepo(state, repoPath, name, *source); err != nil {
				return err
			}
			printLocalSourceSelected(name, source.Name, state.Config.AutoUpload)
			return nil
		case skillSourceChoiceExit:
			fmt.Printf("Skipped: %s\n", name)
			return nil
		}
		return nil
	}

	source, err := chooseLocalSourceOnly(name, sources, state.Config.AutoUpload, addOptions{})
	if err != nil {
		return err
	}
	if source == nil {
		fmt.Printf("Skipped: %s\n", name)
		return nil
	}
	if err := promoteLocalSourceToRepo(state, repoPath, name, *source); err != nil {
		return err
	}
	fmt.Printf("Resolving %s: using %s version\n", name, source.Name)
	return nil
}

func firstNonRepoSource(sources []localSkillSource) *localSkillSource {
	for i := range sources {
		if !sources[i].IsRepo {
			return &sources[i]
		}
	}
	return nil
}

func init() {
	skillSyncResolveCmd.Flags().BoolVar(&resolveUseNacos, "use-nacos", false, "Use Nacos version (non-interactive)")
	skillSyncResolveCmd.Flags().StringVar(&resolveUseAgent, "use-agent", "", "Use a local agent version (non-interactive)")
	skillSyncResolveCmd.Flags().BoolVar(&resolveUseLocal, "use-local", false, "Deprecated: use the first local source")
	skillSyncResolveCmd.Flags().BoolVar(&resolveUseRemote, "use-remote", false, "Deprecated: use --use-nacos")
	skillSyncResolveCmd.Flags().BoolVar(&resolveAll, "all", false, "Apply to all conflicted skills")
	skillSyncResolveCmd.Flags().BoolVar(&resolveNonInteract, "non-interactive", false, "Fail rather than prompt when interaction is required")
	_ = skillSyncResolveCmd.Flags().MarkHidden("use-local")
	_ = skillSyncResolveCmd.Flags().MarkHidden("use-remote")
	skillSyncCmd.AddCommand(skillSyncResolveCmd)
}
