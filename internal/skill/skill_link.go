package skill

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ConflictDecision is the user's choice for a single agent conflict.
type ConflictDecision string

const (
	// ConflictDecisionUseSource backs up the agent's existing skill and links to the source.
	ConflictDecisionUseSource ConflictDecision = "use_source"
	// ConflictDecisionKeepLocal keeps the agent's existing skill and skips linking.
	ConflictDecisionKeepLocal ConflictDecision = "keep_local"
)

// ConflictResolver decides what to do when an agent has a different version
// of the skill from the source.
type ConflictResolver interface {
	Resolve(skillName string, agent AgentDir, sourceHash, agentHash string) (ConflictDecision, error)
}

// LinkOptions controls how a single skill is linked to all agents.
type LinkOptions struct {
	// Resolver decides conflicts on a per-agent basis.
	Resolver ConflictResolver
	// Out is the destination for human-readable progress lines.
	Out io.Writer
}

// AgentLinkResult describes the outcome for one agent.
type AgentLinkResult struct {
	Agent      AgentDir
	Status     AgentSkillStatus
	BackupPath string
	Decision   ConflictDecision
	Skipped    bool
	Error      error
}

// LinkResult aggregates the outcome of LinkSkillToAllAgents across agents.
type LinkResult struct {
	SkillName string
	Agents    []AgentLinkResult
}

type AgentDetachAction string

const (
	AgentDetachCopied       AgentDetachAction = "copied"
	AgentDetachKeptCopy     AgentDetachAction = "kept_copy"
	AgentDetachKeptExisting AgentDetachAction = "kept_existing"
)

// LinkSkillToAllAgents creates symlinks from agentDirs to the skill in repoPath.
// It examines each agent, resolves conflicts via the resolver, and reports per-agent results.
func LinkSkillToAllAgents(repoPath, skillName string, agents []AgentDir, opts LinkOptions) (*LinkResult, error) {
	if opts.Out == nil {
		opts.Out = io.Discard
	}

	repoSkillPath := filepath.Join(repoPath, skillName)
	if _, err := os.Stat(repoSkillPath); err != nil {
		return nil, fmt.Errorf("skill %q not found in repository: %w", skillName, err)
	}

	repoHash, err := ComputeDirectoryHash(repoSkillPath)
	if err != nil {
		return nil, fmt.Errorf("hash repo skill: %w", err)
	}

	result := &LinkResult{SkillName: skillName}

	for _, agent := range agents {
		agentRes := AgentLinkResult{Agent: agent}
		status, err := InspectAgentSkill(agent.Path, skillName, repoPath)
		if err != nil {
			agentRes.Error = err
			result.Agents = append(result.Agents, agentRes)
			continue
		}
		agentRes.Status = status

		switch status {
		case AgentSkillLinked:
			// Already a correct symlink, nothing to do
			agentRes.Skipped = true
			fmt.Fprintf(opts.Out, "  %s\tlinked (already)\n", agent.Name)
		case AgentSkillSame:
			// Replace real dir with symlink (content matches, safe)
			if err := os.RemoveAll(filepath.Join(agent.Path, skillName)); err != nil {
				agentRes.Error = err
				break
			}
			if err := LinkSkillToAgent(repoPath, skillName, agent.Path); err != nil {
				agentRes.Error = err
				break
			}
			fmt.Fprintf(opts.Out, "  %s\tlinked (replaced, same content)\n", agent.Name)
		case AgentSkillMissing:
			if err := LinkSkillToAgent(repoPath, skillName, agent.Path); err != nil {
				agentRes.Error = err
				break
			}
			fmt.Fprintf(opts.Out, "  %s\tlinked (new)\n", agent.Name)
		case AgentSkillConflict:
			if opts.Resolver == nil {
				agentRes.Error = fmt.Errorf("conflict in agent %s but no resolver configured", agent.Name)
				break
			}
			agentHash, _ := ComputeDirectoryHash(filepath.Join(agent.Path, skillName))
			decision, err := opts.Resolver.Resolve(skillName, agent, repoHash, agentHash)
			if err != nil {
				agentRes.Error = err
				break
			}
			agentRes.Decision = decision
			switch decision {
			case ConflictDecisionUseSource:
				backup, err := BackupAgentSkill(agent.Path, skillName)
				if err != nil {
					agentRes.Error = err
					break
				}
				agentRes.BackupPath = backup
				if err := LinkSkillToAgent(repoPath, skillName, agent.Path); err != nil {
					agentRes.Error = err
					break
				}
				if agentRes.BackupPath != "" {
					fmt.Fprintf(opts.Out, "  %s\tbacked up -> linked (%s)\n", agent.Name, agentRes.BackupPath)
				} else {
					fmt.Fprintf(opts.Out, "  %s\tlinked (overwritten)\n", agent.Name)
				}
			case ConflictDecisionKeepLocal:
				agentRes.Skipped = true
				fmt.Fprintf(opts.Out, "  %s\tskipped (kept local)\n", agent.Name)
			}
		case AgentSkillBroken:
			// Broken symlink: remove and re-create
			if err := os.RemoveAll(filepath.Join(agent.Path, skillName)); err != nil {
				agentRes.Error = err
				break
			}
			if err := LinkSkillToAgent(repoPath, skillName, agent.Path); err != nil {
				agentRes.Error = err
				break
			}
			fmt.Fprintf(opts.Out, "  %s\tlinked (repaired broken symlink)\n", agent.Name)
		}

		result.Agents = append(result.Agents, agentRes)
	}

	return result, nil
}

