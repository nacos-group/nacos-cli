package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/skill"
)

func TestEnsureAgentsMergesNewWellKnownAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	codexPath := filepath.Join(home, ".codex", "skills")
	agentsPath := filepath.Join(home, ".agents", "skills")
	for _, path := range []string{codexPath, agentsPath} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}

	state := &skill.SyncState{
		Agents: []skill.AgentDir{
			{Name: "codex", Path: codexPath, AutoFound: true},
		},
	}

	if err := ensureAgents(state); err != nil {
		t.Fatalf("ensureAgents() error = %v", err)
	}

	if len(state.Agents) != 2 {
		t.Fatalf("agents len = %d, want 2: %#v", len(state.Agents), state.Agents)
	}
	if state.Agents[1].Name != "agents" || state.Agents[1].Path != agentsPath {
		t.Fatalf("merged agent = %#v, want agents at %s", state.Agents[1], agentsPath)
	}
}

func TestRunSkillSyncAddLocalNonInteractiveReturnsErrorForAmbiguousSources(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	codexPath := filepath.Join(t.TempDir(), "codex")
	claudePath := filepath.Join(t.TempDir(), "claude")
	writeSkillFile(t, codexPath, "demo", "SKILL.md", "CODEX")
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
		Skills: map[string]skill.SyncSkillEntry{},
	}
	if err := skill.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}

	err = runSkillSyncAddLocal([]string{"demo"}, addOptions{nonInteract: true})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "1 skill(s) failed") ||
		!strings.Contains(err.Error(), "use --from") {
		t.Fatalf("error = %q, want batch failure with --from hint", err.Error())
	}
}
