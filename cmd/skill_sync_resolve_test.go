package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/skill"
)

func TestResolveOneNonInteractiveRequiresExplicitChoice(t *testing.T) {
	withTempHome(t)
	resetResolveFlags := func() {
		resolveUseNacos = false
		resolveUseLocal = false
		resolveUseRemote = false
		resolveUseRepo = false
		resolveUseAgent = ""
		resolveAll = false
		resolveNonInteract = false
	}
	resetResolveFlags()
	t.Cleanup(resetResolveFlags)
	resolveNonInteract = true

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "demo", "SKILL.md", "REPO")

	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeLocal,
		Label:   "latest",
		Repo:    repoPath,
		Agents: []skill.AgentDir{
			{Name: "codex", Path: filepath.Join(t.TempDir(), "codex")},
		},
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:   "demo",
				Status: skill.SyncStatusConflict,
			},
		},
	}

	err = resolveOne(state, "demo", state.Skills["demo"], nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "requires interaction") {
		t.Fatalf("error = %q, want interaction error", err.Error())
	}
}

func TestResolveOneUseRepoInLocalMode(t *testing.T) {
	withTempHome(t)
	resetResolveFlags := func() {
		resolveUseNacos = false
		resolveUseLocal = false
		resolveUseRemote = false
		resolveUseRepo = false
		resolveUseAgent = ""
		resolveAll = false
		resolveNonInteract = false
	}
	resetResolveFlags()
	t.Cleanup(resetResolveFlags)
	resolveUseRepo = true

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "demo", "SKILL.md", "REPO")
	codexPath := filepath.Join(t.TempDir(), "codex")
	claudePath := filepath.Join(t.TempDir(), "claude")
	if err := skill.LinkSkillToAgent(repoPath, "demo", codexPath); err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, claudePath, "demo", "SKILL.md", "CLAUDE")

	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeLocal,
		Label:   "latest",
		Repo:    repoPath,
		Agents: []skill.AgentDir{
			{Name: "codex", Path: codexPath},
			{Name: "claude", Path: claudePath},
		},
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:           "demo",
				Status:         skill.SyncStatusConflict,
				ConflictAgents: []string{"claude"},
			},
		},
	}

	if err := resolveOne(state, "demo", state.Skills["demo"], nil); err != nil {
		t.Fatal(err)
	}

	assertSkillSymlink(t, codexPath, "demo")
	assertSkillSymlink(t, claudePath, "demo")
	assertFileContent(t, filepath.Join(claudePath, "demo", "SKILL.md"), "REPO")
	entry := state.Skills["demo"]
	if entry.Status != skill.SyncStatusLinked {
		t.Fatalf("status = %s, want linked", entry.Status)
	}
	if len(entry.ConflictAgents) != 0 {
		t.Fatalf("conflict agents = %v, want none", entry.ConflictAgents)
	}
}
