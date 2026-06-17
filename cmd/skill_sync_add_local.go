package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/nacos-group/nacos-cli/internal/skill"
)

// addOptions captures CLI flags for the add command (simplified).
type addOptions struct {
	fromAgent   string
	dryRun      bool
	nonInteract bool
	all         bool
}

// runSkillSyncAddLocal handles `skill-sync add` in local mode.
func runSkillSyncAddLocal(skillNames []string, opts addOptions) error {
	state, err := skill.LoadSyncState()
	if err != nil {
		return fmt.Errorf("load sync state: %v", err)
	}
	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		return fmt.Errorf("ensure skill repo: %v", err)
	}
	state.Mode = skill.SyncModeLocal
	state.Profile = skill.CurrentSyncProfile()
	state.Repo = repoPath

	if err := ensureAgents(state); err != nil {
		return err
	}
	if len(state.Agents) == 0 {
		return fmt.Errorf("no agent directories found; use 'skill-sync agent add'")
	}

	if opts.all {
		if !opts.dryRun {
			importAgentUnmanagedLocal(state, repoPath)
		}
		skills, err := skill.ScanSkillRepo()
		if err != nil {
			return fmt.Errorf("scan skill repo: %w", err)
		}
		skillNames = skills
	}

	if opts.dryRun {
		report, err := skill.BuildSyncReport(state, skillNames)
		if err != nil {
			return err
		}
		data, err := report.MarshalIndent()
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if len(skillNames) == 0 {
		fmt.Println("No local skills found.")
		return skill.SaveSyncState(state)
	}

	var failures []string
	for _, skillName := range skillNames {
		if err := addSingleLocal(state, repoPath, skillName, opts); err != nil {
			appendSkillFailure(&failures, skillName, err)
		}
	}

	return saveSyncStateAfterBatch(state, failures)
}

func addSingleLocal(state *skill.SyncState, repoPath, skillName string, opts addOptions) error {
	skillRepoPath := filepath.Join(repoPath, skillName)
	forceLink := false
	if _, err := os.Stat(skillRepoPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// Reverse import from an agent.
		imported, err := importFromAgent(state, repoPath, skillName, opts)
		if err != nil {
			return err
		}
		if !imported {
			fmt.Printf("Skipped: %s\n", skillName)
			return nil
		}
		// The source has been chosen explicitly or is unambiguous, so make it
		// the shared local source for every configured agent.
		forceLink = true
	}

	fmt.Printf("Adding %s...\n", skillName)
	var (
		res            *skill.LinkResult
		conflictAgents []string
		err            error
	)
	if forceLink {
		res, err = skill.LinkSkillForce(repoPath, skillName, state.Agents, os.Stdout)
	} else {
		res, conflictAgents, err = skill.LinkSkillSafe(repoPath, skillName, state.Agents, os.Stdout)
	}
	if err != nil {
		return err
	}

	updateLocalEntryWithConflicts(state, skillName, conflictAgents)
	printAddSummary(skillName, res, conflictAgents)
	return nil
}

// importFromAgent picks a source agent and reverse-imports its skill into the repo.
func importFromAgent(state *skill.SyncState, repoPath, skillName string, opts addOptions) (bool, error) {
	sources := collectLocalSkillSources(state, repoPath, skillName, false)
	if len(sources) == 0 {
		return false, fmt.Errorf("'%s' not found in repo or any agent. Create %s/SKILL.md and retry",
			skillName, filepath.Join(repoPath, skillName))
	}

	source, err := chooseLocalSourceOnly(skillName, sources, state.Config.AutoUpload, opts)
	if err != nil {
		return false, err
	}
	if source == nil {
		return false, nil
	}

	if err := skill.ImportAgentSkillToRepo(repoPath, source.Path, skillName); err != nil {
		return false, fmt.Errorf("import from %s: %w", source.Name, err)
	}
	fmt.Printf("Imported %s from %s into repo.\n", skillName, source.Name)
	return true, nil
}

func updateLocalEntryWithConflicts(state *skill.SyncState, skillName string, conflictAgents []string) {
	repoPath, _ := skill.GetSkillRepoPath()
	hash, _ := skill.ComputeDirectoryHash(filepath.Join(repoPath, skillName))
	entry, ok := state.Skills[skillName]
	if !ok {
		entry = skill.SyncSkillEntry{Name: skillName}
	}
	entry.LocalHash = hash
	entry.SyncedHash = hash
	entry.ConflictAgents = conflictAgents
	if len(conflictAgents) > 0 {
		entry.Status = skill.SyncStatusConflict
	} else {
		entry.Status = skill.SyncStatusLinked
	}
	entry.UpdatedAt = nowUTC()
	state.Skills[skillName] = entry
}

func printAddSummary(skillName string, result *skill.LinkResult, conflictAgents []string) {
	linked, skipped := summarizeResult(result)
	if len(conflictAgents) == 0 {
		fmt.Printf("Added: %s (linked to %d agent(s))\n", skillName, linked)
		return
	}
	fmt.Printf("Added: %s (linked to %d, skipped %d)\n", skillName, linked, skipped)
	fmt.Printf("Conflicts: %v\n", conflictAgents)
	fmt.Printf("Run 'skill-sync resolve %s' to fix.\n", skillName)
}

// summarizeResult counts agents linked vs skipped.
func summarizeResult(r *skill.LinkResult) (linked, skipped int) {
	for _, a := range r.Agents {
		if a.Error != nil {
			continue
		}
		if a.Skipped {
			skipped++
		} else {
			linked++
		}
	}
	return linked, skipped
}

// ensureAgents auto-discovers agent directories on first run.
func ensureAgents(state *skill.SyncState) error {
	discovered, err := skill.DiscoverAgents()
	if err != nil {
		return fmt.Errorf("discover agents: %w", err)
	}
	if len(discovered) == 0 {
		homeDir, _ := os.UserHomeDir()
		defaultPath := filepath.Join(homeDir, ".skills")
		if err := os.MkdirAll(defaultPath, 0755); err == nil {
			discovered = []skill.AgentDir{{Name: "default", Path: defaultPath, AutoFound: true}}
			fmt.Printf("Created default agent directory: %s\n", defaultPath)
		}
	}
	discovered = missingAgents(state.Agents, discovered)
	if len(discovered) > 0 {
		if len(state.Agents) == 0 {
			state.Agents = discovered
		} else {
			state.Agents = append(state.Agents, discovered...)
		}
		fmt.Println("Agents discovered:")
		for _, a := range discovered {
			fmt.Printf("  %-10s %s\n", a.Name, a.Path)
		}
		fmt.Println()
	}
	return nil
}

func missingAgents(existing, discovered []skill.AgentDir) []skill.AgentDir {
	if len(existing) == 0 {
		return discovered
	}

	names := make(map[string]struct{}, len(existing))
	paths := make(map[string]struct{}, len(existing))
	for _, agent := range existing {
		names[agent.Name] = struct{}{}
		if abs, err := filepath.Abs(agent.Path); err == nil {
			paths[abs] = struct{}{}
		}
	}

	var missing []skill.AgentDir
	for _, agent := range discovered {
		if _, ok := names[agent.Name]; ok {
			continue
		}
		abs, err := filepath.Abs(agent.Path)
		if err == nil {
			if _, ok := paths[abs]; ok {
				continue
			}
		}
		missing = append(missing, agent)
	}
	return missing
}

func readLine(r io.Reader) string {
	buf := make([]byte, 256)
	n, _ := r.Read(buf)
	for i := 0; i < n; i++ {
		if buf[i] == '\n' {
			return string(buf[:i])
		}
	}
	return string(buf[:n])
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
