package cmd

import "github.com/nacos-group/nacos-cli/internal/skill"

func ensureFetchedResolvedVersion(svc *skill.SkillService, skillName, label string, fetched *skill.SkillQueryResult) {
	if svc == nil || fetched == nil || fetched.Deleted || fetched.ResolvedVersion != "" {
		return
	}
	if version := resolveSkillLabelVersion(svc, skillName, label); version != "" {
		fetched.ResolvedVersion = version
	}
}

func resolveSkillLabelVersion(svc *skill.SkillService, skillName, label string) string {
	if svc == nil {
		return ""
	}
	if label == "" {
		label = "latest"
	}
	detail, err := svc.DescribeSkill(skillName)
	if err != nil || detail == nil || detail.Labels == nil {
		return ""
	}
	return detail.Labels[label]
}

func refreshMissingNacosVersions(state *skill.SyncState, svc *skill.SkillService) bool {
	if state == nil || svc == nil || state.Mode != skill.SyncModeNacos {
		return false
	}

	changed := false
	for name, entry := range state.Skills {
		if entry.ResolvedVersion != "" {
			continue
		}
		label := entry.Label
		if label == "" {
			label = state.Label
		}
		if version := resolveSkillLabelVersion(svc, name, label); version != "" {
			entry.ResolvedVersion = version
			state.Skills[name] = entry
			changed = true
		}
	}
	return changed
}
