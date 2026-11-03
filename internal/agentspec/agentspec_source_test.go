package agentspec

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/client"
)

func agentSpecTestClient(t *testing.T, server *httptest.Server, namespace string) *client.NacosClient {
	t.Helper()
	c, err := client.NewNacosClient(strings.TrimPrefix(server.URL, "http://"), namespace,
		client.AuthTypeToken, "", "", "", "", "", "", "", "http", client.WithToken("session-token"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func hubAgentSpecItem(id, name, version string) AgentSpecListItem {
	item := AgentSpecListItem{Name: id, NamespaceId: SharedNamespaceID, DiscoverySource: "HUB",
		Hub: &AgentSpecHubInfo{HubEntryID: id, ResourceType: "agentspec", ResourceName: name,
			Version: version, SourceDisplayName: "Team B", Author: "author"}}
	item.applyHubMetadata()
	return item
}

func TestAgentSpecSourceIndependentPagesAndWireMetadata(t *testing.T) {
	for _, mode := range []string{"namespace", "hub", "all", ""} {
		t.Run(mode, func(t *testing.T) {
			var requested []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if r.URL.Path != "/nacos/v3/admin/ai/agentspecs/list" || q.Get("pageNo") != "2" || q.Get("pageSize") != "3" || q.Get("agentSpecName") != "same" || q.Get("search") != "accurate" {
					t.Errorf("unexpected list request %s", r.URL)
				}
				if r.Header.Get("Authorization") != "Bearer session-token" {
					t.Error("lost authenticated session")
				}
				requested = append(requested, q.Get("namespaceId"))
				if q.Get("namespaceId") == SharedNamespaceID {
					fmt.Fprint(w, `{"code":0,"data":{"totalCount":7,"pageNumber":2,"pagesAvailable":3,"pageItems":[{"name":"749","updateTime":999,"downloadCount":999,"hub":{"hubEntryId":"749","resourceType":"agentspec","resourceName":"same","version":"v2","sourceDisplayName":"Team B","author":"author","updateTime":123,"downloadCount":4}}]}}`)
				} else {
					fmt.Fprint(w, `{"code":0,"data":{"totalCount":5,"pageNumber":2,"pagesAvailable":2,"pageItems":[{"name":"same","scope":"PRIVATE","editingVersion":"v3","reviewingVersion":"v2","onlineCnt":1,"labels":{"latest":"v1"}}]}}`)
				}
			}))
			defer server.Close()
			c := agentSpecTestClient(t, server, "team")
			service := NewAgentSpecService(c)
			pages, err := service.ListAgentSpecsBySource("same", 2, 3, mode)
			if err != nil {
				t.Fatal(err)
			}
			var want []string
			if mode != "hub" {
				want = append(want, "team")
				if pages.Namespace.TotalCount != 5 || pages.Namespace.PageItems[0].EditingVersion == nil || *pages.Namespace.PageItems[0].Scope != "PRIVATE" {
					t.Fatalf("lost ordinary governance: %+v", pages.Namespace)
				}
			}
			if mode != "namespace" {
				want = append(want, SharedNamespaceID)
				item := pages.Hub.PageItems[0]
				if pages.Hub.TotalCount != 7 || item.Name != "749" || item.DisplayName != "same" || item.Version != "v2" || item.SourceDisplayName != "Team B" || item.Owner != "author" || item.UpdateTime != 123 || *item.DownloadCount != 4 || item.DiscoverySource != "HUB" {
					t.Fatalf("lost Hub identity/own metadata: %+v", item)
				}
			}
			if !reflect.DeepEqual(requested, want) || c.Namespace != "team" {
				t.Fatalf("requests=%v, want=%v; active namespace=%s", requested, want, c.Namespace)
			}
		})
	}
}

