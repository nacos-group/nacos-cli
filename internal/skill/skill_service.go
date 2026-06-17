package skill

import (
	"archive/zip"
	"bytes"
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
	"gopkg.in/yaml.v3"
)

// SkillService handles skill-related operations
type SkillService struct {
	client *client.NacosClient
}

// SkillInfo represents skill metadata
type SkillInfo struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
}

// SkillListItem represents a skill item in the list, mirroring the admin
// SkillSummary payload returned by GET /v3/admin/ai/skills/list.
type SkillListItem struct {
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Owner            string            `json:"owner,omitempty"`
	Enable           bool              `json:"enable"`
	Scope            string            `json:"scope,omitempty"`
	BizTags          string            `json:"bizTags,omitempty"`
	From             string            `json:"from,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	EditingVersion   string            `json:"editingVersion,omitempty"`
	ReviewingVersion string            `json:"reviewingVersion,omitempty"`
	OnlineCnt        *int              `json:"onlineCnt,omitempty"`
	DownloadCount    *int64            `json:"downloadCount,omitempty"`
	UpdateTime       *int64            `json:"updateTime,omitempty"`
}

// SkillVersionSummary mirrors com.alibaba.nacos.api.ai.model.skills.SkillMeta.SkillVersionSummary.
type SkillVersionSummary struct {
	Version             string `json:"version"`
	Status              string `json:"status"`
	Author              string `json:"author,omitempty"`
	CommitMsg           string `json:"commitMsg,omitempty"`
	CreateTime          *int64 `json:"createTime,omitempty"`
	UpdateTime          *int64 `json:"updateTime,omitempty"`
	PublishPipelineInfo string `json:"publishPipelineInfo,omitempty"`
	DownloadCount       *int64 `json:"downloadCount,omitempty"`
}

// SkillDetail mirrors the admin SkillMeta payload (SkillSummary + versions).
type SkillDetail struct {
	SkillListItem
	Versions []SkillVersionSummary `json:"versions,omitempty"`
}

// NewSkillService creates a new skill service
func NewSkillService(nacosClient *client.NacosClient) *SkillService {
	return &SkillService{
		client: nacosClient,
	}
}

// Client returns the underlying NacosClient.
func (s *SkillService) Client() *client.NacosClient {
	return s.client
}

// SkillListResponse represents the response from skill list API
type SkillListResponse struct {
	TotalCount     int             `json:"totalCount"`
	PageNumber     int             `json:"pageNumber"`
	PagesAvailable int             `json:"pagesAvailable"`
	PageItems      []SkillListItem `json:"pageItems"`
}

// V3Response represents the v3 API response wrapper
type V3Response struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// ListSkills lists all skills with name and description
func (s *SkillService) ListSkills(skillName string, pageNo, pageSize int) ([]SkillListItem, int, error) {
	if err := s.client.EnsureTokenValid(); err != nil {
		return nil, 0, err
	}
	params := url.Values{}
	params.Set("pageNo", fmt.Sprintf("%d", pageNo))
	params.Set("pageSize", fmt.Sprintf("%d", pageSize))
	params.Set("namespaceId", s.client.Namespace)

	if skillName != "" {
		params.Set("skillName", skillName)
	}

	listURL := fmt.Sprintf("%s/nacos/v3/admin/ai/skills/list?%s",
		s.client.BaseURL(), params.Encode())

	req, err := s.client.NewAuthedRequest("GET", listURL, nil)
	if err != nil {
		return nil, 0, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("list skills failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, 0, client.ParseHTTPError(resp.StatusCode, respBody, "list skills")
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
		return nil, 0, fmt.Errorf("list skills failed: code=%d, message=%s", v3Resp.Code, v3Resp.Message)
	}

	var skillList SkillListResponse
	if err := json.Unmarshal(v3Resp.Data, &skillList); err != nil {
		return nil, 0, fmt.Errorf("parse skill list failed: %w", err)
	}

	return skillList.PageItems, skillList.TotalCount, nil
}

// DescribeSkill fetches the admin SkillMeta detail (governance + versions)
// via GET /v3/admin/ai/skills?skillName=X&namespaceId=Y.
func (s *SkillService) DescribeSkill(skillName string) (*SkillDetail, error) {
	if err := s.client.EnsureTokenValid(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(skillName) == "" {
		return nil, fmt.Errorf("skillName is required")
	}

	params := url.Values{}
	params.Set("namespaceId", s.client.Namespace)
	params.Set("skillName", skillName)

	describeURL := fmt.Sprintf("%s/nacos/v3/admin/ai/skills?%s",
		s.client.BaseURL(), params.Encode())

	req, err := s.client.NewAuthedRequest("GET", describeURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("describe skill failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, client.ParseHTTPError(resp.StatusCode, respBody, "describe skill")
	}

	var v3Resp V3Response
	if err := json.Unmarshal(respBody, &v3Resp); err != nil {
		return nil, fmt.Errorf("parse response failed: %w", err)
	}
	if v3Resp.Code != 0 {
		return nil, fmt.Errorf("describe skill failed: code=%d, message=%s", v3Resp.Code, v3Resp.Message)
	}

	var detail SkillDetail
	if err := json.Unmarshal(v3Resp.Data, &detail); err != nil {
		return nil, fmt.Errorf("parse skill detail failed: %w", err)
	}
	return &detail, nil
}

// UpdateSkillScope sets the skill visibility scope (PUBLIC or PRIVATE).
func (s *SkillService) UpdateSkillScope(skillName, scope string) error {
	if err := s.client.EnsureTokenValid(); err != nil {
		return err
	}
	if strings.TrimSpace(skillName) == "" {
		return fmt.Errorf("skillName is required")
	}
	normalizedScope, err := normalizeSkillScope(scope)
	if err != nil {
		return err
	}

	params := url.Values{}
	params.Set("namespaceId", s.client.Namespace)
	params.Set("skillName", skillName)
	params.Set("scope", normalizedScope)

	scopeURL := fmt.Sprintf("%s/nacos/v3/admin/ai/skills/scope?%s",
		s.client.BaseURL(), params.Encode())
	req, err := s.client.NewAuthedRequest("PUT", scopeURL, nil)
	if err != nil {
		return err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("update skill scope failed: %w", err)
	}
	defer resp.Body.Close()

	return checkV3Response(resp, "update skill scope")
}

// UpdateSkillBizTags sets skill metadata tags.
func (s *SkillService) UpdateSkillBizTags(skillName, bizTags string) error {
	if err := s.client.EnsureTokenValid(); err != nil {
		return err
	}
	if strings.TrimSpace(skillName) == "" {
		return fmt.Errorf("skillName is required")
	}
	bizTags = strings.TrimSpace(bizTags)
	if bizTags == "" {
		return fmt.Errorf("bizTags are required")
	}

	params := url.Values{}
	params.Set("namespaceId", s.client.Namespace)
	params.Set("skillName", skillName)
	params.Set("bizTags", bizTags)

	bizTagsURL := fmt.Sprintf("%s/nacos/v3/admin/ai/skills/biz-tags?%s",
		s.client.BaseURL(), params.Encode())
	req, err := s.client.NewAuthedRequest("PUT", bizTagsURL, nil)
	if err != nil {
		return err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("update skill bizTags failed: %w", err)
	}
	defer resp.Body.Close()

	return checkV3Response(resp, "update skill bizTags")
}

func normalizeSkillScope(scope string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(scope))
	switch normalized {
	case "PUBLIC", "PRIVATE":
		return normalized, nil
	default:
		return "", fmt.Errorf("scope must be PUBLIC or PRIVATE")
	}
}

func checkV3Response(resp *http.Response, operation string) error {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s response failed: %w", operation, err)
	}
	if resp.StatusCode != 200 {
		return client.ParseHTTPError(resp.StatusCode, respBody, operation)
	}

	var v3Resp V3Response
	if err := json.Unmarshal(respBody, &v3Resp); err != nil {
		return fmt.Errorf("parse %s response failed: %w", operation, err)
	}
	if v3Resp.Code != 0 {
		return fmt.Errorf("%s failed: code=%d, message=%s", operation, v3Resp.Code, v3Resp.Message)
	}
	return nil
}

// GetSkill downloads a skill as ZIP via the Client Skill API and extracts it to local directory.
// The server returns a ZIP binary stream containing skillName/SKILL.md and resource files.
// Priority for version resolution: label > version > latest.
func (s *SkillService) GetSkill(skillName, outputDir string, version, label string) error {
	if err := s.client.EnsureTokenValid(); err != nil {
		return err
	}
	params := url.Values{}
	params.Set("namespaceId", s.client.Namespace)
	params.Set("name", skillName)
	if version != "" {
		params.Set("version", version)
	}
	if label != "" {
		params.Set("label", label)
	}

	apiURL := fmt.Sprintf("%s/nacos/v3/client/ai/skills?%s",
		s.client.BaseURL(), params.Encode())

	req, err := s.client.NewAuthedRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get skill: %w", err)
	}
	defer resp.Body.Close()

	zipBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return client.ParseHTTPError(resp.StatusCode, zipBytes, "get skill")
	}

	// Extract ZIP to output directory
	return extractZip(zipBytes, outputDir)
}

// ExtractSkillZip extracts a ZIP byte array to the target directory.
// ZIP entries like "skillName/SKILL.md" are extracted preserving their path structure.
func ExtractSkillZip(zipBytes []byte, targetDir string) error {
	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return fmt.Errorf("failed to read zip: %w", err)
	}

	for _, f := range zipReader.File {
		// Security: reject path traversal
		if strings.Contains(f.Name, "..") {
			return fmt.Errorf("unsafe zip entry path: %s", f.Name)
		}

		destPath := filepath.Join(targetDir, f.Name)

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", destPath, err)
			}
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open zip entry %s: %w", f.Name, err)
		}

		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("failed to read zip entry %s: %w", f.Name, err)
		}

		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", destPath, err)
		}
	}

	return nil
}

// extractZip preserves the existing private helper name for older call sites in this package.
func extractZip(zipBytes []byte, targetDir string) error {
	return ExtractSkillZip(zipBytes, targetDir)
}

// UploadSkill uploads a skill draft from local directory or a pre-built zip file.
// If skillPath points to a .zip file it is uploaded directly; otherwise the
// directory is packed into a zip on-the-fly with files at the archive root.
func (s *SkillService) UploadSkill(skillPath string, overwrite bool) error {
	if err := s.client.EnsureTokenValid(); err != nil {
		return err
	}
	var zipBuffer *bytes.Buffer
	var skillName string

	if strings.HasSuffix(strings.ToLower(skillPath), ".zip") {
		// Direct zip upload
		data, err := os.ReadFile(skillPath)
		if err != nil {
			return fmt.Errorf("failed to read zip file: %w", err)
		}
		zipBuffer = bytes.NewBuffer(data)
		// Use the zip filename (without .zip) as the display name
		base := filepath.Base(skillPath)
		skillName = strings.TrimSuffix(base, filepath.Ext(base))
	} else {
		// Pack directory into zip
		skillName = filepath.Base(skillPath)
		walkRoot, err := resolveDirectoryRoot(skillPath)
		if err != nil {
			return fmt.Errorf("failed to resolve skill directory: %w", err)
		}
		zipBuffer = new(bytes.Buffer)
		zipWriter := zip.NewWriter(zipBuffer)

		err = filepath.Walk(walkRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			relPath, err := filepath.Rel(walkRoot, path)
			if err != nil {
				return err
			}
			// Use forward slashes for ZIP paths (required by ZIP spec) and
			// do NOT prefix with skillName — server expects files at the root
			// of the archive (e.g. SKILL.md, not skillName/SKILL.md).
			zipPath := filepath.ToSlash(relPath)
			writer, err := zipWriter.Create(zipPath)
			if err != nil {
				return err
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(writer, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
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

	part, err := writer.CreateFormFile("file", fmt.Sprintf("%s.zip", skillName))
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
	uploadURL := fmt.Sprintf("%s/nacos/v3/admin/ai/skills/upload?namespaceId=%s&overwrite=%t",
		s.client.BaseURL(), s.client.Namespace, overwrite)
	req, err := s.client.NewAuthedRequest("POST", uploadURL, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return client.ParseHTTPError(resp.StatusCode, respBody, "upload skill")
	}

	return nil
}

// PublishSkill publishes an approved (reviewing) skill version to make it online.
// By default, updates the `latest` route label to the published version.
func (s *SkillService) PublishSkill(skillName, version string, updateLatestLabel bool) error {
	if err := s.client.EnsureTokenValid(); err != nil {
		return err
	}

	if strings.TrimSpace(skillName) == "" {
		return fmt.Errorf("skillName is required")
	}
	if strings.TrimSpace(version) == "" {
		return fmt.Errorf("version is required")
	}

	params := url.Values{}
	params.Set("namespaceId", s.client.Namespace)
	params.Set("skillName", skillName)
	params.Set("version", version)
	if updateLatestLabel {
		params.Set("updateLatestLabel", "true")
	} else {
		params.Set("updateLatestLabel", "false")
	}

	publishURL := fmt.Sprintf("%s/nacos/v3/admin/ai/skills/publish?%s",
		s.client.BaseURL(), params.Encode())
	req, err := s.client.NewAuthedRequest("POST", publishURL, nil)
	if err != nil {
		return err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("publish failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read publish response failed: %w", err)
	}

	if resp.StatusCode != 200 {
		return client.ParseHTTPError(resp.StatusCode, respBody, "publish skill")
	}

	var v3Resp V3Response
	if err := json.Unmarshal(respBody, &v3Resp); err != nil {
		return fmt.Errorf("parse publish response failed: %w", err)
	}
	if v3Resp.Code != 0 {
		return fmt.Errorf("publish skill failed: code=%d, message=%s", v3Resp.Code, v3Resp.Message)
	}

	return nil
}

// SubmitSkill submits a draft skill version for review.
func (s *SkillService) SubmitSkill(skillName, version string) error {
	if err := s.client.EnsureTokenValid(); err != nil {
		return err
	}

	params := url.Values{}
	params.Set("namespaceId", s.client.Namespace)
	params.Set("skillName", skillName)
	if version != "" {
		params.Set("version", version)
	}

	submitURL := fmt.Sprintf("%s/nacos/v3/admin/ai/skills/submit?%s",
		s.client.BaseURL(), params.Encode())
	req, err := s.client.NewAuthedRequest("POST", submitURL, nil)
	if err != nil {
		return err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("submit failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read submit response failed: %w", err)
	}

	if resp.StatusCode != 200 {
		return client.ParseHTTPError(resp.StatusCode, respBody, "submit skill")
	}

	var v3Resp V3Response
	if err := json.Unmarshal(respBody, &v3Resp); err != nil {
		return fmt.Errorf("parse submit response failed: %w", err)
	}
	if v3Resp.Code != 0 {
		return fmt.Errorf("submit skill failed: code=%d, message=%s", v3Resp.Code, v3Resp.Message)
	}

	return nil
}

// SkillQueryResult holds the result of a conditional skill query.
type SkillQueryResult struct {
	Md5             string // content fingerprint from X-Nacos-Skill-Md5 header
	ResolvedVersion string // resolved version from X-Nacos-Skill-Resolved-Version header
	Updated         bool   // true if new content was downloaded
	Deleted         bool   // true if the skill was not found (404)
	ZipBytes        []byte // ZIP payload when Updated=true
}

// FetchSkill performs a conditional skill download using the MD5 fingerprint.
// If the server content matches the provided md5, HTTP 304 is returned and no ZIP is downloaded.
// The returned ZIP payload is not extracted; callers decide when and where to apply it.
func (s *SkillService) FetchSkill(skillName, version, label, md5 string) (*SkillQueryResult, error) {
	if err := s.client.EnsureTokenValid(); err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("namespaceId", s.client.Namespace)
	params.Set("name", skillName)
	if version != "" {
		params.Set("version", version)
	}
	if label != "" {
		params.Set("label", label)
	}
	if md5 != "" {
		params.Set("md5", md5)
	}

	apiURL := fmt.Sprintf("%s/nacos/v3/client/ai/skills?%s",
		s.client.BaseURL(), params.Encode())

	req, err := s.client.NewAuthedRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query skill: %w", err)
	}
	defer resp.Body.Close()

	// HTTP 304 Not Modified - content unchanged.
	// Still read the resolved version header so callers can detect label
	// switches even when server claims md5 is unchanged.
	if resp.StatusCode == http.StatusNotModified {
		responseMd5 := resp.Header.Get("X-Nacos-Skill-Md5")
		if responseMd5 == "" {
			responseMd5 = md5
		}
		return &SkillQueryResult{
			Md5:             responseMd5,
			ResolvedVersion: resp.Header.Get("X-Nacos-Skill-Resolved-Version"),
			Updated:         false,
		}, nil
	}

	// HTTP 404 Not Found - skill deleted
	if resp.StatusCode == http.StatusNotFound {
		return &SkillQueryResult{Deleted: true}, nil
	}

	zipBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, client.ParseHTTPError(resp.StatusCode, zipBytes, "query skill")
	}

	// Extract response headers
	newMd5 := resp.Header.Get("X-Nacos-Skill-Md5")
	resolvedVersion := resp.Header.Get("X-Nacos-Skill-Resolved-Version")

	return &SkillQueryResult{
		Md5:             newMd5,
		ResolvedVersion: resolvedVersion,
		Updated:         true,
		ZipBytes:        zipBytes,
	}, nil
}

// QuerySkill performs a conditional skill download and extracts the ZIP to outputDir on 200.
func (s *SkillService) QuerySkill(skillName, outputDir, version, label, md5 string) (*SkillQueryResult, error) {
	result, err := s.FetchSkill(skillName, version, label, md5)
	if err != nil {
		return nil, err
	}
	if result.Updated {
		if err := extractZip(result.ZipBytes, outputDir); err != nil {
			return nil, fmt.Errorf("failed to extract skill: %w", err)
		}
	}
	return result, nil
}

// ParseSkillMD parses SKILL.md file
func (s *SkillService) ParseSkillMD(mdPath string) (*SkillInfo, error) {
	content, err := os.ReadFile(mdPath)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return nil, fmt.Errorf("invalid SKILL.md format")
	}

	// Find end of frontmatter
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			endIdx = i
			break
		}
	}

	if endIdx == -1 {
		return nil, fmt.Errorf("invalid SKILL.md format: no closing ---")
	}

	// Parse YAML frontmatter
	frontmatter := strings.Join(lines[1:endIdx], "\n")
	var skillInfo SkillInfo
	if err := yaml.Unmarshal([]byte(frontmatter), &skillInfo); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	return &skillInfo, nil
}
