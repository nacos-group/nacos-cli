package skill

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/client"
)

func TestSkillSourceRejectsInvalidOptionsBeforeRequests(t *testing.T) {
	service := NewSkillService(&client.NacosClient{})
	for _, test := range []struct {
		page, size int
		source     string
	}{
		{0, 20, "all"}, {1, 0, "hub"}, {1, 101, "namespace"}, {1 << 30, 20, "all"}, {1, 20, "invalid"},
	} {
		if _, err := service.ListSkillsBySource("", test.page, test.size, test.source); err == nil {
			t.Fatalf("invalid options accepted: %+v", test)
		}
	}
}

func TestSkillSourceEmptyHubKeepsNamespaceAndClientIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("namespaceId") == SharedNamespaceID {
			fmt.Fprint(w, `{"code":0,"data":{"totalCount":0,"pageNumber":1,"pagesAvailable":0,"pageItems":[]}}`)
			return
		}
		fmt.Fprint(w, `{"code":0,"data":{"totalCount":1,"pageNumber":1,"pagesAvailable":1,"pageItems":[{"name":"demo","labels":{"latest":"v1"},"editingVersion":"v2"}]}}`)
	}))
	defer server.Close()
	nacosClient, err := client.NewNacosClient(strings.TrimPrefix(server.URL, "http://"), "public", client.AuthTypeNone, "", "", "", "", "", "", "", "http")
	if err != nil {
		t.Fatal(err)
	}
	service := NewSkillService(nacosClient)
	pages, err := service.ListSkillsBySource("", 1, 20, "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages.Namespace.PageItems) != 1 || len(pages.Hub.PageItems) != 0 || pages.Hub.TotalCount != 0 || nacosClient.Namespace != "public" {
		t.Fatalf("bad empty Hub result or mutated client: %+v", pages)
	}
	items, total, err := service.ListSkills("", 1, 20)
	if err != nil || total != 1 || len(items) != 1 || items[0].Labels["latest"] != "v1" || items[0].EditingVersion != "v2" {
		t.Fatalf("existing ListSkills behavior changed: %+v %d %v", items, total, err)
	}
}
