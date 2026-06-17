package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/skill"
)

func TestOverwriteFlagValue(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		want      bool
		wantError bool
	}{
		{name: "true", value: "true", want: true},
		{name: "false", value: "false", want: false},
		{name: "uppercase rejected", value: "TRUE", wantError: true},
		{name: "numeric rejected", value: "1", wantError: true},
		{name: "empty rejected", value: "", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := false
			err := overwriteFlagValue{value: &got}.Set(tt.value)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("value = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpdateSyncStateAfterUploadRecordsUploadedVersionAndMd5(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "demo", "SKILL.md", "local version")
	localHash, err := skill.ComputeDirectoryHash(filepath.Join(repoPath, "demo"))
	if err != nil {
		t.Fatal(err)
	}

	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Label:   "latest",
		Repo:    repoPath,
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:   "demo",
				Status: skill.SyncStatusLocalChanges,
			},
		},
	}
	if err := skill.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nacos/v3/admin/ai/skills":
			response := skill.V3Response{Code: 0}
			response.Data, _ = json.Marshal(skill.SkillDetail{
				SkillListItem: skill.SkillListItem{
					Name:           "demo",
					EditingVersion: "0.0.2",
				},
			})
			_ = json.NewEncoder(w).Encode(response)
		case "/nacos/v3/client/ai/skills":
			if got := r.URL.Query().Get("version"); got != "0.0.2" {
				t.Fatalf("version = %q, want 0.0.2", got)
			}
			w.Header().Set("X-Nacos-Skill-Md5", "md5-uploaded")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("zip bytes are not inspected by FetchSkill"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	updateSyncStateAfterUpload("demo", newSkillServiceForTest(t, server.URL), filepath.Join(repoPath, "demo"))

	state, err = skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	got := state.Skills["demo"]
	if got.Status != skill.SyncStatusUploaded {
		t.Fatalf("status = %s, want uploaded", got.Status)
	}
	if got.UploadedVersion != "0.0.2" || got.UploadedMd5 != "md5-uploaded" {
		t.Fatalf("uploaded = version:%q md5:%q, want 0.0.2/md5-uploaded", got.UploadedVersion, got.UploadedMd5)
	}
	if got.LocalHash != localHash {
		t.Fatalf("local hash = %q, want %q", got.LocalHash, localHash)
	}
}

func TestDiscoverSkillDirsIncludesSymlinkDirectory(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real-skill")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "SKILL.md"), []byte("# Real Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(root, "linked-skill")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "not-a-skill.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	got := discoverSkillDirs(root, entries)

	assertStringInSlice(t, got, "linked-skill")
}

func assertStringInSlice(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("%q not found in %v", want, values)
}
