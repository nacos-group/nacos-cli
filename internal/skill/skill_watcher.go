package skill

import (
	"context"
	"fmt"
	"time"

	"github.com/nacos-group/nacos-cli/internal/client"
)

// SkillUpdateResult represents the result of polling a single skill.
type SkillUpdateResult struct {
	SkillName       string
	Updated         bool
	Deleted         bool
	NewMd5          string
	ResolvedVersion string
	Error           error
}

// SkillWatcher periodically polls for skill changes and auto-updates local files.
type SkillWatcher struct {
	client       *client.NacosClient
	skillService *SkillService
	outputDir    string
	interval     time.Duration
	onChange     func(result SkillUpdateResult) // callback on change
}

// NewSkillWatcher creates a new skill watcher.
func NewSkillWatcher(nacosClient *client.NacosClient, outputDir string, interval time.Duration, onChange func(SkillUpdateResult)) *SkillWatcher {
	return &SkillWatcher{
		client:       nacosClient,
		skillService: NewSkillService(nacosClient),
		outputDir:    outputDir,
		interval:     interval,
		onChange:     onChange,
	}
}

// Start begins the polling loop. Blocks until ctx is cancelled.
func (w *SkillWatcher) Start(ctx context.Context) error {
	// Do an initial poll immediately
	w.pollOnce()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			w.pollOnce()
		}
	}
}

// pollOnce performs a single poll cycle over all subscribed skills.
func (w *SkillWatcher) pollOnce() []SkillUpdateResult {
	lock, err := LoadSkillsLock(w.outputDir)
	if err != nil {
		if w.onChange != nil {
			w.onChange(SkillUpdateResult{
				Error: fmt.Errorf("failed to load skills-lock.json: %w", err),
			})
		}
		return nil
	}

	subscribed := lock.GetSubscribedSkills()
	if len(subscribed) == 0 {
		return nil
	}

	var results []SkillUpdateResult
	changed := false

	for _, entry := range subscribed {
		// Ensure token is valid before each query
		if err := w.client.EnsureTokenValid(); err != nil {
			result := SkillUpdateResult{
				SkillName: entry.Name,
				Error:     fmt.Errorf("auth failed: %w", err),
			}
			results = append(results, result)
			if w.onChange != nil {
				w.onChange(result)
			}
			continue
		}

		queryResult, err := w.skillService.QuerySkill(
			entry.Name, w.outputDir, entry.Version, entry.Label, entry.Md5,
		)

		if err != nil {
			result := SkillUpdateResult{
				SkillName: entry.Name,
				Error:     err,
			}
			results = append(results, result)
			if w.onChange != nil {
				w.onChange(result)
			}
			continue
		}

		if queryResult.Deleted {
			result := SkillUpdateResult{
				SkillName: entry.Name,
				Deleted:   true,
			}
			results = append(results, result)
			if w.onChange != nil {
				w.onChange(result)
			}
			// Remove from lock
			delete(lock.Skills, entry.Name)
			changed = true
			continue
		}

		if queryResult.Updated {
			// Update lock entry with new MD5 and version
			lock.AddSubscription(entry.Name, entry.Version, entry.Label,
				queryResult.Md5, queryResult.ResolvedVersion)
			changed = true

			result := SkillUpdateResult{
				SkillName:       entry.Name,
				Updated:         true,
				NewMd5:          queryResult.Md5,
				ResolvedVersion: queryResult.ResolvedVersion,
			}
			results = append(results, result)
			if w.onChange != nil {
				w.onChange(result)
			}
		}
	}

	// Save lock if any changes were made
	if changed {
		if err := SaveSkillsLock(w.outputDir, lock); err != nil {
			if w.onChange != nil {
				w.onChange(SkillUpdateResult{
					Error: fmt.Errorf("failed to save skills-lock.json: %w", err),
				})
			}
		}
	}

	return results
}
