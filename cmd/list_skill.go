package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nacos-group/nacos-cli/internal/client"
	"github.com/nacos-group/nacos-cli/internal/config"
	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/nacos-group/nacos-cli/internal/util"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const defaultDescLimit = 200

var listSkillCmd = newSkillListCommand()

func newSkillListCommand() *cobra.Command {
	command := newSkillListCommandForSession(nil, "", "")
	command.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := cmd.ValidateFlagGroups(); err != nil {
			return err
		}
		rootCmd.PersistentPreRun(cmd, args)
		return nil
	}
	return command
}

// Session commands reuse the authenticated client and preserve interactive ns switches.
func newSkillListCommandForSession(sessionClient *client.NacosClient, sessionProfile, sessionConfig string) *cobra.Command {
	var page, size int
	var name, output string
	var hub, noHub bool
	command := &cobra.Command{
		Use:   "skill-list",
		Short: "List skills using the current profile's source preference",
		Long:  help.SkillList.FormatForCLI("nacos-cli"),
		Args:  cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Reject conflicting source flags before profile setup or other side effects.
			if err := cmd.ValidateFlagGroups(); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			output = strings.ToLower(output)
			if output != "pretty" && output != "json" {
				return fmt.Errorf("unsupported --output %q (expect pretty or json)", output)
			}
			profile, path := sessionProfile, sessionConfig
			if sessionClient == nil {
				profile, path = profileName, configFile
			}
			source, err := resolveSkillListSourceForProfile(cmd, hub, noHub, profile, path)
			if err != nil {
				return err
			}
			nacosClient := sessionClient
			if nacosClient == nil {
				nacosClient = mustNewNacosClient()
			}
			pages, err := skill.NewSkillService(nacosClient).ListSkillsBySource(name, page, size, source)
			if err != nil {
				return err
			}
			if output == "json" && source == config.SkillSourceAll {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(pages)
			}
			for _, entry := range []struct {
				label string
				page  *skill.SkillListResponse
			}{{"Namespace", pages.Namespace}, {"Hub", pages.Hub}} {
				if entry.page == nil {
					continue
				}
				if output == "json" {
					renderSkillListJSON(cmd.OutOrStdout(), entry.page.PageItems, entry.page.TotalCount, page, size)
				} else {
					color := skillListColorEnabled(cmd.OutOrStdout(), sessionClient != nil)
					fmt.Fprintln(cmd.OutOrStdout(), skillListText(color, "36", entry.label+" skills (independent pagination)"))
					renderSkillListPretty(cmd.OutOrStdout(), entry.page.PageItems, entry.page.TotalCount, page, size, color)
				}
			}
			return nil
		},
	}
	command.Flags().IntVar(&page, "page", 1, "Page number for each selected source")
	command.Flags().IntVar(&size, "size", 20, "Page size per source (1-100)")
	command.Flags().StringVar(&name, "name", "", "Filter by skill name (supports wildcard *)")
	command.Flags().StringVar(&output, "output", "pretty", "Output format: pretty | json")
	command.Flags().BoolVar(&hub, "hub", false, "List only Hub skills, overriding the profile")
	command.Flags().BoolVar(&noHub, "no-hub", false, "List only current Namespace skills, overriding the profile")
	command.MarkFlagsMutuallyExclusive("hub", "no-hub")
	return command
}

func resolveSkillListSource(cmd *cobra.Command, hub, noHub bool) (string, error) {
	return resolveSkillListSourceForProfile(cmd, hub, noHub, profileName, configFile)
}

func resolveSkillListSourceForProfile(cmd *cobra.Command, hub, noHub bool, profile, path string) (string, error) {
	if err := cmd.ValidateFlagGroups(); err != nil {
		return "", err
	}
	if hub {
		return config.SkillSourceHub, nil
	}
	if noHub {
		return config.SkillSourceNamespace, nil
	}
	// Load source preferences even when connection flags bypass root's profile loading.
	if path == "" {
		if profile == "" {
			var err error
			profile, err = config.GetCurrentProfile()
			if err != nil {
				return "", err
			}
		}
		var err error
		path, err = config.GetProfileConfigPath(profile)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return config.NormalizeSkillSource("")
		}
	}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		return "", err
	}
	return config.NormalizeSkillSource(cfg.Source)
}

func init() {
	rootCmd.AddCommand(listSkillCmd)
}

