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

// SkillListItem represents a skill item in the list with name and description
type SkillListItem struct {
	Name        string
	Description string
}

// NewSkillService creates a new skill service
func NewSkillService(nacosClient *client.NacosClient) *SkillService {
	return &SkillService{
		client: nacosClient,
	}
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

	listURL := fmt.Sprintf("http://%s/nacos/v3/admin/ai/skills/list?%s",
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

	apiURL := fmt.Sprintf("http://%s/nacos/v3/client/ai/skills?%s",
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

// extractZip extracts a ZIP byte array to the target directory.
// ZIP entries like "skillName/SKILL.md" are extracted preserving their path structure.
func extractZip(zipBytes []byte, targetDir string) error {
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

// UploadSkill uploads a skill from local directory or a pre-built zip file.
// If skillPath points to a .zip file it is uploaded directly; otherwise the
// directory is packed into a zip on-the-fly (skillName/... structure).
func (s *SkillService) UploadSkill(skillPath string) error {
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
		zipBuffer = new(bytes.Buffer)
		zipWriter := zip.NewWriter(zipBuffer)

		err := filepath.Walk(skillPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			relPath, err := filepath.Rel(skillPath, path)
			if err != nil {
				return err
			}
			zipPath := filepath.Join(skillName, relPath)
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
	uploadURL := fmt.Sprintf("http://%s/nacos/v3/admin/ai/skills/upload?namespaceId=%s",
		s.client.ServerAddr, s.client.Namespace)
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
		return client.ParseHTTPError(resp.StatusCode, respBody, "upload skill")
	}

	return nil
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

