package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/skill"
)

func TestPrintSyncStatusSummaryIncludesSkillTable(t *testing.T) {
	withTempHome(t)

	primary := filepath.Join(t.TempDir(), "primary")
	secondary := filepath.Join(t.TempDir(), "secondary")
	writeSkillFile(t, primary, "demo", "SKILL.md", "content")
	writeSkillFile(t, secondary, "demo", "SKILL.md", "content")

	localHash, err := skill.ComputeDirectoryHash(filepath.Join(primary, "demo"))
	if err != nil {
		t.Fatal(err)
	}

	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Profile: "default",
		Label:   "latest",
		Agents: []skill.AgentDir{
			{Name: "codex", Path: primary},
			{Name: "claude", Path: secondary},
		},
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:            "demo",
				Label:           "latest",
				ResolvedVersion: "0.0.4",
				RemoteMd5:       "b4fce67c123456",
				LocalHash:       localHash,
				SyncedHash:      localHash,
				Status:          skill.SyncStatusSynced,
				UpdatedAt:       "2026-06-05T05:56:31Z",
			},
		},
	}

	output := captureStdout(t, func() {
		printSyncStatusSummary(state)
	})

	for _, want := range []string{
		"Mode: nacos",
		"Profile: default",
		"Tracking label: latest",
		"SKILL  STATUS  VERSION  AGENTS        NEXT",
		"demo   Synced  0.0.4    codex,claude  -",
		"Total: 1 skills",
		"Agents: [codex claude]",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}

	if strings.Contains(output, "Subscriptions:") {
		t.Fatalf("output should not include Subscriptions line:\n%s", output)
	}
}

func TestNextActionUploadBlockedShowsDraftVersion(t *testing.T) {
	state := &skill.SyncState{
		Mode:   skill.SyncModeNacos,
		Config: skill.SyncConfig{AutoUpload: true},
	}
	entry := skill.SyncSkillEntry{
		Name:                "demo",
		Status:              skill.SyncStatusUploadBlocked,
		BlockedDraftVersion: "0.0.2",
	}

	got := nextAction("demo", entry, state)
	want := "Nacos draft 0.0.2 exists; review/clear it, auto-upload will retry"
	if got != want {
		t.Fatalf("next action = %q, want %q", got, want)
	}
}

func TestPrintSyncStatusSummaryDetectsLocalAgentDrift(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	codex := filepath.Join(t.TempDir(), "codex")
	claude := filepath.Join(t.TempDir(), "claude")
	writeSkillFile(t, repoPath, "demo", "SKILL.md", "codex content")

	hash, err := skill.ComputeDirectoryHash(filepath.Join(repoPath, "demo"))
	if err != nil {
		t.Fatal(err)
	}

	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeLocal,
		Repo:    repoPath,
		Label:   "latest",
		Agents: []skill.AgentDir{
			{Name: "codex", Path: codex},
			{Name: "claude", Path: claude},
		},
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:       "demo",
				LocalHash:  hash,
				SyncedHash: hash,
				Status:     skill.SyncStatusLinked,
			},
		},
	}
	if _, err := skill.LinkSkillForce(repoPath, "demo", state.Agents, io.Discard); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(claude, "demo")); err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, claude, "demo", "SKILL.md", "claude drift")

	output := captureStdout(t, func() {
		printSyncStatusSummary(state)
	})

	for _, want := range []string{
		"demo   Conflict  codex,claude≠  skill-sync resolve demo",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}

	entry := state.Skills["demo"]
	if entry.Status != skill.SyncStatusConflict {
		t.Fatalf("status = %s, want Conflict", entry.Status)
	}
	if got, want := strings.Join(entry.ConflictAgents, ","), "claude"; got != want {
		t.Fatalf("conflict agents = %q, want %q", got, want)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	origStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer

	fn()

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = origStdout
	t.Cleanup(func() {
		_ = reader.Close()
	})

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