// renderSkillListJSON emits the raw page payload so scripts can consume all
// SkillSummary fields returned by the admin list API.
func renderSkillListJSON(w io.Writer, skills []skill.SkillListItem, totalCount, pageNo, pageSize int) {
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
	fmt.Fprintln(w, string(data))
}

// renderSkillListPretty prints a multi-line, human-readable view that surfaces
// governance metadata (latest/editing/reviewing/onlineCnt/enable/scope/...).
func renderSkillListPretty(w io.Writer, skills []skill.SkillListItem, totalCount, pageNo, pageSize int, color bool) {
	totalPages := 0
	if pageSize > 0 {
		totalPages = (totalCount + pageSize - 1) / pageSize
	}

	if len(skills) == 0 {
		if totalPages == 0 {
			fmt.Fprintln(w, skillListText(color, "33", "No skills found"))
		} else {
			fmt.Fprintln(w, skillListText(color, "33", fmt.Sprintf("Page %d is out of range (Total: %d items, Total pages: %d)", pageNo, totalCount, totalPages)))
		}
		return
	}

	asciiMode := os.Getenv("NO_UNICODE_OUTPUT") != ""
	separator := util.SeparatorLine(79, asciiMode)

	fmt.Fprintf(w, "%s %s\n", skillListText(color, "1;36", "Skill List"),
		skillListText(color, "90", fmt.Sprintf("(Page: %d/%d, Total: %d)", pageNo, totalPages, totalCount)))
	fmt.Fprintln(w, skillListText(color, "36", separator))
	for i, s := range skills {
		printSkillListItem(w, (pageNo-1)*pageSize+i+1, s, color)
	}
}

// skillListColorEnabled keeps terminal presentation consistent for direct and REPL commands.
func skillListColorEnabled(w io.Writer, interactive bool) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if interactive {
		return true
	}
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func skillListText(color bool, code, text string) string {
	if !color {
		return text
	}
	return "\033[" + code + "m" + text + "\033[0m"
}

// printSkillListItem retains discovery identity and governance metadata in either presentation.
func printSkillListItem(w io.Writer, idx int, s skill.SkillListItem, color bool) {
	name := s.DisplayName
	if name == "" {
		name = s.Name
	}
	if s.Description != "" {
		desc := truncateDesc(s.Description, defaultDescLimit)
		fmt.Fprintf(w, "%s %s %s\n", skillListText(color, "90", fmt.Sprintf("%3d.", idx)),
			skillListText(color, "32", name), skillListText(color, "90", "- "+desc))
	} else {
		fmt.Fprintf(w, "%s %s\n", skillListText(color, "90", fmt.Sprintf("%3d.", idx)), skillListText(color, "32", name))
	}
	if s.DiscoverySource != "" {
		identity := fmt.Sprintf("source=%s  name=%s", s.DiscoverySource, s.Name)
		if s.DiscoverySource == "HUB" {
			identity += fmt.Sprintf("  hubEntryId=%s  version=%s", s.HubEntryID, s.Version)
		} else {
			identity += "  namespace=" + s.NamespaceID
		}
		fmt.Fprintln(w, "     "+skillListText(color, "90", identity))
	}

	// Line 2: version governance signals.
	statusLabel, statusColor := "enabled", "32"
	if !s.Enable {
		statusLabel, statusColor = "disabled", "31"
	}
	onlineCnt := "-"
	if s.OnlineCnt != nil {
		onlineCnt = fmt.Sprintf("%d", *s.OnlineCnt)
	}
	fmt.Fprintf(w, "     %s%s  %s%s  %s%s  %s%s  %s\n",
		skillListText(color, "90", "latest="), dashIfEmpty(s.Labels["latest"]),
		skillListText(color, "90", "editing="), dashIfEmpty(s.EditingVersion),
		skillListText(color, "90", "reviewing="), dashIfEmpty(s.ReviewingVersion),
		skillListText(color, "90", "online="), onlineCnt,
		skillListText(color, statusColor, "status="+statusLabel),
	)

	// Line 3: governance metadata (printed only when present).
	var meta []string
	if s.Scope != "" {
		meta = append(meta, "scope="+s.Scope)
	}
	if s.BizTags != "" {
		meta = append(meta, "bizTags="+s.BizTags)
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
		fmt.Fprintln(w, "     "+skillListText(color, "90", strings.Join(meta, "  ")))
	}

	// Line 4: extra labels beyond "latest" (e.g. stable=v2).
	if extra := extraLabels(s.Labels); len(extra) > 0 {
		fmt.Fprintln(w, "     "+skillListText(color, "90", "labels: "+strings.Join(extra, ", ")))
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
