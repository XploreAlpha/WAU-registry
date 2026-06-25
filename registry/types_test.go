package registry

import (
	"encoding/json"
	"testing"
)

// TestHeartbeatRequest_UniverseLabels_JSON 验证 HeartbeatRequest.UniverseLabels
// 字段的 JSON 序列化 / 反序列化(M5-1 B.3 引入)
//
// 覆盖:
//   1. 包含 UniverseLabels 时,JSON 含 "universeLabels" 字段
//   2. omitempty:空 map 不出现在 JSON
//   3. 反序列化:老 agent 不填此字段 = nil map(向后兼容)
func TestHeartbeatRequest_UniverseLabels_JSON(t *testing.T) {
	t.Run("round-trip with all 6 reserved labels", func(t *testing.T) {
		req := HeartbeatRequest{
			AgentID: "test-agent",
			URL:     "http://test:8080",
			UniverseLabels: map[string]string{
				"region":         "us-west-1",
				"gpu":            "true",
				"tier":           "high-performance",
				"security_level": "trusted",
				"load":           "low",
				"universe_role":  "business",
			},
		}
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got HeartbeatRequest
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(got.UniverseLabels) != 6 {
			t.Errorf("expected 6 labels, got %d (%v)", len(got.UniverseLabels), got.UniverseLabels)
		}
		for k, v := range req.UniverseLabels {
			if got.UniverseLabels[k] != v {
				t.Errorf("label %s: got %q, want %q", k, got.UniverseLabels[k], v)
			}
		}
	})

	t.Run("omitempty: nil labels omitted from JSON", func(t *testing.T) {
		req := HeartbeatRequest{AgentID: "x"}
		data, _ := json.Marshal(req)
		// 验证 JSON 不含 "universeLabels" 字段(omitempty 生效)
		if containsBytes(data, []byte("universeLabels")) {
			t.Errorf("expected universeLabels omitted, got JSON: %s", data)
		}
	})

	t.Run("backward compat: legacy heartbeat without field decodes to nil", func(t *testing.T) {
		// 模拟 v0.7.x 老 agent 发的 JSON(无 universeLabels 字段)
		legacy := `{"agentId":"legacy","name":"legacy","url":"http://x:8080","version":"v0.7.1","skills":[]}`
		var got HeartbeatRequest
		if err := json.Unmarshal([]byte(legacy), &got); err != nil {
			t.Fatalf("unmarshal legacy: %v", err)
		}
		if got.UniverseLabels != nil {
			t.Errorf("expected nil UniverseLabels for legacy, got %v", got.UniverseLabels)
		}
	})
}

// TestAgentCard_UniverseLabels_JSON 验证 AgentCard.UniverseLabels 字段
func TestAgentCard_UniverseLabels_JSON(t *testing.T) {
	t.Run("persisted labels survive round-trip", func(t *testing.T) {
		card := AgentCard{
			ID: "agent-1", Name: "agent-1", URL: "http://x", Version: "v0.8.0",
			Universe: "us-west-1",
			UniverseLabels: map[string]string{
				"region": "us-west-1",
				"tier":   "high-performance",
			},
		}
		data, _ := json.Marshal(card)
		var got AgentCard
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.UniverseLabels["region"] != "us-west-1" {
			t.Errorf("region label lost: %v", got.UniverseLabels)
		}
		if got.UniverseLabels["tier"] != "high-performance" {
			t.Errorf("tier label lost: %v", got.UniverseLabels)
		}
	})

	t.Run("backward compat: v0.7.x card decodes to nil labels", func(t *testing.T) {
		legacy := `{"id":"a","name":"a","url":"http://x","version":"v0.7.1","firstSeen":0,"lastSeen":0}`
		var got AgentCard
		if err := json.Unmarshal([]byte(legacy), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.UniverseLabels != nil {
			t.Errorf("expected nil UniverseLabels for v0.7.x card, got %v", got.UniverseLabels)
		}
	})
}

// TestUniverseLabels_6ReservedKeys 验证 6 reserved keys 序列化字段名跟 kernel 一致
//
// kernel internal/registry/universe_labels.go 的 ReservedLabelKeys 是 source of truth
// wau-registry 不应漂移
func TestUniverseLabels_6ReservedKeys(t *testing.T) {
	req := HeartbeatRequest{
		UniverseLabels: map[string]string{
			"region":         "x",
			"gpu":            "true",
			"tier":           "low",
			"security_level": "trusted",
			"load":           "idle",
			"universe_role":  "business",
		},
	}
	data, _ := json.Marshal(req)
	for _, k := range []string{"region", "gpu", "tier", "security_level", "load", "universe_role"} {
		if !containsBytes(data, []byte(`"`+k+`"`)) {
			t.Errorf("expected reserved key %q in JSON: %s", k, data)
		}
	}
}

// containsBytes helper — strings.Contains 风格但用 []byte 避免 import
func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
