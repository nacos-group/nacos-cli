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

// runSkillSyncAddNacos handles `skill-sync add` in Nacos mode.
// Behavior:
//   - skill present on Nacos -> choose Nacos unless a local source is selected
//   - skill missing on Nacos but present locally -> choose a local source
//   - local source choices become Local changes; auto-upload decides upload
//   - skill missing everywhere -> error
func runSkillSyncAddNacos(skillNames []string, opts addOptions) error {
	state, err := skill.LoadSyncState()
	if err != nil {
		return fmt.Errorf("load sync state: %v", err)
	}
	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		return fmt.Errorf("ensure skill repo: %v", err)
	}
	state.Mode = skill.SyncModeNacos
	state.Profile = skill.CurrentSyncProfile()
	state.Repo = repoPath

	if err := ensureAgents(state); err != nil {
		return err
	}
	if len(state.Agents) == 0 {
		return fmt.Errorf("no agent directories found; use 'skill-sync agent add'")
	}

	var skillService *skill.SkillService
	if opts.all {
		nacosClient := mustNewNacosClient()
		skillService = skill.NewSkillService(nacosClient)
		nacosSkills, err := listAllNacosSkills(skillService)
		if err != nil {
			return fmt.Errorf("list Nacos skills: %w", err)
		}
		skillNames = selectUnmanagedNacosSkillNames(state, nacosSkills)
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
		fmt.Println("No new Nacos skills to add.")
		return skill.SaveSyncState(state)
	}

	if skillService == nil {
		nacosClient := mustNewNacosClient()
		skillService = skill.NewSkillService(nacosClient)
	}

	var failures []string
	for _, skillName := range skillNames {
		if err := addSingleNacos(state, repoPath, skillName, skillService, opts); err != nil {
			appendSkillFailure(&failures, skillName, err)
		}
	}

	return saveSyncStateAfterBatch(state, failures)
}

