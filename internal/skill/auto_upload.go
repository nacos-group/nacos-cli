package skill

import (
	"fmt"
	"strings"
	"time"
)

// AutoUploadDecision describes what the daemon should do for a single skill.
type AutoUploadDecision string

const (
	// AutoUploadNoChange means hashes match; nothing to do.
	AutoUploadNoChange AutoUploadDecision = "no_change"
	// AutoUploadDebouncing means a change was just detected; wait another cycle.
	AutoUploadDebouncing AutoUploadDecision = "debouncing"
	// AutoUploadShouldUpload means the change is stable and safe to upload.
	AutoUploadShouldUpload AutoUploadDecision = "should_upload"
	// AutoUploadBlockedReviewing means a reviewing version exists on Nacos.
	AutoUploadBlockedReviewing AutoUploadDecision = "blocked_reviewing"
	// AutoUploadBlockedForeignDraft means an editing draft exists that we did not author.
	AutoUploadBlockedForeignDraft AutoUploadDecision = "blocked_foreign_draft"
	// AutoUploadDisabled means auto-upload is off for this skill.
	AutoUploadDisabled AutoUploadDecision = "disabled"
	// AutoUploadConflict means status is Conflict; user must resolve first.
	AutoUploadConflict AutoUploadDecision = "conflict"
)

// AutoUploadEvaluation is the per-skill outcome of the auto-upload check.
type AutoUploadEvaluation struct {
	SkillName       string
	Decision        AutoUploadDecision
	CurrentHash     string
	Reason          string
	RemoteEditing   string // version of an existing editing draft
	RemoteReviewing string // version of an existing reviewing version
}

// EvaluateAutoUpload inspects a single skill and decides whether the daemon
// should fire an upload during this cycle.
//
// The returned evaluation may also mutate the entry's PendingChangeHash when
// debounce is in progress; callers should persist state if changed.
func EvaluateAutoUpload(state *SyncState, entry *SyncSkillEntry, repoPath string, skillService *SkillService) (AutoUploadEvaluation, error) {
	eval := AutoUploadEvaluation{SkillName: entry.Name}

	// Global / per-skill switch
	if !state.Config.AutoUpload || entry.AutoUploadDisabled {
		eval.Decision = AutoUploadDisabled
		return eval, nil
	}
	// Conflict already; daemon stays out
	if entry.Status == SyncStatusConflict {
		eval.Decision = AutoUploadConflict
		return eval, nil
	}

	currentHash, err := ComputeDirectoryHash(skillRepoSkillPath(repoPath, entry.Name))
	if err != nil {
		return eval, err
	}
	eval.CurrentHash = currentHash

	if currentHash == "" {
		eval.Decision = AutoUploadNoChange
		return eval, nil
	}

	if entry.Status == SyncStatusLocalChanges {
		return evaluateStableAutoUploadChange(entry, skillService, eval, currentHash)
	}
	if entry.Status == SyncStatusUploadBlocked {
		if entry.PendingChangeHash == "" {
			entry.PendingChangeHash = currentHash
		}
		return evaluateAutoUploadRemoteGate(entry, skillService, eval)
	}

	// No change since last sync
	if currentHash == entry.SyncedHash || currentHash == entry.LocalHash {
		// If there was a pending change but it reverted, clear it
		entry.PendingChangeHash = ""
		eval.Decision = AutoUploadNoChange
		return eval, nil
	}

	return evaluateStableAutoUploadChange(entry, skillService, eval, currentHash)
}

func evaluateStableAutoUploadChange(entry *SyncSkillEntry, skillService *SkillService, eval AutoUploadEvaluation, currentHash string) (AutoUploadEvaluation, error) {
	// debounce: require two consecutive polls reporting the same hash
	if entry.PendingChangeHash != currentHash {
		entry.PendingChangeHash = currentHash
		eval.Decision = AutoUploadDebouncing
		return eval, nil
	}

	return evaluateAutoUploadRemoteGate(entry, skillService, eval)
}

func evaluateAutoUploadRemoteGate(entry *SyncSkillEntry, skillService *SkillService, eval AutoUploadEvaluation) (AutoUploadEvaluation, error) {
	// Hash stable for two cycles. Now consult Nacos before uploading.
	detail, err := skillService.DescribeSkill(entry.Name)
	if err != nil {
		if isSkillNotFoundError(err) {
			eval.Decision = AutoUploadShouldUpload
			return eval, nil
		}
		return eval, err
	}

	// Reviewing locks the editing slot: never overwrite.
	if detail.ReviewingVersion != "" {
		eval.Decision = AutoUploadBlockedReviewing
		eval.RemoteReviewing = detail.ReviewingVersion
		eval.Reason = fmt.Sprintf("reviewing version %s in progress", detail.ReviewingVersion)
		return eval, nil
	}

	// Editing exists; must verify it's "ours" via md5 we recorded last upload.
	if detail.EditingVersion != "" {
		// Fetch the current draft md5 from Nacos.
		fetched, err := skillService.FetchSkill(entry.Name, detail.EditingVersion, "", entry.LastUploadedMd5)
		if err != nil {
			return eval, err
		}
		// 304 means the remote draft md5 matches what we sent (i.e. matches our last upload).
		if !fetched.Updated && entry.LastUploadedMd5 != "" {
			eval.Decision = AutoUploadShouldUpload
			return eval, nil
		}
		// Either we have no last-uploaded md5, or it differs.
		eval.Decision = AutoUploadBlockedForeignDraft
		eval.RemoteEditing = detail.EditingVersion
		eval.Reason = "remote draft modified by others"
		return eval, nil
	}

	// No editing version on Nacos; safe to create a new draft.
	eval.Decision = AutoUploadShouldUpload
	return eval, nil
}

func isSkillNotFoundError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "404 Not Found") || strings.Contains(msg, "resource not found")
}

// PerformAutoUpload uploads the current repo skill content to Nacos and
// records the new lastUploadedMd5 in the state entry.
func PerformAutoUpload(state *SyncState, entry *SyncSkillEntry, repoPath string, skillService *SkillService, currentHash string) error {
	repoSkillDir := skillRepoSkillPath(repoPath, entry.Name)
	if err := skillService.UploadSkill(repoSkillDir, true); err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	return RecordUploadedSkill(state, entry, skillService, currentHash)
}

// RecordUploadedSkill records the draft produced by a successful upload.
func RecordUploadedSkill(state *SyncState, entry *SyncSkillEntry, skillService *SkillService, currentHash string) error {
	detail, err := skillService.DescribeSkill(entry.Name)
	if err != nil {
		return err
	}
	editingVersion := detail.EditingVersion
	if editingVersion == "" {
		return fmt.Errorf("uploaded draft version not found")
	}
	fetched, err := skillService.FetchSkill(entry.Name, editingVersion, "", "")
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	entry.UploadedVersion = editingVersion
	entry.UploadedMd5 = fetched.Md5
	entry.LastUploadedMd5 = fetched.Md5
	entry.LastUploadedAt = now
	entry.Status = SyncStatusUploaded
	entry.LocalHash = currentHash
	entry.PendingChangeHash = ""
	entry.BlockedDraftVersion = ""
	entry.BlockedReviewVersion = ""
	entry.UpdatedAt = now
	state.Skills[entry.Name] = *entry
	return nil
}

func skillRepoSkillPath(repoPath, skillName string) string {
	return repoPath + "/" + skillName
}
