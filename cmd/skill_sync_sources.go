package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nacos-group/nacos-cli/internal/skill"
)

type localSkillSource struct {
	Name       string
	Path       string
	Hash       string
	ModifiedAt time.Time
	IsRepo     bool
}

type skillSourceChoice string

const (
	skillSourceChoiceNacos skillSourceChoice = "nacos"
	skillSourceChoiceLocal skillSourceChoice = "local"
	skillSourceChoiceExit  skillSourceChoice = "exit"
)

func collectLocalSkillSources(state *skill.SyncState, repoPath, skillName string, includeRepo bool) []localSkillSource {
	var sources []localSkillSource
	repoSkillPath := filepath.Join(repoPath, skillName)
	repoAbs, _ := filepath.Abs(repoSkillPath)
	repoRealAbs := repoAbs
	if resolved, err := filepath.EvalSymlinks(repoSkillPath); err == nil {
		repoRealAbs, _ = filepath.Abs(resolved)
	}
	hasRepoSource := false

	for _, agent := range state.Agents {
		agentSkillPath := filepath.Join(agent.Path, skillName)
		info, err := os.Lstat(agentSkillPath)
		if err != nil {
			continue
		}
		sourcePath := agentSkillPath
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(agentSkillPath)
			if err != nil {
				continue
			}
			sourcePath = resolved
			sourceAbs, _ := filepath.Abs(sourcePath)
			if sourceAbs == repoAbs || sourceAbs == repoRealAbs {
				continue
			}
		}
		if _, err := os.Stat(filepath.Join(sourcePath, "SKILL.md")); err != nil {
			continue
		}
		hash, _ := skill.ComputeDirectoryHash(sourcePath)
		if hash == "" {
			continue
		}
		sourceAbs, _ := filepath.Abs(sourcePath)
		isRepo := sourceAbs == repoAbs
		if isRepo {
			hasRepoSource = true
		}
		sources = append(sources, localSkillSource{
			Name:       agent.Name,
			Path:       sourcePath,
			Hash:       hash,
			ModifiedAt: info.ModTime(),
			IsRepo:     isRepo,
		})
	}

	if includeRepo && !hasRepoSource {
		if info, err := os.Stat(repoSkillPath); err == nil && info.IsDir() {
			if _, err := os.Stat(filepath.Join(repoSkillPath, "SKILL.md")); err == nil {
				hash, _ := skill.ComputeDirectoryHash(repoSkillPath)
				if hash != "" {
					sources = append(sources, localSkillSource{
						Name:       "repo",
						Path:       repoSkillPath,
						Hash:       hash,
						ModifiedAt: info.ModTime(),
						IsRepo:     true,
					})
				}
			}
		}
	}

	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].IsRepo != sources[j].IsRepo {
			return !sources[i].IsRepo
		}
		return sources[i].ModifiedAt.After(sources[j].ModifiedAt)
	})
	return sources
}

func uniqueLocalSourcesByHash(sources []localSkillSource) []localSkillSource {
	seen := make(map[string]bool, len(sources))
	unique := make([]localSkillSource, 0, len(sources))
	for _, source := range sources {
		key := source.Hash
		if key == "" {
			key = source.Path
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, source)
	}
	return unique
}

func localSourcesDifferentFrom(sources []localSkillSource, hash string) []localSkillSource {
	if hash == "" {
		return uniqueLocalSourcesByHash(sources)
	}
	var different []localSkillSource
	for _, source := range sources {
		if source.Hash != hash {
			different = append(different, source)
		}
	}
	return uniqueLocalSourcesByHash(different)
}

func findLocalSource(sources []localSkillSource, name string) *localSkillSource {
	for i := range sources {
		if sources[i].Name == name {
			return &sources[i]
		}
	}
	return nil
}

func chooseNacosOrLocalSource(skillName string, sources []localSkillSource, autoUpload bool, opts addOptions) (skillSourceChoice, *localSkillSource, error) {
	sources = uniqueLocalSourcesByHash(sources)
	if opts.fromAgent != "" {
		if opts.fromAgent == "latest" {
			if len(sources) == 0 {
				return skillSourceChoiceExit, nil, fmt.Errorf("--from latest requested but no local sources exist")
			}
			return skillSourceChoiceLocal, &sources[0], nil
		}
		source := findLocalSource(sources, opts.fromAgent)
		if source == nil {
			return skillSourceChoiceExit, nil, fmt.Errorf("--from %q not found in local sources", opts.fromAgent)
		}
		return skillSourceChoiceLocal, source, nil
	}
	if opts.nonInteract {
		return skillSourceChoiceNacos, nil, nil
	}
	if len(sources) == 0 {
		fmt.Printf("\n%s has no usable local source.\n", skillName)
		fmt.Println("Choose source:")
		fmt.Println("  [1] Use Nacos version")
		fmt.Println("  [2] Exit")
		fmt.Printf("Choice [1]: ")
		switch strings.TrimSpace(readLine(os.Stdin)) {
		case "2":
			return skillSourceChoiceExit, nil, nil
		default:
			return skillSourceChoiceNacos, nil, nil
		}
	}

	fmt.Printf("\nLocal versions also exist for %s:\n", skillName)
	printLocalSources(sources)
	printAutoUploadSourceHint(autoUpload)
	fmt.Println("Choose source:")
	fmt.Println("  [1] Use Nacos version")
	for i, source := range sources {
		fmt.Printf("  [%d] Use %s version\n", i+2, source.Name)
	}
	exitChoice := len(sources) + 2
	fmt.Printf("  [%d] Exit\n", exitChoice)
	fmt.Printf("Choice [1]: ")

	ans := strings.TrimSpace(readLine(os.Stdin))
	if ans == "" || ans == "1" {
		return skillSourceChoiceNacos, nil, nil
	}
	idx, err := strconv.Atoi(ans)
	if err != nil {
		fmt.Println("Invalid choice.")
		return skillSourceChoiceExit, nil, nil
	}
	if idx == exitChoice {
		return skillSourceChoiceExit, nil, nil
	}
	if idx >= 2 && idx < exitChoice {
		return skillSourceChoiceLocal, &sources[idx-2], nil
	}
	fmt.Println("Invalid choice.")
	return skillSourceChoiceExit, nil, nil
}

