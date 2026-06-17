package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var (
	statusSkillOutput string
)

var statusSkillCmd = &cobra.Command{
	Use:        "skill-status",
	Short:      "Show status of installed and subscribed skills",
	Long:       help.SkillStatus.FormatForCLI("nacos-cli"),
	Deprecated: "use 'nacos-cli skill-sync status' instead",
	Args:       cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Resolve output directory
		outputDir := resolveSkillOutputDir(statusSkillOutput)

		// Load lock file
		lock, err := skill.LoadSkillsLock(outputDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load skills-lock.json: %v\n", err)
			os.Exit(1)
		}

		if len(lock.Skills) == 0 {
			fmt.Println("No skills tracked. Use 'skill-get' or 'skill-subscribe' to install skills.")
			printWatcherStatus(outputDir)
			return
		}

		// Sort skill names for consistent output
		names := make([]string, 0, len(lock.Skills))
		for name := range lock.Skills {
			names = append(names, name)
		}
		sort.Strings(names)

		// Print table
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "SKILL\tVERSION\tMD5\tSUBSCRIBED\tLABEL\tUPDATED\n")
		fmt.Fprintf(w, "-----\t-------\t---\t----------\t-----\t-------\n")

		for _, name := range names {
			entry := lock.Skills[name]

			version := entry.ResolvedVersion
			if version == "" {
				version = entry.Version
			}
			if version == "" {
				version = "latest"
			}

			md5Display := shortMd5(entry.Md5)
			if md5Display == "" {
				md5Display = "-"
			}

			subscribed := "no"
			if entry.Subscribed {
				subscribed = "yes"
			}

			label := entry.Label
			if label == "" {
				label = "-"
			}

			updatedAt := entry.UpdatedAt
			if updatedAt != "" {
				// Shorten ISO8601 for display
				if idx := strings.IndexByte(updatedAt, 'T'); idx > 0 {
					updatedAt = updatedAt[:idx] + " " + updatedAt[idx+1:]
				}
				if idx := strings.IndexByte(updatedAt, 'Z'); idx > 0 {
					updatedAt = updatedAt[:idx]
				}
			} else {
				updatedAt = "-"
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				name, version, md5Display, subscribed, label, updatedAt)
		}

		w.Flush()

		// Summary
		subscribedCount := 0
		for _, entry := range lock.Skills {
			if entry.Subscribed {
				subscribedCount++
			}
		}
		fmt.Printf("\nTotal: %d skills (%d subscribed)\n", len(lock.Skills), subscribedCount)
		fmt.Printf("Lock file: %s\n", filepath.Join(outputDir, skill.SkillsLockFile))

		printWatcherStatus(outputDir)
	},
}

func init() {
	statusSkillCmd.Flags().StringVarP(&statusSkillOutput, "output", "o", "", "Output directory (default: ~/.skills)")
	_ = statusSkillCmd.MarkFlagDirname("output")
	rootCmd.AddCommand(statusSkillCmd)
}

func printWatcherStatus(outputDir string) {
	pid, err := skill.LoadWatcherPID(outputDir)
	if err != nil {
		fmt.Printf("Watcher: unknown (%v)\n", err)
	} else if pid != 0 && skill.IsProcessRunning(pid) {
		fmt.Printf("Watcher: running (pid: %d, log: %s)\n", pid, skill.WatcherLogPath(outputDir))
	} else if pid != 0 {
		fmt.Printf("Watcher: stopped (stale pid: %d)\n", pid)
	} else {
		fmt.Printf("Watcher: stopped\n")
	}
}
