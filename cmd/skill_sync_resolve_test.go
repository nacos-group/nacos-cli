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
