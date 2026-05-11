package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/client"
	"github.com/nacos-group/nacos-cli/internal/config"
	"github.com/nacos-group/nacos-cli/internal/terminal"
	"github.com/spf13/cobra"
)

var (
	serverAddr    string
	host          string
	port          int
	namespace     string
	authType      string
	username      string
	password      string
	accessKey     string
	secretKey     string
	securityToken string
	stsURL        string
	stsAuthToken  string
	configFile    string
	profileName   string // Profile name for config file (default, dev, prod, etc.)
	verbose       bool   // Enable verbose/debug output
)

var rootCmd = &cobra.Command{
	Use:   "nacos-cli",
	Short: "Nacos CLI - A command-line tool for managing Nacos configurations and skills",
	Long: `Nacos CLI is a powerful command-line tool for interacting with Nacos.
It supports configuration management, skill management, and provides an interactive terminal.

Examples:
  nacos-cli                 # Use default config (~/.nacos-cli/default.conf)
  nacos-cli --profile dev   # Use dev config (~/.nacos-cli/dev.conf)
  nacos-cli --profile prod  # Use prod config (~/.nacos-cli/prod.conf)
  nacos-cli profile edit    # Edit default config
  nacos-cli profile edit dev   # Edit dev config
  nacos-cli profile show    # Show default config
  nacos-cli profile show dev   # Show dev config`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Skip config loading for help, completion, and profile subcommands
		skipCommands := map[string]bool{
			"help": true, "completion": true,
			"profile": true, "edit": true, "show": true,
		}
		if skipCommands[cmd.Name()] {
			return
		}

		// Determine config loading strategy
		// Priority: --config > env arg > default
		var fileConfig *config.Config
		var err error

		// Check if any connection parameters are provided via command line
		hasCommandLineConfig := host != "" || port > 0 || serverAddr != "" || username != "" || password != "" || accessKey != "" || secretKey != "" || securityToken != "" || authType == "sts-hiclaw"

		if configFile != "" {
			// Explicit config file specified
			fileConfig, err = config.LoadConfig(configFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to load config file: %v\n", err)
			}
		} else if !hasCommandLineConfig {
			// No command line config provided, use profile-based config
			envName := config.DefaultProfile
			if profileName != "" {
				envName = profileName
			}
			// This will load, prompt for missing fields, and save
			fileConfig, _, err = config.LoadOrCreateConfig(envName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Failed to load or create config: %v\n", err)
				os.Exit(1)
			}
		}

		// Apply configuration with priority: command line > config file > default
		// Server address: --server has highest priority
		if serverAddr == "" {
			// Try to build from --host and --port
			if host != "" {
				if port > 0 {
					serverAddr = fmt.Sprintf("%s:%d", host, port)
				} else if strings.Contains(host, ":") {
					// Host already contains port
					serverAddr = host
				} else {
					// When only host is specified, use the standard Nacos port.
					serverAddr = fmt.Sprintf("%s:8848", host)
				}
			} else if port > 0 {
				// Only port specified, use the local default host.
				serverAddr = fmt.Sprintf("127.0.0.1:%d", port)
			} else if fileConfig != nil {
				// Use from config file
				serverAddr = fileConfig.GetServerAddr()
			}
		}

		// Namespace: command line > config file > default (empty)
		if namespace == "" && fileConfig != nil && fileConfig.Namespace != "" {
			namespace = fileConfig.Namespace
		}

		// AuthType: command line > config file > env var > auto-detect by NewNacosClient
		if authType == "" && fileConfig != nil && fileConfig.AuthType != "" {
			authType = fileConfig.AuthType
		}
		if authType == "" {
			if envAuthType := os.Getenv("NACOS_AUTH_TYPE"); envAuthType != "" {
				authType = envAuthType
			}
		}

		// Username: command line > config file
		if username == "" && fileConfig != nil && fileConfig.Username != "" {
			username = fileConfig.Username
		}

		// Password: command line > config file
		if password == "" && fileConfig != nil && fileConfig.Password != "" {
			password = fileConfig.Password
		}

		// AccessKey / SecretKey / SecurityToken: command line > config file
		if accessKey == "" && fileConfig != nil {
			accessKey = fileConfig.AccessKey
		}
		if secretKey == "" && fileConfig != nil {
			secretKey = fileConfig.SecretKey
		}
		if securityToken == "" && fileConfig != nil {
			securityToken = fileConfig.SecurityToken
		}

		// Set default server address only when neither --host nor --port is provided.
		if serverAddr == "" {
			serverAddr = "market.hiclaw.io:80"
		}

		// For sts-hiclaw auth, read HICLAW_CONTROLLER_URL and HICLAW_AUTH_TOKEN_FILE from environment variables
		if authType == "sts-hiclaw" {
			if stsURL == "" {
				controllerURL := os.Getenv("HICLAW_CONTROLLER_URL")
				if controllerURL != "" {
					stsURL = strings.TrimRight(controllerURL, "/") + "/api/v1/credentials/sts"
				}
			}
			if stsAuthToken == "" {
				tokenFile := os.Getenv("HICLAW_AUTH_TOKEN_FILE")
				if tokenFile != "" {
					data, err := os.ReadFile(tokenFile)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error: failed to read HICLAW_AUTH_TOKEN_FILE (%s): %v\n", tokenFile, err)
						os.Exit(1)
					}
					stsAuthToken = strings.TrimSpace(string(data))
				}
			}
			if stsURL == "" || stsAuthToken == "" {
				fmt.Fprintf(os.Stderr, "Error: sts-hiclaw auth requires HICLAW_CONTROLLER_URL and HICLAW_AUTH_TOKEN_FILE environment variables\n")
				os.Exit(1)
			}
		} else if authType == "" && os.Getenv("HICLAW_CONTROLLER_URL") != "" && os.Getenv("HICLAW_AUTH_TOKEN_FILE") != "" {
			// Auto-detect sts-hiclaw auth from environment variables
			authType = "sts-hiclaw"
			controllerURL := os.Getenv("HICLAW_CONTROLLER_URL")
			stsURL = strings.TrimRight(controllerURL, "/") + "/api/v1/credentials/sts"
			tokenFile := os.Getenv("HICLAW_AUTH_TOKEN_FILE")
			data, err := os.ReadFile(tokenFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to read HICLAW_AUTH_TOKEN_FILE (%s): %v\n", tokenFile, err)
				os.Exit(1)
			}
			stsAuthToken = strings.TrimSpace(string(data))
		}

		if verbose {
			fmt.Fprintf(os.Stderr, "[debug] authType=%s\n", authType)
			fmt.Fprintf(os.Stderr, "[debug] serverAddr=%s\n", serverAddr)
			fmt.Fprintf(os.Stderr, "[debug] namespace=%s\n", namespace)
			if stsURL != "" {
				fmt.Fprintf(os.Stderr, "[debug] stsURL=%s\n", stsURL)
			}
			if stsAuthToken != "" {
				masked := stsAuthToken
				if len(masked) > 10 {
					masked = masked[:10] + "..."
				}
				fmt.Fprintf(os.Stderr, "[debug] stsAuthToken=%s\n", masked)
			}
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior: start interactive terminal
		nacosClient := mustNewNacosClient()
		term := terminal.NewTerminal(nacosClient)
		if err := term.Start(); err != nil {
			checkError(err)
		}
	},
}

