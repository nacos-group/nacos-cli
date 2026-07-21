package client

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestBaseURL(t *testing.T) {
	tests := []struct {
		name       string
		serverAddr string
		scheme     string
		want       string
	}{
		{
			name:       "default http scheme",
			serverAddr: "nacos.example.com:8848",
			scheme:     "",
			want:       "http://nacos.example.com:8848",
		},
		{
			name:       "explicit http scheme",
			serverAddr: "nacos.example.com:8848",
			scheme:     "http",
			want:       "http://nacos.example.com:8848",
		},
		{
			name:       "https scheme",
			serverAddr: "nacos.example.com:443",
			scheme:     "https",
			want:       "https://nacos.example.com:443",
		},
		{
			name:       "https without explicit port",
			serverAddr: "nacos.example.com",
			scheme:     "https",
			want:       "https://nacos.example.com",
		},
		{
			name:       "uppercase scheme normalized",
			serverAddr: "nacos.example.com:8848",
			scheme:     "HTTPS",
			want:       "HTTPS://nacos.example.com:8848",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &NacosClient{
				ServerAddr: tt.serverAddr,
				Scheme:     tt.scheme,
			}
			got := c.BaseURL()
			if got != tt.want {
				t.Errorf("BaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewNacosClientScheme(t *testing.T) {
	// Test that scheme is properly stored when creating client
	c, err := NewNacosClient(
		"localhost:8848",
		"public",
		AuthTypeNone,
		"", "", "", "", "", "", "",
		"https",
	)
	if err != nil {
		t.Fatalf("NewNacosClient() error = %v", err)
	}
	if c.Scheme != "https" {
		t.Errorf("Scheme = %q, want %q", c.Scheme, "https")
	}
	if c.BaseURL() != "https://localhost:8848" {
		t.Errorf("BaseURL() = %q, want %q", c.BaseURL(), "https://localhost:8848")
	}
}

func TestNewNacosClientDefaultScheme(t *testing.T) {
	// Test that empty scheme defaults to "http"
	c, err := NewNacosClient(
		"localhost:8848",
		"public",
		AuthTypeNone,
		"", "", "", "", "", "", "",
		"",
	)
	if err != nil {
		t.Fatalf("NewNacosClient() error = %v", err)
	}
	if c.Scheme != "http" {
		t.Errorf("Scheme = %q, want %q", c.Scheme, "http")
	}
	if c.BaseURL() != "http://localhost:8848" {
		t.Errorf("BaseURL() = %q, want %q", c.BaseURL(), "http://localhost:8848")
	}
}

func TestFetchStsCredentialsSendsClusterIDHeader(t *testing.T) {
	t.Setenv("HICLAW_CLUSTER_ID", "cluster-123")

	stsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer auth-token" {
			t.Fatalf("Authorization header = %q, want %q", got, "Bearer auth-token")
		}
		if got := r.Header.Get("X-HiClaw-Cluster-ID"); got != "cluster-123" {
			t.Fatalf("X-HiClaw-Cluster-ID header = %q, want %q", got, "cluster-123")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_key_id":"ak","access_key_secret":"sk","security_token":"token","expires_in_sec":600}`))
	}))
	defer stsServer.Close()

	c, err := NewNacosClient(
		"localhost:8848",
		"public",
		AuthTypeStsToken,
		"", "", "", "", "",
		stsServer.URL,
		"auth-token",
		"",
	)
	if err != nil {
		t.Fatalf("NewNacosClient() error = %v", err)
	}
	if c.AccessKey != "ak" || c.SecretKey != "sk" || c.SecurityToken != "token" {
		t.Fatalf("STS credentials = (%q, %q, %q), want (ak, sk, token)", c.AccessKey, c.SecretKey, c.SecurityToken)
	}
}

func TestFetchStsCredentialsSendsAgentTeamsClusterIDHeader(t *testing.T) {
	t.Setenv("AGENTTEAMS_CLUSTER_ID", "agentteams-cluster")
	t.Setenv("HICLAW_CLUSTER_ID", "hiclaw-cluster")

	stsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-AgentTeams-Cluster-ID"); got != "agentteams-cluster" {
			t.Fatalf("X-AgentTeams-Cluster-ID = %q, want %q", got, "agentteams-cluster")
		}
		if got := r.Header.Get("X-HiClaw-Cluster-ID"); got != "" {
			t.Fatalf("X-HiClaw-Cluster-ID = %q, want empty", got)
		}
		_, _ = w.Write([]byte(`{"access_key_id":"ak","access_key_secret":"sk","security_token":"st","expires_in_sec":60}`))
	}))
	defer stsServer.Close()

	if _, err := NewNacosClient(
		"localhost:8848",
		"public",
		AuthTypeStsAgentTeams,
		"", "", "", "", "",
		stsServer.URL,
		"auth-token",
		"http",
	); err != nil {
		t.Fatalf("NewNacosClient() error = %v", err)
	}
}

