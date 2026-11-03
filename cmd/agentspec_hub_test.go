package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/agentspec"
	"github.com/nacos-group/nacos-cli/internal/client"
	"github.com/nacos-group/nacos-cli/internal/config"
	"github.com/spf13/cobra"
)

func newAgentSpecHubTestClient(t *testing.T, server *httptest.Server) *client.NacosClient {
	t.Helper()
	c, err := client.NewNacosClient(strings.TrimPrefix(server.URL, "http://"), "business", client.AuthTypeToken,
		"", "", "", "", "", "", "", "http", client.WithToken("session-token"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func agentSpecHubListFixture(w http.ResponseWriter) {
	fmt.Fprint(w, `{"code":0,"data":{"totalCount":1,"pageNumber":1,"pagesAvailable":1,"pageItems":[{"name":"749","enable":true,"onlineCnt":1,"labels":{"latest":"v2"},"hub":{"hubEntryId":"749","resourceType":"agentspec","resourceName":"same","version":"v2","sourceDisplayName":"Team B","author":"author","downloadCount":4,"updateTime":123}}]}}`)
}

func TestAgentSpecListSourcePriorityJSONAndSessionReload(t *testing.T) {
	for _, preference := range []string{"", "namespace", "hub", "all"} {
		for _, flag := range []string{"", "--hub", "--no-hub"} {
			t.Run(preference+"/"+flag, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "profile.conf")
				if err := (&config.Config{Source: preference}).SaveConfig(path); err != nil {
					t.Fatal(err)
				}
				var requested []string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					q := r.URL.Query()
					if r.URL.Path != "/nacos/v3/admin/ai/agentspecs/list" || q.Get("agentSpecName") != "same" || q.Get("pageNo") != "2" || q.Get("pageSize") != "3" {
						t.Errorf("wrong request %s", r.URL)
					}
					requested = append(requested, q.Get("namespaceId"))
					if q.Get("namespaceId") == agentspec.SharedNamespaceID {
						agentSpecHubListFixture(w)
					} else {
						fmt.Fprint(w, `{"code":0,"data":{"totalCount":5,"pageNumber":2,"pagesAvailable":2,"pageItems":[{"name":"same","scope":"PRIVATE","editingVersion":"v3","onlineCnt":1}]}}`)
					}
				}))
				defer server.Close()
				c := newAgentSpecHubTestClient(t, server)
				command := newAgentSpecListCommandForSession(c, "team", path)
				var output bytes.Buffer
				command.SetOut(&output)
				args := []string{"--page", "2", "--size", "3", "--name", "same", "--output", "json"}
				if flag != "" {
					args = append(args, flag)
				}
				command.SetArgs(args)
				if err := command.Execute(); err != nil {
					t.Fatal(err)
				}
				mode, _ := config.NormalizeSkillSource(preference)
				if flag == "--hub" {
					mode = "hub"
				}
				if flag == "--no-hub" {
					mode = "namespace"
				}
				var want []string
				if mode != "hub" {
					want = append(want, "business")
				}
				if mode != "namespace" {
					want = append(want, agentspec.SharedNamespaceID)
				}
				if !reflect.DeepEqual(requested, want) || c.Namespace != "business" {
					t.Fatalf("requests=%v want=%v namespace=%s", requested, want, c.Namespace)
				}
				var payload map[string]json.RawMessage
				if err := json.Unmarshal(output.Bytes(), &payload); err != nil || strings.Contains(output.String(), "\033[") {
					t.Fatalf("invalid/colored JSON %s: %v", output.String(), err)
				}
				if mode == "all" {
					var pages agentspec.AgentSpecSourcePages
					if err := json.Unmarshal(output.Bytes(), &pages); err != nil || pages.Namespace.TotalCount != 5 || pages.Hub.TotalCount != 1 || pages.Hub.PageItems[0].DisplayName != "same" {
						t.Fatalf("lost independent pages: %s, %v", output.String(), err)
					}
				} else if payload["pageItems"] == nil || string(payload["pageNo"]) != "2" {
					t.Fatalf("single-source JSON contract changed: %s", output.String())
				}
				// An already-open terminal must reload source and retain ns switches.
				requested = nil
				if err := (&config.Config{Source: "namespace"}).SaveConfig(path); err != nil {
					t.Fatal(err)
				}
				c.Namespace = "switched"
				command = newAgentSpecListCommandForSession(c, "team", path)
				command.SetOut(&bytes.Buffer{})
				command.SetArgs([]string{"--page", "2", "--size", "3", "--name", "same"})
				if err := command.Execute(); err != nil || !reflect.DeepEqual(requested, []string{"switched"}) {
					t.Fatalf("profile/namespace not reloaded: %v %v", requested, err)
				}
			})
		}
	}
}

