package cmd

import (
	"fmt"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/spf13/cobra"
)

var getConfigCmd = &cobra.Command{
	Use:   "config-get [dataId] [group]",
	Short: "Get a specific configuration",
	Long:  help.ConfigGet.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		dataID := args[0]
		group := args[1]

		// Create Nacos client
		nacosClient := mustNewNacosClient()

		// Get config
		fmt.Printf("Fetching config: %s (%s)...\n\n", dataID, group)
		content, err := nacosClient.GetConfig(dataID, group)
		checkError(err)

		if content == "" {
			fmt.Println("Configuration not found")
			return
		}

		// Display content
		fmt.Println("═══════════════════════════════════════")
		fmt.Printf("Data ID: %s\n", dataID)
		fmt.Printf("Group: %s\n", group)
		fmt.Println("═══════════════════════════════════════")
		fmt.Println(content)
	},
}

func init() {
	rootCmd.AddCommand(getConfigCmd)
}
