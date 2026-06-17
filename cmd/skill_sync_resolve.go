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
	resolveUseRepo     bool
	resolveUseAgent    string
	resolveAll         bool
	resolveNonInteract bool
)

type resolveOptions struct {
	UseNacos    bool
	UseRepo     bool
	UseLocal    bool
	UseAgent    string
	NonInteract bool
}

type resolveDecisionKind string

const (
	resolveDecisionNacos resolveDecisionKind = "nacos"
	resolveDecisionRepo  resolveDecisionKind = "repo"
	resolveDecisionLocal resolveDecisionKind = "local"
	resolveDecisionExit  resolveDecisionKind = "exit"
)

type resolveDecision struct {
	Kind   resolveDecisionKind
	Source *localSkillSource
}

var skillSyncResolveCmd = &cobra.Command{
	Use:   "resolve [skill]",
	Short: "Resolve a sync conflict",
	Long: `Resolve a sync conflict.

Resolution follows the same source-choice model as add.

Nacos mode:
  [1] Use Nacos version
  [2] Use one local agent version and mark Local changes
  [3] Exit

Local mode:
  [1] Use central repo version
  [2] Use one local agent version
  [3] Exit

Non-interactive flags:
  --use-nacos          use Nacos version
  --use-repo           use the central local repo version
  --use-agent NAME     use a local agent version and mark Local changes
  --all                resolve every conflicted skill with the chosen flag
  --non-interactive    fail if --use-nacos, --use-repo, or --use-agent is not provided`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		opts := currentResolveOptions()
		if err := validateResolveOptions(opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: choose only one of --use-nacos, --use-repo, --use-agent, --use-local, or --use-remote\n")
			os.Exit(1)
		}
		if err := ensureSkillSyncProfileReady(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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
				fmt.Fprintf(os.Stderr, "Error: skill %q is not managed by skill-sync\n", name)
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
			if err := resolveOneWithOptions(state, name, entry, skillService, opts); err != nil {
				appendSkillFailure(&failures, name, err)
			}
		}

		if err := saveSyncStateAfterBatch(state, failures); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func resolveOne(state *skill.SyncState, name string, entry skill.SyncSkillEntry, svc *skill.SkillService) error {
	return resolveOneWithOptions(state, name, entry, svc, currentResolveOptions())
}

// resolveOneWithOptions builds a conflict-resolution decision, then executes it.
func resolveOneWithOptions(state *skill.SyncState, name string, entry skill.SyncSkillEntry, svc *skill.SkillService, opts resolveOptions) error {
	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		return err
	}

	sources := collectLocalSkillSources(state, repoPath, name, true)
	decision, err := buildResolveDecision(state, name, entry, sources, svc, opts)
	if err != nil {
		return err
	}
	return executeResolveDecision(state, repoPath, name, decision, svc)
}

func currentResolveOptions() resolveOptions {
	return resolveOptions{
		UseNacos:    resolveUseNacos || resolveUseRemote,
		UseRepo:     resolveUseRepo,
		UseLocal:    resolveUseLocal,
		UseAgent:    resolveUseAgent,
		NonInteract: resolveNonInteract,
	}
}

func validateResolveOptions(opts resolveOptions) error {
	choices := 0
	for _, selected := range []bool{opts.UseNacos, opts.UseRepo, opts.UseLocal, opts.UseAgent != ""} {
		if selected {
			choices++
		}
	}
	if choices > 1 {
		return fmt.Errorf("choose only one resolve source")
	}
	return nil
}

