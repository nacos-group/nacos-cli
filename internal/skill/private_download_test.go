package skill

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateSkillDownloadsSendResourceNameAndExactVersion(t *testing.T) {
	var file bytes.Buffer
	archive := zip.NewWriter(&file)
	entry, err := archive.Create("web-monitor/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("# web-monitor\n")); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()
		if r.URL.Path != "/nacos/v3/client/ai/skills" || r.Method != http.MethodGet ||
			params.Get("name") != "web-monitor" || params.Get("skillName") != "web-monitor" ||
			params.Get("version") != "0.0.2" || params.Get("namespaceId") != "test-ns" {
			http.Error(w, "missing Skill query parameters", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Nacos-Skill-Resolved-Version", "0.0.2")
		_, _ = w.Write(file.Bytes())
	}))
	defer server.Close()

	client, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	service := NewSkillService(client)

	t.Run("GetSkill", func(t *testing.T) {
		out := t.TempDir()
		if err := service.GetSkill("web-monitor", out, "0.0.2", ""); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(out, "web-monitor", "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("QuerySkill", func(t *testing.T) {
		out := t.TempDir()
		result, err := service.QuerySkill("web-monitor", out, "0.0.2", "", "")
		if err != nil {
			t.Fatal(err)
		}
		if !result.Updated || result.ResolvedVersion != "0.0.2" {
			t.Fatalf("unexpected result: %+v", result)
		}
		if _, err := os.Stat(filepath.Join(out, "web-monitor", "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	})
}