// SetVersionInfo sets the version information for the root command.
// Called from main.go with values injected via ldflags.
func SetVersionInfo(version, commit, date string) {
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date)
	rootCmd.InitDefaultVersionFlag()
	if flag := rootCmd.Flags().Lookup("version"); flag != nil {
		flag.Shorthand = "v"
	}
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags - new style
	rootCmd.PersistentFlags().StringVar(&host, "host", "", "Nacos server host (default: market.hiclaw.io)")
	rootCmd.PersistentFlags().IntVar(&port, "port", 0, "Nacos server port (default: 8848 when used with --host)")
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Path to configuration file")
	rootCmd.PersistentFlags().StringVar(&profileName, "profile", "", "Profile name (e.g., dev, prod). Loads ~/.nacos-cli/<profile>.conf")

	// Global flags - legacy style (for backward compatibility)
	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "", "Nacos server address (e.g., market.hiclaw.io:80)")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Namespace ID")
	rootCmd.PersistentFlags().StringVar(&authType, "auth-type", "", "Auth type: nacos | aliyun | sts-hiclaw")
	rootCmd.PersistentFlags().StringVarP(&username, "username", "u", "", "Username (nacos auth)")
	rootCmd.PersistentFlags().StringVarP(&password, "password", "p", "", "Password (nacos auth)")
	rootCmd.PersistentFlags().StringVar(&accessKey, "access-key", "", "AccessKey (aliyun/sts-hiclaw auth)")
	rootCmd.PersistentFlags().StringVar(&secretKey, "secret-key", "", "SecretKey (aliyun/sts-hiclaw auth)")
	rootCmd.PersistentFlags().StringVar(&securityToken, "security-token", "", "STS SecurityToken (sts-hiclaw auth)")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose/debug output")

	// Mark legacy server flag as deprecated but still functional
	rootCmd.PersistentFlags().MarkDeprecated("server", "use --host and --port instead")
}

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// mustNewNacosClient creates a NacosClient and exits with a clear error message on failure (e.g. login failed).
func mustNewNacosClient() *client.NacosClient {
	c, err := client.NewNacosClient(serverAddr, namespace, authType, username, password, accessKey, secretKey, securityToken, stsURL, stsAuthToken, func(c *client.NacosClient) {
		c.Verbose = verbose
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return c
}
