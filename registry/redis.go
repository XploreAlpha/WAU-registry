package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore Redis 实现
type RedisStore struct {
	client *redis.Client
	logger *slog.Logger
}

// NewRedisStore 创建 Redis 注册中心
func NewRedisStore(client *redis.Client, logger *slog.Logger) *RedisStore {
	return &RedisStore{
		client: client,
		logger: logger,
	}
}

// Heartbeat 心跳 = 注册 + 保活
func (r *RedisStore) Heartbeat(ctx context.Context, req *HeartbeatRequest) error {
	now := time.Now().Unix()

	// 1. 构建 Agent Card 数据
	cardKey := fmt.Sprintf("a2a:cards:%s", req.AgentID)

	// 检查是否首次注册
	exists, err := r.client.Exists(ctx, cardKey).Result()
	if err != nil {
		return fmt.Errorf("check card exists: %w", err)
	}

	firstSeen := now
	if exists == 0 {
		// 首次注册，记录时间
		firstSeen = now
		r.logger.Info("New agent registered", "agent", req.AgentID)
	} else {
		// 获取原有首次注册时间
		if original, err := r.client.HGet(ctx, cardKey, "firstSeen").Int64(); err == nil {
			firstSeen = original
		}
	}

	// 2. 保存 Card 数据
	cardData := map[string]interface{}{
		"id":        req.AgentID,
		"name":      req.Name,
		"url":       req.URL,
		"version":   req.Version,
		"skills":    joinSkills(req.Skills),
		"universe":   req.Universe,
		"firstSeen":  firstSeen,
		"lastSeen":   now,
		"online":     true,
	}

	// v0.8.0 M1-2:多协议声明
	if len(req.Protocols) > 0 {
		cardData["protocols"] = joinStrings(req.Protocols, ",")
	}
	if len(req.Endpoints) > 0 {
		// Endpoints 是结构化数据,JSON 序列化存 Redis HASH
		// 单个 field 存完整 JSON,反序列化时 parse
		if epJSON, err := marshalEndpoints(req.Endpoints); err == nil {
			cardData["endpoints"] = epJSON
		} else {
			r.logger.Warn("failed to marshal endpoints, skipping", "agent", req.AgentID, "error", err)
		}
	}

	// v0.8.0 M4-3 hotfix 2: lineage(自繁衍)
	// 仅在非空时写(omitempty,顶级 agent 不占 Redis 存储)
	if req.ParentAgentID != "" {
		cardData["parentAgentId"] = req.ParentAgentID
	}

	pipe := r.client.Pipeline()

	// 设置 Card 数据
	pipe.HSet(ctx, cardKey, cardData)

	// 设置 TTL (3分钟)
	pipe.Expire(ctx, cardKey, CardTTL)

	// 3. 保存负载数据
	loadKey := fmt.Sprintf("wau:load:%s", req.AgentID)
	loadData := map[string]interface{}{
		"agentId":     req.AgentID,
		"activeTasks": req.ActiveTasks,
		"maxCapacity": req.MaxCapacity,
		"cpuUsage":    req.CPUUsage,
		"memoryUsage": req.MemoryUsage,
		"lastSeen":    now,
	}
	pipe.HSet(ctx, loadKey, loadData)
	pipe.Expire(ctx, loadKey, LoadTTL)

	// 4. 更新在线列表
	pipe.SAdd(ctx, "a2a:card-names", req.AgentID)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("heartbeat exec: %w", err)
	}

	return nil
}

