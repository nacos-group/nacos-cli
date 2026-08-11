package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/spf13/cobra"
)

var configGetOutput string // pretty (default) | json | raw

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

		output := strings.ToLower(configGetOutput)
		if output == "" {
			output = "pretty"
		}
		if output == "pretty" {
			fmt.Printf("Fetching config: %s (%s)...\n\n", dataID, group)
		}

		content, err := nacosClient.GetConfig(dataID, group)
		checkError(err)

		switch output {
		case "pretty":
			renderConfigGetPretty(dataID, group, content)
		case "json":
			renderConfigGetJSON(dataID, group, content)
		case "raw":
			renderConfigGetRaw(content)
		default:
			fmt.Fprintf(os.Stderr, "Error: unsupported --output value %q (expect 'pretty', 'json' or 'raw')\n", configGetOutput)
			os.Exit(1)
		}
	},
}

func init() {
	getConfigCmd.Flags().StringVar(&configGetOutput, "output", "pretty", "Output format: pretty | json | raw")
	rootCmd.AddCommand(getConfigCmd)
}

// renderConfigGetPretty prints a human-readable header frame followed by the
// config content.
func renderConfigGetPretty(dataID, group, content string) {
	if content == "" {
		fmt.Println("Configuration not found")
		return
	}
	fmt.Println("═══════════════════════════════════════")
	fmt.Printf("Data ID: %s\n", dataID)
	fmt.Printf("Group: %s\n", group)
	fmt.Println("═══════════════════════════════════════")
	fmt.Println(content)
}

// renderConfigGetRaw prints the config content verbatim with no decoration, so
// it can be piped to a file or another tool without contamination.
func renderConfigGetRaw(content string) {
	if content == "" {
		return
	}
	fmt.Print(content)
}

// renderConfigGetJSON emits the raw {dataId, group, content} payload so scripts
// can consume it without parsing decorative output.
func renderConfigGetJSON(dataID, group, content string) {
	data, err := json.MarshalIndent(map[string]string{
		"dataId":  dataID,
		"group":   group,
		"content": content,
	}, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to encode JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}
