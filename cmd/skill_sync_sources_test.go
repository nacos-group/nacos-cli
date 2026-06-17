package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/skill"
)

func TestCollectLocalSkillSourcesSkipsAgentsLinkedToRepo(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "pdf", "SKILL.md", "NACOS VERSION")

	codex := t.TempDir()
	defaultAgent := t.TempDir()
	claude := t.TempDir()
	if err := skill.LinkSkillToAgent(repoPath, "pdf", codex); err != nil {
		t.Fatal(err)
	}
	if err := skill.LinkSkillToAgent(repoPath, "pdf", defaultAgent); err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, claude, "pdf", "SKILL.md", "CLAUDE VERSION")

	state := &skill.SyncState{
		Agents: []skill.AgentDir{
			{Name: "codex", Path: codex},
			{Name: "default", Path: defaultAgent},
			{Name: "claude", Path: claude},
		},
	}

	sources := collectLocalSkillSources(state, repoPath, "pdf", true)
	var names []string
	for _, source := range sources {
		names = append(names, source.Name)
	}

	if containsString(names, "codex") || containsString(names, "default") {
		t.Fatalf("repo symlinks should not be listed as agent sources, got %v", names)
	}
	if !containsString(names, "claude") {
		t.Fatalf("real agent source missing, got %v", names)
	}
	if !containsString(names, "repo") {
		t.Fatalf("repo source missing, got %v", names)
	}
}

func TestPromoteLocalSourceToRepoStagesSelectedAgentBeforeRelinking(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "pdf", "SKILL.md", "NACOS VERSION")

	codex := t.TempDir()
	claude := t.TempDir()
	qoder := t.TempDir()
	defaultAgent := t.TempDir()
	writeSkillFile(t, codex, "pdf", "SKILL.md", "CODEX VERSION")
	writeSkillFile(t, claude, "pdf", "SKILL.md", "CLAUDE VERSION")
	writeSkillFile(t, defaultAgent, "pdf", "SKILL.md", "DEFAULT VERSION")

	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Label:   "latest",
		Agents: []skill.AgentDir{
			{Name: "codex", Path: codex},
			{Name: "claude", Path: claude},
			{Name: "qoder", Path: qoder},
			{Name: "default", Path: defaultAgent},
		},
		Skills: map[string]skill.SyncSkillEntry{},
	}

	source := localSkillSource{Name: "codex", Path: filepath.Join(codex, "pdf")}
	var promoteErr error
	captureStdout(t, func() {
		promoteErr = promoteLocalSourceToRepo(state, repoPath, "pdf", source)
	})
	if promoteErr != nil {
		t.Fatal(promoteErr)
	}

	assertFileContent(t, filepath.Join(repoPath, "pdf", "SKILL.md"), "CODEX VERSION")
	for _, agentPath := range []string{codex, claude, qoder, defaultAgent} {
		skillPath := filepath.Join(agentPath, "pdf")
		info, err := os.Lstat(skillPath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s should be a symlink, got %v", skillPath, info.Mode())
		}
		assertFileContent(t, filepath.Join(skillPath, "SKILL.md"), "CODEX VERSION")
	}
	if got := state.Skills["pdf"].Status; got != skill.SyncStatusLocalChanges {
		t.Fatalf("status = %s, want Local changes", got)
	}
}

func TestChooseLocalSourceOnlyNonInteractiveRequiresFromForMultipleSources(t *testing.T) {
	sources := []localSkillSource{
		{Name: "codex", Path: "/tmp/codex/demo", Hash: "hash-a"},
		{Name: "claude", Path: "/tmp/claude/demo", Hash: "hash-b"},
	}

	source, err := chooseLocalSourceOnly("demo", sources, false, addOptions{nonInteract: true})
	if err == nil {
		t.Fatal("expected error")
	}
	if source != nil {
		t.Fatalf("source = %#v, want nil", source)
	}
	if !strings.Contains(err.Error(), "use --from") {
		t.Fatalf("error = %q, want --from hint", err.Error())
	}
}

func TestChooseLocalSourceOnlyFromMissingErrors(t *testing.T) {
	sources := []localSkillSource{{Name: "codex", Path: "/tmp/codex/demo", Hash: "hash-a"}}

	source, err := chooseLocalSourceOnly("demo", sources, false, addOptions{fromAgent: "claude", nonInteract: true})
	if err == nil {
		t.Fatal("expected error")
	}
	if source != nil {
		t.Fatalf("source = %#v, want nil", source)
	}
	if !strings.Contains(err.Error(), `--from "claude" not found`) {
		t.Fatalf("error = %q, want missing --from error", err.Error())
	}
}

func TestChooseNacosOrLocalSourceNonInteractiveDefaultsToNacos(t *testing.T) {
	sources := []localSkillSource{{Name: "codex", Path: "/tmp/codex/demo", Hash: "hash-a"}}

	choice, source, err := chooseNacosOrLocalSource("demo", sources, false, addOptions{nonInteract: true})
	if err != nil {
		t.Fatal(err)
	}
	if choice != skillSourceChoiceNacos {
		t.Fatalf("choice = %s, want nacos", choice)
	}
	if source != nil {
		t.Fatalf("source = %#v, want nil", source)
	}
}

func TestChooseNacosOrLocalSourceFromLatestSelectsLocal(t *testing.T) {
	sources := []localSkillSource{
		{Name: "codex", Path: "/tmp/codex/demo", Hash: "hash-a"},
		{Name: "claude", Path: "/tmp/claude/demo", Hash: "hash-b"},
	}

	choice, source, err := chooseNacosOrLocalSource("demo", sources, false, addOptions{fromAgent: "latest", nonInteract: true})
	if err != nil {
		t.Fatal(err)
	}
	if choice != skillSourceChoiceLocal {
		t.Fatalf("choice = %s, want local", choice)
	}
	if source == nil || source.Name != "codex" {
		t.Fatalf("source = %#v, want codex", source)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
