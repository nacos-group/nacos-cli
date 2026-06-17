package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncStateAndRepoAreProfileScoped(t *testing.T) {
	withSkillTempHome(t)
	SetCurrentSyncProfile("profileA")
	t.Cleanup(func() { SetCurrentSyncProfile("") })

	repoA, err := EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	stateA, err := LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	stateA.Mode = SyncModeNacos
	stateA.AddSkill("pdf", "latest", "1.0.0", "md5-a", "hash-a")
	if err := SaveSyncState(stateA); err != nil {
		t.Fatal(err)
	}

	SetCurrentSyncProfile("profileB")
	repoB, err := EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	if repoA == repoB {
		t.Fatalf("profile repos should differ: %s", repoA)
	}
	stateB, err := LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if len(stateB.Skills) != 0 {
		t.Fatalf("profileB should start with no skills, got %v", stateB.Skills)
	}

	SetCurrentSyncProfile("profileA")
	loadedA, err := LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loadedA.Skills["pdf"]; !ok {
		t.Fatal("profileA skill should be preserved")
	}
}

func TestSyncAgentsAreSharedAcrossProfiles(t *testing.T) {
	withSkillTempHome(t)
	SetCurrentSyncProfile("profileA")
	t.Cleanup(func() { SetCurrentSyncProfile("") })

	stateA, err := LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	stateA.Agents = []AgentDir{{Name: "codex", Path: filepath.Join(t.TempDir(), "codex")}}
	if err := SaveSyncState(stateA); err != nil {
		t.Fatal(err)
	}

	SetCurrentSyncProfile("profileB")
	stateB, err := LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if len(stateB.Agents) != 1 || stateB.Agents[0].Name != "codex" {
		t.Fatalf("agents should be shared across profiles, got %+v", stateB.Agents)
	}
}

func withSkillTempHome(t *testing.T) {
	t.Helper()
	oldHome := os.Getenv("HOME")
	home := t.TempDir()
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
	})
}
