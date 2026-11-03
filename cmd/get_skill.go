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

	"github.com/nacos-group/nacos-cli/internal/client"
	"github.com/nacos-group/nacos-cli/internal/config"
	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var getSkillCmd = newSkillGetCommand()

var errSkillSelectionCancelled = errors.New("selection cancelled")

func newSkillGetCommand() *cobra.Command {
	command := newSkillGetCommandForSession(nil, "", "", nil)
	command.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := cmd.ValidateFlagGroups(); err != nil {
			return err
		}
		rootCmd.PersistentPreRun(cmd, args)
		return nil
	}
	return command
}

func newSkillGetCommandForSession(sessionClient *client.NacosClient, sessionProfile, sessionConfig string, readSelection func(string) (string, error)) *cobra.Command {
	var output, version, label string
	var hub, noHub, nonInteractive bool
	command := &cobra.Command{
		Use:               "skill-get <skillName> [hubEntryId] [version]",
		Short:             "Download a skill by name; choose a source when names are ambiguous",
		Long:              help.SkillGet.FormatForCLI("nacos-cli"),
		Args:              cobra.MinimumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return cmd.ValidateFlagGroups() },
		RunE: func(cmd *cobra.Command, args []string) error {
			names, entryID, selectedVersion, err := parseSkillGetSelection(args, version)
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
			if nacosClient.Namespace == skill.SharedNamespaceID {
				return fmt.Errorf("keep the current business Namespace; use skill-get <skillName> <hubEntryId> to select Hub")
			}
			if output == "" || output == "~" || strings.HasPrefix(output, "~/") {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				switch output {
				case "":
					output = filepath.Join(homeDir, ".skills")
				case "~":
					output = homeDir
				default:
					output = filepath.Join(homeDir, output[2:])
				}
			}
			service := skill.NewSkillService(nacosClient)
			var failures []string
			var failedNames []string
			for index, name := range names {
				if len(names) > 1 {
					fmt.Fprintf(cmd.OutOrStdout(), "\n[%d/%d] Fetching skill: %s...\n", index+1, len(names), name)
				}
				item, err := chooseSkillDownload(cmd, service, name, entryID, source, nonInteractive, readSelection)
				if errors.Is(err, errSkillSelectionCancelled) {
					return nil
				}
				if err == nil {
					err = downloadSelectedSkill(cmd.OutOrStdout(), cmd.ErrOrStderr(), service, item, output, selectedVersion, label)
				}
				if err != nil {
					failures = append(failures, fmt.Sprintf("%s: %v", name, err))
					failedNames = append(failedNames, name)
					if len(names) > 1 {
						fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to download skill '%s': %v\n", name, err)
					}
				}
			}
			if len(names) > 1 {
				fmt.Fprintln(cmd.OutOrStdout(), "\n========== Summary ==========")
				fmt.Fprintf(cmd.OutOrStdout(), "Total: %d | Success: %d | Failed: %d\n", len(names), len(names)-len(failures), len(failures))
				if len(failedNames) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "Failed skills: %s\n", strings.Join(failedNames, ", "))
				}
			}
			if len(failures) != 0 {
				return fmt.Errorf("download failed: %s", strings.Join(failures, "; "))
			}
			return nil
		},
	}
	command.Flags().StringVarP(&output, "output", "o", "", "Output directory (default: ~/.skills)")
	command.Flags().StringVar(&version, "version", "", "Version (Hub defaults to its only shared version)")
	command.Flags().StringVar(&label, "label", "", "Version label for ordinary Namespace skills")
	command.Flags().BoolVar(&hub, "hub", false, "Find only Hub skills, overriding the profile")
	command.Flags().BoolVar(&noHub, "no-hub", false, "Download only from the current Namespace")
	command.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Report ambiguous matches without prompting")
	command.MarkFlagsMutuallyExclusive("hub", "no-hub")
	_ = command.MarkFlagDirname("output")
	return command
}

