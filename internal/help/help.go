package help

import (
	"fmt"
	"strings"
)

// CommandHelp defines the help information for a command
type CommandHelp struct {
	Command     string
	Description string
	Parameters  []string
	Examples    []string
}

// All command help definitions
var (
	SkillList = CommandHelp{
		Command:     "skill-list",
		Description: "List all skills from Nacos configuration center.",
		Parameters: []string{
			"--name string   Filter by skill name (supports wildcard *)",
			"--page int      Page number (default: 1)",
			"--size int      Page size (default: 20)",
		},
		Examples: []string{
			"# List all skills",
			"skill-list",
			"",
			"# Search by name",
			"skill-list --name \"creator\"",
			"",
			"# With pagination",
			"skill-list --page 2 --size 10",
		},
	}

	SkillGet = CommandHelp{
		Command:     "skill-get",
		Description: "Download a skill from Nacos to local directory via the Client Skill API.",
		Parameters: []string{
			"skillName...    Required. One or more skill names to download",
			"-o, --output    Output directory (default: ~/.skills)",
			"--version       Specific version to download (e.g. v1, v2)",
			"--label         Route label to resolve version (e.g. latest, stable)",
		},
		Examples: []string{
			"# Download the latest version of a skill",
			"skill-get skill-creator",
			"",
			"# Download a specific version",
			"skill-get skill-creator --version v2",
			"",
			"# Download via label",
			"skill-get skill-creator --label stable",
			"",
			"# Download to a custom directory",
			"skill-get skill-creator -o ~/my-skills",
			"",
			"# Download multiple skills",
			"skill-get skill-creator skill-analyzer",
		},
	}

	SkillPublish = CommandHelp{
		Command:     "skill-publish",
		Description: "Publish a skill to Nacos by uploading it as a ZIP file (creates a draft version).\nReview and go-online operations should be done via the Nacos console.",
		Parameters: []string{
			"skillPath       Required. Path to the skill directory",
			"--all           Publish all skills in the specified directory",
		},
		Examples: []string{
			"# Publish a single skill",
			"skill-publish ./my-skill",
			"",
			"# Publish all skills in a directory",
			"skill-publish --all ./skills-folder",
			"",
			"Note:",
			"  - Skill directory must contain SKILL.md",
			"  - After publishing, use the Nacos console to review and go online",
		},
	}

	ConfigList = CommandHelp{
		Command:     "config-list",
		Description: "List all configurations from Nacos configuration center.",
		Parameters: []string{
			"--data-id string   Filter by data ID (supports wildcard *)",
			"--group string     Filter by group (supports wildcard *)",
			"--page int         Page number (default: 1)",
			"--size int         Page size (default: 20)",
		},
		Examples: []string{
			"# List all configurations",
			"config-list",
			"",
			"# Filter by data ID",
			"config-list --data-id resource*",
			"",
			"# Filter by group",
			"config-list --group skill_*",
			"",
			"# Combine filters with pagination",
			"config-list --data-id *config* --group DEFAULT_GROUP --page 1 --size 50",
		},
	}

	ConfigGet = CommandHelp{
		Command:     "config-get",
		Description: "Get a specific configuration from Nacos.",
		Parameters: []string{
			"dataId          Required. Configuration data ID",
			"group           Required. Configuration group name",
		},
		Examples: []string{
			"# Get a configuration",
			"config-get application.yaml DEFAULT_GROUP",
			"",
			"# Get a skill configuration",
			"config-get skill.json skill_skill-creator",
		},
	}

	ConfigSet = CommandHelp{
		Command:     "config-set",
		Description: "Publish a configuration to Nacos (create or update).",
		Parameters: []string{
			"dataId          Required. Configuration data ID",
			"group           Required. Configuration group name",
			"--file, -f      Path to config file (default: read from stdin)",
		},
		Examples: []string{
			"# Publish from file",
			"config-set application.yaml DEFAULT_GROUP --file ./application.yaml",
			"",
			"# Publish from stdin",
			" echo 'key: value' | nacos-cli config-set app.yaml DEFAULT_GROUP",
			"",
			"# Publish JSON config",
			"config-set skill.json skill_my-skill -f ./skill.json",
		},
	}

	SkillSync = CommandHelp{
		Command:     "skill-sync",
		Description: "(Removed) Skill sync is no longer supported.",
		Parameters:  []string{},
		Examples:    []string{},
	}

	AgentSpecList = CommandHelp{
		Command:     "agentspec-list",
		Description: "List all agent specs from Nacos configuration center.",
		Parameters: []string{
			"--name string     Filter by agent spec name",
			"--search string   Search mode: accurate or blur",
			"--page int        Page number (default: 1)",
			"--size int        Page size (default: 20)",
		},
		Examples: []string{
			"# List all agent specs",
			"agentspec-list",
			"",
			"# Search by name (blur)",
			"agentspec-list --name \"worker\" --search blur",
			"",
			"# With pagination",
			"agentspec-list --page 2 --size 10",
		},
	}

	AgentSpecGet = CommandHelp{
		Command:     "agentspec-get",
		Description: "Download an agent spec from Nacos to local directory via the Client AgentSpec API.",
		Parameters: []string{
			"name...         Required. One or more agent spec names to download",
			"-o, --output    Output directory (default: ~/.agentspecs)",
			"--version       Specific version to download (e.g. v1, v2)",
			"--label         Route label to resolve version (e.g. latest, stable)",
		},
		Examples: []string{
			"# Download the latest version of an agent spec",
			"agentspec-get my-worker",
			"",
			"# Download a specific version",
			"agentspec-get my-worker --version v2",
			"",
			"# Download via label",
			"agentspec-get my-worker --label stable",
			"",
			"# Download to a custom directory",
			"agentspec-get my-worker -o ~/my-specs",
			"",
			"# Download multiple agent specs",
			"agentspec-get worker-a worker-b",
		},
	}

	AgentSpecUpload = CommandHelp{
		Command:     "agentspec-upload",
		Description: "Upload an agent spec to Nacos by uploading it as a ZIP file (creates a draft version).\nReview and go-online operations should be done via the Nacos console.",
		Parameters: []string{
			"agentSpecPath   Required. Path to the agent spec directory or .zip file",
			"--all           Upload all agent specs in the specified directory",
		},
		Examples: []string{
			"# Upload a single agent spec",
			"agentspec-upload ./my-worker",
			"",
			"# Upload a pre-built zip file",
			"agentspec-upload ./my-worker.zip",
			"",
			"# Upload all agent specs in a directory",
			"agentspec-upload --all ./specs-folder",
			"",
			"Note:",
			"  - Agent spec directory must contain manifest.json",
			"  - After uploading, use the Nacos console to review and go online",
		},
	}
)

