package skill

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/config"
)

// ModeOverride represents a per-invocation override of the persisted mode.
type ModeOverride int

const (
	// ModeOverrideNone means no override; use the persisted state mode.
	ModeOverrideNone ModeOverride = iota
	// ModeOverrideLocal forces local mode for this invocation.
	ModeOverrideLocal
	// ModeOverrideNacos forces nacos mode (with explicit profile) for this invocation.
	ModeOverrideNacos
)

// ResolveModeOptions controls how the sync mode is resolved.
type ResolveModeOptions struct {
	// Override is the temporary override, if any.
	Override ModeOverride
	// ProfileHint is the profile name supplied via --profile (only meaningful with ModeOverrideNacos).
	ProfileHint string
	// Interactive controls whether the user may be prompted on the first run.
	Interactive bool
	// Out is where prompts and informational messages go (defaults to os.Stdout).
	Out *os.File
	// In is where the user's input is read from (defaults to os.Stdin).
	In *os.File
}

// ResolveModeResult is the outcome of mode resolution.
type ResolveModeResult struct {
	Mode    SyncMode
	Profile string
	// Persisted reports whether the mode was just written to state during this call.
	Persisted bool
}

// ResolveSyncMode determines the active sync mode for an upcoming command.
// It honors temporary overrides, then the persisted state mode. On first use
// (mode unset), it inspects the available profile and may interactively ask
// the user which mode to lock in.
func ResolveSyncMode(state *SyncState, opts ResolveModeOptions) (ResolveModeResult, error) {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.In == nil {
		opts.In = os.Stdin
	}

	// Honor overrides first
	switch opts.Override {
	case ModeOverrideLocal:
		return ResolveModeResult{Mode: SyncModeLocal}, nil
	case ModeOverrideNacos:
		return ResolveModeResult{Mode: SyncModeNacos, Profile: opts.ProfileHint}, nil
	}

	// Use persisted mode if set
	if state.Mode != SyncModeUnset {
		return ResolveModeResult{Mode: state.Mode, Profile: state.Profile}, nil
	}

	// First run: detect a usable profile to suggest Nacos mode
	usableProfile, profileOK := DetectUsableProfile()

	if !profileOK {
		// No profile -> automatically local mode
		state.Mode = SyncModeLocal
		if err := SaveSyncState(state); err != nil {
			return ResolveModeResult{}, err
		}
		return ResolveModeResult{Mode: SyncModeLocal, Persisted: true}, nil
	}

	if !opts.Interactive {
		// Non-interactive: default to local mode and let the user opt-in later.
		state.Mode = SyncModeLocal
		if err := SaveSyncState(state); err != nil {
			return ResolveModeResult{}, err
		}
		return ResolveModeResult{Mode: SyncModeLocal, Persisted: true}, nil
	}

	// Ask the user
	fmt.Fprintf(opts.Out, "Detected configured profile: %s\n", usableProfile)
	fmt.Fprintf(opts.Out, "Use Nacos mode? [Y/n]: ")
	reader := bufio.NewReader(opts.In)
	answer, _ := reader.ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))

	if answer == "" || answer == "y" || answer == "yes" {
		state.Mode = SyncModeNacos
		state.Profile = usableProfile
	} else {
		state.Mode = SyncModeLocal
	}
	if err := SaveSyncState(state); err != nil {
		return ResolveModeResult{}, err
	}
	return ResolveModeResult{Mode: state.Mode, Profile: state.Profile, Persisted: true}, nil
}

// DetectUsableProfile returns the current profile name if it exists and has
// the required fields filled in.
func DetectUsableProfile() (string, bool) {
	profile, err := config.GetCurrentProfile()
	if err != nil || profile == "" {
		profile = config.DefaultProfile
	}
	exists, err := config.ProfileExists(profile)
	if err != nil || !exists {
		return "", false
	}
	cfgPath, err := config.GetProfileConfigPath(profile)
	if err != nil {
		return "", false
	}
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		return "", false
	}
	if !cfg.IsComplete() {
		return "", false
	}
	return profile, true
}

// SetMode persists an explicit mode change requested by the user via
// `skill-sync mode local|nacos`.
func SetMode(state *SyncState, mode SyncMode, profile string) error {
	switch mode {
	case SyncModeLocal:
		state.Mode = SyncModeLocal
		state.Profile = ""
	case SyncModeNacos:
		if profile == "" {
			detected, ok := DetectUsableProfile()
			if !ok {
				return fmt.Errorf("no usable profile found; configure one via 'profile edit' first")
			}
			profile = detected
		} else {
			exists, err := config.ProfileExists(profile)
			if err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("profile %q does not exist", profile)
			}
		}
		state.Mode = SyncModeNacos
		state.Profile = profile
	default:
		return fmt.Errorf("invalid mode %q (must be local or nacos)", mode)
	}
	return SaveSyncState(state)
}
