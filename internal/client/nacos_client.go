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

// Auth types
const (
	AuthTypeNacos  = "nacos"  // username/password, token refresh
	AuthTypeAliyun = "aliyun" // AK/SK, long-lived
)

// NacosClient represents a Nacos API client
type NacosClient struct {
	ServerAddr       string
	Namespace        string
	AuthType         string // "nacos" or "aliyun"
	Username         string
	Password         string
	AccessKey        string // Aliyun AK
	SecretKey        string // Aliyun SK
	AccessToken      string
	TokenExpireAt    time.Time // from server tokenTtl (Nacos only)
	authLoginVersion string    // "v3" or "v1", fixed after first successful login
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

// NewNacosClient creates a new Nacos client. Use authType "nacos" (username/password) or "aliyun" (AK/SK).
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

// isLocalAddr checks if the server address is a local address
func (c *NacosClient) isLocalAddr() bool {
	addr := strings.ToLower(c.ServerAddr)
	return strings.HasPrefix(addr, "127.0.0.1") ||
		strings.HasPrefix(addr, "localhost") ||
		strings.HasPrefix(addr, "0.0.0.0")
}

// login gets an access token. Tries v3 first, then v1; sticks to the version that succeeds.
func (c *NacosClient) login() error {
	form := map[string]string{"username": c.Username, "password": c.Password}
	isLocal := c.isLocalAddr()

	tryV3 := c.authLoginVersion == "" || c.authLoginVersion == "v3"
	if tryV3 {
		u := fmt.Sprintf("http://%s/nacos/v3/auth/user/login", c.ServerAddr)
		resp, err := c.httpClient.R().SetFormData(form).Post(u)
		if resp != nil && resp.StatusCode() == 200 && c.applyLoginResponse(resp.Body()) {
			c.authLoginVersion = "v3"
		} else if !isLocal {
			if err != nil {
				fmt.Printf("v3 login failed: %v\n", err)
			} else if resp != nil && resp.StatusCode() != 200 {
				fmt.Printf("v3 login failed: status=%d, body=%s\n", resp.StatusCode(), string(resp.Body()))
			}
		}
	}

	// v1 login
	u := fmt.Sprintf("http://%s/nacos/v1/auth/login", c.ServerAddr)
	resp, err := c.httpClient.R().SetFormData(form).Post(u)
	if resp != nil && resp.StatusCode() == 200 && c.applyLoginResponse(resp.Body()) {
		c.authLoginVersion = "v1"
	} else if !isLocal {
		if err != nil {
			fmt.Printf("v1 login failed: %v\n", err)
		} else if resp != nil && resp.StatusCode() != 200 {
			fmt.Printf("v1 login failed: status=%d, body=%s\n", resp.StatusCode(), string(resp.Body()))
		}
	}
	return nil
}

// applyLoginResponse parses login JSON and sets AccessToken, TokenExpireAt. Returns true if accessToken present.
func (c *NacosClient) applyLoginResponse(body []byte) bool {
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return false
	}
	// Prefer nested data (v3 Result wrapper)
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

// ensureTokenValid refreshes token when needed (Nacos only; Aliyun has no token).
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

// getSignData builds SPAS sign payload (tenant, group, timeStamp).
// Matches Java SpasAdapter.getSignData logic:
// - If tenant is blank: group+timeStamp (or just timeStamp if group is blank)
// - If tenant is not blank: tenant+group+timeStamp (or tenant+timeStamp if group is blank)
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

// spasSign signs data with HMAC-SHA1 + Base64 using SK.
func spasSign(signData, secretKey string) string {
	mac := hmac.New(sha1.New, []byte(secretKey))
	mac.Write([]byte(signData))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// setSpasHeaders sets Aliyun AK/SK headers: timeStamp, Spas-AccessKey, Spas-Signature.
// Signature logic matches Java SpasAdapter.signWithHmacSha1Encrypt implementation.
func (c *NacosClient) setSpasHeaders(req *resty.Request, tenant, group string) {
	if c.AuthType != AuthTypeAliyun || c.AccessKey == "" || c.SecretKey == "" {
		return
	}
	// Use UnixMilli() to match Java System.currentTimeMillis()
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	req.SetHeader("timeStamp", ts)
	req.SetHeader("Spas-AccessKey", c.AccessKey)
	// Normalize tenant: treat "public" as empty string for signing
	normalizedTenant := tenant
	if normalizedTenant == "public" {
		normalizedTenant = ""
	}
	signData := getSignData(normalizedTenant, group, ts)
	req.SetHeader("Spas-Signature", spasSign(signData, c.SecretKey))
}

// ListConfigs retrieves a list of configurations. API version is determined by authLoginVersion (v1 vs v3).
func (c *NacosClient) ListConfigs(dataID, groupName, namespaceID string, pageNo, pageSize int) (*ConfigListResponse, error) {
	if err := c.ensureTokenValid(); err != nil {
		return nil, err
	}
	ns := namespaceID
	if ns == "" {
		ns = c.Namespace
	}

	// 根据登录时确定的版本选择接口
	if c.authLoginVersion == "v1" {
		return c.listConfigsV1(dataID, groupName, ns, pageNo, pageSize)
	}

	// v3 或未定：走 v3，非 200 直接报错
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

// listConfigsV1 retrieves configs using v1 API
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
		"groupName": group, // v3 API uses "groupName" instead of "group"
		"content":   content,
	}

	if c.Namespace != "" {
		params["namespaceId"] = c.Namespace // v3 API uses "namespaceId" instead of "tenant"
	}

	if c.AuthType == AuthTypeNacos && c.AccessToken != "" {
		// v3 API uses Authorization header instead of accessToken param
		// But we'll also keep it in params for compatibility if needed
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

	// v3 API returns Result<Boolean>: {"code":0,"message":null,"data":true}
	var v3Resp V3Response
	if err := json.Unmarshal(resp.Body(), &v3Resp); err != nil {
		// If not JSON, might be plain "true" (backward compatibility)
		if string(resp.Body()) == "true" {
			return nil
		}
		return fmt.Errorf("publish config failed: invalid response format: %s", string(resp.Body()))
	}
	if v3Resp.Code != 0 {
		return fmt.Errorf("publish config failed: code=%d, message=%s", v3Resp.Code, v3Resp.Message)
	}
	// Check if data is true
	var result bool
	if err := json.Unmarshal(v3Resp.Data, &result); err != nil {
		return fmt.Errorf("publish config failed: invalid data format: %w", err)
	}
	if !result {
		return fmt.Errorf("publish config failed: server returned false")
	}

	return nil
}
