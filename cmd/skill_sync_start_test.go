package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/client"
	"github.com/nacos-group/nacos-cli/internal/skill"
)

func TestSyncPollOnceAppliesRemoteUpdateWhenLocalClean(t *testing.T) {
	withTempHome(t)

	primary := filepath.Join(t.TempDir(), "primary")
	secondary := filepath.Join(t.TempDir(), "secondary")
	writeSkillFile(t, primary, "demo", "SKILL.md", "old")
	writeSkillFile(t, primary, "demo", "stale.txt", "stale")
	writeSkillFile(t, secondary, "demo", "SKILL.md", "old")
	writeSkillFile(t, secondary, "demo", "stale.txt", "stale")

	syncedHash, err := skill.ComputeDirectoryHash(filepath.Join(primary, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	saveSyncStateForTest(t, primary, secondary, syncedHash)

	server := newSkillQueryServer(t, "m1", "m2", "v2", map[string]string{
		"demo/SKILL.md": "new",
	})
	defer server.Close()

	skillService := newSkillServiceForTest(t, server.URL)
	syncPollOnce(skillService)

	assertFileContent(t, filepath.Join(primary, "demo", "SKILL.md"), "new")
	assertFileMissing(t, filepath.Join(primary, "demo", "stale.txt"))
	assertFileContent(t, filepath.Join(secondary, "demo", "SKILL.md"), "new")
	assertFileMissing(t, filepath.Join(secondary, "demo", "stale.txt"))
	assertSkillSymlink(t, primary, "demo")
	assertSkillSymlink(t, secondary, "demo")

	state, err := skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	entry := state.Skills["demo"]
	if entry.Status != skill.SyncStatusSynced {
		t.Fatalf("status = %s, want synced", entry.Status)
	}
	if entry.RemoteMd5 != "m2" || entry.ResolvedVersion != "v2" {
		t.Fatalf("remote state = md5:%s version:%s, want m2/v2", entry.RemoteMd5, entry.ResolvedVersion)
	}
	if entry.SyncedHash == syncedHash {
		t.Fatal("synced hash did not update")
	}
}

func TestSyncPollOnceKeepsLocalChangesWhenRemoteUpdates(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "demo", "SKILL.md", "local chosen")

	primary := filepath.Join(t.TempDir(), "primary")
	secondary := filepath.Join(t.TempDir(), "secondary")
	if err := skill.LinkSkillToAgent(repoPath, "demo", primary); err != nil {
		t.Fatal(err)
	}
	if err := skill.LinkSkillToAgent(repoPath, "demo", secondary); err != nil {
		t.Fatal(err)
	}

	localHash, err := skill.ComputeDirectoryHash(filepath.Join(repoPath, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Label:   "latest",
		Config:  skill.SyncConfig{AutoUpload: false},
		Repo:    repoPath,
		Agents: []skill.AgentDir{
			{Name: "primary", Path: primary},
			{Name: "secondary", Path: secondary},
		},
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:      "demo",
				Label:     "latest",
				RemoteMd5: "m1",
				LocalHash: localHash,
				Status:    skill.SyncStatusLocalChanges,
			},
		},
	}
	if err := skill.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}

	server := newSkillQueryServer(t, "m1", "m2", "v2", map[string]string{
		"demo/SKILL.md": "remote update",
	})
	defer server.Close()

	skillService := newSkillServiceForTest(t, server.URL)
	syncPollOnce(skillService)

	assertFileContent(t, filepath.Join(repoPath, "demo", "SKILL.md"), "local chosen")
	assertFileContent(t, filepath.Join(primary, "demo", "SKILL.md"), "local chosen")
	assertFileContent(t, filepath.Join(secondary, "demo", "SKILL.md"), "local chosen")
	assertSkillSymlink(t, primary, "demo")
	assertSkillSymlink(t, secondary, "demo")

	state, err = skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	entry := state.Skills["demo"]
	if entry.Status != skill.SyncStatusLocalChanges {
		t.Fatalf("status = %s, want local changes", entry.Status)
	}
	if entry.RemoteMd5 != "m2" || entry.ResolvedVersion != "v2" {
		t.Fatalf("remote state = md5:%s version:%s, want m2/v2", entry.RemoteMd5, entry.ResolvedVersion)
	}
}

