package skill

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/client"
)

func TestEvaluateAutoUploadLocalChangesDebouncesEvenWhenLocalHashRecorded(t *testing.T) {
	repoPath := t.TempDir()
	writeAutoUploadSkill(t, repoPath, "demo", "local version")

	currentHash, err := ComputeDirectoryHash(filepath.Join(repoPath, "demo"))
	if err != nil {
		t.Fatal(err)
	}

	state := &SyncState{
		Config: SyncConfig{AutoUpload: true},
	}
	entry := &SyncSkillEntry{
		Name:       "demo",
		LocalHash:  currentHash,
		SyncedHash: "previous-synced-hash",
		Status:     SyncStatusLocalChanges,
	}

	eval, err := EvaluateAutoUpload(state, entry, repoPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if eval.Decision != AutoUploadDebouncing {
		t.Fatalf("decision = %s, want %s", eval.Decision, AutoUploadDebouncing)
	}
	if entry.PendingChangeHash != currentHash {
		t.Fatalf("pending hash = %s, want %s", entry.PendingChangeHash, currentHash)
	}
}

func TestEvaluateAutoUploadBlockedRetriesWhenDraftCleared(t *testing.T) {
	repoPath := t.TempDir()
	writeAutoUploadSkill(t, repoPath, "demo", "local version")

	currentHash, err := ComputeDirectoryHash(filepath.Join(repoPath, "demo"))
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/admin/ai/skills" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		response := V3Response{Code: 0}
		response.Data, _ = json.Marshal(SkillDetail{
			SkillListItem: SkillListItem{Name: "demo"},
		})
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	skillService := newAutoUploadSkillService(t, server.URL)
	state := &SyncState{
		Config: SyncConfig{AutoUpload: true},
	}
	entry := &SyncSkillEntry{
		Name:                "demo",
		LocalHash:           currentHash,
		Status:              SyncStatusUploadBlocked,
		PendingChangeHash:   currentHash,
		BlockedDraftVersion: "0.0.2",
	}

	eval, err := EvaluateAutoUpload(state, entry, repoPath, skillService)
	if err != nil {
		t.Fatal(err)
	}
	if eval.Decision != AutoUploadShouldUpload {
		t.Fatalf("decision = %s, want %s", eval.Decision, AutoUploadShouldUpload)
	}
}

func writeAutoUploadSkill(t *testing.T, repoPath, name, content string) {
	t.Helper()
	dir := filepath.Join(repoPath, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newAutoUploadSkillService(t *testing.T, serverURL string) *SkillService {
	t.Helper()
	nacosClient, err := client.NewNacosClient(
		strings.TrimPrefix(serverURL, "http://"),
		"test-ns",
		client.AuthTypeNone,
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"http",
	)
	if err != nil {
		t.Fatal(err)
	}
	return NewSkillService(nacosClient)
}
