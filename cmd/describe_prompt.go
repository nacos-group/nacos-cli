package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/prompt"
	"github.com/nacos-group/nacos-cli/internal/util"
	"github.com/spf13/cobra"
)

var promptDescribeOutput string

var describePromptCmd = &cobra.Command{
	Use:   "prompt-describe [promptKey]",
	Short: "Show detailed info of a prompt, including version list and per-version status",
	Long:  help.PromptDescribe.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nacosClient := mustNewNacosClient()
		promptService := prompt.NewPromptService(nacosClient)

		detail, err := promptService.DescribePrompt(args[0])
		checkError(err)

		switch strings.ToLower(promptDescribeOutput) {
		case "json":
			renderPromptDetailJSON(detail)
		case "", "pretty":
			renderPromptDetailPretty(detail)
		default:
			fmt.Fprintf(os.Stderr, "Error: unsupported --output value %q (expect 'pretty' or 'json')\n", promptDescribeOutput)
			os.Exit(1)
		}
	},
}

func init() {
	describePromptCmd.Flags().StringVar(&promptDescribeOutput, "output", "pretty", "Output format: pretty | json")
	rootCmd.AddCommand(describePromptCmd)
}

func renderPromptDetailJSON(d *prompt.PromptDetail) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to encode JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func renderPromptDetailPretty(d *prompt.PromptDetail) {
	asciiMode := os.Getenv("NO_UNICODE_OUTPUT") != ""
	separator := util.SeparatorLine(79, asciiMode)

	key := d.PromptKey
	if key == "" {
		key = d.Name
	}
	fmt.Printf("Prompt: %s\n", key)
	fmt.Println(separator)
	if d.Description != nil && *d.Description != "" {
		fmt.Printf("  description: %s\n", *d.Description)
	}

	// Governance metadata block.
	onlineCnt := "-"
	if d.OnlineCnt != nil {
		onlineCnt = fmt.Sprintf("%d", *d.OnlineCnt)
	}
	fmt.Printf("  latest=%s  editing=%s  reviewing=%s  online=%s\n",
		dashIfEmpty(d.Labels["latest"]),
		dashIfEmpty(d.EditingVersion),
		dashIfEmpty(d.ReviewingVersion),
		onlineCnt,
	)

	var meta []string
	if len(d.BizTags) > 0 {
		meta = append(meta, "bizTags="+strings.Join(d.BizTags, ","))
	}
	if d.Owner != "" {
		meta = append(meta, "owner="+d.Owner)
	}
	if d.GmtModified != nil && *d.GmtModified > 0 {
		meta = append(meta, "updated="+time.UnixMilli(*d.GmtModified).Format("2006-01-02 15:04:05"))
	}
	if d.DownloadCount != nil && *d.DownloadCount > 0 {
		meta = append(meta, fmt.Sprintf("downloads=%d", *d.DownloadCount))
	}
	if len(meta) > 0 {
		fmt.Println("  " + strings.Join(meta, "  "))
	}
	if extra := extraLabels(d.Labels); len(extra) > 0 {
		fmt.Println("  labels: " + strings.Join(extra, ", "))
	}

	// Version table.
	fmt.Println()
	fmt.Println("Versions:")
	if len(d.VersionDetails) == 0 {
		fmt.Println("  (none)")
		return
	}

	versions := sortedPromptVersions(d.VersionDetails)
	widths := computePromptVersionColumnWidths(versions)
	header := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %s",
		widths.version, "VERSION",
		widths.status, "STATUS",
		widths.author, "AUTHOR",
		widths.updated, "UPDATED",
		"COMMIT")
	fmt.Println(header)
	fmt.Println("  " + util.SeparatorLine(len(header)-2, asciiMode))
	for _, v := range versions {
		commitMsg := ""
		if v.CommitMsg != nil {
			commitMsg = *v.CommitMsg
		}
		fmt.Printf("  %-*s  %-*s  %-*s  %-*s  %s\n",
			widths.version, v.Version,
			widths.status, dashIfEmpty(v.Status),
			widths.author, dashIfEmpty(v.SrcUser),
			widths.updated, promptFormatTimestamp(v.GmtModified),
			truncateDesc(strings.ReplaceAll(commitMsg, "\n", " "), 60),
		)
	}
}

type promptVersionColumnWidths struct {
	version int
	status  int
	author  int
	updated int
}

func computePromptVersionColumnWidths(versions []prompt.PromptVersionSummary) promptVersionColumnWidths {
	w := promptVersionColumnWidths{version: 7, status: 9, author: 8, updated: 19}
	for _, v := range versions {
		if n := len(v.Version); n > w.version {
			w.version = n
		}
		if n := len(v.Status); n > w.status {
			w.status = n
		}
		if n := len(v.SrcUser); n > w.author {
			w.author = n
		}
	}
	return w
}

func sortedPromptVersions(versions []prompt.PromptVersionSummary) []prompt.PromptVersionSummary {
	out := make([]prompt.PromptVersionSummary, len(versions))
	copy(out, versions)
	sort.SliceStable(out, func(i, j int) bool {
		ti := promptVersionSortKey(out[i])
		tj := promptVersionSortKey(out[j])
		if ti != tj {
			return ti > tj
		}
		return out[i].Version > out[j].Version
	})
	return out
}

func promptVersionSortKey(v prompt.PromptVersionSummary) int64 {
	if v.GmtModified != nil {
		return *v.GmtModified
	}
	return 0
}

func promptFormatTimestamp(ts *int64) string {
	if ts == nil || *ts <= 0 {
		return "-"
	}
	return time.UnixMilli(*ts).Format("2006-01-02 15:04:05")
}
