package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nacos-group/nacos-cli/internal/agentspec"
	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/util"
	"github.com/spf13/cobra"
)

var agentSpecDescribeOutput string // pretty (default) | json

var describeAgentSpecCmd = &cobra.Command{
	Use:   "agentspec-describe [agentSpecName]",
	Short: "Show detailed info of an agent spec, including version list and per-version status",
	Long:  help.AgentSpecDescribe.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nacosClient := mustNewNacosClient()
		agentSpecService := agentspec.NewAgentSpecService(nacosClient)

		detail, err := agentSpecService.DescribeAgentSpec(args[0])
		checkError(err)

		switch strings.ToLower(agentSpecDescribeOutput) {
		case "json":
			renderAgentSpecDetailJSON(detail)
		case "", "pretty":
			renderAgentSpecDetailPretty(detail)
		default:
			fmt.Fprintf(os.Stderr, "Error: unsupported --output value %q (expect 'pretty' or 'json')\n", agentSpecDescribeOutput)
			os.Exit(1)
		}
	},
}

func init() {
	describeAgentSpecCmd.Flags().StringVar(&agentSpecDescribeOutput, "output", "pretty", "Output format: pretty | json")
	rootCmd.AddCommand(describeAgentSpecCmd)
}

// renderAgentSpecDetailJSON emits the raw AgentSpecMeta payload (AgentSpecSummary + versions).
func renderAgentSpecDetailJSON(d *agentspec.AgentSpecDetail) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to encode JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

// renderAgentSpecDetailPretty prints a two-section view: governance metadata, then
// a version table showing version/status/author/updateTime/description.
func renderAgentSpecDetailPretty(d *agentspec.AgentSpecDetail) {
	asciiMode := os.Getenv("NO_UNICODE_OUTPUT") != ""
	separator := util.SeparatorLine(79, asciiMode)

	fmt.Printf("AgentSpec: %s\n", d.Name)
	fmt.Println(separator)
	if desc := strOrEmpty(d.Description); desc != "" {
		fmt.Printf("  description: %s\n", desc)
	}

	// Governance metadata block.
	statusLabel := "enabled"
	if !d.Enable {
		statusLabel = "disabled"
	}
	fmt.Printf("  latest=%s  editing=%s  reviewing=%s  online=%d  status=%s\n",
		dashIfEmpty(d.Labels["latest"]),
		dashIfEmpty(strOrEmpty(d.EditingVersion)),
		dashIfEmpty(strOrEmpty(d.ReviewingVersion)),
		d.OnlineCnt,
		statusLabel,
	)

	var meta []string
	if scope := strOrEmpty(d.Scope); scope != "" {
		meta = append(meta, "scope="+scope)
	}
	if tags := strOrEmpty(d.BizTags); tags != "" {
		meta = append(meta, "bizTags="+tags)
	}
	if d.UpdateTime > 0 {
		meta = append(meta, "updated="+time.UnixMilli(d.UpdateTime).Format("2006-01-02 15:04:05"))
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
	if len(d.Versions) == 0 {
		fmt.Println("  (none)")
		return
	}

	versions := sortedAgentSpecVersions(d.Versions)
	widths := computeAgentSpecVersionColumnWidths(versions)
	header := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %s",
		widths.version, "VERSION",
		widths.status, "STATUS",
		widths.author, "AUTHOR",
		widths.updated, "UPDATED",
		"DESCRIPTION")
	fmt.Println(header)
	fmt.Println("  " + util.SeparatorLine(len(header)-2, asciiMode))
	for _, v := range versions {
		fmt.Printf("  %-*s  %-*s  %-*s  %-*s  %s\n",
			widths.version, v.Version,
			widths.status, dashIfEmpty(v.Status),
			widths.author, dashIfEmpty(v.Author),
			widths.updated, formatTimestamp(v.UpdateTime),
			truncateDesc(strings.ReplaceAll(v.Description, "\n", " "), 60),
		)
	}
}

type agentSpecVersionColumnWidths struct {
	version int
	status  int
	author  int
	updated int
}

func computeAgentSpecVersionColumnWidths(versions []agentspec.AgentSpecVersionSummary) agentSpecVersionColumnWidths {
	w := agentSpecVersionColumnWidths{version: 7, status: 9, author: 8, updated: 19}
	for _, v := range versions {
		if n := len(v.Version); n > w.version {
			w.version = n
		}
		if n := len(v.Status); n > w.status {
			w.status = n
		}
		if n := len(v.Author); n > w.author {
			w.author = n
		}
	}
	return w
}

// sortedAgentSpecVersions returns versions sorted by updateTime desc (fallback: createTime desc, then name).
func sortedAgentSpecVersions(versions []agentspec.AgentSpecVersionSummary) []agentspec.AgentSpecVersionSummary {
	out := make([]agentspec.AgentSpecVersionSummary, len(versions))
	copy(out, versions)
	sort.SliceStable(out, func(i, j int) bool {
		ti := agentSpecVersionSortKey(out[i])
		tj := agentSpecVersionSortKey(out[j])
		if ti != tj {
			return ti > tj
		}
		return out[i].Version > out[j].Version
	})
	return out
}

func agentSpecVersionSortKey(v agentspec.AgentSpecVersionSummary) int64 {
	if v.UpdateTime != nil {
		return *v.UpdateTime
	}
	if v.CreateTime != nil {
		return *v.CreateTime
	}
	return 0
}

// strOrEmpty dereferences a *string, returning "" if the pointer is nil.
func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
