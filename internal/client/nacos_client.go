package client

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	AuthTypeNone   = "none"   // No authentication (public registry)
	AuthTypeNacos  = "nacos"  // Username/password authentication
	AuthTypeAliyun = "aliyun" // AccessKey/SecretKey authentication
	AuthTypeToken  = "token"  // Pre-issued access token (no login required)
)

// NacosClient represents a Nacos API client
type NacosClient struct {
	ServerAddr       string
	Namespace        string
	AuthType         string
	Username         string
	Password         string
	AccessKey        string
	SecretKey        string
	AccessToken      string
	TokenExpireAt    time.Time
	authLoginVersion string // "v3" or "v1", determined by first successful login
	httpClient       *resty.Client
}

// Config represents a Nacos configuration
type Config struct {
	DataID    string `json:"dataId"`
	Group     string `json:"group"`
	GroupName string `json:"groupName"`
	Content   string `json:"content"`
	Type      string `json:"type"`
}

// ConfigListResponse represents the response of list configs API
type ConfigListResponse struct {
	TotalCount     int      `json:"totalCount"`
	PageNumber     int      `json:"pageNumber"`
	PagesAvailable int      `json:"pagesAvailable"`
	PageItems      []Config `json:"pageItems"`
}

// V3Response represents the v3 API response wrapper
type V3Response struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// ParseHTTPError converts an HTTP error response into a user-friendly error message.
// It handles common HTTP status codes with actionable hints.
func ParseHTTPError(statusCode int, body []byte, operation string) error {
	// Try to extract message from v3 response body
	serverMsg := ""
	if len(body) > 0 {
		var v3 V3Response
		if err := json.Unmarshal(body, &v3); err == nil && v3.Message != "" {
			serverMsg = v3.Message
		}
	}

	switch statusCode {
	case 401:
		hint := "authentication required — please check your username/password or token"
		if serverMsg != "" {
			return fmt.Errorf("%s failed (401 Unauthorized): %s\nHint: %s", operation, serverMsg, hint)
		}
		return fmt.Errorf("%s failed (401 Unauthorized): %s", operation, hint)
	case 403:
		hint := "access denied — token may be expired or you lack permission for this operation"
		if serverMsg != "" {
			return fmt.Errorf("%s failed (403 Forbidden): %s\nHint: %s", operation, serverMsg, hint)
		}
		return fmt.Errorf("%s failed (403 Forbidden): %s", operation, hint)
	case 404:
		hint := "resource not found — check the name/namespace or whether it exists"
		if serverMsg != "" {
			return fmt.Errorf("%s failed (404 Not Found): %s\nHint: %s", operation, serverMsg, hint)
		}
		return fmt.Errorf("%s failed (404 Not Found): %s", operation, hint)
	case 500:
		hint := "server internal error — check Nacos server logs for details"
		if serverMsg != "" {
			return fmt.Errorf("%s failed (500 Internal Server Error): %s\nHint: %s", operation, serverMsg, hint)
		}
		return fmt.Errorf("%s failed (500 Internal Server Error): %s", operation, hint)
	default:
		if serverMsg != "" {
			return fmt.Errorf("%s failed (HTTP %d): %s", operation, statusCode, serverMsg)
		}
		if len(body) > 0 {
			// Truncate long bodies
			bodyStr := string(body)
			if len(bodyStr) > 200 {
				bodyStr = bodyStr[:200] + "..."
			}
			return fmt.Errorf("%s failed (HTTP %d): %s", operation, statusCode, bodyStr)
		}
		return fmt.Errorf("%s failed (HTTP %d)", operation, statusCode)
	}
}

// NewNacosClient creates a new Nacos client with automatic authentication.
// If token is non-empty, it is used directly as the Bearer token and no login request is made.
// Returns an error if login is required but fails (e.g. wrong credentials).
func NewNacosClient(serverAddr, namespace, authType, username, password, accessKey, secretKey, token string) (*NacosClient, error) {
	if namespace == "" {
		namespace = "public"
	}
	if authType == "" {
		if token != "" {
			authType = AuthTypeToken
		} else if accessKey != "" && secretKey != "" {
			authType = AuthTypeAliyun
		} else if username != "" && password != "" {
			authType = AuthTypeNacos
		} else {
			authType = AuthTypeNone
		}
	}

	c := &NacosClient{
		ServerAddr:  serverAddr,
		Namespace:   namespace,
		AuthType:    authType,
		Username:    username,
		Password:    password,
		AccessKey:   accessKey,
		SecretKey:   secretKey,
		AccessToken: token,
		httpClient:  resty.New(),
	}

	// If a token is provided directly, skip login entirely.
	if token != "" {
		return c, nil
	}

	if c.AuthType == AuthTypeNacos {
		if err := c.login(); err != nil {
			return nil, fmt.Errorf("login failed: %w", err)
		}
	}
	return c, nil
}

