package cmd

import (
	"fmt"

	"github.com/nov11/nacos-cli/internal/client"
	"github.com/nov11/nacos-cli/internal/help"
	"github.com/nov11/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var (
	skillListPage    int
	skillListSize    int
	skillListName    string
	skillListShowDesc bool
)

var listSkillCmd = &cobra.Command{
	Use:   "skill-list",
	Short: "List all skills",
	Long:  help.SkillList.FormatForCLI("nacos-cli"),
	Run: func(cmd *cobra.Command, args []string) {
		// Create Nacos client
		nacosClient := client.NewNacosClient(serverAddr, namespace, authType, username, password, accessKey, secretKey)

		// Create skill service
		skillService := skill.NewSkillService(nacosClient)

		// List skills
		skills, totalCount, err := skillService.ListSkills(skillListName, skillListPage, skillListSize)
		checkError(err)

		// Display results
		if len(skills) == 0 {
			fmt.Println("No skills found")
			return
		}

		fmt.Printf("Skill List (Total: %d)\n", totalCount)
		fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
		for i, skill := range skills {
			if skillListShowDesc && skill.Description != "" {
				fmt.Printf("%3d. %s - %s\n", i+1, skill.Name, skill.Description)
			} else {
				fmt.Printf("%3d. %s\n", i+1, skill.Name)
			}
		}
	},
}

func init() {
	listSkillCmd.Flags().IntVar(&skillListPage, "page", 1, "Page number (default: 1)")
	listSkillCmd.Flags().IntVar(&skillListSize, "size", 20, "Page size (default: 20)")
	listSkillCmd.Flags().StringVar(&skillListName, "name", "", "Filter by skill name (supports wildcard *)")
	listSkillCmd.Flags().BoolVar(&skillListShowDesc, "desc", false, "Show skill description")
	rootCmd.AddCommand(listSkillCmd)
}
