package agentspec

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/nacos-group/nacos-cli/internal/client"
)

// AgentSpecService handles agentspec-related operations
type AgentSpecService struct {
	client *client.NacosClient
}

// AgentSpecListItem represents an agentspec item in the admin list (AgentSpecSummary in Nacos).
type AgentSpecListItem struct {
	NamespaceId      string            `json:"namespaceId,omitempty"`
	Name             string            `json:"name"`
	Description      *string           `json:"description"`
	Enable           bool              `json:"enable"`
	Labels           map[string]string `json:"labels"`
	Scope            *string           `json:"scope,omitempty"`
	BizTags          *string           `json:"bizTags,omitempty"`
	EditingVersion   *string           `json:"editingVersion"`
	ReviewingVersion *string           `json:"reviewingVersion"`
	OnlineCnt        int               `json:"onlineCnt"`
	DownloadCount    *int64            `json:"downloadCount,omitempty"`
	UpdateTime       int64             `json:"updateTime"`
}

// AgentSpecListResponse represents the response from agentspec list API
type AgentSpecListResponse struct {
	TotalCount     int                 `json:"totalCount"`
	PageNumber     int                 `json:"pageNumber"`
	PagesAvailable int                 `json:"pagesAvailable"`
	PageItems      []AgentSpecListItem `json:"pageItems"`
}

// AgentSpec represents a complete agentspec
type AgentSpec struct {
	NamespaceId string                        `json:"namespaceId"`
	Name        string                        `json:"name"`
	Description string                        `json:"description"`
	BizTags     string                        `json:"bizTags,omitempty"`
	Content     string                        `json:"content"` // manifest.json string
	Resource    map[string]*AgentSpecResource `json:"resource,omitempty"`
}

// AgentSpecResource represents a single resource in an agentspec
type AgentSpecResource struct {
	Name     string                 `json:"name"`
	Type     string                 `json:"type"`
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// V3Response represents the v3 API response wrapper
type V3Response struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// NewAgentSpecService creates a new agentspec service
func NewAgentSpecService(nacosClient *client.NacosClient) *AgentSpecService {
	return &AgentSpecService{
		client: nacosClient,
	}
}

// ListAgentSpecs lists all agentspecs with admin API
func (s *AgentSpecService) ListAgentSpecs(agentSpecName string, search string, pageNo, pageSize int) ([]AgentSpecListItem, int, error) {
	params := url.Values{}
	params.Set("pageNo", fmt.Sprintf("%d", pageNo))
	params.Set("pageSize", fmt.Sprintf("%d", pageSize))
	params.Set("namespaceId", s.client.Namespace)

	if agentSpecName != "" {
		params.Set("agentSpecName", agentSpecName)
		// Server requires 'search' param for name filtering to take effect
		if search == "" {
			search = "accurate"
		}
	}
	if search != "" {
		params.Set("search", search)
	}

	listURL := fmt.Sprintf("http://%s/nacos/v3/admin/ai/agentspecs/list?%s",
		s.client.ServerAddr, params.Encode())

	req, err := http.NewRequest("GET", listURL, nil)
	if err != nil {
		return nil, 0, err
	}

	if s.client.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.client.AccessToken))
	}

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("list agentspecs failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, 0, client.ParseHTTPError(resp.StatusCode, respBody, "list agentspecs")
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("read response failed: %w", err)
	}

	var v3Resp V3Response
	if err := json.Unmarshal(respBody, &v3Resp); err != nil {
		return nil, 0, fmt.Errorf("parse response failed: %w", err)
	}

	if v3Resp.Code != 0 {
		return nil, 0, fmt.Errorf("list agentspecs failed: code=%d, message=%s", v3Resp.Code, v3Resp.Message)
	}

	var listResp AgentSpecListResponse
	if err := json.Unmarshal(v3Resp.Data, &listResp); err != nil {
		return nil, 0, fmt.Errorf("parse agentspec list failed: %w", err)
	}

	return listResp.PageItems, listResp.TotalCount, nil
}

// buildResourceRelativePath matches Nacos AgentSpecOperationServiceImpl.buildResourcePath:
// if type is blank, use name only; if name already starts with "type/", use name; else "type/name".
func buildResourceRelativePath(res *AgentSpecResource) string {
	if res == nil {
		return ""
	}
	t := strings.TrimSpace(res.Type)
	n := strings.TrimSpace(res.Name)
	if t == "" {
		return n
	}
	prefix := t + "/"
	if strings.HasPrefix(n, prefix) {
		return n
	}
	return t + "/" + n
}