func TestSyncPollOnceKeepsLocalOnlySkillWhenRemoteMissing(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "demo", "SKILL.md", "local only")

	primary := filepath.Join(t.TempDir(), "primary")
	secondary := filepath.Join(t.TempDir(), "secondary")
	if err := skill.LinkSkillToAgent(repoPath, "demo", primary); err != nil {
		t.Fatal(err)
	}
	if err := skill.LinkSkillToAgent(repoPath, "demo", secondary); err != nil {
		t.Fatal(err)
	}

	localHash, err := skill.ComputeDirectoryHash(filepath.Join(repoPath, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Label:   "latest",
		Config:  skill.SyncConfig{AutoUpload: true},
		Repo:    repoPath,
		Agents: []skill.AgentDir{
			{Name: "primary", Path: primary},
			{Name: "secondary", Path: secondary},
		},
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:      "demo",
				Label:     "latest",
				LocalHash: localHash,
				Status:    skill.SyncStatusLocalChanges,
			},
		},
	}
	if err := skill.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/client/ai/skills" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		requests++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	skillService := newSkillServiceForTest(t, server.URL)
	syncPollOnce(skillService)

	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	assertFileContent(t, filepath.Join(repoPath, "demo", "SKILL.md"), "local only")
	assertFileContent(t, filepath.Join(primary, "demo", "SKILL.md"), "local only")
	assertFileContent(t, filepath.Join(secondary, "demo", "SKILL.md"), "local only")
	assertSkillSymlink(t, primary, "demo")
	assertSkillSymlink(t, secondary, "demo")

	state, err = skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := state.Skills["demo"]
	if !ok {
		t.Fatal("local-only skill was removed from sync state")
	}
	if entry.Status != skill.SyncStatusLocalChanges {
		t.Fatalf("status = %s, want local changes", entry.Status)
	}
	if entry.PendingChangeHash != localHash {
		t.Fatalf("pending change hash = %s, want %s", entry.PendingChangeHash, localHash)
	}
}

