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

// TryAutoTransitionToSynced checks if a skill in "uploaded" state has been
// published online, and if so, transitions it to "synced".
func TryAutoTransitionToSynced(state *SyncState, skillName string, skillService *SkillService) bool {
	entry, ok := state.Skills[skillName]
	if !ok {
		return false
	}

	if entry.Status != SyncStatusUploaded {
		return false
	}

	uploadedVersion := entry.UploadedVersion
	if uploadedVersion == "" {
		uploadedVersion = entry.ResolvedVersion
	}
	if uploadedVersion == "" {
		return false
	}
	uploadedMd5 := entry.UploadedMd5
	if uploadedMd5 == "" {
		uploadedMd5 = entry.LastUploadedMd5
	}

	status, err := CheckPublishStatus(skillService, skillName, uploadedVersion)
	if err != nil {
		return false
	}

	if status == "" {
		entry.Status = SyncStatusLocalChanges
		entry.UploadedVersion = ""
		entry.UploadedMd5 = ""
		state.Skills[skillName] = entry
		return true
	}

	if status != "online" {
		return false
	}

	if uploadedMd5 != "" {
		fetched, err := skillService.FetchSkill(skillName, uploadedVersion, "", uploadedMd5)
		if err != nil {
			return false
		}
		if fetched.Deleted {
			entry.Status = SyncStatusLocalChanges
			entry.UploadedVersion = ""
			entry.UploadedMd5 = ""
			state.Skills[skillName] = entry
			return true
		}
		if fetched.Updated && fetched.Md5 == "" {
			return false
		}
		if fetched.Updated && fetched.Md5 != uploadedMd5 {
			entry.RemoteMd5 = fetched.Md5
			entry.ResolvedVersion = fetched.ResolvedVersion
			entry.Status = SyncStatusConflict
			state.Skills[skillName] = entry
			return true
		}
	}

	entry.ResolvedVersion = uploadedVersion
	entry.RemoteMd5 = uploadedMd5
	entry.SyncedHash = entry.LocalHash
	entry.Status = SyncStatusSynced
	entry.UploadedVersion = ""
	entry.UploadedMd5 = ""
	state.Skills[skillName] = entry
	return true
}
