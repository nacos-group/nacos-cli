package terminal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
	"github.com/nov11/nacos-cli/internal/agentspec"
	"github.com/nov11/nacos-cli/internal/client"
	"github.com/nov11/nacos-cli/internal/help"
	"github.com/nov11/nacos-cli/internal/skill"
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
	case client.AuthTypeAliyun:
		if t.client.AccessKey != "" {
			// Show first 8 chars of access key
			ak := t.client.AccessKey
			if len(ak) > 8 {
				ak = ak[:8]
			}
			return fmt.Sprintf("\033[32m%s@nacos>\033[0m ", ak)
		}
	case client.AuthTypeToken:
		return "\033[32m(token)nacos>\033[0m "
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
		),
		readline.PcItem("skill-get",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
		),
		readline.PcItem("skill-publish",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
			readline.PcItem("--all"),
		),
		readline.PcItem("agentspec-list",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
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
	case client.AuthTypeToken:
		fmt.Printf("\033[33mAuth:\033[0m Token (authenticated)\n")
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
	case "skill-get":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showSkillGetHelp()
		} else {
			t.getSkill(args)
		}
	case "skill-publish":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showSkillPublishHelp()
		} else {
			t.uploadSkill(args)
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
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "", "Options: --name, --page, --size", "")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "skill-get", "Download a skill to ~/.skills", "skill-get <name> [--version v1] [--label stable]")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "skill-publish", "Publish a skill from local", "skill-publish <path>")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "", "Publish all skills in directory", "skill-publish --all <folder>")
	fmt.Println()

	// AgentSpec Management
	fmt.Println("\033[1;33mAgentSpec Management\033[0m")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "agentspec-list", "List all agent specs", "agentspec-list [options]")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "", "Options: --name, --search, --page, --size", "")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "agentspec-get", "Download an agent spec to ~/.agentspecs", "agentspec-get <name> [--version v1] [--label stable]")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "agentspec-upload", "Upload an agent spec from local", "agentspec-upload <path>")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "", "Upload all agent specs in directory", "agentspec-upload --all <folder>")
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
	case client.AuthTypeToken:
		return "token (authenticated)"
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
				fmt.Sscanf(value, "%d", &page)
			}
		} else if arg == "--page" && i+1 < len(args) {
			i++
			fmt.Sscanf(args[i], "%d", &page)
		} else if arg == "--page=" && i+1 < len(args) {
			i++
			fmt.Sscanf(args[i], "%d", &page)
		} else if strings.HasPrefix(arg, "--size=") {
			value := strings.TrimPrefix(arg, "--size=")
			if value != "" {
				fmt.Sscanf(value, "%d", &size)
			}
		} else if arg == "--size" && i+1 < len(args) {
			i++
			fmt.Sscanf(args[i], "%d", &size)
		} else if arg == "--size=" && i+1 < len(args) {
			i++
			fmt.Sscanf(args[i], "%d", &size)
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
	for i, skill := range skills {
		if skill.Description != "" {
			desc := truncateDesc(skill.Description, defaultDescLimit)
			fmt.Printf("\033[90m%3d.\033[0m \033[32m%s\033[0m \033[90m- %s\033[0m\n", (page-1)*size+i+1, skill.Name, desc)
		} else {
			fmt.Printf("\033[90m%3d.\033[0m \033[32m%s\033[0m\n", (page-1)*size+i+1, skill.Name)
		}
	}
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

// uploadSkill uploads a skill
func (t *Terminal) uploadSkill(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: skill-upload <skillPath> or skill-upload --all <folder>")
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
			fmt.Println("Usage: skill-upload --all <folder> or skill-upload <folder> --all")
			return
		}
		t.uploadAllSkills(folderPath)
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

	err := t.skillService.UploadSkill(skillPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Skill uploaded successfully!\n")
}

// uploadAllSkills uploads all skills in a directory
func (t *Terminal) uploadAllSkills(folderPath string) {
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
		fmt.Printf("[%d/%d] Uploading skill: %s\n", i+1, len(skillDirs), skillName)
		fmt.Println(strings.Repeat("=", 80))

		skillPath := filepath.Join(folderPath, skillName)
		err := t.skillService.UploadSkill(skillPath)
		if err != nil {
			fmt.Printf("Upload failed: %v\n", err)
			failedCount++
		} else {
			fmt.Printf("Upload successful!\n")
			successCount++
		}
		fmt.Println()
	}

	// Summary
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("Batch Upload Complete")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Success: %d\n", successCount)
	if failedCount > 0 {
		fmt.Printf("Failed: %d\n", failedCount)
	}
	fmt.Printf("Total: %d\n", len(skillDirs))
	fmt.Println()
	fmt.Println("Tip: Use 'skill-list' to view all uploaded skills")
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
				fmt.Sscanf(value, "%d", &page)
			}
		} else if arg == "--page" && i+1 < len(args) {
			i++
			fmt.Sscanf(args[i], "%d", &page)
		} else if arg == "--page=" && i+1 < len(args) {
			i++
			fmt.Sscanf(args[i], "%d", &page)
		} else if strings.HasPrefix(arg, "--size=") {
			value := strings.TrimPrefix(arg, "--size=")
			if value != "" {
				fmt.Sscanf(value, "%d", &size)
			}
		} else if arg == "--size" && i+1 < len(args) {
			i++
			fmt.Sscanf(args[i], "%d", &size)
		} else if arg == "--size=" && i+1 < len(args) {
			i++
			fmt.Sscanf(args[i], "%d", &size)
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

func (t *Terminal) showSkillGetHelp() {
	help.SkillGet.FormatForTerminal()
}

func (t *Terminal) showSkillPublishHelp() {
	help.SkillPublish.FormatForTerminal()
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

func (t *Terminal) showSkillSyncHelp() {
	fmt.Println("\033[33mskill-sync has been removed.\033[0m")
	fmt.Println("\033[90mUse 'skill-get' to download skills.\033[0m")
}

// AgentSpec command help methods

func (t *Terminal) showAgentSpecListHelp() {
	help.AgentSpecList.FormatForTerminal()
}

func (t *Terminal) showAgentSpecGetHelp() {
	help.AgentSpecGet.FormatForTerminal()
}

func (t *Terminal) showAgentSpecUploadHelp() {
	help.AgentSpecUpload.FormatForTerminal()
}

// listAgentSpecs lists all agent specs
func (t *Terminal) listAgentSpecs(args []string) {
	// Parse flags
	var name, search string
	var page, size int = 1, 20

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--name=") {
			name = strings.TrimPrefix(arg, "--name=")
		} else if arg == "--name" && i+1 < len(args) {
			i++
			name = args[i]
		} else if strings.HasPrefix(arg, "--search=") {
			search = strings.TrimPrefix(arg, "--search=")
		} else if arg == "--search" && i+1 < len(args) {
			i++
			search = args[i]
		} else if strings.HasPrefix(arg, "--page=") {
			value := strings.TrimPrefix(arg, "--page=")
			if value != "" {
				fmt.Sscanf(value, "%d", &page)
			}
		} else if arg == "--page" && i+1 < len(args) {
			i++
			fmt.Sscanf(args[i], "%d", &page)
		} else if strings.HasPrefix(arg, "--size=") {
			value := strings.TrimPrefix(arg, "--size=")
			if value != "" {
				fmt.Sscanf(value, "%d", &size)
			}
		} else if arg == "--size" && i+1 < len(args) {
			i++
			fmt.Sscanf(args[i], "%d", &size)
		}
	}

	fmt.Print("\033[90mFetching agent specs...\033[0m\r")

	specs, totalCount, err := t.agentSpecService.ListAgentSpecs(name, search, page, size)
	if err != nil {
		fmt.Printf("\033[31mError:\033[0m %v\n", err)
		return
	}

	fmt.Print("\033[K") // Clear line

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
	for i, spec := range specs {
		enableStr := "\033[32menabled\033[0m"
		if !spec.Enable {
			enableStr = "\033[31mdisabled\033[0m"
		}
		if spec.Description != nil && *spec.Description != "" {
			desc := truncateDesc(*spec.Description, defaultDescLimit)
			fmt.Printf("\033[90m%3d.\033[0m \033[32m%s\033[0m \033[90m- %s\033[0m [%s, \033[90monline:%d\033[0m]\n", (page-1)*size+i+1, spec.Name, desc, enableStr, spec.OnlineCnt)
		} else {
			fmt.Printf("\033[90m%3d.\033[0m \033[32m%s\033[0m [%s, \033[90monline:%d\033[0m]\n", (page-1)*size+i+1, spec.Name, enableStr, spec.OnlineCnt)
		}
	}
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

// uploadAgentSpec uploads an agent spec
func (t *Terminal) uploadAgentSpec(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: agentspec-upload <agentSpecPath> or agentspec-upload --all <folder>")
		return
	}

	// Check for --all flag in any position
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

	// If --all found but no folder after it, check if folder is before --all
	if allFlagIndex >= 0 && folderPath == "" {
		if allFlagIndex > 0 {
			folderPath = args[allFlagIndex-1]
		}
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

	// Single agent spec upload
	specPath := args[0]

	// Expand ~ to home directory
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

	err := t.agentSpecService.UploadAgentSpec(specPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Agent spec uploaded successfully!\n")
}

// uploadAllAgentSpecs uploads all agent specs in a directory
func (t *Terminal) uploadAllAgentSpecs(folderPath string) {
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

	var specDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Check if manifest.json exists
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

	successCount := 0
	failedCount := 0

	for i, specName := range specDirs {
		fmt.Println(strings.Repeat("=", 80))
		fmt.Printf("[%d/%d] Uploading agent spec: %s\n", i+1, len(specDirs), specName)
		fmt.Println(strings.Repeat("=", 80))

		specPath := filepath.Join(folderPath, specName)
		err := t.agentSpecService.UploadAgentSpec(specPath)
		if err != nil {
			fmt.Printf("Upload failed: %v\n", err)
			failedCount++
		} else {
			fmt.Printf("Upload successful!\n")
			successCount++
		}
		fmt.Println()
	}

	// Summary
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("Batch Upload Complete")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Success: %d\n", successCount)
	if failedCount > 0 {
		fmt.Printf("Failed: %d\n", failedCount)
	}
	fmt.Printf("Total: %d\n", len(specDirs))
	fmt.Println()
	fmt.Println("Tip: Use 'agentspec-list' to view all uploaded agent specs")
}

// truncateDesc truncates description to maxLen and appends ...... if needed
func truncateDesc(desc string, maxLen int) string {
	runes := []rune(desc)
	if len(runes) <= maxLen {
		return desc
	}
	return string(runes[:maxLen]) + "......"
}
