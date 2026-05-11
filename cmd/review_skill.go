package cmd

import (
	"fmt"
	"os"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var skillReviewVersion string

var reviewSkillCmd = &cobra.Command{
	Use:   "skill-review [skillName]",
	Short: "Submit a skill draft for review (editing -> reviewing)",
	Long:  help.SkillReview.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nacosClient := mustNewNacosClient()
		skillService := skill.NewSkillService(nacosClient)

		skillName := args[0]
		fmt.Printf("Submitting skill for review: %s...\n", skillName)
		if err := skillService.SubmitSkill(skillName, skillReviewVersion); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to submit skill '%s' for review: %v\n", skillName, err)
			os.Exit(1)
		}
		fmt.Printf("Skill submitted for review successfully!\n")
		fmt.Printf("  Tip: After the review passes, run 'skill-release %s --version <ver>' to publish it online.\n", skillName)
	},
}

func init() {
	reviewSkillCmd.Flags().StringVar(&skillReviewVersion, "version", "", "Specific draft version to submit")
	rootCmd.AddCommand(reviewSkillCmd)
}
