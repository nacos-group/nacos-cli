package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nacos-group/nacos-cli/internal/skill"
)

// startInitOptions controls the initial sync behavior of the start command.
type startInitOptions struct {
	UseRemoteOnConflict bool
	Refresh             bool
}

type startConflict struct {
	Name   string
	Reason string
}

type startConflictDecision string

const (
	startConflictUseNacos startConflictDecision = "nacos"
	startConflictSkip     startConflictDecision = "skip"
	startConflictExit     startConflictDecision = "exit"
)

// runLocalInitialSync handles the first-run sync in local mode.
//
// Default behavior:
//   - Link every skill in the central repo to all agents using LinkSkillSafe
//     (any agent with a different real directory is left untouched and recorded
//     as a conflict on the entry).
func runLocalInitialSync(state *skill.SyncState, opts startInitOptions) bool {
	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: skill repo: %v\n", err)
		return true
	}
	state.Repo = repoPath

	skills, _ := skill.ScanSkillRepo()
	if len(skills) == 0 {
		fmt.Println("Repository is empty. Put a skill in", repoPath, "and rerun.")
		_ = skill.SaveSyncState(state)
		return true
	}

	fmt.Printf("Linking %d skill(s) from %s...\n", len(skills), repoPath)
	for _, name := range skills {
		linkSingleLocal(state, repoPath, name, opts)
	}

	_ = skill.SaveSyncState(state)
	return true
}

func linkSingleLocal(state *skill.SyncState, repoPath, skillName string, opts startInitOptions) {
	var (
		res            *skill.LinkResult
		conflictAgents []string
		err            error
	)
	if opts.UseRemoteOnConflict || opts.Refresh {
		res, err = skill.LinkSkillForce(repoPath, skillName, state.Agents, os.Stdout)
	} else {
		res, conflictAgents, err = skill.LinkSkillSafe(repoPath, skillName, state.Agents, os.Stdout)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %s\tlink failed: %v\n", skillName, err)
		return
	}
	updateLocalEntryWithConflicts(state, skillName, conflictAgents)
	_ = res
}

// importAgentUnmanagedLocal scans agent dirs for unmanaged skills (real dirs
// not yet in the repo). Single-source or all-same-content cases are imported
// automatically; multi-version conflicts are reported and skipped.
func importAgentUnmanagedLocal(state *skill.SyncState, repoPath string) {
	type entry struct {
		name       string
		candidates []skill.AgentSearchResult
	}
	groups := make(map[string][]skill.AgentSearchResult)
	for _, agent := range state.Agents {
		entries, err := os.ReadDir(agent.Path)
		if err != nil {
			continue
		}
		for _, dir := range entries {
			if !dir.IsDir() || strings.HasPrefix(dir.Name(), ".") {
				continue
			}
			name := dir.Name()
			if _, err := os.Stat(filepath.Join(repoPath, name)); err == nil {
				continue
			}
			agentSkillPath := filepath.Join(agent.Path, name)
			info, err := os.Lstat(agentSkillPath)
			if err != nil || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if _, err := os.Stat(filepath.Join(agentSkillPath, "SKILL.md")); err != nil {
				continue
			}
			hash, _ := skill.ComputeDirectoryHash(agentSkillPath)
			groups[name] = append(groups[name], skill.AgentSearchResult{
				Agent:      agent,
				Path:       agentSkillPath,
				ModifiedAt: info.ModTime(),
				Hash:       hash,
			})
		}
	}
	if len(groups) == 0 {
		return
	}

	var imported, skipped []entry
	names := make([]string, 0, len(groups))
	for n := range groups {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		cands := groups[name]
		sort.Slice(cands, func(i, j int) bool {
			return cands[i].ModifiedAt.After(cands[j].ModifiedAt)
		})
		if isAllSameHash(cands) {
			source := cands[0]
			if err := skill.ImportAgentSkillToRepo(repoPath, source.Path, name); err != nil {
				fmt.Fprintf(os.Stderr, "  %s\timport failed: %v\n", name, err)
				continue
			}
			imported = append(imported, entry{name: name, candidates: cands})
			fmt.Printf("  %s\timported (from %s)\n", name, source.Agent.Name)
		} else {
			skipped = append(skipped, entry{name: name, candidates: cands})
		}
	}

	if len(imported) > 0 {
		fmt.Printf("Imported %d unmanaged skill(s).\n", len(imported))
	}
	if len(skipped) > 0 {
		fmt.Printf("\nSkipped %d unmanaged skill(s) with multi-version conflicts:\n", len(skipped))
		for _, e := range skipped {
			agentNames := make([]string, len(e.candidates))
			for i, c := range e.candidates {
				agentNames[i] = c.Agent.Name
			}
			fmt.Printf("  %s\tin %v\n", e.name, agentNames)
		}
		fmt.Printf("Use 'skill-sync add <skill>' to choose a source.\n")
	}
}

