# wau-registry 部署

> wau-registry 是 Go 库,**无独立服务**。生产集成方式 = WAU-core-kernel `go.mod` 引入 + 调用 `registry.New(...)`。

## 集成步骤

```go
// 1. 在 go.mod 中加
// require github.com/XploreAlpha/wau-registry v0.9.0

// 2. 在 WAU-core-kernel/internal/server/main.go 启动时
reg := registry.New(registry.Options{
    Storage: storage.NewMemory(), // 生产可换 Postgres
    TTL:     5 * time.Minute,
})
```

## 持久化

默认 in-memory。生产用 Postgres 后端:

```go
import "github.com/XploreAlpha/wau-registry/storage/postgres"

reg := registry.New(registry.Options{
    Storage: postgres.New(postgres.DSN{
        DSN: $REGISTRY_DSN, // env 占位,per [[feedback-redis-password-leak-2026-06-21]]
    }),
})
```

## 进程管理

不适用(库)。

## 配置

| 字段 | 默认 | 说明 |
|---|---|---|
| `TTL` | `5m` | peer 失效时间 |
| `Storage` | `memory` | `memory` / `postgres` |

## 升级路径

- v0.9.0(Acorn)→ v0.8.0(Sprout):
  - registry API 100% 兼容
  - 已注册的 peer 状态不迁移(内存型)

## 配套

- 独立服务版本 [wau-registry-service](../wau-registry-service/) — **可选**,通过 RPC 暴露同能力
- 大多数部署用本库即可(service 版适合多实例共享注册中心)
