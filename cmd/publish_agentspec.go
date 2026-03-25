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

var (
	agentSpecPublishAll bool
)

var publishAgentSpecCmd = &cobra.Command{
	Use:   "agentspec-publish [agentSpecPath]",
	Short: "Publish an agent spec to Nacos (upload as ZIP)",
	Long:  help.AgentSpecPublish.FormatForCLI("nacos-cli"),
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Error: agent spec path required\n")
			os.Exit(1)
		}
		specPath := args[0]

		nacosClient := mustNewNacosClient()
		agentSpecService := agentspec.NewAgentSpecService(nacosClient)

		if agentSpecPublishAll {
			publishAllAgentSpecs(specPath, agentSpecService)
			return
		}

		publishSingleAgentSpec(specPath, agentSpecService)
	},
}

func publishSingleAgentSpec(specPath string, agentSpecService *agentspec.AgentSpecService) {
	expanded, err := util.ExpandTilde(specPath)
	checkError(err)
	specPath = expanded

	absPath, err := filepath.Abs(specPath)
	checkError(err)

	specName := filepath.Base(absPath)
	fmt.Printf("Publishing agent spec: %s...\n", specName)

	err = agentSpecService.UploadAgentSpec(absPath)
	checkError(err)

	fmt.Printf("Agent spec published successfully!\n")
	fmt.Printf("  Tip: Use the Nacos console to review and go online, or use 'agentspec-list' to verify.\n")
}

func publishAllAgentSpecs(folderPath string, agentSpecService *agentspec.AgentSpecService) {
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
		fmt.Printf("[%d/%d] Publishing agent spec: %s\n", i+1, len(specDirs), specName)
		fmt.Println(strings.Repeat("=", 80))

		specPath := filepath.Join(folderPath, specName)
		err := agentSpecService.UploadAgentSpec(specPath)
		if err != nil {
			fmt.Printf("Publish failed: %v\n", err)
			failedCount++
		} else {
			fmt.Printf("Publish successful!\n")
			successCount++
		}
		fmt.Println()
	}

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("Batch Publish Complete")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Success: %d\n", successCount)
	if failedCount > 0 {
		fmt.Printf("Failed: %d\n", failedCount)
	}
	fmt.Printf("Total: %d\n", len(specDirs))
	fmt.Println()
	fmt.Println("Tip: Use the Nacos console to review and go online, or use 'agentspec-list' to verify.")
}

func init() {
	publishAgentSpecCmd.Flags().BoolVar(&agentSpecPublishAll, "all", false, "Publish all agent specs in the directory")
	rootCmd.AddCommand(publishAgentSpecCmd)
}
