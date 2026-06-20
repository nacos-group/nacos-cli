package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

const (
	DefaultConfigDir = ".nacos-cli"
	DefaultProfile   = "default"
	ConfigFileSuffix = ".conf"
	SettingsFileName = "settings.yaml"
)

// Config represents the Nacos CLI configuration
type Config struct {
	Host          string `yaml:"host"`
	Port          int    `yaml:"port"`
	Scheme        string `yaml:"scheme"`   // http | https (default: http)
	AuthType      string `yaml:"authType"` // nacos | aliyun | sts-hiclaw | sts-agentteams
	Username      string `yaml:"username"`
	Password      string `yaml:"password"`
	AccessKey     string `yaml:"accessKey"`     // Aliyun AK (AuthType=aliyun)
	SecretKey     string `yaml:"secretKey"`     // Aliyun SK (AuthType=aliyun)
	SecurityToken string `yaml:"securityToken"` // STS SecurityToken (legacy)
	Namespace     string `yaml:"namespace"`
}

// Settings stores CLI-wide profile state.
type Settings struct {
	CurrentProfile string `yaml:"currentProfile"`
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
	if err := config.DecryptSensitiveFields(); err != nil {
		return nil, fmt.Errorf("failed to decrypt config file: %w", err)
	}

	return &config, nil
}

// NormalizeProfileName returns the default profile when name is empty.
func NormalizeProfileName(profile string) string {
	if profile == "" {
		return DefaultProfile
	}
	return profile
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

// GetScheme returns the configured scheme, defaulting to "http".
func (c *Config) GetScheme() string {
	if c.Scheme == "" {
		return "http"
	}
	return strings.ToLower(c.Scheme)
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
	profile = NormalizeProfileName(profile)
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, profile+ConfigFileSuffix), nil
}

// GetSettingsPath returns the CLI settings file path.
func GetSettingsPath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, SettingsFileName), nil
}

// EnsureConfigDir ensures the config directory exists
func EnsureConfigDir() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}
	return os.Chmod(configDir, 0700)
}

// LoadSettings loads CLI settings. Missing settings are treated as defaults.
func LoadSettings() (*Settings, error) {
	settingsPath, err := GetSettingsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return &Settings{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read settings file: %w", err)
	}
	var settings Settings
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("failed to parse settings file: %w", err)
	}
	settings.CurrentProfile = NormalizeProfileName(settings.CurrentProfile)
	return &settings, nil
}

// SaveSettings saves CLI settings to disk.
func SaveSettings(settings *Settings) error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}
	settingsPath, err := GetSettingsPath()
	if err != nil {
		return err
	}
	settingsToSave := *settings
	settingsToSave.CurrentProfile = NormalizeProfileName(settingsToSave.CurrentProfile)
	data, err := yaml.Marshal(&settingsToSave)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}
	if err := os.WriteFile(settingsPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write settings file: %w", err)
	}
	return nil
}

// GetCurrentProfile returns the current profile, falling back to default.
func GetCurrentProfile() (string, error) {
	settings, err := LoadSettings()
	if err != nil {
		return "", err
	}
	return NormalizeProfileName(settings.CurrentProfile), nil
}

// SetCurrentProfile updates the current profile setting.
func SetCurrentProfile(profile string) error {
	return SaveSettings(&Settings{CurrentProfile: NormalizeProfileName(profile)})
}

// ClearCurrentProfile removes the settings file if it exists.
func ClearCurrentProfile() error {
	settingsPath, err := GetSettingsPath()
	if err != nil {
		return err
	}
	if err := os.Remove(settingsPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove settings file: %w", err)
	}
	return nil
}

// ListProfiles returns profile names discovered under the config directory.
func ListProfiles() ([]string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(configDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read config directory: %w", err)
	}
	profiles := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ConfigFileSuffix) {
			continue
		}
		profile := strings.TrimSuffix(name, ConfigFileSuffix)
		if profile != "" {
			profiles = append(profiles, profile)
		}
	}
	sort.Strings(profiles)
	return profiles, nil
}

// ProfileExists reports whether a profile config file exists.
func ProfileExists(profile string) (bool, error) {
	configPath, err := GetProfileConfigPath(profile)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(configPath); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}

// DeleteProfile removes a profile config file.
func DeleteProfile(profile string) error {
	configPath, err := GetProfileConfigPath(profile)
	if err != nil {
		return err
	}
	if err := os.Remove(configPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("profile %q does not exist", NormalizeProfileName(profile))
		}
		return fmt.Errorf("failed to delete profile %q: %w", NormalizeProfileName(profile), err)
	}
	return nil
}