func TestAgentSpecCommandsRejectConflictBeforeProfileSetup(t *testing.T) {
	for _, factory := range []func() *cobra.Command{newAgentSpecListCommand, newAgentSpecGetCommand} {
		for _, flags := range [][]string{{"--hub", "--no-hub"}, {"--no-hub", "--hub"}, {"--hub=false", "--no-hub"}} {
			command := factory()
			args := append([]string{}, flags...)
			if command.Name() == "agentspec-get" {
				args = append(args, "same")
			}
			command.SetArgs(args)
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "hub") {
				t.Fatalf("flag conflict accepted by %s: %v %v", command.Name(), flags, err)
			}
		}
	}
	command := newAgentSpecGetCommandForSession(nil, "", "", nil)
	command.SetArgs([]string{"same", "749", "--no-hub"})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "hubEntryId") {
		t.Fatalf("explicit Hub ID and --no-hub accepted: %v", err)
	}
}

func TestAgentSpecDirectCommandReadsProfileSourceWithConnectionFlags(t *testing.T) {
	resetRootConfigForTest(t)
	setTestHome(t, t.TempDir())
	path, err := config.GetProfileConfigPath("team")
	if err != nil {
		t.Fatal(err)
	}
	if err := (&config.Config{Source: "hub"}).SaveConfig(path); err != nil {
		t.Fatal(err)
	}
	if err := config.SetCurrentProfile("team"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("namespaceId") != agentspec.SharedNamespaceID {
			t.Error("connection flags suppressed source")
		}
		agentSpecHubListFixture(w)
	}))
	defer server.Close()
	serverAddr, namespace, authType, scheme = strings.TrimPrefix(server.URL, "http://"), "business", client.AuthTypeNone, "http"
	command := newAgentSpecListCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"--output", "json"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "hubEntryId") || strings.Contains(output.String(), "\033[") {
		t.Fatalf("direct output %s", output.String())
	}
}