func TestSyncPollOnceAutoUploadsStableLocalOnlySkillWhenRemoteMissing(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "demo", "SKILL.md", "local only")

	primary := filepath.Join(t.TempDir(), "primary")
	if err := skill.LinkSkillToAgent(repoPath, "demo", primary); err != nil {
		t.Fatal(err)
	}

	localHash, err := skill.ComputeDirectoryHash(filepath.Join(repoPath, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Label:   "latest",
		Config:  skill.SyncConfig{AutoUpload: true},
		Repo:    repoPath,
		Agents: []skill.AgentDir{
			{Name: "primary", Path: primary},
		},
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:              "demo",
				Label:             "latest",
				LocalHash:         localHash,
				Status:            skill.SyncStatusLocalChanges,
				PendingChangeHash: localHash,
			},
		},
	}
	if err := skill.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}

	uploaded := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nacos/v3/client/ai/skills":
			if r.URL.Query().Get("version") == "0.0.2" {
				w.Header().Set("X-Nacos-Skill-Md5", "md5-uploaded")
				w.Header().Set("X-Nacos-Skill-Resolved-Version", "0.0.2")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(makeZip(t, map[string]string{"SKILL.md": "local only"}))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		case "/nacos/v3/admin/ai/skills":
			if !uploaded {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			resp := skill.V3Response{
				Code: 0,
				Data: json.RawMessage(`{"name":"demo","editingVersion":"0.0.2","versions":[{"version":"0.0.2","status":"editing"}]}`),
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "/nacos/v3/admin/ai/skills/upload":
			uploaded = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	skillService := newSkillServiceForTest(t, server.URL)
	syncPollOnce(skillService)

	if !uploaded {
		t.Fatal("expected auto-upload")
	}
	state, err = skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	entry := state.Skills["demo"]
	if entry.Status != skill.SyncStatusUploaded {
		t.Fatalf("status = %s, want uploaded", entry.Status)
	}
	if entry.UploadedVersion != "0.0.2" || entry.UploadedMd5 != "md5-uploaded" {
		t.Fatalf("uploaded = version:%q md5:%q, want 0.0.2/md5-uploaded", entry.UploadedVersion, entry.UploadedMd5)
	}
}

func TestSyncPollOnceTransitionsUploadedSkillWhenLatestMissing(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "demo", "SKILL.md", "uploaded local")

	primary := filepath.Join(t.TempDir(), "primary")
	if err := skill.LinkSkillToAgent(repoPath, "demo", primary); err != nil {
		t.Fatal(err)
	}

	localHash, err := skill.ComputeDirectoryHash(filepath.Join(repoPath, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Label:   "latest",
		Config:  skill.SyncConfig{AutoUpload: true},
		Repo:    repoPath,
		Agents: []skill.AgentDir{
			{Name: "primary", Path: primary},
		},
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:            "demo",
				Label:           "latest",
				RemoteMd5:       "old-latest-md5",
				ResolvedVersion: "0.0.1",
				LocalHash:       localHash,
				Status:          skill.SyncStatusUploaded,
				UploadedVersion: "0.0.2",
				UploadedMd5:     "md5-uploaded",
				LastUploadedMd5: "md5-uploaded",
			},
		},
	}
	if err := skill.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nacos/v3/client/ai/skills":
			if r.URL.Query().Get("version") == "0.0.2" {
				if r.URL.Query().Get("md5") != "md5-uploaded" {
					t.Fatalf("uploaded version should be verified by md5, got %q", r.URL.Query().Get("md5"))
				}
				w.Header().Set("X-Nacos-Skill-Md5", "md5-uploaded")
				w.Header().Set("X-Nacos-Skill-Resolved-Version", "0.0.2")
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		case "/nacos/v3/admin/ai/skills":
			resp := skill.V3Response{
				Code: 0,
				Data: json.RawMessage(`{"name":"demo","versions":[{"version":"0.0.2","status":"online"}]}`),
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	skillService := newSkillServiceForTest(t, server.URL)
	syncPollOnce(skillService)

	state, err = skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	entry := state.Skills["demo"]
	if entry.Status != skill.SyncStatusSynced {
		t.Fatalf("status = %s, want synced", entry.Status)
	}
	if entry.ResolvedVersion != "0.0.2" || entry.RemoteMd5 != "md5-uploaded" {
		t.Fatalf("remote state = version:%q md5:%q, want 0.0.2/md5-uploaded", entry.ResolvedVersion, entry.RemoteMd5)
	}
	if entry.UploadedVersion != "" || entry.UploadedMd5 != "" {
		t.Fatalf("uploaded markers were not cleared: version:%q md5:%q", entry.UploadedVersion, entry.UploadedMd5)
	}
	assertSkillSymlink(t, primary, "demo")
}

func TestSyncPollOnceDetachesSyncedSkillWhenRemoteDeleted(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "demo", "SKILL.md", "kept local")

	primary := filepath.Join(t.TempDir(), "primary")
	secondary := filepath.Join(t.TempDir(), "secondary")
	localHash, err := skill.ComputeDirectoryHash(filepath.Join(repoPath, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Label:   "latest",
		Config:  skill.SyncConfig{AutoUpload: true},
		Repo:    repoPath,
		Agents: []skill.AgentDir{
			{Name: "primary", Path: primary},
			{Name: "secondary", Path: secondary},
		},
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:            "demo",
				Label:           "latest",
				RemoteMd5:       "m1",
				ResolvedVersion: "v1",
				LocalHash:       localHash,
				SyncedHash:      localHash,
				Status:          skill.SyncStatusSynced,
			},
		},
	}
	if err := skill.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/client/ai/skills" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	skillService := newSkillServiceForTest(t, server.URL)
	syncPollOnce(skillService)

	assertFileContent(t, filepath.Join(repoPath, "demo", "SKILL.md"), "kept local")
	assertFileContent(t, filepath.Join(primary, "demo", "SKILL.md"), "kept local")
	assertFileContent(t, filepath.Join(secondary, "demo", "SKILL.md"), "kept local")
	assertNotSymlink(t, filepath.Join(primary, "demo"))
	assertNotSymlink(t, filepath.Join(secondary, "demo"))

	state, err = skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Skills["demo"]; ok {
		t.Fatal("remote-deleted synced skill should be removed from sync state")
	}
}

