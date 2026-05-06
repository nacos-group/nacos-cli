package cmd

import (
	"fmt"
	"os"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var skillSubmitVersion string

var submitSkillCmd = &cobra.Command{
	Use:   "skill-submit [skillName]",
	Short: "Submit a skill draft for review",
	Long:  help.SkillSubmit.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nacosClient := mustNewNacosClient()
		skillService := skill.NewSkillService(nacosClient)

		skillName := args[0]
		fmt.Printf("Submitting skill draft: %s...\n", skillName)
		if err := skillService.SubmitSkill(skillName, skillSubmitVersion); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to submit skill '%s': %v\n", skillName, err)
			os.Exit(1)
		}
		fmt.Printf("Skill submitted successfully!\n")
		fmt.Printf("  Tip: Auto-publish after review depends on server configuration. Use 'skill-list' to verify.\n")
	},
}

func init() {
	submitSkillCmd.Flags().StringVar(&skillSubmitVersion, "version", "", "Specific draft version to submit")
	rootCmd.AddCommand(submitSkillCmd)
}
