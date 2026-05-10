package registry

import (
	"context"
	"time"
)

// HeartbeatRequest 心跳请求
// 心跳 = 注册 + 保活 (同一个动作)
type HeartbeatRequest struct {
	AgentID      string   `json:"agentId"`
	Name         string   `json:"name"`
	URL          string   `json:"url"`
	Version      string   `json:"version"`
	Skills       []string `json:"skills"`
	Universe     string   `json:"universe,omitempty"`

	// 负载信息
	ActiveTasks  int      `json:"activeTasks"`
	MaxCapacity  int      `json:"maxCapacity"`
	CPUUsage     float64  `json:"cpuUsage"`
	MemoryUsage  float64  `json:"memoryUsage"`
}

// AgentCard Agent 注册信息
type AgentCard struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	Version     string   `json:"version"`
	Skills      []string `json:"skills"`
	Universe    string   `json:"universe,omitempty"`
	FirstSeen   int64    `json:"firstSeen"`
	LastSeen    int64    `json:"lastSeen"`
	Online      bool     `json:"online"`
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
