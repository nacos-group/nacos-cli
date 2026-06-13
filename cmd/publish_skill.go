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

// publishAll is the legacy flag for skill-publish --all.
var publishAll bool

// publishSkillCmd is DEPRECATED. It now performs skill-upload followed by
// skill-review (submit for review) as a backward-compatible shortcut.
//
// Users should migrate to the new three-step flow:
//
//	skill-upload  -> skill-review  -> skill-release
var publishSkillCmd = &cobra.Command{
	Use:   "skill-publish [skillPath]",
	Short: "[DEPRECATED] Upload then submit a skill draft for review (use skill-upload/skill-review/skill-release instead)",
	Long:  help.SkillPublish.FormatForCLI("nacos-cli"),
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		printPublishDeprecationWarning()

		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Error: skill path required\n")
			os.Exit(1)
		}
		skillPath := args[0]

		nacosClient := mustNewNacosClient()
		skillService := skill.NewSkillService(nacosClient)

		if publishAll {
			publishAllLegacy(skillPath, skillService)
			return
		}
		publishSingleLegacy(skillPath, skillService)
	},
}

// printPublishDeprecationWarning prints a visible deprecation notice to stderr.
func printPublishDeprecationWarning() {
	fmt.Fprintln(os.Stderr, "------------------------------------------------------------")
	fmt.Fprintln(os.Stderr, "[DEPRECATED] 'skill-publish' will be removed in a future release.")
	fmt.Fprintln(os.Stderr, "  It now runs 'skill-upload' + 'skill-review' for compatibility.")
	fmt.Fprintln(os.Stderr, "  Please migrate to the new flow:")
	fmt.Fprintln(os.Stderr, "    1) skill-upload  <skillPath>          # upload as editing draft")
	fmt.Fprintln(os.Stderr, "    2) skill-review  <skillName>          # submit for review")
	fmt.Fprintln(os.Stderr, "    3) skill-release <skillName> --version <ver>  # publish online")
	fmt.Fprintln(os.Stderr, "------------------------------------------------------------")
}

// publishSingleLegacy runs upload + review for a single skill directory/zip.
func publishSingleLegacy(skillPath string, skillService *skill.SkillService) {
	if strings.HasPrefix(skillPath, "~") {
		homeDir, err := os.UserHomeDir()
		checkError(err)
		skillPath = filepath.Join(homeDir, skillPath[1:])
	}

	absPath, err := filepath.Abs(skillPath)
	checkError(err)

	skillName := deriveSkillNameFromPath(absPath)
	fmt.Printf("[1/2] Uploading skill: %s...\n", skillName)
	if err := skillService.UploadSkill(absPath, false); err != nil {
		fmt.Fprintf(os.Stderr, "Error: upload failed for '%s': %v\n", skillName, err)
		os.Exit(1)
	}
	fmt.Printf("Upload successful.\n")

	fmt.Printf("[2/2] Submitting skill for review: %s...\n", skillName)
	if err := skillService.SubmitSkill(skillName, ""); err != nil {
		fmt.Fprintf(os.Stderr, "Error: submit-for-review failed for '%s': %v\n", skillName, err)
		os.Exit(1)
	}
	fmt.Printf("Submitted for review successfully.\n")
	fmt.Printf("  Tip: After the review passes, run 'skill-release %s --version <ver>' to publish online.\n", skillName)
}

// publishAllLegacy runs upload + review for each skill directory under folderPath.
func publishAllLegacy(folderPath string, skillService *skill.SkillService) {
	if strings.HasPrefix(folderPath, "~") {
		homeDir, err := os.UserHomeDir()
		checkError(err)
		folderPath = filepath.Join(homeDir, folderPath[1:])
	}

	entries, err := os.ReadDir(folderPath)
	checkError(err)

	var skillDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillMDPath := filepath.Join(folderPath, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillMDPath); err == nil {
			skillDirs = append(skillDirs, entry.Name())
		}
	}

	if len(skillDirs) == 0 {
		fmt.Println("No skills found (directories with SKILL.md)")
		return
	}

	fmt.Printf("Found %d skills:\n", len(skillDirs))
	for _, name := range skillDirs {
		fmt.Printf("  - %s\n", name)
	}
	fmt.Println()

	successCount := 0
	failedCount := 0

	for i, skillName := range skillDirs {
		fmt.Println(strings.Repeat("=", 80))
		fmt.Printf("[%d/%d] upload+review: %s\n", i+1, len(skillDirs), skillName)
		fmt.Println(strings.Repeat("=", 80))

		skillPath := filepath.Join(folderPath, skillName)
		if err := skillService.UploadSkill(skillPath, false); err != nil {
			fmt.Printf("Upload failed: %v\n", err)
			failedCount++
			fmt.Println()
			continue
		}
		if err := skillService.SubmitSkill(skillName, ""); err != nil {
			fmt.Printf("Submit-for-review failed: %v\n", err)
			failedCount++
			fmt.Println()
			continue
		}
		fmt.Printf("Upload + review submitted successfully!\n")
		successCount++
		fmt.Println()
	}

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("Batch Publish (Deprecated) Complete")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Success: %d\n", successCount)
	if failedCount > 0 {
		fmt.Printf("Failed: %d\n", failedCount)
	}
	fmt.Printf("Total: %d\n", len(skillDirs))
	fmt.Println()
	fmt.Println("Tip: After each review passes, run 'skill-release <skillName> --version <ver>' to publish online.")
}

// deriveSkillNameFromPath returns the skill name from a directory path or a zip file path.
func deriveSkillNameFromPath(absPath string) string {
	base := filepath.Base(absPath)
	if strings.HasSuffix(strings.ToLower(base), ".zip") {
		return strings.TrimSuffix(base, filepath.Ext(base))
	}
	return base
}

func init() {
	publishSkillCmd.Flags().BoolVar(&publishAll, "all", false, "Publish all skills in the directory (deprecated)")
	rootCmd.AddCommand(publishSkillCmd)
}
