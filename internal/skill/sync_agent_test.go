package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverAgentsIncludesAgentSkillDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := os.MkdirAll(filepath.Join(home, ".agents", "skills"), 0755); err != nil {
		t.Fatalf("mkdir .agents/skills: %v", err)
	}

	agents, err := DiscoverAgents()
	if err != nil {
		t.Fatalf("DiscoverAgents() error = %v", err)
	}

	byName := map[string]string{}
	for _, agent := range agents {
		byName[agent.Name] = agent.Path
	}

	want := filepath.Join(home, ".agents", "skills")
	if byName["agents"] != want {
		t.Fatalf("agent %q path = %q, want %q; all agents: %#v", "agents", byName["agents"], want, agents)
	}
	if _, ok := byName["agent"]; ok {
		t.Fatalf("agent %q should not be auto-discovered; all agents: %#v", "agent", agents)
	}
}