func TestAgentSpecSourceEmptyHubErrorsAndPagingBounds(t *testing.T) {
	failHub := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("namespaceId") == SharedNamespaceID && failHub {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"denied"}`)
			return
		}
		fmt.Fprint(w, `{"code":0,"data":{"totalCount":0,"pagesAvailable":0,"pageItems":null}}`)
	}))
	defer server.Close()
	c := agentSpecTestClient(t, server, "")
	originalNamespace := c.Namespace
	service := NewAgentSpecService(c)
	pages, err := service.ListAgentSpecsBySource("", 1, 20, "all")
	if err != nil || pages.Hub.PageItems == nil || pages.Namespace.PageItems == nil || c.Namespace != originalNamespace {
		t.Fatalf("empty Hub handling: %+v %v", pages, err)
	}
	failHub = true
	if result, err := service.ListAgentSpecsBySource("", 1, 20, "all"); err == nil || result != nil {
		t.Fatal("Hub permission failure must not silently yield a partial list")
	}
	for _, paging := range [][2]int{{0, 20}, {1, 0}, {1, 101}, {math.MaxInt32, 100}} {
		if _, err := service.ListAgentSpecsBySource("", paging[0], paging[1], "all"); err == nil {
			t.Fatalf("accepted invalid pagination %v", paging)
		}
	}
	if _, err := service.ListAgentSpecsBySource("", 1, 20, "invalid"); err == nil {
		t.Fatal("invalid source accepted")
	}
	c.Namespace = SharedNamespaceID
	if _, err := service.ListAgentSpecsBySource("", 1, 20, "hub"); err == nil {
		t.Fatal("manual Hub namespace accepted")
	}
	if _, err := service.FindAgentSpecDownloadCandidates("same", "hub"); err == nil {
		t.Fatal("manual Hub namespace accepted for download discovery")
	}
	item := AgentSpecListItem{UpdateTime: 999, Hub: &AgentSpecHubInfo{}}
	item.applyHubMetadata()
	if item.UpdateTime != 0 {
		t.Fatal("missing Hub date fell back to source date")
	}
}

func TestAgentSpecCandidatesIncludeLaterPageAndPrivateOnlineButNotDraftOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("agentSpecName") != "same" || q.Get("search") != "accurate" {
			t.Errorf("expected exact-name candidate search: %s", r.URL)
		}
		if q.Get("namespaceId") != SharedNamespaceID {
			fmt.Fprint(w, `{"code":0,"data":{"totalCount":3,"pagesAvailable":1,"pageItems":[{"name":"same","scope":"PRIVATE","onlineCnt":1},{"name":"same","editingVersion":"draft","onlineCnt":0},{"name":"different","onlineCnt":1}]}}`)
			return
		}
		id := "749"
		if q.Get("pageNo") == "2" {
			id = "750"
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "data": AgentSpecListResponse{
			TotalCount: 101, PagesAvailable: 2, PageItems: []AgentSpecListItem{hubAgentSpecItem(id, "same", "v2")},
		}})
	}))
	defer server.Close()
	c := agentSpecTestClient(t, server, "team")
	items, err := NewAgentSpecService(c).FindAgentSpecDownloadCandidates("same", "all")
	if err != nil || len(items) != 3 || items[0].DiscoverySource != "NAMESPACE" || items[1].HubEntryID != "749" || items[2].HubEntryID != "750" || c.Namespace != "team" {
		t.Fatalf("candidates=%+v err=%v", items, err)
	}
}

func TestAgentSpecHubDownloadPinsVersionPreservesPackageAndSession(t *testing.T) {
	for _, label := range []string{"", "latest"} {
		t.Run(label, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if r.URL.Path != "/nacos/v3/client/ai/agentspecs" || q.Get("namespaceId") != SharedNamespaceID || q.Get("name") != "749" || q.Get("version") != "v2" || q.Get("label") != "" {
					t.Errorf("wrong pinned request %s", r.URL)
				}
				if r.Header.Get("Authorization") != "Bearer session-token" {
					t.Error("download lost authenticated session")
				}
				fmt.Fprint(w, `{"code":0,"data":{"name":"same","content":"{\"worker\":{\"suggested_name\":\"same\"}}","resource":{"a":{"name":"AGENTS.md","content":"# Original"},"b":{"type":"assets","name":"data.bin","content":"AAEC/w==","metadata":{"encoding":"base64"}},"c":{"type":"config","name":"config/app.yaml","content":"key: value"}}}}`)
			}))
			defer server.Close()
			c := agentSpecTestClient(t, server, "team")
			output := t.TempDir()
			if err := NewAgentSpecService(c).GetAgentSpecCandidate(hubAgentSpecItem("749", "same", "v2"), output, "", label); err != nil {
				t.Fatal(err)
			}
			for name, expected := range map[string]string{"AGENTS.md": "# Original", "assets/data.bin": string([]byte{0, 1, 2, 255}), "config/app.yaml": "key: value"} {
				data, err := os.ReadFile(filepath.Join(output, "same", filepath.FromSlash(name)))
				if err != nil || string(data) != expected {
					t.Fatalf("package changed at %s: %q %v", name, data, err)
				}
			}
			manifest, err := os.ReadFile(filepath.Join(output, "same", "manifest.json"))
			if err != nil || !json.Valid(manifest) || !strings.Contains(string(manifest), "suggested_name") || c.Namespace != "team" {
				t.Fatalf("manifest/session changed: %s %v", manifest, err)
			}
			if _, err := os.Stat(filepath.Join(output, "749")); !os.IsNotExist(err) {
				t.Fatal("opaque Hub ID used as package directory")
			}
		})
	}
}

func TestAgentSpecHubRejectsWrongVersionLabelIdentityAndRevocation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"share revoked or version changed"}`)
	}))
	defer server.Close()
	service := NewAgentSpecService(agentSpecTestClient(t, server, "team"))
	item := hubAgentSpecItem("749", "same", "v2")
	for _, selection := range [][2]string{{"v1", ""}, {"", "stable"}, {"v1", "latest"}} {
		if err := service.GetAgentSpecCandidate(item, t.TempDir(), selection[0], selection[1]); err == nil {
			t.Fatalf("invalid fixed version/label accepted: %v", selection)
		}
	}
	invalid := item
	invalid.HubEntryID = "750"
	if err := service.GetAgentSpecCandidate(invalid, t.TempDir(), "", ""); err == nil || calls != 0 {
		t.Fatal("invalid Hub identity reached download API")
	}
	output := t.TempDir()
	if err := service.GetAgentSpecCandidate(item, output, "", ""); err == nil || !strings.Contains(err.Error(), "revoked") || calls != 1 {
		t.Fatalf("revocation not propagated: %v, calls=%d", err, calls)
	}
	entries, _ := os.ReadDir(output)
	if len(entries) != 0 {
		t.Fatal("failed download created local files")
	}
}

