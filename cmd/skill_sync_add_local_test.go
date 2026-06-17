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

func TestRunSkillSyncAddLocalFromAgentLinksAllAgents(t *testing.T) {
	withTempHome(t)
	home := os.Getenv("HOME")

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	codexPath := filepath.Join(home, ".codex", "skills")
	claudePath := filepath.Join(home, ".claude", "skills")
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

	if err := runSkillSyncAddLocal([]string{"demo"}, addOptions{fromAgent: "codex", nonInteract: true}); err != nil {
		t.Fatal(err)
	}

	assertFileContent(t, filepath.Join(repoPath, "demo", "SKILL.md"), "CODEX")
	assertSkillSymlink(t, codexPath, "demo")
	assertSkillSymlink(t, claudePath, "demo")
	assertFileContent(t, filepath.Join(claudePath, "demo", "SKILL.md"), "CODEX")

	state, err = skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	entry := state.Skills["demo"]
	if entry.Status != skill.SyncStatusLinked {
		t.Fatalf("status = %s, want linked", entry.Status)
	}
	if len(entry.ConflictAgents) != 0 {
		t.Fatalf("conflict agents = %v, want none", entry.ConflictAgents)
	}
}

func TestRunSkillSyncAddLocalAllLinksRepoSkills(t *testing.T) {
	withTempHome(t)
	home := os.Getenv("HOME")

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "alpha", "SKILL.md", "ALPHA")
	writeSkillFile(t, repoPath, "beta", "SKILL.md", "BETA")

	agentPath := filepath.Join(home, ".codex", "skills")
	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeLocal,
		Label:   "latest",
		Repo:    repoPath,
		Agents: []skill.AgentDir{
			{Name: "codex", Path: agentPath},
		},
		Skills: map[string]skill.SyncSkillEntry{},
	}
	if err := skill.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}

	if err := runSkillSyncAddLocal(nil, addOptions{all: true, nonInteract: true}); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"alpha", "beta"} {
		assertSkillSymlink(t, agentPath, name)
		assertFileContent(t, filepath.Join(agentPath, name, "SKILL.md"), strings.ToUpper(name))
	}

	state, err = skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "beta"} {
		entry := state.Skills[name]
		if entry.Status != skill.SyncStatusLinked {
			t.Fatalf("%s status = %s, want linked", name, entry.Status)
		}
	}
}
