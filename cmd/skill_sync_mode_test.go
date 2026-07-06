package cmd

import (
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/config"
	"github.com/nacos-group/nacos-cli/internal/skill"
)

func TestRunSkillSyncModeLocalWorksWithoutProfileConfig(t *testing.T) {
	withTempHome(t)

	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Profile: "default",
		Label:   "latest",
		Config:  skill.SyncConfig{AutoUpload: true},
		Skills:  map[string]skill.SyncSkillEntry{},
	}
	if err := skill.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := runSkillSyncMode("local"); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "Skill sync mode: local") {
		t.Fatalf("output missing local confirmation:\n%s", output)
	}

	loaded, err := skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Mode != skill.SyncModeLocal {
		t.Fatalf("mode = %q, want local", loaded.Mode)
	}
	if loaded.Profile != "default" {
		t.Fatalf("profile = %q, want default", loaded.Profile)
	}
}

func TestRunSkillSyncModeRejectsInvalidMode(t *testing.T) {
	withTempHome(t)

	err := runSkillSyncMode("remote")
	if err == nil {
		t.Fatal("expected invalid mode error")
	}
	if !strings.Contains(err.Error(), "must be local or nacos") {
		t.Fatalf("error = %q, want mode hint", err.Error())
	}
}

func TestRunSkillSyncModeNacosRemindsUserToStart(t *testing.T) {
	withTempHome(t)
	origProfileName := profileName
	profileName = "team"
	t.Cleanup(func() {
		profileName = origProfileName
	})

	configPath, err := config.GetProfileConfigPath("team")
	if err != nil {
		t.Fatal(err)
	}
	if err := (&config.Config{Host: "127.0.0.1", AuthType: "none"}).SaveConfig(configPath); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := runSkillSyncMode("nacos"); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{
		"Skill sync mode: nacos (profile: team)",
		"nacos-cli skill-sync start --profile team",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}
