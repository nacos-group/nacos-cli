package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nov11/nacos-cli/internal/agentspec"
	"github.com/nov11/nacos-cli/internal/help"
	"github.com/nov11/nacos-cli/internal/util"
	"github.com/spf13/cobra"
)

var (
	agentSpecUploadAll bool
)

var uploadAgentSpecCmd = &cobra.Command{
	Use:   "agentspec-upload [agentSpecPath]",
	Short: "Upload an agent spec to Nacos (upload as ZIP)",
	Long:  help.AgentSpecUpload.FormatForCLI("nacos-cli"),
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Error: agent spec path required\n")
			os.Exit(1)
		}
		specPath := args[0]

		// Create Nacos client
		nacosClient := mustNewNacosClient()

		// Create agentspec service
		agentSpecService := agentspec.NewAgentSpecService(nacosClient)

		// Handle batch upload
		if agentSpecUploadAll {
			uploadAllAgentSpecs(specPath, agentSpecService)
			return
		}

		// Single agent spec upload
		uploadSingleAgentSpec(specPath, agentSpecService)
	},
}

func uploadSingleAgentSpec(specPath string, agentSpecService *agentspec.AgentSpecService) {
	// Expand ~ to home directory
	expanded, err := util.ExpandTilde(specPath)
	checkError(err)
	specPath = expanded

	// Expand path
	absPath, err := filepath.Abs(specPath)
	checkError(err)

	specName := filepath.Base(absPath)
	fmt.Printf("Uploading agent spec: %s...\n", specName)

	err = agentSpecService.UploadAgentSpec(absPath)
	checkError(err)

	fmt.Printf("Agent spec uploaded successfully!\n")
	fmt.Printf("  Tip: Use the Nacos console to review and go online, or use 'agentspec-list' to verify.\n")
}

func uploadAllAgentSpecs(folderPath string, agentSpecService *agentspec.AgentSpecService) {
	// Expand ~ to home directory
	expanded, err := util.ExpandTilde(folderPath)
	checkError(err)
	folderPath = expanded

	// List subdirectories
	entries, err := os.ReadDir(folderPath)
	checkError(err)

	var specDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Check if manifest.json exists
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
		err := agentSpecService.UploadAgentSpec(specPath)
		if err != nil {
			fmt.Printf("Upload failed: %v\n", err)
			failedCount++
		} else {
			fmt.Printf("Upload successful!\n")
			successCount++
		}
		fmt.Println()
	}

	// Summary
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("Batch Upload Complete")
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
	uploadAgentSpecCmd.Flags().BoolVar(&agentSpecUploadAll, "all", false, "Upload all agent specs in the directory")
	rootCmd.AddCommand(uploadAgentSpecCmd)
}
