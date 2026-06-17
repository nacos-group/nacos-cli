package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/skill"
)

func TestEnsureFetchedResolvedVersionUsesDescribeLabel(t *testing.T) {
	server := newSkillDescribeServer(t, map[string]string{"latest": "0.0.7"})
	defer server.Close()

	fetched := &skill.SkillQueryResult{Md5: "remote-md5", Updated: true}
	ensureFetchedResolvedVersion(newSkillServiceForTest(t, server.URL), "demo", "latest", fetched)

	if fetched.ResolvedVersion != "0.0.7" {
		t.Fatalf("resolved version = %q, want 0.0.7", fetched.ResolvedVersion)
	}
}

func TestRefreshMissingNacosVersionsFillsOnlyMissingEntries(t *testing.T) {
	server := newSkillDescribeServer(t, map[string]string{"latest": "0.0.7"})
	defer server.Close()

	state := &skill.SyncState{
		Mode:  skill.SyncModeNacos,
		Label: "latest",
		Skills: map[string]skill.SyncSkillEntry{
			"demo":  {Name: "demo", Label: "latest", Status: skill.SyncStatusSynced},
			"fixed": {Name: "fixed", Label: "latest", ResolvedVersion: "0.0.1", Status: skill.SyncStatusSynced},
		},
	}

	changed := refreshMissingNacosVersions(state, newSkillServiceForTest(t, server.URL))
	if !changed {
		t.Fatal("expected refresh to report changes")
	}
	if got := state.Skills["demo"].ResolvedVersion; got != "0.0.7" {
		t.Fatalf("demo version = %q, want 0.0.7", got)
	}
	if got := state.Skills["fixed"].ResolvedVersion; got != "0.0.1" {
		t.Fatalf("fixed version = %q, want existing 0.0.1", got)
	}
}

func newSkillDescribeServer(t *testing.T, labels map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/admin/ai/skills" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("skillName"); got != "demo" {
			t.Fatalf("skillName = %s, want demo", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"code":0,"message":"success","data":{"name":"demo","labels":%s}}`, labelsJSON(labels))
	}))
}

func labelsJSON(labels map[string]string) string {
	out := "{"
	first := true
	for key, value := range labels {
		if !first {
			out += ","
		}
		first = false
		out += fmt.Sprintf("%q:%q", key, value)
	}
	out += "}"
	return out
}
