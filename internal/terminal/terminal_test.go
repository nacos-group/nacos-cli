package terminal

import (
	"testing"
)

func TestParseCommandArgs(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCmd   string
		expectedArgs  []string
		description   string
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

func TestParseCommandArgsEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedCmd   string
		expectedArgs  []string
	}{
		{
			name:          "empty input",
			input:         "",
			expectedCmd:   "",
			expectedArgs:  nil,
		},
		{
			name:          "whitespace only",
			input:         "   ",
			expectedCmd:   "",
			expectedArgs:  nil,
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
