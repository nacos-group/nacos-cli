package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/skill"
)

func TestAddSingleNacosTreatsVersionOnlyLocalSourceAsSame(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}

	agentPath := t.TempDir()
	localSkill := `---
name: demo
description: test skill
---
# Demo
`
	remoteSkill := `---
version: 0.0.1
name: demo
description: test skill
---
# Demo
`
	writeSkillFile(t, agentPath, "demo", "SKILL.md", localSkill)

	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Label:   "latest",
		Agents: []skill.AgentDir{
			{Name: "agents", Path: agentPath},
		},
		Skills: map[string]skill.SyncSkillEntry{},
	}

	server := newSkillQueryServer(t, "", "remote-md5", "0.0.1", map[string]string{
		"demo/SKILL.md": remoteSkill,
	})
	defer server.Close()
	svc := newSkillServiceForTest(t, server.URL)

	var addErr error
	output := captureStdout(t, func() {
		addErr = addSingleNacos(state, repoPath, "demo", svc, addOptions{nonInteract: true})
	})
	if addErr != nil {
		t.Fatal(addErr)
	}

	if strings.Contains(output, "backed up -> linked") {
		t.Fatalf("version-only difference should not be backed up as conflict, output:\n%s", output)
	}
	if !strings.Contains(output, "linked (replaced, same content)") {
		t.Fatalf("output should report same-content relink, got:\n%s", output)
	}

	entry := state.Skills["demo"]
	if entry.Status != skill.SyncStatusSynced {
		t.Fatalf("status = %s, want Synced", entry.Status)
	}

	skillPath := filepath.Join(agentPath, "demo")
	info, err := os.Lstat(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s should be a symlink, got %v", skillPath, info.Mode())
	}
	assertFileContent(t, filepath.Join(skillPath, "SKILL.md"), remoteSkill)
}

func TestSelectUnmanagedNacosSkillNamesSkipsManagedAndSorts(t *testing.T) {
	state := &skill.SyncState{
		Skills: map[string]skill.SyncSkillEntry{
			"beta": {Name: "beta"},
		},
	}

	names := selectUnmanagedNacosSkillNames(state, []skill.SkillListItem{
		{Name: "beta"},
		{Name: "gamma"},
		{Name: "alpha"},
		{Name: "gamma"},
		{Name: " "},
	})

	want := []string{"alpha", "gamma"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("names = %v, want %v", names, want)
	}
}
