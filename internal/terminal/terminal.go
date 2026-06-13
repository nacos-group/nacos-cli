package terminal

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/nacos-group/nacos-cli/internal/agentspec"
	"github.com/nacos-group/nacos-cli/internal/client"
	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/skill"
)

const defaultDescLimit = 200

// Terminal represents an interactive terminal
type Terminal struct {
	client           *client.NacosClient
	skillService     *skill.SkillService
	agentSpecService *agentspec.AgentSpecService
	rl               *readline.Instance
	running          bool
}

// NewTerminal creates a new interactive terminal
func NewTerminal(nacosClient *client.NacosClient) *Terminal {
	return &Terminal{
		client:           nacosClient,
		skillService:     skill.NewSkillService(nacosClient),
		agentSpecService: agentspec.NewAgentSpecService(nacosClient),
		running:          true,
	}
}

// getPrompt returns the prompt string with user info
func (t *Terminal) getPrompt() string {
	// Show abbreviated user info in prompt
	switch t.client.AuthType {
	case client.AuthTypeNacos:
		if t.client.Username != "" {
			return fmt.Sprintf("\033[32m%s@nacos>\033[0m ", t.client.Username)
		}
	case client.AuthTypeAliyun, client.AuthTypeStsToken:
		if t.client.AccessKey != "" {
			ak := t.client.AccessKey
			if len(ak) > 8 {
				ak = ak[:8]
			}
			return fmt.Sprintf("\033[32m%s@nacos>\033[0m ", ak)
		}
	}
	return "\033[32mnacos>\033[0m "
}

// completer provides command auto-completion
func completer() *readline.PrefixCompleter {
	return readline.NewPrefixCompleter(
		readline.PcItem("help"),
		readline.PcItem("quit"),
		readline.PcItem("skill-list",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--name"),
			readline.PcItem("--page"),
			readline.PcItem("--size"),
			readline.PcItem("--output"),
		),
		readline.PcItem("skill-describe",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--output"),
		),
		readline.PcItem("skill-get",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
		),
		readline.PcItem("skill-upload",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--all"),
			readline.PcItem("--overwrite"),
		),
		readline.PcItem("skill-publish",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--all"),
		),
		readline.PcItem("skill-review",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--version"),
		),
		readline.PcItem("skill-release",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--version"),
			readline.PcItem("--update-latest"),
		),
		readline.PcItem("skill-scope",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--scope"),
		),
		readline.PcItem("skill-tags",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--tags"),
		),
		readline.PcItem("agentspec-list",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--name"),
			readline.PcItem("--page"),
			readline.PcItem("--size"),
			readline.PcItem("--output"),
		),
		readline.PcItem("agentspec-describe",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--output"),
		),
		readline.PcItem("agentspec-get",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
		),
		readline.PcItem("agentspec-upload",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--all"),
		),
		readline.PcItem("agentspec-publish",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--all"),
		),
		readline.PcItem("agentspec-review",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--version"),
		),
		readline.PcItem("agentspec-release",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--version"),
			readline.PcItem("--update-latest"),
		),
		readline.PcItem("config-list",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
		),
		readline.PcItem("config-get",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
		),
		readline.PcItem("config-set",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--file"),
			readline.PcItem("-f"),
		),
		readline.PcItem("clear"),
		readline.PcItem("server"),
		readline.PcItem("ns"),
	)
}

// Start starts the interactive terminal
func (t *Terminal) Start() error {
	// Configure readline
	historyFile := filepath.Join(os.TempDir(), ".nacos-cli-history")

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          t.getPrompt(),
		HistoryFile:     historyFile,
		AutoComplete:    completer(),
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		return err
	}
	defer rl.Close()

	t.rl = rl

	t.printWelcome()

	for t.running {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		t.handleCommand(line)
	}

	return nil
}

// printWelcome prints welcome message
func (t *Terminal) printWelcome() {
	fmt.Println("\033[36m╔════════════════════════════════════════════════════════╗\033[0m")
	fmt.Println("\033[36m║\033[0m                  \033[1mNacos CLI Terminal\033[0m                   \033[36m║\033[0m")
	fmt.Println("\033[36m╚════════════════════════════════════════════════════════╝\033[0m")
	fmt.Printf("\033[33mServer:\033[0m %s\n", t.client.ServerAddr)
	if t.client.Namespace != "" {
		fmt.Printf("\033[33mNamespace:\033[0m %s\n", t.client.Namespace)
	}
	// Show user info based on auth type
	switch t.client.AuthType {
	case client.AuthTypeNacos:
		if t.client.Username != "" {
			fmt.Printf("\033[33mUser:\033[0m %s (username/password)\n", t.client.Username)
		}
	case client.AuthTypeAliyun:
		if t.client.AccessKey != "" {
			fmt.Printf("\033[33mUser:\033[0m %s (AccessKey)\n", t.client.AccessKey)
		}
	case client.AuthTypeStsToken:
		if t.client.AccessKey != "" {
			fmt.Printf("\033[33mUser:\033[0m %s (STS-HICLAW)\n", t.client.AccessKey)
		}
	case client.AuthTypeNone:
		fmt.Printf("\033[33mAuth:\033[0m None (public access)\n")
	}
	fmt.Println()
	fmt.Println("\033[90mType '\033[0mhelp\033[90m' for available commands\033[0m")
	fmt.Println("\033[90mPress '\033[0mTab\033[90m' for auto-completion\033[0m")
	fmt.Println("\033[90mPress '\033[0mCtrl+C\033[90m' or type '\033[0mquit\033[90m' to quit\033[0m")
	fmt.Println()
}

// parseCommandArgs parses command line arguments, properly handling flags and their values
// For example: "skill-get my-skill --label latest -o /path" should recognize:
//   - skill name: my-skill
//   - flags: --label latest, -o /path
//
// This prevents flags and their values from being treated as additional skill names
func parseCommandArgs(input string) (cmd string, args []string) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", nil
	}

	cmd = parts[0]
	args = make([]string, 0, len(parts)-1)

	// Parse remaining parts, handling flags properly
	for i := 1; i < len(parts); i++ {
		arg := parts[i]

		// Check if this is a flag
		if strings.HasPrefix(arg, "-") {
			args = append(args, arg)
			// Check if flag takes a value (not a boolean flag like --help or --all)
			// Flags that don't take values
			booleanFlags := map[string]bool{
				"--help": true, "-h": true,
				"--all": true,
			}

			// If it's a long flag (--flag), check if value is separate
			if strings.HasPrefix(arg, "--") && !strings.Contains(arg, "=") {
				if !booleanFlags[arg] && i+1 < len(parts) && !strings.HasPrefix(parts[i+1], "-") {
					// Next arg is the value, skip it in main args (it will be handled by flag parser)
					args = append(args, parts[i+1])
					i++ // Skip next arg since we consumed it
				}
			} else if strings.HasPrefix(arg, "-") && !strings.Contains(arg, "=") {
				// Short flag (-o), check if value is separate
				if !booleanFlags[arg] && len(arg) == 2 && i+1 < len(parts) && !strings.HasPrefix(parts[i+1], "-") {
					// Next arg is the value
					args = append(args, parts[i+1])
					i++ // Skip next arg
				}
			}
		} else {
			// Not a flag, treat as positional argument
			args = append(args, arg)
		}
	}

	return cmd, args
}

// handleCommand handles user command
func (t *Terminal) handleCommand(input string) {
	cmd, args := parseCommandArgs(input)

	switch cmd {
	case "help":
		t.showHelp()
	case "quit":
		t.exit()
	case "skill-list":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showSkillListHelp()
		} else {
			t.listSkills(args)
		}
	case "skill-describe":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showSkillDescribeHelp()
		} else {
			t.describeSkill(args)
		}
	case "skill-get":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showSkillGetHelp()
		} else {
			t.getSkill(args)
		}
	case "skill-upload":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showSkillUploadHelp()
		} else {
			t.uploadSkill(args)
		}
	case "skill-publish":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showSkillPublishHelp()
		} else {
			t.publishLegacy(args)
		}
	case "skill-review":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showSkillReviewHelp()
		} else {
			t.submitSkill(args)
		}
	case "skill-release":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showSkillReleaseHelp()
		} else {
			t.releaseSkill(args)
		}
	case "skill-scope", "skill-visibility":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showSkillScopeHelp()
		} else {
			t.updateSkillScope(args)
		}
	case "skill-tags", "skill-biz-tags":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showSkillTagsHelp()
		} else {
			t.updateSkillTags(args)
		}
	case "skill-sync":
		fmt.Println("\033[33mskill-sync has been removed.\033[0m")
		fmt.Println("\033[90mUse 'skill-get' to download skills.\033[0m")
	case "agentspec-list":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showAgentSpecListHelp()
		} else {
			t.listAgentSpecs(args)
		}
	case "agentspec-describe":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showAgentSpecDescribeHelp()
		} else {
			t.describeAgentSpec(args)
		}
	case "agentspec-get":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showAgentSpecGetHelp()
		} else {
			t.getAgentSpec(args)
		}
	case "agentspec-upload":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showAgentSpecUploadHelp()
		} else {
			t.uploadAgentSpec(args)
		}
	case "agentspec-publish":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showAgentSpecPublishHelp()
		} else {
			t.publishAgentSpecLegacy(args)
		}
	case "agentspec-review":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showAgentSpecReviewHelp()
		} else {
			t.submitAgentSpec(args)
		}
	case "agentspec-release":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showAgentSpecReleaseHelp()
		} else {
			t.releaseAgentSpec(args)
		}
	case "config-list":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showConfigListHelp()
		} else {
			t.listConfigs(args)
		}
	case "config-get":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showConfigGetHelp()
		} else {
			t.getConfig(args)
		}
	case "config-set":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showConfigSetHelp()
		} else {
			t.setConfig(args)
		}
	case "clear":
		t.clear()
	case "server":
		t.showServerInfo()
	case "ns":
		t.namespace(args)
	default:
		fmt.Printf("\033[31mUnknown command:\033[0m %s\n", cmd)
		fmt.Println("\033[90mType '\033[0mhelp\033[90m' for available commands\033[0m")
	}
	fmt.Println()
}