func TestInitialNacosPullSkipsLocalChanges(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "demo", "SKILL.md", "local chosen")

	primary := filepath.Join(t.TempDir(), "primary")
	secondary := filepath.Join(t.TempDir(), "secondary")
	if err := skill.LinkSkillToAgent(repoPath, "demo", primary); err != nil {
		t.Fatal(err)
	}
	if err := skill.LinkSkillToAgent(repoPath, "demo", secondary); err != nil {
		t.Fatal(err)
	}

	localHash, err := skill.ComputeDirectoryHash(filepath.Join(repoPath, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Label:   "latest",
		Config:  skill.SyncConfig{AutoUpload: true},
		Repo:    repoPath,
		Agents: []skill.AgentDir{
			{Name: "primary", Path: primary},
			{Name: "secondary", Path: secondary},
		},
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:      "demo",
				Label:     "latest",
				RemoteMd5: "m1",
				LocalHash: localHash,
				Status:    skill.SyncStatusLocalChanges,
			},
		},
	}

	pullAndLinkOne(state, repoPath, "demo", nil, startInitOptions{}, nil)

	assertFileContent(t, filepath.Join(repoPath, "demo", "SKILL.md"), "local chosen")
	assertFileContent(t, filepath.Join(primary, "demo", "SKILL.md"), "local chosen")
	assertFileContent(t, filepath.Join(secondary, "demo", "SKILL.md"), "local chosen")
	assertSkillSymlink(t, primary, "demo")
	assertSkillSymlink(t, secondary, "demo")
	if got := state.Skills["demo"].Status; got != skill.SyncStatusLocalChanges {
		t.Fatalf("status = %s, want local changes", got)
	}
}

func TestSyncPollOnceDoesNotOverwriteDirtyLocalOnRemoteUpdate(t *testing.T) {
	withTempHome(t)

	primary := filepath.Join(t.TempDir(), "primary")
	secondary := filepath.Join(t.TempDir(), "secondary")
	writeSkillFile(t, primary, "demo", "SKILL.md", "old")
	writeSkillFile(t, secondary, "demo", "SKILL.md", "old")

	syncedHash, err := skill.ComputeDirectoryHash(filepath.Join(primary, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	saveSyncStateForTest(t, primary, secondary, syncedHash)

	writeSkillFile(t, primary, "demo", "SKILL.md", "local edit")

	server := newSkillQueryServer(t, "m1", "m2", "v2", map[string]string{
		"demo/SKILL.md": "remote update",
	})
	defer server.Close()

	skillService := newSkillServiceForTest(t, server.URL)
	syncPollOnce(skillService)

	assertFileContent(t, filepath.Join(primary, "demo", "SKILL.md"), "local edit")
	assertFileContent(t, filepath.Join(secondary, "demo", "SKILL.md"), "old")

	state, err := skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	entry := state.Skills["demo"]
	if entry.Status != skill.SyncStatusConflict {
		t.Fatalf("status = %s, want conflict", entry.Status)
	}
	if entry.RemoteMd5 != "m2" || entry.ResolvedVersion != "v2" {
		t.Fatalf("remote state = md5:%s version:%s, want m2/v2", entry.RemoteMd5, entry.ResolvedVersion)
	}
}

func TestSyncPollOnceForcesPullWhenResolvedVersionMovesOn304(t *testing.T) {
	withTempHome(t)

	primary := filepath.Join(t.TempDir(), "primary")
	secondary := filepath.Join(t.TempDir(), "secondary")
	writeSkillFile(t, primary, "demo", "SKILL.md", "old")
	writeSkillFile(t, secondary, "demo", "SKILL.md", "old")

	syncedHash, err := skill.ComputeDirectoryHash(filepath.Join(primary, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	saveSyncStateForTest(t, primary, secondary, syncedHash)

	zipBytes := makeZip(t, map[string]string{
		"demo/SKILL.md": "new",
	})
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/client/ai/skills" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("name"); got != "demo" {
			t.Fatalf("name = %s, want demo", got)
		}
		if got := r.URL.Query().Get("label"); got != "latest" {
			t.Fatalf("label = %s, want latest", got)
		}
		switch requests {
		case 0:
			if got := r.URL.Query().Get("md5"); got != "m1" {
				t.Fatalf("first md5 = %s, want m1", got)
			}
			w.Header().Set("X-Nacos-Skill-Md5", "m1")
			w.Header().Set("X-Nacos-Skill-Resolved-Version", "v2")
			w.WriteHeader(http.StatusNotModified)
		case 1:
			if got := r.URL.Query().Get("md5"); got != "" {
				t.Fatalf("force-pull md5 = %s, want empty", got)
			}
			w.Header().Set("X-Nacos-Skill-Md5", "m2")
			w.Header().Set("X-Nacos-Skill-Resolved-Version", "v2")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(zipBytes)
		default:
			t.Fatalf("unexpected extra request %d", requests+1)
		}
		requests++
	}))
	defer server.Close()

	skillService := newSkillServiceForTest(t, server.URL)
	syncPollOnce(skillService)

	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	assertFileContent(t, filepath.Join(primary, "demo", "SKILL.md"), "new")
	assertFileContent(t, filepath.Join(secondary, "demo", "SKILL.md"), "new")

	state, err := skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	entry := state.Skills["demo"]
	if entry.Status != skill.SyncStatusSynced {
		t.Fatalf("status = %s, want synced", entry.Status)
	}
	if entry.RemoteMd5 != "m2" || entry.ResolvedVersion != "v2" {
		t.Fatalf("remote state = md5:%s version:%s, want m2/v2", entry.RemoteMd5, entry.ResolvedVersion)
	}
}