// isLocalAddr checks if the server address is localhost
func (c *NacosClient) isLocalAddr() bool {
	addr := strings.ToLower(c.ServerAddr)
	return strings.HasPrefix(addr, "127.0.0.1") ||
		strings.HasPrefix(addr, "localhost") ||
		strings.HasPrefix(addr, "0.0.0.0")
}

// login attempts to authenticate with Nacos server using v3 API first, then falls back to v1.
// For Nacos 3.x, v3 login succeeds but some legacy v1 APIs (like config list) may return 410 (Gone),
// so once v3 login succeeds we MUST NOT override authLoginVersion with v1.
func (c *NacosClient) login() error {
	form := map[string]string{"username": c.Username, "password": c.Password}
	isLocal := c.isLocalAddr()

	// Prefer v3 login. If we've previously determined v1 only, skip v3.
	tryV3 := c.authLoginVersion == "" || c.authLoginVersion == "v3"
	if tryV3 {
		u := fmt.Sprintf("http://%s/nacos/v3/auth/user/login", c.ServerAddr)
		resp, err := c.httpClient.R().SetFormData(form).Post(u)
		if err != nil {
			if !isLocal {
				fmt.Printf("v3 login failed: %v\n", err)
			}
		} else if resp != nil && resp.StatusCode() == 200 && c.applyLoginResponse(resp.Body()) {
			c.authLoginVersion = "v3"
			return nil
		} else if !isLocal && resp != nil {
			fmt.Printf("v3 login failed: status=%d, body=%s\n", resp.StatusCode(), string(resp.Body()))
		}
	}

	// Fallback to v1 login if v3 is unavailable (e.g., older Nacos versions).
	u := fmt.Sprintf("http://%s/nacos/v1/auth/login", c.ServerAddr)
	resp, err := c.httpClient.R().SetFormData(form).Post(u)
	if err != nil {
		if !isLocal {
			fmt.Printf("v1 login failed: %v\n", err)
		}
		return err
	}
	if resp != nil && resp.StatusCode() == 200 && c.applyLoginResponse(resp.Body()) {
		c.authLoginVersion = "v1"
		return nil
	}
	if !isLocal && resp != nil {
		fmt.Printf("v1 login failed: status=%d, body=%s\n", resp.StatusCode(), string(resp.Body()))
	}
	return fmt.Errorf("login failed: status=%d", resp.StatusCode())
}

// applyLoginResponse parses login response and extracts access token
func (c *NacosClient) applyLoginResponse(body []byte) bool {
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return false
	}
	if data, ok := result["data"].(map[string]interface{}); ok {
		return c.applyLoginFromMap(data)
	}
	return c.applyLoginFromMap(result)
}

func (c *NacosClient) applyLoginFromMap(m map[string]interface{}) bool {
	token, ok := m["accessToken"].(string)
	if !ok || token == "" {
		return false
	}
	c.AccessToken = token
	var ttlSec int64 = 0
	switch v := m["tokenTtl"].(type) {
	case float64:
		ttlSec = int64(v)
	case int:
		ttlSec = int64(v)
	case int64:
		ttlSec = v
	}
	if ttlSec > 0 {
		c.TokenExpireAt = time.Now().Add(time.Duration(ttlSec) * time.Second)
	} else {
		c.TokenExpireAt = time.Time{}
	}
	return true
}

// EnsureTokenValid ensures the access token is valid, refreshing if necessary
func (c *NacosClient) EnsureTokenValid() error {
	// Token auth: user-supplied token, no refresh
	if c.AuthType == AuthTypeToken {
		return nil
	}
	if c.AuthType != AuthTypeNacos {
		return nil
	}
	if c.AccessToken == "" {
		return c.login()
	}
	if !c.TokenExpireAt.IsZero() && time.Now().Add(5*time.Second).After(c.TokenExpireAt) {
		return c.login()
	}
	return nil
}

