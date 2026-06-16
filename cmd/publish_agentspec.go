package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/agentspec"
	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/util"
	"github.com/spf13/cobra"
)

// agentSpecPublishAll is the legacy flag for agentspec-publish --all.
var agentSpecPublishAll bool

// publishAgentSpecCmd is DEPRECATED. It now performs agentspec-upload followed by
// agentspec-review (submit for review) as a backward-compatible shortcut.
//
// Users should migrate to the new three-step flow:
//
//	agentspec-upload  -> agentspec-review  -> agentspec-release
var publishAgentSpecCmd = &cobra.Command{
	Use:               "agentspec-publish [agentSpecPath]",
	Short:             "[DEPRECATED] Upload then submit an agent spec draft for review (use agentspec-upload/agentspec-review/agentspec-release instead)",
	Long:              help.AgentSpecPublish.FormatForCLI("nacos-cli"),
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completePathArg(0),
	Run: func(cmd *cobra.Command, args []string) {
		printAgentSpecPublishDeprecationWarning()

		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Error: agent spec path required\n")
			os.Exit(1)
		}
		specPath := args[0]

		nacosClient := mustNewNacosClient()
		agentSpecService := agentspec.NewAgentSpecService(nacosClient)

		if agentSpecPublishAll {
			publishAllAgentSpecsLegacy(specPath, agentSpecService)
			return
		}
		publishSingleAgentSpecLegacy(specPath, agentSpecService)
	},
}

// printAgentSpecPublishDeprecationWarning prints a visible deprecation notice to stderr.
func printAgentSpecPublishDeprecationWarning() {
	fmt.Fprintln(os.Stderr, "------------------------------------------------------------")
	fmt.Fprintln(os.Stderr, "[DEPRECATED] 'agentspec-publish' will be removed in a future release.")
	fmt.Fprintln(os.Stderr, "  It now runs 'agentspec-upload' + 'agentspec-review' for compatibility.")
	fmt.Fprintln(os.Stderr, "  Please migrate to the new flow:")
	fmt.Fprintln(os.Stderr, "    1) agentspec-upload  <agentSpecPath>              # upload as editing draft")
	fmt.Fprintln(os.Stderr, "    2) agentspec-review  <agentSpecName>              # submit for review")
	fmt.Fprintln(os.Stderr, "    3) agentspec-release <agentSpecName> --version <ver>  # publish online")
	fmt.Fprintln(os.Stderr, "------------------------------------------------------------")
}

// publishSingleAgentSpecLegacy runs upload + review for a single agent spec directory/zip.
func publishSingleAgentSpecLegacy(specPath string, agentSpecService *agentspec.AgentSpecService) {
	expanded, err := util.ExpandTilde(specPath)
	checkError(err)
	specPath = expanded

	absPath, err := filepath.Abs(specPath)
	checkError(err)

	specName := deriveAgentSpecNameFromPath(absPath)
	fmt.Printf("[1/2] Uploading agent spec: %s...\n", specName)
	if err := agentSpecService.UploadAgentSpec(absPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: upload failed for '%s': %v\n", specName, err)
		os.Exit(1)
	}
	fmt.Printf("Upload successful.\n")

	fmt.Printf("[2/2] Submitting agent spec for review: %s...\n", specName)
	if err := agentSpecService.SubmitAgentSpec(specName, ""); err != nil {
		fmt.Fprintf(os.Stderr, "Error: submit-for-review failed for '%s': %v\n", specName, err)
		os.Exit(1)
	}
	fmt.Printf("Submitted for review successfully.\n")
	fmt.Printf("  Tip: After the review passes, run 'agentspec-release %s --version <ver>' to publish online.\n", specName)
}

// publishAllAgentSpecsLegacy runs upload + review for each agent spec directory under folderPath.
func publishAllAgentSpecsLegacy(folderPath string, agentSpecService *agentspec.AgentSpecService) {
	expanded, err := util.ExpandTilde(folderPath)
	checkError(err)
	folderPath = expanded

	entries, err := os.ReadDir(folderPath)
	checkError(err)

	var specDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(folderPath, entry.Name(), "manifest.json")
		if _, err := os.Stat(manifestPath); err == nil {
			specDirs = append(specDirs, entry.Name())
		}
	}

	if len(specDirs) == 0 {
		fmt.Println("No agent specs found (directories with manifest.json)")
		return
	}

	fmt.Printf("Found %d agent specs:\n", len(specDirs))
	for _, name := range specDirs {
		fmt.Printf("  - %s\n", name)
	}
	fmt.Println()

	successCount := 0
	failedCount := 0

	for i, specName := range specDirs {
		fmt.Println(strings.Repeat("=", 80))
		fmt.Printf("[%d/%d] upload+review: %s\n", i+1, len(specDirs), specName)
		fmt.Println(strings.Repeat("=", 80))

		specPath := filepath.Join(folderPath, specName)
		if err := agentSpecService.UploadAgentSpec(specPath); err != nil {
			fmt.Printf("Upload failed: %v\n", err)
			failedCount++
			fmt.Println()
			continue
		}
		if err := agentSpecService.SubmitAgentSpec(specName, ""); err != nil {
			fmt.Printf("Submit-for-review failed: %v\n", err)
			failedCount++
			fmt.Println()
			continue
		}
		fmt.Printf("Upload + review submitted successfully!\n")
		successCount++
		fmt.Println()
	}

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("Batch Publish (Deprecated) Complete")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Success: %d\n", successCount)
	if failedCount > 0 {
		fmt.Printf("Failed: %d\n", failedCount)
	}
	fmt.Printf("Total: %d\n", len(specDirs))
	fmt.Println()
	fmt.Println("Tip: After each review passes, run 'agentspec-release <agentSpecName> --version <ver>' to publish online.")
}

func init() {
	publishAgentSpecCmd.Flags().BoolVar(&agentSpecPublishAll, "all", false, "Publish all agent specs in the directory (deprecated)")
	rootCmd.AddCommand(publishAgentSpecCmd)
}
