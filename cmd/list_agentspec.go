package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nacos-group/nacos-cli/internal/agentspec"
	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/util"
	"github.com/spf13/cobra"
)

var (
	agentSpecListPage   int
	agentSpecListSize   int
	agentSpecListName   string
	agentSpecListOutput string // pretty (default) | json
)

var listAgentSpecCmd = &cobra.Command{
	Use:   "agentspec-list",
	Short: "List all agent specs",
	Long:  help.AgentSpecList.FormatForCLI("nacos-cli"),
	Run: func(cmd *cobra.Command, args []string) {
		nacosClient := mustNewNacosClient()
		agentSpecService := agentspec.NewAgentSpecService(nacosClient)

		specs, totalCount, err := agentSpecService.ListAgentSpecs(agentSpecListName, "", agentSpecListPage, agentSpecListSize)
		checkError(err)

		switch strings.ToLower(agentSpecListOutput) {
		case "json":
			renderAgentSpecListJSON(specs, totalCount, agentSpecListPage, agentSpecListSize)
		case "", "pretty":
			renderAgentSpecListPretty(specs, totalCount, agentSpecListPage, agentSpecListSize)
		default:
			fmt.Fprintf(os.Stderr, "Error: unsupported --output value %q (expect 'pretty' or 'json')\n", agentSpecListOutput)
			os.Exit(1)
		}
	},
}

func init() {
	listAgentSpecCmd.Flags().IntVar(&agentSpecListPage, "page", 1, "Page number (default: 1)")
	listAgentSpecCmd.Flags().IntVar(&agentSpecListSize, "size", 20, "Page size (default: 20)")
	listAgentSpecCmd.Flags().StringVar(&agentSpecListName, "name", "", "Filter by agent spec name (supports wildcard *)")
	listAgentSpecCmd.Flags().StringVar(&agentSpecListOutput, "output", "pretty", "Output format: pretty | json")
	rootCmd.AddCommand(listAgentSpecCmd)
}

// renderAgentSpecListJSON emits the raw page payload so scripts can consume all
// AgentSpecSummary fields returned by the admin list API.
func renderAgentSpecListJSON(specs []agentspec.AgentSpecListItem, totalCount, pageNo, pageSize int) {
	totalPages := 0
	if pageSize > 0 {
		totalPages = (totalCount + pageSize - 1) / pageSize
	}
	payload := map[string]interface{}{
		"totalCount": totalCount,
		"pageNo":     pageNo,
		"pageSize":   pageSize,
		"totalPages": totalPages,
		"pageItems":  specs,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to encode JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

// renderAgentSpecListPretty prints a multi-line, human-readable view that surfaces
// governance metadata (latest/editing/reviewing/onlineCnt/enable/scope/bizTags/...).
func renderAgentSpecListPretty(specs []agentspec.AgentSpecListItem, totalCount, pageNo, pageSize int) {
	totalPages := 0
	if pageSize > 0 {
		totalPages = (totalCount + pageSize - 1) / pageSize
	}

	if len(specs) == 0 {
		if totalPages == 0 {
			fmt.Println("No agent specs found")
		} else {
			fmt.Printf("Page %d is out of range (Total: %d items, Total pages: %d)\n", pageNo, totalCount, totalPages)
		}
		return
	}

	asciiMode := os.Getenv("NO_UNICODE_OUTPUT") != ""
	separator := util.SeparatorLine(79, asciiMode)

	fmt.Printf("AgentSpec List (Page: %d/%d, Total: %d)\n", pageNo, totalPages, totalCount)
	fmt.Println(separator)
	for i, spec := range specs {
		printAgentSpecListItem((pageNo-1)*pageSize+i+1, spec)
	}
}

// printAgentSpecListItem renders one agent spec in up to four lines of human-readable output.
func printAgentSpecListItem(idx int, s agentspec.AgentSpecListItem) {
	if desc := strOrEmpty(s.Description); desc != "" {
		shortDesc := truncateDesc(desc, defaultDescLimit)
		fmt.Printf("%3d. %s - %s\n", idx, s.Name, shortDesc)
	} else {
		fmt.Printf("%3d. %s\n", idx, s.Name)
	}

	// Line 2: version governance signals.
	statusLabel := "enabled"
	if !s.Enable {
		statusLabel = "disabled"
	}
	fmt.Printf("     latest=%s  editing=%s  reviewing=%s  online=%d  status=%s\n",
		dashIfEmpty(s.Labels["latest"]),
		dashIfEmpty(strOrEmpty(s.EditingVersion)),
		dashIfEmpty(strOrEmpty(s.ReviewingVersion)),
		s.OnlineCnt,
		statusLabel,
	)

	// Line 3: governance metadata (printed only when present).
	var meta []string
	if scope := strOrEmpty(s.Scope); scope != "" {
		meta = append(meta, "scope="+scope)
	}
	if tags := strOrEmpty(s.BizTags); tags != "" {
		meta = append(meta, "bizTags="+tags)
	}
	if s.UpdateTime > 0 {
		meta = append(meta, "updated="+time.UnixMilli(s.UpdateTime).Format("2006-01-02 15:04:05"))
	}
	if s.DownloadCount != nil && *s.DownloadCount > 0 {
		meta = append(meta, fmt.Sprintf("downloads=%d", *s.DownloadCount))
	}
	if len(meta) > 0 {
		fmt.Println("     " + strings.Join(meta, "  "))
	}

	// Line 4: extra labels beyond "latest" (e.g. stable=v2).
	if extra := extraLabels(s.Labels); len(extra) > 0 {
		fmt.Println("     labels: " + strings.Join(extra, ", "))
	}
}
