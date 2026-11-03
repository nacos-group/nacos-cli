package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/client"
	"github.com/nacos-group/nacos-cli/internal/config"
	"github.com/nacos-group/nacos-cli/internal/skill"
)

func TestSkillGetSelectsDuplicateNamesAndPinsHubVersion(t *testing.T) {
	for _, test := range []struct {
		name              string
		args              []string
		choice            string
		wantID, wantError string
	}{
		{"explicit-id-default-version", []string{"demo", "17"}, "", "17", ""},
		{"explicit-positional-version", []string{"demo", "18", "v1"}, "", "18", ""},
		{"interactive-choice", []string{"demo"}, "2", "17", ""},
		{"interactive-third-match", []string{"demo"}, "3", "18", ""},
		{"ambiguous-noninteractive", []string{"demo", "--non-interactive"}, "", "", "ambiguous name"},
		{"cancel", []string{"demo"}, "0", "", ""},
		{"invalid-choice", []string{"demo"}, "9", "", "invalid choice"},
		{"wrong-version", []string{"demo", "17", "v9"}, "", "", "only shares version"},
		{"wrong-flag-version", []string{"demo", "17", "--version", "v9"}, "", "", "only shares version"},
		{"wrong-label", []string{"demo", "17", "--label", "stable"}, "", "", "one fixed shared version"},
		{"unknown-id", []string{"demo", "99"}, "", "", "no readable Hub entry"},
		{"mismatched-name", []string{"different", "17"}, "", "", "no readable Hub entry"},
		{"id-and-no-hub", []string{"demo", "17", "--no-hub"}, "", "", "cannot be combined"},
		{"conflicting-flags", []string{"demo", "--hub", "--no-hub"}, "", "", "were all set"},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetRootConfigForTest(t)
			setTestHome(t, t.TempDir())
			path, _ := config.GetProfileConfigPath("team")
			// An explicit entry ID must override even a Namespace-only profile.
			preference := "all"
			if len(test.args) > 1 && skill.IsHubEntryID(test.args[1]) {
				preference = "namespace"
			}
			if err := (&config.Config{Source: preference}).SaveConfig(path); err != nil {
				t.Fatal(err)
			}
			var downloaded []string
			archive := makeZip(t, map[string]string{"demo/SKILL.md": "# Selected\n"})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if r.URL.Path == "/nacos/v3/admin/ai/skills/list" {
					if q.Get("search") != "accurate" {
						t.Errorf("not an exact search: %s", r.URL)
					}
					if q.Get("namespaceId") == skill.SharedNamespaceID {
						fmt.Fprint(w, `{"code":0,"data":{"totalCount":2,"pageNumber":1,"pagesAvailable":1,"pageItems":[{"name":"17","hubEntryId":"17","displayName":"demo","owner":"author-a","version":"v2"},{"name":"18","hubEntryId":"18","displayName":"demo","owner":"author-b","version":"v1"}]}}`)
					} else {
						fmt.Fprint(w, `{"code":0,"data":{"totalCount":1,"pageNumber":1,"pagesAvailable":1,"pageItems":[{"name":"demo","onlineCnt":1,"labels":{"latest":"local-v1"}}]}}`)
					}
					return
				}
				if q.Get("namespaceId") != skill.SharedNamespaceID {
					t.Errorf("download used wrong Namespace: %s", r.URL)
				}
				wantVersion := "v2"
				if q.Get("name") == "18" {
					wantVersion = "v1"
				}
				if q.Get("version") != wantVersion || q.Get("label") != "" {
					t.Errorf("unpinned version: %s", r.URL)
				}
				downloaded = append(downloaded, q.Get("name"))
				w.Header().Set("X-Nacos-Skill-Resolved-Version", wantVersion)
				_, _ = w.Write(archive)
			}))
			defer server.Close()
			c, err := client.NewNacosClient(strings.TrimPrefix(server.URL, "http://"), "business-ns", client.AuthTypeNone, "", "", "", "", "", "", "", "http")
			if err != nil {
				t.Fatal(err)
			}
			var readSelection func(string) (string, error)
			if test.choice != "" {
				readSelection = func(string) (string, error) { return test.choice, nil }
			}
			command := newSkillGetCommandForSession(c, "team", path, readSelection)
			var out bytes.Buffer
			command.SetOut(&out)
			command.SetErr(&bytes.Buffer{})
			command.SilenceUsage, command.SilenceErrors = true, true
			dir := t.TempDir()
			command.SetArgs(append(append([]string{}, test.args...), "-o", dir))
			err = command.Execute()
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("error=%v, want %q", err, test.wantError)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if test.wantID != "" {
				if len(downloaded) != 1 || downloaded[0] != test.wantID {
					t.Fatalf("downloads=%v", downloaded)
				}
				if _, err := os.Stat(filepath.Join(dir, "demo", "SKILL.md")); err != nil {
					t.Fatal(err)
				}
			} else if len(downloaded) != 0 {
				t.Fatalf("unexpected download %v", downloaded)
			}
			if c.Namespace != "business-ns" {
				t.Fatal("session Namespace changed")
			}
			if strings.Contains(out.String(), skill.SharedNamespaceID) {
				t.Fatalf("internal Namespace leaked into UI: %s", out.String())
			}
			if strings.Contains(out.String(), "Multiple skills") && !strings.Contains(out.String(), "skill-get demo 17") {
				t.Fatal("missing direct selection command")
			}
		})
	}
}

func TestSkillGetSelectionArgumentsPreserveBatchAndRejectVersionConflict(t *testing.T) {
	names, id, version, err := parseSkillGetSelection([]string{"alpha", "beta"}, "v1")
	if err != nil || len(names) != 2 || id != "" || version != "v1" {
		t.Fatalf("legacy batch changed: %v %s %s %v", names, id, version, err)
	}
	if _, _, _, err := parseSkillGetSelection([]string{"demo", "17", "v2"}, "v3"); err == nil {
		t.Fatal("conflicting versions accepted")
	}
}
