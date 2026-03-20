package agentspec

import (
	"encoding/json"
	"testing"
)

func TestAgentSpecListItem_UnmarshalJSON(t *testing.T) {
	// Test that nullable fields are handled correctly
	jsonData := `{
		"name": "test-worker",
		"description": null,
		"enable": true,
		"labels": {},
		"editingVersion": "v1",
		"reviewingVersion": null,
		"onlineCnt": 0,
		"updateTime": 1773990903499
	}`

	var item AgentSpecListItem
	if err := json.Unmarshal([]byte(jsonData), &item); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if item.Name != "test-worker" {
		t.Errorf("Expected name 'test-worker', got %q", item.Name)
	}
	if item.Description != nil {
		t.Errorf("Expected Description to be nil, got %v", *item.Description)
	}
	if !item.Enable {
		t.Error("Expected Enable to be true")
	}
	if item.EditingVersion == nil || *item.EditingVersion != "v1" {
		t.Errorf("Expected EditingVersion 'v1', got %v", item.EditingVersion)
	}
	if item.ReviewingVersion != nil {
		t.Errorf("Expected ReviewingVersion to be nil, got %v", item.ReviewingVersion)
	}
	if item.OnlineCnt != 0 {
		t.Errorf("Expected OnlineCnt 0, got %d", item.OnlineCnt)
	}
	if item.UpdateTime != 1773990903499 {
		t.Errorf("Expected UpdateTime 1773990903499, got %d", item.UpdateTime)
	}
}

func TestAgentSpecResource_Metadata(t *testing.T) {
	// Test that metadata can contain mixed types
	jsonData := `{
		"name": "app.yaml",
		"type": "config",
		"content": "key: value",
		"metadata": {
			"encoding": "base64",
			"uniformId": 1773990990177,
			"size": "1024"
		}
	}`

	var res AgentSpecResource
	if err := json.Unmarshal([]byte(jsonData), &res); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if res.Name != "app.yaml" {
		t.Errorf("Expected name 'app.yaml', got %q", res.Name)
	}
	if res.Type != "config" {
		t.Errorf("Expected type 'config', got %q", res.Type)
	}
	if res.Content != "key: value" {
		t.Errorf("Expected content 'key: value', got %q", res.Content)
	}

	if res.Metadata == nil {
		t.Fatal("Expected Metadata to be non-nil")
	}

	if enc, ok := res.Metadata["encoding"]; !ok || enc != "base64" {
		t.Errorf("Expected encoding 'base64', got %v", enc)
	}

	if uid, ok := res.Metadata["uniformId"]; !ok || uid != float64(1773990990177) {
		t.Errorf("Expected uniformId 1773990990177, got %v", uid)
	}

	if size, ok := res.Metadata["size"]; !ok || size != "1024" {
		t.Errorf("Expected size '1024', got %v", size)
	}
}
