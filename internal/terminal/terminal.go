package terminal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
	"github.com/nov11/nacos-cli/internal/client"
	"github.com/nov11/nacos-cli/internal/help"
	"github.com/nov11/nacos-cli/internal/skill"
)

// Terminal represents an interactive terminal
type Terminal struct {
	client       *client.NacosClient
	skillService *skill.SkillService
	rl           *readline.Instance
	running      bool
}

// NewTerminal creates a new interactive terminal
func NewTerminal(nacosClient *client.NacosClient) *Terminal {
	return &Terminal{
		client:       nacosClient,
		skillService: skill.NewSkillService(nacosClient),
		running:      true,
	}
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
		readline.PcItem("skill-sync",
			readline.PcItem("--help"),
			readline.PcItem("-h"),
		),
		readline.PcItem("skill-upload",
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
		Prompt:          "\033[32mnacos>\033[0m ",
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
	fmt.Println()
	fmt.Println("\033[90mType '\033[0mhelp\033[90m' for available commands\033[0m")
	fmt.Println("\033[90mPress '\033[0mTab\033[90m' for auto-completion\033[0m")
	fmt.Println("\033[90mPress '\033[0mCtrl+C\033[90m' or type '\033[0mquit\033[90m' to quit\033[0m")
	fmt.Println()
}

// handleCommand handles user command
func (t *Terminal) handleCommand(input string) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return
	}

	cmd := parts[0]
	args := parts[1:]

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
	case "skill-upload":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showSkillUploadHelp()
		} else {
			t.uploadSkill(args)
		}
	case "skill-sync":
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			t.showSkillSyncHelp()
		} else {
			fmt.Println("\033[33mskill-sync is not supported in terminal mode\033[0m")
			fmt.Println("\033[90mUse CLI mode:\033[0m nacos-cli skill-sync <skillName>")
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
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "skill-get", "Download a skill to ~/.skills", "skill-get <name>")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "skill-sync", "Sync skill with Nacos (CLI only)", "skill-sync <name> (CLI mode)")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "skill-upload", "Upload a skill from local", "skill-upload <path>")
	fmt.Printf("\033[32m%-20s\033[0m %-40s %-30s\n", "", "Upload all skills in directory", "skill-upload --all <folder>")
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
	fmt.Println("─────────────────────────────────────────────────────────")
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
	fmt.Println("\033[36m═══════════════════════════════════════════════════════\033[0m")
	for i, skillName := range skills {
		fmt.Printf("\033[90m%3d.\033[0m \033[32m%s\033[0m\n", (page-1)*size+i+1, skillName)
	}
}

// getSkill downloads a skill
func (t *Terminal) getSkill(args []string) {
	if len(args) == 0 {
		fmt.Println("\033[31mUsage:\033[0m skill-get <skillName>")
		return
	}

	skillName := args[0]

	// Default output directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("\033[31mError:\033[0m %v\n", err)
		return
	}
	outputDir := filepath.Join(homeDir, ".skills")

	fmt.Printf("\033[90mDownloading skill: \033[33m%s\033[90m...\033[0m\n", skillName)

	err = t.skillService.GetSkill(skillName, outputDir)
	if err != nil {
		fmt.Printf("\033[31mError:\033[0m %v\n", err)
		return
	}

	fmt.Printf("\033[32mSkill downloaded successfully!\033[0m\n")
	fmt.Printf("  \033[90mLocation:\033[0m %s/%s\n", outputDir, skillName)
}

// uploadSkill uploads a skill
func (t *Terminal) uploadSkill(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: skill-upload <skillPath> or skill-upload --all <folder>")
		return
	}

	// Check for --all flag
	if args[0] == "--all" {
		if len(args) < 2 {
			fmt.Println("Error: folder path required for --all flag")
			return
		}
		t.uploadAllSkills(args[1])
		return
	}

	// Single skill upload
	skillPath := args[0]
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

func (t *Terminal) showSkillUploadHelp() {
	help.SkillUpload.FormatForTerminal()
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
	help.SkillSync.FormatForTerminal()
}