func parseSkillGetSelection(args []string, version string) ([]string, string, string, error) {
	if len(args) >= 2 && skill.IsHubEntryID(args[1]) {
		if len(args) > 3 {
			return nil, "", "", fmt.Errorf("use skill-get <skillName> <hubEntryId> [version]")
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

func chooseSkillDownload(cmd *cobra.Command, service *skill.SkillService, name, entryID, source string, nonInteractive bool, readSelection func(string) (string, error)) (skill.SkillListItem, error) {
	if source == config.SkillSourceNamespace {
		// Preserve the original Client-only path, including old servers and explicit versions.
		namespace := service.Client().Namespace
		if namespace == "" {
			namespace = "public"
		}
		return skill.SkillListItem{Name: name, NamespaceID: namespace, DiscoverySource: "NAMESPACE"}, nil
	}
	candidates, err := service.FindSkillDownloadCandidates(name, source)
	if err != nil {
		return skill.SkillListItem{}, err
	}
	if entryID != "" {
		for _, item := range candidates {
			if item.DiscoverySource == "HUB" && item.HubEntryID == entryID {
				return item, nil
			}
		}
		return skill.SkillListItem{}, fmt.Errorf("no readable Hub entry %s named %q; refresh skill-list --hub", entryID, name)
	}
	if len(candidates) == 0 {
		return skill.SkillListItem{}, fmt.Errorf("no downloadable skill named %q in the selected sources", name)
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Multiple skills named %q; choose the source:\n", name)
	for index, item := range candidates {
		if item.DiscoverySource == "HUB" {
			fmt.Fprintf(cmd.OutOrStdout(), "  [%d] Hub | author=%s | source=%s | version=%s | hubEntryId=%s\n      skill-get %s %s\n", index+1, item.Owner, item.SourceDisplayName, item.Version, item.HubEntryID, name, item.HubEntryID)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "  [%d] Namespace %s | author=%s | latest=%s\n      skill-get %s --no-hub\n", index+1, item.NamespaceID, item.Owner, dashIfEmpty(item.Labels["latest"]), name)
		}
	}
	if nonInteractive || (readSelection == nil && !stdinIsTerminal()) {
		return skill.SkillListItem{}, fmt.Errorf("ambiguous name; specify skill-get %s <hubEntryId> or --no-hub", name)
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
		return skill.SkillListItem{}, fmt.Errorf("read selection: %w", err)
	}
	answer = strings.TrimSpace(answer)
	if answer == "" || answer == "0" {
		return skill.SkillListItem{}, errSkillSelectionCancelled
	}
	index, err := strconv.Atoi(answer)
	if err != nil || index < 1 || index > len(candidates) {
		return skill.SkillListItem{}, fmt.Errorf("invalid choice %q", answer)
	}
	return candidates[index-1], nil
}

func downloadSelectedSkill(out, errOut io.Writer, service *skill.SkillService, item skill.SkillListItem, output, version, label string) error {
	result, err := service.FetchSkillCandidate(item, version, label)
	if err != nil {
		return err
	}
	if result.Deleted {
		return fmt.Errorf("selected skill is unavailable; refresh the list and choose again")
	}
	if !result.Updated {
		return fmt.Errorf("server returned no downloadable content")
	}
	if err := skill.ExtractSkillZip(result.ZipBytes, output); err != nil {
		return err
	}
	location := output
	if item.DiscoverySource != "HUB" {
		location = filepath.Join(output, item.Name)
		lock, err := skill.LoadSkillsLock(output)
		if err == nil {
			lock.RecordInstall(item.Name, version, label, result.Md5, result.ResolvedVersion)
			err = skill.SaveSkillsLock(output, lock)
		}
		if err != nil {
			fmt.Fprintf(errOut, "Warning: could not update skills-lock.json: %v\n", err)
		}
	}
	name := item.DisplayName
	if name == "" {
		name = item.Name
	}
	fmt.Fprintf(out, "Skill downloaded successfully: %s [%s]\n  Location: %s\n", name, item.DiscoverySource, location)
	if item.DiscoverySource == "HUB" {
		fmt.Fprintf(out, "  Hub entry: %s | Shared version: %s\n", item.HubEntryID, item.Version)
	}
	return nil
}

func init() { rootCmd.AddCommand(getSkillCmd) }
