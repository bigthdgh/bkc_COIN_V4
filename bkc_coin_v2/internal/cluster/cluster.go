package cluster

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

// ClusterManager управляет кластером из 15 нод
type ClusterManager struct {
	nodes         []*Node
	shardMapper   *ShardMapper
	loadBalancer  *SimpleLoadBalancer
	healthChecker *HealthChecker
}

// Node представляет ноду в кластере
type Node struct {
	ID             int       `json:"id"`
	URL            string    `json:"url"`
	Role           string    `json:"role"`   // main, market, bank, games
	Status         string    `json:"status"` // active, inactive, maintenance
	LastPing       time.Time `json:"last_ping"`
	RequestCount   int64     `json:"request_count"`
	ErrorCount     int64     `json:"error_count"`
	CPUUsage       float64   `json:"cpu_usage"`
	MemoryUsage    float64   `json:"memory_usage"`
	MaxConnections int       `json:"max_connections"`
	mu             sync.RWMutex
}

// ShardManager управляет шардингом баз данных
type ShardMapper struct {
	NeonShards      []*DatabaseShard // 5 шардов Neon
	SupabaseShards  []*DatabaseShard // 2 шарда Supabase
	CockroachShards []*DatabaseShard // 2 шарда CockroachDB
	RedisInstances  []*RedisInstance // 6 экземпляров Redis
}

// DatabaseShard представляет шард базы данных
type DatabaseShard struct {
	ID             int    `json:"id"`
	Type           string `json:"type"` // neon, supabase, cockroach
	URL            string `json:"url"`
	Database       string `json:"database"`
	Status         string `json:"status"`
	Connections    int    `json:"connections"`
	MaxConnections int    `json:"max_connections"`
}

// RedisInstance представляет экземпляр Redis
type RedisInstance struct {
	ID        int    `json:"id"`
	URL       string `json:"url"`
	Role      string `json:"role"` // cache, anti-cheat, queue, leaderboard, online, locks
	Status    string `json:"status"`
	Memory    int64  `json:"memory_used"`
	MaxMemory int64  `json:"max_memory"`
}

// SimpleLoadBalancer простой балансировщик нагрузки
type SimpleLoadBalancer struct {
	strategy string // round-robin, least-connections, weighted
	counter  int64
	mu       sync.RWMutex
}

// HealthChecker проверяет здоровье нод
type HealthChecker struct {
	interval time.Duration
	timeout  time.Duration
}

// NewClusterManager создает новый менеджер кластера
func NewClusterManager() *ClusterManager {
	cm := &ClusterManager{
		shardMapper:  NewShardMapper(),
		loadBalancer: &SimpleLoadBalancer{strategy: "least-connections"},
		healthChecker: &HealthChecker{
			interval: 30 * time.Second,
			timeout:  5 * time.Second,
		},
	}

	cm.initializeNodes()
	return cm
}

// initializeNodes инициализирует 15 нод кластера
func (cm *ClusterManager) initializeNodes() {
	// Nodes 1-10: Main Core (обработка тапов)
	for i := 1; i <= 10; i++ {
		cm.nodes = append(cm.nodes, &Node{
			ID:     i,
			URL:    fmt.Sprintf("https://node-%d.onrender.com", i),
			Role:   "main",
			Status: "active",
		})
	}

	// Node 11: Market/NFT
	cm.nodes = append(cm.nodes, &Node{
		ID:     11,
		URL:    "https://node-market.onrender.com",
		Role:   "market",
		Status: "active",
	})

	// Node 12: Bank/Finance
	cm.nodes = append(cm.nodes, &Node{
		ID:     12,
		URL:    "https://node-bank.onrender.com",
		Role:   "bank",
		Status: "active",
	})

	// Nodes 13-15: Games/Sockets
	for i := 13; i <= 15; i++ {
		cm.nodes = append(cm.nodes, &Node{
			ID:     i,
			URL:    fmt.Sprintf("https://node-games-%d.onrender.com", i),
			Role:   "games",
			Status: "active",
		})
	}
}

