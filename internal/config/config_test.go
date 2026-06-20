package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSaveConfigEncryptsSensitiveFieldsAndLoadDecrypts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath, err := GetProfileConfigPath("dev")
	if err != nil {
		t.Fatalf("get profile config path: %v", err)
	}
	cfg := &Config{
		Host:          "market.hiclaw.io",
		Port:          80,
		AuthType:      "nacos",
		Username:      "alice@example.com",
		Password:      "password-value-for-test",
		AccessKey:     "access-key-value-for-test",
		SecretKey:     "secret-key-value-for-test",
		SecurityToken: "security-token-value-for-test",
		Namespace:     "test-ns",
	}

	if err := cfg.SaveConfig(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}

	if cfg.Username != "alice@example.com" || cfg.Password != "password-value-for-test" {
		t.Fatalf("SaveConfig mutated in-memory config: %+v", cfg)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	var raw Config
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw config: %v", err)
	}
	for name, value := range map[string]string{
		"username":      raw.Username,
		"password":      raw.Password,
		"accessKey":     raw.AccessKey,
		"secretKey":     raw.SecretKey,
		"securityToken": raw.SecurityToken,
	} {
		if !isEncryptedValue(value) {
			t.Fatalf("%s was not encrypted: %q", name, value)
		}
	}

	keyPath, err := getEncryptionKeyPath()
	if err != nil {
		t.Fatalf("get encryption key path: %v", err)
	}
	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat encryption key: %v", err)
	}
	if keyInfo.Mode().Perm() != 0600 {
		t.Fatalf("encryption key mode = %v, want 0600", keyInfo.Mode().Perm())
	}

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Username != cfg.Username ||
		loaded.Password != cfg.Password ||
		loaded.AccessKey != cfg.AccessKey ||
		loaded.SecretKey != cfg.SecretKey ||
		loaded.SecurityToken != cfg.SecurityToken {
		t.Fatalf("loaded credentials mismatch: got %+v want %+v", loaded, cfg)
	}
}

func TestLoadConfigKeepsPlaintextLegacyConfigCompatible(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	configPath := filepath.Join(homeDir, "legacy.conf")
	if err := os.WriteFile(configPath, []byte(`host: 127.0.0.1
port: 8848
authType: nacos
username: legacy-user
password: legacy-password
namespace: legacy-ns
`), 0600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load legacy config: %v", err)
	}
	if loaded.Username != "legacy-user" || loaded.Password != "legacy-password" {
		t.Fatalf("legacy credentials mismatch: %+v", loaded)
	}

	keyPath, err := getEncryptionKeyPath()
	if err != nil {
		t.Fatalf("get encryption key path: %v", err)
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy plaintext load should not create encryption key, stat err=%v", err)
	}
}

