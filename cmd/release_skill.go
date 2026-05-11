package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var (
	skillReleaseVersion      string
	skillReleaseUpdateLatest bool
)

var releaseSkillCmd = &cobra.Command{
	Use:   "skill-release [skillName]",
	Short: "Release an approved skill version (reviewing -> online)",
	Long:  help.SkillRelease.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if skillReleaseVersion == "" {
			fmt.Fprintf(os.Stderr, "Error: --version is required for skill-release\n")
			os.Exit(1)
		}

		nacosClient := mustNewNacosClient()
		skillService := skill.NewSkillService(nacosClient)

		skillName := args[0]
		fmt.Printf("Releasing skill: %s@%s (updateLatest=%v)...\n", skillName, skillReleaseVersion, skillReleaseUpdateLatest)
		if err := skillService.PublishSkill(skillName, skillReleaseVersion, skillReleaseUpdateLatest); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to release skill '%s@%s': %v\n", skillName, skillReleaseVersion, err)
			maybePrintReleaseRetryHint(err, "skill", skillName)
			os.Exit(1)
		}
		fmt.Printf("Skill released successfully!\n")
		fmt.Printf("  %s@%s is now online.\n", skillName, skillReleaseVersion)
	},
}

func init() {
	releaseSkillCmd.Flags().StringVar(&skillReleaseVersion, "version", "", "Required. Approved (reviewing) version to release")
	releaseSkillCmd.Flags().BoolVar(&skillReleaseUpdateLatest, "update-latest", true, "Whether to update the 'latest' label to the released version")
	rootCmd.AddCommand(releaseSkillCmd)
}

// maybePrintReleaseRetryHint emits a targeted hint when the release call fails
// with HTTP 400 "parameter validate error", which in practice usually means the
// server-side async review pipeline has not yet marked the version as
// 'reviewed'. Used by both skill-release and agentspec-release.
func maybePrintReleaseRetryHint(err error, kind, name string) {
	if err == nil {
		return
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "400") && !strings.Contains(msg, "parameter validate") {
		return
	}
	fmt.Fprintf(os.Stderr, "Hint: if you just ran '%s-review', the server-side review pipeline may still be running.\n", kind)
	fmt.Fprintf(os.Stderr, "      Wait 2-3 seconds, check status with '%s-describe %s', and retry when STATUS=reviewed.\n", kind, name)
}