// NewShardMapper создает новый маппер шардов
func NewShardMapper() *ShardMapper {
	sm := &ShardMapper{}

	// 5 шардов Neon для профилей пользователей
	for i := 0; i < 5; i++ {
		sm.NeonShards = append(sm.NeonShards, &DatabaseShard{
			ID:             i,
			Type:           "neon",
			URL:            fmt.Sprintf("postgresql://user:pass@neon-%d.db.io", i),
			Database:       fmt.Sprintf("bkc_users_%d", i),
			Status:         "active",
			MaxConnections: 100,
		})
	}

	// 2 шарда Supabase для P2P и кредитов
	for i := 0; i < 2; i++ {
		sm.SupabaseShards = append(sm.SupabaseShards, &DatabaseShard{
			ID:             i,
			Type:           "supabase",
			URL:            fmt.Sprintf("postgresql://user:pass@supabase-%d.db.io", i),
			Database:       fmt.Sprintf("bkc_market_%d", i),
			Status:         "active",
			MaxConnections: 50,
		})
	}

	// 2 шарда CockroachDB для логов
	for i := 0; i < 2; i++ {
		sm.CockroachShards = append(sm.CockroachShards, &DatabaseShard{
			ID:             i,
			Type:           "cockroach",
			URL:            fmt.Sprintf("postgresql://user:pass@cockroach-%d.db.io", i),
			Database:       fmt.Sprintf("bkc_logs_%d", i),
			Status:         "active",
			MaxConnections: 25,
		})
	}

	// 6 экземпляров Redis
	roles := []string{"cache", "anti-cheat", "queue", "leaderboard", "online", "locks"}
	for i, role := range roles {
		sm.RedisInstances = append(sm.RedisInstances, &RedisInstance{
			ID:        i,
			URL:       fmt.Sprintf("redis://redis-%d.redis.io", i),
			Role:      role,
			Status:    "active",
			MaxMemory: 512 * 1024 * 1024, // 512MB
		})
	}

	return sm
}

// GetUserShard определяет шард для пользователя
func (sm *ShardMapper) GetUserShard(userID int64) *DatabaseShard {
	shardIndex := userID % 5 // Шардинг по user_id % 5
	return sm.NeonShards[shardIndex]
}

// GetMarketShard определяет шард для маркетплейса
func (sm *ShardMapper) GetMarketShard(marketID int64) *DatabaseShard {
	shardIndex := marketID % 2 // 2 шарда для маркетплейса
	return sm.SupabaseShards[shardIndex]
}

// GetLogShard определяет шард для логов
func (sm *ShardMapper) GetLogShard(logID int64) *DatabaseShard {
	shardIndex := logID % 2 // 2 шарда для логов
	return sm.CockroachShards[shardIndex]
}

// GetRedisInstance получает экземпляр Redis по роли
func (sm *ShardMapper) GetRedisInstance(role string) *RedisInstance {
	for _, instance := range sm.RedisInstances {
		if instance.Role == role {
			return instance
		}
	}
	return sm.RedisInstances[0] // fallback
}

