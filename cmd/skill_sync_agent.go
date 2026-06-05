package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var skillSyncAgentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage agent directories for skill sync",
}

var skillSyncAgentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered agent directories",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		if len(state.Agents) == 0 {
			fmt.Println("No agent directories registered.")
			fmt.Println("Use 'skill-sync agent add <name> <path>' or run 'skill-sync add' for auto-discovery.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "AGENT\tPATH\tSOURCE\n")
		fmt.Fprintf(w, "-----\t----\t------\n")
		for _, agent := range state.Agents {
			source := "manual"
			if agent.AutoFound {
				source = "auto"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", agent.Name, agent.Path, source)
		}
		w.Flush()
	},
}

var skillSyncAgentAddCmd = &cobra.Command{
	Use:   "add <name> <path>",
	Short: "Add a custom agent directory",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		path := args[1]

		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		if err := state.AddAgent(name, path); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if err := skill.SaveSyncState(state); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to save sync state: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Added agent: %s (%s)\n", name, path)
	},
}

var skillSyncAgentRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an agent directory (does not delete files)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		state, err := skill.LoadSyncState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load sync state: %v\n", err)
			os.Exit(1)
		}

		if err := state.RemoveAgent(name); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if err := skill.SaveSyncState(state); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to save sync state: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Removed agent: %s\n", name)
		fmt.Println("  Note: local files are preserved.")
	},
}

func init() {
	skillSyncAgentCmd.AddCommand(skillSyncAgentListCmd)
	skillSyncAgentCmd.AddCommand(skillSyncAgentAddCmd)
	skillSyncAgentCmd.AddCommand(skillSyncAgentRemoveCmd)
	skillSyncCmd.AddCommand(skillSyncAgentCmd)
}
