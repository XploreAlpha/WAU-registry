# wau-registry

> WAU 网络的注册中心模块 - 基于心跳的 Agent 注册与发现

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat-square&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)

---

## 核心设计

**心跳即注册** - 心跳不仅保活，同时完成注册动作。

```
传统设计：
注册 ──→ 心跳 ──→ 在线 (分开的动作)

wau-registry 设计：
心跳 ──→ 注册 + 保活 (同一个动作)
         第一次心跳 = 注册
         后续心跳 = 刷新 TTL
         停止心跳 = 自动下线
```

---

## 核心概念

| 概念 | 说明 |
|------|------|
| **心跳** | Agent 定期发送，刷新 TTL 表示存活 |
| **注册** | 首次心跳时创建 Agent Card |
| **下线** | TTL 过期自动从 Redis 删除 |
| **负载** | Agent 实时 CPU/内存/任务数 |

---

## 接口设计

```go
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

    // GetStatus 获取 Agent 综合状态
    GetStatus(ctx context.Context, agentID string) (*AgentStatus, error)

    // Deregister 注销 Agent
    Deregister(ctx context.Context, agentID string) error

    // Close 关闭连接
    Close() error
}
```

---

## HeartbeatRequest

心跳请求包含 Agent 信息和负载：

```go
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
```

---

## Redis 数据结构

| Key | 类型 | 说明 | TTL |
|-----|------|------|-----|
| `a2a:card-names` | Set | 所有 Agent 名称集合 | 永久 |
| `a2a:cards:{id}` | Hash | Agent Card 数据 | 3 分钟 |
| `wau:load:{id}` | Hash | Agent 负载信息 | 1 分钟 |

### a2a:cards:{id} 数据结构

```json
{
  "id": "benny",
  "name": "Benny",
  "url": "http://benny:8080",
  "version": "1.0.0",
  "skills": "shopping,payment",
  "universe": "default",
  "firstSeen": 1715260800,
  "lastSeen": 1715261100,
  "online": "true"
}
```

### wau:load:{id} 数据结构

```json
{
  "agentId": "benny",
  "activeTasks": 2,
  "maxCapacity": 10,
  "cpuUsage": "0.3",
  "memoryUsage": "0.5",
  "lastSeen": 1715261100
}
```

---

## 使用示例

```go
// 创建 Registry
client := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})
registry := registry.NewRedisStore(client, logger)

// Agent 发送心跳 (首次自动注册)
err := registry.Heartbeat(ctx, &registry.HeartbeatRequest{
    AgentID:     "benny",
    Name:        "Benny",
    URL:         "http://benny:8080",
    Version:     "1.0.0",
    Skills:      []string{"shopping", "payment"},
    ActiveTasks: 2,
    MaxCapacity: 10,
    CPUUsage:    0.3,
    MemoryUsage: 0.5,
})

// 查询在线 Agent
agents, err := registry.GetOnlineAgents(ctx)

// 获取单个 Agent 状态
status, err := registry.GetStatus(ctx, "benny")
```

---

## 心跳流程

```
Agent 启动
    │
    │ 发送心跳 (包含完整 AgentCard 信息)
    │ POST /heartbeat
    │
    ▼
wau-registry.Heartbeat()
    │
    ├── 首次心跳？
    │   ├── 是 → 创建 a2a:cards:{id}，设置 firstSeen
    │   └── 否 → 更新时间戳
    │
    ├── 保存 Card 数据 + 设置 3 分钟 TTL
    ├── 保存负载数据 + 设置 1 分钟 TTL
    └── 更新 a2a:card-names 集合
            │
            ▼
    返回成功

Agent 正常运行中...
    │
    │ 每 30 秒发送一次心跳
    │ 刷新 TTL (保持在线)
    │
    ▼

Agent 停止发送心跳
    │
    ▼
3 分钟后 TTL 过期
    │
    ▼
Redis 自动删除 Card → Agent 下线
```

---

## 与 wau-core-kernel 的关系

```
wau-core-kernel (主项目)
    │
    └── wau-registry (独立模块)
            │
            ├── 提供 Agent 注册发现
            └── 提供心跳管理
```

wau-core-kernel 通过 Go Module 引用：

```go
import "github.com/wau/registry"

registry.Heartbeat(ctx, req)
```

---

## 项目结构

```
wau-registry/
├── registry/
│   ├── types.go          # 接口和数据结构
│   └── redis.go         # Redis 实现
├── go.mod
└── README.md
```

---

## License

MIT License - 详见 [LICENSE](LICENSE) 文件
