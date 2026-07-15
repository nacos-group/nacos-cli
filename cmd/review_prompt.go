package cmd

import (
	"fmt"
	"os"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/prompt"
	"github.com/spf13/cobra"
)

var promptReviewVersion string

var reviewPromptCmd = &cobra.Command{
	Use:   "prompt-review [promptKey]",
	Short: "Submit a prompt draft for review (editing -> reviewing)",
	Long:  help.PromptReview.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nacosClient := mustNewNacosClient()
		promptService := prompt.NewPromptService(nacosClient)

		promptKey := args[0]
		fmt.Printf("Submitting prompt for review: %s...\n", promptKey)
		if err := promptService.Submit(promptKey, promptReviewVersion); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to submit prompt '%s' for review: %v\n", promptKey, err)
			os.Exit(1)
		}
		fmt.Printf("Prompt submitted for review successfully!\n")
		fmt.Printf("  Tip: After the review passes, run 'prompt-release %s --version <ver>' to publish it online.\n", promptKey)
	},
}

func init() {
	reviewPromptCmd.Flags().StringVar(&promptReviewVersion, "version", "", "Specific draft version to submit")
	rootCmd.AddCommand(reviewPromptCmd)
}
