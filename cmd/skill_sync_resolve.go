package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var (
	resolveUseLocal  bool
	resolveUseRemote bool
	resolveAll       bool
)

var skillSyncResolveCmd = &cobra.Command{
	Use:   "resolve [skill]",
	Short: "Resolve conflicts between local and remote versions",
	Long: `Resolve sync conflicts. Default is interactive mode.

Non-interactive flags:
  --use-local   Keep local changes (status → Local changes, then upload)
  --use-remote  Use remote version (auto-backup local, status → Synced)
  --all         Apply strategy to all conflicted skills`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Validate flags
		if resolveUseLocal && resolveUseRemote {
			fmt.Fprintf(os.Stderr, "Error: --use-local and --use-remote are mutually exclusive\n")
			os.Exit(1)
		}

		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		if len(state.Agents) == 0 {
			fmt.Fprintf(os.Stderr, "Error: no agent directories configured\n")
			os.Exit(1)
		}

		// Determine which skills to resolve
		var skillsToResolve []string

		if resolveAll {
			for name, entry := range state.Skills {
				if entry.Status == skill.SyncStatusConflict {
					skillsToResolve = append(skillsToResolve, name)
				}
			}
			if len(skillsToResolve) == 0 {
				fmt.Println("No skills in conflict state.")
				return
			}
		} else {
			if len(args) == 0 {
				fmt.Fprintf(os.Stderr, "Error: skill name required (or use --all)\n")
				os.Exit(1)
			}
			skillName := args[0]
			entry, ok := state.Skills[skillName]
			if !ok {
				fmt.Fprintf(os.Stderr, "Error: skill %q not found in subscriptions\n", skillName)
				os.Exit(1)
			}
			if entry.Status != skill.SyncStatusConflict {
				fmt.Fprintf(os.Stderr, "Error: skill %q is not in conflict state (current: %s)\n",
					skillName, entry.Status.DisplayString())
				os.Exit(1)
			}
			skillsToResolve = []string{skillName}
		}

		nacosClient := mustNewNacosClient()
		skillService := skill.NewSkillService(nacosClient)

		for _, skillName := range skillsToResolve {
			entry := state.Skills[skillName]

			var strategy string
			if resolveUseLocal {
				strategy = "local"
			} else if resolveUseRemote {
				strategy = "remote"
			} else {
				// Interactive mode
				strategy = promptResolveStrategy(skillName, entry)
				if strategy == "skip" {
					fmt.Printf("  Skipped: %s\n", skillName)
					continue
				}
			}

			fmt.Printf("Resolving: %s (strategy: use-%s)\n", skillName, strategy)

			switch strategy {
			case "local":
				if err := skill.ResolveUseLocal(state, skillName); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("  Kept local changes. Use 'skill-upload' to push to Nacos.\n")
			case "remote":
				if err := skill.ResolveUseRemote(state, skillName, skillService, state.Agents); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("  Applied remote version. Local backup created.\n")
			}
		}

		if err := skill.SaveSyncState(state); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to save sync state: %v\n", err)
			os.Exit(1)
		}
	},
}

func promptResolveStrategy(skillName string, entry skill.SyncSkillEntry) string {
	fmt.Printf("\nConflict: %s\n", skillName)
	fmt.Printf("  Local changes: yes\n")
	fmt.Printf("  Remote version: %s\n", entry.ResolvedVersion)
	fmt.Println()
	fmt.Println("Choose:")
	fmt.Println("  1. Keep local changes")
	fmt.Println("  2. Use remote version")
	fmt.Println("  3. Skip")
	fmt.Print("\nChoice [1/2/3]: ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	switch input {
	case "1", "l", "local":
		return "local"
	case "2", "r", "remote":
		return "remote"
	default:
		return "skip"
	}
}

func init() {
	skillSyncResolveCmd.Flags().BoolVar(&resolveUseLocal, "use-local", false, "Keep local changes (non-interactive)")
	skillSyncResolveCmd.Flags().BoolVar(&resolveUseRemote, "use-remote", false, "Use remote version (non-interactive)")
	skillSyncResolveCmd.Flags().BoolVar(&resolveAll, "all", false, "Resolve all conflicted skills")
	skillSyncCmd.AddCommand(skillSyncResolveCmd)
}
