package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

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

func TestSkillSyncRemoveHasAllFlag(t *testing.T) {
	if skillSyncRemoveCmd.Flags().Lookup("all") == nil {
		t.Fatal("skill-sync remove should expose --all")
	}
}

func TestSkillSyncAddHasAllFlag(t *testing.T) {
	if skillSyncAddCmd.Flags().Lookup("all") == nil {
		t.Fatal("skill-sync add should expose --all")
	}
}

func TestValidateSkillSyncAddArgs(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("all", false, "")

	if err := validateSkillSyncAddArgs(cmd, nil); err == nil {
		t.Fatal("add without skill names or --all should fail")
	}
	if err := validateSkillSyncAddArgs(cmd, []string{"demo"}); err != nil {
		t.Fatalf("add with skill name should pass: %v", err)
	}
	if err := cmd.Flags().Set("all", "true"); err != nil {
		t.Fatal(err)
	}
	if err := validateSkillSyncAddArgs(cmd, nil); err != nil {
		t.Fatalf("add --all should pass: %v", err)
	}
	if err := validateSkillSyncAddArgs(cmd, []string{"demo"}); err == nil {
		t.Fatal("add --all with skill names should fail")
	}
}

func TestValidateSkillSyncRemoveArgs(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("all", false, "")

	if err := validateSkillSyncRemoveArgs(cmd, nil); err == nil {
		t.Fatal("remove without skill names or --all should fail")
	}
	if err := validateSkillSyncRemoveArgs(cmd, []string{"demo"}); err != nil {
		t.Fatalf("remove with skill name should pass: %v", err)
	}
	if err := cmd.Flags().Set("all", "true"); err != nil {
		t.Fatal(err)
	}
	if err := validateSkillSyncRemoveArgs(cmd, nil); err != nil {
		t.Fatalf("remove --all should pass: %v", err)
	}
	if err := validateSkillSyncRemoveArgs(cmd, []string{"demo"}); err == nil {
		t.Fatal("remove --all with skill names should fail")
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
