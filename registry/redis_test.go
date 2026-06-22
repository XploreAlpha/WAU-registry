package registry

import (
	"testing"
)

// TestParseEndpointsJSON 验证 Redis HASH 里的 endpoints 字段能正确反序列化
//
// v0.8.0 M1-2:端点是结构化数据(JSON),不是简单字符串
func TestParseEndpointsJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantNil  bool
		wantLen  int
		wantProt string
	}{
		{
			name:    "empty",
			input:   "",
			wantNil: true,
		},
		{
			name:     "single a2a",
			input:    `[{"protocol":"a2a","url":"http://x:8080"}]`,
			wantLen:  1,
			wantProt: "a2a",
		},
		{
			name:    "multi protocol",
			input:   `[{"protocol":"a2a","url":"http://x:8080"},{"protocol":"afp","url":"http://x:9000"}]`,
			wantLen: 2,
		},
		{
			name:    "corrupt json",
			input:   `[{broken`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEndpoints(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if len(got) != tt.wantLen {
				t.Fatalf("len: want %d, got %d", tt.wantLen, len(got))
			}
			if tt.wantProt != "" && got[0].Protocol != tt.wantProt {
				t.Fatalf("protocol: want %s, got %s", tt.wantProt, got[0].Protocol)
			}
		})
	}
}

// TestParseProtocols 验证逗号分隔的协议列表
func TestParseProtocols(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantNil bool
		want    []string
	}{
		{name: "empty", input: "", wantNil: true},
		{name: "single", input: "a2a", want: []string{"a2a"}},
		{name: "multi", input: "a2a,afp,mcp", want: []string{"a2a", "afp", "mcp"}},
		{name: "trailing comma", input: "a2a,afp,", want: []string{"a2a", "afp"}},
		{name: "leading comma", input: ",a2a,afp", want: []string{"a2a", "afp"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseProtocols(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len: want %d, got %d (%v)", len(tt.want), len(got), got)
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Fatalf("[%d]: want %s, got %s", i, w, got[i])
				}
			}
		})
	}
}

// TestMarshalEndpoints 验证 endpoints 序列化
func TestMarshalEndpoints(t *testing.T) {
	eps := []Endpoint{
		{Protocol: "a2a", URL: "http://x:8080"},
		{Protocol: "afp", URL: "http://x:9000", Tenant: "tenant-a"},
	}

	s, err := marshalEndpoints(eps)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if s == "" {
		t.Fatalf("empty result")
	}

	// 反序列化验证
	roundtrip := parseEndpoints(s)
	if len(roundtrip) != 2 {
		t.Fatalf("roundtrip len: want 2, got %d", len(roundtrip))
	}
	if roundtrip[0].Protocol != "a2a" || roundtrip[0].URL != "http://x:8080" {
		t.Fatalf("roundtrip[0]: %+v", roundtrip[0])
	}
	if roundtrip[1].Protocol != "afp" || roundtrip[1].Tenant != "tenant-a" {
		t.Fatalf("roundtrip[1]: %+v", roundtrip[1])
	}
}

// TestJoinStrings 验证通用 join 工具
func TestJoinStrings(t *testing.T) {
	tests := []struct {
		input []string
		sep   string
		want  string
	}{
		{[]string{}, ",", ""},
		{[]string{"a"}, ",", "a"},
		{[]string{"a", "b"}, ",", "a,b"},
		{[]string{"a", "b", "c"}, "|", "a|b|c"},
	}

	for _, tt := range tests {
		got := joinStrings(tt.input, tt.sep)
		if got != tt.want {
			t.Fatalf("%v + %q: want %q, got %q", tt.input, tt.sep, tt.want, got)
		}
	}
}

// TestAgentCardBackwardCompat 验证老 AgentCard (无 Protocols/Endpoints) 仍能正常解析
//
// v0.8.0 M1-2 向后兼容保证:wau-registry 升级前的数据不会破
func TestAgentCardBackwardCompat(t *testing.T) {
	// 模拟 Redis HASH(老格式,只有基础字段)
	data := map[string]string{
		"id":        "benny",
		"name":      "Benny",
		"url":       "http://benny:8080",
		"version":   "1.0.0",
		"skills":    "chat,task",
		"universe":  "general",
		"firstSeen": "1000",
		"lastSeen":  "2000",
		"online":    "true",
		// 故意不填 protocols / endpoints
	}

	card := parseAgentCard(data)
	if card == nil {
		t.Fatal("card is nil")
	}
	if card.URL != "http://benny:8080" {
		t.Fatalf("URL: got %q", card.URL)
	}
	if len(card.Protocols) != 0 {
		t.Fatalf("Protocols should be empty, got %v", card.Protocols)
	}
	if len(card.Endpoints) != 0 {
		t.Fatalf("Endpoints should be empty, got %v", card.Endpoints)
	}
}

// TestAgentCardWithMultiProtocol 验证新字段能正确解析
func TestAgentCardWithMultiProtocol(t *testing.T) {
	data := map[string]string{
		"id":         "benny",
		"name":       "Benny",
		"url":        "http://benny:8080",          // 老字段
		"protocols":  "a2a,afp",                     // 新字段
		"endpoints":  `[{"protocol":"a2a","url":"http://benny:8080"},{"protocol":"afp","url":"http://benny:9000"}]`,
		"firstSeen":  "1000",
		"lastSeen":   "2000",
		"online":     "true",
	}

	card := parseAgentCard(data)
	if card == nil {
		t.Fatal("card is nil")
	}
	if len(card.Protocols) != 2 {
		t.Fatalf("Protocols: want 2, got %d (%v)", len(card.Protocols), card.Protocols)
	}
	if len(card.Endpoints) != 2 {
		t.Fatalf("Endpoints: want 2, got %d (%v)", len(card.Endpoints), card.Endpoints)
	}
	if card.Endpoints[1].Protocol != "afp" || card.Endpoints[1].URL != "http://benny:9000" {
		t.Fatalf("afp endpoint wrong: %+v", card.Endpoints[1])
	}
}