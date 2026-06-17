package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

func TestEnsureSkillSyncProfileReadySwitchesAndDetachesActiveProfile(t *testing.T) {
	withTempHome(t)
	skill.SetCurrentSyncProfile("profileA")
	t.Cleanup(func() {
		skill.SetCurrentSyncProfile("")
		skillSyncSwitchProfile = false
	})

	repoA, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoA, "pdf", "SKILL.md", "profileA pdf")

	agentPath := t.TempDir()
	if err := skill.LinkSkillToAgent(repoA, "pdf", agentPath); err != nil {
		t.Fatal(err)
	}
	stateA := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Profile: "profileA",
		Repo:    repoA,
		Label:   "latest",
		Agents: []skill.AgentDir{
			{Name: "codex", Path: agentPath},
		},
		Skills: map[string]skill.SyncSkillEntry{
			"pdf": {
				Name:   "pdf",
				Label:  "latest",
				Status: skill.SyncStatusSynced,
			},
		},
	}
	if err := skill.SaveSyncState(stateA); err != nil {
		t.Fatal(err)
	}
	if err := skill.SaveActiveSyncProfile("profileA"); err != nil {
		t.Fatal(err)
	}

	skill.SetCurrentSyncProfile("profileB")
	skillSyncSwitchProfile = true
	output := captureStdout(t, func() {
		err = ensureSkillSyncProfileReady(&cobra.Command{})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Switched skill-sync from profile \"profileA\" to \"profileB\"") {
		t.Fatalf("switch output missing confirmation:\n%s", output)
	}

	active, err := skill.LoadActiveSyncProfile()
	if err != nil {
		t.Fatal(err)
	}
	if active != "profileB" {
		t.Fatalf("active profile = %q, want profileB", active)
	}

	agentSkill := filepath.Join(agentPath, "pdf")
	info, err := os.Lstat(agentSkill)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("%s should be detached to a real directory", agentSkill)
	}
	assertFileContent(t, filepath.Join(agentSkill, "SKILL.md"), "profileA pdf")

	loadedA, err := skill.LoadSyncStateForProfile("profileA")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loadedA.Skills["pdf"]; !ok {
		t.Fatal("profileA subscription should be preserved after switching away")
	}
}

func TestEnsureSkillSyncProfileReadyNonInteractiveRequiresSwitchFlag(t *testing.T) {
	withTempHome(t)
	skill.SetCurrentSyncProfile("profileA")
	t.Cleanup(func() {
		skill.SetCurrentSyncProfile("")
		skillSyncSwitchProfile = false
	})

	stateA := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Profile: "profileA",
		Label:   "latest",
		Skills: map[string]skill.SyncSkillEntry{
			"pdf": {Name: "pdf", Label: "latest", Status: skill.SyncStatusSynced},
		},
	}
	if err := skill.SaveSyncState(stateA); err != nil {
		t.Fatal(err)
	}
	if err := skill.SaveActiveSyncProfile("profileA"); err != nil {
		t.Fatal(err)
	}

	skill.SetCurrentSyncProfile("profileB")
	cmd := &cobra.Command{}
	cmd.Flags().Bool("non-interactive", true, "")
	err := ensureSkillSyncProfileReady(cmd)
	if err == nil {
		t.Fatal("expected profile mismatch error")
	}
	if !strings.Contains(err.Error(), "--switch-profile") {
		t.Fatalf("error should mention --switch-profile, got %v", err)
	}
}
