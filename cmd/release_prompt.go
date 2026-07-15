package cmd

import (
	"fmt"
	"os"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/prompt"
	"github.com/spf13/cobra"
)

var (
	promptReleaseVersion      string
	promptReleaseUpdateLatest bool
)

var releasePromptCmd = &cobra.Command{
	Use:   "prompt-release [promptKey]",
	Short: "Release an approved prompt version (reviewing -> online)",
	Long:  help.PromptRelease.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if promptReleaseVersion == "" {
			fmt.Fprintf(os.Stderr, "Error: --version is required for prompt-release\n")
			os.Exit(1)
		}

		nacosClient := mustNewNacosClient()
		promptService := prompt.NewPromptService(nacosClient)

		promptKey := args[0]
		fmt.Printf("Releasing prompt: %s@%s (updateLatest=%v)...\n", promptKey, promptReleaseVersion, promptReleaseUpdateLatest)
		if err := promptService.Publish(promptKey, promptReleaseVersion, promptReleaseUpdateLatest); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to release prompt '%s@%s': %v\n", promptKey, promptReleaseVersion, err)
			maybePrintReleaseRetryHint(err, "prompt", promptKey)
			os.Exit(1)
		}
		fmt.Printf("Prompt released successfully!\n")
		fmt.Printf("  %s@%s is now online.\n", promptKey, promptReleaseVersion)
	},
}

func init() {
	releasePromptCmd.Flags().StringVar(&promptReleaseVersion, "version", "", "Required. Approved (reviewing) version to release")
	releasePromptCmd.Flags().BoolVar(&promptReleaseUpdateLatest, "update-latest", true, "Whether to update the 'latest' label to the released version")
	rootCmd.AddCommand(releasePromptCmd)
}
