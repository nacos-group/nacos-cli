package skill

import (
	"encoding/json"
	"path/filepath"
	"sort"
)

// AgentReport is one entry in a dry-run report.
type AgentReport struct {
	Name       string           `json:"name"`
	Path       string           `json:"path"`
	Status     AgentSkillStatus `json:"status"`
	LocalHash  string           `json:"localHash,omitempty"`
	SourceHash string           `json:"sourceHash,omitempty"`
}

// SkillReport is the dry-run report for a single skill.
type SkillReport struct {
	Name         string        `json:"name"`
	Source       string        `json:"source,omitempty"`
	SourceExists bool          `json:"sourceExists"`
	Agents       []AgentReport `json:"agents"`
}

// SyncReport is the top-level dry-run report.
type SyncReport struct {
	Mode       SyncMode      `json:"mode"`
	Repository string        `json:"repository"`
	Skills     []SkillReport `json:"skills"`
}

// BuildSyncReport produces a JSON-serializable report describing the current
// state of skills × agents, useful for `--dry-run` output.
func BuildSyncReport(state *SyncState, skillNames []string) (*SyncReport, error) {
	repoPath, err := GetSkillRepoPath()
	if err != nil {
		return nil, err
	}

	report := &SyncReport{
		Mode:       state.Mode,
		Repository: repoPath,
	}

	// Sort for stable output
	names := make([]string, len(skillNames))
	copy(names, skillNames)
	sort.Strings(names)

	for _, name := range names {
		sr := SkillReport{Name: name, Source: filepath.Join(repoPath, name)}
		exists, err := SkillExistsInRepo(name)
		if err != nil {
			return nil, err
		}
		sr.SourceExists = exists

		var sourceHash string
		if exists {
			sourceHash, _ = ComputeDirectoryHash(filepath.Join(repoPath, name))
		}

		for _, agent := range state.Agents {
			ar := AgentReport{Name: agent.Name, Path: agent.Path}
			if exists {
				status, err := InspectAgentSkill(agent.Path, name, repoPath)
				if err != nil {
					return nil, err
				}
				ar.Status = status
				if status == AgentSkillConflict || status == AgentSkillSame {
					ar.LocalHash, _ = ComputeDirectoryHash(filepath.Join(agent.Path, name))
					ar.SourceHash = sourceHash
				}
			} else {
				// Source missing: scan agents for "found" status
				found, err := scanSingleAgentForSkill(agent.Path, name)
				if err != nil {
					return nil, err
				}
				if found {
					ar.Status = AgentSkillFound
					ar.LocalHash, _ = ComputeDirectoryHash(filepath.Join(agent.Path, name))
				} else {
					ar.Status = AgentSkillStatus("not_found")
				}
			}
			sr.Agents = append(sr.Agents, ar)
		}

		report.Skills = append(report.Skills, sr)
	}

	return report, nil
}

func scanSingleAgentForSkill(agentPath, skillName string) (bool, error) {
	results, err := FindSkillInAgents(skillName, []AgentDir{{Path: agentPath, Name: ""}})
	if err != nil {
		return false, err
	}
	return len(results) > 0, nil
}

// MarshalJSON produces the indented JSON form of the report.
func (r *SyncReport) MarshalIndent() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