func isAllSameHash(results []skill.AgentSearchResult) bool {
	if len(results) <= 1 {
		return true
	}
	h := results[0].Hash
	if h == "" {
		return false
	}
	for _, r := range results[1:] {
		if r.Hash != h {
			return false
		}
	}
	return true
}

// runNacosInitialSync pulls tracked skills from Nacos on first start.
//
// Default behavior:
//   - Iterate over state.Skills (the user's tracked skills). For each skill,
//     fetch from Nacos. If repo or any agent already has different content,
//     skip and mark Conflict (resolve later). Otherwise pull and link.
//
// With opts.UseRemoteOnConflict:
//   - On conflict, force overwrite (LinkSkillForce) instead of skip.
//
// With opts.Refresh:
//   - Re-pull every tracked skill regardless of local state.
func runNacosInitialSync(state *skill.SyncState, opts startInitOptions) bool {
	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: skill repo: %v\n", err)
		return true
	}
	state.Repo = repoPath

	nacosClient := mustNewNacosClient()
	skillService := skill.NewSkillService(nacosClient)

	var skillNames []string
	for name := range state.Skills {
		skillNames = append(skillNames, name)
	}

	if len(skillNames) == 0 {
		fmt.Println("No skills added yet.")
		fmt.Println("Use 'skill-sync add <name>' to add a skill, or 'skill-sync add --all' to add every skill on Nacos.")
		_ = skill.SaveSyncState(state)
		return true
	}

	sort.Strings(skillNames)
	fmt.Printf("Pulling %d skill(s) from Nacos (label=%s)...\n", len(skillNames), state.Label)

	fetchedByName, preflightConflicts := preflightNacosStartConflicts(state, repoPath, skillNames, skillService, opts)
	if len(preflightConflicts) > 0 && !opts.UseRemoteOnConflict {
		decision := decideStartConflicts(preflightConflicts, !syncStartForeground && !syncStartNonInteract)
		switch decision {
		case startConflictUseNacos:
			opts.UseRemoteOnConflict = true
		case startConflictSkip:
		case startConflictExit:
			fmt.Println("Aborted. Daemon not started.")
			return false
		}
	}

	conflicts := 0
	for _, name := range skillNames {
		if c := pullAndLinkOne(state, repoPath, name, skillService, opts, fetchedByName[name]); c {
			conflicts++
		}
	}

	if conflicts > 0 {
		fmt.Printf("\n%d skill(s) need attention. Run 'skill-sync status' for details.\n", conflicts)
	}

	_ = skill.SaveSyncState(state)
	return true
}

func preflightNacosStartConflicts(state *skill.SyncState, repoPath string, skillNames []string, svc *skill.SkillService, opts startInitOptions) (map[string]*skill.SkillQueryResult, []startConflict) {
	fetchedByName := make(map[string]*skill.SkillQueryResult, len(skillNames))
	var conflicts []startConflict

	for _, name := range skillNames {
		if shouldSkipInitialPullForLocal(state, name, opts) {
			continue
		}
		fetched, err := svc.FetchSkill(name, "", state.Label, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s\tfetch error: %v\n", name, err)
			continue
		}
		ensureFetchedResolvedVersion(svc, name, state.Label, fetched)
		fetchedByName[name] = fetched
		if fetched.Deleted || (!fetched.Updated && fetched.Md5 == "") {
			continue
		}

		repoSkillDir := filepath.Join(repoPath, name)
		repoHash, _ := skill.ComputeDirectoryHash(repoSkillDir)
		remoteHash := ""
		if fetched.Updated && len(fetched.ZipBytes) > 0 {
			sourceDir, cleanup, err := stageFetchedSkill(name, fetched.ZipBytes)
			if err == nil {
				remoteHash, _ = skill.ComputeDirectoryHash(sourceDir)
			}
			if cleanup != nil {
				cleanup()
			}
		}

		if repoHash != "" && isRepoNacosConflict(state, name, repoHash, remoteHash, fetched, opts) {
			conflicts = append(conflicts, startConflict{
				Name:   name,
				Reason: "local repo differs from Nacos",
			})
			continue
		}

		sourceHash := remoteHash
		if sourceHash == "" {
			sourceHash = repoHash
		}
		if sourceHash == "" {
			continue
		}
		localSources := localSourcesDifferentFrom(
			collectLocalSkillSources(state, repoPath, name, false),
			sourceHash,
		)
		if len(localSources) > 0 {
			agentNames := make([]string, 0, len(localSources))
			for _, source := range localSources {
				agentNames = append(agentNames, source.Name)
			}
			conflicts = append(conflicts, startConflict{
				Name:   name,
				Reason: fmt.Sprintf("local agent versions differ from Nacos/source: %s", strings.Join(agentNames, ", ")),
			})
		}
	}

	return fetchedByName, conflicts
}

