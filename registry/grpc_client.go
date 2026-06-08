package registry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/wau/registry/registry/registryv1"
)

// GRPCStore 通过 gRPC 调 wau-registry-service
// 这是 v0.4.0 新增,用于替换 RedisStore(直接读 Redis)
// 数据流:
//   kernel ──gRPC──→ wau-registry-service ──Redis──→ 共享 a2a:* keys
type GRPCStore struct {
	conn   *grpc.ClientConn
	client registryv1.RegistryServiceClient
	logger *slog.Logger
	addr   string
}

// NewGRPCStore 创建 gRPC Store(注意:这里取 GRPCStore 不是 GRPCClient,是为了跟 NewRedisStore 命名风格一致)
//
// addr 形如 "localhost:50052" 或 "wau-registry-service:50052"
func NewGRPCStore(addr string, logger *slog.Logger) (*GRPCStore, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// 不带 TLS(内网)
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dial wau-registry-service: %w", err)
	}

	logger.Info("Connected to wau-registry-service via gRPC", "addr", addr)

	return &GRPCStore{
		conn:   conn,
		client: registryv1.NewRegistryServiceClient(conn),
		logger: logger,
		addr:   addr,
	}, nil
}

// Heartbeat 心跳 = 注册 + 保活
func (g *GRPCStore) Heartbeat(ctx context.Context, req *HeartbeatRequest) error {
	// 转为 proto card
	card := &registryv1.RegistryAgentCard{
		Name:        req.Name,
		Description: "", // lib 没有 description
		Url:         req.URL,
		Skills:      req.Skills,
		Version:     req.Version,
		Universes:   []string{}, // lib 的 universe 单值
	}

	// 如果有 universe,加进去
	if req.Universe != "" {
		card.Universes = []string{req.Universe}
	}

	_, err := g.client.RegisterCard(ctx, &registryv1.RegisterCardRequest{
		Card: card,
	})
	if err != nil {
		return fmt.Errorf("gRPC RegisterCard: %w", err)
	}
	return nil
}

// GetAgent 获取单个 Agent 信息
func (g *GRPCStore) GetAgent(ctx context.Context, agentID string) (*AgentCard, error) {
	resp, err := g.client.GetCard(ctx, &registryv1.GetCardRequest{
		Name: agentID,
	})
	if err != nil {
		return nil, fmt.Errorf("gRPC GetCard: %w", err)
	}
	if !resp.Found || resp.Card == nil {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}
	return g.protoToLibCard(resp.Card), nil
}

// GetAgents 获取所有 Agent
func (g *GRPCStore) GetAgents(ctx context.Context) ([]*AgentCard, error) {
	resp, err := g.client.GetCards(ctx, &registryv1.GetCardsRequest{})
	if err != nil {
		return nil, fmt.Errorf("gRPC GetCards: %w", err)
	}
	return g.protoToLibCards(resp.Cards), nil
}

// GetOnlineAgents 获取在线 Agent 列表
// gRPC 端 GetCards 返回的 card 都默认 90s TTL,等同于"在线"
func (g *GRPCStore) GetOnlineAgents(ctx context.Context) ([]*AgentCard, error) {
	resp, err := g.client.GetCards(ctx, &registryv1.GetCardsRequest{})
	if err != nil {
		return nil, fmt.Errorf("gRPC GetCards: %w", err)
	}
	cards := g.protoToLibCards(resp.Cards)

	// 标记为在线(gRPC 没有 Online 字段,默认 true)
	for _, c := range cards {
		c.Online = true
		c.LastSeen = time.Now().Unix()
	}
	return cards, nil
}

// GetLoad 获取 Agent 负载信息
// wau-registry-service v0.3.0 没实现 load,返回 not implemented
func (g *GRPCStore) GetLoad(ctx context.Context, agentID string) (*AgentLoad, error) {
	return nil, fmt.Errorf("GetLoad not implemented in wau-registry-service v0.3.0")
}

// GetStatus 获取 Agent 综合状态 (Card + Load)
func (g *GRPCStore) GetStatus(ctx context.Context, agentID string) (*AgentStatus, error) {
	card, err := g.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	// load 可能没实现,容错
	load, _ := g.GetLoad(ctx, agentID)
	return &AgentStatus{
		Card: card,
		Load: load,
	}, nil
}

// Deregister 注销 Agent
func (g *GRPCStore) Deregister(ctx context.Context, agentID string) error {
	// wau-registry-service 有 DeleteCard 但没暴露在 Registry interface 里
	// 暂时用 GetCards 拿到 ID 调 DeleteCard(没有直接 DeleteCard gRPC,等 v0.4.1 补充)
	// 当前 v0.3.0 proto 只有 GetCard,没 DeleteCard
	// 暂时返回 not implemented
	return fmt.Errorf("Deregister via gRPC not yet implemented (use direct Redis for now)")
}

// Close 关闭连接
func (g *GRPCStore) Close() error {
	return g.conn.Close()
}

// ============================================================
// proto <-> lib 类型转换
// ============================================================

// protoToLibCard 把 proto card 转 lib AgentCard
func (g *GRPCStore) protoToLibCard(p *registryv1.RegistryAgentCard) *AgentCard {
	if p == nil {
		return nil
	}
	card := &AgentCard{
		ID:       p.Name, // lib 用 ID 字段,proto 只有 name
		Name:     p.Name,
		URL:      p.Url,
		Version:  p.Version,
		Skills:   p.Skills,
		Online:   true,
		LastSeen: time.Now().Unix(),
	}

	// universe 单值,取第一个
	if len(p.Universes) > 0 {
		card.Universe = p.Universes[0]
	}

	// last_seen 转换为 unix 时间戳
	if p.LastSeen != nil {
		card.LastSeen = p.LastSeen.Seconds
	}

	return card
}

// protoToLibCards 批量转换
func (g *GRPCStore) protoToLibCards(ps []*registryv1.RegistryAgentCard) []*AgentCard {
	cards := make([]*AgentCard, 0, len(ps))
	for _, p := range ps {
		if c := g.protoToLibCard(p); c != nil {
			cards = append(cards, c)
		}
	}
	return cards
}
