package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/config"
	"github.com/nacos-group/nacos-cli/internal/skill"
	"github.com/spf13/cobra"
)

func TestHubEntryIDRequiresOneNameAndPositiveDecimalID(t *testing.T) {
	for _, factory := range []func() *cobra.Command{newSkillGetCommand, newAgentSpecGetCommand} {
		for _, tc := range []struct {
			args []string
			want string
		}{
			{[]string{"alpha", "123", "--hub-entry-id", "9"}, "exactly one resource name"},
			{[]string{"alpha", "--hub-entry-id="}, "positive decimal"},
			{[]string{"alpha", "--hub-entry-id", "0"}, "positive decimal"},
			{[]string{"alpha", "--hub-entry-id", "abc"}, "positive decimal"},
		} {
			cmd := factory()
			cmd.SilenceErrors, cmd.SilenceUsage = true, true
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%s %v: got %v, want %s", cmd.Name(), tc.args, err, tc.want)
			}
		}
	}
}

func TestHubGetBatchCancellationPreservesResults(t *testing.T) {
	for _, resource := range []string{"skill", "agentspec"} {
		for _, first := range []string{"ok", "missing"} {
			t.Run(resource+"/"+first, func(t *testing.T) {
				profile := filepath.Join(t.TempDir(), "profile.conf")
				if err := (&config.Config{Source: "all"}).SaveConfig(profile); err != nil {
					t.Fatal(err)
				}
				archive := makeZip(t, map[string]string{"ok/SKILL.md": "# OK"})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					q := r.URL.Query()
					if !strings.HasSuffix(r.URL.Path, "/list") {
						if q.Get("name") != "ok" || q.Get("namespaceId") != "business" {
							t.Errorf("unexpected download: %s", r.URL)
						}
						if resource == "skill" {
							_, _ = w.Write(archive)
						} else {
							fmt.Fprint(w, `{"code":0,"data":{"content":"{}"}}`)
						}
						return
					}
					name := q.Get("skillName")
					if resource == "agentspec" {
						name = q.Get("agentSpecName")
					}
					items := "[]"
					switch name {
					case "ok", "same":
						if q.Get("namespaceId") != skill.SharedNamespaceID {
							items = fmt.Sprintf(`[{"name":%q,"onlineCnt":1}]`, name)
						} else if name == "same" {
							if resource == "agentspec" {
								agentSpecHubListFixture(w)
								return
							}
							items = `[{"name":"17","hubEntryId":"17","displayName":"same","version":"v2"}]`
						}
					case "missing":
					default:
						t.Errorf("searched after cancellation: %s", r.URL)
					}
					count := 0
					if items != "[]" {
						count = 1
					}
					fmt.Fprintf(w, `{"code":0,"data":{"totalCount":%d,"pagesAvailable":1,"pageItems":%s}}`, count, items)
				}))
				defer server.Close()
				c := newAgentSpecHubTestClient(t, server)
				prompts := 0
				cancel := func(string) (string, error) { prompts++; return "0", nil }
				var cmd *cobra.Command
				if resource == "skill" {
					cmd = newSkillGetCommandForSession(c, "", profile, cancel)
				} else {
					cmd = newAgentSpecGetCommandForSession(c, "", profile, cancel)
				}
				var output bytes.Buffer
				cmd.SetOut(&output)
				cmd.SetErr(&output)
				cmd.SilenceErrors, cmd.SilenceUsage = true, true
				cmd.SetArgs([]string{first, "same", "unreached", "-o", t.TempDir()})
				err := cmd.Execute()
				summary := "Total: 1 | Success: 1 | Failed: 0"
				if first == "missing" {
					summary = "Total: 1 | Success: 0 | Failed: 1"
					if err == nil || !strings.Contains(err.Error(), "missing") {
						t.Fatalf("lost earlier failure: %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if prompts != 1 || !strings.Contains(output.String(), summary) || strings.Contains(output.String(), "unreached") {
					t.Fatalf("prompts=%d output=%s", prompts, output.String())
				}
			})
		}
	}
}
