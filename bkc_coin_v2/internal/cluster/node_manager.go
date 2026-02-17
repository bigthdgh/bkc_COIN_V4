package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"sync"
	"time"
)

// NodeType тип ноды
type NodeType string

const (
	NodeTypeCore     NodeType = "core"     // Основные ноды (тапы, регистрация)
	NodeTypeGame     NodeType = "game"     // Игровые ноды (WebSocket)
	NodeTypeMarket   NodeType = "market"   // Ноды барахолки
	NodeTypeBank     NodeType = "bank"     // Ноды банка
	NodeTypeAdmin    NodeType = "admin"    // Админ ноды
	NodeTypeFull     NodeType = "full"     // Полные ноды
)

// NodeStatus статус ноды
type NodeStatus string

const (
	NodeStatusOnline   NodeStatus = "online"
	NodeStatusOffline  NodeStatus = "offline"
	NodeStatusBusy     NodeStatus = "busy"
	NodeStatusMaintenance NodeStatus = "maintenance"
)

// NodeInfo информация о ноде
type NodeInfo struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Type            NodeType  `json:"type"`
	URL             string    `json:"url"`
	Status          NodeStatus `json:"status"`
	Region          string    `json:"region"`
	Capacity        int       `json:"capacity"`
	CurrentLoad     int       `json:"current_load"`
	ResponseTime    int64     `json:"response_time"`
	LastHealthCheck time.Time `json:"last_health_check"`
	Version         string    `json:"version"`
	Features        []string  `json:"features"`
	Priority        int       `json:"priority"`
}

// NodeManager управляет кластером нод
type NodeManager struct {
	nodes           map[string]*NodeInfo
	nodesByType     map[NodeType][]*NodeInfo
	mu              sync.RWMutex
	
	// Балансировщик нагрузки
	loadBalancer    *LoadBalancer
	
	// Конфигурация
	config          NodeManagerConfig
	
	// HTTP клиент
	client          *http.Client
	
	// Контекст
	ctx             context.Context
	cancel          context.CancelFunc
	
	// Метрики
	metrics         *ClusterMetrics
}

// LoadBalancer балансировщик нагрузки
type LoadBalancer struct {
	strategy LoadBalanceStrategy
}

// LoadBalanceStrategy стратегия балансировки
type LoadBalanceStrategy int

const (
	StrategyRoundRobin LoadBalanceStrategy = iota
	StrategyLeastConnections
	StrategyWeightedRoundRobin
	StrategyRandom
	StrategyResponseTime
)

// NodeManagerConfig конфигурация менеджера нод
type NodeManagerConfig struct {
	HealthCheckInterval   time.Duration `json:"health_check_interval"`
	HealthCheckTimeout    time.Duration `json:"health_check_timeout"`
	MaxRetries           int           `json:"max_retries"`
	NodeTimeout          time.Duration `json:"node_timeout"`
	LoadBalanceStrategy  LoadBalanceStrategy `json:"load_balance_strategy"`
	EnableFailover       bool          `json:"enable_failover"`
	FailoverThreshold    int           `json:"failover_threshold"`
}

// ClusterMetrics метрики кластера
type ClusterMetrics struct {
	TotalNodes          int64     `json:"total_nodes"`
	OnlineNodes         int64     `json:"online_nodes"`
	TotalRequests       int64     `json:"total_requests"`
	FailedRequests      int64     `json:"failed_requests"`
	AvgResponseTime     int64     `json:"avg_response_time"`
	LastUpdated         time.Time `json:"last_updated"`
	mu                  sync.RWMutex
}

// DefaultNodeManagerConfig конфигурация по умолчанию
func DefaultNodeManagerConfig() NodeManagerConfig {
	return NodeManagerConfig{
		HealthCheckInterval:   30 * time.Second,
		HealthCheckTimeout:    5 * time.Second,
		MaxRetries:           3,
		NodeTimeout:          10 * time.Second,
		LoadBalanceStrategy:  StrategyLeastConnections,
		EnableFailover:       true,
		FailoverThreshold:    2,
	}
}

// NewNodeManager создает новый менеджер нод
func NewNodeManager(config NodeManagerConfig) *NodeManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &NodeManager{
		nodes:       make(map[string]*NodeInfo),
		nodesByType: make(map[NodeType][]*NodeInfo),
		config:      config,
		client: &http.Client{
			Timeout: config.HealthCheckTimeout,
		},
		ctx:          ctx,
		cancel:       cancel,
		loadBalancer: &LoadBalancer{
			strategy: config.LoadBalanceStrategy,
		},
		metrics: &ClusterMetrics{},
	}
}

