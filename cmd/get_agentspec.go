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
	getAgentSpecOutput  string
	getAgentSpecVersion string
	getAgentSpecLabel   string
)

var getAgentSpecCmd = &cobra.Command{
	Use:   "agentspec-get [name...]",
	Short: "Get one or more agent specs and download them locally",
	Long:  help.AgentSpecGet.FormatForCLI("nacos-cli"),
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		specNames := args

		// Default output directory
		if getAgentSpecOutput == "" {
			homeDir, err := os.UserHomeDir()
			checkError(err)
			getAgentSpecOutput = filepath.Join(homeDir, ".agentspecs")
		} else {
			// Expand ~ to home directory
			expanded, err := util.ExpandTilde(getAgentSpecOutput)
			checkError(err)
			getAgentSpecOutput = expanded
		}

		// Create Nacos client
		nacosClient := mustNewNacosClient()

		// Create agentspec service
		agentSpecService := agentspec.NewAgentSpecService(nacosClient)

		// Track results
		var successCount, failCount int
		var failedSpecs []string

		// Process each agent spec
		for i, specName := range specNames {
			if len(specNames) > 1 {
				fmt.Printf("\n[%d/%d] ", i+1, len(specNames))
			}
			fmt.Printf("Fetching agent spec: %s...\n", specName)
			err := agentSpecService.GetAgentSpec(specName, getAgentSpecOutput, getAgentSpecVersion, getAgentSpecLabel)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to download agent spec '%s': %v\n", specName, err)
				failCount++
				failedSpecs = append(failedSpecs, specName)
			} else {
				specPath := filepath.Join(getAgentSpecOutput, specName)
				fmt.Printf("Agent spec downloaded successfully!\n")
				fmt.Printf("  Location: %s\n", specPath)
				successCount++
			}
		}

		// Summary
		if len(specNames) > 1 {
			fmt.Printf("\n========== Summary ==========\n")
			fmt.Printf("Total: %d | Success: %d | Failed: %d\n", len(specNames), successCount, failCount)
			if failCount > 0 {
				fmt.Printf("Failed agent specs: %s\n", strings.Join(failedSpecs, ", "))
			}
		}

		// Exit with error if any spec failed
		if failCount > 0 {
			os.Exit(1)
		}
	},
}

func init() {
	getAgentSpecCmd.Flags().StringVarP(&getAgentSpecOutput, "output", "o", "", "Output directory (default: ~/.agentspecs)")
	getAgentSpecCmd.Flags().StringVar(&getAgentSpecVersion, "version", "", "Specific version to download (e.g. v1, v2)")
	getAgentSpecCmd.Flags().StringVar(&getAgentSpecLabel, "label", "", "Route label to resolve version (e.g. latest, stable)")
	rootCmd.AddCommand(getAgentSpecCmd)
}