// getSignData builds SPAS signature payload following Aliyun authentication specification
func getSignData(tenant, group, timeStamp string) string {
	if tenant == "" {
		if group == "" {
			return timeStamp
		}
		return group + "+" + timeStamp
	}
	if group != "" {
		return tenant + "+" + group + "+" + timeStamp
	}
	return tenant + "+" + timeStamp
}

// spasSign signs data with HMAC-SHA1 and encodes with Base64
func spasSign(signData, secretKey string) string {
	mac := hmac.New(sha1.New, []byte(secretKey))
	mac.Write([]byte(signData))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// setSpasHeaders sets Aliyun authentication headers for SPAS signature
func (c *NacosClient) setSpasHeaders(req *resty.Request, tenant, group string) {
	if c.AuthType != AuthTypeAliyun || c.AccessKey == "" || c.SecretKey == "" {
		return
	}
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	req.SetHeader("timeStamp", ts)
	req.SetHeader("Spas-AccessKey", c.AccessKey)
	normalizedTenant := tenant
	if normalizedTenant == "public" {
		normalizedTenant = ""
	}
	signData := getSignData(normalizedTenant, group, ts)
	req.SetHeader("Spas-Signature", spasSign(signData, c.SecretKey))
}

// ListConfigs retrieves a list of configurations using v3 or v1 API based on login version
func (c *NacosClient) ListConfigs(dataID, groupName, namespaceID string, pageNo, pageSize int) (*ConfigListResponse, error) {
	if err := c.EnsureTokenValid(); err != nil {
		return nil, err
	}
	ns := namespaceID
	if ns == "" {
		ns = c.Namespace
	}

	if c.authLoginVersion == "v1" {
		return c.listConfigsV1(dataID, groupName, ns, pageNo, pageSize)
	}
	params := url.Values{}
	if strings.Contains(dataID, "*") || strings.Contains(groupName, "*") {
		params.Set("search", "blur")
	} else {
		params.Set("search", "accurate")
	}

	params.Set("dataId", dataID)
	params.Set("groupName", groupName)
	params.Set("pageNo", fmt.Sprintf("%d", pageNo))
	params.Set("pageSize", fmt.Sprintf("%d", pageSize))

	if ns != "" {
		params.Set("namespaceId", ns)
	}

	v3URL := fmt.Sprintf("http://%s/nacos/v3/admin/cs/config/list", c.ServerAddr)
	req := c.httpClient.R().SetQueryString(params.Encode())
	if c.AuthType == AuthTypeNacos && c.AccessToken != "" {
		req.SetHeader("Authorization", fmt.Sprintf("Bearer %s", c.AccessToken))
	}
	c.setSpasHeaders(req, ns, groupName)
	resp, err := req.Get(v3URL)

	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, ParseHTTPError(resp.StatusCode(), resp.Body(), "list configs")
	}

	var v3Resp V3Response
	if err := json.Unmarshal(resp.Body(), &v3Resp); err != nil {
		return nil, err
	}
	if v3Resp.Code != 0 {
		return nil, fmt.Errorf("list configs failed: code=%d, message=%s", v3Resp.Code, v3Resp.Message)
	}
	var configList ConfigListResponse
	if err := json.Unmarshal(v3Resp.Data, &configList); err != nil {
		return nil, err
	}
	return &configList, nil
}

