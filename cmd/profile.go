package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/client"
	"github.com/nacos-group/nacos-cli/internal/config"
	"github.com/nacos-group/nacos-cli/internal/terminal"
	"github.com/spf13/cobra"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage configuration profiles",
	Long: `Manage configuration profiles for different environments.

Examples:
  nacos-cli profile edit           # Edit default config
  nacos-cli profile edit dev       # Edit dev config
  nacos-cli profile show           # Show default config
  nacos-cli profile show dev       # Show dev config`,
}

var profileEditCmd = &cobra.Command{
	Use:   "edit [profile]",
	Short: "Edit a configuration profile",
	Long: `Interactively edit a configuration profile.

Examples:
  nacos-cli profile edit           # Edit default config
  nacos-cli profile edit dev       # Edit dev config
  nacos-cli profile edit prod      # Edit prod config`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Get profile name from args
		profileName := config.DefaultProfile
		if len(args) > 0 {
			profileName = args[0]
		}

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
			nacosClient, err := client.NewNacosClient(
				cfg.GetServerAddr(),
				cfg.Namespace,
				cfg.AuthType,
				cfg.Username,
				cfg.Password,
				cfg.AccessKey,
				cfg.SecretKey,
				cfg.Token,
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			term := terminal.NewTerminal(nacosClient)
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
  nacos-cli profile show           # Show default config
  nacos-cli profile show dev       # Show dev config
  nacos-cli profile show prod      # Show prod config`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Get profile name from args
		profileName := config.DefaultProfile
		if len(args) > 0 {
			profileName = args[0]
		}

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
		if cfg.AuthType == "aliyun" {
			fmt.Printf("%-15s %s\n", "access-key:", cfg.AccessKey)
			fmt.Printf("%-15s %s\n", "secret-key:", maskPassword(cfg.SecretKey))
		} else {
			fmt.Printf("%-15s %s\n", "username:", cfg.Username)
			fmt.Printf("%-15s %s\n", "password:", maskPassword(cfg.Password))
		}
		if cfg.Namespace != "" {
			fmt.Printf("%-15s %s\n", "namespace:", cfg.Namespace)
		} else {
			fmt.Printf("%-15s %s\n", "namespace:", "(public)")
		}
	},
}

// maskPassword masks a password string for display
func maskPassword(pwd string) string {
	if pwd == "" {
		return "(not set)"
	}
	return "******"
}

func init() {
	profileCmd.AddCommand(profileEditCmd)
	profileCmd.AddCommand(profileShowCmd)
	rootCmd.AddCommand(profileCmd)
}
