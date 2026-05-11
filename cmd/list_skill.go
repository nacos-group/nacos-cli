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

var (
	skillListPage   int
	skillListSize   int
	skillListName   string
	skillListOutput string // pretty (default) | json
)

const defaultDescLimit = 200

var listSkillCmd = &cobra.Command{
	Use:   "skill-list",
	Short: "List all skills",
	Long:  help.SkillList.FormatForCLI("nacos-cli"),
	Run: func(cmd *cobra.Command, args []string) {
		// Create Nacos client
		nacosClient := mustNewNacosClient()

		// Create skill service
		skillService := skill.NewSkillService(nacosClient)

		// List skills
		skills, totalCount, err := skillService.ListSkills(skillListName, skillListPage, skillListSize)
		checkError(err)

		switch strings.ToLower(skillListOutput) {
		case "json":
			renderSkillListJSON(skills, totalCount, skillListPage, skillListSize)
		case "", "pretty":
			renderSkillListPretty(skills, totalCount, skillListPage, skillListSize)
		default:
			fmt.Fprintf(os.Stderr, "Error: unsupported --output value %q (expect 'pretty' or 'json')\n", skillListOutput)
			os.Exit(1)
		}
	},
}

func init() {
	listSkillCmd.Flags().IntVar(&skillListPage, "page", 1, "Page number (default: 1)")
	listSkillCmd.Flags().IntVar(&skillListSize, "size", 20, "Page size (default: 20)")
	listSkillCmd.Flags().StringVar(&skillListName, "name", "", "Filter by skill name (supports wildcard *)")
	listSkillCmd.Flags().StringVar(&skillListOutput, "output", "pretty", "Output format: pretty | json")
	rootCmd.AddCommand(listSkillCmd)
}

// renderSkillListJSON emits the raw page payload so scripts can consume all
// SkillSummary fields returned by the admin list API.
func renderSkillListJSON(skills []skill.SkillListItem, totalCount, pageNo, pageSize int) {
	totalPages := 0
	if pageSize > 0 {
		totalPages = (totalCount + pageSize - 1) / pageSize
	}
	payload := map[string]interface{}{
		"totalCount": totalCount,
		"pageNo":     pageNo,
		"pageSize":   pageSize,
		"totalPages": totalPages,
		"pageItems":  skills,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to encode JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

// renderSkillListPretty prints a multi-line, human-readable view that surfaces
// governance metadata (latest/editing/reviewing/onlineCnt/enable/scope/...).
func renderSkillListPretty(skills []skill.SkillListItem, totalCount, pageNo, pageSize int) {
	totalPages := 0
	if pageSize > 0 {
		totalPages = (totalCount + pageSize - 1) / pageSize
	}

	if len(skills) == 0 {
		if totalPages == 0 {
			fmt.Println("No skills found")
		} else {
			fmt.Printf("Page %d is out of range (Total: %d items, Total pages: %d)\n", pageNo, totalCount, totalPages)
		}
		return
	}

	asciiMode := os.Getenv("NO_UNICODE_OUTPUT") != ""
	separator := util.SeparatorLine(79, asciiMode)

	fmt.Printf("Skill List (Page: %d/%d, Total: %d)\n", pageNo, totalPages, totalCount)
	fmt.Println(separator)
	for i, s := range skills {
		printSkillListItem((pageNo-1)*pageSize+i+1, s)
	}
}

// printSkillListItem renders one skill in up to four lines of human-readable output.
func printSkillListItem(idx int, s skill.SkillListItem) {
	if s.Description != "" {
		desc := truncateDesc(s.Description, defaultDescLimit)
		fmt.Printf("%3d. %s - %s\n", idx, s.Name, desc)
	} else {
		fmt.Printf("%3d. %s\n", idx, s.Name)
	}

	// Line 2: version governance signals.
	statusLabel := "enabled"
	if !s.Enable {
		statusLabel = "disabled"
	}
	onlineCnt := "-"
	if s.OnlineCnt != nil {
		onlineCnt = fmt.Sprintf("%d", *s.OnlineCnt)
	}
	fmt.Printf("     latest=%s  editing=%s  reviewing=%s  online=%s  status=%s\n",
		dashIfEmpty(s.Labels["latest"]),
		dashIfEmpty(s.EditingVersion),
		dashIfEmpty(s.ReviewingVersion),
		onlineCnt,
		statusLabel,
	)

	// Line 3: governance metadata (printed only when present).
	var meta []string
	if s.Scope != "" {
		meta = append(meta, "scope="+s.Scope)
	}
	if s.Owner != "" {
		meta = append(meta, "owner="+s.Owner)
	}
	if s.UpdateTime != nil && *s.UpdateTime > 0 {
		meta = append(meta, "updated="+time.UnixMilli(*s.UpdateTime).Format("2006-01-02 15:04:05"))
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

func dashIfEmpty(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

func extraLabels(labels map[string]string) []string {
	var keys []string
	for k := range labels {
		if k == "latest" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+labels[k])
	}
	return out
}

// truncateDesc truncates description to maxLen and appends ...... if needed
func truncateDesc(desc string, maxLen int) string {
	runes := []rune(desc)
	if len(runes) <= maxLen {
		return desc
	}
	return string(runes[:maxLen]) + "......"
}
