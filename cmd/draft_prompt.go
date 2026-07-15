package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/prompt"
	"github.com/spf13/cobra"
)

var (
	promptDraftFile          string
	promptDraftVariables     string
	promptDraftMessage       string
	promptDraftDescription   string
	promptDraftBizTags       string
	promptDraftTargetVersion string
	promptDraftBasedOn       string
)

var draftPromptCmd = &cobra.Command{
	Use:   "prompt-draft [promptKey]",
	Short: "Create or update a prompt draft",
	Long:  help.PromptDraft.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		promptKey := args[0]

		// Read template content from file or stdin
		template := readPromptTemplate()

		nacosClient := mustNewNacosClient()
		promptService := prompt.NewPromptService(nacosClient)

		fmt.Printf("Creating/updating prompt draft: %s...\n", promptKey)
		err := promptService.Draft(promptKey, template, promptDraftVariables,
			promptDraftMessage, promptDraftDescription, promptDraftBizTags,
			promptDraftTargetVersion, promptDraftBasedOn)
		checkError(err)

		fmt.Printf("Prompt draft saved successfully!\n")
		fmt.Printf("  Tip: Use 'prompt-review %s' to submit the draft for review.\n", promptKey)
	},
}

func init() {
	draftPromptCmd.Flags().StringVarP(&promptDraftFile, "file", "f", "", "Path to template file (default: read from stdin)")
	draftPromptCmd.Flags().StringVar(&promptDraftVariables, "variables", "", "Variables JSON array")
	draftPromptCmd.Flags().StringVar(&promptDraftMessage, "message", "", "Commit message for this draft")
	draftPromptCmd.Flags().StringVar(&promptDraftDescription, "description", "", "Prompt description (used when creating new prompt)")
	draftPromptCmd.Flags().StringVar(&promptDraftBizTags, "biz-tags", "", "Business tags (used when creating new prompt)")
	draftPromptCmd.Flags().StringVar(&promptDraftTargetVersion, "target-version", "", "Target version for the new draft (e.g. 1.0.0)")
	draftPromptCmd.Flags().StringVar(&promptDraftBasedOn, "based-on-version", "", "Fork from an existing version (e.g. 1.0.0)")
	rootCmd.AddCommand(draftPromptCmd)
}

func readPromptTemplate() string {
	if promptDraftFile != "" {
		data, err := os.ReadFile(promptDraftFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to read file %s: %v\n", promptDraftFile, err)
			os.Exit(1)
		}
		return string(data)
	}

	// Read from stdin
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		fmt.Fprintf(os.Stderr, "Error: no template provided. Use --file or pipe content via stdin.\n")
		os.Exit(1)
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to read stdin: %v\n", err)
		os.Exit(1)
	}
	if len(data) == 0 {
		fmt.Fprintf(os.Stderr, "Error: empty template. Provide content via --file or stdin.\n")
		os.Exit(1)
	}
	return string(data)
}