// showHelp shows available commands
func (t *Terminal) showHelp() {
	fmt.Println("\033[1;36mAvailable Commands:\033[0m")
	fmt.Println("\033[90m─────────────────────────────────────────────────────────────────────────────────────────────────────────\033[0m")
	fmt.Printf("\033[90m%-20s %-40s %-30s\033[0m\n", "Command", "Description", "Usage")
	fmt.Println("\033[90m─────────────────────────────────────────────────────────────────────────────────────────────────────────\033[0m")

	// Skill Management
	fmt.Println("\033[1;33mSkill Management\033[0m")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "skill-list", "List all skills", "skill-list [options]")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "", "Options: --name, --page, --size, --output", "")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "skill-describe", "Show skill detail + versions", "skill-describe <name> [--output json]")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "skill-get", "Download a skill to ~/.skills", "skill-get <name> [--version v1] [--label stable]")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "skill-upload", "Upload a skill draft (editing)", "skill-upload <path>")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "", "Upload all skills in directory", "skill-upload --all <folder>")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "skill-review", "Submit a draft for review", "skill-review <name> [--version v1]")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "skill-release", "Release an approved version", "skill-release <name> --version v1")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "skill-scope", "Set skill visibility", "skill-scope <name> --scope PUBLIC")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "skill-tags", "Set metadata tags", "skill-tags <name> --tags a,b")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "skill-publish", "[DEPRECATED] upload + review", "skill-publish <path> (use skill-upload/review)")
	fmt.Println()

	// AgentSpec Management
	fmt.Println("\033[1;33mAgentSpec Management\033[0m")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "agentspec-list", "List all agent specs", "agentspec-list [options]")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "", "Options: --name, --page, --size, --output", "")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "agentspec-describe", "Show agent spec detail + versions", "agentspec-describe <name> [--output json]")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "agentspec-get", "Download an agent spec to ~/.agentspecs", "agentspec-get <name> [--version v1] [--label stable]")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "agentspec-upload", "Upload an agent spec draft (editing)", "agentspec-upload <path>")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "", "Upload all agent specs in directory", "agentspec-upload --all <folder>")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "agentspec-review", "Submit a draft for review", "agentspec-review <name> [--version v1]")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "agentspec-release", "Release an approved version", "agentspec-release <name> --version v1")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "agentspec-publish", "[DEPRECATED] upload + review", "agentspec-publish <path> (use upload/review)")
	fmt.Println()

	// Configuration Management
	fmt.Println("\033[1;33mConfiguration Management\033[0m")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "config-list", "List all configurations", "config-list [options]")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "", "Options: --data-id, --group, --page, --size", "")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "config-get", "Get configuration content", "config-get <data-id> <group>")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "config-set", "Publish config (-f file or type content)", "config-set <data-id> <group> [-f <file>]")
	fmt.Println()

	// System
	fmt.Println("\033[1;33mSystem\033[0m")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "server", "Show server information", "server")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "ns", "Show current namespace", "ns")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "ns <namespace>", "Switch to different namespace", "ns <namespace>")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "clear", "Clear screen", "clear")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "help", "Show this help message", "help")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "quit", "Exit terminal", "quit")

	fmt.Println("\033[90m─────────────────────────────────────────────────────────────────────────────────────────────────────────\033[0m")
	fmt.Println("\033[90mTip: Use Tab for auto-completion, ↑↓ for history\033[0m")
}

// exit exits the terminal
func (t *Terminal) exit() {
	fmt.Println("\033[36mGoodbye! Have a great day!\033[0m")
	t.running = false
}

// clear clears the screen
func (t *Terminal) clear() {
	fmt.Print("\033[H\033[2J")
	t.printWelcome()
}

// showServerInfo shows server information
func (t *Terminal) showServerInfo() {
	fmt.Println("Server Information:")
	fmt.Println("─────────────────────────────────────────────────────────")
	fmt.Printf("  Server:    %s\n", t.client.ServerAddr)
	fmt.Printf("  Username:  %s\n", t.client.Username)
	fmt.Printf("  Namespace: %s\n", t.client.Namespace)
	fmt.Printf("  Auth Type: %s\n", t.getAuthTypeDisplay())
	fmt.Println("─────────────────────────────────────────────────────────")
}

// getAuthTypeDisplay returns a human-readable auth type description
func (t *Terminal) getAuthTypeDisplay() string {
	switch t.client.AuthType {
	case client.AuthTypeNacos:
		if t.client.Username != "" {
			return fmt.Sprintf("nacos (user: %s)", t.client.Username)
		}
		return "nacos"
	case client.AuthTypeAliyun:
		if t.client.AccessKey != "" {
			return fmt.Sprintf("aliyun (accessKey: %s...)", t.client.AccessKey[:min(8, len(t.client.AccessKey))])
		}
		return "aliyun"
	case client.AuthTypeStsToken:
		if t.client.AccessKey != "" {
			return fmt.Sprintf("sts-hiclaw (accessKey: %s...)", t.client.AccessKey[:min(8, len(t.client.AccessKey))])
		}
		return "sts-hiclaw"
	case client.AuthTypeNone:
		return "none (public access)"
	default:
		return t.client.AuthType
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// namespace shows or switches namespace
func (t *Terminal) namespace(args []string) {
	if len(args) == 0 {
		// Show current namespace
		fmt.Printf("Current Namespace: %s\n", t.client.Namespace)
		return
	}

	// Switch namespace
	oldNs := t.client.Namespace
	t.client.Namespace = args[0]

	fmt.Printf("Switched namespace from '%s' to '%s'\n", oldNs, t.client.Namespace)
}

// listSkills lists all skills
func (t *Terminal) listSkills(args []string) {
	// Parse flags
	var name string
	var page, size int = 1, 20

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--name=") {
			name = strings.TrimPrefix(arg, "--name=")
		} else if arg == "--name" && i+1 < len(args) {
			i++
			name = args[i]
		} else if arg == "--name=" && i+1 < len(args) {
			i++
			name = args[i]
		} else if strings.HasPrefix(arg, "--page=") {
			value := strings.TrimPrefix(arg, "--page=")
			if value != "" {
				_, _ = fmt.Sscanf(value, "%d", &page)
			}
		} else if arg == "--page" && i+1 < len(args) {
			i++
			_, _ = fmt.Sscanf(args[i], "%d", &page)
		} else if arg == "--page=" && i+1 < len(args) {
			i++
			_, _ = fmt.Sscanf(args[i], "%d", &page)
		} else if strings.HasPrefix(arg, "--size=") {
			value := strings.TrimPrefix(arg, "--size=")
			if value != "" {
				_, _ = fmt.Sscanf(value, "%d", &size)
			}
		} else if arg == "--size" && i+1 < len(args) {
			i++
			_, _ = fmt.Sscanf(args[i], "%d", &size)
		} else if arg == "--size=" && i+1 < len(args) {
			i++
			_, _ = fmt.Sscanf(args[i], "%d", &size)
		}
	}

	fmt.Print("\033[90mFetching skills...\033[0m\r")

	skills, totalCount, err := t.skillService.ListSkills(name, page, size)
	if err != nil {
		fmt.Printf("\033[31mError:\033[0m %v\n", err)
		return
	}

	fmt.Print("\033[K") // Clear line

	if len(skills) == 0 {
		totalPages := (totalCount + size - 1) / size
		if totalPages == 0 {
			fmt.Println("\033[33mNo skills found\033[0m")
		} else {
			fmt.Printf("\033[33mPage %d is out of range\033[0m \033[90m(Total: %d items, Total pages: %d)\033[0m\n", page, totalCount, totalPages)
		}
		return
	}

	fmt.Printf("\n\033[1;36mSkill List\033[0m \033[90m(Page: %d/%d, Total: %d)\033[0m\n", page, (totalCount+size-1)/size, totalCount)
	fmt.Println("\033[36m═══════════════════════════════════════════════════════════════════════════════\033[0m")
	for i, s := range skills {
		t.printSkillSummaryLine((page-1)*size+i+1, s)
	}
}