// FormatForCLI formats help content for CLI mode (Cobra Long description)
func (h *CommandHelp) FormatForCLI(cliPrefix string) string {
	result := h.Description + "\n\nParameters:\n"
	for _, param := range h.Parameters {
		result += "  " + param + "\n"
	}
	result += "\nExamples:\n"
	for _, example := range h.Examples {
		if example == "" {
			result += "\n"
		} else {
			// Replace command name with CLI prefix
			if example[0] != '#' && example[0] != ' ' && example != "Note:" {
				result += "  " + cliPrefix + " " + example + "\n"
			} else {
				result += "  " + example + "\n"
			}
		}
	}
	return result
}

// FormatForTerminal formats help content for terminal mode with colors
func (h *CommandHelp) FormatForTerminal() {
	fmt.Printf("\033[1;36mCommand: %s\033[0m\n", h.Command)
	fmt.Printf("\n%s\n\n", h.Description)
	fmt.Println("\033[33mParameters:\033[0m")
	for _, param := range h.Parameters {
		fmt.Printf("  %s\n", param)
	}
	fmt.Println()
	fmt.Println("\033[33mExamples:\033[0m")
	for _, example := range h.Examples {
		if example == "" {
			fmt.Println()
		} else if strings.HasPrefix(example, "Note:") || strings.HasPrefix(example, "  -") {
			fmt.Printf("\033[33m%s\033[0m\n", example)
		} else {
			fmt.Printf("  %s\n", example)
		}
	}
}
