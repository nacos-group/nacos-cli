package prompt

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/client"
)

// PromptService handles prompt-related operations
type PromptService struct {
	client *client.NacosClient
}

// NewPromptService creates a new prompt service
func NewPromptService(nacosClient *client.NacosClient) *PromptService {
	return &PromptService{
		client: nacosClient,
	}
}

// PromptListItem represents a prompt in the admin list.
type PromptListItem struct {
	Name             string            `json:"name"`
	PromptKey        string            `json:"promptKey"`
	Description      *string           `json:"description"`
	Owner            string            `json:"owner,omitempty"`
	Enable           bool              `json:"enable"`
	BizTags          []string          `json:"bizTags,omitempty"`
	BizTagsStr       *string           `json:"bizTagsStr,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	LatestVersion    string            `json:"latestVersion,omitempty"`
	EditingVersion   string            `json:"editingVersion,omitempty"`
	ReviewingVersion string            `json:"reviewingVersion,omitempty"`
	OnlineCnt        *int              `json:"onlineCnt,omitempty"`
	DownloadCount    *int64            `json:"downloadCount,omitempty"`
	GmtModified      *int64            `json:"gmtModified,omitempty"`
}

// PromptVersionSummary represents a version in the governance detail.
type PromptVersionSummary struct {
	PromptKey           string  `json:"promptKey,omitempty"`
	Version             string  `json:"version"`
	Status              string  `json:"status"`
	SrcUser             string  `json:"srcUser,omitempty"`
	CommitMsg           *string `json:"commitMsg,omitempty"`
	GmtModified         *int64  `json:"gmtModified,omitempty"`
	PublishPipelineInfo string  `json:"publishPipelineInfo,omitempty"`
	DownloadCount       *int64  `json:"downloadCount,omitempty"`
}

// PromptDetail represents the governance detail (meta + versions).
type PromptDetail struct {
	PromptListItem
	Versions       []string               `json:"versions,omitempty"`
	VersionDetails []PromptVersionSummary `json:"versionDetails,omitempty"`
}

// PromptVersionInfo represents a specific prompt version's content.
type PromptVersionInfo struct {
	PromptKey  string          `json:"promptKey"`
	Version    string          `json:"version"`
	Template   string          `json:"template"`
	Variables  json.RawMessage `json:"variables,omitempty"`
	Status     string          `json:"status,omitempty"`
	CommitMsg  string          `json:"commitMsg,omitempty"`
	CreateTime *int64          `json:"createTime,omitempty"`
	UpdateTime *int64          `json:"updateTime,omitempty"`
}

// ClientPrompt represents the client API response.
type ClientPrompt struct {
	PromptKey string          `json:"promptKey"`
	Version   string          `json:"version"`
	Template  string          `json:"template"`
	Variables json.RawMessage `json:"variables,omitempty"`
	Md5       string          `json:"md5,omitempty"`
}

// PromptListResponse represents the paginated list response.
type PromptListResponse struct {
	TotalCount     int              `json:"totalCount"`
	PageNumber     int              `json:"pageNumber"`
	PagesAvailable int              `json:"pagesAvailable"`
	PageItems      []PromptListItem `json:"pageItems"`
}

// V3Response represents the v3 API response wrapper.
type V3Response struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// ListPrompts lists prompts with pagination.
func (s *PromptService) ListPrompts(promptKey string, pageNo, pageSize int) ([]PromptListItem, int, error) {
	if err := s.client.EnsureTokenValid(); err != nil {
		return nil, 0, err
	}
	params := url.Values{}
	params.Set("pageNo", fmt.Sprintf("%d", pageNo))
	params.Set("pageSize", fmt.Sprintf("%d", pageSize))
	params.Set("namespaceId", s.client.Namespace)

	if promptKey != "" {
		params.Set("promptKey", promptKey)
		params.Set("search", "blur")
	}

	listURL := fmt.Sprintf("%s/nacos/v3/admin/ai/prompt/list?%s",
		s.client.BaseURL(), params.Encode())

	req, err := s.client.NewAuthedRequest("GET", listURL, nil)
	if err != nil {
		return nil, 0, err
	}

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		return nil, 0, client.ParseHTTPError(resp.StatusCode, body[:n], "list prompts")
	}

	var v3Resp V3Response
	if err := json.NewDecoder(resp.Body).Decode(&v3Resp); err != nil {
		return nil, 0, fmt.Errorf("decode response: %w", err)
	}
	if v3Resp.Code != 0 {
		return nil, 0, fmt.Errorf("server error: %s", v3Resp.Message)
	}

	var listResp PromptListResponse
	if err := json.Unmarshal(v3Resp.Data, &listResp); err != nil {
		return nil, 0, fmt.Errorf("decode data: %w", err)
	}
	return listResp.PageItems, listResp.TotalCount, nil
}

// DescribePrompt gets the governance detail of a prompt.
func (s *PromptService) DescribePrompt(promptKey string) (*PromptDetail, error) {
	if err := s.client.EnsureTokenValid(); err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("namespaceId", s.client.Namespace)
	params.Set("promptKey", promptKey)

	apiURL := fmt.Sprintf("%s/nacos/v3/admin/ai/prompt/governance?%s",
		s.client.BaseURL(), params.Encode())

	req, err := s.client.NewAuthedRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		return nil, client.ParseHTTPError(resp.StatusCode, body[:n], "describe prompt")
	}

	var v3Resp V3Response
	if err := json.NewDecoder(resp.Body).Decode(&v3Resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if v3Resp.Code != 0 {
		return nil, fmt.Errorf("server error: %s", v3Resp.Message)
	}

	var detail PromptDetail
	if err := json.Unmarshal(v3Resp.Data, &detail); err != nil {
		return nil, fmt.Errorf("decode data: %w", err)
	}
	return &detail, nil
}

// GetPrompt gets a prompt version via client API (version/label/latest).
func (s *PromptService) GetPrompt(promptKey, version, label string) (*ClientPrompt, error) {
	if err := s.client.EnsureTokenValid(); err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("namespaceId", s.client.Namespace)
	params.Set("promptKey", promptKey)
	if version != "" {
		params.Set("version", version)
	}
	if label != "" {
		params.Set("label", label)
	}

	apiURL := fmt.Sprintf("%s/nacos/v3/client/ai/prompt?%s",
		s.client.BaseURL(), params.Encode())

	req, err := s.client.NewAuthedRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		return nil, client.ParseHTTPError(resp.StatusCode, body[:n], "get prompt")
	}

	var v3Resp V3Response
	if err := json.NewDecoder(resp.Body).Decode(&v3Resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if v3Resp.Code != 0 {
		return nil, fmt.Errorf("server error: %s", v3Resp.Message)
	}

	var prompt ClientPrompt
	if err := json.Unmarshal(v3Resp.Data, &prompt); err != nil {
		return nil, fmt.Errorf("decode data: %w", err)
	}
	return &prompt, nil
}

// Draft creates or updates a prompt draft. It first checks if an editing version
// exists; if so, it updates; otherwise it creates a new draft.
// If basedOnVersion is specified but an editing draft already exists, it returns
// an error because the server would reject the fork with 409 CONFLICT.
func (s *PromptService) Draft(promptKey, template, variables, commitMsg, description, bizTags, targetVersion, basedOnVersion string) error {
	if err := s.client.EnsureTokenValid(); err != nil {
		return err
	}

	// Check if editing version exists
	hasEditing := false
	editingVer := ""
	detail, err := s.DescribePrompt(promptKey)
	if err == nil && detail != nil && detail.EditingVersion != "" {
		hasEditing = true
		editingVer = detail.EditingVersion
	}

	if hasEditing {
		if basedOnVersion != "" {
			return fmt.Errorf("cannot fork from version %s: an editing draft (%s) already exists for prompt '%s'. "+
				"Submit or discard the current draft first, then retry with --based-on-version",
				basedOnVersion, editingVer, promptKey)
		}
		return s.updateDraft(promptKey, template, variables, commitMsg)
	}
	return s.createDraft(promptKey, template, variables, commitMsg, description, bizTags, targetVersion, basedOnVersion)
}

func (s *PromptService) createDraft(promptKey, template, variables, commitMsg, description, bizTags, targetVersion, basedOnVersion string) error {
	params := url.Values{}
	params.Set("namespaceId", s.client.Namespace)
	params.Set("promptKey", promptKey)
	params.Set("template", template)
	if variables != "" {
		params.Set("variables", variables)
	}
	if commitMsg != "" {
		params.Set("commitMsg", commitMsg)
	}
	if description != "" {
		params.Set("description", description)
	}
	if bizTags != "" {
		params.Set("bizTags", bizTags)
	}
	if targetVersion != "" {
		params.Set("targetVersion", targetVersion)
	}
	if basedOnVersion != "" {
		params.Set("basedOnVersion", basedOnVersion)
	}

	apiURL := fmt.Sprintf("%s/nacos/v3/admin/ai/prompt/draft", s.client.BaseURL())

	req, err := s.client.NewAuthedRequest("POST", apiURL, strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		return client.ParseHTTPError(resp.StatusCode, body[:n], "create draft")
	}

	var v3Resp V3Response
	if err := json.NewDecoder(resp.Body).Decode(&v3Resp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if v3Resp.Code != 0 {
		return fmt.Errorf("server error: %s", v3Resp.Message)
	}
	return nil
}

func (s *PromptService) updateDraft(promptKey, template, variables, commitMsg string) error {
	params := url.Values{}
	params.Set("namespaceId", s.client.Namespace)
	params.Set("promptKey", promptKey)
	params.Set("template", template)
	if variables != "" {
		params.Set("variables", variables)
	}
	if commitMsg != "" {
		params.Set("commitMsg", commitMsg)
	}

	apiURL := fmt.Sprintf("%s/nacos/v3/admin/ai/prompt/draft", s.client.BaseURL())

	req, err := s.client.NewAuthedRequest("PUT", apiURL, strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		return client.ParseHTTPError(resp.StatusCode, body[:n], "update draft")
	}

	var v3Resp V3Response
	if err := json.NewDecoder(resp.Body).Decode(&v3Resp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if v3Resp.Code != 0 {
		return fmt.Errorf("server error: %s", v3Resp.Message)
	}
	return nil
}

// Submit submits a prompt draft for review (editing -> reviewing).
func (s *PromptService) Submit(promptKey, version string) error {
	if err := s.client.EnsureTokenValid(); err != nil {
		return err
	}
	params := url.Values{}
	params.Set("namespaceId", s.client.Namespace)
	params.Set("promptKey", promptKey)
	if version != "" {
		params.Set("version", version)
	}

	apiURL := fmt.Sprintf("%s/nacos/v3/admin/ai/prompt/submit", s.client.BaseURL())

	req, err := s.client.NewAuthedRequest("POST", apiURL, strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		return client.ParseHTTPError(resp.StatusCode, body[:n], "submit prompt")
	}

	var v3Resp V3Response
	if err := json.NewDecoder(resp.Body).Decode(&v3Resp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if v3Resp.Code != 0 {
		return fmt.Errorf("server error: %s", v3Resp.Message)
	}
	return nil
}

// Publish publishes an approved prompt version (reviewed -> online).
func (s *PromptService) Publish(promptKey, version string, updateLatestLabel bool) error {
	if err := s.client.EnsureTokenValid(); err != nil {
		return err
	}
	params := url.Values{}
	params.Set("namespaceId", s.client.Namespace)
	params.Set("promptKey", promptKey)
	params.Set("version", version)
	if !updateLatestLabel {
		params.Set("updateLatestLabel", "false")
	}

	apiURL := fmt.Sprintf("%s/nacos/v3/admin/ai/prompt/publish", s.client.BaseURL())

	req, err := s.client.NewAuthedRequest("POST", apiURL, strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		return client.ParseHTTPError(resp.StatusCode, body[:n], "publish prompt")
	}

	var v3Resp V3Response
	if err := json.NewDecoder(resp.Body).Decode(&v3Resp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if v3Resp.Code != 0 {
		return fmt.Errorf("server error: %s", v3Resp.Message)
	}
	return nil
}
