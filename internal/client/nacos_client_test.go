package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestPublishConfigWithOptionsUsesConfigType(t *testing.T) {
	tests := []struct {
		name       string
		configType string
		want       string
	}{
		{name: "without config type", configType: "", want: ""},
		{name: "with config type", configType: "yaml", want: "yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/nacos/v3/admin/cs/config" {
					t.Fatalf("path = %q, want /nacos/v3/admin/cs/config", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Fatalf("method = %q, want POST", r.Method)
				}
				if err := r.ParseForm(); err != nil {
					t.Fatalf("ParseForm() error = %v", err)
				}
				if got := r.PostForm.Get("type"); got != tt.want {
					t.Fatalf("type form value = %q, want %q", got, tt.want)
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"code":0,"message":"success","data":true}`))
			}))
			defer server.Close()

			c, err := NewNacosClient(
				strings.TrimPrefix(server.URL, "http://"),
				"public",
				AuthTypeNone,
				"", "", "", "", "", "", "",
				"http",
			)
			if err != nil {
				t.Fatalf("NewNacosClient() error = %v", err)
			}

			opts := PublishConfigOptions{Type: tt.configType}
			if err := c.PublishConfigWithOptions("application.yaml", "DEFAULT_GROUP", "key: value", opts); err != nil {
				t.Fatalf("PublishConfigWithOptions() error = %v", err)
			}
		})
	}
}
