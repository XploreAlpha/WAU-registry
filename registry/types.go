package registry

import (
	"context"
	"time"
)

// Endpoint 单协议端点(v0.8.0 M1-2 引入)
//
// 一个 agent 可以声明多个 endpoint,每个走不同协议。
// 典型场景:agent 同时支持 A2A (JSON-RPC) 和 AFP (HTTP+JSON),
// SDK 走 A2A 调对话,AFP 走大文件 / 流式。
//
// 向后兼容:老 agent 不填 Endpoints,SessionManager 走 URL fallback。
type Endpoint struct {
	Protocol string `json:"protocol"`           // "a2a" / "afp" / "mcp" / ...
	URL      string `json:"url"`                // 该协议的端点 URL
	Tenant   string `json:"tenant,omitempty"`   // AFP 等需要 universe 路由时填
}

// HeartbeatRequest 心跳请求
// 心跳 = 注册 + 保活 (同一个动作)
type HeartbeatRequest struct {
	AgentID  string   `json:"agentId"`
	Name     string   `json:"name"`
	URL      string   `json:"url"` // 保留:默认 A2A 端点(向后兼容)
	Version  string   `json:"version"`
	Skills   []string `json:"skills"`
	Universe string   `json:"universe,omitempty"`

	// 多协议声明(v0.8.0 M1-2 新增)
	// Protocols:agent 声明支持的协议列表(如 ["a2a", "afp"])
	// Endpoints:每个协议对应的 URL(必填 Protocols 中每个都有)
	// 老 agent 不填这两字段,fallback 到 URL 当 A2A。
	Protocols []string   `json:"protocols,omitempty"`
	Endpoints []Endpoint `json:"endpoints,omitempty"`

	// v0.8.0 M4-3 hotfix 2: lineage(自繁衍)
	// 空 = 顶级 agent(老 agent 或主动注册);非空 = 该 agent 是 ParentAgentID 的 child
	// kernel.ReplicateAgent 在 M4-3 繁衍子 agent 时填,跨重启可恢复 lineage
	ParentAgentID string `json:"parentAgentId,omitempty"`

	// 负载信息
	ActiveTasks int     `json:"activeTasks"`
	MaxCapacity int     `json:"maxCapacity"`
	CPUUsage    float64 `json:"cpuUsage"`
	MemoryUsage float64 `json:"memoryUsage"`
}

// AgentCard Agent 注册信息
type AgentCard struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	URL      string   `json:"url"` // 保留:默认 A2A 端点(向后兼容)
	Version  string   `json:"version"`
	Skills   []string `json:"skills"`
	Universe string   `json:"universe,omitempty"`
	FirstSeen int64   `json:"firstSeen"`
	LastSeen  int64   `json:"lastSeen"`
	Online    bool    `json:"online"`

	// 多协议声明(v0.8.0 M1-2 新增)
	// Protocols:声明支持的协议列表
	// Endpoints:每个协议对应的 URL
	Protocols []string   `json:"protocols,omitempty"`
	Endpoints []Endpoint `json:"endpoints,omitempty"`

	// v0.8.0 M4-3 hotfix 2: lineage(自繁衍)
	// 空 = 顶级 agent;非空 = child of ParentAgentID
	ParentAgentID string `json:"parentAgentId,omitempty"`
}

// AgentLoad Agent 负载信息
type AgentLoad struct {
	AgentID     string  `json:"agentId"`
	ActiveTasks int     `json:"activeTasks"`
	MaxCapacity int     `json:"maxCapacity"`
	CPUUsage    float64 `json:"cpuUsage"`
	MemoryUsage float64 `json:"memoryUsage"`
	LastSeen    int64   `json:"lastSeen"`
}

// AgentStatus Agent 综合状态
type AgentStatus struct {
	Card *AgentCard `json:"card"`
	Load *AgentLoad  `json:"load"`
}

// Registry 注册中心接口
// 核心设计：心跳即注册
type Registry interface {
	// Heartbeat 心跳 = 注册 + 保活
	// 第一次心跳创建注册信息，后续心跳刷新 TTL
	Heartbeat(ctx context.Context, req *HeartbeatRequest) error

	// GetAgent 获取 Agent 信息
	GetAgent(ctx context.Context, agentID string) (*AgentCard, error)

	// GetAgents 获取所有 Agent 信息
	GetAgents(ctx context.Context) ([]*AgentCard, error)

	// GetOnlineAgents 获取在线 Agent 列表
	GetOnlineAgents(ctx context.Context) ([]*AgentCard, error)

	// GetLoad 获取 Agent 负载信息
	GetLoad(ctx context.Context, agentID string) (*AgentLoad, error)

	// GetStatus 获取 Agent 综合状态 (Card + Load)
	GetStatus(ctx context.Context, agentID string) (*AgentStatus, error)

	// Deregister 注销 Agent
	Deregister(ctx context.Context, agentID string) error

	// Close 关闭连接
	Close() error
}

// 默认 TTL 配置
const (
	// CardTTL Agent Card 的 TTL，超过时间没心跳自动过期
	CardTTL = 3 * time.Minute

	// LoadTTL 负载信息的 TTL
	LoadTTL = 1 * time.Minute
)