// AddNode добавляет ноду в кластер
func (nm *NodeManager) AddNode(node *NodeInfo) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	
	// Проверка на дубликат
	if _, exists := nm.nodes[node.ID]; exists {
		return fmt.Errorf("node with ID %s already exists", node.ID)
	}
	
	// Установка времени последней проверки
	node.LastHealthCheck = time.Now()
	
	// Добавление ноды
	nm.nodes[node.ID] = node
	
	// Добавление в индекс по типу
	nm.nodesByType[node.Type] = append(nm.nodesByType[node.Type], node)
	
	// Обновление метрик
	nm.updateMetrics()
	
	return nil
}

// RemoveNode удаляет ноду из кластера
func (nm *NodeManager) RemoveNode(nodeID string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	
	node, exists := nm.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node with ID %s not found", nodeID)
	}
	
	// Удаление из основного мапа
	delete(nm.nodes, nodeID)
	
	// Удаление из индекса по типу
	nodes := nm.nodesByType[node.Type]
	for i, n := range nodes {
		if n.ID == nodeID {
			nm.nodesByType[node.Type] = append(nodes[:i], nodes[i+1:]...)
			break
		}
	}
	
	// Обновление метрик
	nm.updateMetrics()
	
	return nil
}

// GetNode получает ноду по ID
func (nm *NodeManager) GetNode(nodeID string) (*NodeInfo, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	
	node, exists := nm.nodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("node with ID %s not found", nodeID)
	}
	
	return node, nil
}

// GetNodesByType получает ноды по типу
func (nm *NodeManager) GetNodesByType(nodeType NodeType) []*NodeInfo {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	
	nodes := make([]*NodeInfo, len(nm.nodesByType[nodeType]))
	copy(nodes, nm.nodesByType[nodeType])
	
	return nodes
}

// GetOnlineNodesByType получает онлайн ноды по типу
func (nm *NodeManager) GetOnlineNodesByType(nodeType NodeType) []*NodeInfo {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	
	var onlineNodes []*NodeInfo
	for _, node := range nm.nodesByType[nodeType] {
		if node.Status == NodeStatusOnline {
			onlineNodes = append(onlineNodes, node)
		}
	}
	
	return onlineNodes
}

// SelectNode выбирает ноду для запроса
func (nm *NodeManager) SelectNode(nodeType NodeType) (*NodeInfo, error) {
	onlineNodes := nm.GetOnlineNodesByType(nodeType)
	if len(onlineNodes) == 0 {
		return nil, fmt.Errorf("no online nodes available for type %s", nodeType)
	}
	
	return nm.loadBalancer.SelectNode(onlineNodes), nil
}

// SelectNodes выбирает несколько нод
func (nm *NodeManager) SelectNodes(nodeType NodeType, count int) ([]*NodeInfo, error) {
	onlineNodes := nm.GetOnlineNodesByType(nodeType)
	if len(onlineNodes) == 0 {
		return nil, fmt.Errorf("no online nodes available for type %s", nodeType)
	}
	
	if count > len(onlineNodes) {
		count = len(onlineNodes)
	}
	
	selected := make([]*NodeInfo, 0, count)
	used := make(map[string]bool)
	
	for len(selected) < count {
		node := nm.loadBalancer.SelectNode(onlineNodes)
		if !used[node.ID] {
			selected = append(selected, node)
			used[node.ID] = true
		}
	}
	
	return selected, nil
}

// SelectNode выбирает ноду на основе стратегии
func (lb *LoadBalancer) SelectNode(nodes []*NodeInfo) *NodeInfo {
	if len(nodes) == 0 {
		return nil
	}
	
	switch lb.strategy {
	case StrategyRoundRobin:
		return lb.roundRobinSelect(nodes)
	case StrategyLeastConnections:
		return lb.leastConnectionsSelect(nodes)
	case StrategyWeightedRoundRobin:
		return lb.weightedRoundRobinSelect(nodes)
	case StrategyRandom:
		return lb.randomSelect(nodes)
	case StrategyResponseTime:
		return lb.responseTimeSelect(nodes)
	default:
		return nodes[0]
	}
}

// roundRobinSelect выбор по кругу
func (lb *LoadBalancer) roundRobinSelect(nodes []*NodeInfo) *NodeInfo {
	// Простая реализация - выбираем ноду с наименьшей нагрузкой
	return lb.leastConnectionsSelect(nodes)
}

// leastConnectionsSelect выбор с наименьшим количеством соединений
func (lb *LoadBalancer) leastConnectionsSelect(nodes []*NodeInfo) *NodeInfo {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].CurrentLoad < nodes[j].CurrentLoad
	})
	return nodes[0]
}

// weightedRoundRobinSelect взвешенный выбор по кругу
func (lb *LoadBalancer) weightedRoundRobinSelect(nodes []*NodeInfo) *NodeInfo {
	// Сортировка по приоритету и нагрузке
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Priority != nodes[j].Priority {
			return nodes[i].Priority > nodes[j].Priority
		}
		return nodes[i].CurrentLoad < nodes[j].CurrentLoad
	})
	return nodes[0]
}

