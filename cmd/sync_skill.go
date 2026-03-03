package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nov11/nacos-cli/internal/client"
	"github.com/nov11/nacos-cli/internal/help"
	"github.com/nov11/nacos-cli/internal/skill"
	"github.com/nov11/nacos-cli/internal/sync"
	"github.com/spf13/cobra"
)

var (
	syncAllSkills bool
	syncOutputDir string
)

var syncSkillCmd = &cobra.Command{
	Use:   "skill-sync [skillName...]",
	Short: "Synchronize a skill with Nacos (real-time updates)",
	Long:  help.SkillSync.FormatForCLI("nacos-cli"),
	Args:  cobra.MinimumNArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		var skillNames []string

		// Handle --all flag
		if syncAllSkills {
			// Create Nacos client to fetch all skills
			nacosClient := client.NewNacosClient(serverAddr, namespace, authType, username, password, accessKey, secretKey)
			skillService := skill.NewSkillService(nacosClient)

			fmt.Println("Fetching list of all skills...")
			skills, _, err := skillService.ListSkills("", 1, 10000)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error fetching skills: %v\n", err)
				os.Exit(1)
			}

			if len(skills) == 0 {
				fmt.Println("No skills found")
				os.Exit(0)
			}

			// Extract skill names from SkillListItem
			for _, s := range skills {
				skillNames = append(skillNames, s.Name)
			}
			fmt.Printf("Found %d skills\n\n", len(skillNames))
		} else if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Error: skill name required (or use --all to sync all skills)\n")
			fmt.Fprintf(os.Stderr, "\nUsage:\n")
			fmt.Fprintf(os.Stderr, "  nacos-cli skill-sync <skillName> [skillName2...]\n")
			fmt.Fprintf(os.Stderr, "  nacos-cli skill-sync --all\n")
			os.Exit(1)
		} else {
			skillNames = args
		}

		// Create Nacos client
		nacosClient := client.NewNacosClient(serverAddr, namespace, authType, username, password, accessKey, secretKey)

		// Create skill syncer
		skillSyncer := sync.NewSkillSyncer(nacosClient, syncOutputDir)

		// Setup signal handling
		stopCh := make(chan struct{})
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-sigCh
			fmt.Println("\n\nStopping synchronization...")
			close(stopCh)
		}()

		// Start sync (single or multiple skills)
		var err error
		if len(skillNames) == 1 {
			err = skillSyncer.StartSync(skillNames[0], stopCh)
		} else {
			err = skillSyncer.StartSyncMultiple(skillNames, stopCh)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Synchronization stopped")
	},
}

func init() {
	syncSkillCmd.Flags().BoolVar(&syncAllSkills, "all", false, "Synchronize all skills")
	syncSkillCmd.Flags().StringVarP(&syncOutputDir, "dir", "d", "", "Output directory for synced skills (default: ~/.skills)")
	rootCmd.AddCommand(syncSkillCmd)
}
