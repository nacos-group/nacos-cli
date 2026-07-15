package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/prompt"
	"github.com/spf13/cobra"
)

var (
	getPromptOutput  string
	getPromptVersion string
	getPromptLabel   string
)

var getPromptCmd = &cobra.Command{
	Use:   "prompt-get [promptKey]",
	Short: "Get a prompt template from Nacos",
	Long:  help.PromptGet.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		promptKey := args[0]

		nacosClient := mustNewNacosClient()
		promptService := prompt.NewPromptService(nacosClient)

		result, err := promptService.GetPrompt(promptKey, getPromptVersion, getPromptLabel)
		checkError(err)

		if getPromptOutput == "" {
			// Print to stdout
			printPromptContent(result)
		} else {
			// Save to file
			savePromptToFile(result, getPromptOutput)
		}
	},
}

func init() {
	getPromptCmd.Flags().StringVarP(&getPromptOutput, "output", "o", "", "Output file path (default: print to stdout)")
	getPromptCmd.Flags().StringVar(&getPromptVersion, "version", "", "Specific version to get")
	getPromptCmd.Flags().StringVar(&getPromptLabel, "label", "", "Route label to resolve version (e.g. latest, stable)")
	rootCmd.AddCommand(getPromptCmd)
}

func printPromptContent(p *prompt.ClientPrompt) {
	fmt.Printf("# Prompt: %s (version: %s)\n\n", p.PromptKey, p.Version)
	fmt.Println(p.Template)

	if len(p.Variables) > 0 && string(p.Variables) != "null" {
		fmt.Printf("\n---\nVariables: %s\n", string(p.Variables))
	}
}

func savePromptToFile(p *prompt.ClientPrompt, outputPath string) {
	content := p.Template

	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to write file %s: %v\n", outputPath, err)
		os.Exit(1)
	}

	fmt.Printf("Prompt saved successfully!\n")
	fmt.Printf("  Key: %s\n", p.PromptKey)
	fmt.Printf("  Version: %s\n", p.Version)
	fmt.Printf("  Location: %s\n", outputPath)

	// Save variables as a separate JSON file if present
	if len(p.Variables) > 0 && string(p.Variables) != "null" {
		varsPath := outputPath + ".variables.json"
		prettyVars, err := json.MarshalIndent(p.Variables, "", "  ")
		if err == nil {
			if err := os.WriteFile(varsPath, prettyVars, 0644); err == nil {
				fmt.Printf("  Variables: %s\n", varsPath)
			}
		}
	}
}
