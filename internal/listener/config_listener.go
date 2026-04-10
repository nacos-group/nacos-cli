package listener

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// PollInterval is the interval between polling requests
	PollInterval = 15 * time.Second
)

// ConfigItem represents a configuration item being monitored
type ConfigItem struct {
	DataID string
	Group  string
	Tenant string
	MD5    string
}

// ChangeHandler is called when a config change is detected
type ChangeHandler func(dataID, group, tenant string) error

// ConfigListener listens for configuration changes from Nacos
type ConfigListener struct {
	serverAddr  string
	username    string
	password    string
	accessToken string
	httpClient  *http.Client
}

// NewConfigListener creates a new configuration listener
func NewConfigListener(serverAddr, username, password string) *ConfigListener {
	return &ConfigListener{
		serverAddr: serverAddr,
		username:   username,
		password:   password,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Login gets access token for authentication using v3 API
func (l *ConfigListener) Login() error {
	loginURL := fmt.Sprintf("http://%s/nacos/v3/auth/user/login", l.serverAddr)

	data := url.Values{}
	data.Set("username", l.username)
	data.Set("password", l.password)

	resp, err := l.httpClient.PostForm(loginURL, data)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read login response failed: %w", err)
	}

	// Parse accessToken from response
	token := extractAccessToken(string(body))
	if token == "" {
		return fmt.Errorf("failed to extract access token from response")
	}

	l.accessToken = token
	return nil
}

// StartListening starts polling for configuration changes (v3 API doesn't support long-polling)
func (l *ConfigListener) StartListening(items []ConfigItem, handler ChangeHandler, stopCh <-chan struct{}) error {
	// Login first to get access token
	if l.username != "" && l.password != "" {
		if err := l.Login(); err != nil {
			fmt.Printf("Warning: Login failed: %v\n", err)
		}
	}

	// Keep a map of current items and their MD5
	currentItems := make(map[string]*ConfigItem)
	for i := range items {
		key := fmt.Sprintf("%s_%s_%s", items[i].DataID, items[i].Group, items[i].Tenant)
		currentItems[key] = &items[i]
	}

	// Create a context that cancels when stopCh is closed
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-stopCh
		cancel()
	}()

	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	// Do an initial check immediately
	l.pollConfigs(ctx, currentItems, handler)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			l.pollConfigs(ctx, currentItems, handler)
		}
	}
}

// pollConfigs polls all configurations and checks for changes
func (l *ConfigListener) pollConfigs(ctx context.Context, currentItems map[string]*ConfigItem, handler ChangeHandler) {
	for key, item := range currentItems {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Fetch latest config
		content, newMD5, err := l.getConfig(item.DataID, item.Group, item.Tenant)
		if err != nil {
			// Check if it's a 404 error (config deleted)
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not exist") || strings.Contains(err.Error(), "config data not exist") {
				// Check if MD5 is already empty (already processed deletion)
				if item.MD5 == "" {
					// Already deleted and MD5 reset, skip
					continue
				}
				// First time seeing deletion, process it
				if err := handler(item.DataID, item.Group, item.Tenant); err != nil {
					fmt.Printf("Handler failed for %s/%s: %v\n", item.DataID, item.Group, err)
				}
				// Reset MD5 to empty so we can detect if skill is recreated
				item.MD5 = ""
				continue
			}
			fmt.Printf("Failed to fetch config %s/%s: %v\n", item.DataID, item.Group, err)
			continue
		}

		// Check if MD5 actually changed
		if item.MD5 == newMD5 {
			// MD5 hasn't changed, skip
			continue
		}

		// Call handler
		if err := handler(item.DataID, item.Group, item.Tenant); err != nil {
			fmt.Printf("Handler failed for %s/%s: %v\n", item.DataID, item.Group, err)
			continue
		}

		// Update MD5 only if handler succeeds
		item.MD5 = newMD5

		_ = content // Suppress unused warning
		_ = key     // Suppress unused warning
	}
}

// getConfig fetches the latest configuration content using v3 client API
func (l *ConfigListener) getConfig(dataID, group, tenant string) (string, string, error) {
	params := url.Values{}
	params.Set("dataId", dataID)
	params.Set("groupName", group)
	if tenant != "" {
		params.Set("namespaceId", tenant)
	}

	configURL := fmt.Sprintf("http://%s/nacos/v3/client/cs/config?%s", l.serverAddr, params.Encode())

	req, err := http.NewRequest("GET", configURL, nil)
	if err != nil {
		return "", "", err
	}
	if l.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+l.accessToken)
	}

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("get config returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse v3 response
	var v3Resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Content string `json:"content"`
			Md5     string `json:"md5"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &v3Resp); err != nil {
		// Fallback: treat as raw content
		contentMD5 := calculateMD5(string(body))
		return string(body), contentMD5, nil
	}

	if v3Resp.Code != 0 {
		return "", "", fmt.Errorf("get config failed: code=%d, message=%s", v3Resp.Code, v3Resp.Message)
	}

	content := v3Resp.Data.Content
	contentMD5 := v3Resp.Data.Md5
	if contentMD5 == "" {
		contentMD5 = calculateMD5(content)
	}

	return content, contentMD5, nil
}

// CalculateMD5 calculates MD5 hash of content (exported for reuse)
func CalculateMD5(content string) string {
	hash := md5.Sum([]byte(content))
	return fmt.Sprintf("%x", hash)
}

// calculateMD5 is the internal version
func calculateMD5(content string) string {
	return CalculateMD5(content)
}

// extractAccessToken extracts access token from JSON response
func extractAccessToken(body string) string {
	// Simple JSON parsing for {"accessToken":"xxx",...} or {"data":{"accessToken":"xxx",...}}
	start := strings.Index(body, `"accessToken":"`)
	if start == -1 {
		return ""
	}
	start += len(`"accessToken":"`)
	end := strings.Index(body[start:], `"`)
	if end == -1 {
		return ""
	}
	return body[start : start+end]
}