// SelectNode выбирает ноду для обработки запроса
func (cm *ClusterManager) SelectNode(role string) *Node {
	cm.loadBalancer.mu.RLock()
	defer cm.loadBalancer.mu.RUnlock()

	var candidates []*Node
	for _, node := range cm.nodes {
		if node.Role == role && node.Status == "active" {
			candidates = append(candidates, node)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	switch cm.loadBalancer.strategy {
	case "round-robin":
		return cm.roundRobinSelect(candidates)
	case "least-connections":
		return cm.leastConnectionsSelect(candidates)
	case "weighted":
		return cm.weightedSelect(candidates)
	default:
		return candidates[0]
	}
}

// roundRobinSelect выбирает ноду по round-robin
func (cm *ClusterManager) roundRobinSelect(candidates []*Node) *Node {
	cm.loadBalancer.counter++
	index := int(cm.loadBalancer.counter) % len(candidates)
	return candidates[index]
}

// leastConnectionsSelect выбирает ноду с наименьшим количеством соединений
func (cm *ClusterManager) leastConnectionsSelect(candidates []*Node) *Node {
	var selected *Node
	minConnections := int64(^uint64(0) >> 1) // max int64

	for _, node := range candidates {
		node.mu.RLock()
		if node.RequestCount < minConnections {
			minConnections = node.RequestCount
			selected = node
		}
		node.mu.RUnlock()
	}

	return selected
}

// weightedSelect выбирает ноду на основе веса (CPU/Memory)
func (cm *ClusterManager) weightedSelect(candidates []*Node) *Node {
	// Рассчитываем веса на основе CPU и Memory
	weights := make([]float64, len(candidates))
	totalWeight := 0.0

	for i, node := range candidates {
		node.mu.RLock()
		// Чем ниже загрузка, тем выше вес
		cpuWeight := (100.0 - node.CPUUsage) / 100.0
		memWeight := (100.0 - node.MemoryUsage) / 100.0
		weight := (cpuWeight + memWeight) / 2.0
		weights[i] = weight
		totalWeight += weight
		node.mu.RUnlock()
	}

	// Выбираем ноду на основе весов
	random := rand.Float64() * totalWeight
	current := 0.0

	for i, weight := range weights {
		current += weight
		if random <= current {
			return candidates[i]
		}
	}

	return candidates[0]
}

// StartHealthChecking запускает проверку здоровья нод
func (cm *ClusterManager) StartHealthChecking(ctx context.Context) {
	ticker := time.NewTicker(cm.healthChecker.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cm.checkAllNodes()
		}
	}
}

// checkAllNodes проверяет здоровье всех нод
func (cm *ClusterManager) checkAllNodes() {
	var wg sync.WaitGroup

	for _, node := range cm.nodes {
		wg.Add(1)
		go func(n *Node) {
			defer wg.Done()
			cm.checkNode(n)
		}(node)
	}

	wg.Wait()
}

// checkNode проверяет здоровье отдельной ноды
func (cm *ClusterManager) checkNode(node *Node) {
	node.mu.Lock()
	defer node.mu.Unlock()

	// Здесь должна быть реальная проверка HTTP endpoint
	// Для примера используем mock

	// Имитируем проверку здоровья
	success := rand.Float32() > 0.05 // 95%成功率

	if success {
		node.Status = "active"
		node.LastPing = time.Now()
		node.ErrorCount = 0
	} else {
		node.ErrorCount++
		if node.ErrorCount > 3 {
			node.Status = "inactive"
		}
	}

	// Обновляем метрики (mock данные)
	node.CPUUsage = rand.Float64() * 80    // 0-80%
	node.MemoryUsage = rand.Float64() * 70 // 0-70%
}

// GetClusterStats получает статистику кластера
func (cm *ClusterManager) GetClusterStats() map[string]interface{} {
	stats := make(map[string]interface{})

	var activeNodes, inactiveNodes int
	var totalRequests, totalErrors int64
	var avgCPU, avgMemory float64

	for _, node := range cm.nodes {
		node.mu.RLock()
		if node.Status == "active" {
			activeNodes++
		} else {
			inactiveNodes++
		}
		totalRequests += node.RequestCount
		totalErrors += node.ErrorCount
		avgCPU += node.CPUUsage
		avgMemory += node.MemoryUsage
		node.mu.RUnlock()
	}

	nodeCount := len(cm.nodes)
	if nodeCount > 0 {
		avgCPU /= float64(nodeCount)
		avgMemory /= float64(nodeCount)
	}

	stats["total_nodes"] = nodeCount
	stats["active_nodes"] = activeNodes
	stats["inactive_nodes"] = inactiveNodes
	stats["total_requests"] = totalRequests
	stats["total_errors"] = totalErrors
	stats["avg_cpu_usage"] = avgCPU
	stats["avg_memory_usage"] = avgMemory
	stats["uptime"] = time.Since(cm.nodes[0].LastPing).String()

	// Статистика шардов
	stats["neon_shards"] = len(cm.shardMapper.NeonShards)
	stats["supabase_shards"] = len(cm.shardMapper.SupabaseShards)
	stats["cockroach_shards"] = len(cm.shardMapper.CockroachShards)
	stats["redis_instances"] = len(cm.shardMapper.RedisInstances)

	return stats
}

// RouteRequest маршрутизирует запрос на нужную ноду
func (cm *ClusterManager) RouteRequest(ctx context.Context, requestType string, userID int64) (*Node, error) {
	var role string

	switch requestType {
	case "tap", "user", "energy", "level":
		role = "main"
	case "market", "nft", "p2p":
		role = "market"
	case "bank", "loan", "credit":
		role = "bank"
	case "games", "crash", "exchange":
		role = "games"
	default:
		role = "main"
	}

	node := cm.SelectNode(role)
	if node == nil {
		return nil, fmt.Errorf("no available nodes for role: %s", role)
	}

	// Увеличиваем счетчик запросов
	node.mu.Lock()
	node.RequestCount++
	node.mu.Unlock()

	return node, nil
}

// GetShardInfo получает информацию о шардах
func (cm *ClusterManager) GetShardInfo() map[string]interface{} {
	info := make(map[string]interface{})

	// Neon шардинг
	neonInfo := make([]map[string]interface{}, 0)
	for _, shard := range cm.shardMapper.NeonShards {
		neonInfo = append(neonInfo, map[string]interface{}{
			"id":              shard.ID,
			"database":        shard.Database,
			"status":          shard.Status,
			"connections":     shard.Connections,
			"max_connections": shard.MaxConnections,
		})
	}
	info["neon_shards"] = neonInfo

	// Supabase шардинг
	supabaseInfo := make([]map[string]interface{}, 0)
	for _, shard := range cm.shardMapper.SupabaseShards {
		supabaseInfo = append(supabaseInfo, map[string]interface{}{
			"id":              shard.ID,
			"database":        shard.Database,
			"status":          shard.Status,
			"connections":     shard.Connections,
			"max_connections": shard.MaxConnections,
		})
	}
	info["supabase_shards"] = supabaseInfo

	// Redis инстансы
	redisInfo := make([]map[string]interface{}, 0)
	for _, instance := range cm.shardMapper.RedisInstances {
		redisInfo = append(redisInfo, map[string]interface{}{
			"id":          instance.ID,
			"role":        instance.Role,
			"status":      instance.Status,
			"memory_used": instance.Memory,
			"max_memory":  instance.MaxMemory,
		})
	}
	info["redis_instances"] = redisInfo

	return info
}

// ScaleNode масштабирует ноду (добавляет/удаляет ресурсы)
func (cm *ClusterManager) ScaleNode(nodeID int, scaleType string) error {
	node := cm.getNodeByID(nodeID)
	if node == nil {
		return fmt.Errorf("node not found: %d", nodeID)
	}

	node.mu.Lock()
	defer node.mu.Unlock()

	switch scaleType {
	case "up":
		// Увеличиваем ресурсы
		node.MaxConnections = int(float64(node.MaxConnections) * 1.5)
		log.Printf("Node %d scaled up: max connections = %d", nodeID, node.MaxConnections)
	case "down":
		// Уменьшаем ресурсы
		node.MaxConnections = int(float64(node.MaxConnections) * 0.7)
		log.Printf("Node %d scaled down: max connections = %d", nodeID, node.MaxConnections)
	default:
		return fmt.Errorf("invalid scale type: %s", scaleType)
	}

	return nil
}

// getNodeByID получает ноду по ID
func (cm *ClusterManager) getNodeByID(nodeID int) *Node {
	for _, node := range cm.nodes {
		if node.ID == nodeID {
			return node
		}
	}
	return nil
}

// GetNodeByURL получает ноду по URL
func (cm *ClusterManager) GetNodeByURL(url string) *Node {
	for _, node := range cm.nodes {
		if node.URL == url {
			return node
		}
	}
	return nil
}

// UpdateNodeMetrics обновляет метрики ноды
func (cm *ClusterManager) UpdateNodeMetrics(nodeID int, cpu, memory float64, requests, errors int64) {
	node := cm.getNodeByID(nodeID)
	if node == nil {
		return
	}

	node.mu.Lock()
	node.CPUUsage = cpu
	node.MemoryUsage = memory
	node.RequestCount = requests
	node.ErrorCount = errors
	node.LastPing = time.Now()
	node.mu.Unlock()
}

// GetLoadDistribution получает распределение нагрузки
func (cm *ClusterManager) GetLoadDistribution() map[string]int {
	distribution := make(map[string]int)

	for _, node := range cm.nodes {
		node.mu.RLock()
		distribution[node.Role]++
		node.mu.RUnlock()
	}

	return distribution
}

// Failover выполняет переключение на резервную ноду
func (cm *ClusterManager) Failover(failedNodeID int) error {
	failedNode := cm.getNodeByID(failedNodeID)
	if failedNode == nil {
		return fmt.Errorf("failed node not found: %d", failedNodeID)
	}

	// Ищем резервную ноду той же роли
	var backupNode *Node
	for _, node := range cm.nodes {
		if node.Role == failedNode.Role && node.ID != failedNodeID && node.Status == "active" {
			backupNode = node
			break
		}
	}

	if backupNode == nil {
		return fmt.Errorf("no backup node available for role: %s", failedNode.Role)
	}

	// Переключаем статус нод
	failedNode.mu.Lock()
	failedNode.Status = "maintenance"
	failedNode.mu.Unlock()

	log.Printf("Failover: Node %d -> Node %d for role %s",
		failedNodeID, backupNode.ID, failedNode.Role)

	return nil
}
