package skill

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// helper: write a SKILL.md under <root>/<name>/ with given content
func writeSkillUnder(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// withRepoHome configures HOME -> tmp and returns the repo path with the given
// skill installed.
func withRepoHome(t *testing.T) (home, repo string) {
	t.Helper()
	tmp := t.TempDir()
	os.Setenv("HOME", tmp)
	t.Cleanup(func() { os.Unsetenv("HOME") })
	repoPath, err := EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	return tmp, repoPath
}

func TestLinkSkillSafe_NewAgentLinks(t *testing.T) {
	_, repo := withRepoHome(t)
	writeSkillUnder(t, repo, "demo", "repo content")

	agentRoot := t.TempDir()
	agents := []AgentDir{{Name: "codex", Path: agentRoot}}

	res, conflicts, err := LinkSkillSafe(repo, "demo", agents, io.Discard)
	if err != nil {
		t.Fatalf("LinkSkillSafe error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %v", conflicts)
	}
	if len(res.Agents) != 1 || res.Agents[0].Skipped {
		t.Fatalf("expected agent linked, got %+v", res.Agents)
	}
	// The agent dir should now contain a symlink to repo/demo
	target, err := os.Readlink(filepath.Join(agentRoot, "demo"))
	if err != nil {
		t.Fatalf("expected symlink, got %v", err)
	}
	if target == "" {
		t.Fatal("symlink target empty")
	}
}

func TestLinkSkillSafe_ConflictKeepsLocal(t *testing.T) {
	_, repo := withRepoHome(t)
	writeSkillUnder(t, repo, "demo", "repo content")

	// Create a conflicting real dir in agent
	agentRoot := t.TempDir()
	conflictDir := filepath.Join(agentRoot, "demo")
	if err := os.MkdirAll(conflictDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conflictDir, "SKILL.md"), []byte("AGENT VERSION"), 0644); err != nil {
		t.Fatal(err)
	}

	agents := []AgentDir{{Name: "codex", Path: agentRoot}}
	_, conflicts, err := LinkSkillSafe(repo, "demo", agents, io.Discard)
	if err != nil {
		t.Fatalf("LinkSkillSafe error: %v", err)
	}
	if len(conflicts) != 1 || conflicts[0] != "codex" {
		t.Fatalf("expected conflicts=[codex], got %v", conflicts)
	}
	// Agent's content should NOT be modified
	body, _ := os.ReadFile(filepath.Join(agentRoot, "demo", "SKILL.md"))
	if string(body) != "AGENT VERSION" {
		t.Fatalf("agent content was overwritten: %q", body)
	}
}

func TestDetachSkillFromAllAgentsCopiesLinkedSkill(t *testing.T) {
	_, repo := withRepoHome(t)
	writeSkillUnder(t, repo, "demo", "repo content")

	agentRoot := t.TempDir()
	if err := LinkSkillToAgent(repo, "demo", agentRoot); err != nil {
		t.Fatal(err)
	}

	if err := DetachSkillFromAllAgents(repo, "demo", []AgentDir{{Name: "codex", Path: agentRoot}}, io.Discard); err != nil {
		t.Fatal(err)
	}

	agentSkill := filepath.Join(agentRoot, "demo")
	info, err := os.Lstat(agentSkill)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("%s should be a real directory after detach", agentSkill)
	}
	body, err := os.ReadFile(filepath.Join(agentSkill, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "repo content" {
		t.Fatalf("detached content = %q, want repo content", body)
	}

	if err := os.WriteFile(filepath.Join(repo, "demo", "SKILL.md"), []byte("changed repo"), 0644); err != nil {
		t.Fatal(err)
	}
	body, err = os.ReadFile(filepath.Join(agentSkill, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "repo content" {
		t.Fatalf("agent copy should not follow repo after detach, got %q", body)
	}
}

func TestDetachSkillFromAllAgentsKeepsExistingLocalConflict(t *testing.T) {
	_, repo := withRepoHome(t)
	writeSkillUnder(t, repo, "demo", "repo content")

	agentRoot := t.TempDir()
	writeSkillUnder(t, agentRoot, "demo", "agent content")

	if err := DetachSkillFromAllAgents(repo, "demo", []AgentDir{{Name: "codex", Path: agentRoot}}, io.Discard); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(agentRoot, "demo", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "agent content" {
		t.Fatalf("agent content = %q, want original local content", body)
	}
}

func TestResolveAgentConflict_UseRepo(t *testing.T) {
	_, repo := withRepoHome(t)
	writeSkillUnder(t, repo, "demo", "REPO VERSION")

	agentRoot := t.TempDir()
	writeSkillUnder(t, agentRoot, "demo", "AGENT VERSION")

	state := &SyncState{
		Version: SyncStateVersion,
		Mode:    SyncModeLocal,
		Agents:  []AgentDir{{Name: "codex", Path: agentRoot}},
		Skills: map[string]SyncSkillEntry{
			"demo": {
				Name:           "demo",
				Status:         SyncStatusConflict,
				ConflictAgents: []string{"codex"},
			},
		},
	}

	if err := ResolveAgentConflictUseRepo(state, "demo"); err != nil {
		t.Fatalf("ResolveAgentConflictUseRepo error: %v", err)
	}

	// Status should be cleared
	if state.Skills["demo"].Status != SyncStatusLinked {
		t.Errorf("status = %s, want Linked", state.Skills["demo"].Status)
	}
	if len(state.Skills["demo"].ConflictAgents) != 0 {
		t.Errorf("ConflictAgents = %v, want empty", state.Skills["demo"].ConflictAgents)
	}

	// Agent dir should now be a symlink with repo content
	target, err := os.Readlink(filepath.Join(agentRoot, "demo"))
	if err != nil {
		t.Fatalf("expected agent dir to be symlink: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(target, "SKILL.md"))
	if string(body) != "REPO VERSION" {
		t.Errorf("symlink content = %q, want %q", body, "REPO VERSION")
	}
}

func TestResolveAgentConflict_UseAgent(t *testing.T) {
	_, repo := withRepoHome(t)
	writeSkillUnder(t, repo, "demo", "REPO VERSION")

	agentRoot := t.TempDir()
	writeSkillUnder(t, agentRoot, "demo", "AGENT VERSION")

	state := &SyncState{
		Version: SyncStateVersion,
		Mode:    SyncModeLocal,
		Agents:  []AgentDir{{Name: "codex", Path: agentRoot}},
		Skills: map[string]SyncSkillEntry{
			"demo": {
				Name:           "demo",
				Status:         SyncStatusConflict,
				ConflictAgents: []string{"codex"},
			},
		},
	}

	if err := ResolveAgentConflictUseAgent(state, "demo", "codex"); err != nil {
		t.Fatalf("ResolveAgentConflictUseAgent error: %v", err)
	}

	// Repo should now have the agent's content
	body, _ := os.ReadFile(filepath.Join(repo, "demo", "SKILL.md"))
	if string(body) != "AGENT VERSION" {
		t.Errorf("repo content = %q, want %q after agent override", body, "AGENT VERSION")
	}

	// Conflict should be cleared
	if len(state.Skills["demo"].ConflictAgents) != 0 {
		t.Errorf("ConflictAgents = %v, want empty", state.Skills["demo"].ConflictAgents)
	}
}
