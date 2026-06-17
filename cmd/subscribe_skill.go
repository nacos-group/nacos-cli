package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nacos-group/nacos-cli/internal/help"
	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

var (
	subscribeSkillOutput     string
	subscribeSkillVersion    string
	subscribeSkillLabel      string
	subscribeSkillInterval   string
	subscribeSkillForeground bool
)

var subscribeSkillCmd = &cobra.Command{
	Use:        "skill-subscribe [skillName...]",
	Short:      "Subscribe to skill updates and auto-update local files",
	Long:       help.SkillSubscribe.FormatForCLI("nacos-cli"),
	Deprecated: "use 'nacos-cli skill-sync add' instead",
	Args:       cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		skillNames := args

		// Resolve output directory
		outputDir := resolveSkillOutputDir(subscribeSkillOutput)

		// Parse interval
		interval, err := time.ParseDuration(subscribeSkillInterval)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid interval %q: %v\n", subscribeSkillInterval, err)
			os.Exit(1)
		}
		if interval < 5*time.Second {
			fmt.Fprintf(os.Stderr, "Error: interval must be at least 5s\n")
			os.Exit(1)
		}

		// Create Nacos client
		nacosClient := mustNewNacosClient()
		skillService := skill.NewSkillService(nacosClient)

		// Load or create lock file
		lock, err := skill.LoadSkillsLock(outputDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load skills-lock.json: %v\n", err)
			os.Exit(1)
		}

		// Register subscriptions and do initial download for new skills
		for _, skillName := range skillNames {
			if existing, ok := lock.Skills[skillName]; ok && existing.Subscribed && existing.Md5 != "" {
				fmt.Printf("Skill %q already subscribed (version: %s, md5: %s)\n",
					skillName, existing.ResolvedVersion, shortMd5(existing.Md5))
				continue
			}

			fmt.Printf("Fetching skill: %s...\n", skillName)

			// Initial download using QuerySkill (no md5 = unconditional download)
			result, err := skillService.QuerySkill(skillName, outputDir, subscribeSkillVersion, subscribeSkillLabel, "")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to download skill %q: %v\n", skillName, err)
				os.Exit(1)
			}

			if result.Deleted {
				fmt.Fprintf(os.Stderr, "Error: skill %q not found on server\n", skillName)
				os.Exit(1)
			}

			lock.AddSubscription(skillName, subscribeSkillVersion, subscribeSkillLabel,
				result.Md5, result.ResolvedVersion)

			fmt.Printf("  Subscribed: %s (version: %s, md5: %s)\n",
				skillName, result.ResolvedVersion, shortMd5(result.Md5))
		}

		// Save lock file
		if err := skill.SaveSkillsLock(outputDir, lock); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to save skills-lock.json: %v\n", err)
			os.Exit(1)
		}

		if !subscribeSkillForeground {
			pid, logPath, err := startBackgroundSkillWatcher(outputDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to start background watcher: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("\nBackground skill watcher started.\n")
			fmt.Printf("  PID: %d\n", pid)
			fmt.Printf("  Log: %s\n", logPath)
			fmt.Printf("  Stop: nacos-cli skill-stop -o %s\n", outputDir)
			return
		}

		// Start watching in the current process.
		fmt.Printf("\nWatching for skill updates (interval: %s)...\n", interval)
		fmt.Printf("Use 'nacos-cli skill-stop -o %s' or press Ctrl+C to stop.\n\n", outputDir)

		currentPID := os.Getpid()
		if existingPID, err := skill.LoadWatcherPID(outputDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to read %s: %v\n", skill.SkillsWatcherPIDFile, err)
		} else if existingPID != 0 && existingPID != currentPID && skill.IsProcessRunning(existingPID) {
			fmt.Fprintf(os.Stderr, "Error: background watcher already running (pid: %d)\n", existingPID)
			os.Exit(1)
		}
		if err := skill.SaveWatcherPID(outputDir, currentPID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", skill.SkillsWatcherPIDFile, err)
		}
		defer func() {
			if err := skill.ClearWatcherPID(outputDir, currentPID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to clear %s: %v\n", skill.SkillsWatcherPIDFile, err)
			}
		}()

		// Setup signal handling for graceful shutdown
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Printf("\nShutting down...\n")
			cancel()
		}()

		// Create watcher with change callback
		watcher := skill.NewSkillWatcher(nacosClient, outputDir, interval, func(result skill.SkillUpdateResult) {
			ts := time.Now().Format("15:04:05")
			if result.Error != nil {
				fmt.Printf("[%s] Error (%s): %v\n", ts, result.SkillName, result.Error)
			} else if result.Updated {
				fmt.Printf("[%s] Updated: %s → %s (md5: %s)\n",
					ts, result.SkillName, result.ResolvedVersion, shortMd5(result.NewMd5))
			} else if result.Deleted {
				fmt.Printf("[%s] Deleted: %s (removed from server)\n", ts, result.SkillName)
			}
		})

		// Block until cancelled
		if err := watcher.Start(ctx); err != nil && err != context.Canceled {
			fmt.Fprintf(os.Stderr, "Error: watcher stopped unexpectedly: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Subscription stopped. Lock file saved.")
	},
}