// listConfigsV1 retrieves configurations using Nacos v1 API
func (c *NacosClient) listConfigsV1(dataID, groupName, namespace string, pageNo, pageSize int) (*ConfigListResponse, error) {
	if err := c.EnsureTokenValid(); err != nil {
		return nil, err
	}
	params := url.Values{}
	if strings.Contains(dataID, "*") || strings.Contains(groupName, "*") {
		params.Set("search", "blur")
	} else {
		params.Set("search", "accurate")
	}
	params.Set("dataId", dataID)
	params.Set("group", groupName)
	params.Set("pageNo", fmt.Sprintf("%d", pageNo))
	params.Set("pageSize", fmt.Sprintf("%d", pageSize))

	if namespace != "" {
		params.Set("tenant", namespace)
	}

	if c.AuthType == AuthTypeNacos && c.AccessToken != "" {
		params.Set("accessToken", c.AccessToken)
	}

	v1URL := fmt.Sprintf("http://%s/nacos/v1/cs/configs", c.ServerAddr)
	req := c.httpClient.R().SetQueryString(params.Encode())
	c.setSpasHeaders(req, namespace, groupName)
	resp, err := req.Get(v1URL)

	if err != nil {
		return nil, fmt.Errorf("v1 request failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return nil, ParseHTTPError(resp.StatusCode(), resp.Body(), "list configs (v1)")
	}

	var configList ConfigListResponse
	if err := json.Unmarshal(resp.Body(), &configList); err != nil {
		return nil, err
	}

	return &configList, nil
}

// GetConfig retrieves a specific configuration using v3 client API
func (c *NacosClient) GetConfig(dataID, group string) (string, error) {
	if err := c.EnsureTokenValid(); err != nil {
		return "", err
	}

	ns := c.Namespace
	if ns == "public" {
		ns = ""
	}

	params := url.Values{}
	params.Set("dataId", dataID)
	params.Set("groupName", group)
	if ns != "" {
		params.Set("namespaceId", ns)
	}

	apiURL := fmt.Sprintf("http://%s/nacos/v3/client/cs/config", c.ServerAddr)
	req := c.httpClient.R().SetQueryString(params.Encode())
	if c.AuthType == AuthTypeNacos && c.AccessToken != "" {
		req.SetHeader("Authorization", fmt.Sprintf("Bearer %s", c.AccessToken))
	}
	c.setSpasHeaders(req, c.Namespace, group)
	resp, err := req.Get(apiURL)

	if err != nil {
		return "", fmt.Errorf("get config failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return "", ParseHTTPError(resp.StatusCode(), resp.Body(), "get config")
	}

	// Parse v3 response
	var v3Resp V3Response
	if err := json.Unmarshal(resp.Body(), &v3Resp); err != nil {
		// If not JSON, return raw content (for backward compatibility)
		return string(resp.Body()), nil
	}
	if v3Resp.Code != 0 {
		return "", fmt.Errorf("get config failed: code=%d, message=%s", v3Resp.Code, v3Resp.Message)
	}

	// Parse config from data
	var config Config
	if err := json.Unmarshal(v3Resp.Data, &config); err != nil {
		// Try to return raw data as string
		var rawContent string
		if err := json.Unmarshal(v3Resp.Data, &rawContent); err != nil {
			return string(v3Resp.Data), nil
		}
		return rawContent, nil
	}

	return config.Content, nil
}

// PublishConfig publishes a configuration
func (c *NacosClient) PublishConfig(dataID, group, content string) error {
	if err := c.EnsureTokenValid(); err != nil {
		return err
	}
	params := map[string]string{
		"dataId":    dataID,
		"groupName": group,
		"content":   content,
	}

	if c.Namespace != "" {
		params["namespaceId"] = c.Namespace
	}

	apiURL := fmt.Sprintf("http://%s/nacos/v3/admin/cs/config", c.ServerAddr)
	req := c.httpClient.R().SetFormData(params)
	if c.AuthType == AuthTypeNacos && c.AccessToken != "" {
		req.SetHeader("Authorization", fmt.Sprintf("Bearer %s", c.AccessToken))
	}
	c.setSpasHeaders(req, c.Namespace, group)
	resp, err := req.Post(apiURL)

	if err != nil {
		return fmt.Errorf("publish config failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return ParseHTTPError(resp.StatusCode(), resp.Body(), "publish config")
	}

	var v3Resp V3Response
	if err := json.Unmarshal(resp.Body(), &v3Resp); err != nil {
		if string(resp.Body()) == "true" {
			return nil
		}
		return fmt.Errorf("publish config failed: invalid response format: %s", string(resp.Body()))
	}
	if v3Resp.Code != 0 {
		return fmt.Errorf("publish config failed: code=%d, message=%s", v3Resp.Code, v3Resp.Message)
	}
	var result bool
	if err := json.Unmarshal(v3Resp.Data, &result); err != nil {
		return fmt.Errorf("publish config failed: invalid data format: %w", err)
	}
	if !result {
		return fmt.Errorf("publish config failed: server returned false")
	}

	return nil
}
