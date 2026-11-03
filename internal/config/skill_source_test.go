package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"testing"
)

func TestSkillSourceProfileRoundTripAndValidation(t *testing.T) {
	for _, source := range []string{"", "namespace", "hub", "all"} {
		cfg := &Config{}
		if err := cfg.SetValue("source", source); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "test.conf")
		if err := cfg.SaveConfig(path); err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		want := source
		if want == "" {
			want = "all"
		}
		got, sensitive, err := loaded.GetValue("source")
		if err != nil || got != want || sensitive {
			t.Fatalf("source = %q sensitive=%v err=%v", got, sensitive, err)
		}
	}
	cfg := &Config{Source: "hub"}
	if err := cfg.SetValue("source", "invalid"); err == nil || cfg.Source != "hub" {
		t.Fatal("invalid mode must fail without changing the preference")
	}
	if _, err := NormalizeSkillSource("true"); err == nil {
		t.Fatal("boolean cannot express three sources")
	}
}

func TestProfileSourceCanonicalAndLegacyYAML(t *testing.T) {
	for _, tc := range []struct{ name, yaml, want string }{
		{"unset", "", "all"},
		{"canonical", "source: namespace\n", "namespace"},
		{"legacy", "skillSource: hub\n", "hub"},
		{"canonical wins", "source: namespace\nskillSource: hub\n", "namespace"},
		{"canonical wins reversed", "skillSource: hub\nsource: namespace\n", "namespace"},
		{"explicit empty wins", "skillSource: hub\nsource: ''\n", "all"},
		{"explicit null wins", "skillSource: hub\nsource: null\n", "all"},
		{"canonical ignores invalid legacy", "source: all\nskillSource: [bad]\n", "all"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "profile.conf")
			if err := os.WriteFile(path, []byte("host: 127.0.0.1\nport: 8848\n"+tc.yaml), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			got, _, err := cfg.GetValue("source")
			if err != nil || got != tc.want || cfg.Host != "127.0.0.1" || cfg.Port != 8848 {
				t.Fatalf("source=%q, err=%v; connection fields must be preserved", got, err)
			}
		})
	}
}

func TestLegacyProfileSourceResaveWritesOneFieldAndPreservesCredentials(t *testing.T) {
	setTestHome(t, t.TempDir())
	path := filepath.Join(t.TempDir(), "profile.conf")
	data := "host: 127.0.0.1\nport: 8848\nauthType: nacos\nusername: reader\npassword: fixture-password\nnamespace: tenant-a\nskillSource: hub\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveConfig(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]interface{}
	if err := yaml.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["source"] != "hub" {
		t.Fatalf("canonical source not saved: %v", fields["source"])
	}
	if _, exists := fields["skillSource"]; exists {
		t.Fatal("legacy field must not be written")
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Source != "hub" || loaded.Host != "127.0.0.1" || loaded.Port != 8848 || loaded.AuthType != "nacos" || loaded.Username != "reader" || loaded.Password != "fixture-password" || loaded.Namespace != "tenant-a" {
		t.Fatal("migration changed connection, credentials or namespace")
	}
}

func TestProfileExposesOnlySourceKey(t *testing.T) {
	cfg := &Config{Source: "hub"}
	for _, legacy := range []string{"skill-source", "skillSource", "skill_source", "agentspecSource"} {
		if err := cfg.SetValue(legacy, "all"); err == nil {
			t.Fatalf("unexpected public key %s", legacy)
		}
		if _, _, err := cfg.GetValue(legacy); err == nil {
			t.Fatalf("unexpected public key %s", legacy)
		}
	}
	if cfg.Source != "hub" {
		t.Fatal("invalid keys changed the source setting")
	}
}
