package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/client"
	"github.com/nacos-group/nacos-cli/internal/config"
	"github.com/nacos-group/nacos-cli/internal/skill"
)

func TestInteractiveSkillListColorsKeepSourceModesAndJSON(t *testing.T) {
	for _, mode := range []string{"namespace", "hub", "all"} {
		for _, output := range []string{"pretty", "json"} {
			t.Run(mode+"/"+output, func(t *testing.T) {
				t.Setenv("NO_COLOR", "")
				path := filepath.Join(t.TempDir(), "profile.conf")
				if err := (&config.Config{Source: mode}).SaveConfig(path); err != nil {
					t.Fatal(err)
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer session-token" {
						t.Error("interactive list lost the authenticated session")
					}
					if r.URL.Path != "/nacos/v3/admin/ai/skills/list" {
						t.Errorf("wrong endpoint %s", r.URL.Path)
					}
					if r.URL.Query().Get("namespaceId") == skill.SharedNamespaceID {
						fmt.Fprint(w, `{"code":0,"data":{"totalCount":1,"pagesAvailable":1,"pageItems":[{"name":"2701","displayName":"same","hubEntryId":"2701","version":"0.0.1","enable":true}]}}`)
					} else {
						if r.URL.Query().Get("namespaceId") != "public" {
							t.Error("session namespace changed")
						}
						fmt.Fprint(w, `{"code":0,"data":{"totalCount":2,"pagesAvailable":1,"pageItems":[{"name":"same","scope":"PUBLIC","enable":true},{"name":"private-draft","scope":"PRIVATE","editingVersion":"0.0.2","onlineCount":0,"enable":true}]}}`)
					}
				}))
				defer server.Close()
				session, err := client.NewNacosClient(strings.TrimPrefix(server.URL, "http://"), "public", client.AuthTypeToken, "", "", "", "", "", "", "", "http", client.WithToken("session-token"))
				if err != nil {
					t.Fatal(err)
				}
				command := newSkillListCommandForSession(session, "team", path)
				var buf bytes.Buffer
				command.SetOut(&buf)
				command.SetArgs([]string{"--output", output})
				if err := command.Execute(); err != nil {
					t.Fatal(err)
				}
				text := buf.String()
				if session.Namespace != "public" {
					t.Fatal("list changed the business namespace")
				}
				if output == "json" {
					if strings.Contains(text, "\033[") || !json.Valid(buf.Bytes()) {
						t.Fatalf("invalid JSON output: %s", text)
					}
					if mode != "hub" && !strings.Contains(text, "private-draft") {
						t.Fatal("lost private draft from namespace results")
					}
					if mode != "namespace" && !strings.Contains(text, "2701") {
						t.Fatal("lost shared identity")
					}
				} else {
					if !strings.Contains(text, "\033[1;36mSkill List\033[0m") || !strings.Contains(text, "\033[32msame\033[0m") {
						t.Fatalf("interactive colors missing: %q", text)
					}
					if mode != "hub" && (!strings.Contains(text, "source=NAMESPACE") || !strings.Contains(text, "private-draft") || !strings.Contains(text, "scope=PRIVATE")) {
						t.Fatal("lost namespace governance or source")
					}
					if mode != "namespace" && !strings.Contains(text, "hubEntryId=2701  version=0.0.1") {
						t.Fatal("lost pinned shared identity")
					}
				}
			})
		}
	}
}

func TestSkillListColorPresentationPreservesPlainText(t *testing.T) {
	zero := 0
	items := []skill.SkillListItem{
		{Name: "2701", DisplayName: "same", DiscoverySource: "HUB", HubEntryID: "2701", Version: "0.0.1", Description: "Pinned description", Enable: true, Labels: map[string]string{"latest": "0.0.1", "stable": "0.0.1"}},
		{Name: "draft", DiscoverySource: "NAMESPACE", NamespaceID: "public", Scope: "PRIVATE", EditingVersion: "0.0.2", OnlineCnt: &zero},
	}
	var plain, colored bytes.Buffer
	renderSkillListPretty(&plain, items, 2, 1, 20, false)
	renderSkillListPretty(&colored, items, 2, 1, 20, true)
	if strings.Contains(plain.String(), "\033[") {
		t.Fatal("non-interactive output contains color escapes")
	}
	stripped := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(colored.String(), "")
	if stripped != plain.String() {
		t.Fatalf("colors changed list data: %q versus %q", stripped, plain.String())
	}
	if !strings.Contains(colored.String(), "\033[31mstatus=disabled\033[0m") {
		t.Fatal("disabled status should retain its red presentation")
	}
}

func TestInteractiveSkillListHonorsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	path := filepath.Join(t.TempDir(), "profile.conf")
	if err := (&config.Config{Source: "namespace"}).SaveConfig(path); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":0,"data":{"totalCount":1,"pagesAvailable":1,"pageItems":[{"name":"same","enable":true}]}}`)
	}))
	defer server.Close()
	session, err := client.NewNacosClient(strings.TrimPrefix(server.URL, "http://"), "public", client.AuthTypeNone, "", "", "", "", "", "", "", "http")
	if err != nil {
		t.Fatal(err)
	}
	command := newSkillListCommandForSession(session, "team", path)
	var buf bytes.Buffer
	command.SetOut(&buf)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\033[") || !strings.Contains(buf.String(), "same") {
		t.Fatalf("invalid NO_COLOR output: %q", buf.String())
	}
}

func TestDirectSkillListRedirectedOutputRemainsPlain(t *testing.T) {
	resetRootConfigForTest(t)
	t.Setenv("NO_COLOR", "")
	setTestHome(t, t.TempDir())
	configFile = filepath.Join(t.TempDir(), "profile.conf")
	if err := (&config.Config{Source: "namespace"}).SaveConfig(configFile); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":0,"data":{"totalCount":1,"pagesAvailable":1,"pageItems":[{"name":"same","enable":true}]}}`)
	}))
	defer server.Close()
	serverAddr = strings.TrimPrefix(server.URL, "http://")
	namespace, authType, scheme = "public", client.AuthTypeNone, "http"
	file, err := os.CreateTemp(t.TempDir(), "list-output-")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	command := newSkillListCommand()
	command.SetOut(file)
	command.SetArgs([]string{"--no-hub"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "\033[") || !strings.Contains(string(data), "same") {
		t.Fatalf("redirected output should remain readable plain text: %q", data)
	}
}
