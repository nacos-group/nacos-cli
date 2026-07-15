package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/prompt"
	"github.com/nacos-group/nacos-cli/internal/util"
	"github.com/spf13/cobra"
)

var (
	promptListPage   int
	promptListSize   int
	promptListName   string
	promptListOutput string
)

var listPromptCmd = &cobra.Command{
	Use:   "prompt-list",
	Short: "List all prompts",
	Long:  help.PromptList.FormatForCLI("nacos-cli"),
	Run: func(cmd *cobra.Command, args []string) {
		nacosClient := mustNewNacosClient()
		promptService := prompt.NewPromptService(nacosClient)

		prompts, totalCount, err := promptService.ListPrompts(promptListName, promptListPage, promptListSize)
		checkError(err)

		switch strings.ToLower(promptListOutput) {
		case "json":
			renderPromptListJSON(prompts, totalCount, promptListPage, promptListSize)
		case "", "pretty":
			renderPromptListPretty(prompts, totalCount, promptListPage, promptListSize)
		default:
			fmt.Fprintf(os.Stderr, "Error: unsupported --output value %q (expect 'pretty' or 'json')\n", promptListOutput)
			os.Exit(1)
		}
	},
}

func init() {
	listPromptCmd.Flags().IntVar(&promptListPage, "page", 1, "Page number (default: 1)")
	listPromptCmd.Flags().IntVar(&promptListSize, "size", 20, "Page size (default: 20)")
	listPromptCmd.Flags().StringVar(&promptListName, "name", "", "Filter by prompt key (exact match)")
	listPromptCmd.Flags().StringVar(&promptListOutput, "output", "pretty", "Output format: pretty | json")
	rootCmd.AddCommand(listPromptCmd)
}

func renderPromptListJSON(prompts []prompt.PromptListItem, totalCount, pageNo, pageSize int) {
	totalPages := 0
	if pageSize > 0 {
		totalPages = (totalCount + pageSize - 1) / pageSize
	}
	payload := map[string]interface{}{
		"totalCount": totalCount,
		"pageNo":     pageNo,
		"pageSize":   pageSize,
		"totalPages": totalPages,
		"pageItems":  prompts,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to encode JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func renderPromptListPretty(prompts []prompt.PromptListItem, totalCount, pageNo, pageSize int) {
	totalPages := 0
	if pageSize > 0 {
		totalPages = (totalCount + pageSize - 1) / pageSize
	}

	if len(prompts) == 0 {
		if totalPages == 0 {
			fmt.Println("No prompts found")
		} else {
			fmt.Printf("Page %d is out of range (Total: %d items, Total pages: %d)\n", pageNo, totalCount, totalPages)
		}
		return
	}

	asciiMode := os.Getenv("NO_UNICODE_OUTPUT") != ""
	separator := util.SeparatorLine(79, asciiMode)

	fmt.Printf("Prompt List (Page: %d/%d, Total: %d)\n", pageNo, totalPages, totalCount)
	fmt.Println(separator)
	for i, p := range prompts {
		printPromptListItem((pageNo-1)*pageSize+i+1, p)
	}
}

func printPromptListItem(idx int, p prompt.PromptListItem) {
	key := p.PromptKey
	if key == "" {
		key = p.Name
	}
	if p.Description != nil && *p.Description != "" {
		desc := truncateDesc(*p.Description, defaultDescLimit)
		fmt.Printf("%3d. %s - %s\n", idx, key, desc)
	} else {
		fmt.Printf("%3d. %s\n", idx, key)
	}

	// Line 2: version governance signals.
	onlineCnt := "-"
	if p.OnlineCnt != nil {
		onlineCnt = fmt.Sprintf("%d", *p.OnlineCnt)
	}
	fmt.Printf("     latest=%s  editing=%s  reviewing=%s  online=%s\n",
		dashIfEmpty(p.Labels["latest"]),
		dashIfEmpty(p.EditingVersion),
		dashIfEmpty(p.ReviewingVersion),
		onlineCnt,
	)

	// Line 3: governance metadata.
	var meta []string
	if len(p.BizTags) > 0 {
		meta = append(meta, "bizTags="+strings.Join(p.BizTags, ","))
	}
	if p.Owner != "" {
		meta = append(meta, "owner="+p.Owner)
	}
	if p.GmtModified != nil && *p.GmtModified > 0 {
		meta = append(meta, "updated="+time.UnixMilli(*p.GmtModified).Format("2006-01-02 15:04:05"))
	}
	if p.DownloadCount != nil && *p.DownloadCount > 0 {
		meta = append(meta, fmt.Sprintf("downloads=%d", *p.DownloadCount))
	}
	if len(meta) > 0 {
		fmt.Println("     " + strings.Join(meta, "  "))
	}

	// Line 4: extra labels beyond "latest".
	if extra := extraLabels(p.Labels); len(extra) > 0 {
		fmt.Println("     labels: " + strings.Join(extra, ", "))
	}
}
