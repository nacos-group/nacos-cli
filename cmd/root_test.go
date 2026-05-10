package cmd

import (
	"os"
	"path/filepath"
	"testing"

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

func resetRootConfigForTest(t *testing.T) {
	t.Helper()

	originalServerAddr := serverAddr
	originalHost := host
	originalPort := port
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