// printSkillSummaryLine renders one skill with governance info (colorized).
func (t *Terminal) printSkillSummaryLine(idx int, s skill.SkillListItem) {
	if s.Description != "" {
		desc := truncateDesc(s.Description, defaultDescLimit)
		fmt.Printf("\033[90m%3d.\033[0m \033[32m%s\033[0m \033[90m- %s\033[0m\n", idx, s.Name, desc)
	} else {
		fmt.Printf("\033[90m%3d.\033[0m \033[32m%s\033[0m\n", idx, s.Name)
	}

	latest := dashIfEmptyTerm(s.Labels["latest"])
	editing := dashIfEmptyTerm(s.EditingVersion)
	reviewing := dashIfEmptyTerm(s.ReviewingVersion)
	onlineCnt := "-"
	if s.OnlineCnt != nil {
		onlineCnt = fmt.Sprintf("%d", *s.OnlineCnt)
	}
	statusLabel := "\033[32menabled\033[0m"
	if !s.Enable {
		statusLabel = "\033[31mdisabled\033[0m"
	}
	fmt.Printf("     \033[90mlatest=\033[0m%s  \033[90mediting=\033[0m%s  \033[90mreviewing=\033[0m%s  \033[90monline=\033[0m%s  %s\n",
		latest, editing, reviewing, onlineCnt, statusLabel)

	var meta []string
	if s.Scope != "" {
		meta = append(meta, "scope="+s.Scope)
	}
	if s.BizTags != "" {
		meta = append(meta, "bizTags="+s.BizTags)
	}
	if s.Owner != "" {
		meta = append(meta, "owner="+s.Owner)
	}
	if s.UpdateTime != nil && *s.UpdateTime > 0 {
		meta = append(meta, "updated="+time.UnixMilli(*s.UpdateTime).Format("2006-01-02 15:04:05"))
	}
	if s.DownloadCount != nil && *s.DownloadCount > 0 {
		meta = append(meta, fmt.Sprintf("downloads=%d", *s.DownloadCount))
	}
	if len(meta) > 0 {
		fmt.Printf("     \033[90m%s\033[0m\n", strings.Join(meta, "  "))
	}
}

func dashIfEmptyTerm(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

// describeSkill fetches and prints a skill's detail including version list.
func (t *Terminal) describeSkill(args []string) {
	if len(args) == 0 {
		fmt.Println("\033[31mUsage:\033[0m skill-describe <skillName>")
		return
	}
	skillName := args[0]

	fmt.Print("\033[90mFetching skill detail...\033[0m\r")
	detail, err := t.skillService.DescribeSkill(skillName)
	fmt.Print("\033[K")
	if err != nil {
		fmt.Printf("\033[31mError:\033[0m %v\n", err)
		return
	}

	fmt.Printf("\n\033[1;36mSkill:\033[0m \033[32m%s\033[0m\n", detail.Name)
	fmt.Println("\033[36m═══════════════════════════════════════════════════════════════════════════════\033[0m")
	if detail.Description != "" {
		fmt.Printf("  \033[90mdescription:\033[0m %s\n", detail.Description)
	}

	statusLabel := "\033[32menabled\033[0m"
	if !detail.Enable {
		statusLabel = "\033[31mdisabled\033[0m"
	}
	onlineCnt := "-"
	if detail.OnlineCnt != nil {
		onlineCnt = fmt.Sprintf("%d", *detail.OnlineCnt)
	}
	fmt.Printf("  \033[90mlatest=\033[0m%s  \033[90mediting=\033[0m%s  \033[90mreviewing=\033[0m%s  \033[90monline=\033[0m%s  %s\n",
		dashIfEmptyTerm(detail.Labels["latest"]),
		dashIfEmptyTerm(detail.EditingVersion),
		dashIfEmptyTerm(detail.ReviewingVersion),
		onlineCnt,
		statusLabel,
	)

	var meta []string
	if detail.Scope != "" {
		meta = append(meta, "scope="+detail.Scope)
	}
	if detail.BizTags != "" {
		meta = append(meta, "bizTags="+detail.BizTags)
	}
	if detail.Owner != "" {
		meta = append(meta, "owner="+detail.Owner)
	}
	if detail.UpdateTime != nil && *detail.UpdateTime > 0 {
		meta = append(meta, "updated="+time.UnixMilli(*detail.UpdateTime).Format("2006-01-02 15:04:05"))
	}
	if detail.DownloadCount != nil && *detail.DownloadCount > 0 {
		meta = append(meta, fmt.Sprintf("downloads=%d", *detail.DownloadCount))
	}
	if len(meta) > 0 {
		fmt.Printf("  \033[90m%s\033[0m\n", strings.Join(meta, "  "))
	}

	fmt.Println()
	fmt.Println("\033[1;33mVersions:\033[0m")
	if len(detail.Versions) == 0 {
		fmt.Println("  \033[90m(none)\033[0m")
		return
	}

	versions := make([]skill.SkillVersionSummary, len(detail.Versions))
	copy(versions, detail.Versions)
	sort.SliceStable(versions, func(i, j int) bool {
		ti := termVersionSortKey(versions[i])
		tj := termVersionSortKey(versions[j])
		if ti != tj {
			return ti > tj
		}
		return versions[i].Version > versions[j].Version
	})

	fmt.Printf("  \033[90m%-12s  %-10s  %-12s  %-19s  %s\033[0m\n", "VERSION", "STATUS", "AUTHOR", "UPDATED", "COMMIT")
	for _, v := range versions {
		updated := "-"
		if v.UpdateTime != nil && *v.UpdateTime > 0 {
			updated = time.UnixMilli(*v.UpdateTime).Format("2006-01-02 15:04:05")
		}
		commit := strings.ReplaceAll(v.CommitMsg, "\n", " ")
		commit = truncateDesc(commit, 60)
		fmt.Printf("  %-12s  %-10s  %-12s  %-19s  %s\n",
			v.Version,
			dashIfEmptyTerm(v.Status),
			dashIfEmptyTerm(v.Author),
			updated,
			commit,
		)
	}
}

func termVersionSortKey(v skill.SkillVersionSummary) int64 {
	if v.UpdateTime != nil {
		return *v.UpdateTime
	}
	if v.CreateTime != nil {
		return *v.CreateTime
	}
	return 0
}

// getSkill downloads one or more skills
func (t *Terminal) getSkill(args []string) {
	if len(args) == 0 {
		fmt.Println("\033[31mUsage:\033[0m skill-get <skillName> [skillName2...]")
		return
	}

	// Parse flags from args
	var skillNames []string
	var outputDir string
	var version, label string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--version" && i+1 < len(args) {
			i++
			version = args[i]
		} else if strings.HasPrefix(arg, "--version=") {
			version = strings.TrimPrefix(arg, "--version=")
		} else if arg == "-o" && i+1 < len(args) {
			i++
			outputDir = args[i]
		} else if strings.HasPrefix(arg, "-o=") {
			outputDir = strings.TrimPrefix(arg, "-o=")
		} else if arg == "--label" && i+1 < len(args) {
			i++
			label = args[i]
		} else if strings.HasPrefix(arg, "--label=") {
			label = strings.TrimPrefix(arg, "--label=")
		} else if strings.HasPrefix(arg, "-") {
			// Unknown flag, skip
			continue
		} else {
			// Positional argument (skill name)
			skillNames = append(skillNames, arg)
		}
	}

	if len(skillNames) == 0 {
		fmt.Println("\033[31mError:\033[0m no skill names specified")
		return
	}

	// Default output directory
	if outputDir == "" {
		homeDir, homeErr := os.UserHomeDir()
		if homeErr != nil {
			fmt.Printf("\033[31mError:\033[0m %v\n", homeErr)
			return
		}
		outputDir = filepath.Join(homeDir, ".skills")
	} else {
		// Expand ~ to home directory
		if strings.HasPrefix(outputDir, "~/") {
			homeDir, homeErr := os.UserHomeDir()
			if homeErr != nil {
				fmt.Printf("\033[31mError:\033[0m %v\n", homeErr)
				return
			}
			outputDir = filepath.Join(homeDir, outputDir[2:])
		} else if outputDir == "~" {
			homeDir, homeErr := os.UserHomeDir()
			if homeErr != nil {
				fmt.Printf("\033[31mError:\033[0m %v\n", homeErr)
				return
			}
			outputDir = homeDir
		}
	}

	// Track results
	var successCount, failCount int
	var failedSkills []string
	var err error

	// Process each skill
	for i, skillName := range skillNames {
		if len(skillNames) > 1 {
			fmt.Printf("\n\033[90m[%d/%d] \033[0m", i+1, len(skillNames))
		}
		fmt.Printf("\033[90mDownloading skill: \033[33m%s\033[90m...\033[0m\n", skillName)

		err = t.skillService.GetSkill(skillName, outputDir, version, label)
		if err != nil {
			fmt.Printf("\033[31mError:\033[0m failed to download skill '%s': %v\n", skillName, err)
			failCount++
			failedSkills = append(failedSkills, skillName)
		} else {
			fmt.Printf("\033[32mSkill downloaded successfully!\033[0m\n")
			fmt.Printf("  \033[90mLocation:\033[0m %s/%s\n", outputDir, skillName)
			successCount++
		}
	}

	// Summary for multiple skills
	if len(skillNames) > 1 {
		fmt.Println()
		fmt.Println("\033[36m========== Summary ==========\033[0m")
		fmt.Printf("Total: %d | \033[32mSuccess:\033[0m %d | \033[31mFailed:\033[0m %d\n", len(skillNames), successCount, failCount)
		if failCount > 0 {
			fmt.Printf("Failed skills: \033[31m%s\033[0m\n", strings.Join(failedSkills, ", "))
		}
	}
}

