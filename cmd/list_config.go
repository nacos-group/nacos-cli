package cmd

import (
	"fmt"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/spf13/cobra"
)

var (
	configListPage   int
	configListSize   int
	configListDataID string
	configListGroup  string
)

var listConfigCmd = &cobra.Command{
	Use:   "config-list",
	Short: "List all configurations",
	Long:  help.ConfigList.FormatForCLI("nacos-cli"),
	Run: func(cmd *cobra.Command, args []string) {
		// Create Nacos client
		nacosClient := mustNewNacosClient()

		// List configs
		configs, err := nacosClient.ListConfigs(configListDataID, configListGroup, "", configListPage, configListSize)
		checkError(err)

		// Display results
		if len(configs.PageItems) == 0 {
			fmt.Println("No configurations found")
			return
		}

		fmt.Printf("Configuration List (Total: %d)\n", configs.TotalCount)
		fmt.Println("═══════════════════════════════════════════════════════════════")
		fmt.Printf("%-5s %-30s %-20s %-10s\n", "No.", "Data ID", "Group", "Type")
		fmt.Println("───────────────────────────────────────────────────────────────")

		for i, config := range configs.PageItems {
			groupName := config.GroupName
			if groupName == "" {
				groupName = config.Group
			}

			dataID := config.DataID
			if len(dataID) > 28 {
				dataID = dataID[:25] + "..."
			}

			if len(groupName) > 18 {
				groupName = groupName[:15] + "..."
			}

			fmt.Printf("%-5d %-30s %-20s %-10s\n", i+1, dataID, groupName, config.Type)
		}
	},
}

func init() {
	listConfigCmd.Flags().IntVar(&configListPage, "page", 1, "Page number (default: 1)")
	listConfigCmd.Flags().IntVar(&configListSize, "size", 20, "Page size (default: 20)")
	listConfigCmd.Flags().StringVar(&configListDataID, "data-id", "", "Filter by data ID (supports wildcard *, e.g. 'resource*')")
	listConfigCmd.Flags().StringVar(&configListGroup, "group", "", "Filter by group (supports wildcard *, e.g. 'skill_*')")
	rootCmd.AddCommand(listConfigCmd)
}
