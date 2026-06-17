package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var skillSyncSwitchProfile bool

func registerSkillSyncProfileFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVar(&skillSyncSwitchProfile, "switch-profile", false, "Detach the active profile and switch skill-sync to this profile")
}

func ensureSkillSyncProfileReady(cmd *cobra.Command) error {
	current := skill.CurrentSyncProfile()
	active, err := skill.LoadActiveSyncProfile()
	if err != nil {
		return err
	}
	if active == "" {
		return skill.SaveActiveSyncProfile(current)
	}
	if active == current {
		return nil
	}

	activeState, err := skill.LoadSyncStateForProfile(active)
	if err != nil {
		return fmt.Errorf("load active profile %q: %w", active, err)
	}
	if len(activeState.Skills) == 0 {
		return skill.SaveActiveSyncProfile(current)
	}

	if !skillSyncSwitchProfile {
		if commandNonInteractive(cmd) || !stdinIsTerminal() {
			return fmt.Errorf("skill-sync is active for profile %q, but this command uses profile %q; use --switch-profile to detach %q skills and switch", active, current, active)
		}
		switchProfile, err := promptSwitchSkillSyncProfile(active, current, os.Stdin, os.Stdout)
		if err != nil {
			return err
		}
		if !switchProfile {
			return fmt.Errorf("profile switch cancelled; run with --profile %s to keep using the active profile", active)
		}
	}

	return switchSkillSyncProfile(active, current, activeState, os.Stdout)
}

func commandNonInteractive(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if flag := cmd.Flags().Lookup("non-interactive"); flag != nil {
		value, _ := cmd.Flags().GetBool("non-interactive")
		return value
	}
	return false
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func promptSwitchSkillSyncProfile(active, current string, in io.Reader, out io.Writer) (bool, error) {
	fmt.Fprintf(out, "skill-sync is active for profile %q.\n", active)
	fmt.Fprintf(out, "Current command uses profile %q.\n\n", current)
	fmt.Fprintf(out, "Switch skill-sync to %s?\n", current)
	fmt.Fprintf(out, "  [1] Switch safely\n")
	fmt.Fprintf(out, "  [2] Keep using %s\n", active)
	fmt.Fprintf(out, "  [3] Exit\n")
	fmt.Fprintf(out, "Choice [3]: ")

	answer, _ := bufio.NewReader(in).ReadString('\n')
	answer = strings.TrimSpace(answer)
	switch answer {
	case "1":
		return true, nil
	case "2", "3", "":
		return false, nil
	default:
		return false, fmt.Errorf("invalid choice %q", answer)
	}
}

func switchSkillSyncProfile(active, current string, activeState *skill.SyncState, out io.Writer) error {
	if err := stopSyncDaemonForProfileSwitch(out); err != nil {
		return err
	}

	repoPath, err := skill.EnsureSkillRepoForProfile(active)
	if err != nil {
		return fmt.Errorf("ensure active profile repo: %w", err)
	}
	activeState.Repo = repoPath
	activeState.Profile = active

	names := activeState.GetSubscribedSkillNames()
	sort.Strings(names)
	if len(names) > 0 {
		fmt.Fprintf(out, "Detaching %d skill(s) from profile %q...\n", len(names), active)
	}
	for _, name := range names {
		fmt.Fprintf(out, "Detaching %s...\n", name)
		if err := skill.DetachSkillFromAllAgents(repoPath, name, activeState.Agents, out); err != nil {
			return fmt.Errorf("detach %s: %w", name, err)
		}
	}
	if err := skill.SaveSyncStateForProfile(active, activeState); err != nil {
		return fmt.Errorf("save active profile state: %w", err)
	}
	if err := skill.SaveActiveSyncProfile(current); err != nil {
		return err
	}
	fmt.Fprintf(out, "Switched skill-sync from profile %q to %q.\n", active, current)
	return nil
}

func stopSyncDaemonForProfileSwitch(out io.Writer) error {
	pid, err := skill.LoadSyncDaemonPID()
	if err != nil {
		return fmt.Errorf("load daemon PID: %w", err)
	}
	if pid == 0 {
		return nil
	}
	if !skill.IsSyncDaemonProcess(pid) {
		_ = skill.ClearSyncDaemonPID(pid)
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find daemon process %d: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("stop daemon %d: %w", pid, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !skill.IsProcessRunning(pid) {
			_ = skill.ClearSyncDaemonPID(pid)
			fmt.Fprintf(out, "Stopped sync daemon (pid: %d).\n", pid)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Fprintf(out, "Stop signal sent to sync daemon (pid: %d).\n", pid)
	return nil
}

func skillSyncProfileMismatch() (active, current string, mismatch bool) {
	active, err := skill.LoadActiveSyncProfile()
	if err != nil || active == "" {
		return "", skill.CurrentSyncProfile(), false
	}
	current = skill.CurrentSyncProfile()
	return active, current, active != current
}
