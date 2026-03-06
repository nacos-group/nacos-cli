package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/nov11/nacos-cli/internal/client"
	"github.com/nov11/nacos-cli/internal/config"
	"github.com/nov11/nacos-cli/internal/terminal"
	"github.com/spf13/cobra"
)

var (
	serverAddr string
	host       string
	port       int
	namespace  string
	authType   string
	username   string
	password   string
	accessKey  string
	secretKey  string
	configFile string
)

var rootCmd = &cobra.Command{
	Use:   "nacos-cli",
	Short: "Nacos CLI - A command-line tool for managing Nacos configurations and skills",
	Long: `Nacos CLI is a powerful command-line tool for interacting with Nacos.
It supports configuration management, skill management, and provides an interactive terminal.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Load configuration from file if specified
		var fileConfig *config.Config
		if configFile != "" {
			cfg, err := config.LoadConfig(configFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to load config file: %v\n", err)
			} else {
				fileConfig = cfg
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
					// Use default port 8848
					serverAddr = fmt.Sprintf("%s:8848", host)
				}
			} else if port > 0 {
				// Only port specified, use default host
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

		// AuthType: command line > config file > default nacos
		if authType == "" {
			if fileConfig != nil && fileConfig.AuthType != "" {
				authType = fileConfig.AuthType
			} else {
				authType = "nacos"
			}
		}

		// Username: command line > config file > default
		if username == "" {
			if fileConfig != nil && fileConfig.Username != "" {
				username = fileConfig.Username
			} else {
				username = "nacos"
			}
		}

		// Password: command line > config file > default
		if password == "" {
			if fileConfig != nil && fileConfig.Password != "" {
				password = fileConfig.Password
			} else {
				password = "nacos"
			}
		}

		// AccessKey / SecretKey: command line > config file（AuthType=aliyun 时使用）
		if accessKey == "" && fileConfig != nil {
			accessKey = fileConfig.AccessKey
		}
		if secretKey == "" && fileConfig != nil {
			secretKey = fileConfig.SecretKey
		}

		// Set default server address if still empty
		if serverAddr == "" {
			serverAddr = "127.0.0.1:8848"
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior: start interactive terminal
		nacosClient := client.NewNacosClient(serverAddr, namespace, authType, username, password, accessKey, secretKey)
		term := terminal.NewTerminal(nacosClient)
		if err := term.Start(); err != nil {
			checkError(err)
		}
	},
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags - new style
	rootCmd.PersistentFlags().StringVar(&host, "host", "", "Nacos server host (e.g., 127.0.0.1)")
	rootCmd.PersistentFlags().IntVar(&port, "port", 0, "Nacos server port (e.g., 8848)")
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Path to configuration file")

	// Global flags - legacy style (for backward compatibility)
	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "", "Nacos server address (e.g., 127.0.0.1:8848)")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Namespace ID")
	rootCmd.PersistentFlags().StringVar(&authType, "auth-type", "", "Auth type: nacos (username/password) or aliyun (AK/SK)")
	rootCmd.PersistentFlags().StringVarP(&username, "username", "u", "", "Username (nacos auth)")
	rootCmd.PersistentFlags().StringVarP(&password, "password", "p", "", "Password (nacos auth)")
	rootCmd.PersistentFlags().StringVar(&accessKey, "access-key", "", "AccessKey (aliyun auth)")
	rootCmd.PersistentFlags().StringVar(&secretKey, "secret-key", "", "SecretKey (aliyun auth)")

	// Mark legacy server flag as deprecated but still functional
	rootCmd.PersistentFlags().MarkDeprecated("server", "use --host and --port instead")
}

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
