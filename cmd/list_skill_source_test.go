package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/client"
	"github.com/nacos-group/nacos-cli/internal/config"
	"github.com/nacos-group/nacos-cli/internal/skill"
)

func TestSkillListSourcePriorityAndRequests(t *testing.T) {
	for _, preference := range []string{"", "namespace", "hub", "all"} {
		for _, flag := range []string{"", "--hub", "--no-hub"} {
			t.Run(preference+"/"+flag, func(t *testing.T) {
				resetRootConfigForTest(t)
				setTestHome(t, t.TempDir())
				path, err := config.GetProfileConfigPath("team")
				if err != nil {
					t.Fatal(err)
				}
				cfg := &config.Config{Source: preference}
				if err := cfg.SaveConfig(path); err != nil {
					t.Fatal(err)
				}
				if err := config.SetCurrentProfile("team"); err != nil {
					t.Fatal(err)
				}
				want, _ := config.NormalizeSkillSource(preference)
				if flag == "--hub" {
					want = "hub"
				}
				if flag == "--no-hub" {
					want = "namespace"
				}
				var namespaces []string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					q := r.URL.Query()
					if r.URL.Path != "/nacos/v3/admin/ai/skills/list" || q.Get("pageNo") != "2" || q.Get("pageSize") != "3" || q.Get("skillName") != "same" {
						t.Errorf("unexpected request %s", r.URL)
					}
					namespaces = append(namespaces, q.Get("namespaceId"))
					if q.Get("namespaceId") == skill.SharedNamespaceID {
						fmt.Fprint(w, `{"code":0,"data":{"totalCount":5,"pageNumber":2,"pagesAvailable":2,"pageItems":[{"name":"17","hubEntryId":"17","displayName":"same","version":"v2","enable":true}]}}`)
					} else {
						fmt.Fprint(w, `{"code":0,"data":{"totalCount":7,"pageNumber":2,"pagesAvailable":3,"pageItems":[{"name":"same","editingVersion":"draft","reviewingVersion":"review","enable":true}]}}`)
					}
				}))
				defer server.Close()
				// Connection flags must not suppress the current profile's source preference.
				serverAddr = strings.TrimPrefix(server.URL, "http://")
				namespace, authType, scheme = "team-ns", client.AuthTypeNone, "http"
				command := newSkillListCommand()
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
				var expected []string
				if want != "hub" {
					expected = append(expected, "team-ns")
				}
				if want != "namespace" {
					expected = append(expected, skill.SharedNamespaceID)
				}
				if !reflect.DeepEqual(namespaces, expected) {
					t.Fatalf("requests %v, want %v", namespaces, expected)
				}
				if want == "all" {
					var pages skill.SkillSourcePages
					if err := json.Unmarshal(output.Bytes(), &pages); err != nil {
						t.Fatal(err)
					}
					if pages.Namespace == nil || pages.Hub == nil || pages.Namespace.TotalCount != 7 || pages.Hub.TotalCount != 5 || pages.Namespace.PageItems[0].EditingVersion != "draft" || pages.Hub.PageItems[0].HubEntryID != "17" {
						t.Fatalf("lost independent pages, identities or governance metadata: %s", output.String())
					}
				} else {
					var page struct {
						PageItems []skill.SkillListItem `json:"pageItems"`
						PageNo    int                   `json:"pageNo"`
					}
					if err := json.Unmarshal(output.Bytes(), &page); err != nil {
						t.Fatal(err)
					}
					if len(page.PageItems) != 1 || page.PageNo != 2 {
						t.Fatalf("invalid single-source page: %s", output.String())
					}
				}
			})
		}
	}
}

func TestSkillListRejectsConflictingFlagsBeforeProfileSetup(t *testing.T) {
	for _, flags := range [][]string{{"--hub", "--no-hub"}, {"--no-hub", "--hub"}, {"--hub=false", "--no-hub"}} {
		command := newSkillListCommand()
		command.SetArgs(flags)
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "hub") {
			t.Fatalf("expected flag conflict for %v, got %v", flags, err)
		}
	}
}

func TestSkillListMissingPreferenceAndExplicitProfile(t *testing.T) {
	resetRootConfigForTest(t)
	setTestHome(t, t.TempDir())
	command := newSkillListCommand()
	if source, err := resolveSkillListSource(command, false, false); err != nil || source != "all" {
		t.Fatalf("default: %s %v", source, err)
	}
	path, _ := config.GetProfileConfigPath("selected")
	if err := (&config.Config{Source: "hub"}).SaveConfig(path); err != nil {
		t.Fatal(err)
	}
	profileName = "selected"
	if source, err := resolveSkillListSource(command, false, false); err != nil || source != "hub" {
		t.Fatalf("explicit profile: %s %v", source, err)
	}
	configFile = path
	profileName = "another"
	if source, err := resolveSkillListSource(command, false, false); err != nil || source != "hub" {
		t.Fatalf("explicit config: %s %v", source, err)
	}
}

