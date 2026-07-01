package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/client"
	"github.com/nacos-group/nacos-cli/internal/config"
	"github.com/nacos-group/nacos-cli/internal/terminal"
	"github.com/spf13/cobra"
)

var (
	profileListOutput     string
	profileDeleteForce    bool
	profileSetIncomplete  bool
	profileKnownFieldKeys = map[string]bool{
		"host":          true,
		"port":          true,
		"server":        true,
		"authtype":      true,
		"username":      true,
		"password":      true,
		"accesskey":     true,
		"secretkey":     true,
		"securitytoken": true,
		"namespace":     true,
	}
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage configuration profiles",
	Long: `Manage configuration profiles for different environments.

Examples:
  nacos-cli profile edit           # Edit default config
  nacos-cli profile edit dev       # Edit dev config
  nacos-cli profile show           # Show default config
  nacos-cli profile show dev       # Show dev config
  nacos-cli profile list           # List profiles
  nacos-cli profile switch dev     # Switch current profile
  nacos-cli profile set dev host=127.0.0.1 port=8848 auth-type=none
  nacos-cli profile get dev server
  nacos-cli profile delete dev`,
}

var profileEditCmd = &cobra.Command{
	Use:   "edit [profile]",
	Short: "Edit a configuration profile",
	Long: `Interactively edit a configuration profile.

Examples:
  nacos-cli profile edit           # Edit current config
  nacos-cli profile edit dev       # Edit dev config
  nacos-cli profile edit prod      # Edit prod config`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		profileName := mustResolveProfileArg(args)

		// Get config path
		configPath, err := config.GetProfileConfigPath(profileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Try to load existing config
		var cfg *config.Config
		if _, err := os.Stat(configPath); err == nil {
			cfg, err = config.LoadConfig(configPath)
			if err != nil {
				if strings.Contains(err.Error(), "failed to decrypt") {
					fmt.Fprintf(os.Stderr, "Error: Failed to load existing config: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("Warning: Failed to load existing config: %v\n", err)
				cfg = &config.Config{}
			}
		} else {
			cfg = &config.Config{}
		}

		// Show current config and prompt for updates
		fmt.Printf("Editing configuration for profile '%s'\n", profileName)
		fmt.Printf("Config file: %s\n", configPath)
		fmt.Println()

		if err := cfg.PromptForUpdate(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Save the updated config
		if err := cfg.SaveConfig(configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to save config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("\nConfiguration saved to %s\n", configPath)

		// Ask user if they want to login
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("\nLogin now? [Y/n] (Enter=Yes): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		input = strings.TrimSpace(strings.ToLower(input))

		// Default to yes (empty input or 'y' or 'yes')
		if input == "" || input == "y" || input == "yes" {
			fmt.Println()
			// Start interactive terminal with the edited config
			var stsURLVal, stsAuthTokenVal string
			if client.IsStsAuthType(cfg.AuthType) {
				var stsErr error
				stsURLVal, stsAuthTokenVal, stsErr = loadStsAuthFromEnv(cfg.AuthType)
				if stsErr != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", stsErr)
					os.Exit(1)
				}
			}
			nacosClient, err := client.NewNacosClient(
				cfg.GetServerAddr(),
				cfg.Namespace,
				cfg.AuthType,
				cfg.Username,
				cfg.Password,
				cfg.AccessKey,
				cfg.SecretKey,
				cfg.SecurityToken,
				stsURLVal,
				stsAuthTokenVal,
				cfg.GetScheme(),
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			term := terminal.NewTerminalWithProfile(nacosClient, profileName)
			if err := term.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Printf("\nTo use this profile, run: nacos-cli --profile %s\n", profileName)
		}
	},
}

var profileShowCmd = &cobra.Command{
	Use:   "show [profile]",
	Short: "Show a configuration profile",
	Long: `Display the current configuration for a profile.

Examples:
  nacos-cli profile show           # Show current config
  nacos-cli profile show dev       # Show dev config
  nacos-cli profile show prod      # Show prod config`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		profileName := mustResolveProfileArg(args)

		// Get config path
		configPath, err := config.GetProfileConfigPath(profileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Check if config exists
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			fmt.Printf("Profile '%s' does not exist.\n", profileName)
			fmt.Printf("Config file: %s\n", configPath)
			fmt.Println("\nRun 'nacos-cli profile edit " + profileName + "' to create it.")
			return
		}

		// Load config
		cfg, err := config.LoadConfig(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to load config: %v\n", err)
			os.Exit(1)
		}

		// Display config
		fmt.Printf("Profile: %s\n", profileName)
		fmt.Printf("Config file: %s\n", configPath)
		fmt.Println("─────────────────────────────────────────")
		fmt.Printf("%-15s %s\n", "host:", cfg.Host)
		fmt.Printf("%-15s %d\n", "port:", cfg.Port)
		fmt.Printf("%-15s %s\n", "auth-type:", cfg.AuthType)
		switch cfg.AuthType {
		case "aliyun":
			fmt.Printf("%-15s %s\n", "access-key:", displayConfigValue(cfg.AccessKey))
			fmt.Printf("%-15s %s\n", "secret-key:", maskSensitiveValue(cfg.SecretKey))
		case "sts-hiclaw", "sts-agentteams":
			controllerEnv, tokenFileEnv := stsAuthEnvNames(cfg.AuthType)
			fmt.Printf("%-15s from %s and %s env vars\n", "credentials:", controllerEnv, tokenFileEnv)
		default:
			fmt.Printf("%-15s %s\n", "username:", displayConfigValue(cfg.Username))
			fmt.Printf("%-15s %s\n", "password:", maskSensitiveValue(cfg.Password))
		}
		if cfg.Namespace != "" {
			fmt.Printf("%-15s %s\n", "namespace:", cfg.Namespace)
		} else {
			fmt.Printf("%-15s %s\n", "namespace:", "(public)")
		}
	},
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configuration profiles",
	Long: `List all configuration profiles under ~/.nacos-cli.

Examples:
  nacos-cli profile list
  nacos-cli profile list --output json`,
	Run: func(cmd *cobra.Command, args []string) {
		profiles, err := config.ListProfiles()
		checkError(err)
		currentProfile, err := config.GetCurrentProfile()
		checkError(err)

		items := make([]profileListItem, 0, len(profiles))
		for _, profile := range profiles {
			items = append(items, inspectProfile(profile, currentProfile))
		}

		switch strings.ToLower(profileListOutput) {
		case "", "pretty":
			renderProfileListPretty(items)
		case "json":
			renderProfileListJSON(items)
		default:
			fmt.Fprintf(os.Stderr, "Error: unsupported --output value %q (expect 'pretty' or 'json')\n", profileListOutput)
			os.Exit(1)
		}
	},
}

var profileSwitchCmd = &cobra.Command{
	Use:   "switch <profile>",
	Short: "Switch the current configuration profile",
	Long: `Switch the current configuration profile used when --profile is omitted.

Examples:
  nacos-cli profile switch dev
  nacos-cli profile switch default`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		profileName := config.NormalizeProfileName(args[0])
		exists, err := config.ProfileExists(profileName)
		checkError(err)
		if !exists {
			fmt.Fprintf(os.Stderr, "Error: profile %q does not exist\n", profileName)
			os.Exit(1)
		}
		configPath, err := config.GetProfileConfigPath(profileName)
		checkError(err)
		cfg, err := config.LoadConfig(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load profile %q: %v\n", profileName, err)
			os.Exit(1)
		}
		if !cfg.IsComplete() {
			fmt.Fprintf(os.Stderr, "Error: profile %q is incomplete (missing: %s)\n", profileName, strings.Join(cfg.GetMissingFields(), ", "))
			fmt.Fprintf(os.Stderr, "Run 'nacos-cli profile edit %s' to complete it.\n", profileName)
			os.Exit(1)
		}
		checkError(config.SetCurrentProfile(profileName))
		fmt.Printf("Current profile switched to %q\n", profileName)
	},
}

var profileDeleteCmd = &cobra.Command{
	Use:   "delete <profile>",
	Short: "Delete a configuration profile",
	Long: `Delete a configuration profile file.

The shared encryption key is not deleted.

Examples:
  nacos-cli profile delete dev
  nacos-cli profile delete dev --force`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		profileName := config.NormalizeProfileName(args[0])
		currentProfile, err := config.GetCurrentProfile()
		checkError(err)
		if profileName == currentProfile && !profileDeleteForce {
			fmt.Fprintf(os.Stderr, "Error: cannot delete current profile %q; switch to another profile first or use --force\n", profileName)
			os.Exit(1)
		}

		if err := config.DeleteProfile(profileName); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if profileName == currentProfile {
			checkError(config.ClearCurrentProfile())
		}
		fmt.Printf("Deleted profile %q\n", profileName)
	},
}

var profileGetCmd = &cobra.Command{
	Use:   "get [profile] [key]",
	Short: "Get configuration profile values",
	Long: `Get one or all values from a configuration profile.

Sensitive values are masked by default.

Examples:
  nacos-cli profile get
  nacos-cli profile get dev
  nacos-cli profile get dev server
  nacos-cli profile get auth-type`,
	Args: cobra.MaximumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		profileName, key := mustResolveProfileGetArgs(args)
		cfg := mustLoadProfile(profileName)
		if key != "" {
			value, sensitive, err := cfg.GetValue(key)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			if sensitive {
				value = maskSensitiveValue(value)
			}
			fmt.Println(value)
			return
		}
		printProfileValues(cfg)
	},
}

var profileSetCmd = &cobra.Command{
	Use:   "set [profile] key=value [key=value...]",
	Short: "Set configuration profile values",
	Long: `Set one or more values in a configuration profile without prompts.

The profile is created if it does not exist. Sensitive values are encrypted
before being saved.

Examples:
  nacos-cli profile set dev host=127.0.0.1 port=8848 auth-type=none
  nacos-cli profile set dev auth-type=nacos username=nacos password=nacos
  nacos-cli profile set dev server=127.0.0.1:8848 namespace=public`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		profileName, pairs := mustResolveProfileSetArgs(args)
		cfg := &config.Config{}
		configPath, err := config.GetProfileConfigPath(profileName)
		checkError(err)
		if exists, err := config.ProfileExists(profileName); err != nil {
			checkError(err)
		} else if exists {
			cfg, err = config.LoadConfig(configPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to load profile %q: %v\n", profileName, err)
				os.Exit(1)
			}
		}

		for _, pair := range pairs {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
				fmt.Fprintf(os.Stderr, "Error: invalid key=value pair %q\n", pair)
				os.Exit(1)
			}
			if err := cfg.SetValue(parts[0], parts[1]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}

		if !profileSetIncomplete && !cfg.IsComplete() {
			fmt.Fprintf(os.Stderr, "Error: profile %q is incomplete (missing: %s)\n", profileName, strings.Join(cfg.GetMissingFields(), ", "))
			fmt.Fprintf(os.Stderr, "Use --allow-incomplete to save a partial profile.\n")
			os.Exit(1)
		}
		if err := cfg.SaveConfig(configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to save profile %q: %v\n", profileName, err)
			os.Exit(1)
		}
		fmt.Printf("Updated profile %q\n", profileName)
	},
}