func buildResolveDecision(state *skill.SyncState, name string, entry skill.SyncSkillEntry, sources []localSkillSource, svc *skill.SkillService, opts resolveOptions) (resolveDecision, error) {
	if opts.UseNacos {
		if state.Mode != skill.SyncModeNacos {
			return resolveDecision{Kind: resolveDecisionRepo}, nil
		}
		if svc == nil {
			return resolveDecision{}, fmt.Errorf("nacos service unavailable; check profile/config")
		}
		return resolveDecision{Kind: resolveDecisionNacos}, nil
	}
	if opts.UseRepo {
		return resolveDecision{Kind: resolveDecisionRepo}, nil
	}
	if opts.UseAgent != "" {
		source := findLocalSource(sources, opts.UseAgent)
		if source == nil {
			return resolveDecision{}, fmt.Errorf("agent %q does not have a usable local version", opts.UseAgent)
		}
		return resolveDecision{Kind: resolveDecisionLocal, Source: source}, nil
	}
	if opts.UseLocal {
		source := firstNonRepoSource(sources)
		if source == nil && len(sources) > 0 {
			source = &sources[0]
		}
		if source == nil {
			return resolveDecision{}, fmt.Errorf("no usable local source")
		}
		return resolveDecision{Kind: resolveDecisionLocal, Source: source}, nil
	}
	if opts.NonInteract {
		return resolveDecision{}, fmt.Errorf("conflict requires interaction; use --use-nacos, --use-repo, or --use-agent")
	}

	if state.Mode == skill.SyncModeNacos {
		if svc == nil {
			return resolveDecision{}, fmt.Errorf("nacos service unavailable; check profile/config")
		}
		fmt.Printf("\n%s has conflicts", name)
		if entry.ResolvedVersion != "" {
			fmt.Printf(" (Nacos %s)", entry.ResolvedVersion)
		}
		fmt.Println(".")
		choice, source, err := chooseNacosOrLocalSource(name, sources, state.Config.AutoUpload, addOptions{})
		if err != nil {
			return resolveDecision{}, err
		}
		switch choice {
		case skillSourceChoiceNacos:
			return resolveDecision{Kind: resolveDecisionNacos}, nil
		case skillSourceChoiceLocal:
			if source == nil {
				return resolveDecision{Kind: resolveDecisionExit}, nil
			}
			return resolveDecision{Kind: resolveDecisionLocal, Source: source}, nil
		case skillSourceChoiceExit:
			return resolveDecision{Kind: resolveDecisionExit}, nil
		}
		return resolveDecision{Kind: resolveDecisionExit}, nil
	}

	source, err := chooseLocalSourceOnly(name, sources, state.Config.AutoUpload, addOptions{})
	if err != nil {
		return resolveDecision{}, err
	}
	if source == nil {
		return resolveDecision{Kind: resolveDecisionExit}, nil
	}
	return resolveDecision{Kind: resolveDecisionLocal, Source: source}, nil
}

func executeResolveDecision(state *skill.SyncState, repoPath, name string, decision resolveDecision, svc *skill.SkillService) error {
	switch decision.Kind {
	case resolveDecisionNacos:
		if svc == nil {
			return fmt.Errorf("nacos service unavailable; check profile/config")
		}
		fmt.Printf("Resolving %s: using Nacos version\n", name)
		return skill.ResolveUseRemote(state, name, svc, state.Agents)
	case resolveDecisionRepo:
		fmt.Printf("Resolving %s: using repo version\n", name)
		return skill.ResolveAgentConflictUseRepo(state, name)
	case resolveDecisionLocal:
		if decision.Source == nil {
			return fmt.Errorf("local source missing")
		}
		if err := promoteLocalSourceToRepo(state, repoPath, name, *decision.Source); err != nil {
			return err
		}
		printLocalSourceSelected(name, decision.Source.Name, state.Mode, state.Config.AutoUpload)
		return nil
	case resolveDecisionExit:
		fmt.Printf("Skipped: %s\n", name)
		return nil
	default:
		return fmt.Errorf("unsupported resolve decision: %s", decision.Kind)
	}
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
	skillSyncResolveCmd.Flags().BoolVar(&resolveUseRepo, "use-repo", false, "Use central repo version (non-interactive)")
	skillSyncResolveCmd.Flags().StringVar(&resolveUseAgent, "use-agent", "", "Use a local agent version (non-interactive)")
	skillSyncResolveCmd.Flags().BoolVar(&resolveUseLocal, "use-local", false, "Deprecated: use the first local source")
	skillSyncResolveCmd.Flags().BoolVar(&resolveUseRemote, "use-remote", false, "Deprecated: use --use-nacos")
	skillSyncResolveCmd.Flags().BoolVar(&resolveAll, "all", false, "Apply to all conflicted skills")
	skillSyncResolveCmd.Flags().BoolVar(&resolveNonInteract, "non-interactive", false, "Fail rather than prompt when interaction is required")
	_ = skillSyncResolveCmd.Flags().MarkHidden("use-local")
	_ = skillSyncResolveCmd.Flags().MarkHidden("use-remote")
	skillSyncCmd.AddCommand(skillSyncResolveCmd)
}
