package cmd

import (
	"github.com/nacos-group/nacos-cli/internal/client"
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
		term := newInteractiveTerminal(nacosClient, currentTerminalProfileName(), configFile)
		if err := term.Start(); err != nil {
			checkError(err)
		}
	},
}

func newInteractiveTerminal(nacosClient *client.NacosClient, profile, path string) *terminal.Terminal {
	term := terminal.NewTerminalWithProfile(nacosClient, profile)
	term.SetSkillListHandler(func(args []string) error {
		command := newSkillListCommandForSession(nacosClient, profile, path)
		command.SilenceUsage, command.SilenceErrors = true, true
		command.SetArgs(args)
		return command.Execute()
	})
	term.SetSkillGetHandler(func(args []string, readSelection func(string) (string, error)) error {
		command := newSkillGetCommandForSession(nacosClient, profile, path, readSelection)
		command.SilenceUsage, command.SilenceErrors = true, true
		command.SetArgs(args)
		return command.Execute()
	})
	term.SetAgentSpecListHandler(func(args []string) error {
		command := newAgentSpecListCommandForSession(nacosClient, profile, path)
		command.SilenceUsage, command.SilenceErrors = true, true
		command.SetArgs(args)
		return command.Execute()
	})
	term.SetAgentSpecGetHandler(func(args []string, readSelection func(string) (string, error)) error {
		command := newAgentSpecGetCommandForSession(nacosClient, profile, path, readSelection)
		command.SilenceUsage, command.SilenceErrors = true, true
		command.SetArgs(args)
		return command.Execute()
	})

	return term
}

func init() {
	rootCmd.AddCommand(interactiveCmd)
}
