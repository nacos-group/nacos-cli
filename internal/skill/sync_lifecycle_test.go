package skill

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecordUploadedSkillStoresVersionAndMd5(t *testing.T) {
	server := newLifecycleSkillServer(t, lifecycleServerState{
		editingVersion: "0.0.2",
		versionStatus:  "editing",
		versionMd5:     "md5-uploaded",
	})
	defer server.Close()

	state := &SyncState{Skills: map[string]SyncSkillEntry{}}
	entry := &SyncSkillEntry{Name: "demo"}

	err := RecordUploadedSkill(state, entry, newAutoUploadSkillService(t, server.URL), "local-hash")
	if err != nil {
		t.Fatal(err)
	}

	got := state.Skills["demo"]
	if got.Status != SyncStatusUploaded {
		t.Fatalf("status = %s, want uploaded", got.Status)
	}
	if got.UploadedVersion != "0.0.2" {
		t.Fatalf("uploaded version = %q, want 0.0.2", got.UploadedVersion)
	}
	if got.UploadedMd5 != "md5-uploaded" || got.LastUploadedMd5 != "md5-uploaded" {
		t.Fatalf("uploaded md5 = %q last = %q, want md5-uploaded", got.UploadedMd5, got.LastUploadedMd5)
	}
	if got.LocalHash != "local-hash" {
		t.Fatalf("local hash = %q, want local-hash", got.LocalHash)
	}
}

func TestTryAutoTransitionToSyncedUsesUploadedVersionAndMd5(t *testing.T) {
	server := newLifecycleSkillServer(t, lifecycleServerState{
		versionStatus: "online",
		versionMd5:    "md5-uploaded",
	})
	defer server.Close()

	state := &SyncState{
		Skills: map[string]SyncSkillEntry{
			"demo": {
				Name:            "demo",
				Status:          SyncStatusUploaded,
				LocalHash:       "local-hash",
				ResolvedVersion: "old-latest",
				UploadedVersion: "0.0.2",
				UploadedMd5:     "md5-uploaded",
			},
		},
	}

	if !TryAutoTransitionToSynced(state, "demo", newAutoUploadSkillService(t, server.URL)) {
		t.Fatal("expected transition")
	}
	got := state.Skills["demo"]
	if got.Status != SyncStatusSynced {
		t.Fatalf("status = %s, want synced", got.Status)
	}
	if got.ResolvedVersion != "0.0.2" || got.RemoteMd5 != "md5-uploaded" {
		t.Fatalf("remote = version:%q md5:%q, want 0.0.2/md5-uploaded", got.ResolvedVersion, got.RemoteMd5)
	}
	if got.SyncedHash != "local-hash" {
		t.Fatalf("synced hash = %q, want local-hash", got.SyncedHash)
	}
	if got.UploadedVersion != "" || got.UploadedMd5 != "" {
		t.Fatalf("uploaded fields should be cleared, got version:%q md5:%q", got.UploadedVersion, got.UploadedMd5)
	}
}

func TestTryAutoTransitionToSyncedUploadedVersionDeleted(t *testing.T) {
	server := newLifecycleSkillServer(t, lifecycleServerState{})
	defer server.Close()

	state := &SyncState{
		Skills: map[string]SyncSkillEntry{
			"demo": {
				Name:            "demo",
				Status:          SyncStatusUploaded,
				LocalHash:       "local-hash",
				UploadedVersion: "0.0.2",
				UploadedMd5:     "md5-uploaded",
			},
		},
	}

	if !TryAutoTransitionToSynced(state, "demo", newAutoUploadSkillService(t, server.URL)) {
		t.Fatal("expected transition")
	}
	got := state.Skills["demo"]
	if got.Status != SyncStatusLocalChanges {
		t.Fatalf("status = %s, want local changes", got.Status)
	}
	if got.UploadedVersion != "" || got.UploadedMd5 != "" {
		t.Fatalf("uploaded fields should be cleared, got version:%q md5:%q", got.UploadedVersion, got.UploadedMd5)
	}
}

func TestTryAutoTransitionToSyncedUploadedMd5Mismatch(t *testing.T) {
	server := newLifecycleSkillServer(t, lifecycleServerState{
		versionStatus: "online",
		versionMd5:    "md5-other",
	})
	defer server.Close()

	state := &SyncState{
		Skills: map[string]SyncSkillEntry{
			"demo": {
				Name:            "demo",
				Status:          SyncStatusUploaded,
				LocalHash:       "local-hash",
				UploadedVersion: "0.0.2",
				UploadedMd5:     "md5-uploaded",
			},
		},
	}

	if !TryAutoTransitionToSynced(state, "demo", newAutoUploadSkillService(t, server.URL)) {
		t.Fatal("expected transition")
	}
	got := state.Skills["demo"]
	if got.Status != SyncStatusConflict {
		t.Fatalf("status = %s, want conflict", got.Status)
	}
	if got.RemoteMd5 != "md5-other" {
		t.Fatalf("remote md5 = %q, want md5-other", got.RemoteMd5)
	}
}

type lifecycleServerState struct {
	editingVersion string
	versionStatus  string
	versionMd5     string
}

func newLifecycleSkillServer(t *testing.T, state lifecycleServerState) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nacos/v3/admin/ai/skills":
			response := V3Response{Code: 0}
			detail := SkillDetail{
				SkillListItem: SkillListItem{
					Name:           "demo",
					EditingVersion: state.editingVersion,
				},
			}
			if state.versionStatus != "" {
				detail.Versions = []SkillVersionSummary{{
					Version: "0.0.2",
					Status:  state.versionStatus,
				}}
			}
			response.Data, _ = json.Marshal(detail)
			_ = json.NewEncoder(w).Encode(response)
		case "/nacos/v3/client/ai/skills":
			if got := r.URL.Query().Get("version"); got != "0.0.2" {
				t.Fatalf("version = %q, want 0.0.2", got)
			}
			if r.URL.Query().Get("md5") != "" && r.URL.Query().Get("md5") == state.versionMd5 {
				w.Header().Set("X-Nacos-Skill-Md5", state.versionMd5)
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("X-Nacos-Skill-Md5", state.versionMd5)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("zip bytes are not inspected by FetchSkill"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
}
