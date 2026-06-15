package cmd

import "testing"

func TestSkillSyncUsesAddRemoveCommandNames(t *testing.T) {
	add := findSkillSyncCommand("add")
	if add == nil {
		t.Fatal("skill-sync should expose add command")
	}
	if add.Hidden {
		t.Fatal("add command should be visible")
	}

	remove := findSkillSyncCommand("remove")
	if remove == nil {
		t.Fatal("skill-sync should expose remove command")
	}
	if remove.Hidden {
		t.Fatal("remove command should be visible")
	}
}

func TestSkillSyncDoesNotRegisterLegacyCommandNames(t *testing.T) {
	for _, name := range []string{"track", "subscribe", "untrack", "unsubscribe"} {
		cmd := findSkillSyncCommand(name)
		if cmd != nil {
			t.Fatalf("legacy command %q should not be registered", name)
		}
	}
}

func findSkillSyncCommand(name string) *cobraCommandView {
	for _, cmd := range skillSyncCmd.Commands() {
		if cmd.Name() == name {
			return &cobraCommandView{Name: cmd.Name(), Hidden: cmd.Hidden}
		}
	}
	return nil
}

type cobraCommandView struct {
	Name   string
	Hidden bool
}