// uploadSkill uploads a skill draft (editing state)
func (t *Terminal) uploadSkill(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: skill-upload <skillPath> [--overwrite true|false] or skill-upload --all <folder> [--overwrite true|false]")
		return
	}

	args, overwrite, err := parseSkillUploadOverwrite(args)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	if len(args) == 0 {
		fmt.Println("Usage: skill-upload <skillPath> [--overwrite true|false] or skill-upload --all <folder> [--overwrite true|false]")
		return
	}

	// Check for --all flag in any position
	allFlagIndex := -1
	folderPath := ""
	for i, arg := range args {
		if arg == "--all" {
			allFlagIndex = i
			// Get folder path from next argument or previous argument
			if i+1 < len(args) {
				folderPath = args[i+1]
			}
			break
		}
	}

	// If --all found but no folder after it, check if folder is before --all
	if allFlagIndex >= 0 && folderPath == "" {
		if allFlagIndex > 0 {
			folderPath = args[allFlagIndex-1]
		}
	}

	if allFlagIndex >= 0 {
		if folderPath == "" {
			fmt.Println("Error: folder path required for --all flag")
			fmt.Println("Usage: skill-upload --all <folder> [--overwrite true|false] or skill-upload <folder> --all [--overwrite true|false]")
			return
		}
		t.uploadAllSkills(folderPath, overwrite)
		return
	}

	// Single skill upload
	skillPath := args[0]

	// Expand ~ to home directory
	if strings.HasPrefix(skillPath, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Error getting home directory: %v\n", err)
			return
		}
		skillPath = filepath.Join(homeDir, skillPath[2:])
	} else if skillPath == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Error getting home directory: %v\n", err)
			return
		}
		skillPath = homeDir
	}

	fmt.Printf("Uploading skill: %s...\n", skillPath)

	err = t.skillService.UploadSkill(skillPath, overwrite)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Skill draft uploaded successfully!\n")
	fmt.Printf("Tip: Use 'skill-review %s' to submit the draft for review.\n", filepath.Base(skillPath))
}

func parseSkillUploadOverwrite(args []string) ([]string, bool, error) {
	overwrite := false
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		value := ""
		if arg == "--overwrite" {
			if i+1 >= len(args) {
				return nil, false, fmt.Errorf("--overwrite must be true or false")
			}
			i++
			value = args[i]
		} else if strings.HasPrefix(arg, "--overwrite=") {
			value = strings.TrimPrefix(arg, "--overwrite=")
		} else {
			filtered = append(filtered, arg)
			continue
		}

		switch value {
		case "false":
			overwrite = false
		case "true":
			overwrite = true
		default:
			return nil, false, fmt.Errorf("--overwrite must be true or false")
		}
	}
	return filtered, overwrite, nil
}

// uploadAllSkills publishes all skill drafts in a directory
func (t *Terminal) uploadAllSkills(folderPath string, overwrite bool) {
	// Expand ~ to home directory
	if strings.HasPrefix(folderPath, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Error getting home directory: %v\n", err)
			return
		}
		folderPath = filepath.Join(homeDir, folderPath[2:])
	} else if folderPath == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Error getting home directory: %v\n", err)
			return
		}
		folderPath = homeDir
	}

	// List subdirectories
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		fmt.Printf("Error reading directory: %v\n", err)
		return
	}

	var skillDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Check if SKILL.md exists
		skillMDPath := filepath.Join(folderPath, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillMDPath); err == nil {
			skillDirs = append(skillDirs, entry.Name())
		}
	}

	if len(skillDirs) == 0 {
		fmt.Println("No skills found (directories with SKILL.md)")
		return
	}

	fmt.Printf("Found %d skills:\n", len(skillDirs))
	for _, name := range skillDirs {
		fmt.Printf("  - %s\n", name)
	}
	fmt.Println()

	successCount := 0
	failedCount := 0

	for i, skillName := range skillDirs {
		fmt.Println(strings.Repeat("=", 80))
		fmt.Printf("[%d/%d] Publishing skill: %s\n", i+1, len(skillDirs), skillName)
		fmt.Println(strings.Repeat("=", 80))

		skillPath := filepath.Join(folderPath, skillName)
		err := t.skillService.UploadSkill(skillPath, overwrite)
		if err != nil {
			fmt.Printf("Publish failed: %v\n", err)
			failedCount++
		} else {
			fmt.Printf("Publish successful!\n")
			successCount++
		}
		fmt.Println()
	}

	// Summary
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("Batch Publish Complete")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Success: %d\n", successCount)
	if failedCount > 0 {
		fmt.Printf("Failed: %d\n", failedCount)
	}
	fmt.Printf("Total: %d\n", len(skillDirs))
	fmt.Println()
	fmt.Println("Tip: Use 'skill-review <skillName>' to submit a draft for review.")
}

// submitSkill submits a skill draft for review (editing -> reviewing)
func (t *Terminal) submitSkill(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: skill-review <skillName> [--version <version>]")
		return
	}

	skillName := args[0]
	version := ""
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--version" && i+1 < len(args) {
			i++
			version = args[i]
		} else if strings.HasPrefix(arg, "--version=") {
			version = strings.TrimPrefix(arg, "--version=")
		}
	}

	fmt.Printf("Submitting skill for review: %s...\n", skillName)
	if err := t.skillService.SubmitSkill(skillName, version); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Skill submitted for review successfully!\n")
	fmt.Printf("Tip: After the review passes, run 'skill-release %s --version <ver>' to publish it online.\n", skillName)
}

// releaseSkill publishes an approved skill version (reviewing -> online)
func (t *Terminal) releaseSkill(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: skill-release <skillName> --version <version> [--update-latest=true|false]")
		return
	}

	skillName := args[0]
	version := ""
	updateLatest := true
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--version" && i+1 < len(args) {
			i++
			version = args[i]
		} else if strings.HasPrefix(arg, "--version=") {
			version = strings.TrimPrefix(arg, "--version=")
		} else if arg == "--update-latest=false" || arg == "--update-latest=False" {
			updateLatest = false
		} else if arg == "--update-latest=true" || arg == "--update-latest=True" {
			updateLatest = true
		} else if arg == "--update-latest" && i+1 < len(args) {
			i++
			updateLatest = args[i] != "false" && args[i] != "False" && args[i] != "0"
		}
	}

	if version == "" {
		fmt.Println("Error: --version is required for skill-release")
		return
	}

	fmt.Printf("Releasing skill: %s@%s (updateLatest=%v)...\n", skillName, version, updateLatest)
	if err := t.skillService.PublishSkill(skillName, version, updateLatest); err != nil {
		fmt.Printf("Error: %v\n", err)
		maybePrintReleaseRetryHintTerm(err, "skill", skillName)
		return
	}
	fmt.Printf("Skill released successfully! %s@%s is now online.\n", skillName, version)
}

// updateSkillScope sets skill visibility scope.
func (t *Terminal) updateSkillScope(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: skill-scope <skillName> --scope <PUBLIC|PRIVATE>")
		return
	}

	skillName := args[0]
	scope := ""
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--scope" && i+1 < len(args) {
			i++
			scope = args[i]
		} else if strings.HasPrefix(arg, "--scope=") {
			scope = strings.TrimPrefix(arg, "--scope=")
		}
	}
	if scope == "" {
		fmt.Println("Error: --scope is required for skill-scope")
		return
	}

	if err := t.skillService.UpdateSkillScope(skillName, scope); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Skill scope updated successfully: %s -> %s\n", skillName, scope)
}

