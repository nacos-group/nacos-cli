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

var uploadSkillCmd = &cobra.Command{
	Use:   "skill-upload [skillPath]",
	Short: "Upload a skill to Nacos (as ZIP, creates an editing draft)",
	Long:  help.SkillUpload.FormatForCLI("nacos-cli"),
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Error: skill path required\n")
			os.Exit(1)
		}
		skillPath := args[0]

		nacosClient := mustNewNacosClient()
		skillService := skill.NewSkillService(nacosClient)

		if uploadAll {
			uploadAllSkills(skillPath, skillService)
			return
		}
		uploadSingleSkill(skillPath, skillService)
	},
}

func uploadSingleSkill(skillPath string, skillService *skill.SkillService) {
	if strings.HasPrefix(skillPath, "~") {
		homeDir, err := os.UserHomeDir()
		checkError(err)
		skillPath = filepath.Join(homeDir, skillPath[1:])
	}

	absPath, err := filepath.Abs(skillPath)
	checkError(err)

	skillName := filepath.Base(absPath)
	fmt.Printf("Uploading skill: %s...\n", skillName)

	err = skillService.UploadSkill(absPath)
	checkError(err)

	fmt.Printf("Skill draft uploaded successfully!\n")
	fmt.Printf("  Tip: Use 'skill-review %s' to submit the draft for review.\n", skillName)
}

func uploadAllSkills(folderPath string, skillService *skill.SkillService) {
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
		fmt.Printf("[%d/%d] Uploading skill: %s\n", i+1, len(skillDirs), skillName)
		fmt.Println(strings.Repeat("=", 80))

		skillPath := filepath.Join(folderPath, skillName)
		if err := skillService.UploadSkill(skillPath); err != nil {
			fmt.Printf("Upload failed: %v\n", err)
			failedCount++
		} else {
			fmt.Printf("Upload successful!\n")
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

func init() {
	uploadSkillCmd.Flags().BoolVar(&uploadAll, "all", false, "Upload all skills in the directory")
	rootCmd.AddCommand(uploadSkillCmd)
}