func displayConfigValue(value string) string {
	if value == "" {
		return "(not set)"
	}
	return value
}

// maskSensitiveValue masks a sensitive config value for display.
func maskSensitiveValue(value string) string {
	if value == "" {
		return "(not set)"
	}
	return "******"
}

type profileListItem struct {
	Current   bool   `json:"current"`
	Name      string `json:"name"`
	AuthType  string `json:"authType"`
	Server    string `json:"server"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

func mustResolveProfileArg(args []string) string {
	if len(args) > 0 {
		return config.NormalizeProfileName(args[0])
	}
	profileName, err := config.GetCurrentProfile()
	checkError(err)
	return profileName
}

func mustResolveProfileGetArgs(args []string) (string, string) {
	switch len(args) {
	case 0:
		return mustResolveProfileArg(nil), ""
	case 1:
		if isProfileFieldKey(args[0]) {
			return mustResolveProfileArg(nil), args[0]
		}
		exists, err := config.ProfileExists(args[0])
		checkError(err)
		if exists {
			return config.NormalizeProfileName(args[0]), ""
		}
		return mustResolveProfileArg(nil), args[0]
	default:
		return config.NormalizeProfileName(args[0]), args[1]
	}
}

func mustResolveProfileSetArgs(args []string) (string, []string) {
	if strings.Contains(args[0], "=") {
		return mustResolveProfileArg(nil), args
	}
	if len(args) == 1 {
		fmt.Fprintf(os.Stderr, "Error: at least one key=value pair is required\n")
		os.Exit(1)
	}
	return config.NormalizeProfileName(args[0]), args[1:]
}

func mustLoadProfile(profileName string) *config.Config {
	configPath, err := config.GetProfileConfigPath(profileName)
	checkError(err)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: profile %q does not exist\n", profileName)
		os.Exit(1)
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load profile %q: %v\n", profileName, err)
		os.Exit(1)
	}
	return cfg
}

func isProfileFieldKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, "_", "")
	return profileKnownFieldKeys[normalized]
}

func inspectProfile(profileName, currentProfile string) profileListItem {
	item := profileListItem{
		Current: profileName == currentProfile,
		Name:    profileName,
		Status:  "ok",
	}
	configPath, err := config.GetProfileConfigPath(profileName)
	if err != nil {
		item.Status = "invalid"
		item.Error = err.Error()
		return item
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if strings.Contains(err.Error(), "failed to parse") {
			item.Status = "invalid"
		} else {
			item.Status = "unreadable"
		}
		item.Error = err.Error()
		return item
	}
	item.AuthType = cfg.AuthType
	if item.AuthType == "" {
		item.AuthType = "none"
	}
	item.Server = cfg.GetServerAddr()
	if item.Server == "" {
		item.Server = "(not set)"
	}
	item.Namespace = cfg.Namespace
	if item.Namespace == "" {
		item.Namespace = "public"
	}
	if !cfg.IsComplete() {
		item.Status = "incomplete"
	}
	return item
}

func renderProfileListPretty(items []profileListItem) {
	if len(items) == 0 {
		fmt.Println("No profiles found")
		return
	}
	fmt.Printf("%-7s %-18s %-12s %-24s %-14s %s\n", "CURRENT", "NAME", "AUTH", "SERVER", "NAMESPACE", "STATUS")
	for _, item := range items {
		current := ""
		if item.Current {
			current = "*"
		}
		status := item.Status
		if item.Error != "" {
			status = status + ": " + item.Error
		}
		fmt.Printf("%-7s %-18s %-12s %-24s %-14s %s\n",
			current, item.Name, item.AuthType, item.Server, item.Namespace, status)
	}
}

func renderProfileListJSON(items []profileListItem) {
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to encode JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func printProfileValues(cfg *config.Config) {
	for _, key := range []string{
		"host",
		"port",
		"server",
		"auth-type",
		"username",
		"password",
		"access-key",
		"secret-key",
		"security-token",
		"namespace",
	} {
		value, sensitive, err := cfg.GetValue(key)
		if err != nil {
			continue
		}
		if sensitive {
			value = maskSensitiveValue(value)
		}
		fmt.Printf("%s=%s\n", key, value)
	}
}

func init() {
	profileCmd.AddCommand(profileEditCmd)
	profileCmd.AddCommand(profileShowCmd)
	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileSwitchCmd)
	profileCmd.AddCommand(profileDeleteCmd)
	profileCmd.AddCommand(profileGetCmd)
	profileCmd.AddCommand(profileSetCmd)
	profileListCmd.Flags().StringVar(&profileListOutput, "output", "pretty", "Output format: pretty | json")
	profileDeleteCmd.Flags().BoolVar(&profileDeleteForce, "force", false, "Delete the current profile and reset current profile state")
	profileSetCmd.Flags().BoolVar(&profileSetIncomplete, "allow-incomplete", false, "Save even if required fields are missing")
	rootCmd.AddCommand(profileCmd)
}