// updateSkillTags sets skill metadata tags.
func (t *Terminal) updateSkillTags(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: skill-tags <skillName> --tags <tags>")
		return
	}

	skillName := args[0]
	tags := ""
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--tags" && i+1 < len(args) {
			i++
			tags = args[i]
		} else if strings.HasPrefix(arg, "--tags=") {
			tags = strings.TrimPrefix(arg, "--tags=")
		}
	}
	if tags == "" {
		fmt.Println("Error: --tags is required for skill-tags")
		return
	}

	if err := t.skillService.UpdateSkillBizTags(skillName, tags); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Skill tags updated successfully: %s -> %s\n", skillName, tags)
}

// publishLegacy runs upload + review as a backward-compatible shortcut for the
// deprecated skill-publish command.
func (t *Terminal) publishLegacy(args []string) {
	fmt.Println("\033[33m[DEPRECATED] 'skill-publish' will be removed in a future release.\033[0m")
	fmt.Println("\033[90m  It now runs 'skill-upload' + 'skill-review' for compatibility.\033[0m")
	fmt.Println("\033[90m  Please migrate to: skill-upload -> skill-review -> skill-release.\033[0m")

	if len(args) == 0 {
		fmt.Println("Usage: skill-publish <skillPath> or skill-publish --all <folder>")
		return
	}

	// Reuse upload logic; on success, automatically submit for review.
	// For simplicity in interactive mode, batch --all is handled by uploadSkill,
	// which only performs upload; we then iterate submit using the folder entries.
	hasAll := false
	folderPath := ""
	for i, a := range args {
		if a == "--all" {
			hasAll = true
			if i+1 < len(args) {
				folderPath = args[i+1]
			} else if i > 0 {
				folderPath = args[i-1]
			}
			break
		}
	}

	if hasAll {
		if folderPath == "" {
			fmt.Println("Error: folder path required for --all flag")
			return
		}
		// Upload all first, then submit all drafts for review.
		t.uploadAllSkills(folderPath, false)
		t.reviewAllSkills(folderPath)
		return
	}

	skillPath := args[0]
	if strings.HasPrefix(skillPath, "~/") {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			skillPath = filepath.Join(homeDir, skillPath[2:])
		}
	} else if skillPath == "~" {
		if homeDir, err := os.UserHomeDir(); err == nil {
			skillPath = homeDir
		}
	}

	skillName := filepath.Base(skillPath)
	if strings.HasSuffix(strings.ToLower(skillName), ".zip") {
		skillName = strings.TrimSuffix(skillName, filepath.Ext(skillName))
	}

	fmt.Printf("[1/2] Uploading skill: %s...\n", skillPath)
	if err := t.skillService.UploadSkill(skillPath, false); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Upload successful.\n")

	fmt.Printf("[2/2] Submitting skill for review: %s...\n", skillName)
	if err := t.skillService.SubmitSkill(skillName, ""); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Submitted for review successfully.\n")
	fmt.Printf("Tip: After the review passes, run 'skill-release %s --version <ver>' to publish online.\n", skillName)
}

// reviewAllSkills submits every skill directory (with SKILL.md) under folderPath for review.
func (t *Terminal) reviewAllSkills(folderPath string) {
	if strings.HasPrefix(folderPath, "~/") {
		if homeDir, err := os.UserHomeDir(); err == nil {
			folderPath = filepath.Join(homeDir, folderPath[2:])
		}
	}
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		fmt.Printf("Error reading folder: %v\n", err)
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(folderPath, entry.Name(), "SKILL.md")); err != nil {
			continue
		}
		skillName := entry.Name()
		fmt.Printf("Submitting skill for review: %s...\n", skillName)
		if err := t.skillService.SubmitSkill(skillName, ""); err != nil {
			fmt.Printf("  Failed: %v\n", err)
			continue
		}
		fmt.Printf("  OK\n")
	}
}

// listConfigs lists all configurations
func (t *Terminal) listConfigs(args []string) {
	// Parse flags
	var dataID, group string
	var page, size int = 1, 20

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--data-id=") {
			dataID = strings.TrimPrefix(arg, "--data-id=")
		} else if arg == "--data-id" && i+1 < len(args) {
			i++
			dataID = args[i]
		} else if arg == "--data-id=" && i+1 < len(args) {
			i++
			dataID = args[i]
		} else if strings.HasPrefix(arg, "--group=") {
			group = strings.TrimPrefix(arg, "--group=")
		} else if arg == "--group" && i+1 < len(args) {
			i++
			group = args[i]
		} else if arg == "--group=" && i+1 < len(args) {
			i++
			group = args[i]
		} else if strings.HasPrefix(arg, "--page=") {
			value := strings.TrimPrefix(arg, "--page=")
			if value != "" {
				_, _ = fmt.Sscanf(value, "%d", &page)
			}
		} else if arg == "--page" && i+1 < len(args) {
			i++
			_, _ = fmt.Sscanf(args[i], "%d", &page)
		} else if arg == "--page=" && i+1 < len(args) {
			i++
			_, _ = fmt.Sscanf(args[i], "%d", &page)
		} else if strings.HasPrefix(arg, "--size=") {
			value := strings.TrimPrefix(arg, "--size=")
			if value != "" {
				_, _ = fmt.Sscanf(value, "%d", &size)
			}
		} else if arg == "--size" && i+1 < len(args) {
			i++
			_, _ = fmt.Sscanf(args[i], "%d", &size)
		} else if arg == "--size=" && i+1 < len(args) {
			i++
			_, _ = fmt.Sscanf(args[i], "%d", &size)
		}
	}

	fmt.Print("\033[90mFetching configurations...\033[0m\r")

	configs, err := t.client.ListConfigs(dataID, group, "", page, size)
	if err != nil {
		fmt.Printf("\033[31mError:\033[0m %v\n", err)
		return
	}

	fmt.Print("\033[K") // Clear line

	if len(configs.PageItems) == 0 {
		totalPages := (configs.TotalCount + size - 1) / size
		if totalPages == 0 {
			fmt.Println("\033[33mNo configurations found\033[0m")
		} else {
			fmt.Printf("\033[33mPage %d is out of range\033[0m \033[90m(Total: %d items, Total pages: %d)\033[0m\n", page, configs.TotalCount, totalPages)
		}
		return
	}

	fmt.Printf("\n\033[1;36mConfiguration List\033[0m \033[90m(Page: %d/%d, Total: %d)\033[0m\n", page, (configs.TotalCount+size-1)/size, configs.TotalCount)
	fmt.Println("\033[36m═══════════════════════════════════════════════════════════════\033[0m")
	fmt.Printf("\033[90m%-5s %-30s %-20s %-10s\033[0m\n", "No.", "Data ID", "Group", "Type")
	fmt.Println("\033[90m───────────────────────────────────────────────────────────────\033[0m")

	for i, config := range configs.PageItems {
		groupName := config.GroupName
		if groupName == "" {
			groupName = config.Group
		}

		dataID := config.DataID
		if len(dataID) > 28 {
			dataID = dataID[:25] + "..."
		}

		if len(groupName) > 18 {
			groupName = groupName[:15] + "..."
		}

		fmt.Printf("%-5d \033[32m%-30s\033[0m \033[33m%-20s\033[0m \033[90m%-10s\033[0m\n",
			(page-1)*size+i+1, dataID, groupName, config.Type)
	}
}

// setConfig publishes a configuration (interactive mode: requires --file/-f)
func (t *Terminal) setConfig(args []string) {
	var dataID, group, filePath string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-f" || arg == "--file" {
			if i+1 < len(args) {
				i++
				filePath = args[i]
			}
			continue
		}
		if dataID == "" {
			dataID = arg
		} else if group == "" {
			group = arg
		}
	}

	if dataID == "" || group == "" {
		fmt.Println("\033[31mUsage:\033[0m config-set <data-id> <group> [-f <file>]")
		fmt.Println("\033[90mWithout -f: enter content in next lines, empty line to finish.\033[0m")
		return
	}

	var content string
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("\033[31mError:\033[0m read file %s: %v\n", filePath, err)
			return
		}
		content = string(data)
	} else {
		// Read content from terminal: multi-line until empty line or single "."
		fmt.Println("\033[90mEnter config content. Finish with a blank line or a single dot line.\033[0m")
		fmt.Println("\033[90m  (Type your content, then press Enter, then press Enter again — or type \".\" and Enter)\033[0m")
		var lines []string
		for {
			line, err := t.rl.Readline()
			if err == readline.ErrInterrupt {
				fmt.Println("\033[33mCancelled\033[0m")
				return
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				fmt.Printf("\033[31mError:\033[0m %v\n", err)
				return
			}
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || trimmed == "." {
				break
			}
			lines = append(lines, line)
		}
		content = strings.Join(lines, "\n")
	}

	if content == "" {
		fmt.Println("\033[31mError:\033[0m config content is empty (use -f <file> or type content)")
		return
	}

	fmt.Printf("\033[90mPublishing config: \033[33m%s\033[90m (\033[33m%s\033[90m)...\033[0m\n", dataID, group)
	if err := t.client.PublishConfig(dataID, group, content); err != nil {
		fmt.Printf("\033[31mError:\033[0m %v\n", err)
		return
	}
	fmt.Println("\033[32mConfiguration published successfully\033[0m")
}

