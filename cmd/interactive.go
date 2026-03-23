package cmd

import (
	"github.com/nacos-group/nacos-cli/internal/terminal"
	"github.com/spf13/cobra"
)

var interactiveCmd = &cobra.Command{
	Use:   "interactive",
	Short: "Start interactive terminal mode",
	Long:  `Start an interactive terminal for managing Nacos configurations and skills`,
	Run: func(cmd *cobra.Command, args []string) {
		// Create Nacos client
		nacosClient := mustNewNacosClient()

		// Create and start terminal
		term := terminal.NewTerminal(nacosClient)
		if err := term.Start(); err != nil {
			checkError(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(interactiveCmd)
}