func TestAgentSpecHubSignsSelectedNamespaceForListAndGet(t *testing.T) {
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mac := hmac.New(sha1.New, []byte("test-secret"))
		_, _ = mac.Write([]byte(SharedNamespaceID + "+DEFAULT_GROUP+" + r.Header.Get("timeStamp")))
		if r.Header.Get("Spas-Signature") != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
			t.Errorf("wrong namespace signature for %s", r.URL)
		}
		requested = append(requested, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/list") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "data": AgentSpecListResponse{
				TotalCount: 1, PagesAvailable: 1, PageItems: []AgentSpecListItem{hubAgentSpecItem("749", "same", "v2")},
			}})
			return
		}
		fmt.Fprint(w, `{"code":0,"data":{"content":"{}"}}`)
	}))
	defer server.Close()
	c, err := client.NewNacosClient(strings.TrimPrefix(server.URL, "http://"), "business", client.AuthTypeAliyun,
		"", "", "test-key", "test-secret", "", "", "", "http")
	if err != nil {
		t.Fatal(err)
	}
	service := NewAgentSpecService(c)
	items, err := service.FindAgentSpecDownloadCandidates("same", "hub")
	if err != nil || len(items) != 1 {
		t.Fatalf("candidate search: %v %v", items, err)
	}
	if err := service.GetAgentSpecCandidate(items[0], t.TempDir(), "", ""); err != nil {
		t.Fatal(err)
	}
	if len(requested) != 2 || c.Namespace != "business" {
		t.Fatalf("changed session or requests: %v %s", requested, c.Namespace)
	}
}

func TestAgentSpecHubRejectsInvalidPackageBeforeWriting(t *testing.T) {
	for _, response := range []string{
		`{"code":0,"data":null}`,
		`{"code":0,"data":{"content":"{}","resource":{"a":{"name":"../outside.txt","content":"escape"}}}}`,
		`{"code":0,"data":{"content":"{}","resource":{"a":{"name":"..\\outside.txt","content":"escape"}}}}`,
	} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, response) }))
			defer server.Close()
			service := NewAgentSpecService(agentSpecTestClient(t, server, "team"))
			output := t.TempDir()
			if err := service.GetAgentSpecCandidate(hubAgentSpecItem("749", "same", "v2"), output, "", ""); err == nil {
				t.Fatal("invalid Hub content accepted")
			}
			entries, _ := os.ReadDir(output)
			if len(entries) != 0 {
				t.Fatal("invalid package created files")
			}
			if err := service.GetAgentSpecCandidate(hubAgentSpecItem("749", "..", "v2"), output, "", ""); err == nil {
				t.Fatal("unsafe package directory accepted")
			}
		})
	}
}

func TestAgentSpecHubPreservesNativeEmptyAndRawManifest(t *testing.T) {
	for _, content := range []string{"", "# Existing raw content"} {
		t.Run(content, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "data": AgentSpec{Content: content}})
			}))
			defer server.Close()
			output := t.TempDir()
			service := NewAgentSpecService(agentSpecTestClient(t, server, "team"))
			if err := service.GetAgentSpecCandidate(hubAgentSpecItem("749", "same", "v2"), output, "", ""); err != nil {
				t.Fatal(err)
			}
			manifest, err := os.ReadFile(filepath.Join(output, "same", "manifest.json"))
			if err != nil || string(manifest) != content {
				t.Fatalf("changed original manifest behavior: %q %v", manifest, err)
			}
		})
	}
}
