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
	AuthTypeNacos  = "nacos"  // Username/password authentication
	AuthTypeAliyun = "aliyun" // AccessKey/SecretKey authentication
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

// NewNacosClient creates a new Nacos client with automatic authentication
func NewNacosClient(serverAddr, namespace, authType, username, password, accessKey, secretKey string) *NacosClient {
	if namespace == "" {
		namespace = "public"
	}
	if authType == "" {
		if accessKey != "" && secretKey != "" {
			authType = AuthTypeAliyun
		} else {
			authType = AuthTypeNacos
		}
	}

	c := &NacosClient{
		ServerAddr: serverAddr,
		Namespace:  namespace,
		AuthType:   authType,
		Username:   username,
		Password:   password,
		AccessKey:  accessKey,
		SecretKey:  secretKey,
		httpClient: resty.New(),
	}

	if c.AuthType == AuthTypeNacos {
		if err := c.login(); err != nil {
			fmt.Printf("Warning: Login failed: %v\n", err)
		}
	}
	return c
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

// ensureTokenValid ensures the access token is valid, refreshing if necessary
func (c *NacosClient) ensureTokenValid() error {
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
	if err := c.ensureTokenValid(); err != nil {
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
		return nil, fmt.Errorf("list configs failed: status=%d, body=%s", resp.StatusCode(), string(resp.Body()))
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
	if err := c.ensureTokenValid(); err != nil {
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
		return nil, fmt.Errorf("v1 list configs failed: status=%d", resp.StatusCode())
	}

	var configList ConfigListResponse
	if err := json.Unmarshal(resp.Body(), &configList); err != nil {
		return nil, err
	}

	return &configList, nil
}

// GetConfig retrieves a specific configuration
func (c *NacosClient) GetConfig(dataID, group string) (string, error) {
	if err := c.ensureTokenValid(); err != nil {
		return "", err
	}
	params := url.Values{}
	params.Set("dataId", dataID)
	params.Set("group", group)

	if c.Namespace != "" {
		params.Set("tenant", c.Namespace)
	}

	if c.AuthType == AuthTypeNacos && c.AccessToken != "" {
		params.Set("accessToken", c.AccessToken)
	}

	apiURL := fmt.Sprintf("http://%s/nacos/v1/cs/configs", c.ServerAddr)
	req := c.httpClient.R().SetQueryString(params.Encode())
	c.setSpasHeaders(req, c.Namespace, group)
	resp, err := req.Get(apiURL)

	if err != nil {
		return "", fmt.Errorf("get config failed: %w", err)
	}

	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("get config failed: status=%d", resp.StatusCode())
	}

	return string(resp.Body()), nil
}

// PublishConfig publishes a configuration
func (c *NacosClient) PublishConfig(dataID, group, content string) error {
	if err := c.ensureTokenValid(); err != nil {
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
		return fmt.Errorf("publish config failed: status=%d, body=%s", resp.StatusCode(), string(resp.Body()))
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