func init() {
	subscribeSkillCmd.Flags().StringVarP(&subscribeSkillOutput, "output", "o", "", "Output directory (default: ~/.skills)")
	subscribeSkillCmd.Flags().StringVar(&subscribeSkillVersion, "version", "", "Pin to a specific version (e.g. v1, v2)")
	subscribeSkillCmd.Flags().StringVar(&subscribeSkillLabel, "label", "", "Route label to resolve version (e.g. latest, stable)")
	subscribeSkillCmd.Flags().StringVar(&subscribeSkillInterval, "interval", "30s", "Poll interval (e.g. 10s, 1m, 5m)")
	subscribeSkillCmd.Flags().BoolVar(&subscribeSkillForeground, "foreground", false, "Run watcher in foreground instead of background")
	_ = subscribeSkillCmd.MarkFlagDirname("output")
	rootCmd.AddCommand(subscribeSkillCmd)
}

func startBackgroundSkillWatcher(outputDir string) (int, string, error) {
	existingPID, err := skill.LoadWatcherPID(outputDir)
	if err != nil {
		return 0, "", err
	}
	if existingPID != 0 {
		if skill.IsProcessRunning(existingPID) {
			return 0, "", fmt.Errorf("watcher already running (pid: %d)", existingPID)
		}
		if err := skill.ClearWatcherPID(outputDir, existingPID); err != nil {
			return 0, "", err
		}
	}

	executable, err := os.Executable()
	if err != nil {
		return 0, "", err
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return 0, "", err
	}
	logPath := skill.WatcherLogPath(outputDir)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, "", err
	}
	defer logFile.Close()

	args := backgroundWatcherArgs(os.Args[1:])

	cmd := exec.Command(executable, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = os.Environ()
	cmd.SysProcAttr = backgroundSysProcAttr()

	if err := cmd.Start(); err != nil {
		return 0, "", err
	}

	pid := cmd.Process.Pid
	if err := skill.SaveWatcherPID(outputDir, pid); err != nil {
		_ = cmd.Process.Kill()
		return 0, "", err
	}
	if err := cmd.Process.Release(); err != nil {
		return 0, "", err
	}
	return pid, logPath, nil
}

func backgroundWatcherArgs(args []string) []string {
	result := make([]string, 0, len(args)+1)
	for _, arg := range args {
		if arg == "--foreground" || strings.HasPrefix(arg, "--foreground=") {
			continue
		}
		result = append(result, arg)
	}
	return append(result, "--foreground")
}

// resolveSkillOutputDir resolves the output directory path with ~ expansion.
func resolveSkillOutputDir(output string) string {
	if output == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
			os.Exit(1)
		}
		return filepath.Join(homeDir, ".skills")
	}
	if strings.HasPrefix(output, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
			os.Exit(1)
		}
		return filepath.Join(homeDir, output[2:])
	}
	if output == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
			os.Exit(1)
		}
		return homeDir
	}
	return output
}

// shortMd5 returns the first 8 characters of an MD5 hash for display.
func shortMd5(md5 string) string {
	if len(md5) > 8 {
		return md5[:8]
	}
	return md5
}