// getConfig gets configuration content
func (t *Terminal) getConfig(args []string) {
	if len(args) < 2 {
		fmt.Println("\033[31mUsage:\033[0m config-get <data-id> <group>")
		return
	}

	dataID := args[0]
	group := args[1]

	fmt.Printf("\033[90mFetching config: \033[33m%s\033[90m (\033[33m%s\033[90m)...\033[0m\n\n", dataID, group)

	content, err := t.client.GetConfig(dataID, group)
	if err != nil {
		fmt.Printf("\033[31mError:\033[0m %v\n", err)
		return
	}

	if content == "" {
		fmt.Println("\033[33mConfiguration not found\033[0m")
		return
	}

	fmt.Println("\033[36m═══════════════════════════════════════\033[0m")
	fmt.Printf("\033[33mData ID:\033[0m %s\n", dataID)
	fmt.Printf("\033[33mGroup:\033[0m %s\n", group)
	fmt.Println("\033[36m═══════════════════════════════════════\033[0m")
	fmt.Println(content)
}

// Command help methods

func (t *Terminal) showSkillListHelp() {
	help.SkillList.FormatForTerminal()
}

func (t *Terminal) showSkillDescribeHelp() {
	help.SkillDescribe.FormatForTerminal()
}

func (t *Terminal) showSkillGetHelp() {
	help.SkillGet.FormatForTerminal()
}

func (t *Terminal) showSkillPublishHelp() {
	help.SkillPublish.FormatForTerminal()
}

func (t *Terminal) showSkillUploadHelp() {
	help.SkillUpload.FormatForTerminal()
}

func (t *Terminal) showSkillReviewHelp() {
	help.SkillReview.FormatForTerminal()
}

func (t *Terminal) showSkillReleaseHelp() {
	help.SkillRelease.FormatForTerminal()
}

func (t *Terminal) showSkillScopeHelp() {
	help.SkillScope.FormatForTerminal()
}

func (t *Terminal) showSkillTagsHelp() {
	help.SkillTags.FormatForTerminal()
}

func (t *Terminal) showConfigListHelp() {
	help.ConfigList.FormatForTerminal()
}

func (t *Terminal) showConfigGetHelp() {
	help.ConfigGet.FormatForTerminal()
}

func (t *Terminal) showConfigSetHelp() {
	help.ConfigSet.FormatForTerminal()
}

// AgentSpec command help methods

func (t *Terminal) showAgentSpecListHelp() {
	help.AgentSpecList.FormatForTerminal()
}

func (t *Terminal) showAgentSpecDescribeHelp() {
	help.AgentSpecDescribe.FormatForTerminal()
}

func (t *Terminal) showAgentSpecGetHelp() {
	help.AgentSpecGet.FormatForTerminal()
}

func (t *Terminal) showAgentSpecUploadHelp() {
	help.AgentSpecUpload.FormatForTerminal()
}

func (t *Terminal) showAgentSpecReviewHelp() {
	help.AgentSpecReview.FormatForTerminal()
}

func (t *Terminal) showAgentSpecReleaseHelp() {
	help.AgentSpecRelease.FormatForTerminal()
}

func (t *Terminal) showAgentSpecPublishHelp() {
	help.AgentSpecPublish.FormatForTerminal()
}

// listAgentSpecs lists all agent specs
func (t *Terminal) listAgentSpecs(args []string) {
	// Parse flags
	var name string
	var page, size int = 1, 20
	output := "pretty"

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case strings.HasPrefix(arg, "--name="):
			name = strings.TrimPrefix(arg, "--name=")
		case arg == "--name" && i+1 < len(args):
			i++
			name = args[i]
		case strings.HasPrefix(arg, "--page="):
			v := strings.TrimPrefix(arg, "--page=")
			if v != "" {
				_, _ = fmt.Sscanf(v, "%d", &page)
			}
		case arg == "--page" && i+1 < len(args):
			i++
			_, _ = fmt.Sscanf(args[i], "%d", &page)
		case strings.HasPrefix(arg, "--size="):
			v := strings.TrimPrefix(arg, "--size=")
			if v != "" {
				_, _ = fmt.Sscanf(v, "%d", &size)
			}
		case arg == "--size" && i+1 < len(args):
			i++
			_, _ = fmt.Sscanf(args[i], "%d", &size)
		case strings.HasPrefix(arg, "--output="):
			output = strings.TrimPrefix(arg, "--output=")
		case arg == "--output" && i+1 < len(args):
			i++
			output = args[i]
		case arg == "-o" && i+1 < len(args):
			i++
			output = args[i]
		}
	}

	if output != "pretty" && output != "json" {
		fmt.Printf("\033[31mError:\033[0m unsupported output format %q (expected pretty or json)\n", output)
		return
	}

	if output == "pretty" {
		fmt.Print("\033[90mFetching agent specs...\033[0m\r")
	}

	specs, totalCount, err := t.agentSpecService.ListAgentSpecs(name, "", page, size)
	if output == "pretty" {
		fmt.Print("\033[K") // Clear line
	}
	if err != nil {
		fmt.Printf("\033[31mError:\033[0m %v\n", err)
		return
	}

	if output == "json" {
		payload := map[string]interface{}{
			"totalCount": totalCount,
			"page":       page,
			"size":       size,
			"items":      specs,
		}
		b, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(b))
		return
	}

	if len(specs) == 0 {
		totalPages := (totalCount + size - 1) / size
		if totalPages == 0 {
			fmt.Println("\033[33mNo agent specs found\033[0m")
		} else {
			fmt.Printf("\033[33mPage %d is out of range\033[0m \033[90m(Total: %d items, Total pages: %d)\033[0m\n", page, totalCount, totalPages)
		}
		return
	}

	fmt.Printf("\n\033[1;36mAgentSpec List\033[0m \033[90m(Page: %d/%d, Total: %d)\033[0m\n", page, (totalCount+size-1)/size, totalCount)
	fmt.Println("\033[36m═══════════════════════════════════════════════════════════════════════════════\033[0m")
	for i, s := range specs {
		t.printAgentSpecSummaryLine((page-1)*size+i+1, s)
	}
}

// printAgentSpecSummaryLine renders one agent spec with governance info (colorized).
func (t *Terminal) printAgentSpecSummaryLine(idx int, s agentspec.AgentSpecListItem) {
	desc := strOrEmptyTerm(s.Description)
	if desc != "" {
		fmt.Printf("\033[90m%3d.\033[0m \033[32m%s\033[0m \033[90m- %s\033[0m\n", idx, s.Name, truncateDesc(desc, defaultDescLimit))
	} else {
		fmt.Printf("\033[90m%3d.\033[0m \033[32m%s\033[0m\n", idx, s.Name)
	}

	latest := dashIfEmptyTerm(s.Labels["latest"])
	editing := dashIfEmptyTerm(strOrEmptyTerm(s.EditingVersion))
	reviewing := dashIfEmptyTerm(strOrEmptyTerm(s.ReviewingVersion))
	statusLabel := "\033[32menabled\033[0m"
	if !s.Enable {
		statusLabel = "\033[31mdisabled\033[0m"
	}
	fmt.Printf("     \033[90mlatest=\033[0m%s  \033[90mediting=\033[0m%s  \033[90mreviewing=\033[0m%s  \033[90monline=\033[0m%d  %s\n",
		latest, editing, reviewing, s.OnlineCnt, statusLabel)

	var meta []string
	if scope := strOrEmptyTerm(s.Scope); scope != "" {
		meta = append(meta, "scope="+scope)
	}
	if biz := strOrEmptyTerm(s.BizTags); biz != "" {
		meta = append(meta, "bizTags="+biz)
	}
	if s.UpdateTime > 0 {
		meta = append(meta, "updated="+time.UnixMilli(s.UpdateTime).Format("2006-01-02 15:04:05"))
	}
	if s.DownloadCount != nil && *s.DownloadCount > 0 {
		meta = append(meta, fmt.Sprintf("downloads=%d", *s.DownloadCount))
	}
	if len(meta) > 0 {
		fmt.Printf("     \033[90m%s\033[0m\n", strings.Join(meta, "  "))
	}
}