func isRepoNacosConflict(state *skill.SyncState, name, repoHash, remoteHash string, fetched *skill.SkillQueryResult, opts startInitOptions) bool {
	if opts.Refresh {
		entry := state.Skills[name]
		return entry.SyncedHash != "" && entry.SyncedHash != repoHash
	}
	if remoteHash != "" && repoHash == remoteHash {
		return false
	}
	if !fetched.Updated {
		return false
	}
	entry := state.Skills[name]
	return entry.SyncedHash != "" && entry.SyncedHash != repoHash
}

func shouldSkipInitialPullForLocal(state *skill.SyncState, name string, opts startInitOptions) bool {
	if opts.Refresh || opts.UseRemoteOnConflict {
		return false
	}
	entry, ok := state.Skills[name]
	return ok && shouldProtectLocalFromRemote(entry.Status)
}

func decideStartConflicts(conflicts []startConflict, interactive bool) startConflictDecision {
	if !interactive {
		printStartConflicts(conflicts)
		fmt.Println("Non-interactive start: recorded and skipped conflicts.")
		return startConflictSkip
	}

	printStartConflicts(conflicts)
	fmt.Println("Choose how to continue:")
	fmt.Println("  [1] Use Nacos version for all conflicts")
	fmt.Println("  [2] Record and skip conflicts")
	fmt.Println("  [3] Exit")
	fmt.Printf("Choice [2]: ")

	switch strings.TrimSpace(readLine(os.Stdin)) {
	case "1":
		return startConflictUseNacos
	case "", "2":
		return startConflictSkip
	case "3":
		return startConflictExit
	default:
		fmt.Println("Invalid choice. Record and skip conflicts.")
		return startConflictSkip
	}
}

func printStartConflicts(conflicts []startConflict) {
	fmt.Println()
	fmt.Println("Conflicts found:")
	for _, conflict := range conflicts {
		fmt.Printf("  %-12s %s\n", conflict.Name, conflict.Reason)
	}
	fmt.Println()
}

