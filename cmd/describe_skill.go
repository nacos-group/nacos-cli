package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/nacos-group/nacos-cli/internal/util"
	"github.com/spf13/cobra"
)

var skillDescribeOutput string // pretty (default) | json

var describeSkillCmd = &cobra.Command{
	Use:   "skill-describe [skillName]",
	Short: "Show detailed info of a skill, including version list and per-version status",
	Long:  help.SkillDescribe.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nacosClient := mustNewNacosClient()
		skillService := skill.NewSkillService(nacosClient)

		detail, err := skillService.DescribeSkill(args[0])
		checkError(err)

		switch strings.ToLower(skillDescribeOutput) {
		case "json":
			renderSkillDetailJSON(detail)
		case "", "pretty":
			renderSkillDetailPretty(detail)
		default:
			fmt.Fprintf(os.Stderr, "Error: unsupported --output value %q (expect 'pretty' or 'json')\n", skillDescribeOutput)
			os.Exit(1)
		}
	},
}

func init() {
	describeSkillCmd.Flags().StringVar(&skillDescribeOutput, "output", "pretty", "Output format: pretty | json")
	rootCmd.AddCommand(describeSkillCmd)
}

// renderSkillDetailJSON emits the raw SkillMeta payload (SkillSummary + versions).
func renderSkillDetailJSON(d *skill.SkillDetail) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to encode JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

// renderSkillDetailPretty prints a two-section view: governance metadata, then
// a version table showing version/status/author/updateTime/commitMsg.
func renderSkillDetailPretty(d *skill.SkillDetail) {
	asciiMode := os.Getenv("NO_UNICODE_OUTPUT") != ""
	separator := util.SeparatorLine(79, asciiMode)

	fmt.Printf("Skill: %s\n", d.Name)
	fmt.Println(separator)
	if d.Description != "" {
		fmt.Printf("  description: %s\n", d.Description)
	}

	// Governance metadata block.
	statusLabel := "enabled"
	if !d.Enable {
		statusLabel = "disabled"
	}
	onlineCnt := "-"
	if d.OnlineCnt != nil {
		onlineCnt = fmt.Sprintf("%d", *d.OnlineCnt)
	}
	fmt.Printf("  latest=%s  editing=%s  reviewing=%s  online=%s  status=%s\n",
		dashIfEmpty(d.Labels["latest"]),
		dashIfEmpty(d.EditingVersion),
		dashIfEmpty(d.ReviewingVersion),
		onlineCnt,
		statusLabel,
	)

	var meta []string
	if d.Scope != "" {
		meta = append(meta, "scope="+d.Scope)
	}
	if d.Owner != "" {
		meta = append(meta, "owner="+d.Owner)
	}
	if d.UpdateTime != nil && *d.UpdateTime > 0 {
		meta = append(meta, "updated="+time.UnixMilli(*d.UpdateTime).Format("2006-01-02 15:04:05"))
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

	versions := sortedVersions(d.Versions)
	widths := computeVersionColumnWidths(versions)
	header := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %s",
		widths.version, "VERSION",
		widths.status, "STATUS",
		widths.author, "AUTHOR",
		widths.updated, "UPDATED",
		"COMMIT")
	fmt.Println(header)
	fmt.Println("  " + util.SeparatorLine(len(header)-2, asciiMode))
	for _, v := range versions {
		fmt.Printf("  %-*s  %-*s  %-*s  %-*s  %s\n",
			widths.version, v.Version,
			widths.status, dashIfEmpty(v.Status),
			widths.author, dashIfEmpty(v.Author),
			widths.updated, formatTimestamp(v.UpdateTime),
			truncateDesc(strings.ReplaceAll(v.CommitMsg, "\n", " "), 60),
		)
	}
}

type versionColumnWidths struct {
	version int
	status  int
	author  int
	updated int
}

func computeVersionColumnWidths(versions []skill.SkillVersionSummary) versionColumnWidths {
	w := versionColumnWidths{version: 7, status: 9, author: 8, updated: 19}
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

// sortedVersions returns versions sorted by updateTime desc (fallback: createTime desc, then name).
func sortedVersions(versions []skill.SkillVersionSummary) []skill.SkillVersionSummary {
	out := make([]skill.SkillVersionSummary, len(versions))
	copy(out, versions)
	sort.SliceStable(out, func(i, j int) bool {
		ti := versionSortKey(out[i])
		tj := versionSortKey(out[j])
		if ti != tj {
			return ti > tj
		}
		return out[i].Version > out[j].Version
	})
	return out
}

func versionSortKey(v skill.SkillVersionSummary) int64 {
	if v.UpdateTime != nil {
		return *v.UpdateTime
	}
	if v.CreateTime != nil {
		return *v.CreateTime
	}
	return 0
}

func formatTimestamp(ts *int64) string {
	if ts == nil || *ts <= 0 {
		return "-"
	}
	return time.UnixMilli(*ts).Format("2006-01-02 15:04:05")
}
