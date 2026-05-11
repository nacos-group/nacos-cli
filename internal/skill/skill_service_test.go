package skill

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

	nacosClient, err := client.NewNacosClient(strings.TrimPrefix(server.URL, "http://"), "test-ns", client.AuthTypeToken, "", "", "", "", "token")
	if err != nil {
		t.Fatal(err)
	}

	if err := NewSkillService(nacosClient).UploadSkill(skillDir); err != nil {
		t.Fatal(err)
	}
	if !uploadCalled {
		t.Fatal("upload was not called")
	}
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

	nacosClient, err := client.NewNacosClient(strings.TrimPrefix(server.URL, "http://"), "test-ns", client.AuthTypeToken, "", "", "", "", "token")
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