func TestAgentSpecListHubFailureDoesNotEmitPartialResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("namespaceId") == agentspec.SharedNamespaceID {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"denied"}`)
			return
		}
		fmt.Fprint(w, `{"code":0,"data":{"totalCount":1,"pageItems":[{"name":"local"}]}}`)
	}))
	defer server.Close()
	command := newAgentSpecListCommandForSession(newAgentSpecHubTestClient(t, server), "", "")
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"--output", "json"})
	if err := command.Execute(); err == nil || strings.Contains(output.String(), "pageItems") {
		t.Fatalf("partial success emitted: %s %v", output.String(), err)
	}
}

func TestAgentSpecGetSameNameSelectionAndExplicitIdentity(t *testing.T) {
	for _, selection := range []string{"choose-local", "choose-hub", "explicit", "non-interactive", "cancel", "invalid-choice", "wrong-version", "wrong-label", "wrong-name"} {
		t.Run(selection, func(t *testing.T) {
			var downloadedNamespace, downloadedVersion, downloadedLabel, downloadedName string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if r.Header.Get("Authorization") != "Bearer session-token" {
					t.Error("lost auth session")
				}
				if r.URL.Path == "/nacos/v3/admin/ai/agentspecs/list" {
					if q.Get("namespaceId") == agentspec.SharedNamespaceID {
						agentSpecHubListFixture(w)
					} else {
						fmt.Fprint(w, `{"code":0,"data":{"totalCount":1,"pagesAvailable":1,"pageItems":[{"name":"same","onlineCnt":1,"scope":"PRIVATE","owner":"local-author","labels":{"latest":"v3"}}]}}`)
					}
					return
				}
				if r.URL.Path != "/nacos/v3/client/ai/agentspecs" {
					t.Errorf("unexpected endpoint %s", r.URL)
				}
				downloadedNamespace, downloadedVersion, downloadedLabel, downloadedName = q.Get("namespaceId"), q.Get("version"), q.Get("label"), q.Get("name")
				fmt.Fprint(w, `{"code":0,"data":{"content":"{\"worker\":{\"suggested_name\":\"same\"}}","resource":{"a":{"name":"AGENTS.md","content":"# Original"}}}}`)
			}))
			defer server.Close()
			outputDir := t.TempDir()
			args := []string{"same", "-o", outputDir}
			var choose func(string) (string, error)
			switch selection {
			case "choose-local":
				choose = func(string) (string, error) { return "1", nil }
				args = append(args, "--label", "stable")
			case "choose-hub":
				choose = func(string) (string, error) { return "2", nil }
			case "explicit":
				args = append(args, "749", "v2")
			case "non-interactive":
				args = append(args, "--non-interactive")
			case "cancel":
				choose = func(string) (string, error) { return "", nil }
			case "invalid-choice":
				choose = func(string) (string, error) { return "9", nil }
			case "wrong-version":
				args = append(args, "749", "--version", "v1")
			case "wrong-label":
				args = append(args, "749", "--label", "stable")
			case "wrong-name":
				args[0] = "different"
				args = append(args, "749")
			}
			c := newAgentSpecHubTestClient(t, server)
			command := newAgentSpecGetCommandForSession(c, "", "", choose)
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetErr(&bytes.Buffer{})
			command.SetArgs(args)
			command.SilenceErrors, command.SilenceUsage = true, true
			err := command.Execute()
			wantDownload := selection == "choose-local" || selection == "choose-hub" || selection == "explicit"
			if (err == nil) != (wantDownload || selection == "cancel") {
				t.Fatalf("unexpected error %v", err)
			}
			if !wantDownload {
				entries, _ := os.ReadDir(outputDir)
				if downloadedName != "" || len(entries) != 0 {
					t.Fatal("invalid/ambiguous/cancelled selection downloaded content")
				}
				return
			}
			if selection == "choose-local" {
				if downloadedNamespace != "business" || downloadedName != "same" || downloadedLabel != "stable" {
					t.Fatal("ordinary label/identity changed")
				}
			} else if downloadedNamespace != agentspec.SharedNamespaceID || downloadedName != "749" || downloadedVersion != "v2" || downloadedLabel != "" {
				t.Fatal("Hub request did not pin selected version")
			}
			if c.Namespace != "business" {
				t.Fatal("download polluted business namespace")
			}
			if _, err := os.Stat(filepath.Join(outputDir, "same", "manifest.json")); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "Location: "+filepath.Join(outputDir, "same")) {
				t.Fatal("wrong reported package directory")
			}
			if selection == "choose-hub" && (!strings.Contains(output.String(), "Team B") || !strings.Contains(output.String(), "local-author")) {
				t.Fatal("ambiguity options lost source/author")
			}
		})
	}
}

func TestAgentSpecOrdinaryBatchRetainsClientOnlyVersionLabelAndSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/nacos/v3/client/ai/agentspecs" || q.Get("namespaceId") != "business" || q.Get("version") != "v1" || q.Get("label") != "stable" {
			t.Errorf("native get contract changed: %s", r.URL)
		}
		if q.Get("name") == "missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, `{"code":0,"data":{"content":"{}"}}`)
	}))
	defer server.Close()
	command := newAgentSpecGetCommandForSession(newAgentSpecHubTestClient(t, server), "", "", nil)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"missing", "same", "--no-hub", "--version", "v1", "--label", "stable", "-o", t.TempDir()})
	command.SilenceErrors, command.SilenceUsage = true, true
	if err := command.Execute(); err == nil {
		t.Fatal("partial failure must return an error")
	}
	for _, expected := range []string{"[1/2] Fetching agent spec: missing", "[2/2] Fetching agent spec: same", "Total: 2 | Success: 1 | Failed: 1", "Failed agent specs: missing"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing %q in %s", expected, output.String())
		}
	}
}

func TestAgentSpecProfileSourceSharedWithSkillAndExplicitIDOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.conf")
	if err := (&config.Config{Source: "namespace"}).SaveConfig(path); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("namespaceId") != agentspec.SharedNamespaceID {
			t.Error("explicit ID did not override namespace profile")
		}
		if strings.HasSuffix(r.URL.Path, "/list") {
			agentSpecHubListFixture(w)
			return
		}
		fmt.Fprint(w, `{"code":0,"data":{"content":"{}"}}`)
	}))
	defer server.Close()
	command := newAgentSpecGetCommandForSession(newAgentSpecHubTestClient(t, server), "team", path, nil)
	command.SetOut(&bytes.Buffer{})
	command.SetArgs([]string{"same", "749", "-o", t.TempDir()})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"agentSpecSource", "agentspec-source"} {
		if isProfileFieldKey(key) {
			t.Fatalf("introduced separate profile source %s", key)
		}
	}
	for _, args := range [][]string{{"same", "749", "v2", "extra"}, {"same", "749", "v2"}} {
		_, _, _, err := parseAgentSpecGetSelection(args, "v1")
		if err == nil {
			t.Fatalf("invalid positional version accepted: %v", args)
		}
	}
}

func TestAgentSpecListColorsAndRedirectedOutput(t *testing.T) {
	for _, format := range []string{"pretty", "json"} {
		for _, noColor := range []string{"", "1"} {
			t.Run(format+noColor, func(t *testing.T) {
				t.Setenv("NO_COLOR", noColor)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { agentSpecHubListFixture(w) }))
				defer server.Close()
				command := newAgentSpecListCommandForSession(newAgentSpecHubTestClient(t, server), "", "")
				var output bytes.Buffer
				command.SetOut(&output)
				command.SetArgs([]string{"--hub", "--output", format})
				if err := command.Execute(); err != nil {
					t.Fatal(err)
				}
				wantColor := format == "pretty" && noColor == ""
				if strings.Contains(output.String(), "\033[") != wantColor {
					t.Fatalf("wrong color output %q", output.String())
				}
			})
		}
	}
}
