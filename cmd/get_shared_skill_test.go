package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/client"
	"github.com/nacos-group/nacos-cli/internal/skill"
)

func TestOrdinarySkillBatchRetainsProgressSummaryAndPartialFailure(t *testing.T) {
	resetRootConfigForTest(t)
	archive := makeZip(t, map[string]string{"123/SKILL.md": "# Demo\n"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/client/ai/skills" {
			t.Errorf("ordinary download changed endpoint: %s", r.URL.Path)
		}
		if r.URL.Query().Get("name") == "missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("name") != "123" {
			t.Errorf("expected numeric resource name: %s", r.URL)
		}
		w.Header().Set("X-Nacos-Skill-Resolved-Version", "v2")
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	c, err := client.NewNacosClient(strings.TrimPrefix(server.URL, "http://"), "team", client.AuthTypeNone, "", "", "", "", "", "", "", "http")
	if err != nil {
		t.Fatal(err)
	}
	command := newSkillGetCommandForSession(c, "", "", nil)
	command.SilenceUsage, command.SilenceErrors = true, true
	command.SetArgs([]string{"missing", "123", "--no-hub", "-o", t.TempDir()})
	var output, errors bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&errors)
	if err := command.Execute(); err == nil {
		t.Fatal("partial failure must return an error")
	}
	for _, expected := range []string{"[1/2] Fetching skill: missing", "[2/2] Fetching skill: 123", "Total: 2 | Success: 1 | Failed: 1", "Failed skills: missing"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("missing batch result %q in %s", expected, output.String())
		}
	}
	if !strings.Contains(errors.String(), "failed to download skill 'missing'") {
		t.Errorf("missing per-resource failure: %s", errors.String())
	}
}

func TestSharedSkillGetPreservesExtractionAndOrdinaryLock(t *testing.T) {
	for _, testCase := range []struct {
		name, destination    string
		shared, existingLock bool
	}{
		{"shared-without-lock", "", true, false},
		{"shared-with-existing-lock", "directory", true, true},
		{"ordinary-download", "directory", false, false},
		{"shared-with-sync-link", "link", true, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			resetRootConfigForTest(t)
			outputDir := t.TempDir()
			archive := makeZip(t, map[string]string{"demo/SKILL.md": "# Demo\n"})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/nacos/v3/admin/ai/skills/list" {
					if r.URL.Query().Get("namespaceId") != skill.SharedNamespaceID || r.URL.Query().Get("skillName") != "demo" || r.URL.Query().Get("search") != "accurate" {
						t.Errorf("unexpected discovery %s", r.URL)
					}
					fmt.Fprint(w, `{"code":0,"data":{"totalCount":1,"pageNumber":1,"pagesAvailable":1,"pageItems":[{"name":"17","hubEntryId":"17","displayName":"demo","version":"v2"}]}}`)
					return
				}
				wantNamespace, wantName := "team", "demo"
				if testCase.shared {
					wantNamespace, wantName = skill.SharedNamespaceID, "17"
				}
				if r.URL.Path != "/nacos/v3/client/ai/skills" || r.URL.Query().Get("namespaceId") != wantNamespace || r.URL.Query().Get("name") != wantName || r.URL.Query().Get("version") != "v2" {
					t.Errorf("unexpected download identity: %s", r.URL)
				}
				w.Header().Set("X-Nacos-Skill-Resolved-Version", "v2")
				_, _ = w.Write(archive)
			}))
			defer server.Close()
			c, err := client.NewNacosClient(strings.TrimPrefix(server.URL, "http://"), "team", client.AuthTypeNone, "", "", "", "", "", "", "", "http")
			if err != nil {
				t.Fatal(err)
			}
			lockPath := filepath.Join(outputDir, skill.SkillsLockFile)
			var originalLock []byte
			if testCase.existingLock {
				lock, err := skill.LoadSkillsLock(outputDir)
				if err != nil {
					t.Fatal(err)
				}
				lock.RecordInstall("existing", "v1", "", "md5", "v1")
				if err := skill.SaveSkillsLock(outputDir, lock); err != nil {
					t.Fatal(err)
				}
				originalLock, _ = os.ReadFile(lockPath)
			}
			command := newSkillGetCommandForSession(c, "", "", nil)
			args := []string{"demo", "--version", "v2", "-o", outputDir}
			if testCase.shared {
				args = append(args, "--hub-entry-id", "17")
			} else {
				args = append(args, "--no-hub")
			}
			dest := filepath.Join(outputDir, "demo")
			switch testCase.destination {
			case "link":
				if err := os.Symlink(t.TempDir(), dest); err != nil {
					if runtime.GOOS == "windows" {
						t.Skipf("symlink unavailable: %v", err)
					}
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(dest, 0755); err != nil {
					t.Fatal(err)
				}
			}
			if testCase.destination != "" {
				if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("old"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			command.SetArgs(args)
			var output, warnings bytes.Buffer
			command.SetErr(&warnings)
			command.SetOut(&output)
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(filepath.Join(dest, "SKILL.md")); err != nil || string(data) != "# Demo\n" {
				t.Fatalf("extracted content=%q, err=%v", data, err)
			}
			if strings.Contains(warnings.String(), dest+" already exists") != (testCase.destination == "directory") ||
				strings.Contains(warnings.String(), dest+" is a link; matching files in its target") != (testCase.destination == "link") {
				t.Fatalf("unexpected overwrite warning: %q", warnings.String())
			}
			if c.Namespace != "team" {
				t.Fatal("Hub download changed the session Namespace")
			}
			if testCase.shared {
				if !strings.Contains(output.String(), "Location: "+outputDir+"\n") {
					t.Errorf("incorrect shared location: %s", output.String())
				}
				updatedLock, err := os.ReadFile(lockPath)
				if testCase.existingLock && (err != nil || !bytes.Equal(originalLock, updatedLock)) {
					t.Fatal("shared download changed the ordinary sync lock")
				}
				if !testCase.existingLock && !os.IsNotExist(err) {
					t.Fatalf("shared download created a name-only lock: %v", err)
				}
				return
			}
			lock, err := skill.LoadSkillsLock(outputDir)
			if err != nil || lock.Skills["demo"].ResolvedVersion != "v2" {
				t.Fatalf("ordinary tracking lost: %+v %v", lock, err)
			}
			if !strings.Contains(output.String(), "Location: "+filepath.Join(outputDir, "demo")) {
				t.Errorf("ordinary location changed: %s", output.String())
			}
		})
	}
}
