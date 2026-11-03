package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/agentspec"
	"github.com/nacos-group/nacos-cli/internal/client"
	"github.com/nacos-group/nacos-cli/internal/config"
	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/util"
	"github.com/spf13/cobra"
)

var getAgentSpecCmd = newAgentSpecGetCommand()

var errAgentSpecSelectionCancelled = errors.New("selection cancelled")

func newAgentSpecGetCommand() *cobra.Command {
	command := newAgentSpecGetCommandForSession(nil, "", "", nil)
	command.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := cmd.ValidateFlagGroups(); err != nil {
			return err
		}
		rootCmd.PersistentPreRun(cmd, args)
		return nil
	}
	return command
}

func newAgentSpecGetCommandForSession(sessionClient *client.NacosClient, sessionProfile, sessionConfig string, readSelection func(string) (string, error)) *cobra.Command {
	var output, version, label string
	var hub, noHub, nonInteractive bool
	command := &cobra.Command{
		Use:               "agentspec-get <name> [hubEntryId] [version]",
		Short:             "Download an agent spec by name; choose a source when names are ambiguous",
		Long:              help.AgentSpecGet.FormatForCLI("nacos-cli"),
		Args:              cobra.MinimumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return cmd.ValidateFlagGroups() },
		RunE: func(cmd *cobra.Command, args []string) error {
			names, entryID, selectedVersion, err := parseAgentSpecGetSelection(args, version)
			if err != nil {
				return err
			}
			if entryID != "" && noHub {
				return fmt.Errorf("hubEntryId cannot be combined with --no-hub")
			}
			profile, path := sessionProfile, sessionConfig
			if sessionClient == nil {
				profile, path = profileName, configFile
			}
			source, err := resolveSkillListSourceForProfile(cmd, hub || entryID != "", noHub, profile, path)
			if err != nil {
				return err
			}
			nacosClient := sessionClient
			if nacosClient == nil {
				nacosClient = mustNewNacosClient()
			}
			if nacosClient.Namespace == agentspec.SharedNamespaceID {
				return fmt.Errorf("keep the current business Namespace; use agentspec-get <name> <hubEntryId> to select Hub")
			}
			if output == "" {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				output = filepath.Join(homeDir, ".agentspecs")
			} else {
				output, err = util.ExpandTilde(output)
				if err != nil {
					return err
				}
			}
			service := agentspec.NewAgentSpecService(nacosClient)
			var failures []string
			var failedNames []string
			for index, name := range names {
				if len(names) > 1 {
					fmt.Fprintf(cmd.OutOrStdout(), "\n[%d/%d] Fetching agent spec: %s...\n", index+1, len(names), name)
				}
				item, err := chooseAgentSpecDownload(cmd, service, name, entryID, source, nonInteractive, readSelection)
				if errors.Is(err, errAgentSpecSelectionCancelled) {
					return nil
				}
				if err == nil {
					err = downloadSelectedAgentSpec(cmd.OutOrStdout(), service, item, output, selectedVersion, label)
				}
				if err != nil {
					failures = append(failures, fmt.Sprintf("%s: %v", name, err))
					failedNames = append(failedNames, name)
					if len(names) > 1 {
						fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to download agent spec '%s': %v\n", name, err)
					}
				}
			}
			if len(names) > 1 {
				fmt.Fprintln(cmd.OutOrStdout(), "\n========== Summary ==========")
				fmt.Fprintf(cmd.OutOrStdout(), "Total: %d | Success: %d | Failed: %d\n", len(names), len(names)-len(failures), len(failures))
				if len(failedNames) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "Failed agent specs: %s\n", strings.Join(failedNames, ", "))
				}
			}
			if len(failures) != 0 {
				return fmt.Errorf("download failed: %s", strings.Join(failures, "; "))
			}
			return nil
		},
	}
	command.Flags().StringVarP(&output, "output", "o", "", "Output directory (default: ~/.agentspecs)")
	command.Flags().StringVar(&version, "version", "", "Version (Hub defaults to its only shared version)")
	command.Flags().StringVar(&label, "label", "", "Version label for ordinary Namespace agent specs")
	command.Flags().BoolVar(&hub, "hub", false, "Find only Hub agent specs, overriding the profile")
	command.Flags().BoolVar(&noHub, "no-hub", false, "Download only from the current Namespace")
	command.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Report ambiguous matches without prompting")
	command.MarkFlagsMutuallyExclusive("hub", "no-hub")
	_ = command.MarkFlagDirname("output")
	return command
}

