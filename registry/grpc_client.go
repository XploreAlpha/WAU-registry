package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// HTTPStore 通过 HTTP 调 wau-registry-service
// 这是 v0.4.0 新增,用于替换 RedisStore(直接读 Redis)
//
// 设计选择:虽然叫 GRPCStore(为了跟 NewRedisStore 命名风格一致),
// 实际用 HTTP 是因为:
//   1. wau-registry 库不能 import wau-registry-service 的 internal proto(Go internal 规则)
//   2. 自己 copy proto 会跟 kernel 自己的 proto 冲突
//   3. HTTP 更简单,无 protobuf 注册问题
//
// 数据流:
//   kernel → HTTP → wau-registry-service → Redis
type HTTPStore struct {
	httpClient *http.Client
	baseURL    string
	logger     *slog.Logger
}

// HTTPClient HTTP 端点
// 注意:grpc_client.go 改用 HTTP 后,这个名字仅供 NewGRPCStore 内部用
// v0.4.1+ 计划:用 gRPC + 共享 proto(等 wau-registry-proto 独立 repo 创建)
type HTTPClient struct {
	BaseURL string
}

// HTTPStore 别名(为了跟 NewRedisStore 命名风格统一)
type HTTPStoreAlias = HTTPStore

// NewGRPCStore 创建 HTTP Store(内部用 HTTP 调 wau-registry-service)
//
// addr 形如 "localhost:18401" (HTTP 端口,不是 gRPC 的 50052)
// 保留 GRPCStore 名字是为了不破坏 main.go 的调用
func NewGRPCStore(addr string, logger *slog.Logger) (*HTTPStore, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// 如果用户传的是 gRPC 端口 :50052,自动改成 HTTP :18401
	// 因为 gRPC 端没法用(避免 proto 冲突)
	// 简化处理:直接拼 http://
	if len(addr) > 0 && addr[0] == ':' {
		addr = "localhost" + addr
	}
	baseURL := "http://" + addr

	logger.Info("Connected to wau-registry-service via HTTP", "url", baseURL)

	return &HTTPStore{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		baseURL:    baseURL,
		logger:     logger,
	}, nil
}

// agentCardJSON 内部 JSON schema(跟 wau-registry-service HTTP API 一致)
type agentCardJSON struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	URL         string   `json:"url"`
	Skills      []string `json:"skills"`
	Universes   []string `json:"universes"`
	Version     string   `json:"version"`
}

// Heartbeat 心跳 = 注册 + 保活
func (h *HTTPStore) Heartbeat(ctx context.Context, req *HeartbeatRequest) error {
	card := agentCardJSON{
		Name:    req.Name,
		URL:     req.URL,
		Skills:  req.Skills,
		Version: req.Version,
	}
	if req.Universe != "" {
		card.Universes = []string{req.Universe}
	}

	resp, err := h.post(ctx, "/registry/agents", card)
	if err != nil {
		return fmt.Errorf("HTTP POST /registry/agents: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// GetAgent 获取单个 Agent 信息
func (h *HTTPStore) GetAgent(ctx context.Context, agentID string) (*AgentCard, error) {
	resp, err := h.get(ctx, "/registry/agents/"+agentID)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET /registry/agents/%s: %w", agentID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var card agentCardJSON
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return h.jsonToLibCard(&card), nil
}

// GetAgents 获取所有 Agent
func (h *HTTPStore) GetAgents(ctx context.Context) ([]*AgentCard, error) {
	return h.GetOnlineAgents(ctx) // registry-service 没有 GetAgents 跟 GetOnlineAgents 区分
}

// GetOnlineAgents 获取在线 Agent 列表
// HTTP 端 GET /registry/agents 返回 {"cards": [...], "total": N}
func (h *HTTPStore) GetOnlineAgents(ctx context.Context) ([]*AgentCard, error) {
	resp, err := h.get(ctx, "/registry/agents")
	if err != nil {
		return nil, fmt.Errorf("HTTP GET /registry/agents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var response struct {
		Cards []agentCardJSON `json:"cards"`
		Total int             `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	cards := make([]*AgentCard, 0, len(response.Cards))
	now := time.Now().Unix()
	for i := range response.Cards {
		c := h.jsonToLibCard(&response.Cards[i])
		c.Online = true
		c.LastSeen = now
		cards = append(cards, c)
	}
	return cards, nil
}

// GetLoad 获取 Agent 负载信息
// wau-registry-service v0.3.0 没实现 load,返回 not implemented
func (h *HTTPStore) GetLoad(ctx context.Context, agentID string) (*AgentLoad, error) {
	return nil, fmt.Errorf("GetLoad not implemented in wau-registry-service v0.3.0")
}

// GetStatus 获取 Agent 综合状态 (Card + Load)
func (h *HTTPStore) GetStatus(ctx context.Context, agentID string) (*AgentStatus, error) {
	card, err := h.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	// load 暂未实现,容错
	load, _ := h.GetLoad(ctx, agentID)
	return &AgentStatus{
		Card: card,
		Load: load,
	}, nil
}

// Deregister 注销 Agent
// wau-registry-service v0.3.0 没暴露 DELETE HTTP 端点
func (h *HTTPStore) Deregister(ctx context.Context, agentID string) error {
	return fmt.Errorf("Deregister via HTTP not yet implemented in wau-registry-service v0.3.0")
}

// Close 关闭连接(HTTP 无状态,无需真正关闭)
func (h *HTTPStore) Close() error {
	return nil
}

// ============================================================
// HTTP 辅助
// ============================================================

func (h *HTTPStore) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", h.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	return h.httpClient.Do(req)
}

func (h *HTTPStore) post(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", h.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return h.httpClient.Do(req)
}

// ============================================================
// JSON → lib 类型转换
// ============================================================

func (h *HTTPStore) jsonToLibCard(j *agentCardJSON) *AgentCard {
	if j == nil {
		return nil
	}
	card := &AgentCard{
		ID:      j.Name, // lib 用 ID,proto 只有 name
		Name:    j.Name,
		URL:     j.URL,
		Version: j.Version,
		Skills:  j.Skills,
		Online:  true,
	}
	if len(j.Universes) > 0 {
		card.Universe = j.Universes[0]
	}
	return card
}

// 防止 io 未使用(go vet 友好)
var _ = io.EOF
