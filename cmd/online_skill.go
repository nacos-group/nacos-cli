package cmd

import (
	"fmt"
	"os"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var (
	skillOnlineVersion  string
	skillOfflineVersion string
)

var onlineSkillCmd = &cobra.Command{
	Use:   "skill-online [skillName]",
	Short: "Bring a skill or skill version online",
	Long:  help.SkillOnline.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		skillName := args[0]
		nacosClient := mustNewNacosClient()
		skillService := skill.NewSkillService(nacosClient)

		printSkillOnlineStatusAction("Bringing online", skillName, skillOnlineVersion)
		if err := skillService.OnlineSkill(skillName, skillOnlineVersion); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to bring skill '%s' online: %v\n", skillName, err)
			os.Exit(1)
		}
		printSkillOnlineStatusResult("online", skillName, skillOnlineVersion)
	},
}

var offlineSkillCmd = &cobra.Command{
	Use:   "skill-offline [skillName]",
	Short: "Take a skill or skill version offline",
	Long:  help.SkillOffline.FormatForCLI("nacos-cli"),
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		skillName := args[0]
		nacosClient := mustNewNacosClient()
		skillService := skill.NewSkillService(nacosClient)

		printSkillOnlineStatusAction("Taking offline", skillName, skillOfflineVersion)
		if err := skillService.OfflineSkill(skillName, skillOfflineVersion); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to take skill '%s' offline: %v\n", skillName, err)
			os.Exit(1)
		}
		printSkillOnlineStatusResult("offline", skillName, skillOfflineVersion)
	},
}

func init() {
	onlineSkillCmd.Flags().StringVar(&skillOnlineVersion, "version", "", "Optional. Version to bring online")
	offlineSkillCmd.Flags().StringVar(&skillOfflineVersion, "version", "", "Optional. Version to take offline")
	rootCmd.AddCommand(onlineSkillCmd)
	rootCmd.AddCommand(offlineSkillCmd)
}

func printSkillOnlineStatusAction(action, skillName, version string) {
	if version == "" {
		fmt.Printf("%s skill: %s...\n", action, skillName)
		return
	}
	fmt.Printf("%s skill version: %s@%s...\n", action, skillName, version)
}

func printSkillOnlineStatusResult(status, skillName, version string) {
	if version == "" {
		fmt.Printf("Skill updated successfully: %s is now %s.\n", skillName, status)
		return
	}
	fmt.Printf("Skill version updated successfully: %s@%s is now %s.\n", skillName, version, status)
}
