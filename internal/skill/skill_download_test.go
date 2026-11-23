package skill

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/client"
)

func TestFindSkillDownloadCandidatesChecksLaterPagesAndExactNames(t *testing.T) {
	var pages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("search") != "accurate" || q.Get("skillName") != "demo" {
			t.Errorf("bad exact-name query: %s", r.URL)
		}
		pages = append(pages, q.Get("pageNo"))
		if q.Get("pageNo") == "1" {
			fmt.Fprint(w, `{"code":0,"data":{"totalCount":101,"pageNumber":1,"pagesAvailable":2,"pageItems":[{"name":"17","hubEntryId":"17","displayName":"demo","version":"v2"},{"name":"99","hubEntryId":"99","displayName":"demo-extra","version":"v2"}]}}`)
		} else {
			fmt.Fprint(w, `{"code":0,"data":{"totalCount":101,"pageNumber":2,"pagesAvailable":2,"pageItems":[{"name":"18","hubEntryId":"18","displayName":"demo","version":"v2"}]}}`)
		}
	}))
	defer server.Close()
	c, err := client.NewNacosClient(strings.TrimPrefix(server.URL, "http://"), "team", client.AuthTypeNone, "", "", "", "", "", "", "", "http")
	if err != nil {
		t.Fatal(err)
	}
	items, err := NewSkillService(c).FindSkillDownloadCandidates("demo", "hub")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "17" || items[1].Name != "18" || strings.Join(pages, ",") != "1,2" {
		t.Fatalf("incomplete/incorrect candidates: %+v pages=%v", items, pages)
	}
}

func TestSkillDownloadUsesTargetNamespaceForSpasAndPreservesSession(t *testing.T) {
	var downloads int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mac := hmac.New(sha1.New, []byte("test-secret"))
		_, _ = mac.Write([]byte(SharedNamespaceID + "+DEFAULT_GROUP+" + r.Header.Get("timeStamp")))
		want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		if r.Header.Get("Spas-Signature") != want {
			t.Errorf("request did not sign target Namespace: %s", r.URL)
		}
		if r.URL.Path == "/nacos/v3/admin/ai/skills/list" {
			fmt.Fprint(w, `{"code":0,"data":{"totalCount":1,"pageNumber":1,"pagesAvailable":1,"pageItems":[{"name":"17","hubEntryId":"17","displayName":"demo","version":"v2"}]}}`)
			return
		}
		downloads++
		if r.URL.Query().Get("version") != "v2" {
			t.Errorf("version not pinned: %s", r.URL)
		}
		http.Error(w, "revoked", http.StatusNotFound)
	}))
	defer server.Close()
	c, err := client.NewNacosClient(strings.TrimPrefix(server.URL, "http://"), "business", client.AuthTypeAliyun, "", "", "test-key", "test-secret", "", "", "", "http")
	if err != nil {
		t.Fatal(err)
	}
	service := NewSkillService(c)
	items, err := service.FindSkillDownloadCandidates("demo", "hub")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.FetchSkillCandidate(items[0], "", "")
	if err != nil || result == nil || !result.Deleted || downloads != 1 {
		t.Fatalf("stale share not reported: %+v %v", result, err)
	}
	if c.Namespace != "business" {
		t.Fatal("session Namespace was modified")
	}
}