func TestSkillListInvalidProfileModeAndFlagOverride(t *testing.T) {
	resetRootConfigForTest(t)
	configFile = t.TempDir() + "/invalid.conf"
	if err := os.WriteFile(configFile, []byte("source: invalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command := newSkillListCommand()
	if _, err := resolveSkillListSource(command, false, false); err == nil {
		t.Fatal("invalid profile mode must be reported")
	}
	if source, err := resolveSkillListSource(command, true, false); err != nil || source != "hub" {
		t.Fatalf("flag must override profile: %s %v", source, err)
	}
}

func TestSkillListReusesExistingCommandAndProfileKeys(t *testing.T) {
	for _, command := range rootCmd.Commands() {
		if command.Name() == "skill-search" {
			t.Fatal("source selection must not introduce skill-search")
		}
	}
	for _, key := range []string{"source"} {
		if !isProfileFieldKey(key) {
			t.Fatalf("profile key not recognized: %s", key)
		}
	}
	for _, key := range []string{"skill-source", "skillSource", "skill_source", "agentspecSource"} {
		if isProfileFieldKey(key) {
			t.Fatalf("legacy key is still exposed: %s", key)
		}
	}
}

func TestSkillListDoesNotReturnPartialResultsOnHubFailure(t *testing.T) {
	resetRootConfigForTest(t)
	setTestHome(t, t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("namespaceId") == skill.SharedNamespaceID {
			http.Error(w, "denied", http.StatusForbidden)
			return
		}
		fmt.Fprint(w, `{"code":0,"data":{"totalCount":1,"pageNumber":1,"pagesAvailable":1,"pageItems":[{"name":"local"}]}}`)
	}))
	defer server.Close()
	serverAddr, authType, scheme = strings.TrimPrefix(server.URL, "http://"), client.AuthTypeNone, "http"
	command := newSkillListCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"--output", "json"})
	if err := command.Execute(); err == nil {
		t.Fatal("Hub failure must propagate")
	}
	if strings.Contains(output.String(), "pageItems") {
		t.Fatalf("partial data emitted: %s", output.String())
	}
}

func TestSkillListSessionKeepsClientNamespaceAndProfile(t *testing.T) {
	resetRootConfigForTest(t)
	setTestHome(t, t.TempDir())
	path, _ := config.GetProfileConfigPath("interactive-team")
	if err := (&config.Config{Source: "namespace"}).SaveConfig(path); err != nil {
		t.Fatal(err)
	}
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Query().Get("namespaceId"))
		fmt.Fprint(w, `{"code":0,"data":{"totalCount":0,"pageNumber":1,"pagesAvailable":0,"pageItems":[]}}`)
	}))
	defer server.Close()
	sessionClient, err := client.NewNacosClient(strings.TrimPrefix(server.URL, "http://"), "before-switch", client.AuthTypeNone, "", "", "", "", "", "", "", "http")
	if err != nil {
		t.Fatal(err)
	}
	// Global flags belong to startup; ns switches must affect the bound session instead.
	namespace, profileName = "stale-global", "unrelated-profile"
	sessionClient.Namespace = "after-switch"
	for _, flags := range [][]string{nil, {"--hub"}, {"--no-hub"}} {
		command := newSkillListCommandForSession(sessionClient, "interactive-team", path)
		command.SetOut(&bytes.Buffer{})
		command.SetArgs(flags)
		if err := command.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{"after-switch", skill.SharedNamespaceID, "after-switch"}; !reflect.DeepEqual(requested, want) {
		t.Fatalf("session queries %v, want %v", requested, want)
	}
}

func TestSkillListLoadsLegacySourceWithoutChangingOverridePriority(t *testing.T) {
	resetRootConfigForTest(t)
	configFile = t.TempDir() + "/legacy.conf"
	if err := os.WriteFile(configFile, []byte("skillSource: hub\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command := newSkillListCommand()
	if source, err := resolveSkillListSource(command, false, false); err != nil || source != "hub" {
		t.Fatalf("legacy source=%s err=%v", source, err)
	}
	if source, err := resolveSkillListSource(command, false, true); err != nil || source != "namespace" {
		t.Fatalf("override source=%s err=%v", source, err)
	}
}
