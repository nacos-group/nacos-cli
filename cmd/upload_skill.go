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

var uploadAll bool
var uploadOverwrite bool

var uploadSkillCmd = &cobra.Command{
	Use:               "skill-upload [skillPath]",
	Short:             "Upload a skill to Nacos (as ZIP, creates an editing draft)",
	Long:              help.SkillUpload.FormatForCLI("nacos-cli"),
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completePathArg(0),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Error: skill path required\n")
			os.Exit(1)
		}
		skillPath := args[0]

		nacosClient := mustNewNacosClient()
		skillService := skill.NewSkillService(nacosClient)

		if uploadAll {
			uploadAllSkills(skillPath, skillService, uploadOverwrite)
			return
		}
		uploadSingleSkill(skillPath, skillService, uploadOverwrite)
	},
}

type overwriteFlagValue struct {
	value *bool
}

func (flag overwriteFlagValue) Set(value string) error {
	switch value {
	case "false":
		*flag.value = false
		return nil
	case "true":
		*flag.value = true
		return nil
	default:
		return fmt.Errorf("--overwrite must be true or false")
	}
}

func (flag overwriteFlagValue) String() string {
	if flag.value == nil {
		return "false"
	}
	return fmt.Sprintf("%t", *flag.value)
}

func (flag overwriteFlagValue) Type() string {
	return "bool"
}

func uploadSingleSkill(skillPath string, skillService *skill.SkillService, overwrite bool) {
	if strings.HasPrefix(skillPath, "~") {
		homeDir, err := os.UserHomeDir()
		checkError(err)
		skillPath = filepath.Join(homeDir, skillPath[1:])
	}

	absPath, err := filepath.Abs(skillPath)
	checkError(err)

	skillName := filepath.Base(absPath)
	fmt.Printf("Uploading skill: %s...\n", skillName)

	err = skillService.UploadSkill(absPath, overwrite)
	checkError(err)

	fmt.Printf("Skill draft uploaded successfully!\n")
	fmt.Printf("  Tip: Use 'skill-review %s' to submit the draft for review.\n", skillName)

	// Update sync state if skill is tracked
	updateSyncStateAfterUpload(skillName, skillService, absPath)
}

func uploadAllSkills(folderPath string, skillService *skill.SkillService, overwrite bool) {
	if strings.HasPrefix(folderPath, "~") {
		homeDir, err := os.UserHomeDir()
		checkError(err)
		folderPath = filepath.Join(homeDir, folderPath[1:])
	}

	entries, err := os.ReadDir(folderPath)
	checkError(err)

	skillDirs := discoverSkillDirs(folderPath, entries)

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
		fmt.Printf("[%d/%d] Uploading skill: %s\n", i+1, len(skillDirs), skillName)
		fmt.Println(strings.Repeat("=", 80))

		skillPath := filepath.Join(folderPath, skillName)
		if err := skillService.UploadSkill(skillPath, overwrite); err != nil {
			fmt.Printf("Upload failed: %v\n", err)
			failedCount++
		} else {
			fmt.Printf("Upload successful!\n")
			updateSyncStateAfterUpload(skillName, skillService, skillPath)
			successCount++
		}
		fmt.Println()
	}

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("Batch Upload Complete")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Success: %d\n", successCount)
	if failedCount > 0 {
		fmt.Printf("Failed: %d\n", failedCount)
	}
	fmt.Printf("Total: %d\n", len(skillDirs))
	fmt.Println()
	fmt.Println("Tip: Use 'skill-review <skillName>' to submit a draft for review.")
}

func discoverSkillDirs(folderPath string, entries []os.DirEntry) []string {
	var skillDirs []string
	for _, entry := range entries {
		skillPath := filepath.Join(folderPath, entry.Name())
		info, err := os.Stat(skillPath)
		if err != nil || !info.IsDir() {
			continue
		}
		skillMDPath := filepath.Join(skillPath, "SKILL.md")
		if _, err := os.Stat(skillMDPath); err == nil {
			skillDirs = append(skillDirs, entry.Name())
		}
	}
	return skillDirs
}

func init() {
	uploadSkillCmd.Flags().BoolVar(&uploadAll, "all", false, "Upload all skills in the directory")
	uploadSkillCmd.Flags().Var(overwriteFlagValue{value: &uploadOverwrite}, "overwrite", "Whether to overwrite existing draft: true | false")
	rootCmd.AddCommand(uploadSkillCmd)
}

// updateSyncStateAfterUpload refreshes the sync state after a successful upload.
func updateSyncStateAfterUpload(skillName string, skillService *skill.SkillService, uploadedPath string) {
	state, err := skill.LoadSyncState()
	if err != nil {
		return // Non-fatal: sync state might not exist yet
	}

	entry, ok := state.Skills[skillName]
	if !ok {
		return // Skill not tracked in sync state
	}

	localHash := entry.LocalHash
	if info, err := os.Stat(uploadedPath); err == nil && info.IsDir() {
		if hash, err := skill.ComputeDirectoryHash(uploadedPath); err == nil && hash != "" {
			localHash = hash
		}
	} else if state.Repo != "" {
		if hash, err := skill.ComputeDirectoryHash(filepath.Join(state.Repo, skillName)); err == nil && hash != "" {
			localHash = hash
		}
	}

	if err := skill.RecordUploadedSkill(state, &entry, skillService, localHash); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to update sync upload state: %v\n", err)
		return
	}

	if err := skill.SaveSyncState(state); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to update sync state: %v\n", err)
	} else {
		fmt.Printf("  Sync state updated: %s → Uploaded (%s)\n", skillName, entry.UploadedVersion)
	}
}
