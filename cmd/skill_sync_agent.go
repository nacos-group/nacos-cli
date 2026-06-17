package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var skillSyncAgentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage agent directories for skill sync",
}

var skillSyncAgentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered agent directories",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		if len(state.Agents) == 0 {
			fmt.Println("No agent directories registered.")
			fmt.Println("Use 'skill-sync agent add <name> <path>' or run 'skill-sync add' for auto-discovery.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "AGENT\tPATH\tSOURCE\n")
		fmt.Fprintf(w, "-----\t----\t------\n")
		for _, agent := range state.Agents {
			source := "manual"
			if agent.AutoFound {
				source = "auto"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", agent.Name, agent.Path, source)
		}
		w.Flush()
	},
}

var skillSyncAgentAddCmd = &cobra.Command{
	Use:               "add <name> <path>",
	Short:             "Add a custom agent directory",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeDirArg(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := ensureSkillSyncProfileReady(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := runSkillSyncAgentAdd(args[0], args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var skillSyncAgentRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an agent directory (does not delete files)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		if err := state.RemoveAgent(name); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if err := skill.SaveSyncState(state); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to save sync state: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Removed agent: %s\n", name)
		fmt.Println("  Note: local files are preserved.")
	},
}

func init() {
	skillSyncAgentCmd.AddCommand(skillSyncAgentListCmd)
	skillSyncAgentCmd.AddCommand(skillSyncAgentAddCmd)
	skillSyncAgentCmd.AddCommand(skillSyncAgentRemoveCmd)
	skillSyncCmd.AddCommand(skillSyncAgentCmd)
}

func runSkillSyncAgentAdd(name, path string) error {
	state, err := skill.LoadSyncState()
	if err != nil {
		return fmt.Errorf("failed to load sync state: %w", err)
	}

	agentIndex := len(state.Agents)
	if err := state.AddAgent(name, path); err != nil {
		return err
	}
	agent := state.Agents[agentIndex]

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		return fmt.Errorf("ensure skill repo: %w", err)
	}
	state.Repo = repoPath

	synced, conflicts, err := syncExistingSkillsToAgent(state, repoPath, agent, os.Stdout)
	if saveErr := skill.SaveSyncState(state); saveErr != nil {
		return fmt.Errorf("failed to save sync state: %w", saveErr)
	}
	if err != nil {
		return err
	}

	fmt.Printf("Added agent: %s (%s)\n", agent.Name, agent.Path)
	if synced > 0 {
		fmt.Printf("Synced %d existing skill(s) to %s.\n", synced, agent.Name)
	}
	if len(conflicts) > 0 {
		fmt.Printf("Conflicts: %v\n", conflicts)
		fmt.Println("Run 'skill-sync status' to review.")
	}
	return nil
}

func syncExistingSkillsToAgent(state *skill.SyncState, repoPath string, agent skill.AgentDir, out io.Writer) (int, []string, error) {
	names, err := skillNamesForAgentSync(state)
	if err != nil {
		return 0, nil, err
	}
	if len(names) == 0 {
		return 0, nil, nil
	}

	if out != nil {
		fmt.Fprintf(out, "Syncing %d existing skill(s) to %s...\n", len(names), agent.Name)
	}

	synced := 0
	var conflicts []string
	var failures []string
	for _, name := range names {
		if out != nil {
			fmt.Fprintf(out, "%s\n", name)
		}
		res, conflictAgents, err := skill.LinkSkillSafe(repoPath, name, []skill.AgentDir{agent}, out)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		linked, _ := summarizeResult(res)
		if len(conflictAgents) > 0 {
			markAgentSyncConflict(state, name, agent.Name)
			conflicts = append(conflicts, name)
		} else if _, ok := state.Skills[name]; !ok && state.Mode == skill.SyncModeLocal {
			updateLocalEntryWithConflicts(state, name, nil)
		}
		synced += linked
	}

	if len(failures) > 0 {
		return synced, conflicts, fmt.Errorf("failed to sync %d skill(s): %s", len(failures), strings.Join(failures, "; "))
	}
	return synced, conflicts, nil
}

func skillNamesForAgentSync(state *skill.SyncState) ([]string, error) {
	repoSkills, err := skill.ScanSkillRepo()
	if err != nil {
		return nil, err
	}
	if len(state.Skills) == 0 {
		return repoSkills, nil
	}

	repoSet := make(map[string]bool, len(repoSkills))
	for _, name := range repoSkills {
		repoSet[name] = true
	}

	names := make([]string, 0, len(state.Skills))
	for name := range state.Skills {
		if repoSet[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func markAgentSyncConflict(state *skill.SyncState, skillName, agentName string) {
	entry, ok := state.Skills[skillName]
	if !ok {
		entry = skill.SyncSkillEntry{Name: skillName}
	}
	for _, existing := range entry.ConflictAgents {
		if existing == agentName {
			entry.Status = skill.SyncStatusConflict
			entry.UpdatedAt = nowUTC()
			state.Skills[skillName] = entry
			return
		}
	}
	entry.ConflictAgents = append(entry.ConflictAgents, agentName)
	sort.Strings(entry.ConflictAgents)
	entry.Status = skill.SyncStatusConflict
	entry.UpdatedAt = nowUTC()
	state.Skills[skillName] = entry
}
