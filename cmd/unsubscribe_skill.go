package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var (
	unsubscribeSkillOutput string
	unsubscribeSkillPurge  bool
)

var unsubscribeSkillCmd = &cobra.Command{
	Use:        "skill-unsubscribe [skillName...]",
	Short:      "Unsubscribe from skill updates",
	Long:       help.SkillUnsubscribe.FormatForCLI("nacos-cli"),
	Deprecated: "use 'nacos-cli skill-sync remove' instead",
	Args:       cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		skillNames := args

		// Resolve output directory
		outputDir := resolveSkillOutputDir(unsubscribeSkillOutput)

		// Load lock file
		lock, err := skill.LoadSkillsLock(outputDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load skills-lock.json: %v\n", err)
			os.Exit(1)
		}

		for _, skillName := range skillNames {
			entry, exists := lock.Skills[skillName]
			if !exists {
				fmt.Printf("Skill %q not found in lock file, skipping.\n", skillName)
				continue
			}

			if !entry.Subscribed {
				fmt.Printf("Skill %q is not subscribed, skipping.\n", skillName)
				continue
			}

			lock.RemoveSubscription(skillName)
			fmt.Printf("Unsubscribed: %s\n", skillName)

			// Optionally remove local files
			if unsubscribeSkillPurge {
				skillDir := filepath.Join(outputDir, skillName)
				if err := os.RemoveAll(skillDir); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to remove %s: %v\n", skillDir, err)
				} else {
					fmt.Printf("  Purged: %s\n", skillDir)
				}
				// Also remove from lock entirely
				delete(lock.Skills, skillName)
			}
		}

		// Save lock file
		if err := skill.SaveSkillsLock(outputDir, lock); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to save skills-lock.json: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	unsubscribeSkillCmd.Flags().StringVarP(&unsubscribeSkillOutput, "output", "o", "", "Output directory (default: ~/.skills)")
	unsubscribeSkillCmd.Flags().BoolVar(&unsubscribeSkillPurge, "purge", false, "Remove local skill files after unsubscribing")
	_ = unsubscribeSkillCmd.MarkFlagDirname("output")
	rootCmd.AddCommand(unsubscribeSkillCmd)
}