// IsComplete checks if the configuration has all required fields for authentication
func (c *Config) IsComplete() bool {
	// Host is always required
	if c.Host == "" {
		return false
	}

	// Check based on auth type
	authType := strings.ToLower(c.AuthType)

	// No auth: only host is needed
	if authType == "" || authType == "none" {
		return true
	}

	if authType == "aliyun" {
		return c.AccessKey != "" && c.SecretKey != ""
	}

	if isStsAuthType(authType) {
		// STS credentials are fetched dynamically from controller env vars.
		return true
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
	} else if isStsAuthType(authType) {
		// STS credentials are fetched dynamically; no config fields required
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
	if err := os.MkdirAll(parentDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configToSave := *c
	if err := configToSave.EncryptSensitiveFields(); err != nil {
		return fmt.Errorf("failed to encrypt config: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(&configToSave)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file with restricted permissions (0600 for security)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// SetValue updates one configuration field by CLI key name.
func (c *Config) SetValue(key, value string) error {
	switch normalizeConfigKey(key) {
	case "host":
		c.Host = value
	case "port":
		if value == "" {
			c.Port = 0
			return nil
		}
		port, err := strconv.Atoi(value)
		if err != nil || port < 0 {
			return fmt.Errorf("invalid port value %q", value)
		}
		c.Port = port
	case "server":
		return c.SetServerAddr(value)
	case "authtype":
		authType, err := NormalizeAuthType(value)
		if err != nil {
			return err
		}
		c.AuthType = authType
	case "username":
		c.Username = value
	case "password":
		c.Password = value
	case "accesskey":
		c.AccessKey = value
	case "secretkey":
		c.SecretKey = value
	case "securitytoken":
		c.SecurityToken = value
	case "namespace":
		c.Namespace = value
	default:
		return fmt.Errorf("unknown profile key %q", key)
	}
	return nil
}

// GetValue returns one configuration field by CLI key name.
func (c *Config) GetValue(key string) (string, bool, error) {
	switch normalizeConfigKey(key) {
	case "host":
		return c.Host, false, nil
	case "port":
		if c.Port == 0 {
			return "", false, nil
		}
		return strconv.Itoa(c.Port), false, nil
	case "server":
		return c.GetServerAddr(), false, nil
	case "authtype":
		return c.AuthType, false, nil
	case "username":
		return c.Username, false, nil
	case "password":
		return c.Password, true, nil
	case "accesskey":
		return c.AccessKey, false, nil
	case "secretkey":
		return c.SecretKey, true, nil
	case "securitytoken":
		return c.SecurityToken, true, nil
	case "namespace":
		return c.Namespace, false, nil
	default:
		return "", false, fmt.Errorf("unknown profile key %q", key)
	}
}

// SetServerAddr updates Host and Port from a host[:port] value.
func (c *Config) SetServerAddr(server string) error {
	server = strings.TrimSpace(server)
	if server == "" {
		c.Host = ""
		c.Port = 0
		return nil
	}
	hostValue := server
	portValue := 0
	if strings.Count(server, ":") == 1 {
		parts := strings.SplitN(server, ":", 2)
		hostValue = parts[0]
		if parts[1] != "" {
			parsedPort, err := strconv.Atoi(parts[1])
			if err != nil || parsedPort < 0 {
				return fmt.Errorf("invalid server port value %q", parts[1])
			}
			portValue = parsedPort
		}
	}
	c.Host = hostValue
	c.Port = portValue
	return nil
}

// NormalizeAuthType validates and normalizes an auth type value.
func NormalizeAuthType(authType string) (string, error) {
	authType = strings.TrimSpace(strings.ToLower(authType))
	if authType == "" {
		return "", nil
	}
	if authType == "sts-url" {
		return "sts-hiclaw", nil
	}
	switch authType {
	case "none", "nacos", "aliyun", "sts-hiclaw", "sts-agentteams":
		return authType, nil
	default:
		return "", fmt.Errorf("invalid auth type: %s (must be 'none', 'nacos', 'aliyun', 'sts-hiclaw' or 'sts-agentteams')", authType)
	}
}

func isStsAuthType(authType string) bool {
	return authType == "sts-url" || authType == "sts-hiclaw" || authType == "sts-agentteams"
}

func normalizeConfigKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	key = strings.ReplaceAll(key, "-", "")
	key = strings.ReplaceAll(key, "_", "")
	return key
}

// EncryptSensitiveFields encrypts credential fields before writing them to disk.
func (c *Config) EncryptSensitiveFields() error {
	var err error
	if c.Username, err = encryptValue(c.Username); err != nil {
		return fmt.Errorf("username: %w", err)
	}
	if c.Password, err = encryptValue(c.Password); err != nil {
		return fmt.Errorf("password: %w", err)
	}
	if c.AccessKey, err = encryptValue(c.AccessKey); err != nil {
		return fmt.Errorf("accessKey: %w", err)
	}
	if c.SecretKey, err = encryptValue(c.SecretKey); err != nil {
		return fmt.Errorf("secretKey: %w", err)
	}
	if c.SecurityToken, err = encryptValue(c.SecurityToken); err != nil {
		return fmt.Errorf("securityToken: %w", err)
	}
	return nil
}

// DecryptSensitiveFields decrypts credential fields after reading them from disk.
func (c *Config) DecryptSensitiveFields() error {
	var err error
	if c.Username, err = decryptValue(c.Username); err != nil {
		return fmt.Errorf("username: %w", err)
	}
	if c.Password, err = decryptValue(c.Password); err != nil {
		return fmt.Errorf("password: %w", err)
	}
	if c.AccessKey, err = decryptValue(c.AccessKey); err != nil {
		return fmt.Errorf("accessKey: %w", err)
	}
	if c.SecretKey, err = decryptValue(c.SecretKey); err != nil {
		return fmt.Errorf("secretKey: %w", err)
	}
	if c.SecurityToken, err = decryptValue(c.SecurityToken); err != nil {
		return fmt.Errorf("securityToken: %w", err)
	}
	return nil
}

func (c *Config) hasPlaintextSensitiveFields() bool {
	for _, value := range []string{
		c.Username,
		c.Password,
		c.AccessKey,
		c.SecretKey,
		c.SecurityToken,
	} {
		if value != "" && !isEncryptedValue(value) {
			return true
		}
	}
	return false
}

func configFileHasPlaintextSensitiveFields(configPath string) (bool, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false, err
	}
	var raw Config
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false, err
	}
	return raw.hasPlaintextSensitiveFields(), nil
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
		fmt.Print("Enter auth type (none/nacos/aliyun/sts-hiclaw/sts-agentteams) [none]: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read auth type: %w", err)
		}
		input = strings.TrimSpace(strings.ToLower(input))
		if input == "" {
			c.AuthType = "none"
		} else if input == "none" || input == "nacos" || input == "aliyun" || input == "sts-hiclaw" || input == "sts-agentteams" || input == "sts-url" {
			if input == "sts-url" {
				input = "sts-hiclaw"
			}
			c.AuthType = input
		} else {
			return fmt.Errorf("invalid auth type: %s (must be 'none', 'nacos', 'aliyun', 'sts-hiclaw' or 'sts-agentteams')", input)
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
				return fmt.Errorf("access key is required for %s auth", c.AuthType)
			}
		}
		if c.SecretKey == "" {
			fmt.Print("Enter SecretKey: ")
			c.SecretKey = readPassword(reader)
			if c.SecretKey == "" {
				return fmt.Errorf("secret key is required for %s auth", c.AuthType)
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
	hasPlaintextSensitiveFields := false
	if _, err := os.Stat(configPath); err == nil {
		hasPlaintextSensitiveFields, err = configFileHasPlaintextSensitiveFields(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to inspect config from %s: %v\n", configPath, err)
		}
		cfg, err = LoadConfig(configPath)
		if err != nil {
			if strings.Contains(err.Error(), "failed to decrypt") {
				return nil, "", err
			}
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
	} else if hasPlaintextSensitiveFields {
		if err := cfg.SaveConfig(configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to encrypt sensitive fields in %s: %v\n", configPath, err)
		} else {
			fmt.Fprintf(os.Stderr, "Encrypted sensitive fields in %s\n", configPath)
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
	fmt.Printf("Enter auth type (none/nacos/aliyun/sts-hiclaw/sts-agentteams) [%s]: ", currentAuthType)
	input, err = reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read auth type: %w", err)
	}
	input = strings.TrimSpace(strings.ToLower(input))
	if input != "" {
		if input != "none" && input != "nacos" && input != "aliyun" && input != "sts-hiclaw" && input != "sts-agentteams" && input != "sts-url" {
			return fmt.Errorf("invalid auth type: %s (must be 'none', 'nacos', 'aliyun', 'sts-hiclaw' or 'sts-agentteams')", input)
		}
		if input == "sts-url" {
			input = "sts-hiclaw"
		}
		c.AuthType = input
	} else if c.AuthType == "" {
		c.AuthType = "none"
	}

	// Credentials based on auth type
	if c.AuthType == "aliyun" {
		// AccessKey
		currentAK := formatCurrent(c.AccessKey, true)
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
			return fmt.Errorf("access key is required for %s auth", c.AuthType)
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
			return fmt.Errorf("secret key is required for %s auth", c.AuthType)
		}
	} else if isStsAuthType(c.AuthType) {
		if c.AuthType == "sts-agentteams" {
			fmt.Println("Note: sts-agentteams credentials are obtained from AGENTTEAMS_CONTROLLER_URL and AGENTTEAMS_AUTH_TOKEN_FILE environment variables.")
		} else {
			fmt.Println("Note: sts-hiclaw credentials are obtained from HICLAW_CONTROLLER_URL and HICLAW_AUTH_TOKEN_FILE environment variables.")
		}
	} else if c.AuthType == "nacos" {
		// Nacos auth - Username
		currentUser := formatCurrent(c.Username, true)
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