func parseAgentSpecGetSelection(args []string, version string) ([]string, string, string, error) {
	if len(args) >= 2 && agentspec.IsHubEntryID(args[1]) {
		if len(args) > 3 {
			return nil, "", "", fmt.Errorf("use agentspec-get <name> <hubEntryId> [version]")
		}
		if len(args) == 3 {
			if version != "" && version != args[2] {
				return nil, "", "", fmt.Errorf("positional version conflicts with --version")
			}
			version = args[2]
		}
		return args[:1], args[1], version, nil
	}
	return args, "", version, nil
}

func chooseAgentSpecDownload(cmd *cobra.Command, service *agentspec.AgentSpecService, name, entryID, source string, nonInteractive bool, readSelection func(string) (string, error)) (agentspec.AgentSpecListItem, error) {
	if source == config.SkillSourceNamespace {
		// Preserve the original Client-only path, including old servers and explicit versions.
		namespace := service.Client().Namespace
		if namespace == "" {
			namespace = "public"
		}
		return agentspec.AgentSpecListItem{Name: name, NamespaceId: namespace, DiscoverySource: "NAMESPACE"}, nil
	}
	candidates, err := service.FindAgentSpecDownloadCandidates(name, source)
	if err != nil {
		return agentspec.AgentSpecListItem{}, err
	}
	if entryID != "" {
		for _, item := range candidates {
			if item.DiscoverySource == "HUB" && item.HubEntryID == entryID {
				return item, nil
			}
		}
		return agentspec.AgentSpecListItem{}, fmt.Errorf("no readable Hub entry %s named %q; refresh agentspec-list --hub", entryID, name)
	}
	if len(candidates) == 0 {
		return agentspec.AgentSpecListItem{}, fmt.Errorf("no downloadable agent spec named %q in the selected sources", name)
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Multiple agent specs named %q; choose the source:\n", name)
	for index, item := range candidates {
		if item.DiscoverySource == "HUB" {
			fmt.Fprintf(cmd.OutOrStdout(), "  [%d] Hub | author=%s | source=%s | version=%s | hubEntryId=%s\n      agentspec-get %s %s\n", index+1, item.Owner, item.SourceDisplayName, item.Version, item.HubEntryID, name, item.HubEntryID)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "  [%d] Namespace %s | author=%s | latest=%s\n      agentspec-get %s --no-hub\n", index+1, item.NamespaceId, item.Owner, dashIfEmpty(item.Labels["latest"]), name)
		}
	}
	if nonInteractive || (readSelection == nil && !stdinIsTerminal()) {
		return agentspec.AgentSpecListItem{}, fmt.Errorf("ambiguous name; specify agentspec-get %s <hubEntryId> or --no-hub", name)
	}
	if readSelection == nil {
		reader := bufio.NewReader(cmd.InOrStdin())
		readSelection = func(prompt string) (string, error) {
			fmt.Fprint(cmd.OutOrStdout(), prompt)
			return reader.ReadString('\n')
		}
	}
	answer, err := readSelection(fmt.Sprintf("Choose 1-%d (Enter/0 to cancel and use a command above): ", len(candidates)))
	if err != nil {
		return agentspec.AgentSpecListItem{}, fmt.Errorf("read selection: %w", err)
	}
	answer = strings.TrimSpace(answer)
	if answer == "" || answer == "0" {
		return agentspec.AgentSpecListItem{}, errAgentSpecSelectionCancelled
	}
	index, err := strconv.Atoi(answer)
	if err != nil || index < 1 || index > len(candidates) {
		return agentspec.AgentSpecListItem{}, fmt.Errorf("invalid choice %q", answer)
	}
	return candidates[index-1], nil
}

func downloadSelectedAgentSpec(out io.Writer, service *agentspec.AgentSpecService, item agentspec.AgentSpecListItem, output, version, label string) error {
	if err := service.GetAgentSpecCandidate(item, output, version, label); err != nil {
		return err
	}
	name := item.Name
	if item.DiscoverySource == "HUB" {
		name = item.DisplayName
	}
	fmt.Fprintf(out, "Agent spec downloaded successfully: %s [%s]\n  Location: %s\n", name, item.DiscoverySource, filepath.Join(output, name))
	if item.DiscoverySource == "HUB" {
		fmt.Fprintf(out, "  Hub entry: %s | Shared version: %s\n", item.HubEntryID, item.Version)
	}
	return nil
}

func init() { rootCmd.AddCommand(getAgentSpecCmd) }