func TestSyncPollOnceDoesNotReapplyWhenFetchedHashMatchesSyncedWithoutMd5(t *testing.T) {
	withTempHome(t)

	repoPath, err := skill.EnsureSkillRepo()
	if err != nil {
		t.Fatal(err)
	}
	writeSkillFile(t, repoPath, "demo", "SKILL.md", "old")

	primary := filepath.Join(t.TempDir(), "primary")
	secondary := filepath.Join(t.TempDir(), "secondary")
	if err := skill.LinkSkillToAgent(repoPath, "demo", primary); err != nil {
		t.Fatal(err)
	}
	if err := skill.LinkSkillToAgent(repoPath, "demo", secondary); err != nil {
		t.Fatal(err)
	}

	syncedHash, err := skill.ComputeDirectoryHash(filepath.Join(repoPath, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Mode:    skill.SyncModeNacos,
		Label:   "latest",
		Config:  skill.SyncConfig{AutoUpload: true},
		Repo:    repoPath,
		Agents: []skill.AgentDir{
			{Name: "primary", Path: primary},
			{Name: "secondary", Path: secondary},
		},
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:       "demo",
				Label:      "latest",
				LocalHash:  syncedHash,
				SyncedHash: syncedHash,
				Status:     skill.SyncStatusSynced,
			},
		},
	}
	if err := skill.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}

	zipBytes := makeZip(t, map[string]string{
		"demo/SKILL.md": "old",
	})
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/client/ai/skills" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		requests++
		if got := r.URL.Query().Get("md5"); got != "" {
			t.Fatalf("md5 = %s, want empty", got)
		}
		w.Header().Set("X-Nacos-Skill-Resolved-Version", "v1")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(zipBytes)
	}))
	defer server.Close()

	skillService := newSkillServiceForTest(t, server.URL)
	syncPollOnce(skillService)

	backupRoot := filepath.Join(repoPath, "..", ".skill-sync-backup")
	if _, err := os.Stat(backupRoot); err == nil {
		t.Fatalf("backup root %s exists; unchanged content should not be backed up", backupRoot)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	assertFileContent(t, filepath.Join(repoPath, "demo", "SKILL.md"), "old")
	assertSkillSymlink(t, primary, "demo")
	assertSkillSymlink(t, secondary, "demo")

	state, err = skill.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	entry := state.Skills["demo"]
	if entry.Status != skill.SyncStatusSynced {
		t.Fatalf("status = %s, want synced", entry.Status)
	}
	if entry.ResolvedVersion != "v1" {
		t.Fatalf("resolved version = %q, want v1", entry.ResolvedVersion)
	}
	if entry.SyncedHash != syncedHash {
		t.Fatalf("synced hash changed: %s != %s", entry.SyncedHash, syncedHash)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestSyncPollOnceSkipsDownloadWhenVersionUnchangedWithoutMd5(t *testing.T) {
	withTempHome(t)

	primary := filepath.Join(t.TempDir(), "primary")
	secondary := filepath.Join(t.TempDir(), "secondary")
	writeSkillFile(t, primary, "demo", "SKILL.md", "old")
	writeSkillFile(t, secondary, "demo", "SKILL.md", "old")

	syncedHash, err := skill.ComputeDirectoryHash(filepath.Join(primary, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Label:   "latest",
		Agents: []skill.AgentDir{
			{Name: "primary", Path: primary},
			{Name: "secondary", Path: secondary},
		},
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:            "demo",
				Label:           "latest",
				ResolvedVersion: "v1",
				LocalHash:       syncedHash,
				SyncedHash:      syncedHash,
				Status:          skill.SyncStatusSynced,
			},
		},
	}
	if err := skill.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}

	describeRequests := 0
	fetchRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nacos/v3/admin/ai/skills":
			describeRequests++
			if got := r.URL.Query().Get("skillName"); got != "demo" {
				t.Fatalf("skillName = %s, want demo", got)
			}
			resp := skill.V3Response{
				Code: 0,
				Data: json.RawMessage(`{"labels":{"latest":"v1"}}`),
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "/nacos/v3/client/ai/skills":
			fetchRequests++
			t.Fatalf("fetch should be skipped when version is unchanged and md5 is unavailable")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	skillService := newSkillServiceForTest(t, server.URL)
	syncPollOnce(skillService)

	if describeRequests != 1 {
		t.Fatalf("describe requests = %d, want 1", describeRequests)
	}
	if fetchRequests != 0 {
		t.Fatalf("fetch requests = %d, want 0", fetchRequests)
	}
}

func TestRotateLogIfNeeded(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "skill-sync.log")
	if err := os.WriteFile(logPath, []byte("current-large"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath+".1", []byte("old-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath+".2", []byte("old-2"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := rotateLogIfNeeded(logPath, 4, 2); err != nil {
		t.Fatal(err)
	}

	assertFileContent(t, logPath+".1", "current-large")
	assertFileContent(t, logPath+".2", "old-1")
	if _, err := os.Stat(logPath); err == nil {
		t.Fatal("active log should be moved after rotation")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestSkillSyncStartHasNonInteractiveFlag(t *testing.T) {
	if flag := skillSyncStartCmd.Flags().Lookup("non-interactive"); flag == nil {
		t.Fatal("start command should expose --non-interactive")
	}
}

func TestSkillSyncStartDoesNotHaveAllFlag(t *testing.T) {
	if flag := skillSyncStartCmd.Flags().Lookup("all"); flag != nil {
		t.Fatal("start command should not expose --all; use add --all for bulk membership")
	}
}

func TestDecideStartConflictsNonInteractiveSkips(t *testing.T) {
	decision := decideStartConflicts([]startConflict{
		{Name: "demo", Reason: "local differs from Nacos"},
	}, false)
	if decision != startConflictSkip {
		t.Fatalf("decision = %s, want skip", decision)
	}
}

func withTempHome(t *testing.T) {
	t.Helper()
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	if err := os.Setenv("HOME", tmpHome); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
	})
}

func saveSyncStateForTest(t *testing.T, primary, secondary, syncedHash string) {
	t.Helper()
	state := &skill.SyncState{
		Version: skill.SyncStateVersion,
		Label:   "latest",
		Agents: []skill.AgentDir{
			{Name: "primary", Path: primary},
			{Name: "secondary", Path: secondary},
		},
		Skills: map[string]skill.SyncSkillEntry{
			"demo": {
				Name:            "demo",
				Label:           "latest",
				ResolvedVersion: "v1",
				RemoteMd5:       "m1",
				LocalHash:       syncedHash,
				SyncedHash:      syncedHash,
				Status:          skill.SyncStatusSynced,
			},
		},
	}
	if err := skill.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}
}

func newSkillQueryServer(t *testing.T, expectedMd5, returnedMd5, returnedVersion string, files map[string]string) *httptest.Server {
	t.Helper()
	zipBytes := makeZip(t, files)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/client/ai/skills" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("name"); got != "demo" {
			t.Fatalf("name = %s, want demo", got)
		}
		if got := r.URL.Query().Get("label"); got != "latest" {
			t.Fatalf("label = %s, want latest", got)
		}
		if got := r.URL.Query().Get("md5"); got != expectedMd5 {
			t.Fatalf("md5 = %s, want %s", got, expectedMd5)
		}
		w.Header().Set("X-Nacos-Skill-Md5", returnedMd5)
		w.Header().Set("X-Nacos-Skill-Resolved-Version", returnedVersion)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(zipBytes)
	}))
}

func newSkillServiceForTest(t *testing.T, serverURL string) *skill.SkillService {
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
	return skill.NewSkillService(nacosClient)
}

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeSkillFile(t *testing.T, root, skillName, relPath, content string) {
	t.Helper()
	path := filepath.Join(root, skillName, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, string(data), want)
	}
}

func assertFileMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s exists, want missing", path)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func assertSkillSymlink(t *testing.T, agentPath, skillName string) {
	t.Helper()
	skillPath := filepath.Join(agentPath, skillName)
	info, err := os.Lstat(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s should be a symlink, got %v", skillPath, info.Mode())
	}
}

func assertNotSymlink(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("%s should not be a symlink", path)
	}
}
