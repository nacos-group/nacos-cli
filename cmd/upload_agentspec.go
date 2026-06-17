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

var agentSpecUploadAll bool

var uploadAgentSpecCmd = &cobra.Command{
	Use:               "agentspec-upload [agentSpecPath]",
	Short:             "Upload an agent spec to Nacos (as ZIP, creates an editing draft)",
	Long:              help.AgentSpecUpload.FormatForCLI("nacos-cli"),
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completePathArg(0),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Error: agent spec path required\n")
			os.Exit(1)
		}
		specPath := args[0]

		nacosClient := mustNewNacosClient()
		agentSpecService := agentspec.NewAgentSpecService(nacosClient)

		if agentSpecUploadAll {
			uploadAllAgentSpecs(specPath, agentSpecService)
			return
		}
		uploadSingleAgentSpec(specPath, agentSpecService)
	},
}

func uploadSingleAgentSpec(specPath string, agentSpecService *agentspec.AgentSpecService) {
	expanded, err := util.ExpandTilde(specPath)
	checkError(err)
	specPath = expanded

	absPath, err := filepath.Abs(specPath)
	checkError(err)

	specName := deriveAgentSpecNameFromPath(absPath)
	fmt.Printf("Uploading agent spec: %s...\n", specName)

	err = agentSpecService.UploadAgentSpec(absPath)
	checkError(err)

	fmt.Printf("Agent spec draft uploaded successfully!\n")
	fmt.Printf("  Tip: Use 'agentspec-review %s' to submit the draft for review.\n", specName)
}

func uploadAllAgentSpecs(folderPath string, agentSpecService *agentspec.AgentSpecService) {
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
		fmt.Printf("[%d/%d] Uploading agent spec: %s\n", i+1, len(specDirs), specName)
		fmt.Println(strings.Repeat("=", 80))

		specPath := filepath.Join(folderPath, specName)
		if err := agentSpecService.UploadAgentSpec(specPath); err != nil {
			fmt.Printf("Upload failed: %v\n", err)
			failedCount++
		} else {
			fmt.Printf("Upload successful!\n")
			successCount++
		}
		fmt.Println()
	}

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("Batch Upload Complete")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Success: %d\n", successCount)
	if failedCount > 0 {
		fmt.Printf("Failed: %d\n", failedCount)
	}
	fmt.Printf("Total: %d\n", len(specDirs))
	fmt.Println()
	fmt.Println("Tip: Use 'agentspec-review <agentSpecName>' to submit a draft for review.")
}

// deriveAgentSpecNameFromPath returns the agent spec name from a directory path or a zip file path.
func deriveAgentSpecNameFromPath(absPath string) string {
	base := filepath.Base(absPath)
	if strings.HasSuffix(strings.ToLower(base), ".zip") {
		return strings.TrimSuffix(base, filepath.Ext(base))
	}
	return base
}

func init() {
	uploadAgentSpecCmd.Flags().BoolVar(&agentSpecUploadAll, "all", false, "Upload all agent specs in the directory")
	rootCmd.AddCommand(uploadAgentSpecCmd)
}
