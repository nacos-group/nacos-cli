package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nacos-group/nacos-cli/internal/agentspec"
	"github.com/nacos-group/nacos-cli/internal/client"
	"github.com/nacos-group/nacos-cli/internal/config"
	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/util"
	"github.com/spf13/cobra"
)

var listAgentSpecCmd = newAgentSpecListCommand()

func newAgentSpecListCommand() *cobra.Command {
	command := newAgentSpecListCommandForSession(nil, "", "")
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
func newAgentSpecListCommandForSession(sessionClient *client.NacosClient, sessionProfile, sessionConfig string) *cobra.Command {
	var page, size int
	var name, output string
	var hub, noHub bool
	command := &cobra.Command{
		Use:   "agentspec-list",
		Short: "List agent specs using the current profile's source preference",
		Long:  help.AgentSpecList.FormatForCLI("nacos-cli"),
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
			pages, err := agentspec.NewAgentSpecService(nacosClient).ListAgentSpecsBySource(name, page, size, source)
			if err != nil {
				return err
			}
			if output == "json" && source == config.SkillSourceAll {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(pages)
			}
			for _, entry := range []struct {
				label string
				page  *agentspec.AgentSpecListResponse
			}{{"Namespace", pages.Namespace}, {"Hub", pages.Hub}} {
				if entry.page == nil {
					continue
				}
				if output == "json" {
					renderAgentSpecListJSON(cmd.OutOrStdout(), entry.page.PageItems, entry.page.TotalCount, page, size)
				} else {
					color := skillListColorEnabled(cmd.OutOrStdout(), sessionClient != nil)
					fmt.Fprintln(cmd.OutOrStdout(), skillListText(color, "36", entry.label+" agent specs (independent pagination)"))
					renderAgentSpecListPretty(cmd.OutOrStdout(), entry.page.PageItems, entry.page.TotalCount, page, size, color)
				}
			}
			return nil
		},
	}
	command.Flags().IntVar(&page, "page", 1, "Page number for each selected source")
	command.Flags().IntVar(&size, "size", 20, "Page size per source (1-100)")
	command.Flags().StringVar(&name, "name", "", "Filter by agent spec name")
	command.Flags().StringVar(&output, "output", "pretty", "Output format: pretty | json")
	command.Flags().BoolVar(&hub, "hub", false, "List only Hub agent specs, overriding the profile")
	command.Flags().BoolVar(&noHub, "no-hub", false, "List only current Namespace agent specs, overriding the profile")
	command.MarkFlagsMutuallyExclusive("hub", "no-hub")
	return command
}

func init() { rootCmd.AddCommand(listAgentSpecCmd) }

// renderAgentSpecListJSON emits the raw page payload so scripts can consume all
// AgentSpecSummary fields returned by the admin list API.
func renderAgentSpecListJSON(w io.Writer, specs []agentspec.AgentSpecListItem, totalCount, pageNo, pageSize int) {
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
	fmt.Fprintln(w, string(data))
}

// renderAgentSpecListPretty prints a multi-line, human-readable view that surfaces
// governance metadata (latest/editing/reviewing/onlineCnt/enable/scope/...).
func renderAgentSpecListPretty(w io.Writer, specs []agentspec.AgentSpecListItem, totalCount, pageNo, pageSize int, color bool) {
	totalPages := 0
	if pageSize > 0 {
		totalPages = (totalCount + pageSize - 1) / pageSize
	}

	if len(specs) == 0 {
		if totalPages == 0 {
			fmt.Fprintln(w, skillListText(color, "33", "No specs found"))
		} else {
			fmt.Fprintln(w, skillListText(color, "33", fmt.Sprintf("Page %d is out of range (Total: %d items, Total pages: %d)", pageNo, totalCount, totalPages)))
		}
		return
	}

	asciiMode := os.Getenv("NO_UNICODE_OUTPUT") != ""
	separator := util.SeparatorLine(79, asciiMode)

	fmt.Fprintf(w, "%s %s\n", skillListText(color, "1;36", "AgentSpec List"),
		skillListText(color, "90", fmt.Sprintf("(Page: %d/%d, Total: %d)", pageNo, totalPages, totalCount)))
	fmt.Fprintln(w, skillListText(color, "36", separator))
	for i, s := range specs {
		printAgentSpecListItem(w, (pageNo-1)*pageSize+i+1, s, color)
	}
}

// printAgentSpecListItem retains discovery identity and governance metadata in either presentation.
func printAgentSpecListItem(w io.Writer, idx int, s agentspec.AgentSpecListItem, color bool) {
	name := s.DisplayName
	if name == "" {
		name = s.Name
	}
	if strOrEmpty(s.Description) != "" {
		desc := truncateDesc(strOrEmpty(s.Description), defaultDescLimit)
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
			identity += "  namespace=" + s.NamespaceId
		}
		fmt.Fprintln(w, "     "+skillListText(color, "90", identity))
	}

	// Line 2: version governance signals.
	statusLabel, statusColor := "enabled", "32"
	if !s.Enable {
		statusLabel, statusColor = "disabled", "31"
	}
	onlineCnt := fmt.Sprintf("%d", s.OnlineCnt)
	fmt.Fprintf(w, "     %s%s  %s%s  %s%s  %s%s  %s\n",
		skillListText(color, "90", "latest="), dashIfEmpty(s.Labels["latest"]),
		skillListText(color, "90", "editing="), dashIfEmpty(strOrEmpty(s.EditingVersion)),
		skillListText(color, "90", "reviewing="), dashIfEmpty(strOrEmpty(s.ReviewingVersion)),
		skillListText(color, "90", "online="), onlineCnt,
		skillListText(color, statusColor, "status="+statusLabel),
	)

	// Line 3: governance metadata (printed only when present).
	var meta []string
	if strOrEmpty(s.Scope) != "" {
		meta = append(meta, "scope="+strOrEmpty(s.Scope))
	}
	if strOrEmpty(s.BizTags) != "" {
		meta = append(meta, "bizTags="+strOrEmpty(s.BizTags))
	}
	if s.Owner != "" {
		meta = append(meta, "owner="+s.Owner)
	}
	if s.UpdateTime > 0 {
		meta = append(meta, "updated="+time.UnixMilli(s.UpdateTime).Format("2006-01-02 15:04:05"))
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
