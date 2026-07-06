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
		Description: "List all skills with governance info (latest/editing/reviewing/onlineCnt/enable/scope/owner/updateTime).",
		Parameters: []string{
			"--name string   Filter by skill name (supports wildcard *)",
			"--page int      Page number (default: 1)",
			"--size int      Page size (default: 20)",
			"--output string Output format: pretty | json (default: pretty)",
		},
		Examples: []string{
			"# List all skills (human-readable, multi-line)",
			"skill-list",
			"",
			"# Search by name",
			"skill-list --name \"creator\"",
			"",
			"# With pagination",
			"skill-list --page 2 --size 10",
			"",
			"# Machine-readable JSON (for scripts / jq)",
			"skill-list --output json",
			"",
			"Note:",
			"  - Every row shows latest/editing/reviewing version pointers and online count",
			"  - For full version-level status (editing/reviewing/online/offline per version), use skill-describe",
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

	SkillUpload = CommandHelp{
		Command:     "skill-upload",
		Description: "Upload a skill to Nacos as a ZIP file (creates an editing draft version).",
		Parameters: []string{
			"skillPath       Required. Path to the skill directory (or .zip file)",
			"--all           Upload all skills in the specified directory",
			"--overwrite     Whether to overwrite an existing draft: true | false (default: false)",
		},
		Examples: []string{
			"# Upload a single skill",
			"skill-upload ./my-skill",
			"",
			"# Upload and overwrite an existing draft",
			"skill-upload ./my-skill --overwrite true",
			"",
			"# Upload all skills in a directory",
			"skill-upload --all ./skills-folder",
			"",
			"Note:",
			"  - Skill directory must contain SKILL.md",
			"  - After upload, use skill-review to submit the draft for review",
			"  - After review passes, use skill-release to publish the version online",
		},
	}

	SkillReview = CommandHelp{
		Command:     "skill-review",
		Description: "Submit a skill draft version for review (moves editing -> reviewing).",
		Parameters: []string{
			"skillName       Required. Skill name to submit for review",
			"--version       Optional. Specific draft version to submit",
		},
		Examples: []string{
			"# Submit the current draft for review",
			"skill-review my-skill",
			"",
			"# Submit a specific draft version",
			"skill-review my-skill --version 1.0.0",
			"",
			"Note:",
			"  - If --version is omitted, the server submits the current editingVersion",
			"  - After the review passes, call skill-release to make it online",
		},
	}

	SkillRelease = CommandHelp{
		Command:     "skill-release",
		Description: "Release (publish) an approved skill version to make it online.",
		Parameters: []string{
			"skillName            Required. Skill name to release",
			"--version            Required. Approved (reviewing) version to release",
			"--update-latest      Whether to update the 'latest' label (default: true)",
		},
		Examples: []string{
			"# Release an approved version and mark it as latest",
			"skill-release my-skill --version 1.0.0",
			"",
			"# Release without updating the latest label",
			"skill-release my-skill --version 1.0.0 --update-latest=false",
			"",
			"Note:",
			"  - The target version must be in 'reviewing' state (approved by pipeline)",
			"  - Flow: skill-upload -> skill-review -> skill-release",
		},
	}

	SkillOnline = CommandHelp{
		Command:     "skill-online",
		Description: "Bring a whole skill or one skill version online.",
		Parameters: []string{
			"skillName       Required. Skill name",
			"--version       Optional. Version to bring online. If omitted, enables the whole skill",
		},
		Examples: []string{
			"# Bring the whole skill online",
			"skill-online my-skill",
			"",
			"# Bring one version online",
			"skill-online my-skill --version 1.0.0",
		},
	}

	SkillOffline = CommandHelp{
		Command:     "skill-offline",
		Description: "Take a whole skill or one skill version offline.",
		Parameters: []string{
			"skillName       Required. Skill name",
			"--version       Optional. Version to take offline. If omitted, disables the whole skill",
		},
		Examples: []string{
			"# Take the whole skill offline",
			"skill-offline my-skill",
			"",
			"# Take one version offline",
			"skill-offline my-skill --version 1.0.0",
		},
	}

	SkillScope = CommandHelp{
		Command:     "skill-scope",
		Description: "Set the visibility scope of a skill.",
		Parameters: []string{
			"skillName       Required. Skill name",
			"--scope         Required. Visibility scope: PUBLIC or PRIVATE",
		},
		Examples: []string{
			"# Make a skill public",
			"skill-scope my-skill --scope PUBLIC",
			"",
			"# Make a skill private",
			"skill-scope my-skill --scope PRIVATE",
		},
	}

	SkillTags = CommandHelp{
		Command:     "skill-tags",
		Description: "Set metadata tags for a skill.",
		Parameters: []string{
			"skillName       Required. Skill name",
			"--tags          Required. Skill metadata tags, for example retail,finance",
		},
		Examples: []string{
			"# Set metadata tags",
			"skill-tags my-skill --tags retail,finance",
		},
	}

	SkillDescribe = CommandHelp{
		Command:     "skill-describe",
		Description: "Show detailed info of a single skill, including governance metadata and the full version list with per-version status (editing/reviewing/online/offline).",
		Parameters: []string{
			"skillName       Required. Skill name to describe",
			"--output string Output format: pretty | json (default: pretty)",
		},
		Examples: []string{
			"# Show skill detail in human-readable form",
			"skill-describe my-skill",
			"",
			"# Machine-readable JSON (for scripts / jq)",
			"skill-describe my-skill --output json",
			"",
			"Note:",
			"  - Use this to answer: which versions exist, which has been approved, which is online",
			"  - The 'status' column reflects each version's lifecycle state",
		},
	}

	SkillPublish = CommandHelp{
		Command:     "skill-publish",
		Description: "[DEPRECATED] Equivalent to running skill-upload followed by skill-review.\nPlease migrate to: skill-upload (-> upload draft), skill-review (-> submit for review), skill-release (-> publish online).",
		Parameters: []string{
			"skillPath       Required. Path to the skill directory",
			"--all           Process all skills in the specified directory",
		},
		Examples: []string{
			"# [DEPRECATED] Upload and submit a single skill for review",
			"skill-publish ./my-skill",
			"",
			"# [DEPRECATED] Upload and submit all skills in a directory",
			"skill-publish --all ./skills-folder",
			"",
			"Note:",
			"  - This command is deprecated. Use skill-upload + skill-review + skill-release instead.",
		},
	}

	SkillSubscribe = CommandHelp{
		Command:     "skill-subscribe",
		Description: "Subscribe to skill updates and start a background watcher that auto-updates local files.",
		Parameters: []string{
			"skillName...    Required. One or more skill names to subscribe",
			"-o, --output    Output directory (default: ~/.skills)",
			"--version       Pin to a specific version (e.g. v1, v2)",
			"--label         Route label to resolve version (e.g. latest, stable)",
			"--interval      Poll interval (default: 30s, min: 5s)",
			"--foreground    Run watcher in the current process instead of the background",
		},
		Examples: []string{
			"# Subscribe to a skill and start the background watcher",
			"skill-subscribe pdf",
			"",
			"# Subscribe to multiple skills with custom interval",
			"skill-subscribe pdf commit --interval 10s",
			"",
			"# Subscribe with a specific label",
			"skill-subscribe pdf --label stable",
			"",
			"# Run in foreground for debugging",
			"skill-subscribe pdf --foreground",
			"",
			"Note:",
			"  - Runs in the background by default and polls the server periodically for changes",
			"  - Auto-updates local skill files when MD5 fingerprint changes",
			"  - Subscription state is persisted in skills-lock.json",
			"  - Background watcher state is persisted in skills-watcher.pid",
			"  - Use skill-stop to stop the background watcher",
		},
	}

	SkillUnsubscribe = CommandHelp{
		Command:     "skill-unsubscribe",
		Description: "Unsubscribe from skill updates to stop auto-updating.",
		Parameters: []string{
			"skillName...    Required. One or more skill names to unsubscribe",
			"-o, --output    Output directory (default: ~/.skills)",
			"--purge         Remove local skill files after unsubscribing",
		},
		Examples: []string{
			"# Unsubscribe from a skill (keep local files)",
			"skill-unsubscribe pdf",
			"",
			"# Unsubscribe and remove local files",
			"skill-unsubscribe pdf --purge",
		},
	}

	SkillStatus = CommandHelp{
		Command:     "skill-status",
		Description: "Show status of all installed and subscribed skills tracked in skills-lock.json.",
		Parameters: []string{
			"-o, --output    Output directory (default: ~/.skills)",
		},
		Examples: []string{
			"# Show status of all tracked skills",
			"skill-status",
			"",
			"Note:",
			"  - Displays: skill name, version, MD5, subscription status, last updated, watcher status",
			"  - Use skill-subscribe to add subscriptions",
			"  - Use skill-unsubscribe to remove subscriptions",
		},
	}

	SkillStop = CommandHelp{
		Command:     "skill-stop",
		Description: "Stop the background skill subscription watcher.",
		Parameters: []string{
			"-o, --output    Output directory (default: ~/.skills)",
		},
		Examples: []string{
			"# Stop the default background watcher",
			"skill-stop",
			"",
			"# Stop the watcher for a custom skill directory",
			"skill-stop -o ~/my-skills",
			"",
			"Note:",
			"  - Stops the watcher process recorded in skills-watcher.pid",
			"  - Does not unsubscribe skills or delete local skill files",
			"  - Use skill-unsubscribe to remove subscriptions",
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
		Description: "Unified skill synchronization management across agent directories.",
		Parameters: []string{
			"add <skill|--all> Add and initial pull/link",
			"remove <skill>  Remove from sync management (keeps agent copies)",
			"mode <local|nacos> Set local or Nacos sync mode",
			"status          Show sync state table",
			"resolve <skill> Resolve conflicts (--use-local / --use-remote)",
			"start           Start background sync daemon",
			"stop            Stop background sync daemon",
			"set-label       Set global tracking label",
			"agent           Manage agent directories",
			"pull <skill>    Manually pull latest version",
		},
		Examples: []string{
			"# Add a skill",
			"skill-sync add pdf",
			"",
			"# Start background sync",
			"skill-sync start",
			"",
			"# Check status",
			"skill-sync status",
			"",
			"# Resolve conflict non-interactively",
			"skill-sync resolve pdf --use-remote",
			"",
			"# Resolve all conflicts",
			"skill-sync resolve --all --use-local",
		},
	}

	AgentSpecList = CommandHelp{
		Command:     "agentspec-list",
		Description: "List all agent specs with governance info (latest/editing/reviewing/onlineCnt/enable/scope/bizTags/updateTime).",
		Parameters: []string{
			"--name string   Filter by agent spec name (supports wildcard *)",
			"--page int      Page number (default: 1)",
			"--size int      Page size (default: 20)",
			"--output string Output format: pretty | json (default: pretty)",
		},
		Examples: []string{
			"# List all agent specs (human-readable, multi-line)",
			"agentspec-list",
			"",
			"# Search by name",
			"agentspec-list --name \"worker\"",
			"",
			"# With pagination",
			"agentspec-list --page 2 --size 10",
			"",
			"# Machine-readable JSON (for scripts / jq)",
			"agentspec-list --output json",
			"",
			"Note:",
			"  - Every row shows latest/editing/reviewing version pointers and online count",
			"  - For full version-level status (editing/reviewing/online/offline per version), use agentspec-describe",
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
		Description: "Upload an agent spec to Nacos as a ZIP file (creates an editing draft version).",
		Parameters: []string{
			"agentSpecPath   Required. Path to the agent spec directory (or .zip file)",
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
			"  - After upload, use agentspec-review to submit the draft for review",
			"  - After review passes, use agentspec-release to publish the version online",
		},
	}

	AgentSpecReview = CommandHelp{
		Command:     "agentspec-review",
		Description: "Submit an agent spec draft version for review (moves editing -> reviewing).",
		Parameters: []string{
			"agentSpecName   Required. Agent spec name to submit for review",
			"--version       Optional. Specific draft version to submit",
		},
		Examples: []string{
			"# Submit the current draft for review",
			"agentspec-review my-worker",
			"",
			"# Submit a specific draft version",
			"agentspec-review my-worker --version 1.0.0",
			"",
			"Note:",
			"  - If --version is omitted, the server submits the current editingVersion",
			"  - After the review passes, call agentspec-release to make it online",
		},
	}

	AgentSpecRelease = CommandHelp{
		Command:     "agentspec-release",
		Description: "Release (publish) an approved agent spec version to make it online.",
		Parameters: []string{
			"agentSpecName        Required. Agent spec name to release",
			"--version            Required. Approved (reviewing) version to release",
			"--update-latest      Whether to update the 'latest' label (default: true)",
		},
		Examples: []string{
			"# Release an approved version and mark it as latest",
			"agentspec-release my-worker --version 1.0.0",
			"",
			"# Release without updating the latest label",
			"agentspec-release my-worker --version 1.0.0 --update-latest=false",
			"",
			"Note:",
			"  - The target version must be in 'reviewing' state (approved by pipeline)",
			"  - Flow: agentspec-upload -> agentspec-review -> agentspec-release",
		},
	}

	AgentSpecDescribe = CommandHelp{
		Command:     "agentspec-describe",
		Description: "Show detailed info of a single agent spec, including governance metadata and the full version list with per-version status (editing/reviewing/online/offline).",
		Parameters: []string{
			"agentSpecName   Required. Agent spec name to describe",
			"--output string Output format: pretty | json (default: pretty)",
		},
		Examples: []string{
			"# Show agent spec detail in human-readable form",
			"agentspec-describe my-worker",
			"",
			"# Machine-readable JSON (for scripts / jq)",
			"agentspec-describe my-worker --output json",
			"",
			"Note:",
			"  - Use this to answer: which versions exist, which has been approved, which is online",
			"  - The 'status' column reflects each version's lifecycle state",
		},
	}

	AgentSpecPublish = CommandHelp{
		Command:     "agentspec-publish",
		Description: "[DEPRECATED] Equivalent to running agentspec-upload followed by agentspec-review.\nPlease migrate to: agentspec-upload (-> upload draft), agentspec-review (-> submit for review), agentspec-release (-> publish online).",
		Parameters: []string{
			"agentSpecPath   Required. Path to the agent spec directory",
			"--all           Process all agent specs in the specified directory",
		},
		Examples: []string{
			"# [DEPRECATED] Upload and submit a single agent spec for review",
			"agentspec-publish ./my-worker",
			"",
			"# [DEPRECATED] Upload and submit all agent specs in a directory",
			"agentspec-publish --all ./specs-folder",
			"",
			"Note:",
			"  - This command is deprecated. Use agentspec-upload + agentspec-review + agentspec-release instead.",
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