// pullAndLinkOne fetches a single skill from Nacos and links it. Returns true
// when the skill ended up in conflict state.
func pullAndLinkOne(state *skill.SyncState, repoPath, name string, svc *skill.SkillService, opts startInitOptions, fetched *skill.SkillQueryResult) bool {
	if shouldSkipInitialPullForLocal(state, name, opts) {
		entry := state.Skills[name]
		fmt.Printf("  %s\tkept %s\n", name, entry.Status.DisplayString())
		return false
	}
	if fetched == nil {
		var err error
		fetched, err = svc.FetchSkill(name, "", state.Label, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s\tfetch error: %v\n", name, err)
			return false
		}
	}
	ensureFetchedResolvedVersion(svc, name, state.Label, fetched)
	if fetched.Deleted || (!fetched.Updated && fetched.Md5 == "") {
		fmt.Printf("  %s\tnot pullable (no online version)\n", name)
		return false
	}

	repoSkillDir := filepath.Join(repoPath, name)
	repoHash, _ := skill.ComputeDirectoryHash(repoSkillDir)
	repoNeedsRefresh := opts.Refresh || repoHash == "" || (fetched.Updated && repoHash != fetched.Md5)

	// Detect repo-vs-Nacos conflict (only meaningful when remote returned a new
	// version; if Updated=false, the server-side md5 we got equals what we last
	// pulled, so repoHash should match remoteMd5).
	repoConflict := false
	if repoHash != "" && fetched.Updated && repoHash != fetched.Md5 {
		// Compare against the entry's last synced hash to decide whether the
		// repo divergence is a real conflict (someone modified locally) or
		// simply that the repo is on an older version we last pulled.
		entry := state.Skills[name]
		if entry.SyncedHash != "" && entry.SyncedHash != repoHash {
			repoConflict = true
		}
	}

	// Pull into repo when needed.
	if repoNeedsRefresh && fetched.Updated {
		if repoConflict && !opts.UseRemoteOnConflict {
			fmt.Printf("  %s\tskipped: local repo differs from Nacos (run 'resolve %s')\n", name, name)
			markRepoConflict(state, name, fetched, repoHash)
			return true
		}
		if repoConflict && opts.UseRemoteOnConflict {
			// Backup repo dir before overwriting.
			if err := backupRepoDir(repoPath, name); err != nil {
				fmt.Fprintf(os.Stderr, "  %s\tbackup repo failed: %v\n", name, err)
				return false
			}
		}
		if err := os.RemoveAll(repoSkillDir); err != nil {
			fmt.Fprintf(os.Stderr, "  %s\tclear repo failed: %v\n", name, err)
			return false
		}
		if err := skill.ExtractSkillZip(fetched.ZipBytes, repoPath); err != nil {
			fmt.Fprintf(os.Stderr, "  %s\textract failed: %v\n", name, err)
			return false
		}
	}

	// Link to agents.
	var (
		conflictAgents []string
		err            error
	)
	if opts.UseRemoteOnConflict {
		if _, err := skill.LinkSkillForce(repoPath, name, state.Agents, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "  %s\tlink failed: %v\n", name, err)
			return false
		}
	} else {
		_, conflictAgents, err = skill.LinkSkillSafe(repoPath, name, state.Agents, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s\tlink failed: %v\n", name, err)
			return false
		}
	}

	updateNacosEntryAfterPull(state, name, fetched, conflictAgents)
	if len(conflictAgents) > 0 {
		fmt.Printf("  %s\tpulled %s, conflicts in %v (run 'resolve %s')\n", name, fetched.ResolvedVersion, conflictAgents, name)
		return true
	}
	fmt.Printf("  %s\tpulled %s\n", name, fetched.ResolvedVersion)
	return false
}

func markRepoConflict(state *skill.SyncState, name string, fetched *skill.SkillQueryResult, repoHash string) {
	entry, ok := state.Skills[name]
	if !ok {
		entry = skill.SyncSkillEntry{Name: name}
	}
	entry.Label = state.Label
	entry.ResolvedVersion = fetched.ResolvedVersion
	entry.RemoteMd5 = fetched.Md5
	entry.LocalHash = repoHash
	entry.Status = skill.SyncStatusConflict
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	state.Skills[name] = entry
}

func updateNacosEntryAfterPull(state *skill.SyncState, name string, fetched *skill.SkillQueryResult, conflictAgents []string) {
	repoPath, _ := skill.GetSkillRepoPath()
	hash, _ := skill.ComputeDirectoryHash(filepath.Join(repoPath, name))
	entry, ok := state.Skills[name]
	if !ok {
		entry = skill.SyncSkillEntry{Name: name}
	}
	entry.Label = state.Label
	entry.ResolvedVersion = fetched.ResolvedVersion
	entry.RemoteMd5 = fetched.Md5
	entry.LocalHash = hash
	entry.SyncedHash = hash
	entry.ConflictAgents = conflictAgents
	if len(conflictAgents) > 0 {
		entry.Status = skill.SyncStatusConflict
	} else {
		entry.Status = skill.SyncStatusSynced
	}
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	state.Skills[name] = entry
}

func backupRepoDir(repoPath, name string) error {
	src := filepath.Join(repoPath, name)
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect repo skill: %w", err)
	}
	backupDir := filepath.Join(repoPath, "..", ".skill-sync-backup")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("create repo backup dir: %w", err)
	}
	dest := filepath.Join(backupDir, fmt.Sprintf("repo-%s-%s", name, time.Now().Format("20060102T150405")))
	if err := os.Rename(src, dest); err != nil {
		return fmt.Errorf("backup repo skill: %w", err)
	}
	return nil
}

// listAllNacosSkills paginates through all skills accessible in the namespace.
func listAllNacosSkills(svc *skill.SkillService) ([]skill.SkillListItem, error) {
	var all []skill.SkillListItem
	pageNo := 1
	for {
		items, total, err := svc.ListSkills("", pageNo, 100)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(all) >= total || len(items) == 0 {
			break
		}
		pageNo++
	}
	return all, nil
}