func TestLoadOrCreateConfigMigratesPlaintextProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath, err := GetProfileConfigPath("default")
	if err != nil {
		t.Fatalf("get profile config path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`host: 127.0.0.1
port: 8848
authType: nacos
username: migration-user
password: migration-password
namespace: migration-ns
`), 0600); err != nil {
		t.Fatalf("write plaintext profile: %v", err)
	}

	loaded, _, err := LoadOrCreateConfig("default")
	if err != nil {
		t.Fatalf("load or create config: %v", err)
	}
	if loaded.Username != "migration-user" || loaded.Password != "migration-password" {
		t.Fatalf("loaded migrated credentials mismatch: %+v", loaded)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	var raw Config
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal migrated config: %v", err)
	}
	if !isEncryptedValue(raw.Username) || !isEncryptedValue(raw.Password) {
		t.Fatalf("plaintext profile was not migrated: %+v", raw)
	}
}

func TestLoadEncryptedConfigRequiresExistingKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath, err := GetProfileConfigPath("dev")
	if err != nil {
		t.Fatalf("get profile config path: %v", err)
	}
	cfg := &Config{
		Host:     "market.hiclaw.io",
		Port:     80,
		AuthType: "nacos",
		Username: "encrypted-user",
		Password: "encrypted-password",
	}
	if err := cfg.SaveConfig(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}
	keyPath, err := getEncryptionKeyPath()
	if err != nil {
		t.Fatalf("get encryption key path: %v", err)
	}
	if err := os.Remove(keyPath); err != nil {
		t.Fatalf("remove encryption key: %v", err)
	}

	if _, err := LoadConfig(configPath); err == nil {
		t.Fatalf("LoadConfig succeeded without encryption key")
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("encrypted load without key should not create a new key, stat err=%v", err)
	}
}

func TestGetScheme(t *testing.T) {
	tests := []struct {
		name   string
		scheme string
		want   string
	}{
		{"empty defaults to http", "", "http"},
		{"explicit http", "http", "http"},
		{"explicit https", "https", "https"},
		{"uppercase HTTPS normalized", "HTTPS", "https"},
		{"mixed case", "Https", "https"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Scheme: tt.scheme}
			got := cfg.GetScheme()
			if got != tt.want {
				t.Errorf("GetScheme() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfigWithSchemeField(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath, err := GetProfileConfigPath("https-test")
	if err != nil {
		t.Fatalf("get profile config path: %v", err)
	}
	cfg := &Config{
		Host:   "nacos.example.com",
		Port:   443,
		Scheme: "https",
	}
	if err := cfg.SaveConfig(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Scheme != "https" {
		t.Errorf("loaded Scheme = %q, want %q", loaded.Scheme, "https")
	}
	if loaded.GetScheme() != "https" {
		t.Errorf("GetScheme() = %q, want %q", loaded.GetScheme(), "https")
	}
	if loaded.GetServerAddr() != "nacos.example.com:443" {
		t.Errorf("GetServerAddr() = %q, want %q", loaded.GetServerAddr(), "nacos.example.com:443")
	}
}

func TestCurrentProfileSettings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	current, err := GetCurrentProfile()
	if err != nil {
		t.Fatalf("get default current profile: %v", err)
	}
	if current != DefaultProfile {
		t.Fatalf("current profile = %q, want %q", current, DefaultProfile)
	}

	if err := SetCurrentProfile("dev"); err != nil {
		t.Fatalf("set current profile: %v", err)
	}
	current, err = GetCurrentProfile()
	if err != nil {
		t.Fatalf("get current profile: %v", err)
	}
	if current != "dev" {
		t.Fatalf("current profile = %q, want dev", current)
	}

	if err := ClearCurrentProfile(); err != nil {
		t.Fatalf("clear current profile: %v", err)
	}
	current, err = GetCurrentProfile()
	if err != nil {
		t.Fatalf("get cleared current profile: %v", err)
	}
	if current != DefaultProfile {
		t.Fatalf("current profile after clear = %q, want %q", current, DefaultProfile)
	}
}

func TestListAndDeleteProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	for _, profile := range []string{"prod", "default", "dev"} {
		configPath, err := GetProfileConfigPath(profile)
		if err != nil {
			t.Fatalf("get profile path: %v", err)
		}
		cfg := &Config{Host: "127.0.0.1", Port: 8848, AuthType: "none"}
		if err := cfg.SaveConfig(configPath); err != nil {
			t.Fatalf("save profile %s: %v", profile, err)
		}
	}

	profiles, err := ListProfiles()
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	want := []string{"default", "dev", "prod"}
	if !reflect.DeepEqual(profiles, want) {
		t.Fatalf("profiles = %#v, want %#v", profiles, want)
	}

	if err := DeleteProfile("dev"); err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	exists, err := ProfileExists("dev")
	if err != nil {
		t.Fatalf("profile exists: %v", err)
	}
	if exists {
		t.Fatalf("profile dev still exists")
	}
}

func TestConfigSetValueAndGetValue(t *testing.T) {
	var cfg Config

	for _, pair := range []struct {
		key   string
		value string
	}{
		{"server", "127.0.0.1:8848"},
		{"auth-type", "sts-url"},
		{"username", "alice"},
		{"access-key", "ak"},
		{"secret-key", "sk"},
		{"security-token", "token"},
		{"namespace", "test-ns"},
	} {
		if err := cfg.SetValue(pair.key, pair.value); err != nil {
			t.Fatalf("set %s: %v", pair.key, err)
		}
	}

	if cfg.Host != "127.0.0.1" || cfg.Port != 8848 {
		t.Fatalf("server split mismatch: %+v", cfg)
	}
	if cfg.AuthType != "sts-hiclaw" {
		t.Fatalf("auth type = %q, want sts-hiclaw", cfg.AuthType)
	}

	value, sensitive, err := cfg.GetValue("access-key")
	if err != nil {
		t.Fatalf("get access-key: %v", err)
	}
	if value != "ak" || sensitive {
		t.Fatalf("access-key value=%q sensitive=%v", value, sensitive)
	}

	value, sensitive, err = cfg.GetValue("username")
	if err != nil {
		t.Fatalf("get username: %v", err)
	}
	if value != "alice" || sensitive {
		t.Fatalf("username value=%q sensitive=%v", value, sensitive)
	}

	value, sensitive, err = cfg.GetValue("secret-key")
	if err != nil {
		t.Fatalf("get secret-key: %v", err)
	}
	if value != "sk" || !sensitive {
		t.Fatalf("secret-key value=%q sensitive=%v", value, sensitive)
	}

	value, sensitive, err = cfg.GetValue("server")
	if err != nil {
		t.Fatalf("get server: %v", err)
	}
	if value != "127.0.0.1:8848" || sensitive {
		t.Fatalf("server value=%q sensitive=%v", value, sensitive)
	}
}

func TestNormalizeAuthTypeAcceptsStsAgentTeams(t *testing.T) {
	authType, err := NormalizeAuthType("STS-AgentTeams")
	if err != nil {
		t.Fatalf("NormalizeAuthType() error = %v", err)
	}
	if authType != "sts-agentteams" {
		t.Fatalf("auth type = %q, want sts-agentteams", authType)
	}

	cfg := Config{Host: "127.0.0.1", AuthType: authType}
	if !cfg.IsComplete() {
		t.Fatalf("sts-agentteams config should be complete without local credentials")
	}
	if missing := cfg.GetMissingFields(); len(missing) != 0 {
		t.Fatalf("missing fields = %v, want none", missing)
	}
}
