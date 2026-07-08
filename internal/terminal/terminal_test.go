package terminal

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/client"
)

func TestParseCommandArgs(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedCmd  string
		expectedArgs []string
		description  string
	}{
		{
			name:         "simple command without flags",
			input:        "skill-get my-skill",
			expectedCmd:  "skill-get",
			expectedArgs: []string{"my-skill"},
			description:  "Basic command with single positional argument",
		},
		{
			name:         "command with long flag and value",
			input:        "skill-get my-skill --label latest",
			expectedCmd:  "skill-get",
			expectedArgs: []string{"my-skill", "--label", "latest"},
			description:  "Long flag with separate value should be grouped together",
		},
		{
			name:         "command with short flag and value",
			input:        "skill-get my-skill -o /path/to/output",
			expectedCmd:  "skill-get",
			expectedArgs: []string{"my-skill", "-o", "/path/to/output"},
			description:  "Short flag with value should be grouped together",
		},
		{
			name:         "command with multiple flags",
			input:        "skill-get my-skill --version v1 --label stable",
			expectedCmd:  "skill-get",
			expectedArgs: []string{"my-skill", "--version", "v1", "--label", "stable"},
			description:  "Multiple flags with values should all be parsed correctly",
		},
		{
			name:         "command with boolean flag",
			input:        "skill-publish --help",
			expectedCmd:  "skill-publish",
			expectedArgs: []string{"--help"},
			description:  "Boolean flag should not consume next argument",
		},
		{
			name:         "command with mixed args and flags",
			input:        "skill-get skill1 skill2 --label prod -o /tmp",
			expectedCmd:  "skill-get",
			expectedArgs: []string{"skill1", "skill2", "--label", "prod", "-o", "/tmp"},
			description:  "Multiple positional args with flags should work",
		},
		{
			name:         "agentspec command with flags",
			input:        "agentspec-get my-spec --label prod",
			expectedCmd:  "agentspec-get",
			expectedArgs: []string{"my-spec", "--label", "prod"},
			description:  "AgentSpec command with long flag",
		},
		{
			name:         "config-set with file flag",
			input:        "config-set data-id group -f /path/to/file.yaml",
			expectedCmd:  "config-set",
			expectedArgs: []string{"data-id", "group", "-f", "/path/to/file.yaml"},
			description:  "Config set with short file flag",
		},
		{
			name:         "config-set with type flag",
			input:        "config-set data-id group -f /path/to/file.yaml --type yaml",
			expectedCmd:  "config-set",
			expectedArgs: []string{"data-id", "group", "-f", "/path/to/file.yaml", "--type", "yaml"},
			description:  "Config set with explicit config type flag",
		},
		{
			name:         "command with home directory path",
			input:        "skill-get my-skill -o ~/skills",
			expectedCmd:  "skill-get",
			expectedArgs: []string{"my-skill", "-o", "~/skills"},
			description:  "Path with tilde should be treated as single argument",
		},
		{
			name:         "complex command with all options",
			input:        "skill-get test-skill --version v2 --label latest -o /tmp/output",
			expectedCmd:  "skill-get",
			expectedArgs: []string{"test-skill", "--version", "v2", "--label", "latest", "-o", "/tmp/output"},
			description:  "Complex command with multiple flags and values",
		},
		{
			name:         "command with help flag",
			input:        "skill-get --help",
			expectedCmd:  "skill-get",
			expectedArgs: []string{"--help"},
			description:  "Help flag should be recognized as boolean",
		},
		{
			name:         "command with h short flag",
			input:        "skill-get -h",
			expectedCmd:  "skill-get",
			expectedArgs: []string{"-h"},
			description:  "Short help flag should be recognized as boolean",
		},
		{
			name:         "skill upload overwrite flag with value",
			input:        "skill-upload ./my-skill --overwrite true",
			expectedCmd:  "skill-upload",
			expectedArgs: []string{"./my-skill", "--overwrite", "true"},
			description:  "Overwrite flag should consume true or false as its value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args := parseCommandArgs(tt.input)

			if cmd != tt.expectedCmd {
				t.Errorf("parseCommandArgs(%q) cmd = %q, want %q\nTest: %s\nDescription: %s",
					tt.input, cmd, tt.expectedCmd, tt.name, tt.description)
			}

			if len(args) != len(tt.expectedArgs) {
				t.Errorf("parseCommandArgs(%q) args length = %d, want %d\nGot:  %v\nWant: %v\nTest: %s\nDescription: %s",
					tt.input, len(args), len(tt.expectedArgs), args, tt.expectedArgs, tt.name, tt.description)
			}

			for i, arg := range args {
				if i < len(tt.expectedArgs) && arg != tt.expectedArgs[i] {
					t.Errorf("parseCommandArgs(%q) args[%d] = %q, want %q\nFull got:  %v\nFull want: %v\nTest: %s\nDescription: %s",
						tt.input, i, arg, tt.expectedArgs[i], args, tt.expectedArgs, tt.name, tt.description)
				}
			}
		})
	}
}

