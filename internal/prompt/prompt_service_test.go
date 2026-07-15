package prompt

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nacos-group/nacos-cli/internal/client"
)

func strPtr(s string) *string { return &s }

func newTestNacosClient(serverURL string) (*client.NacosClient, error) {
	return client.NewNacosClient(
		strings.TrimPrefix(serverURL, "http://"),
		"test-ns",
		client.AuthTypeNone,
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"http",
	)
}

func TestListPrompts(t *testing.T) {
	var listCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/admin/ai/prompt/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		listCalled = true
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if got := r.URL.Query().Get("namespaceId"); got != "test-ns" {
			t.Fatalf("namespaceId = %s, want test-ns", got)
		}
		if got := r.URL.Query().Get("pageNo"); got != "1" {
			t.Fatalf("pageNo = %s, want 1", got)
		}
		if got := r.URL.Query().Get("pageSize"); got != "20" {
			t.Fatalf("pageSize = %s, want 20", got)
		}

		resp := V3Response{Code: 0, Message: "success"}
		listData := PromptListResponse{
			TotalCount:     1,
			PageNumber:     1,
			PagesAvailable: 1,
			PageItems: []PromptListItem{
				{PromptKey: "test-prompt", Description: strPtr("A test prompt")},
			},
		}
		data, _ := json.Marshal(listData)
		resp.Data = data
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	items, total, err := NewPromptService(nacosClient).ListPrompts("", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !listCalled {
		t.Fatal("list was not called")
	}
	if total != 1 {
		t.Fatalf("totalCount = %d, want 1", total)
	}
	if len(items) != 1 || items[0].PromptKey != "test-prompt" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestListPromptsWithFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("promptKey"); got != "my-prompt" {
			t.Fatalf("promptKey = %s, want my-prompt", got)
		}
		if got := r.URL.Query().Get("search"); got != "blur" {
			t.Fatalf("search = %s, want blur", got)
		}
		resp := V3Response{Code: 0, Message: "success"}
		listData := PromptListResponse{TotalCount: 0, PageItems: []PromptListItem{}}
		data, _ := json.Marshal(listData)
		resp.Data = data
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = NewPromptService(nacosClient).ListPrompts("my-prompt", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDescribePrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/admin/ai/prompt/governance" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if got := r.URL.Query().Get("promptKey"); got != "test-prompt" {
			t.Fatalf("promptKey = %s, want test-prompt", got)
		}

		detail := PromptDetail{
			PromptListItem: PromptListItem{
				PromptKey:      "test-prompt",
				Description:    strPtr("desc"),
				EditingVersion: "0.0.1",
			},
			VersionDetails: []PromptVersionSummary{
				{Version: "0.0.1", Status: "draft"},
			},
		}
		resp := V3Response{Code: 0, Message: "success"}
		data, _ := json.Marshal(detail)
		resp.Data = data
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	d, err := NewPromptService(nacosClient).DescribePrompt("test-prompt")
	if err != nil {
		t.Fatal(err)
	}
	if d.EditingVersion != "0.0.1" {
		t.Fatalf("editingVersion = %s, want 0.0.1", d.EditingVersion)
	}
	if len(d.VersionDetails) != 1 || d.VersionDetails[0].Status != "draft" {
		t.Fatalf("unexpected versions: %+v", d.Versions)
	}
}

func TestGetPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/client/ai/prompt" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("promptKey"); got != "test-prompt" {
			t.Fatalf("promptKey = %s, want test-prompt", got)
		}
		if got := r.URL.Query().Get("version"); got != "1.0.0" {
			t.Fatalf("version = %s, want 1.0.0", got)
		}

		p := ClientPrompt{
			PromptKey: "test-prompt",
			Version:   "1.0.0",
			Template:  "Hello {{name}}!",
		}
		resp := V3Response{Code: 0, Message: "success"}
		data, _ := json.Marshal(p)
		resp.Data = data
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	p, err := NewPromptService(nacosClient).GetPrompt("test-prompt", "1.0.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Template != "Hello {{name}}!" {
		t.Fatalf("template = %s, want Hello {{name}}!", p.Template)
	}
	if p.Version != "1.0.0" {
		t.Fatalf("version = %s, want 1.0.0", p.Version)
	}
}

func TestDraftCreatesNewPrompt(t *testing.T) {
	var createCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nacos/v3/admin/ai/prompt/governance":
			// Prompt doesn't exist yet -> return error
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 404, "message": "not found",
			})
		case "/nacos/v3/admin/ai/prompt/draft":
			if r.Method != http.MethodPost {
				t.Fatalf("draft method = %s, want POST", r.Method)
			}
			createCalled = true
			body, _ := io.ReadAll(r.Body)
			params := string(body)
			if !strings.Contains(params, "promptKey=test-prompt") {
				t.Fatalf("missing promptKey in body: %s", params)
			}
			if !strings.Contains(params, "template=") {
				t.Fatalf("missing template in body: %s", params)
			}
			resp := V3Response{Code: 0, Message: "success"}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	err = NewPromptService(nacosClient).Draft("test-prompt", "Hello!", "", "init", "A test", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !createCalled {
		t.Fatal("createDraft was not called")
	}
}

