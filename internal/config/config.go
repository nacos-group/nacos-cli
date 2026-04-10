package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

const (
	DefaultConfigDir  = ".nacos-cli"
	DefaultProfile    = "default"
	ConfigFileSuffix  = ".conf"
)

// Config represents the Nacos CLI configuration
type Config struct {
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	AuthType  string `yaml:"authType"` // nacos | aliyun | token
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	Token     string `yaml:"token"`     // Pre-issued access token (skips username/password login)
	AccessKey string `yaml:"accessKey"` // Aliyun AK（AuthType=aliyun 时使用）
	SecretKey string `yaml:"secretKey"` // Aliyun SK
	Namespace string `yaml:"namespace"`
}

// LoadConfig loads configuration from a file
func LoadConfig(configPath string) (*Config, error) {
	// Expand home directory if needed
	if configPath == "~" || (len(configPath) > 1 && configPath[:2] == "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		if configPath == "~" {
			configPath = homeDir
		} else {
			configPath = filepath.Join(homeDir, configPath[2:])
		}
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", absPath)
	}

	// Read file
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// GetServerAddr returns the server address in format "host:port"
func (c *Config) GetServerAddr() string {
	if c.Host == "" {
		return ""
	}
	// If port is 0 or not set, check if host already contains port
	if c.Port == 0 {
		// Check if host already contains ":"
		if strings.Contains(c.Host, ":") {
			return c.Host
		}
		// Default to the standard Nacos port when only host is configured.
		return fmt.Sprintf("%s:8848", c.Host)
	}
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// GetConfigDir returns the default config directory path (~/.nacos-cli)
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, DefaultConfigDir), nil
}

// GetProfileConfigPath returns the config file path for a given profile
// e.g., profile="dev" -> ~/.nacos-cli/dev.conf
func GetProfileConfigPath(profile string) (string, error) {
	if profile == "" {
		profile = DefaultProfile
	}
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, profile+ConfigFileSuffix), nil
}

// EnsureConfigDir ensures the config directory exists
func EnsureConfigDir() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(configDir, 0755)
}

// IsComplete checks if the configuration has all required fields for authentication
func (c *Config) IsComplete() bool {
	// Host is always required
	if c.Host == "" {
		return false
	}

	// Token auth: only token is needed
	if c.Token != "" {
		return true
	}

	// Check based on auth type
	authType := strings.ToLower(c.AuthType)

	// No auth: only host is needed
	if authType == "" || authType == "none" {
		return true
	}

	if authType == "aliyun" {
		// Aliyun auth requires AccessKey and SecretKey
		return c.AccessKey != "" && c.SecretKey != ""
	}

	// Nacos auth requires username and password
	return c.Username != "" && c.Password != ""
}

// GetMissingFields returns a list of missing required fields
func (c *Config) GetMissingFields() []string {
	var missing []string

	if c.Host == "" {
		missing = append(missing, "host")
	}

	// Token auth: no other fields required
	if c.Token != "" {
		return missing
	}

	authType := strings.ToLower(c.AuthType)

	// No auth: no credential fields required
	if authType == "" || authType == "none" {
		return missing
	}

	if authType == "aliyun" {
		if c.AccessKey == "" {
			missing = append(missing, "accessKey")
		}
		if c.SecretKey == "" {
			missing = append(missing, "secretKey")
		}
	} else {
		// Nacos auth
		if c.Username == "" {
			missing = append(missing, "username")
		}
		if c.Password == "" {
			missing = append(missing, "password")
		}
	}

	return missing
}