// strOrEmptyTerm dereferences a *string, returning "" if nil.
func strOrEmptyTerm(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// getAgentSpec downloads one or more agent specs
func (t *Terminal) getAgentSpec(args []string) {
	if len(args) == 0 {
		fmt.Println("\033[31mUsage:\033[0m agentspec-get <name> [name2...]")
		return
	}

	// Parse flags from args
	var specNames []string
	var outputDir string
	var version, label string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--version" && i+1 < len(args) {
			i++
			version = args[i]
		} else if strings.HasPrefix(arg, "--version=") {
			version = strings.TrimPrefix(arg, "--version=")
		} else if arg == "-o" && i+1 < len(args) {
			i++
			outputDir = args[i]
		} else if strings.HasPrefix(arg, "-o=") {
			outputDir = strings.TrimPrefix(arg, "-o=")
		} else if arg == "--label" && i+1 < len(args) {
			i++
			label = args[i]
		} else if strings.HasPrefix(arg, "--label=") {
			label = strings.TrimPrefix(arg, "--label=")
		} else if strings.HasPrefix(arg, "-") {
			// Unknown flag, skip
			continue
		} else {
			// Positional argument (spec name)
			specNames = append(specNames, arg)
		}
	}

	if len(specNames) == 0 {
		fmt.Println("\033[31mError:\033[0m no agent spec names specified")
		return
	}

	// Default output directory
	if outputDir == "" {
		homeDir, homeErr := os.UserHomeDir()
		if homeErr != nil {
			fmt.Printf("\033[31mError:\033[0m %v\n", homeErr)
			return
		}
		outputDir = filepath.Join(homeDir, ".agentspecs")
	} else {
		// Expand ~ to home directory
		if strings.HasPrefix(outputDir, "~/") {
			homeDir, homeErr := os.UserHomeDir()
			if homeErr != nil {
				fmt.Printf("\033[31mError:\033[0m %v\n", homeErr)
				return
			}
			outputDir = filepath.Join(homeDir, outputDir[2:])
		} else if outputDir == "~" {
			homeDir, homeErr := os.UserHomeDir()
			if homeErr != nil {
				fmt.Printf("\033[31mError:\033[0m %v\n", homeErr)
				return
			}
			outputDir = homeDir
		}
	}

	// Track results
	var successCount, failCount int
	var failedSpecs []string
	var err error

	// Process each spec
	for i, specName := range specNames {
		if len(specNames) > 1 {
			fmt.Printf("\n\033[90m[%d/%d] \033[0m", i+1, len(specNames))
		}
		fmt.Printf("\033[90mDownloading agent spec: \033[33m%s\033[90m...\033[0m\n", specName)

		err = t.agentSpecService.GetAgentSpec(specName, outputDir, version, label)
		if err != nil {
			fmt.Printf("\033[31mError:\033[0m failed to download agent spec '%s': %v\n", specName, err)
			failCount++
			failedSpecs = append(failedSpecs, specName)
		} else {
			fmt.Printf("\033[32mAgent spec downloaded successfully!\033[0m\n")
			fmt.Printf("  \033[90mLocation:\033[0m %s/%s\n", outputDir, specName)
			successCount++
		}
	}

	// Summary for multiple specs
	if len(specNames) > 1 {
		fmt.Println()
		fmt.Println("\033[36m========== Summary ==========\033[0m")
		fmt.Printf("Total: %d | \033[32mSuccess:\033[0m %d | \033[31mFailed:\033[0m %d\n", len(specNames), successCount, failCount)
		if failCount > 0 {
			fmt.Printf("Failed agent specs: \033[31m%s\033[0m\n", strings.Join(failedSpecs, ", "))
		}
	}
}

// uploadAgentSpec uploads an agent spec draft (editing state)
func (t *Terminal) uploadAgentSpec(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: agentspec-upload <agentSpecPath> or agentspec-upload --all <folder>")
		return
	}

	allFlagIndex := -1
	folderPath := ""
	for i, arg := range args {
		if arg == "--all" {
			allFlagIndex = i
			if i+1 < len(args) {
				folderPath = args[i+1]
			}
			break
		}
	}
	if allFlagIndex >= 0 && folderPath == "" && allFlagIndex > 0 {
		folderPath = args[allFlagIndex-1]
	}

	if allFlagIndex >= 0 {
		if folderPath == "" {
			fmt.Println("Error: folder path required for --all flag")
			fmt.Println("Usage: agentspec-upload --all <folder> or agentspec-upload <folder> --all")
			return
		}
		t.uploadAllAgentSpecs(folderPath)
		return
	}

	specPath := args[0]
	if strings.HasPrefix(specPath, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Error getting home directory: %v\n", err)
			return
		}
		specPath = filepath.Join(homeDir, specPath[2:])
	} else if specPath == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Error getting home directory: %v\n", err)
			return
		}
		specPath = homeDir
	}

	fmt.Printf("Uploading agent spec: %s...\n", specPath)
	if err := t.agentSpecService.UploadAgentSpec(specPath); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	specName := filepath.Base(specPath)
	if strings.HasSuffix(strings.ToLower(specName), ".zip") {
		specName = strings.TrimSuffix(specName, filepath.Ext(specName))
	}
	fmt.Printf("Agent spec draft uploaded successfully!\n")
	fmt.Printf("Tip: Use 'agentspec-review %s' to submit the draft for review.\n", specName)
}

// uploadAllAgentSpecs uploads all agent spec drafts in a directory.
func (t *Terminal) uploadAllAgentSpecs(folderPath string) {
	if strings.HasPrefix(folderPath, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Error getting home directory: %v\n", err)
			return
		}
		folderPath = filepath.Join(homeDir, folderPath[2:])
	} else if folderPath == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Error getting home directory: %v\n", err)
			return
		}
		folderPath = homeDir
	}

	entries, err := os.ReadDir(folderPath)
	if err != nil {
		fmt.Printf("Error reading directory: %v\n", err)
		return
	}

	var specDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(folderPath, entry.Name(), "manifest.json")
		if _, err := os.Stat(manifestPath); err == nil {
			specDirs = append(specDirs, entry.Name())
		}
	}

	if len(specDirs) == 0 {
		fmt.Println("No agent specs found (directories with manifest.json)")
		return
	}

	fmt.Printf("Found %d agent specs:\n", len(specDirs))
	for _, name := range specDirs {
		fmt.Printf("  - %s\n", name)
	}
	fmt.Println()

	successCount, failedCount := 0, 0
	for i, specName := range specDirs {
		fmt.Println(strings.Repeat("=", 80))
		fmt.Printf("[%d/%d] Uploading agent spec: %s\n", i+1, len(specDirs), specName)
		fmt.Println(strings.Repeat("=", 80))

		specPath := filepath.Join(folderPath, specName)
		if err := t.agentSpecService.UploadAgentSpec(specPath); err != nil {
			fmt.Printf("Upload failed: %v\n", err)
			failedCount++
		} else {
			fmt.Printf("Upload successful!\n")
			successCount++
		}
		fmt.Println()
	}

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("Batch Upload Complete")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Success: %d\n", successCount)
	if failedCount > 0 {
		fmt.Printf("Failed: %d\n", failedCount)
	}
	fmt.Printf("Total: %d\n", len(specDirs))
	fmt.Println()
	fmt.Println("Tip: Use 'agentspec-review <agentSpecName>' to submit a draft for review.")
}

