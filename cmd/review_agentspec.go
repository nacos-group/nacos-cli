package cmd

import (
	"fmt"
	"os"

	"github.com/nacos-group/nacos-cli/internal/agentspec"
	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/spf13/cobra"
)

var agentSpecReviewVersion string

var reviewAgentSpecCmd = &cobra.Command{
	Use:   "agentspec-review [agentSpecName]",
	Short: "Submit an agent spec draft for review (editing -> reviewing)",
	Long:  help.AgentSpecReview.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nacosClient := mustNewNacosClient()
		agentSpecService := agentspec.NewAgentSpecService(nacosClient)

		specName := args[0]
		fmt.Printf("Submitting agent spec for review: %s...\n", specName)
		if err := agentSpecService.SubmitAgentSpec(specName, agentSpecReviewVersion); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to submit agent spec '%s' for review: %v\n", specName, err)
			os.Exit(1)
		}
		fmt.Printf("Agent spec submitted for review successfully!\n")
		fmt.Printf("  Tip: After the review passes, run 'agentspec-release %s --version <ver>' to publish it online.\n", specName)
	},
}

func init() {
	reviewAgentSpecCmd.Flags().StringVar(&agentSpecReviewVersion, "version", "", "Specific draft version to submit")
	rootCmd.AddCommand(reviewAgentSpecCmd)
}