func selectUnmanagedNacosSkillNames(state *skill.SyncState, items []skill.SkillListItem) []string {
	seen := make(map[string]struct{}, len(items))
	var names []string
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		if _, ok := state.Skills[name]; ok {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func addSingleNacos(state *skill.SyncState, repoPath, skillName string, svc *skill.SkillService, opts addOptions) error {
	fmt.Printf("Adding %s (Nacos mode, label=%s)...\n", skillName, state.Label)

	fetched, err := svc.FetchSkill(skillName, "", state.Label, "")
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	ensureFetchedResolvedVersion(svc, skillName, state.Label, fetched)

	remoteAvailable := !fetched.Deleted && (fetched.Updated || fetched.Md5 != "")

	if !remoteAvailable {
		return addLocalSourceAsChanges(state, repoPath, skillName, opts)
	}

	remoteHash := ""
	if fetched.Updated && len(fetched.ZipBytes) > 0 {
		sourceDir, cleanup, err := stageFetchedSkill(skillName, fetched.ZipBytes)
		if err != nil {
			return err
		}
		remoteHash, err = skill.ComputeDirectoryHash(sourceDir)
		cleanup()
		if err != nil {
			return err
		}
	}

	localSources := localSourcesDifferentFrom(
		collectLocalSkillSources(state, repoPath, skillName, true),
		remoteHash,
	)
	if len(localSources) > 0 {
		fmt.Printf("Found %s on Nacos", skillName)
		if fetched.ResolvedVersion != "" {
			fmt.Printf(" (%s)", fetched.ResolvedVersion)
		}
		fmt.Println(".")
		choice, source, err := chooseNacosOrLocalSource(skillName, localSources, state.Config.AutoUpload, opts)
		if err != nil {
			return err
		}
		switch choice {
		case skillSourceChoiceNacos:
			return applyFetchedNacosSkill(state, repoPath, skillName, fetched, true)
		case skillSourceChoiceLocal:
			if source == nil {
				return nil
			}
			if err := promoteLocalSourceToRepo(state, repoPath, skillName, *source); err != nil {
				return err
			}
			recordLocalSourceRemoteInfo(state, skillName, fetched)
			printLocalSourceSelected(skillName, source.Name, state.Mode, state.Config.AutoUpload)
			return nil
		case skillSourceChoiceExit:
			fmt.Printf("Skipped: %s\n", skillName)
			return nil
		}
	}

	return applyFetchedNacosSkill(state, repoPath, skillName, fetched, false)
}

func applyFetchedNacosSkill(state *skill.SyncState, repoPath, skillName string, fetched *skill.SkillQueryResult, force bool) error {
	if fetched.Updated {
		repoSkillDir := filepath.Join(repoPath, skillName)
		if force {
			if err := backupRepoDir(repoPath, skillName); err != nil {
				return fmt.Errorf("backup repo dir: %w", err)
			}
		} else if err := os.RemoveAll(repoSkillDir); err != nil {
			return fmt.Errorf("clear repo dir: %w", err)
		}
		if err := skill.ExtractSkillZip(fetched.ZipBytes, repoPath); err != nil {
			return fmt.Errorf("extract: %w", err)
		}
	}

	var (
		res            *skill.LinkResult
		conflictAgents []string
		err            error
	)
	if force {
		res, err = skill.LinkSkillForce(repoPath, skillName, state.Agents, os.Stdout)
	} else {
		res, conflictAgents, err = skill.LinkSkillSafe(repoPath, skillName, state.Agents, os.Stdout)
	}
	if err != nil {
		return err
	}

	updateNacosEntryWithConflicts(state, skillName, fetched, conflictAgents)
	printAddSummary(skillName, res, conflictAgents)
	return nil
}

func addLocalSourceAsChanges(state *skill.SyncState, repoPath, skillName string, opts addOptions) error {
	sources := collectLocalSkillSources(state, repoPath, skillName, true)
	if len(sources) == 0 {
		return fmt.Errorf("'%s' not found on Nacos and not in any agent", skillName)
	}

	fmt.Printf("%s not found on Nacos.\n", skillName)
	source, err := chooseLocalSourceOnly(skillName, sources, state.Config.AutoUpload, opts)
	if err != nil {
		return err
	}
	if source == nil {
		fmt.Printf("Skipped: %s\n", skillName)
		return nil
	}
	if err := promoteLocalSourceToRepo(state, repoPath, skillName, *source); err != nil {
		return err
	}
	printLocalSourceSelected(skillName, source.Name, state.Mode, state.Config.AutoUpload)
	return nil
}

func recordLocalSourceRemoteInfo(state *skill.SyncState, skillName string, fetched *skill.SkillQueryResult) {
	entry, ok := state.Skills[skillName]
	if !ok {
		entry = skill.SyncSkillEntry{Name: skillName}
	}
	entry.Label = state.Label
	entry.RemoteMd5 = fetched.Md5
	entry.ResolvedVersion = fetched.ResolvedVersion
	entry.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	state.Skills[skillName] = entry
}

func printLocalSourceSelected(skillName, sourceName string, mode skill.SyncMode, autoUpload bool) {
	if mode != skill.SyncModeNacos {
		fmt.Printf("Selected %s version for %s. Status: Linked.\n", sourceName, skillName)
		return
	}
	fmt.Printf("Selected %s version for %s. Status: Local changes.\n", sourceName, skillName)
	if autoUpload {
		fmt.Println("Auto-upload is enabled; the daemon will upload it as draft.")
	} else {
		fmt.Println("Auto-upload is disabled; it will stay local until uploaded manually.")
	}
}

func updateNacosEntryWithConflicts(state *skill.SyncState, skillName string, fetched *skill.SkillQueryResult, conflictAgents []string) {
	repoPath, _ := skill.GetSkillRepoPath()
	hash, _ := skill.ComputeDirectoryHash(filepath.Join(repoPath, skillName))
	entry, ok := state.Skills[skillName]
	if !ok {
		entry = skill.SyncSkillEntry{Name: skillName}
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
	state.Skills[skillName] = entry
}
