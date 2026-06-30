package skill

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/client"
)

func TestUploadSkillOnlyUploadsDraft(t *testing.T) {
	var uploadCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nacos/v3/admin/ai/skills/upload":
			uploadCalled = true
			if r.Method != http.MethodPost {
				t.Fatalf("upload method = %s, want POST", r.Method)
			}
			if got := r.URL.Query().Get("namespaceId"); got != "test-ns" {
				t.Fatalf("upload namespaceId = %s, want test-ns", got)
			}
			if got := r.URL.Query().Get("overwrite"); got != "false" {
				t.Fatalf("upload overwrite = %s, want false", got)
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("parse multipart upload: %v", err)
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("read uploaded file: %v", err)
			}
			defer file.Close()
			if header.Filename != "demo-skill.zip" {
				t.Fatalf("uploaded filename = %s, want demo-skill.zip", header.Filename)
			}
			data, err := io.ReadAll(file)
			if err != nil {
				t.Fatalf("read upload body: %v", err)
			}
			if !strings.Contains(string(data), "SKILL.md") {
				t.Fatalf("uploaded zip does not contain SKILL.md")
			}
			w.WriteHeader(http.StatusOK)
		case "/nacos/v3/admin/ai/skills/submit":
			t.Fatal("upload should not submit skill draft")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	skillDir := t.TempDir() + "/demo-skill"
	if err := os.Mkdir(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillDir+"/SKILL.md", []byte("# Demo Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if err := NewSkillService(nacosClient).UploadSkill(skillDir, false); err != nil {
		t.Fatal(err)
	}
	if !uploadCalled {
		t.Fatal("upload was not called")
	}
}

func TestUploadSkillSendsOverwriteQueryParam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/admin/ai/skills/upload" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("namespaceId"); got != "test-ns" {
			t.Fatalf("upload namespaceId = %s, want test-ns", got)
		}
		if got := r.URL.Query().Get("overwrite"); got != "true" {
			t.Fatalf("upload overwrite = %s, want true", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	skillDir := t.TempDir() + "/demo-skill"
	if err := os.Mkdir(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillDir+"/SKILL.md", []byte("# Demo Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if err := NewSkillService(nacosClient).UploadSkill(skillDir, true); err != nil {
		t.Fatal(err)
	}
}

func TestUploadSkillFollowsRootSymlinkDirectory(t *testing.T) {
	var zipData []byte
	var uploadedFilename string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/admin/ai/skills/upload" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart upload: %v", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("read uploaded file: %v", err)
		}
		defer file.Close()
		uploadedFilename = header.Filename
		zipData, err = io.ReadAll(file)
		if err != nil {
			t.Fatalf("read upload body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmp := t.TempDir()
	realDir := filepath.Join(tmp, "real-skill")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "SKILL.md"), []byte("# Real Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(tmp, "linked-skill")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if err := NewSkillService(nacosClient).UploadSkill(linkDir, false); err != nil {
		t.Fatal(err)
	}
	if uploadedFilename != "linked-skill.zip" {
		t.Fatalf("uploaded filename = %s, want linked-skill.zip", uploadedFilename)
	}
	assertZipContains(t, zipData, "SKILL.md")
}

func TestSubmitSkillSendsFormParams(t *testing.T) {
	var submitCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/admin/ai/skills/submit" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		submitCalled = true
		if r.Method != http.MethodPost {
			t.Fatalf("submit method = %s, want POST", r.Method)
		}
		if got := r.URL.Query().Get("namespaceId"); got != "test-ns" {
			t.Fatalf("submit namespaceId = %s, want test-ns", got)
		}
		if got := r.URL.Query().Get("skillName"); got != "demo-skill" {
			t.Fatalf("submit skillName = %s, want demo-skill", got)
		}
		if got := r.URL.Query().Get("version"); got != "1.0.0" {
			t.Fatalf("submit version = %s, want 1.0.0", got)
		}
		_ = json.NewEncoder(w).Encode(V3Response{Code: 0, Message: "success"})
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if err := NewSkillService(nacosClient).SubmitSkill("demo-skill", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if !submitCalled {
		t.Fatal("submit was not called")
	}
}

func assertZipContains(t *testing.T, data []byte, name string) {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	for _, f := range reader.File {
		if f.Name == name {
			return
		}
	}
	t.Fatalf("zip entry %q not found", name)
}

func TestUpdateSkillScopeSendsParams(t *testing.T) {
	var scopeCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/admin/ai/skills/scope" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		scopeCalled = true
		if r.Method != http.MethodPut {
			t.Fatalf("scope method = %s, want PUT", r.Method)
		}
		if got := r.URL.Query().Get("namespaceId"); got != "test-ns" {
			t.Fatalf("scope namespaceId = %s, want test-ns", got)
		}
		if got := r.URL.Query().Get("skillName"); got != "demo-skill" {
			t.Fatalf("scope skillName = %s, want demo-skill", got)
		}
		if got := r.URL.Query().Get("scope"); got != "PUBLIC" {
			t.Fatalf("scope = %s, want PUBLIC", got)
		}
		_ = json.NewEncoder(w).Encode(V3Response{Code: 0, Message: "success"})
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if err := NewSkillService(nacosClient).UpdateSkillScope("demo-skill", "public"); err != nil {
		t.Fatal(err)
	}
	if !scopeCalled {
		t.Fatal("scope was not called")
	}
}

func TestUpdateSkillBizTagsSendsParams(t *testing.T) {
	var bizTagsCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/admin/ai/skills/biz-tags" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		bizTagsCalled = true
		if r.Method != http.MethodPut {
			t.Fatalf("bizTags method = %s, want PUT", r.Method)
		}
		if got := r.URL.Query().Get("namespaceId"); got != "test-ns" {
			t.Fatalf("bizTags namespaceId = %s, want test-ns", got)
		}
		if got := r.URL.Query().Get("skillName"); got != "demo-skill" {
			t.Fatalf("bizTags skillName = %s, want demo-skill", got)
		}
		if got := r.URL.Query().Get("bizTags"); got != "retail,finance" {
			t.Fatalf("bizTags = %s, want retail,finance", got)
		}
		_ = json.NewEncoder(w).Encode(V3Response{Code: 0, Message: "success"})
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if err := NewSkillService(nacosClient).UpdateSkillBizTags("demo-skill", "retail,finance"); err != nil {
		t.Fatal(err)
	}
	if !bizTagsCalled {
		t.Fatal("bizTags was not called")
	}
}

func TestOnlineSkillSendsSkillScopeParams(t *testing.T) {
	var onlineCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/admin/ai/skills/online" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		onlineCalled = true
		if r.Method != http.MethodPost {
			t.Fatalf("online method = %s, want POST", r.Method)
		}
		if got := r.URL.Query().Get("namespaceId"); got != "test-ns" {
			t.Fatalf("online namespaceId = %s, want test-ns", got)
		}
		if got := r.URL.Query().Get("skillName"); got != "demo-skill" {
			t.Fatalf("online skillName = %s, want demo-skill", got)
		}
		if got := r.URL.Query().Get("scope"); got != "skill" {
			t.Fatalf("online scope = %s, want skill", got)
		}
		if got := r.URL.Query().Get("version"); got != "" {
			t.Fatalf("online version = %s, want empty", got)
		}
		_ = json.NewEncoder(w).Encode(V3Response{Code: 0, Message: "success"})
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if err := NewSkillService(nacosClient).OnlineSkill("demo-skill", ""); err != nil {
		t.Fatal(err)
	}
	if !onlineCalled {
		t.Fatal("online was not called")
	}
}

func TestOfflineSkillSendsVersionScopeParams(t *testing.T) {
	var offlineCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/admin/ai/skills/offline" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		offlineCalled = true
		if r.Method != http.MethodPost {
			t.Fatalf("offline method = %s, want POST", r.Method)
		}
		if got := r.URL.Query().Get("namespaceId"); got != "test-ns" {
			t.Fatalf("offline namespaceId = %s, want test-ns", got)
		}
		if got := r.URL.Query().Get("skillName"); got != "demo-skill" {
			t.Fatalf("offline skillName = %s, want demo-skill", got)
		}
		if got := r.URL.Query().Get("scope"); got != "version" {
			t.Fatalf("offline scope = %s, want version", got)
		}
		if got := r.URL.Query().Get("version"); got != "1.0.0" {
			t.Fatalf("offline version = %s, want 1.0.0", got)
		}
		_ = json.NewEncoder(w).Encode(V3Response{Code: 0, Message: "success"})
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if err := NewSkillService(nacosClient).OfflineSkill("demo-skill", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if !offlineCalled {
		t.Fatal("offline was not called")
	}
}

func newTestNacosClient(serverURL string) (*client.NacosClient, error) {
	return client.NewNacosClient(
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
}

// TestUploadSkillZipPaths verifies that:
// 1. ZIP entries have NO extra skillName/ prefix (#46)
// 2. ZIP entries use forward slashes even on Windows (#26)
func TestUploadSkillZipPaths(t *testing.T) {
	var zipData []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/admin/ai/skills/upload" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("read uploaded file: %v", err)
		}
		defer file.Close()
		zipData, err = io.ReadAll(file)
		if err != nil {
			t.Fatalf("read upload body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create a skill directory with nested subdirectory
	skillDir := filepath.Join(t.TempDir(), "my-skill")
	subDir := filepath.Join(skillDir, "prompts", "templates")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "default.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if err := NewSkillService(nacosClient).UploadSkill(skillDir, false); err != nil {
		t.Fatal(err)
	}

	// Parse the ZIP and verify paths
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}

	expectedPaths := map[string]bool{
		"SKILL.md":                      false,
		"prompts/templates/default.txt": false,
	}

	for _, f := range reader.File {
		// Verify no backslashes (Windows path separator)
		if strings.Contains(f.Name, "\\") {
			t.Errorf("ZIP entry contains backslash: %q", f.Name)
		}
		// Verify no skillName prefix
		if strings.HasPrefix(f.Name, "my-skill/") {
			t.Errorf("ZIP entry has unexpected skillName prefix: %q", f.Name)
		}
		if _, ok := expectedPaths[f.Name]; ok {
			expectedPaths[f.Name] = true
		} else {
			t.Errorf("unexpected ZIP entry: %q", f.Name)
		}
	}

	for path, found := range expectedPaths {
		if !found {
			t.Errorf("expected ZIP entry not found: %q", path)
		}
	}
}