// randomSelect случайный выбор
func (lb *LoadBalancer) randomSelect(nodes []*NodeInfo) *NodeInfo {
	return nodes[rand.Intn(len(nodes))]
}

// responseTimeSelect выбор по времени отклика
func (lb *LoadBalancer) responseTimeSelect(nodes []*NodeInfo) *NodeInfo {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ResponseTime < nodes[j].ResponseTime
	})
	return nodes[0]
}

// Start запускает менеджер нод
func (nm *NodeManager) Start() {
	// Запуск health checker
	go nm.healthChecker()
	
	// Запуск метрик
	go nm.metricsUpdater()
}

// Stop останавливает менеджер нод
func (nm *NodeManager) Stop() {
	nm.cancel()
}

// healthChecker проверяет здоровье нод
func (nm *NodeManager) healthChecker() {
	ticker := time.NewTicker(nm.config.HealthCheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-nm.ctx.Done():
			return
		case <-ticker.C:
			nm.checkAllNodes()
		}
	}
}

// checkAllNodes проверяет все ноды
func (nm *NodeManager) checkAllNodes() {
	nm.mu.RLock()
	nodes := make([]*NodeInfo, 0, len(nm.nodes))
	for _, node := range nm.nodes {
		nodes = append(nodes, node)
	}
	nm.mu.RUnlock()
	
	for _, node := range nodes {
		go nm.checkNode(node)
	}
}

// checkNode проверяет здоровье отдельной ноды
func (nm *NodeManager) checkNode(node *NodeInfo) {
	start := time.Now()
	
	// Проверка здоровья ноды
	healthURL := fmt.Sprintf("%s/api/v1/health", node.URL)
	resp, err := nm.client.Get(healthURL)
	responseTime := time.Since(start).Milliseconds()
	
	nm.mu.Lock()
	defer nm.mu.Unlock()
	
	node.LastHealthCheck = time.Now()
	node.ResponseTime = responseTime
	
	if err != nil || resp.StatusCode != http.StatusOK {
		node.Status = NodeStatusOffline
		if resp != nil {
			resp.Body.Close()
		}
		return
	}
	
	defer resp.Body.Close()
	
	// Парсинг ответа для получения нагрузки
	var healthResp struct {
		Status  string `json:"status"`
		Load    int    `json:"load"`
		Uptime  int64  `json:"uptime"`
		Version string `json:"version"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err == nil {
		node.CurrentLoad = healthResp.Load
		node.Version = healthResp.Version
	}
	
	// Обновление статуса
	if node.CurrentLoad >= node.Capacity*90/100 {
		node.Status = NodeStatusBusy
	} else {
		node.Status = NodeStatusOnline
	}
}

// metricsUpdater обновляет метрики
func (nm *NodeManager) metricsUpdater() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-nm.ctx.Done():
			return
		case <-ticker.C:
			nm.updateMetrics()
		}
	}
}

// updateMetrics обновляет метрики кластера
func (nm *NodeManager) updateMetrics() {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	
	nm.metrics.mu.Lock()
	defer nm.metrics.mu.Unlock()
	
	nm.metrics.TotalNodes = int64(len(nm.nodes))
	nm.metrics.OnlineNodes = 0
	
	for _, node := range nm.nodes {
		if node.Status == NodeStatusOnline {
			nm.metrics.OnlineNodes++
		}
	}
	
	nm.metrics.LastUpdated = time.Now()
}

// GetMetrics возвращает метрики кластера
func (nm *NodeManager) GetMetrics() ClusterMetrics {
	nm.metrics.mu.RLock()
	defer nm.metrics.mu.RUnlock()
	
	return *nm.metrics
}

// GetAllNodes возвращает все ноды
func (nm *NodeManager) GetAllNodes() []*NodeInfo {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	
	nodes := make([]*NodeInfo, 0, len(nm.nodes))
	for _, node := range nm.nodes {
		nodes = append(nodes, node)
	}
	
	return nodes
}

// UpdateNode обновляет информацию о ноде
func (nm *NodeManager) UpdateNode(nodeID string, updates func(*NodeInfo)) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	
	node, exists := nm.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node with ID %s not found", nodeID)
	}
	
	updates(node)
	
	return nil
}

// SetLoadBalanceStrategy устанавливает стратегию балансировки
func (nm *NodeManager) SetLoadBalanceStrategy(strategy LoadBalanceStrategy) {
	nm.loadBalancer.strategy = strategy
}

// GetLoadBalanceStrategy возвращает текущую стратегию
func (nm *NodeManager) GetLoadBalanceStrategy() LoadBalanceStrategy {
	return nm.loadBalancer.strategy
}