// GetAgent 获取 Agent 信息
func (r *RedisStore) GetAgent(ctx context.Context, agentID string) (*AgentCard, error) {
	cardKey := fmt.Sprintf("a2a:cards:%s", agentID)

	data, err := r.client.HGetAll(ctx, cardKey).Result()
	if err != nil {
		return nil, fmt.Errorf("get card: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	return parseAgentCard(data), nil
}

// GetAgents 获取所有 Agent 信息
func (r *RedisStore) GetAgents(ctx context.Context) ([]*AgentCard, error) {
	names, err := r.client.SMembers(ctx, "a2a:card-names").Result()
	if err != nil {
		return nil, fmt.Errorf("get card names: %w", err)
	}

	cards := make([]*AgentCard, 0, len(names))
	for _, name := range names {
		card, err := r.GetAgent(ctx, name)
		if err != nil {
			continue
		}
		cards = append(cards, card)
	}

	return cards, nil
}

// GetOnlineAgents 获取在线 Agent 列表
// 只返回 TTL 未过期的 Agent
func (r *RedisStore) GetOnlineAgents(ctx context.Context) ([]*AgentCard, error) {
	names, err := r.client.SMembers(ctx, "a2a:card-names").Result()
	if err != nil {
		return nil, fmt.Errorf("get card names: %w", err)
	}

	cards := make([]*AgentCard, 0)

	for _, name := range names {
		cardKey := fmt.Sprintf("a2a:cards:%s", name)
		ttl, err := r.client.TTL(ctx, cardKey).Result()
		if err != nil {
			continue
		}

		// TTL > 0 表示在线
		if ttl > 0 {
			data, err := r.client.HGetAll(ctx, cardKey).Result()
			if err != nil || len(data) == 0 {
				continue
			}
			card := parseAgentCard(data)
			card.Online = true
			cards = append(cards, card)
		} else {
			// TTL 已过期，从在线列表移除
			r.client.SRem(ctx, "a2a:card-names", name)
		}
	}

	return cards, nil
}

// GetLoad 获取 Agent 负载信息
func (r *RedisStore) GetLoad(ctx context.Context, agentID string) (*AgentLoad, error) {
	loadKey := fmt.Sprintf("wau:load:%s", agentID)

	data, err := r.client.HGetAll(ctx, loadKey).Result()
	if err != nil {
		return nil, fmt.Errorf("get load: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("load not found: %s", agentID)
	}

	return parseAgentLoad(data), nil
}

// GetStatus 获取 Agent 综合状态
func (r *RedisStore) GetStatus(ctx context.Context, agentID string) (*AgentStatus, error) {
	card, err := r.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}

	load, err := r.GetLoad(ctx, agentID)
	if err != nil {
		load = nil // 负载可能没有
	}

	return &AgentStatus{
		Card: card,
		Load: load,
	}, nil
}

// Deregister 注销 Agent
func (r *RedisStore) Deregister(ctx context.Context, agentID string) error {
	pipe := r.client.Pipeline()

	pipe.Del(ctx, fmt.Sprintf("a2a:cards:%s", agentID))
	pipe.Del(ctx, fmt.Sprintf("wau:load:%s", agentID))
	pipe.SRem(ctx, "a2a:card-names", agentID)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("deregister: %w", err)
	}

	r.logger.Info("Agent deregistered", "agent", agentID)
	return nil
}

// Close 关闭连接
func (r *RedisStore) Close() error {
	return r.client.Close()
}

// 辅助函数

func joinSkills(skills []string) string {
	return joinStrings(skills, ",")
}

// joinStrings 用 sep 连接字符串切片
//
// v0.8.0 M1-2:通用化 join 逻辑,Protocols 字段也复用
func joinStrings(items []string, sep string) string {
	if len(items) == 0 {
		return ""
	}
	result := items[0]
	for i := 1; i < len(items); i++ {
		result += sep + items[i]
	}
	return result
}

// marshalEndpoints 把 []Endpoint 序列化成 JSON 字符串(给 Redis HASH 存)
//
// v0.8.0 M1-2:Endpoints 是结构化数据,不能简单 join,必须 JSON
func marshalEndpoints(eps []Endpoint) (string, error) {
	data, err := json.Marshal(eps)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// parseEndpoints 从 Redis HASH 字符串反序列化 []Endpoint
//
// 失败容错:解析失败 → log warn + 返 nil(降级到 URL fallback,不抛错)
func parseEndpoints(s string) []Endpoint {
	if s == "" {
		return nil
	}
	var eps []Endpoint
	if err := json.Unmarshal([]byte(s), &eps); err != nil {
		return nil
	}
	return eps
}

// parseProtocols 从 Redis HASH 字符串(逗号分隔)反序列化 []string
func parseProtocols(s string) []string {
	if s == "" {
		return nil
	}
	return splitSkills(s) // 复用已有的逗号分隔逻辑
}

func splitSkills(s string) []string {
	if s == "" {
		return nil
	}
	var skills []string
	for _, part := range splitString(s, ",") {
		if part != "" {
			skills = append(skills, part)
		}
	}
	return skills
}

func splitString(s, sep string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
		}
	}
	result = append(result, s[start:])
	return result
}

func parseAgentCard(data map[string]string) *AgentCard {
	if data == nil {
		return nil
	}

	card := &AgentCard{
		ID:        data["id"],
		Name:      data["name"],
		URL:       data["url"],
		Version:   data["version"],
		Universe:  data["universe"],
		Skills:    splitSkills(data["skills"]),
		Online:    data["online"] == "true",
	}

	if firstSeen, err := parseInt64(data["firstSeen"]); err == nil {
		card.FirstSeen = firstSeen
	}
	if lastSeen, err := parseInt64(data["lastSeen"]); err == nil {
		card.LastSeen = lastSeen
	}

	// v0.8.0 M1-2:多协议字段(降级处理,缺失时当空切片)
	if protocols := parseProtocols(data["protocols"]); len(protocols) > 0 {
		card.Protocols = protocols
	}
	if endpoints := parseEndpoints(data["endpoints"]); len(endpoints) > 0 {
		card.Endpoints = endpoints
	}

	// v0.8.0 M4-3 hotfix 2: lineage(自繁衍)
	// HASH 缺字段 → 空字符串(顶级 agent,lineage 无)
	card.ParentAgentID = data["parentAgentId"]

	return card
}

func parseAgentLoad(data map[string]string) *AgentLoad {
	if data == nil {
		return nil
	}

	load := &AgentLoad{}

	if agentID, ok := data["agentId"]; ok {
		load.AgentID = agentID
	}
	if activeTasks, err := parseInt(data["activeTasks"]); err == nil {
		load.ActiveTasks = activeTasks
	}
	if maxCapacity, err := parseInt(data["maxCapacity"]); err == nil {
		load.MaxCapacity = maxCapacity
	}
	if cpuUsage, err := parseFloat(data["cpuUsage"]); err == nil {
		load.CPUUsage = cpuUsage
	}
	if memoryUsage, err := parseFloat(data["memoryUsage"]); err == nil {
		load.MemoryUsage = memoryUsage
	}
	if lastSeen, err := parseInt64(data["lastSeen"]); err == nil {
		load.LastSeen = lastSeen
	}

	return load
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