func TestShowServerInfoIncludesProfile(t *testing.T) {
	term := NewTerminalWithProfile(&client.NacosClient{
		ServerAddr: "127.0.0.1:8848",
		Namespace:  "test-ns",
		AuthType:   client.AuthTypeNone,
	}, "dev")

	output := captureTerminalStdout(t, func() {
		term.showServerInfo()
	})
	if !strings.Contains(output, "Profile:   dev") {
		t.Fatalf("server info should include profile name:\n%s", output)
	}
	if !strings.Contains(output, "Server:    127.0.0.1:8848") {
		t.Fatalf("server info should include server address:\n%s", output)
	}
}

func TestShowServerInfoUsesUserForAliyunAuth(t *testing.T) {
	term := NewTerminalWithProfile(&client.NacosClient{
		ServerAddr: "127.0.0.1:8848",
		Namespace:  "test-ns",
		AuthType:   client.AuthTypeAliyun,
		AccessKey:  "LTAI5t6DxxTJDK77fhAXCnMz",
	}, "hangzhou")

	output := captureTerminalStdout(t, func() {
		term.showServerInfo()
	})
	if strings.Contains(output, "Username:") {
		t.Fatalf("aliyun server info should not show username:\n%s", output)
	}
	if !strings.Contains(output, "User:      LTAI5t6DxxTJDK77fhAXCnMz (AccessKey)") {
		t.Fatalf("aliyun server info should show access key user:\n%s", output)
	}
	if !strings.Contains(output, "Auth Type: aliyun") {
		t.Fatalf("aliyun server info should show plain auth type:\n%s", output)
	}
	if strings.Contains(output, "accessKey:") {
		t.Fatalf("auth type should not repeat access key details:\n%s", output)
	}
}

func TestWelcomeUsesSameConnectionInfoAsServer(t *testing.T) {
	term := NewTerminalWithProfile(&client.NacosClient{
		ServerAddr: "127.0.0.1:8848",
		Namespace:  "",
		AuthType:   client.AuthTypeAliyun,
		AccessKey:  "LTAI5t6DxxTJDK77fhAXCnMz",
	}, "hangzhou")

	welcomeOutput := captureTerminalStdout(t, func() {
		term.printWelcome()
	})
	serverOutput := captureTerminalStdout(t, func() {
		term.showServerInfo()
	})

	for _, want := range []string{
		"Profile:   hangzhou",
		"Server:    127.0.0.1:8848",
		"User:      LTAI5t6DxxTJDK77fhAXCnMz (AccessKey)",
		"Namespace: (public)",
		"Auth Type: aliyun",
	} {
		if !strings.Contains(stripANSI(welcomeOutput), want) {
			t.Fatalf("welcome output missing %q:\n%s", want, welcomeOutput)
		}
		if !strings.Contains(serverOutput, want) {
			t.Fatalf("server output missing %q:\n%s", want, serverOutput)
		}
	}
}

func TestParseSkillUploadOverwrite(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedArgs   []string
		expectedResult bool
		wantErr        bool
	}{
		{
			name:           "default false",
			args:           []string{"./my-skill"},
			expectedArgs:   []string{"./my-skill"},
			expectedResult: false,
		},
		{
			name:           "separate true value",
			args:           []string{"./my-skill", "--overwrite", "true"},
			expectedArgs:   []string{"./my-skill"},
			expectedResult: true,
		},
		{
			name:           "equals false value",
			args:           []string{"--overwrite=false", "./my-skill"},
			expectedArgs:   []string{"./my-skill"},
			expectedResult: false,
		},
		{
			name:    "missing value",
			args:    []string{"./my-skill", "--overwrite"},
			wantErr: true,
		},
		{
			name:    "invalid value",
			args:    []string{"./my-skill", "--overwrite", "yes"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, overwrite, err := parseSkillUploadOverwrite(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if overwrite != tt.expectedResult {
				t.Fatalf("overwrite = %v, want %v", overwrite, tt.expectedResult)
			}
			if len(args) != len(tt.expectedArgs) {
				t.Fatalf("args length = %d, want %d; got %v", len(args), len(tt.expectedArgs), args)
			}
			for i, arg := range args {
				if arg != tt.expectedArgs[i] {
					t.Fatalf("args[%d] = %q, want %q", i, arg, tt.expectedArgs[i])
				}
			}
		})
	}
}

func TestParseCommandArgsEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedCmd  string
		expectedArgs []string
	}{
		{
			name:         "empty input",
			input:        "",
			expectedCmd:  "",
			expectedArgs: nil,
		},
		{
			name:         "whitespace only",
			input:        "   ",
			expectedCmd:  "",
			expectedArgs: nil,
		},
		{
			name:         "command only",
			input:        "skill-list",
			expectedCmd:  "skill-list",
			expectedArgs: []string{},
		},
		{
			name:         "multiple spaces between args",
			input:        "skill-get    my-skill    --label    latest",
			expectedCmd:  "skill-get",
			expectedArgs: []string{"my-skill", "--label", "latest"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args := parseCommandArgs(tt.input)

			if cmd != tt.expectedCmd {
				t.Errorf("parseCommandArgs(%q) cmd = %q, want %q", tt.input, cmd, tt.expectedCmd)
			}

			if len(args) != len(tt.expectedArgs) {
				t.Errorf("parseCommandArgs(%q) args length = %d, want %d", tt.input, len(args), len(tt.expectedArgs))
			}
		})
	}
}

func captureTerminalStdout(t *testing.T, fn func()) string {
	t.Helper()
	origStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = origStdout
	}()
	fn()
	_ = writer.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func stripANSI(s string) string {
	return regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(s, "")
}

func TestCompleterCompletesCommands(t *testing.T) {
	c := completer()
	line := []rune("skill-u")
	got, offset := c.Do(line, len(line))

	if offset != len("skill-u") {
		t.Fatalf("offset = %d, want %d", offset, len("skill-u"))
	}
	assertCompletionSuffix(t, got, "pload")
}

func TestCompleterCompletesPathArgument(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "alpha-dir"), 0755); err != nil {
		t.Fatal(err)
	}
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatal(err)
		}
	}()

	c := completer()
	line := []rune("skill-upload alp")
	got, offset := c.Do(line, len(line))

	if offset != len("alp") {
		t.Fatalf("offset = %d, want %d", offset, len("alp"))
	}
	assertCompletionSuffix(t, got, "ha-dir/")
	assertCompletionSuffix(t, got, "ha.txt")
}

func TestCompleterDisplaysPathCandidatesWithoutDirPrefix(t *testing.T) {
	home := t.TempDir()
	skillsDir := filepath.Join(home, ".codex", "skills")
	if err := os.MkdirAll(filepath.Join(skillsDir, "stop-slop"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "skill_stats.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	c := completer()
	line := []rune("skill-upload ~/.codex/skills/s")
	got, offset := c.Do(line, len(line))

	if offset != len("s") {
		t.Fatalf("offset = %d, want %d", offset, len("s"))
	}
	displayed := displayedCompletionStrings(line, offset, got)
	assertStringPresent(t, displayed, "skill_stats.json")
	assertStringPresent(t, displayed, "stop-slop/")
	assertStringAbsent(t, displayed, "~/.codex/skills/skill_stats.json")
	assertStringAbsent(t, displayed, "~/.codex/skills/stop-slop/")
}

func TestCompleterCompletesPathFlagValue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatal(err)
		}
	}()

	c := completer()
	line := []rune("config-set data DEFAULT_GROUP --file conf")
	got, offset := c.Do(line, len(line))

	if offset != len("conf") {
		t.Fatalf("offset = %d, want %d", offset, len("conf"))
	}
	assertCompletionSuffix(t, got, "ig.yaml")
}

func TestCompleterCompletesInlinePathFlagValue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatal(err)
		}
	}()

	c := completer()
	line := []rune("config-set data DEFAULT_GROUP --file=conf")
	got, offset := c.Do(line, len(line))

	if offset != len("conf") {
		t.Fatalf("offset = %d, want %d", offset, len("conf"))
	}
	assertCompletionSuffix(t, got, "ig.yaml")
}

func assertCompletionSuffix(t *testing.T, candidates [][]rune, want string) {
	t.Helper()
	for _, candidate := range candidates {
		if string(candidate) == want {
			return
		}
	}
	t.Fatalf("completion %q not found in %q", want, completionStrings(candidates))
}

func completionStrings(candidates [][]rune) []string {
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, string(candidate))
	}
	return out
}

func displayedCompletionStrings(line []rune, offset int, candidates [][]rune) []string {
	prefix := string(line[len(line)-offset:])
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, prefix+string(candidate))
	}
	return out
}

func assertStringPresent(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("%q not found in %q", want, values)
}

func assertStringAbsent(t *testing.T, values []string, unwanted string) {
	t.Helper()
	for _, value := range values {
		if value == unwanted {
			t.Fatalf("%q should not be present in %q", unwanted, values)
		}
	}
}
