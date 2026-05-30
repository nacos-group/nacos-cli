package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `Generate shell completion script for nacos-cli.

This command generates shell completion scripts that enable tab completion
for commands and flags in your shell.

Examples:
  # Generate bash completion
  nacos-cli completion bash > /etc/bash_completion.d/nacos-cli

  # Generate zsh completion
  nacos-cli completion zsh > /usr/local/share/zsh/site-functions/_nacos-cli

  # Generate fish completion
  nacos-cli completion fish > ~/.config/fish/completions/nacos-cli.fish

  # For macOS bash users
  nacos-cli completion bash > /usr/local/etc/bash_completion.d/nacos-cli

Setup instructions:
  Bash:
    $ source <(nacos-cli completion bash)
    # To load completions for every session, add to ~/.bashrc:
    $ echo 'source <(nacos-cli completion bash)' >> ~/.bashrc

  Zsh:
    # If shell completion is not already enabled, enable it:
    $ echo "autoload -U compinit; compinit" >> ~/.zshrc
    # Load completions for every session:
    $ nacos-cli completion zsh > "${fpath[1]}/_nacos-cli"
    $ source ~/.zshrc

  Fish:
    $ nacos-cli completion fish | source
    # To load completions for every session:
    $ nacos-cli completion fish > ~/.config/fish/completions/nacos-cli.fish`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			_ = cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			_ = cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			_ = cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			_ = cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
