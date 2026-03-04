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
	"time"

	"github.com/nov11/nacos-cli/internal/client"
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

// Skill represents a complete skill
type Skill struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Instruction string              `json:"instruction"`
	UniformId   interface{}         `json:"uniformId"` // Can be string or number
	Resources   []map[string]string `json:"resources"`
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
		return nil, 0, fmt.Errorf("list skills failed: status=%d, body=%s", resp.StatusCode, string(respBody))
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

// GetSkill retrieves a skill and saves it to local directory
func (s *SkillService) GetSkill(skillName, outputDir string) error {
	const maxRetries = 3
	const retryDelay = 3 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := s.getSkillWithValidation(skillName, outputDir)
		if err == nil {
			return nil
		}

		// Check if it's a uniformId mismatch error
		if strings.Contains(err.Error(), "uniformId mismatch") {
			fmt.Printf("\nuniformId is inconsistent: %v\n", err)
			if attempt < maxRetries {
				fmt.Printf("   等待 3 秒后重试 (%d/%d)...\n\n", attempt, maxRetries)
				time.Sleep(retryDelay)
				continue
			}
		}

		return err
	}

	return fmt.Errorf("重试 %d 次后仍失败", maxRetries)
}

// getSkillWithValidation retrieves a skill with uniformId validation
func (s *SkillService) getSkillWithValidation(skillName, outputDir string) error {
	group := fmt.Sprintf("skill_%s", skillName)

	// Get skill.json
	skillJSON, err := s.client.GetConfig("skill.json", group)
	if err != nil {
		return fmt.Errorf("failed to get skill.json: %w", err)
	}

	// Parse skill data
	var skill Skill
	if err := json.Unmarshal([]byte(skillJSON), &skill); err != nil {
		return fmt.Errorf("failed to parse skill.json: %w", err)
	}

	// Create output directory
	skillDir := filepath.Join(outputDir, skillName)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Download resources
	resourceContents := make(map[string]map[string]interface{})
	for _, resourceInfo := range skill.Resources {
		resourceName := resourceInfo["name"]
		if resourceName == "" {
			continue
		}

		resourceType := resourceInfo["type"]

		// Construct dataId: resource_{type}_{name}.json
		// Replace . with __ in name (e.g., init_skill.py -> init_skill__py)
		normalizedName := strings.ReplaceAll(resourceName, ".", "__")
		resourceDataID := fmt.Sprintf("resource_%s_%s.json", resourceType, normalizedName)
		resourceJSON, err := s.client.GetConfig(resourceDataID, group)
		if err != nil {
			continue
		}

		var resourceData map[string]interface{}
		if err := json.Unmarshal([]byte(resourceJSON), &resourceData); err != nil {
			continue
		}

		// Validate uniformId consistency
		skillUniformIdStr := normalizeUniformId(skill.UniformId)
		resourceUniformIdStr := normalizeUniformId(resourceData["uniformId"])

		if skillUniformIdStr != "" && resourceUniformIdStr != "" && resourceUniformIdStr != skillUniformIdStr {
			return fmt.Errorf("uniformId mismatch: skill.json has '%s', but resource '%s' has '%s'",
				skillUniformIdStr, resourceName, resourceUniformIdStr)
		}

		resourceContents[resourceName] = resourceData

		// Save resource file
		finalName, ok := resourceData["name"].(string)
		if !ok {
			continue
		}

		finalType, ok := resourceData["type"].(string)
		if !ok {
			continue
		}

		content, ok := resourceData["content"].(string)
		if !ok {
			continue
		}

		// Determine file directory based on type
		var fileDir string
		if finalType != "" {
			// If type is specified, use it as subdirectory name
			fileDir = filepath.Join(skillDir, finalType)
		} else {
			// If type is empty, save in skill root directory
			fileDir = skillDir
		}

		if err := os.MkdirAll(fileDir, 0755); err != nil {
			return err
		}

		filePath := filepath.Join(fileDir, finalName)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return err
		}
	}

	// Generate SKILL.md
	if err := s.generateSkillMD(skillDir, &skill, resourceContents); err != nil {
		return err
	}

	return nil
}

// generateSkillMD creates SKILL.md file
func (s *SkillService) generateSkillMD(skillDir string, skill *Skill, resources map[string]map[string]interface{}) error {
	var md strings.Builder

	// YAML frontmatter
	md.WriteString("---\n")
	md.WriteString(fmt.Sprintf("name: %s\n", skill.Name))
	md.WriteString(fmt.Sprintf("description: \"%s\"\n", skill.Description))
	md.WriteString("---\n\n")

	// Instruction
	md.WriteString(skill.Instruction)
	md.WriteString("\n")

	// Write to file
	mdPath := filepath.Join(skillDir, "SKILL.md")
	return os.WriteFile(mdPath, []byte(md.String()), 0644)
}

// UploadSkill uploads a skill from local directory
func (s *SkillService) UploadSkill(skillPath string) error {
	// Create ZIP file
	zipBuffer := new(bytes.Buffer)
	zipWriter := zip.NewWriter(zipBuffer)

	skillName := filepath.Base(skillPath)

	err := filepath.Walk(skillPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(skillPath, path)
		if err != nil {
			return err
		}

		// Create file in ZIP with skill directory name
		zipPath := filepath.Join(skillName, relPath)
		writer, err := zipWriter.Create(zipPath)
		if err != nil {
			return err
		}

		// Copy file content
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

	// Add authentication header
	if s.client.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.client.AccessToken))
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: status=%d, body=%s", resp.StatusCode, string(respBody))
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

// normalizeUniformId converts uniformId to string (handles both string and number types)
func normalizeUniformId(uniformId interface{}) string {
	if uniformId == nil {
		return ""
	}

	switch v := uniformId.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
