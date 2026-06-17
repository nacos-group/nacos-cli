package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverAgentsIncludesAgentSkillDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	wantAgents := map[string]string{
		"codex":     filepath.Join(home, ".codex", "skills"),
		"claude":    filepath.Join(home, ".claude", "skills"),
		"qoder":     filepath.Join(home, ".qoder", "skills"),
		"qoderwork": filepath.Join(home, ".qoderwork", "skills"),
		"cursor":    filepath.Join(home, ".cursor", "skills"),
		"kiro":      filepath.Join(home, ".kiro", "skills"),
		"lingma":    filepath.Join(home, ".lingma", "skills"),
		"copaw":     filepath.Join(home, ".copaw", "skill_pool"),
		"openclaw":  filepath.Join(home, ".openclaw", "skills"),
		"agents":    filepath.Join(home, ".agents", "skills"),
		"default":   filepath.Join(home, ".skills"),
	}
	for name, path := range wantAgents {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatalf("mkdir %s for %s: %v", path, name, err)
		}
	}

	agents, err := DiscoverAgents()
	if err != nil {
		t.Fatalf("DiscoverAgents() error = %v", err)
	}

	byName := map[string]string{}
	for _, agent := range agents {
		byName[agent.Name] = agent.Path
	}

	for name, want := range wantAgents {
		if byName[name] != want {
			t.Fatalf("agent %q path = %q, want %q; all agents: %#v", name, byName[name], want, agents)
		}
	}
	if _, ok := byName["agent"]; ok {
		t.Fatalf("agent %q should not be auto-discovered; all agents: %#v", "agent", agents)
	}
}