// SaveConfig saves the configuration to a file
func (c *Config) SaveConfig(configPath string) error {
	// Expand home directory if needed
	if configPath == "~" || (len(configPath) > 1 && configPath[:2] == "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		if configPath == "~" {
			configPath = homeDir
		} else {
			configPath = filepath.Join(homeDir, configPath[2:])
		}
	}

	// Ensure parent directory exists
	parentDir := filepath.Dir(configPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file with restricted permissions (0600 for security)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// PromptForMissingFields interactively prompts the user to input missing configuration fields
func (c *Config) PromptForMissingFields() error {
	reader := bufio.NewReader(os.Stdin)

	// Prompt for host if missing
	if c.Host == "" {
		fmt.Print("Enter Nacos host [market.hiclaw.io]: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read host: %w", err)
		}
		input = strings.TrimSpace(input)
		if input == "" {
			c.Host = "market.hiclaw.io"
		} else {
			c.Host = input
		}
	}

	// Prompt for port if not set
	if c.Port == 0 {
		fmt.Print("Enter Nacos port [80]: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read port: %w", err)
		}
		input = strings.TrimSpace(input)
		if input == "" {
			c.Port = 80
		} else {
			port, err := strconv.Atoi(input)
			if err != nil {
				return fmt.Errorf("invalid port number: %w", err)
			}
			c.Port = port
		}
	}

	// Prompt for auth type if not set
	if c.AuthType == "" {
		fmt.Print("Enter auth type (none/nacos/aliyun) [none]: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read auth type: %w", err)
		}
		input = strings.TrimSpace(strings.ToLower(input))
		if input == "" {
			c.AuthType = "none"
		} else if input == "none" || input == "nacos" || input == "aliyun" {
			c.AuthType = input
		} else {
			return fmt.Errorf("invalid auth type: %s (must be 'none', 'nacos' or 'aliyun')", input)
		}
	}

	// Prompt for credentials based on auth type
	if c.AuthType == "aliyun" {
		if c.AccessKey == "" {
			fmt.Print("Enter AccessKey: ")
			input, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read access key: %w", err)
			}
			c.AccessKey = strings.TrimSpace(input)
			if c.AccessKey == "" {
				return fmt.Errorf("access key is required for aliyun auth")
			}
		}
		if c.SecretKey == "" {
			fmt.Print("Enter SecretKey: ")
			c.SecretKey = readPassword(reader)
			if c.SecretKey == "" {
				return fmt.Errorf("secret key is required for aliyun auth")
			}
		}
	} else if c.AuthType == "nacos" {
		// Nacos auth
		if c.Username == "" {
			fmt.Print("Enter username: ")
			input, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read username: %w", err)
			}
			c.Username = strings.TrimSpace(input)
			if c.Username == "" {
				return fmt.Errorf("username is required")
			}
		}
		if c.Password == "" {
			fmt.Print("Enter password: ")
			password := readPassword(reader)
			if password == "" {
				return fmt.Errorf("password is required")
			}
			c.Password = password
		}
	}
	// authType == "none": skip credential prompts

	// Optionally prompt for namespace
	if c.Namespace == "" {
		fmt.Print("Enter namespace (leave empty for public): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read namespace: %w", err)
		}
		c.Namespace = strings.TrimSpace(input)
	}

	return nil
}

// readPassword reads a password from input, using hidden input if running in a TTY
func readPassword(reader *bufio.Reader) string {
	// Check if stdin is a terminal
	if term.IsTerminal(int(os.Stdin.Fd())) {
		bytePassword, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println() // New line after password input
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(bytePassword))
	}

	// Fallback to regular input for non-TTY (e.g., piped input)
	input, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimSpace(input)
}

// LoadOrCreateConfig loads config from profile, prompts for missing fields, and saves
func LoadOrCreateConfig(profile string) (*Config, string, error) {
	configPath, err := GetProfileConfigPath(profile)
	if err != nil {
		return nil, "", err
	}

	var cfg *Config

	// Try to load existing config
	if _, err := os.Stat(configPath); err == nil {
		cfg, err = LoadConfig(configPath)
		if err != nil {
			fmt.Printf("Warning: Failed to load config from %s: %v\n", configPath, err)
			cfg = &Config{}
		}
	} else {
		cfg = &Config{}
	}

	// Check if config is complete
	if !cfg.IsComplete() {
		missing := cfg.GetMissingFields()
		if len(missing) > 0 {
			fmt.Printf("Configuration incomplete (missing: %s)\n", strings.Join(missing, ", "))
		}
		fmt.Printf("Please enter the required configuration for profile '%s':\n", profile)

		// Prompt for missing fields
		if err := cfg.PromptForMissingFields(); err != nil {
			return nil, "", fmt.Errorf("failed to get configuration: %w", err)
		}

		// Save the completed config
		if err := cfg.SaveConfig(configPath); err != nil {
			fmt.Printf("Warning: Failed to save config to %s: %v\n", configPath, err)
		} else {
			fmt.Printf("Configuration saved to %s\n", configPath)
		}
	}

	return cfg, configPath, nil
}

