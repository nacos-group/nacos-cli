package cmd

import (
	"fmt"

	"github.com/nov11/nacos-cli/internal/agentspec"
	"github.com/nov11/nacos-cli/internal/help"
	"github.com/spf13/cobra"
)

var (
	agentSpecListPage   int
	agentSpecListSize   int
	agentSpecListName   string
	agentSpecListSearch string
)

var listAgentSpecCmd = &cobra.Command{
	Use:   "agentspec-list",
	Short: "List all agent specs",
	Long:  help.AgentSpecList.FormatForCLI("nacos-cli"),
	Run: func(cmd *cobra.Command, args []string) {
		// Create Nacos client
		nacosClient := mustNewNacosClient()

		// Create agentspec service
		agentSpecService := agentspec.NewAgentSpecService(nacosClient)

		// List agent specs
		specs, totalCount, err := agentSpecService.ListAgentSpecs(agentSpecListName, agentSpecListSearch, agentSpecListPage, agentSpecListSize)
		checkError(err)

		// Display results
		if len(specs) == 0 {
			fmt.Println("No agent specs found")
			return
		}

		fmt.Printf("AgentSpec List (Total: %d)\n", totalCount)
		fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
		for i, spec := range specs {
			enableStr := "enabled"
			if !spec.Enable {
				enableStr = "disabled"
			}
			if spec.Description != nil && *spec.Description != "" {
				desc := truncateDesc(*spec.Description, defaultDescLimit)
				fmt.Printf("%3d. %s - %s [%s, online:%d]\n", i+1, spec.Name, desc, enableStr, spec.OnlineCnt)
			} else {
				fmt.Printf("%3d. %s [%s, online:%d]\n", i+1, spec.Name, enableStr, spec.OnlineCnt)
			}
		}
	},
}

func init() {
	listAgentSpecCmd.Flags().IntVar(&agentSpecListPage, "page", 1, "Page number (default: 1)")
	listAgentSpecCmd.Flags().IntVar(&agentSpecListSize, "size", 20, "Page size (default: 20)")
	listAgentSpecCmd.Flags().StringVar(&agentSpecListName, "name", "", "Filter by agent spec name")
	listAgentSpecCmd.Flags().StringVar(&agentSpecListSearch, "search", "", "Search mode: accurate or blur")
	rootCmd.AddCommand(listAgentSpecCmd)
}