// GetAgentSpec retrieves an agentspec via the Client API and saves it to local directory.
// Priority for version resolution: label > version > latest.
func (s *AgentSpecService) GetAgentSpec(name, outputDir string, version, label string) error {
	params := url.Values{}
	params.Set("namespaceId", s.client.Namespace)
	params.Set("name", name)
	if version != "" {
		params.Set("version", version)
	}
	if label != "" {
		params.Set("label", label)
	}

	apiURL := fmt.Sprintf("http://%s/nacos/v3/client/ai/agentspecs?%s",
		s.client.ServerAddr, params.Encode())

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	if s.client.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.client.AccessToken))
	}

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get agentspec: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return client.ParseHTTPError(resp.StatusCode, respBody, "get agentspec")
	}

	var v3Resp V3Response
	if err := json.Unmarshal(respBody, &v3Resp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	if v3Resp.Code != 0 {
		return fmt.Errorf("get agentspec failed: code=%d, message=%s", v3Resp.Code, v3Resp.Message)
	}

	var spec AgentSpec
	if err := json.Unmarshal(v3Resp.Data, &spec); err != nil {
		return fmt.Errorf("failed to parse agentspec: %w", err)
	}

	// Create output directory
	specDir := filepath.Join(outputDir, name)
	if err := os.MkdirAll(specDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Save resource files (paths aligned with server buildResourcePath)
	for _, res := range spec.Resource {
		if res == nil || res.Content == "" {
			continue
		}
		rel := buildResourceRelativePath(res)
		if rel == "" {
			continue
		}
		filePath := filepath.Join(specDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return fmt.Errorf("failed to create resource directory: %w", err)
		}
		// Decode base64 content for binary files
		var data []byte
		if enc, ok := res.Metadata["encoding"]; ok && enc == "base64" {
			decoded, err := base64.StdEncoding.DecodeString(res.Content)
			if err != nil {
				return fmt.Errorf("failed to decode base64 resource %s: %w", res.Name, err)
			}
			data = decoded
		} else {
			data = []byte(res.Content)
		}
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return fmt.Errorf("failed to write resource file %s: %w", res.Name, err)
		}
	}

	// Generate manifest.json
	return s.generateManifest(specDir, spec.Content)
}

// generateManifest creates manifest.json file with pretty-printed JSON
func (s *AgentSpecService) generateManifest(specDir string, content string) error {
	// Try to pretty-print the JSON content
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(content), &raw); err == nil {
		var buf bytes.Buffer
		if err := json.Indent(&buf, raw, "", "  "); err == nil {
			content = buf.String()
		}
	}

	manifestPath := filepath.Join(specDir, "manifest.json")
	return os.WriteFile(manifestPath, []byte(content), 0644)
}

// UploadAgentSpec uploads an agentspec from local directory or a pre-built zip file.
// If agentSpecPath points to a .zip file it is uploaded directly; otherwise the
// directory is packed into a zip on-the-fly.
func (s *AgentSpecService) UploadAgentSpec(agentSpecPath string) error {
	var zipBuffer *bytes.Buffer
	var specName string

	if strings.HasSuffix(strings.ToLower(agentSpecPath), ".zip") {
		// Direct zip upload
		data, err := os.ReadFile(agentSpecPath)
		if err != nil {
			return fmt.Errorf("failed to read zip file: %w", err)
		}
		zipBuffer = bytes.NewBuffer(data)
		base := filepath.Base(agentSpecPath)
		specName = strings.TrimSuffix(base, filepath.Ext(base))
	} else {
		// Pack directory into zip
		specName = filepath.Base(agentSpecPath)
		zipBuffer = new(bytes.Buffer)
		zipWriter := zip.NewWriter(zipBuffer)

		err := filepath.Walk(agentSpecPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			relPath, err := filepath.Rel(agentSpecPath, path)
			if err != nil {
				return err
			}
			zipPath := filepath.Join(specName, relPath)
			writer, err := zipWriter.Create(zipPath)
			if err != nil {
				return err
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(writer, file)
			return err
		})
		if err != nil {
			return fmt.Errorf("failed to create ZIP: %w", err)
		}
		if err := zipWriter.Close(); err != nil {
			return err
		}
	}

	// Upload ZIP via multipart form
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", fmt.Sprintf("%s.zip", specName))
	if err != nil {
		return err
	}

	if _, err := io.Copy(part, zipBuffer); err != nil {
		return err
	}

	if err := writer.Close(); err != nil {
		return err
	}

	// Send HTTP request
	uploadParams := url.Values{}
	uploadParams.Set("namespaceId", s.client.Namespace)
	uploadParams.Set("overwrite", "false")
	uploadURL := fmt.Sprintf("http://%s/nacos/v3/admin/ai/agentspecs/upload?%s",
		s.client.ServerAddr, uploadParams.Encode())
	req, err := http.NewRequest("POST", uploadURL, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	if s.client.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.client.AccessToken))
	}

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return client.ParseHTTPError(resp.StatusCode, respBody, "upload agentspec")
	}

	return nil
}

// ParseManifest parses manifest.json and returns the agentspec name (worker.suggested_name)
func (s *AgentSpecService) ParseManifest(manifestPath string) (string, error) {
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", err
	}

	var manifest map[string]interface{}
	if err := json.Unmarshal(content, &manifest); err != nil {
		return "", fmt.Errorf("failed to parse manifest.json: %w", err)
	}

	worker, ok := manifest["worker"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid manifest.json: missing 'worker' object")
	}

	name, ok := worker["suggested_name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("invalid manifest.json: missing 'worker.suggested_name'")
	}

	return name, nil
}
