package cmd

import (
	"bufio"
	"fmt"
	"os"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/spf13/cobra"
)

var setConfigFile string

var setConfigCmd = &cobra.Command{
	Use:   "config-set [dataId] [group]",
	Short: "Publish a configuration to Nacos",
	Long:  help.ConfigSet.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		dataID := args[0]
		group := args[1]

		content, err := readSetConfigContent()
		checkError(err)

		if content == "" {
			fmt.Fprintf(os.Stderr, "Error: config content is empty (use --file or stdin)\n")
			os.Exit(1)
		}

		// Create Nacos client
		nacosClient := mustNewNacosClient()

		fmt.Printf("Publishing config: %s (%s)...\n", dataID, group)
		err = nacosClient.PublishConfig(dataID, group, content)
		checkError(err)

		fmt.Println("Configuration published successfully")
	},
}

func readSetConfigContent() (string, error) {
	if setConfigFile != "" {
		data, err := os.ReadFile(setConfigFile)
		if err != nil {
			return "", fmt.Errorf("read file %s: %w", setConfigFile, err)
		}
		return string(data), nil
	}
	// Read from stdin
	var content string
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		if content != "" {
			content += "\n"
		}
		content += scanner.Text()
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return content, nil
}

func init() {
	setConfigCmd.Flags().StringVarP(&setConfigFile, "file", "f", "", "Path to config file (default: read from stdin)")
	rootCmd.AddCommand(setConfigCmd)
}
