package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderConfigGetPretty_WithContent(t *testing.T) {
	got := captureStdout(t, func() {
		renderConfigGetPretty("my-data-id", "my-group", "key: value\nfoo: bar\n")
	})

	if !strings.Contains(got, "Data ID: my-data-id") {
		t.Fatalf("missing data ID header\ngot:\n%s", got)
	}
	if !strings.Contains(got, "Group: my-group") {
		t.Fatalf("missing group header\ngot:\n%s", got)
	}
	if !strings.Contains(got, "key: value") {
		t.Fatalf("missing content\ngot:\n%s", got)
	}
	if !strings.Contains(got, "═══════════════════════════════════════") {
		t.Fatalf("missing separator\ngot:\n%s", got)
	}
}

func TestRenderConfigGetPretty_Empty(t *testing.T) {
	got := captureStdout(t, func() {
		renderConfigGetPretty("d", "g", "")
	})

	if got != "Configuration not found\n" {
		t.Fatalf("got %q, want %q", got, "Configuration not found\n")
	}
}

func TestRenderConfigGetJSON(t *testing.T) {
	got := captureStdout(t, func() {
		renderConfigGetJSON("my-data-id", "my-group", "hello\nworld")
	})

	var parsed map[string]string
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, got)
	}

	if parsed["dataId"] != "my-data-id" {
		t.Errorf("dataId = %q, want %q", parsed["dataId"], "my-data-id")
	}
	if parsed["group"] != "my-group" {
		t.Errorf("group = %q, want %q", parsed["group"], "my-group")
	}
	if parsed["content"] != "hello\nworld" {
		t.Errorf("content = %q, want %q", parsed["content"], "hello\nworld")
	}
}

func TestRenderConfigGetJSON_EmptyContent(t *testing.T) {
	got := captureStdout(t, func() {
		renderConfigGetJSON("d", "g", "")
	})

	var parsed map[string]string
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, got)
	}

	if parsed["dataId"] != "d" {
		t.Errorf("dataId = %q", parsed["dataId"])
	}
	if parsed["group"] != "g" {
		t.Errorf("group = %q", parsed["group"])
	}
	if parsed["content"] != "" {
		t.Errorf("content = %q, want empty", parsed["content"])
	}
}

func TestRenderConfigGetRaw(t *testing.T) {
	t.Run("with content", func(t *testing.T) {
		got := captureStdout(t, func() {
			renderConfigGetRaw("line1\nline2\n")
		})
		if got != "line1\nline2\n" {
			t.Fatalf("got %q, want %q", got, "line1\nline2\n")
		}
	})

	t.Run("without trailing newline", func(t *testing.T) {
		got := captureStdout(t, func() {
			renderConfigGetRaw("no-newline")
		})
		if got != "no-newline" {
			t.Fatalf("got %q, want %q", got, "no-newline")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		got := captureStdout(t, func() {
			renderConfigGetRaw("")
		})
		if got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
}

func TestConfigGetOutputFlagRegistered(t *testing.T) {
	flag := getConfigCmd.Flags().Lookup("output")
	if flag == nil {
		t.Fatal("--output flag not registered on config-get command")
	}
	if flag.DefValue != "pretty" {
		t.Errorf("default = %q, want %q", flag.DefValue, "pretty")
	}
}