func TestNacosClientReusesHTTPClientWithTimeout(t *testing.T) {
	c, err := NewNacosClient(
		"127.0.0.1:8848",
		"public",
		AuthTypeNone,
		"", "", "", "", "", "", "",
		"http",
	)
	if err != nil {
		t.Fatal(err)
	}

	first := c.HTTPClient()
	second := c.HTTPClient()
	if first != second {
		t.Fatal("HTTPClient returned different instances")
	}
	if first.Timeout != DefaultHTTPTimeout {
		t.Fatalf("timeout = %s, want %s", first.Timeout, DefaultHTTPTimeout)
	}
}

func TestNewNacosClientWithTokenSetsAuthorizationHeader(t *testing.T) {
	c, err := NewNacosClient(
		"127.0.0.1:8848",
		"public",
		AuthTypeNone,
		"", "", "", "", "", "", "",
		"http",
		WithToken("test-token"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if c.AuthType != AuthTypeToken {
		t.Fatalf("AuthType = %q, want %q", c.AuthType, AuthTypeToken)
	}

	req, err := c.NewAuthedRequest(http.MethodGet, c.BaseURL(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Fatalf("Authorization header = %q, want %q", got, "Bearer test-token")
	}
	if got := c.httpClient.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Fatalf("resty Authorization header = %q, want %q", got, "Bearer test-token")
	}
}

func TestNacosTokenTransportOnV3ConfigRequest(t *testing.T) {
	tests := []struct {
		name              string
		transport         string
		wantAuthorization string
		wantAccessHeader  string
		wantAccessQuery   string
	}{
		{name: "default bearer", wantAuthorization: "Bearer test-token"},
		{name: "raw authorization", transport: TokenTransportAuthorization, wantAuthorization: "test-token"},
		{name: "accessToken header", transport: TokenTransportHeader, wantAccessHeader: "test-token"},
		{name: "accessToken query", transport: TokenTransportQuery, wantAccessQuery: "test-token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/nacos/v3/auth/user/login":
					_, _ = w.Write([]byte(`{"accessToken":"test-token","tokenTtl":18000}`))
				case "/nacos/v3/client/cs/config":
					if got := r.Header.Get("Authorization"); got != tt.wantAuthorization {
						t.Errorf("Authorization header = %q, want %q", got, tt.wantAuthorization)
					}
					if got := r.Header.Get("accessToken"); got != tt.wantAccessHeader {
						t.Errorf("accessToken header = %q, want %q", got, tt.wantAccessHeader)
					}
					if got := r.URL.Query().Get("accessToken"); got != tt.wantAccessQuery {
						t.Errorf("accessToken query = %q, want %q", got, tt.wantAccessQuery)
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"code":0,"message":"","data":{"content":"ok"}}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			serverURL, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			client, err := NewNacosClient(
				serverURL.Host, "public", AuthTypeNacos,
				"user", "password", "", "", "", "", "", serverURL.Scheme,
				WithTokenTransport(tt.transport),
			)
			if err != nil {
				t.Fatal(err)
			}
			content, err := client.GetConfig("test", "DEFAULT_GROUP")
			if err != nil {
				t.Fatal(err)
			}
			if content != "ok" {
				t.Fatalf("content = %q, want ok", content)
			}
		})
	}
}

func TestNormalizeTokenTransportRejectsInvalidValue(t *testing.T) {
	if _, err := NormalizeTokenTransport("cookie"); err == nil {
		t.Fatal("NormalizeTokenTransport accepted invalid value")
	}
}

func TestConfigAPIsFallBackToV1OnMissingV3Routes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nacos/v3/auth/user/login":
			_, _ = w.Write([]byte(`{"accessToken":"test-token","tokenTtl":18000}`))
		case "/nacos/v3/admin/cs/config/list", "/nacos/v3/client/cs/config", "/nacos/v3/admin/cs/config":
			http.NotFound(w, r)
		case "/nacos/v1/cs/configs":
			if r.Method == http.MethodPost {
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if got := r.Form.Get("accessToken"); got != "test-token" {
					t.Errorf("publish accessToken = %q, want test-token", got)
				}
				_, _ = w.Write([]byte("true"))
				return
			}
			if got := r.URL.Query().Get("accessToken"); got != "test-token" {
				t.Errorf("query accessToken = %q, want test-token", got)
			}
			if r.URL.Query().Get("pageNo") != "" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"totalCount":1,"pageNumber":1,"pagesAvailable":1,"pageItems":[{"dataId":"test","group":"DEFAULT_GROUP"}]}`))
				return
			}
			_, _ = w.Write([]byte("content-from-v1"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewNacosClient(
		serverURL.Host, "namespace", AuthTypeNacos,
		"user", "password", "", "", "", "", "", serverURL.Scheme,
		WithTokenTransport(TokenTransportQuery),
	)
	if err != nil {
		t.Fatal(err)
	}

	configs, err := c.ListConfigs("*", "*", "namespace", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if configs.TotalCount != 1 {
		t.Fatalf("TotalCount = %d, want 1", configs.TotalCount)
	}
	content, err := c.GetConfig("test", "DEFAULT_GROUP")
	if err != nil {
		t.Fatal(err)
	}
	if content != "content-from-v1" {
		t.Fatalf("content = %q, want content-from-v1", content)
	}
	if err := c.PublishConfig("test", "DEFAULT_GROUP", "updated"); err != nil {
		t.Fatal(err)
	}
}