// PromptForUpdate prompts the user to update existing configuration fields
// Shows current values (passwords masked) as defaults
func (c *Config) PromptForUpdate() error {
	reader := bufio.NewReader(os.Stdin)

	// Helper to format current value display
	formatCurrent := func(val string, isMasked bool) string {
		if val == "" {
			return ""
		}
		if isMasked {
			return "******"
		}
		return val
	}

	// Host
	currentHost := formatCurrent(c.Host, false)
	if currentHost != "" {
		fmt.Printf("Enter Nacos host [%s]: ", currentHost)
	} else {
		fmt.Print("Enter Nacos host [market.hiclaw.io]: ")
	}
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read host: %w", err)
	}
	input = strings.TrimSpace(input)
	if input != "" {
		c.Host = input
	} else if c.Host == "" {
		c.Host = "market.hiclaw.io"
	}

	// Port
	currentPort := "80"
	if c.Port > 0 {
		currentPort = strconv.Itoa(c.Port)
	}
	fmt.Printf("Enter Nacos port [%s]: ", currentPort)
	input, err = reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read port: %w", err)
	}
	input = strings.TrimSpace(input)
	if input != "" {
		port, err := strconv.Atoi(input)
		if err != nil {
			return fmt.Errorf("invalid port number: %w", err)
		}
		c.Port = port
	} else if c.Port == 0 {
		c.Port = 80
	}

	// Auth type
	currentAuthType := c.AuthType
	if currentAuthType == "" {
		currentAuthType = "none"
	}
	fmt.Printf("Enter auth type (none/nacos/aliyun) [%s]: ", currentAuthType)
	input, err = reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read auth type: %w", err)
	}
	input = strings.TrimSpace(strings.ToLower(input))
	if input != "" {
		if input != "none" && input != "nacos" && input != "aliyun" {
			return fmt.Errorf("invalid auth type: %s (must be 'none', 'nacos' or 'aliyun')", input)
		}
		c.AuthType = input
	} else if c.AuthType == "" {
		c.AuthType = "none"
	}

	// Credentials based on auth type
	if c.AuthType == "aliyun" {
		// AccessKey
		currentAK := formatCurrent(c.AccessKey, false)
		if currentAK != "" {
			fmt.Printf("Enter AccessKey [%s]: ", currentAK)
		} else {
			fmt.Print("Enter AccessKey: ")
		}
		input, err = reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read access key: %w", err)
		}
		input = strings.TrimSpace(input)
		if input != "" {
			c.AccessKey = input
		}
		if c.AccessKey == "" {
			return fmt.Errorf("access key is required for aliyun auth")
		}

		// SecretKey
		if c.SecretKey != "" {
			fmt.Print("Enter SecretKey [******] (press Enter to keep current): ")
		} else {
			fmt.Print("Enter SecretKey: ")
		}
		newSK := readPassword(reader)
		if newSK != "" {
			c.SecretKey = newSK
		}
		if c.SecretKey == "" {
			return fmt.Errorf("secret key is required for aliyun auth")
		}
	} else if c.AuthType == "nacos" {
		// Nacos auth - Username
		currentUser := formatCurrent(c.Username, false)
		if currentUser != "" {
			fmt.Printf("Enter username [%s]: ", currentUser)
		} else {
			fmt.Print("Enter username: ")
		}
		input, err = reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read username: %w", err)
		}
		input = strings.TrimSpace(input)
		if input != "" {
			c.Username = input
		}
		if c.Username == "" {
			return fmt.Errorf("username is required")
		}

		// Password
		if c.Password != "" {
			fmt.Print("Enter password [******] (press Enter to keep current): ")
		} else {
			fmt.Print("Enter password: ")
		}
		newPwd := readPassword(reader)
		if newPwd != "" {
			c.Password = newPwd
		}
		if c.Password == "" {
			return fmt.Errorf("password is required")
		}
	}
	// authType == "none": skip credential prompts

	// Namespace
	currentNS := c.Namespace
	if currentNS != "" {
		fmt.Printf("Enter namespace [%s]: ", currentNS)
	} else {
		fmt.Print("Enter namespace (leave empty for public): ")
	}
	input, err = reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read namespace: %w", err)
	}
	input = strings.TrimSpace(input)
	if input != "" {
		c.Namespace = input
	}

	return nil
}
