# wau-registry 15 分钟跑通

> 目标:作为 Go 库嵌入 WAU-core-kernel + 注册 1 个 peer + 验证查询。

## 前置

- Go 1.21+
- 端口:无独立端口(库,不启服务)

## 步骤

### 1. 拉源码

```bash
cd ~/project/wau-registry
git pull origin main
go build ./...
```

### 2. 在 WAU-core-kernel 侧嵌入

```go
import "github.com/XploreAlpha/wau-registry"

reg := registry.New(registry.Options{
    Storage: storage.NewMemory(),
    TTL:     5 * time.Minute,
})

// 注册 peer
reg.Register(registry.Peer{
    ID:   "wau-edge-1",
    Type: "edge",
    Addr: "127.0.0.1:18401",
    Meta: map[string]string{"tunnel_id": "t-001"},
})

// 查询
peer, err := reg.Get("wau-edge-1")
```

### 3. 验证生命周期

peer 注册后 5 分钟无心跳自动失效。

## 下一步

- [DEPLOY.md](DEPLOY.md) — 作为 WAU-core-kernel 子模块
- [ARCHITECTURE.md](ARCHITECTURE.md) — 索引 + 失效策略
- [README.md](README.md) — v0.9.0 收口段
