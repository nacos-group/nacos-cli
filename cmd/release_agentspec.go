package cmd

import (
	"fmt"
	"os"

	"github.com/nacos-group/nacos-cli/internal/agentspec"
	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/spf13/cobra"
)

var (
	agentSpecReleaseVersion      string
	agentSpecReleaseUpdateLatest bool
)

var releaseAgentSpecCmd = &cobra.Command{
	Use:   "agentspec-release [agentSpecName]",
	Short: "Release an approved agent spec version (reviewing -> online)",
	Long:  help.AgentSpecRelease.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if agentSpecReleaseVersion == "" {
			fmt.Fprintf(os.Stderr, "Error: --version is required for agentspec-release\n")
			os.Exit(1)
		}

		nacosClient := mustNewNacosClient()
		agentSpecService := agentspec.NewAgentSpecService(nacosClient)

		specName := args[0]
		fmt.Printf("Releasing agent spec: %s@%s (updateLatest=%v)...\n", specName, agentSpecReleaseVersion, agentSpecReleaseUpdateLatest)
		if err := agentSpecService.PublishAgentSpec(specName, agentSpecReleaseVersion, agentSpecReleaseUpdateLatest); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to release agent spec '%s@%s': %v\n", specName, agentSpecReleaseVersion, err)
			maybePrintReleaseRetryHint(err, "agentspec", specName)
			os.Exit(1)
		}
		fmt.Printf("Agent spec released successfully!\n")
		fmt.Printf("  %s@%s is now online.\n", specName, agentSpecReleaseVersion)
	},
}

func init() {
	releaseAgentSpecCmd.Flags().StringVar(&agentSpecReleaseVersion, "version", "", "Required. Approved (reviewing) version to release")
	releaseAgentSpecCmd.Flags().BoolVar(&agentSpecReleaseUpdateLatest, "update-latest", true, "Whether to update the 'latest' label to the released version")
	rootCmd.AddCommand(releaseAgentSpecCmd)
}
