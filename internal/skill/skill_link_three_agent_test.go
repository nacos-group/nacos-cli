package skill

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveAgentConflict_UseAgent_ThreeAgents reproduces a scenario reported
// during manual testing: 3 agents each hold a different version of `pdf`.
// After the user picks one agent's version as the new source, ALL three agents
// should end up with symlinks pointing at the central repo (including the
// previously conflicting ones).
func TestResolveAgentConflict_UseAgent_ThreeAgents(t *testing.T) {
	_, repo := withRepoHome(t)
	// Initial repo content (different from any agent).
	writeSkillUnder(t, repo, "pdf", "INITIAL REPO VERSION")

	// 3 agent dirs each with a real `pdf` dir of different content.
	codex := t.TempDir()
	claude := t.TempDir()
	qoder := t.TempDir()

	createRealSkill := func(root, content string) {
		dir := filepath.Join(root, "pdf")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	createRealSkill(codex, "CODEX VERSION (chosen as new source)")
	createRealSkill(claude, "CLAUDE VERSION (conflict)")
	createRealSkill(qoder, "QODER VERSION (conflict)")

	state := &SyncState{
		Version: SyncStateVersion,
		Mode:    SyncModeLocal,
		Agents: []AgentDir{
			{Name: "codex", Path: codex},
			{Name: "claude", Path: claude},
			{Name: "qoder", Path: qoder},
		},
		Skills: map[string]SyncSkillEntry{
			"pdf": {
				Name:           "pdf",
				Status:         SyncStatusConflict,
				ConflictAgents: []string{"codex", "claude", "qoder"},
			},
		},
	}

	if err := ResolveAgentConflictUseAgent(state, "pdf", "codex"); err != nil {
		t.Fatalf("ResolveAgentConflictUseAgent error: %v", err)
	}

	// repo content should now be codex's version.
	body, _ := os.ReadFile(filepath.Join(repo, "pdf", "SKILL.md"))
	if !strings.Contains(string(body), "CODEX VERSION") {
		t.Errorf("repo content = %q, want CODEX VERSION", body)
	}

	// All three agent dirs should now be symlinks pointing at repo/pdf.
	for _, root := range []string{codex, claude, qoder} {
		path := filepath.Join(root, "pdf")
		info, err := os.Lstat(path)
		if err != nil {
			t.Errorf("%s: lstat error: %v", root, err)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s: expected symlink, got mode %v", root, info.Mode())
			continue
		}
		// Verify the symlink actually resolves to the new repo content.
		linkBody, err := os.ReadFile(filepath.Join(path, "SKILL.md"))
		if err != nil {
			t.Errorf("%s: read through symlink failed: %v", root, err)
			continue
		}
		if !strings.Contains(string(linkBody), "CODEX VERSION") {
			t.Errorf("%s: symlink content = %q, want CODEX VERSION", root, linkBody)
		}
	}

	// Also: subsequent LinkSkillSafe should report no conflicts now.
	_, conflicts, err := LinkSkillSafe(repo, "pdf", state.Agents, io.Discard)
	if err != nil {
		t.Fatalf("LinkSkillSafe post-resolve error: %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts after resolve, got %v", conflicts)
	}
}

// TestResolveAgentConflict_UseAgent_MixedSymlinkAndConflict simulates a more
// realistic case: some agents already have a symlink to the (old) repo, others
// hold a divergent real directory. After UseAgent, all agents should reflect
// the new content.
func TestResolveAgentConflict_UseAgent_MixedSymlinkAndConflict(t *testing.T) {
	_, repo := withRepoHome(t)
	writeSkillUnder(t, repo, "pdf", "OLD REPO")

	codex := t.TempDir()
	claude := t.TempDir()
	qoder := t.TempDir()

	// codex is already linked to the (old) repo.
	if err := LinkSkillToAgent(repo, "pdf", codex); err != nil {
		t.Fatal(err)
	}

	// claude has a conflicting real dir.
	writeSkillUnder(t, claude, "pdf", "CLAUDE V1")

	// qoder will be picked as source.
	writeSkillUnder(t, qoder, "pdf", "QODER V1 (NEW SOURCE)")

	state := &SyncState{
		Version: SyncStateVersion,
		Mode:    SyncModeLocal,
		Agents: []AgentDir{
			{Name: "codex", Path: codex},
			{Name: "claude", Path: claude},
			{Name: "qoder", Path: qoder},
		},
		Skills: map[string]SyncSkillEntry{
			"pdf": {
				Name:           "pdf",
				Status:         SyncStatusConflict,
				ConflictAgents: []string{"claude", "qoder"},
			},
		},
	}

	if err := ResolveAgentConflictUseAgent(state, "pdf", "qoder"); err != nil {
		t.Fatalf("ResolveAgentConflictUseAgent error: %v", err)
	}

	// Every agent should now read the new content through whatever path exists.
	for _, root := range []string{codex, claude, qoder} {
		body, err := os.ReadFile(filepath.Join(root, "pdf", "SKILL.md"))
		if err != nil {
			t.Errorf("%s: read failed: %v", root, err)
			continue
		}
		if !strings.Contains(string(body), "QODER V1") {
			t.Errorf("%s: content = %q, want QODER V1", root, body)
		}
		// Each should be a symlink (or whatever the result is), but at minimum
		// it should point at the central repo. Check via Lstat: not a real
		// dir owned by the agent anymore.
		info, err := os.Lstat(filepath.Join(root, "pdf"))
		if err != nil {
			t.Errorf("%s: lstat failed: %v", root, err)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s: expected symlink after resolve, got mode %v", root, info.Mode())
		}
	}
}
