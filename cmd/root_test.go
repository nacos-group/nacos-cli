package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/config"
	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

func TestPersistentPreRunUsesNacosEnvConfig(t *testing.T) {
	resetRootConfigForTest(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NACOS_HOST", "127.0.0.1")
	t.Setenv("NACOS_PORT", "8848")
	t.Setenv("NACOS_NAMESPACE", "env-ns")

	rootCmd.PersistentPreRun(&cobra.Command{Use: "skill-list"}, nil)

	if serverAddr != "127.0.0.1:8848" {
		t.Fatalf("serverAddr = %q, want %q", serverAddr, "127.0.0.1:8848")
	}
	if namespace != "env-ns" {
		t.Fatalf("namespace = %q, want %q", namespace, "env-ns")
	}
}

func TestPersistentPreRunConfigOverridesNacosEnvConfig(t *testing.T) {
	resetRootConfigForTest(t)
	t.Setenv("NACOS_HOST", "127.0.0.1")
	t.Setenv("NACOS_PORT", "8848")
	t.Setenv("NACOS_NAMESPACE", "env-ns")

	dir := t.TempDir()
	configFile = filepath.Join(dir, "local.conf")
	if err := os.WriteFile(configFile, []byte("host: 10.0.0.1\nport: 8849\nnamespace: file-ns\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	rootCmd.PersistentPreRun(&cobra.Command{Use: "skill-list"}, nil)

	if serverAddr != "10.0.0.1:8849" {
		t.Fatalf("serverAddr = %q, want %q", serverAddr, "10.0.0.1:8849")
	}
	if namespace != "file-ns" {
		t.Fatalf("namespace = %q, want %q", namespace, "file-ns")
	}
}

func TestPersistentPreRunCommandLineIgnoresInvalidEnvPort(t *testing.T) {
	resetRootConfigForTest(t)
	t.Setenv("NACOS_PORT", "bad-port")
	host = "10.0.0.1"
	port = 8849

	rootCmd.PersistentPreRun(&cobra.Command{Use: "skill-list"}, nil)

	if serverAddr != "10.0.0.1:8849" {
		t.Fatalf("serverAddr = %q, want %q", serverAddr, "10.0.0.1:8849")
	}
}

func TestPersistentPreRunConfigIgnoresInvalidEnvPort(t *testing.T) {
	resetRootConfigForTest(t)
	t.Setenv("NACOS_PORT", "bad-port")

	dir := t.TempDir()
	configFile = filepath.Join(dir, "local.conf")
	if err := os.WriteFile(configFile, []byte("host: 10.0.0.1\nport: 8849\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	rootCmd.PersistentPreRun(&cobra.Command{Use: "skill-list"}, nil)

	if serverAddr != "10.0.0.1:8849" {
		t.Fatalf("serverAddr = %q, want %q", serverAddr, "10.0.0.1:8849")
	}
}

func TestPersistentPreRunDoesNotAutoDetectStsHiclaw(t *testing.T) {
	resetRootConfigForTest(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NACOS_HOST", "127.0.0.1")
	t.Setenv("HICLAW_CONTROLLER_URL", "http://controller")
	t.Setenv("HICLAW_AUTH_TOKEN_FILE", filepath.Join(t.TempDir(), "token"))

	rootCmd.PersistentPreRun(&cobra.Command{Use: "skill-list"}, nil)

	if authType != "" {
		t.Fatalf("authType = %q, want empty", authType)
	}
	if stsURL != "" {
		t.Fatalf("stsURL = %q, want empty", stsURL)
	}
	if stsAuthToken != "" {
		t.Fatalf("stsAuthToken = %q, want empty", stsAuthToken)
	}
}

func TestPersistentPreRunStsAgentTeamsUsesAgentTeamsEnv(t *testing.T) {
	resetRootConfigForTest(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NACOS_HOST", "127.0.0.1")
	t.Setenv("NACOS_AUTH_TYPE", "sts-agentteams")
	t.Setenv("HICLAW_CONTROLLER_URL", "http://hiclaw-controller")
	t.Setenv("HICLAW_AUTH_TOKEN_FILE", filepath.Join(t.TempDir(), "hiclaw-token"))

	tokenFile := filepath.Join(t.TempDir(), "agentteams-token")
	if err := os.WriteFile(tokenFile, []byte("agentteams-token\n"), 0600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	t.Setenv("AGENTTEAMS_CONTROLLER_URL", "http://agentteams-controller/")
	t.Setenv("AGENTTEAMS_AUTH_TOKEN_FILE", tokenFile)

	rootCmd.PersistentPreRun(&cobra.Command{Use: "skill-list"}, nil)

	if authType != "sts-agentteams" {
		t.Fatalf("authType = %q, want sts-agentteams", authType)
	}
	if stsURL != "http://agentteams-controller/api/v1/credentials/sts" {
		t.Fatalf("stsURL = %q, want agentteams controller STS URL", stsURL)
	}
	if stsAuthToken != "agentteams-token" {
		t.Fatalf("stsAuthToken = %q, want agentteams-token", stsAuthToken)
	}
}

func TestPersistentPreRunAuthTypeOverrideKeepsProfileConfig(t *testing.T) {
	resetRootConfigForTest(t)
	t.Setenv("HOME", t.TempDir())

	profilePath, err := config.GetProfileConfigPath("dev")
	if err != nil {
		t.Fatalf("get profile path: %v", err)
	}
	cfg := &config.Config{
		Host:      "10.0.0.2",
		Port:      8848,
		AuthType:  "none",
		AccessKey: "profile-ak",
		SecretKey: "profile-sk",
	}
	if err := cfg.SaveConfig(profilePath); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if err := config.SetCurrentProfile("dev"); err != nil {
		t.Fatalf("set current profile: %v", err)
	}
	authType = "aliyun"

	rootCmd.PersistentPreRun(&cobra.Command{Use: "skill-list"}, nil)

	if serverAddr != "10.0.0.2:8848" {
		t.Fatalf("serverAddr = %q, want %q", serverAddr, "10.0.0.2:8848")
	}
	if authType != "aliyun" {
		t.Fatalf("authType = %q, want aliyun", authType)
	}
	if accessKey != "profile-ak" || secretKey != "profile-sk" {
		t.Fatalf("profile credentials not loaded: accessKey=%q secretKey=%q", accessKey, secretKey)
	}
}

func TestCurrentTerminalProfileName(t *testing.T) {
	resetRootConfigForTest(t)
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(func() {
		profileName = ""
	})
	if err := config.SetCurrentProfile("dev"); err != nil {
		t.Fatalf("set current profile: %v", err)
	}

	if got := currentTerminalProfileName(); got != "dev" {
		t.Fatalf("current terminal profile = %q, want dev", got)
	}

	profileName = "prod"
	if got := currentTerminalProfileName(); got != "prod" {
		t.Fatalf("explicit terminal profile = %q, want prod", got)
	}
}

func TestSchemePriority_HttpHostOverridesProfileHttps(t *testing.T) {
	resetRootConfigForTest(t)

	// Simulate profile with scheme: https
	dir := t.TempDir()
	configFile = filepath.Join(dir, "local.conf")
	if err := os.WriteFile(configFile, []byte("host: 10.0.0.1\nport: 8848\nscheme: https\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// User explicitly passes --host http://nacos.example.com
	host = "http://nacos.example.com"

	rootCmd.PersistentPreRun(&cobra.Command{Use: "skill-list"}, nil)

	// The explicit http:// prefix in --host should override profile's scheme: https
	if scheme != "http" {
		t.Fatalf("scheme = %q, want %q (--host http:// prefix should override profile scheme)", scheme, "http")
	}
	if serverAddr != "nacos.example.com:8848" {
		t.Fatalf("serverAddr = %q, want %q", serverAddr, "nacos.example.com:8848")
	}
}

func TestSchemePriority_HttpsHostOverridesEnv(t *testing.T) {
	resetRootConfigForTest(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NACOS_SCHEME", "http")

	// User explicitly passes --host https://nacos.example.com
	host = "https://nacos.example.com:443"

	rootCmd.PersistentPreRun(&cobra.Command{Use: "skill-list"}, nil)

	// The explicit https:// prefix in --host should override env NACOS_SCHEME=http
	if scheme != "https" {
		t.Fatalf("scheme = %q, want %q (--host https:// prefix should override env)", scheme, "https")
	}
	if serverAddr != "nacos.example.com:443" {
		t.Fatalf("serverAddr = %q, want %q", serverAddr, "nacos.example.com:443")
	}
}

func TestSchemePriority_ExplicitFlagOverridesHostPrefix(t *testing.T) {
	resetRootConfigForTest(t)
	t.Setenv("HOME", t.TempDir())

	// --scheme flag takes highest priority, even over --host prefix
	scheme = "http"
	host = "https://nacos.example.com"

	rootCmd.PersistentPreRun(&cobra.Command{Use: "skill-list"}, nil)

	// --scheme flag wins over --host https:// prefix
	if scheme != "http" {
		t.Fatalf("scheme = %q, want %q (--scheme flag should override host prefix)", scheme, "http")
	}
}

func TestPersistentPreRunUsesCurrentProfile(t *testing.T) {
	resetRootConfigForTest(t)
	t.Setenv("HOME", t.TempDir())

	configPath, err := config.GetProfileConfigPath("dev")
	if err != nil {
		t.Fatalf("get profile config path: %v", err)
	}
	cfg := &config.Config{Host: "10.0.0.2", Port: 8848, AuthType: "none", Namespace: "dev-ns"}
	if err := cfg.SaveConfig(configPath); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if err := config.SetCurrentProfile("dev"); err != nil {
		t.Fatalf("set current profile: %v", err)
	}

	rootCmd.PersistentPreRun(&cobra.Command{Use: "skill-list"}, nil)

	if serverAddr != "10.0.0.2:8848" {
		t.Fatalf("serverAddr = %q, want %q", serverAddr, "10.0.0.2:8848")
	}
	if namespace != "dev-ns" {
		t.Fatalf("namespace = %q, want dev-ns", namespace)
	}
}

func TestPersistentPreRunExplicitProfileOverridesCurrentProfile(t *testing.T) {
	resetRootConfigForTest(t)
	t.Setenv("HOME", t.TempDir())

	devPath, err := config.GetProfileConfigPath("dev")
	if err != nil {
		t.Fatalf("get dev profile path: %v", err)
	}
	prodPath, err := config.GetProfileConfigPath("prod")
	if err != nil {
		t.Fatalf("get prod profile path: %v", err)
	}
	if err := (&config.Config{Host: "10.0.0.2", Port: 8848, AuthType: "none", Namespace: "dev-ns"}).SaveConfig(devPath); err != nil {
		t.Fatalf("save dev profile: %v", err)
	}
	if err := (&config.Config{Host: "10.0.0.3", Port: 8849, AuthType: "none", Namespace: "prod-ns"}).SaveConfig(prodPath); err != nil {
		t.Fatalf("save prod profile: %v", err)
	}
	if err := config.SetCurrentProfile("dev"); err != nil {
		t.Fatalf("set current profile: %v", err)
	}
	profileName = "prod"

	rootCmd.PersistentPreRun(&cobra.Command{Use: "skill-list"}, nil)

	if serverAddr != "10.0.0.3:8849" {
		t.Fatalf("serverAddr = %q, want %q", serverAddr, "10.0.0.3:8849")
	}
	if namespace != "prod-ns" {
		t.Fatalf("namespace = %q, want prod-ns", namespace)
	}
}

func TestPersistentPreRunDoesNotCreateConfigForSkillSyncWithoutProfile(t *testing.T) {
	resetRootConfigForTest(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	rootCmd.PersistentPreRun(skillSyncTestCommand("add"), nil)

	configPath, err := config.GetProfileConfigPath("default")
	if err != nil {
		t.Fatalf("get profile path: %v", err)
	}
	if _, err := os.Stat(configPath); err == nil {
		t.Fatalf("config file %s should not be created by skill-sync", configPath)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if serverAddr != "market.hiclaw.io:80" {
		t.Fatalf("serverAddr = %q, want default market address", serverAddr)
	}
}

func TestPersistentPreRunSkillSyncWithoutProfileUsesActiveProfile(t *testing.T) {
	resetRootConfigForTest(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(func() {
		skill.SetCurrentSyncProfile("")
	})

	teamPath, err := config.GetProfileConfigPath("team")
	if err != nil {
		t.Fatal(err)
	}
	if err := (&config.Config{Host: "10.0.0.2", Port: 8848, AuthType: "none", Namespace: "team-ns"}).SaveConfig(teamPath); err != nil {
		t.Fatal(err)
	}
	localPath, err := config.GetProfileConfigPath("local")
	if err != nil {
		t.Fatal(err)
	}
	if err := (&config.Config{Host: "10.0.0.3", Port: 8848, AuthType: "none", Namespace: "local-ns"}).SaveConfig(localPath); err != nil {
		t.Fatal(err)
	}
	if err := config.SetCurrentProfile("team"); err != nil {
		t.Fatal(err)
	}
	if err := skill.SaveActiveSyncProfile("local"); err != nil {
		t.Fatal(err)
	}

	rootCmd.PersistentPreRun(skillSyncTestCommand("start"), nil)

	if got := skill.CurrentSyncProfile(); got != "local" {
		t.Fatalf("current sync profile = %q, want local", got)
	}
	if namespace != "local-ns" {
		t.Fatalf("namespace = %q, want local-ns", namespace)
	}
}

func skillSyncTestCommand(name string) *cobra.Command {
	root := &cobra.Command{Use: "nacos-cli"}
	sync := &cobra.Command{Use: "skill-sync"}
	child := &cobra.Command{Use: name}
	root.AddCommand(sync)
	sync.AddCommand(child)
	return child
}

func resetRootConfigForTest(t *testing.T) {
	t.Helper()

	originalServerAddr := serverAddr
	originalHost := host
	originalPort := port
	originalScheme := scheme
	originalNamespace := namespace
	originalAuthType := authType
	originalUsername := username
	originalPassword := password
	originalAccessKey := accessKey
	originalSecretKey := secretKey
	originalSecurityToken := securityToken
	originalStsURL := stsURL
	originalStsAuthToken := stsAuthToken
	originalConfigFile := configFile
	originalProfileName := profileName
	originalVerbose := verbose

	serverAddr = ""
	host = ""
	port = 0
	scheme = ""
	namespace = ""
	authType = ""
	username = ""
	password = ""
	accessKey = ""
	secretKey = ""
	securityToken = ""
	stsURL = ""
	stsAuthToken = ""
	configFile = ""
	profileName = ""
	verbose = false

	t.Cleanup(func() {
		serverAddr = originalServerAddr
		host = originalHost
		port = originalPort
		scheme = originalScheme
		namespace = originalNamespace
		authType = originalAuthType
		username = originalUsername
		password = originalPassword
		accessKey = originalAccessKey
		secretKey = originalSecretKey
		securityToken = originalSecurityToken
		stsURL = originalStsURL
		stsAuthToken = originalStsAuthToken
		configFile = originalConfigFile
		profileName = originalProfileName
		verbose = originalVerbose
	})
}