// DetachSkillFromAllAgents stops symlink-based management while preserving a
// usable local copy in each agent whenever possible.
func DetachSkillFromAllAgents(repoPath, skillName string, agents []AgentDir, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	var failures []string
	for _, agent := range agents {
		action, err := DetachSkillFromAgent(repoPath, skillName, agent.Path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", agent.Name, err))
			fmt.Fprintf(out, "  %s\tfailed: %v\n", agent.Name, err)
			continue
		}
		switch action {
		case AgentDetachCopied:
			fmt.Fprintf(out, "  %s\tdetached (copied)\n", agent.Name)
		case AgentDetachKeptCopy:
			fmt.Fprintf(out, "  %s\tkept local copy\n", agent.Name)
		case AgentDetachKeptExisting:
			fmt.Fprintf(out, "  %s\tkept existing local\n", agent.Name)
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("failed to detach %s from %d agent(s): %s", skillName, len(failures), strings.Join(failures, "; "))
	}
	return nil
}

// DetachSkillFromAgent converts a managed symlink into a real local copy. If
// the agent already has a real directory, it is kept untouched.
func DetachSkillFromAgent(repoPath, skillName, agentPath string) (AgentDetachAction, error) {
	repoSkillPath := filepath.Join(repoPath, skillName)
	agentSkillPath := filepath.Join(agentPath, skillName)

	status, err := InspectAgentSkill(agentPath, skillName, repoPath)
	if err != nil {
		return "", err
	}
	switch status {
	case AgentSkillSame:
		return AgentDetachKeptCopy, nil
	case AgentSkillConflict, AgentSkillFound:
		return AgentDetachKeptExisting, nil
	case AgentSkillLinked, AgentSkillMissing, AgentSkillBroken:
		if _, err := os.Stat(repoSkillPath); err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("repo source missing: %s", repoSkillPath)
			}
			return "", err
		}
		if err := os.RemoveAll(agentSkillPath); err != nil {
			return "", fmt.Errorf("remove managed entry: %w", err)
		}
		if err := copyDir(repoSkillPath, agentSkillPath); err != nil {
			return "", fmt.Errorf("copy local skill: %w", err)
		}
		return AgentDetachCopied, nil
	default:
		return "", fmt.Errorf("unsupported agent skill status: %s", status)
	}
}

// UnlinkSkillFromAllAgents removes the skill symlink from every agent.
func UnlinkSkillFromAllAgents(skillName string, agents []AgentDir, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	for _, agent := range agents {
		if err := UnlinkSkillFromAgent(skillName, agent.Path); err != nil {
			fmt.Fprintf(out, "  %s\tfailed: %v\n", agent.Name, err)
			continue
		}
		fmt.Fprintf(out, "  %s\tunlinked\n", agent.Name)
	}
	return nil
}

// AgentSearchResult describes a skill that was found in agent directories.
type AgentSearchResult struct {
	Agent      AgentDir
	Path       string
	ModifiedAt time.Time
	Hash       string
}

// FindSkillInAgents scans all agent directories for a skill not present in the source.
// Returns the list of agents that contain the skill, sorted by modification time descending.
func FindSkillInAgents(skillName string, agents []AgentDir) ([]AgentSearchResult, error) {
	var results []AgentSearchResult
	for _, agent := range agents {
		path := filepath.Join(agent.Path, skillName)
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		// Skip symlinks pointing at the (missing) repo skill
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if !info.IsDir() {
			continue
		}
		hash, _ := ComputeDirectoryHash(path)
		results = append(results, AgentSearchResult{
			Agent:      agent,
			Path:       path,
			ModifiedAt: info.ModTime(),
			Hash:       hash,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].ModifiedAt.After(results[j].ModifiedAt)
	})
	return results, nil
}

// FixedResolver always returns the same decision. Used in non-interactive mode.
type FixedResolver struct {
	Decision ConflictDecision
}

// Resolve implements ConflictResolver.
func (f FixedResolver) Resolve(_ string, _ AgentDir, _, _ string) (ConflictDecision, error) {
	return f.Decision, nil
}

// ----- High-level safe link -----

// LinkSkillSafe links a repo skill to all agents using the safe-by-default
// behavior described in the design: any agent that already has a real directory
// with different content is left untouched and reported in conflictAgents.
// Agents already linked or holding identical content (or missing) are linked.
// The returned LinkResult mirrors LinkSkillToAllAgents, but with a fixed
// keep-local resolver under the hood.
func LinkSkillSafe(repoPath, skillName string, agents []AgentDir, out io.Writer) (*LinkResult, []string, error) {
	if out == nil {
		out = io.Discard
	}
	opts := LinkOptions{
		Resolver: FixedResolver{Decision: ConflictDecisionKeepLocal},
		Out:      out,
	}
	res, err := LinkSkillToAllAgents(repoPath, skillName, agents, opts)
	if err != nil {
		return nil, nil, err
	}
	var conflictAgents []string
	for _, a := range res.Agents {
		if a.Status == AgentSkillConflict && a.Decision == ConflictDecisionKeepLocal {
			conflictAgents = append(conflictAgents, a.Agent.Name)
		}
	}
	sort.Strings(conflictAgents)
	return res, conflictAgents, nil
}

// LinkSkillForce overwrites all agents with the repo version, backing up any
// real directories. Used when the user explicitly opts into use-remote-on-conflict.
func LinkSkillForce(repoPath, skillName string, agents []AgentDir, out io.Writer) (*LinkResult, error) {
	if out == nil {
		out = io.Discard
	}
	opts := LinkOptions{
		Resolver: FixedResolver{Decision: ConflictDecisionUseSource},
		Out:      out,
	}
	return LinkSkillToAllAgents(repoPath, skillName, agents, opts)
}
