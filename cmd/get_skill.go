package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var (
	getSkillOutput  string
	getSkillVersion string
	getSkillLabel   string
)

var getSkillCmd = &cobra.Command{
	Use:   "skill-get [skillName...]",
	Short: "Get one or more skills and download them locally",
	Long:  help.SkillGet.FormatForCLI("nacos-cli"),
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		skillNames := args

		// Default output directory
		if getSkillOutput == "" {
			homeDir, err := os.UserHomeDir()
			checkError(err)
			getSkillOutput = filepath.Join(homeDir, ".skills")
		} else {
			// Expand ~ to home directory
			if strings.HasPrefix(getSkillOutput, "~/") {
				homeDir, err := os.UserHomeDir()
				checkError(err)
				getSkillOutput = filepath.Join(homeDir, getSkillOutput[2:])
			} else if getSkillOutput == "~" {
				homeDir, err := os.UserHomeDir()
				checkError(err)
				getSkillOutput = homeDir
			}
		}

		// Create Nacos client
		nacosClient := mustNewNacosClient()

		// Create skill service
		skillService := skill.NewSkillService(nacosClient)

		// Load lock file for recording installs
		lock, lockErr := skill.LoadSkillsLock(getSkillOutput)
		if lockErr != nil {
			// Non-fatal: just warn and continue without lock tracking
			fmt.Fprintf(os.Stderr, "Warning: failed to load skills-lock.json: %v\n", lockErr)
		}

		// Track results
		var successCount, failCount int
		var failedSkills []string

		// Process each skill
		for i, skillName := range skillNames {
			if len(skillNames) > 1 {
				fmt.Printf("\n[%d/%d] ", i+1, len(skillNames))
			}
			fmt.Printf("Fetching skill: %s...\n", skillName)

			// Use QuerySkill for download + MD5 tracking
			result, err := skillService.QuerySkill(skillName, getSkillOutput, getSkillVersion, getSkillLabel, "")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to download skill '%s': %v\n", skillName, err)
				failCount++
				failedSkills = append(failedSkills, skillName)
			} else if result.Deleted {
				fmt.Fprintf(os.Stderr, "Error: skill '%s' not found on server\n", skillName)
				failCount++
				failedSkills = append(failedSkills, skillName)
			} else {
				skillPath := filepath.Join(getSkillOutput, skillName)
				fmt.Printf("Skill downloaded successfully!\n")
				fmt.Printf("  Location: %s\n", skillPath)
				successCount++

				// Record in lock file
				if lock != nil {
					lock.RecordInstall(skillName, getSkillVersion, getSkillLabel,
						result.Md5, result.ResolvedVersion)
				}
			}
		}

		// Save lock file if we have successful downloads
		if lock != nil && successCount > 0 {
			if err := skill.SaveSkillsLock(getSkillOutput, lock); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save skills-lock.json: %v\n", err)
			}
		}

		// Summary
		if len(skillNames) > 1 {
			fmt.Printf("\n========== Summary ==========\n")
			fmt.Printf("Total: %d | Success: %d | Failed: %d\n", len(skillNames), successCount, failCount)
			if failCount > 0 {
				fmt.Printf("Failed skills: %s\n", strings.Join(failedSkills, ", "))
			}
		}

		// Exit with error if any skill failed
		if failCount > 0 {
			os.Exit(1)
		}
	},
}

func init() {
	getSkillCmd.Flags().StringVarP(&getSkillOutput, "output", "o", "", "Output directory (default: ~/.skills)")
	getSkillCmd.Flags().StringVar(&getSkillVersion, "version", "", "Specific version to download (e.g. v1, v2)")
	getSkillCmd.Flags().StringVar(&getSkillLabel, "label", "", "Route label to resolve version (e.g. latest, stable)")
	_ = getSkillCmd.MarkFlagDirname("output")
	rootCmd.AddCommand(getSkillCmd)
}
