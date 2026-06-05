package cmd

import (
	"archive/zip"
	"bytes"
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