func TestDraftUpdatesExistingDraft(t *testing.T) {
	var updateCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nacos/v3/admin/ai/prompt/governance":
			// Prompt exists with editing version
			detail := PromptDetail{
				PromptListItem: PromptListItem{
					PromptKey:      "test-prompt",
					EditingVersion: "0.0.1",
				},
			}
			resp := V3Response{Code: 0, Message: "success"}
			data, _ := json.Marshal(detail)
			resp.Data = data
			_ = json.NewEncoder(w).Encode(resp)
		case "/nacos/v3/admin/ai/prompt/draft":
			if r.Method != http.MethodPut {
				t.Fatalf("draft method = %s, want PUT", r.Method)
			}
			updateCalled = true
			resp := V3Response{Code: 0, Message: "success"}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	err = NewPromptService(nacosClient).Draft("test-prompt", "Updated!", "", "update", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !updateCalled {
		t.Fatal("updateDraft was not called")
	}
}

func TestSubmitPrompt(t *testing.T) {
	var submitCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/admin/ai/prompt/submit" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		submitCalled = true
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		params := string(body)
		if !strings.Contains(params, "promptKey=test-prompt") {
			t.Fatalf("missing promptKey: %s", params)
		}
		if !strings.Contains(params, "namespaceId=test-ns") {
			t.Fatalf("missing namespaceId: %s", params)
		}
		resp := V3Response{Code: 0, Message: "success"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	err = NewPromptService(nacosClient).Submit("test-prompt", "")
	if err != nil {
		t.Fatal(err)
	}
	if !submitCalled {
		t.Fatal("submit was not called")
	}
}

func TestPublishPrompt(t *testing.T) {
	var publishCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/admin/ai/prompt/publish" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		publishCalled = true
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		params := string(body)
		if !strings.Contains(params, "promptKey=test-prompt") {
			t.Fatalf("missing promptKey: %s", params)
		}
		if !strings.Contains(params, "version=1.0.0") {
			t.Fatalf("missing version: %s", params)
		}
		resp := V3Response{Code: 0, Message: "success"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	err = NewPromptService(nacosClient).Publish("test-prompt", "1.0.0", true)
	if err != nil {
		t.Fatal(err)
	}
	if !publishCalled {
		t.Fatal("publish was not called")
	}
}

func TestPublishPromptNoUpdateLatest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		params := string(body)
		if !strings.Contains(params, "updateLatestLabel=false") {
			t.Fatalf("expected updateLatestLabel=false in body: %s", params)
		}
		resp := V3Response{Code: 0, Message: "success"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	err = NewPromptService(nacosClient).Publish("test-prompt", "1.0.0", false)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetPromptWithLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/client/ai/prompt" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("label"); got != "stable" {
			t.Fatalf("label = %s, want stable", got)
		}
		// version should not be set when label is specified
		if got := r.URL.Query().Get("version"); got != "" {
			t.Fatalf("version should be empty, got %s", got)
		}

		p := ClientPrompt{
			PromptKey: "test-prompt",
			Version:   "2.0.0",
			Template:  "Stable template",
		}
		resp := V3Response{Code: 0, Message: "success"}
		data, _ := json.Marshal(p)
		resp.Data = data
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	p, err := NewPromptService(nacosClient).GetPrompt("test-prompt", "", "stable")
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != "2.0.0" {
		t.Fatalf("version = %s, want 2.0.0", p.Version)
	}
	if p.Template != "Stable template" {
		t.Fatalf("template = %s, want Stable template", p.Template)
	}
}

func TestListPromptsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := V3Response{Code: 0, Message: "success"}
		listData := PromptListResponse{TotalCount: 0, PageItems: []PromptListItem{}}
		data, _ := json.Marshal(listData)
		resp.Data = data
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	items, total, err := NewPromptService(nacosClient).ListPrompts("", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatalf("totalCount = %d, want 0", total)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestDraftWithVariablesAndBizTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nacos/v3/admin/ai/prompt/governance":
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 404, "message": "not found",
			})
		case "/nacos/v3/admin/ai/prompt/draft":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			body, _ := io.ReadAll(r.Body)
			params := string(body)
			if !strings.Contains(params, "variables=") {
				t.Fatalf("missing variables in body: %s", params)
			}
			if !strings.Contains(params, "bizTags=test") {
				t.Fatalf("missing bizTags in body: %s", params)
			}
			if !strings.Contains(params, "description=Test+desc") {
				t.Fatalf("missing description in body: %s", params)
			}
			resp := V3Response{Code: 0, Message: "success"}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	vars := `[{"name":"domain","defaultValue":"coding"}]`
	err = NewPromptService(nacosClient).Draft("new-prompt", "Hello {{domain}}", vars, "init", "Test desc", "test", "", "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestSubmitPromptWithVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nacos/v3/admin/ai/prompt/submit" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		params := string(body)
		if !strings.Contains(params, "version=0.0.2") {
			t.Fatalf("missing version=0.0.2 in body: %s", params)
		}
		resp := V3Response{Code: 0, Message: "success"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	err = NewPromptService(nacosClient).Submit("test-prompt", "0.0.2")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDescribePromptNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 404, "message": "prompt not found",
		})
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewPromptService(nacosClient).DescribePrompt("nonexistent")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestGetPromptVersionPriorityOverLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// When both version and label are provided, version should take priority
		if got := r.URL.Query().Get("version"); got != "1.0.0" {
			t.Fatalf("version = %s, want 1.0.0", got)
		}
		// label should still be sent (server decides priority)
		p := ClientPrompt{
			PromptKey: "test-prompt",
			Version:   "1.0.0",
			Template:  "Versioned template",
		}
		resp := V3Response{Code: 0, Message: "success"}
		data, _ := json.Marshal(p)
		resp.Data = data
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	p, err := NewPromptService(nacosClient).GetPrompt("test-prompt", "1.0.0", "latest")
	if err != nil {
		t.Fatal(err)
	}
	if p.Template != "Versioned template" {
		t.Fatalf("template = %s, want Versioned template", p.Template)
	}
}

func TestDraftHTTPErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nacos/v3/admin/ai/prompt/governance":
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 404, "message": "not found",
			})
		case "/nacos/v3/admin/ai/prompt/draft":
			// Simulate server error
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 409, "message": "draft already exists",
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	err = NewPromptService(nacosClient).Draft("test-prompt", "Hello!", "", "init", "", "", "", "")
	if err == nil {
		t.Fatal("expected error for 409, got nil")
	}
	if !strings.Contains(err.Error(), "409") && !strings.Contains(err.Error(), "draft already exists") {
		t.Fatalf("error should mention 409 or conflict: %v", err)
	}
}

func TestDraftWithTargetVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nacos/v3/admin/ai/prompt/governance":
			// Prompt doesn't exist → triggers create path
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 404, "message": "not found",
			})
		case "/nacos/v3/admin/ai/prompt/draft":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			body, _ := io.ReadAll(r.Body)
			params := string(body)
			if !strings.Contains(params, "targetVersion=1.0.0") {
				t.Fatalf("missing targetVersion=1.0.0 in body: %s", params)
			}
			if !strings.Contains(params, "promptKey=versioned-prompt") {
				t.Fatalf("missing promptKey in body: %s", params)
			}
			resp := V3Response{Code: 0, Message: "success"}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	err = NewPromptService(nacosClient).Draft("versioned-prompt", "Hello!", "", "init", "", "", "1.0.0", "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDraftWithBasedOnVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nacos/v3/admin/ai/prompt/governance":
			// Prompt exists but no editing version (all versions are online/published)
			detail := PromptDetail{
				PromptListItem: PromptListItem{
					PromptKey:      "fork-prompt",
					EditingVersion: "", // no editing
				},
			}
			resp := V3Response{Code: 0, Message: "success"}
			data, _ := json.Marshal(detail)
			resp.Data = data
			_ = json.NewEncoder(w).Encode(resp)
		case "/nacos/v3/admin/ai/prompt/draft":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			body, _ := io.ReadAll(r.Body)
			params := string(body)
			if !strings.Contains(params, "basedOnVersion=1.0.0") {
				t.Fatalf("missing basedOnVersion=1.0.0 in body: %s", params)
			}
			if !strings.Contains(params, "promptKey=fork-prompt") {
				t.Fatalf("missing promptKey in body: %s", params)
			}
			resp := V3Response{Code: 0, Message: "success"}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	err = NewPromptService(nacosClient).Draft("fork-prompt", "Forked!", "", "fork from v1", "", "", "", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDraftBasedOnVersionConflictWithEditing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prompt exists WITH an editing version
		detail := PromptDetail{
			PromptListItem: PromptListItem{
				PromptKey:      "conflict-prompt",
				EditingVersion: "0.0.2",
			},
		}
		resp := V3Response{Code: 0, Message: "success"}
		data, _ := json.Marshal(detail)
		resp.Data = data
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	nacosClient, err := newTestNacosClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	// Should return error because editing draft exists and basedOnVersion is set
	err = NewPromptService(nacosClient).Draft("conflict-prompt", "New!", "", "fork", "", "", "", "1.0.0")
	if err == nil {
		t.Fatal("expected error when basedOnVersion conflicts with existing editing draft")
	}
	if !strings.Contains(err.Error(), "cannot fork") {
		t.Fatalf("error should mention 'cannot fork': %v", err)
	}
	if !strings.Contains(err.Error(), "0.0.2") {
		t.Fatalf("error should mention editing version '0.0.2': %v", err)
	}
}