func chooseLocalSourceOnly(skillName string, sources []localSkillSource, autoUpload bool, opts addOptions) (*localSkillSource, error) {
	sources = uniqueLocalSourcesByHash(sources)
	if len(sources) == 0 {
		return nil, nil
	}
	if opts.fromAgent == "latest" {
		return &sources[0], nil
	}
	if opts.fromAgent != "" {
		source := findLocalSource(sources, opts.fromAgent)
		if source == nil {
			return nil, fmt.Errorf("--from %q not found in local sources", opts.fromAgent)
		}
		return source, nil
	}
	if len(sources) == 1 {
		return &sources[0], nil
	}
	if opts.nonInteract {
		return nil, fmt.Errorf("multiple local sources found for %s; use --from <agent> or --from latest", skillName)
	}

	fmt.Printf("\nLocal versions found for %s:\n", skillName)
	printLocalSources(sources)
	printAutoUploadSourceHint(autoUpload)
	fmt.Println("Choose source:")
	for i, source := range sources {
		fmt.Printf("  [%d] Use %s version\n", i+1, source.Name)
	}
	exitChoice := len(sources) + 1
	fmt.Printf("  [%d] Exit\n", exitChoice)
	fmt.Printf("Choice [%d]: ", exitChoice)

	ans := strings.TrimSpace(readLine(os.Stdin))
	if ans == "" {
		return nil, nil
	}
	idx, err := strconv.Atoi(ans)
	if err != nil {
		return nil, nil
	}
	if idx >= 1 && idx <= len(sources) {
		return &sources[idx-1], nil
	}
	return nil, nil
}

func printLocalSources(sources []localSkillSource) {
	for _, source := range sources {
		fmt.Printf("  %-8s hash %s\n", source.Name, shortHash(source.Hash))
	}
}

func printAutoUploadSourceHint(autoUpload bool) {
	if autoUpload {
		fmt.Println("Auto-upload: enabled")
		fmt.Println("Choosing a local source will mark Local changes; the daemon will upload it as draft.")
		return
	}
	fmt.Println("Auto-upload: disabled")
	fmt.Println("Choosing a local source will mark Local changes and will not upload automatically.")
}

func promoteLocalSourceToRepo(state *skill.SyncState, repoPath, skillName string, source localSkillSource) error {
	repoSkillPath := filepath.Join(repoPath, skillName)
	sourceAbs, _ := filepath.Abs(source.Path)
	repoAbs, _ := filepath.Abs(repoSkillPath)

	if sourceAbs != repoAbs {
		stageRoot, err := os.MkdirTemp("", "nacos-skill-local-source-")
		if err != nil {
			return fmt.Errorf("stage local source: %w", err)
		}
		defer os.RemoveAll(stageRoot)
		if err := skill.ImportAgentSkillToRepo(stageRoot, source.Path, skillName); err != nil {
			return fmt.Errorf("stage from %s: %w", source.Name, err)
		}
		stagedSource := filepath.Join(stageRoot, skillName)
		if err := backupRepoDir(repoPath, skillName); err != nil {
			return fmt.Errorf("backup repo dir: %w", err)
		}
		if err := skill.ImportAgentSkillToRepo(repoPath, stagedSource, skillName); err != nil {
			return fmt.Errorf("import from %s: %w", source.Name, err)
		}
		fmt.Printf("Imported %s from %s into repo.\n", skillName, source.Name)
	}

	if _, err := skill.LinkSkillForce(repoPath, skillName, state.Agents, os.Stdout); err != nil {
		return err
	}

	hash, _ := skill.ComputeDirectoryHash(repoSkillPath)
	now := time.Now().UTC().Format(time.RFC3339)
	entry, ok := state.Skills[skillName]
	if !ok {
		entry = skill.SyncSkillEntry{Name: skillName}
	}
	entry.Label = state.Label
	entry.LocalHash = hash
	entry.ConflictAgents = nil
	if state.Mode == skill.SyncModeNacos {
		entry.Status = skill.SyncStatusLocalChanges
	} else {
		entry.SyncedHash = hash
		entry.Status = skill.SyncStatusLinked
	}
	entry.UpdatedAt = now
	state.Skills[skillName] = entry
	return nil
}
