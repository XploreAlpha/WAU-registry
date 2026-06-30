# wau-registry 架构(库)

## 模块拆分

```
wau-registry/
├── registry.go              # 主入口(Options + Peer + Register/Get/List)
├── storage/
│   ├── memory.go            # 内存存储
│   └── postgres.go          # Postgres 后端
├── peer.go                  # Peer struct(ID, Type, Addr, Meta)
├── index.go                 # 索引(按 Type / Addr 过滤)
├── ttl.go                   # 失效策略
└── README.md / QUICKSTART.md / DEPLOY.md / ARCHITECTURE.md / CHANGELOG.md
```

## 数据流

```
WAU-core-kernel 内嵌
    ↓ 启动时
registry.New(Options)
    ↓ Sidecar 上线
register.Register(Peer{ID, Type, Addr})
    ↓ Sidecar 心跳
register.Heartbeat(ID)
    ↓ TTL 过期
后台 goroutine 清理
    ↓ 查询时
register.Get(ID) → Peer
```

## 关键决策

| 决策 | 内容 |
|---|---|
| **库 vs 服务** | per [[project-wau-registry-vs-service]] |
| **A2A 兼容** | 读 AgentCard JSON(per [[project-wau-registry-a2a-compat]]) |
| **TTL 失效** | 5 分钟默认心跳 |

## 接口边界

- **入**:`Register` / `Get` / `List` / `Heartbeat` / `Unregister`
- **出**:Peer 对象 / List 数组
- **依赖**:可选 Postgres
- **被依赖**:WAU-core-kernel(嵌入)

## Peer schema

```go
type Peer struct {
    ID       string            // 唯一 ID
    Type     string            // "edge" / "channel" / "llm-router" / ...
    Addr     string            // gRPC 监听地址
    Meta     map[string]string // 扩展 metadata
    RegisterTime time.Time
    LastHeartbeat time.Time
}
```

## 性能预算

| 指标 | 目标 |
|---|---|
| Register/Get P50 | < 0.1 ms |
| List 100 peers | < 1 ms |
| 失效扫描 | 后台 1Hz |

## 跟其他仓的关系

- **上游(用本库)**:WAU-core-kernel
- **下游**:无
- **配套服务**:wau-registry-service