// describeAgentSpec fetches and prints one agent spec's detail including version list.
func (t *Terminal) describeAgentSpec(args []string) {
	if len(args) == 0 {
		fmt.Println("\033[31mUsage:\033[0m agentspec-describe <agentSpecName>")
		return
	}
	specName := args[0]

	fmt.Print("\033[90mFetching agent spec detail...\033[0m\r")
	detail, err := t.agentSpecService.DescribeAgentSpec(specName)
	fmt.Print("\033[K")
	if err != nil {
		fmt.Printf("\033[31mError:\033[0m %v\n", err)
		return
	}

	fmt.Printf("\n\033[1;36mAgentSpec:\033[0m \033[32m%s\033[0m\n", detail.Name)
	fmt.Println("\033[36m═══════════════════════════════════════════════════════════════════════════════\033[0m")
	if desc := strOrEmptyTerm(detail.Description); desc != "" {
		fmt.Printf("  \033[90mdescription:\033[0m %s\n", desc)
	}

	statusLabel := "\033[32menabled\033[0m"
	if !detail.Enable {
		statusLabel = "\033[31mdisabled\033[0m"
	}
	fmt.Printf("  \033[90mlatest=\033[0m%s  \033[90mediting=\033[0m%s  \033[90mreviewing=\033[0m%s  \033[90monline=\033[0m%d  %s\n",
		dashIfEmptyTerm(detail.Labels["latest"]),
		dashIfEmptyTerm(strOrEmptyTerm(detail.EditingVersion)),
		dashIfEmptyTerm(strOrEmptyTerm(detail.ReviewingVersion)),
		detail.OnlineCnt,
		statusLabel,
	)

	var meta []string
	if scope := strOrEmptyTerm(detail.Scope); scope != "" {
		meta = append(meta, "scope="+scope)
	}
	if biz := strOrEmptyTerm(detail.BizTags); biz != "" {
		meta = append(meta, "bizTags="+biz)
	}
	if detail.UpdateTime > 0 {
		meta = append(meta, "updated="+time.UnixMilli(detail.UpdateTime).Format("2006-01-02 15:04:05"))
	}
	if detail.DownloadCount != nil && *detail.DownloadCount > 0 {
		meta = append(meta, fmt.Sprintf("downloads=%d", *detail.DownloadCount))
	}
	if len(meta) > 0 {
		fmt.Printf("  \033[90m%s\033[0m\n", strings.Join(meta, "  "))
	}

	fmt.Println()
	fmt.Println("\033[1;33mVersions:\033[0m")
	if len(detail.Versions) == 0 {
		fmt.Println("  \033[90m(none)\033[0m")
		return
	}

	versions := make([]agentspec.AgentSpecVersionSummary, len(detail.Versions))
	copy(versions, detail.Versions)
	sort.SliceStable(versions, func(i, j int) bool {
		ti := termAgentSpecVersionSortKey(versions[i])
		tj := termAgentSpecVersionSortKey(versions[j])
		if ti != tj {
			return ti > tj
		}
		return versions[i].Version > versions[j].Version
	})

	fmt.Printf("  \033[90m%-12s  %-10s  %-12s  %-19s  %s\033[0m\n", "VERSION", "STATUS", "AUTHOR", "UPDATED", "DESCRIPTION")
	for _, v := range versions {
		updated := "-"
		if v.UpdateTime != nil && *v.UpdateTime > 0 {
			updated = time.UnixMilli(*v.UpdateTime).Format("2006-01-02 15:04:05")
		}
		desc := strings.ReplaceAll(v.Description, "\n", " ")
		desc = truncateDesc(desc, 60)
		fmt.Printf("  %-12s  %-10s  %-12s  %-19s  %s\n",
			v.Version,
			dashIfEmptyTerm(v.Status),
			dashIfEmptyTerm(v.Author),
			updated,
			desc,
		)
	}
}

func termAgentSpecVersionSortKey(v agentspec.AgentSpecVersionSummary) int64 {
	if v.UpdateTime != nil {
		return *v.UpdateTime
	}
	if v.CreateTime != nil {
		return *v.CreateTime
	}
	return 0
}

// submitAgentSpec submits an agent spec draft for review (editing -> reviewing).
func (t *Terminal) submitAgentSpec(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: agentspec-review <agentSpecName> [--version <version>]")
		return
	}

	specName := args[0]
	version := ""
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--version" && i+1 < len(args) {
			i++
			version = args[i]
		} else if strings.HasPrefix(arg, "--version=") {
			version = strings.TrimPrefix(arg, "--version=")
		}
	}

	fmt.Printf("Submitting agent spec for review: %s...\n", specName)
	if err := t.agentSpecService.SubmitAgentSpec(specName, version); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Agent spec submitted for review successfully!\n")
	fmt.Printf("Tip: After the review passes, run 'agentspec-release %s --version <ver>' to publish it online.\n", specName)
}

// releaseAgentSpec publishes an approved agent spec version (reviewing -> online).
func (t *Terminal) releaseAgentSpec(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: agentspec-release <agentSpecName> --version <version> [--update-latest=true|false]")
		return
	}

	specName := args[0]
	version := ""
	updateLatest := true
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--version" && i+1 < len(args) {
			i++
			version = args[i]
		} else if strings.HasPrefix(arg, "--version=") {
			version = strings.TrimPrefix(arg, "--version=")
		} else if arg == "--update-latest=false" || arg == "--update-latest=False" {
			updateLatest = false
		} else if arg == "--update-latest=true" || arg == "--update-latest=True" {
			updateLatest = true
		} else if arg == "--update-latest" && i+1 < len(args) {
			i++
			updateLatest = args[i] != "false" && args[i] != "False" && args[i] != "0"
		}
	}

	if version == "" {
		fmt.Println("Error: --version is required for agentspec-release")
		return
	}

	fmt.Printf("Releasing agent spec: %s@%s (updateLatest=%v)...\n", specName, version, updateLatest)
	if err := t.agentSpecService.PublishAgentSpec(specName, version, updateLatest); err != nil {
		fmt.Printf("Error: %v\n", err)
		maybePrintReleaseRetryHintTerm(err, "agentspec", specName)
		return
	}
	fmt.Printf("Agent spec released successfully! %s@%s is now online.\n", specName, version)
}

// maybePrintReleaseRetryHintTerm mirrors cmd.maybePrintReleaseRetryHint for the
// interactive terminal mode: when a *-release fails with HTTP 400 "parameter
// validate error", the root cause is usually the async review pipeline not
// having marked the version as 'reviewed' yet.
func maybePrintReleaseRetryHintTerm(err error, kind, name string) {
	if err == nil {
		return
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "400") && !strings.Contains(msg, "parameter validate") {
		return
	}
	fmt.Printf("\033[33mHint:\033[0m if you just ran '%s-review', the server-side review pipeline may still be running.\n", kind)
	fmt.Printf("      Wait 2-3 seconds, check status with '%s-describe %s', and retry when STATUS=reviewed.\n", kind, name)
}

// publishAgentSpecLegacy runs upload + review as a backward-compatible shortcut for the
// deprecated agentspec-publish command.
func (t *Terminal) publishAgentSpecLegacy(args []string) {
	fmt.Println("\033[33m[DEPRECATED] 'agentspec-publish' will be removed in a future release.\033[0m")
	fmt.Println("\033[90m  It now runs 'agentspec-upload' + 'agentspec-review' for compatibility.\033[0m")
	fmt.Println("\033[90m  Please migrate to: agentspec-upload -> agentspec-review -> agentspec-release.\033[0m")

	if len(args) == 0 {
		fmt.Println("Usage: agentspec-publish <agentSpecPath> or agentspec-publish --all <folder>")
		return
	}

	hasAll := false
	folderPath := ""
	for i, a := range args {
		if a == "--all" {
			hasAll = true
			if i+1 < len(args) {
				folderPath = args[i+1]
			} else if i > 0 {
				folderPath = args[i-1]
			}
			break
		}
	}

	if hasAll {
		if folderPath == "" {
			fmt.Println("Error: folder path required for --all flag")
			return
		}
		t.uploadAllAgentSpecs(folderPath)
		t.reviewAllAgentSpecs(folderPath)
		return
	}

	specPath := args[0]
	if strings.HasPrefix(specPath, "~/") {
		if homeDir, err := os.UserHomeDir(); err == nil {
			specPath = filepath.Join(homeDir, specPath[2:])
		}
	} else if specPath == "~" {
		if homeDir, err := os.UserHomeDir(); err == nil {
			specPath = homeDir
		}
	}

	specName := filepath.Base(specPath)
	if strings.HasSuffix(strings.ToLower(specName), ".zip") {
		specName = strings.TrimSuffix(specName, filepath.Ext(specName))
	}

	fmt.Printf("[1/2] Uploading agent spec: %s...\n", specPath)
	if err := t.agentSpecService.UploadAgentSpec(specPath); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Upload successful.\n")

	fmt.Printf("[2/2] Submitting agent spec for review: %s...\n", specName)
	if err := t.agentSpecService.SubmitAgentSpec(specName, ""); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Submitted for review successfully.\n")
	fmt.Printf("Tip: After the review passes, run 'agentspec-release %s --version <ver>' to publish online.\n", specName)
}

// reviewAllAgentSpecs submits every agent spec directory (with manifest.json) under folderPath for review.
func (t *Terminal) reviewAllAgentSpecs(folderPath string) {
	if strings.HasPrefix(folderPath, "~/") {
		if homeDir, err := os.UserHomeDir(); err == nil {
			folderPath = filepath.Join(homeDir, folderPath[2:])
		}
	}
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		fmt.Printf("Error reading folder: %v\n", err)
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(folderPath, entry.Name(), "manifest.json")); err != nil {
			continue
		}
		specName := entry.Name()
		fmt.Printf("Submitting agent spec for review: %s...\n", specName)
		if err := t.agentSpecService.SubmitAgentSpec(specName, ""); err != nil {
			fmt.Printf("  Failed: %v\n", err)
			continue
		}
		fmt.Printf("  OK\n")
	}
}

// truncateDesc truncates description to maxLen and appends ...... if needed
func truncateDesc(desc string, maxLen int) string {
	runes := []rune(desc)
	if len(runes) <= maxLen {
		return desc
	}
	return string(runes[:maxLen]) + "......"
}
