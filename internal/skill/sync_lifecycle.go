package skill

// CheckPublishStatus queries the Nacos lifecycle status for a specific skill version.
// Returns the version status string (e.g., "editing", "reviewing", "online", "offline").
func CheckPublishStatus(skillService *SkillService, skillName, resolvedVersion string) (string, error) {
	detail, err := skillService.DescribeSkill(skillName)
	if err != nil {
		return "", err
	}

	for _, v := range detail.Versions {
		if v.Version == resolvedVersion {
			return v.Status, nil
		}
	}

	return "", nil
}

// TryAutoTransitionToSynced checks if a skill in "uploaded" state has been published online,
// and if so, transitions it to "synced".
func TryAutoTransitionToSynced(state *SyncState, skillName string, skillService *SkillService) bool {
	entry, ok := state.Skills[skillName]
	if !ok {
		return false
	}

	if entry.Status != SyncStatusUploaded {
		return false
	}

	if entry.ResolvedVersion == "" {
		return false
	}

	status, err := CheckPublishStatus(skillService, skillName, entry.ResolvedVersion)
	if err != nil {
		return false
	}

	if status == "online" {
		entry.SyncedHash = entry.LocalHash
		entry.Status = SyncStatusSynced
		state.Skills[skillName] = entry
		return true
	}

	return false
}
