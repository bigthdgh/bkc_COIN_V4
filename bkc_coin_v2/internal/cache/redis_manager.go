package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisManager управляет Redis кэшем и Pub/Sub
type RedisManager struct {
	clients []*redis.Client
	current int
	mu      sync.RWMutex
	
	// Pub/Sub подписчики
	subscribers map[string][]chan Message
	subMu       sync.RWMutex
	
	// Конфигурация
	config RedisConfig
	
	// Контекст
	ctx    context.Context
	cancel context.CancelFunc
	
	// Метрики
	metrics *RedisMetrics
}

// RedisConfig конфигурация Redis
type RedisConfig struct {
	Nodes           []RedisNode `json:"nodes"`
	DefaultTTL      time.Duration `json:"default_ttl"`
	KeyPrefix       string        `json:"key_prefix"`
	EnableReplication bool         `json:"enable_replication"`
	MaxRetries      int           `json:"max_retries"`
	PoolSize        int           `json:"pool_size"`
}

// RedisNode конфигурация Redis ноды
type RedisNode struct {
	ID       int    `json:"id"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
	Database int    `json:"database"`
	Region   string `json:"region"`
	Primary  bool   `json:"primary"`
}

// Message сообщение для Pub/Sub
type Message struct {
	Channel   string      `json:"channel"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
	NodeID    string      `json:"node_id"`
	MessageID string      `json:"message_id"`
}

// RedisMetrics метрики Redis
type RedisMetrics struct {
	TotalCommands     int64 `json:"total_commands"`
	FailedCommands    int64 `json:"failed_commands"`
	CacheHits         int64 `json:"cache_hits"`
	CacheMisses       int64 `json:"cache_misses"`
	PubSubMessages    int64 `json:"pubsub_messages"`
	ActiveConnections int64 `json:"active_connections"`
	mu                sync.RWMutex
}

// CacheEntry запись в кэше
type CacheEntry struct {
	Key        string      `json:"key"`
	Value      interface{} `json:"value"`
	ExpiresAt  time.Time   `json:"expires_at"`
	CreatedAt  time.Time   `json:"created_at"`
	AccessCount int64      `json:"access_count"`
}

// LeaderboardEntry запись в лидерборде
type LeaderboardEntry struct {
	UserID    int64   `json:"user_id"`
	Username  string  `json:"username"`
	Score     float64 `json:"score"`
	Rank      int     `json:"rank"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DefaultRedisConfig конфигурация по умолчанию
func DefaultRedisConfig() RedisConfig {
	return RedisConfig{
		Nodes: []RedisNode{
			{
				ID:       0,
				Host:     "redis-12345.c1.us-east-1-2.ec2.cloud.redislabs.com",
				Port:     12345,
				Password: "",
				Database: 0,
				Region:   "us-east-1",
				Primary:  true,
			},
			{
				ID:       1,
				Host:     "redis-12346.c1.us-east-1-2.ec2.cloud.redislabs.com",
				Port:     12346,
				Password: "",
				Database: 0,
				Region:   "us-east-1",
				Primary:  false,
			},
		},
		DefaultTTL:       5 * time.Minute,
		KeyPrefix:        "bkc:",
		EnableReplication: true,
		MaxRetries:       3,
		PoolSize:         50,
	}
}

// NewRedisManager создает новый менеджер Redis
func NewRedisManager(config RedisConfig) *RedisManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	rm := &RedisManager{
		clients:    make([]*redis.Client, 0),
		subscribers: make(map[string][]chan Message),
		config:     config,
		ctx:        ctx,
		cancel:     cancel,
		metrics:    &RedisMetrics{},
	}
	
	// Подключение к Redis нодам
	for _, node := range config.Nodes {
		client := redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%d", node.Host, node.Port),
			Password: node.Password,
			DB:       node.Database,
			MaxRetries: config.MaxRetries,
			PoolSize: config.PoolSize,
		})
		
		// Проверка соединения
		if err := client.Ping(ctx).Err(); err != nil {
			log.Printf("Failed to connect to Redis node %d: %v", node.ID, err)
			continue
		}
		
		rm.clients = append(rm.clients, client)
		log.Printf("Connected to Redis node %d at %s:%d", node.ID, node.Host, node.Port)
	}
	
	if len(rm.clients) == 0 {
		log.Fatal("No Redis nodes available")
	}
	
	return rm
}

// getClient получает клиента Redis (round-robin)
func (rm *RedisManager) getClient() *redis.Client {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	if len(rm.clients) == 0 {
		return nil
	}
	
	client := rm.clients[rm.current]
	rm.current = (rm.current + 1) % len(rm.clients)
	return client
}

// getPrimaryClient получает primary клиент Redis
func (rm *RedisManager) getPrimaryClient() *redis.Client {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	for i, client := range rm.clients {
		if i < len(rm.config.Nodes) && rm.config.Nodes[i].Primary {
			return client
		}
	}
	
	// Если нет primary, возвращаем первый
	if len(rm.clients) > 0 {
		return rm.clients[0]
	}
	
	return nil
}

// Set устанавливает значение в кэш
func (rm *RedisManager) Set(ctx context.Context, key string, value interface{}, ttl ...time.Duration) error {
	client := rm.getClient()
	if client == nil {
		return fmt.Errorf("no Redis clients available")
	}
	
	fullKey := rm.config.KeyPrefix + key
	
	var expiration time.Duration
	if len(ttl) > 0 {
		expiration = ttl[0]
	} else {
		expiration = rm.config.DefaultTTL
	}
	
	// Сериализация значения
	jsonValue, err := json.Marshal(value)
	if err != nil {
		rm.incrementFailedCommands()
		return fmt.Errorf("failed to marshal value: %w", err)
	}
	
	// Установка значения
	err = client.Set(ctx, fullKey, jsonValue, expiration).Err()
	if err != nil {
		rm.incrementFailedCommands()
		return fmt.Errorf("failed to set key: %w", err)
	}
	
	// Репликация на другие ноды если включено
	if rm.config.EnableReplication {
		go rm.replicateSet(ctx, fullKey, jsonValue, expiration)
	}
	
	rm.incrementTotalCommands()
	return nil
}

// Get получает значение из кэша
func (rm *RedisManager) Get(ctx context.Context, key string, dest interface{}) error {
	client := rm.getClient()
	if client == nil {
		rm.incrementCacheMisses()
		return fmt.Errorf("no Redis clients available")
	}
	
	fullKey := rm.config.KeyPrefix + key
	
	// Получение значения
	result, err := client.Get(ctx, fullKey).Result()
	if err != nil {
		if err == redis.Nil {
			rm.incrementCacheMisses()
			return fmt.Errorf("key not found")
		}
		rm.incrementFailedCommands()
		return fmt.Errorf("failed to get key: %w", err)
	}
	
	// Десериализация значения
	if err := json.Unmarshal([]byte(result), dest); err != nil {
		rm.incrementFailedCommands()
		return fmt.Errorf("failed to unmarshal value: %w", err)
	}
	
	rm.incrementCacheHits()
	rm.incrementTotalCommands()
	return nil
}

// Delete удаляет ключ из кэша
func (rm *RedisManager) Delete(ctx context.Context, key string) error {
	client := rm.getPrimaryClient()
	if client == nil {
		return fmt.Errorf("no Redis clients available")
	}
	
	fullKey := rm.config.KeyPrefix + key
	
	err := client.Del(ctx, fullKey).Err()
	if err != nil {
		rm.incrementFailedCommands()
		return fmt.Errorf("failed to delete key: %w", err)
	}
	
	// Репликация удаления
	if rm.config.EnableReplication {
		go rm.replicateDelete(ctx, fullKey)
	}
	
	rm.incrementTotalCommands()
	return nil
}

// Exists проверяет существование ключа
func (rm *RedisManager) Exists(ctx context.Context, key string) (bool, error) {
	client := rm.getClient()
	if client == nil {
		return false, fmt.Errorf("no Redis clients available")
	}
	
	fullKey := rm.config.KeyPrefix + key
	
	result, err := client.Exists(ctx, fullKey).Result()
	if err != nil {
		rm.incrementFailedCommands()
		return false, fmt.Errorf("failed to check key existence: %w", err)
	}
	
	rm.incrementTotalCommands()
	return result > 0, nil
}

// Increment увеличивает числовое значение
func (rm *RedisManager) Increment(ctx context.Context, key string, value int64) (int64, error) {
	client := rm.getPrimaryClient()
	if client == nil {
		return 0, fmt.Errorf("no Redis clients available")
	}
	
	fullKey := rm.config.KeyPrefix + key
	
	result, err := client.IncrBy(ctx, fullKey, value).Result()
	if err != nil {
		rm.incrementFailedCommands()
		return 0, fmt.Errorf("failed to increment key: %w", err)
	}
	
	rm.incrementTotalCommands()
	return result, nil
}

// Publish публикует сообщение в канал
func (rm *RedisManager) Publish(ctx context.Context, channel string, data interface{}) error {
	client := rm.getPrimaryClient()
	if client == nil {
		return fmt.Errorf("no Redis clients available")
	}
	
	message := Message{
		Channel:   channel,
		Data:      data,
		Timestamp: time.Now(),
		NodeID:    fmt.Sprintf("node_%d", rm.current),
		MessageID: fmt.Sprintf("%d_%d", time.Now().UnixNano(), rm.current),
	}
	
	jsonMessage, err := json.Marshal(message)
	if err != nil {
		rm.incrementFailedCommands()
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	
	fullChannel := rm.config.KeyPrefix + channel
	
	err = client.Publish(ctx, fullChannel, jsonMessage).Err()
	if err != nil {
		rm.incrementFailedCommands()
		return fmt.Errorf("failed to publish message: %w", err)
	}
	
	rm.incrementPubSubMessages()
	rm.incrementTotalCommands()
	return nil
}

// Subscribe подписывается на канал
func (rm *RedisManager) Subscribe(ctx context.Context, channel string) (<-chan Message, error) {
	client := rm.getClient()
	if client == nil {
		return nil, fmt.Errorf("no Redis clients available")
	}
	
	fullChannel := rm.config.KeyPrefix + channel
	
	// Создание канала для сообщений
	msgChan := make(chan Message, 100)
	
	// Добавление подписчика
	rm.subMu.Lock()
	rm.subscribers[channel] = append(rm.subscribers[channel], msgChan)
	rm.subMu.Unlock()
	
	// Запуск подписчика в горутине
	go rm.handleSubscription(ctx, client, fullChannel, channel, msgChan)
	
	return msgChan, nil
}

// handleSubscription обрабатывает подписку
func (rm *RedisManager) handleSubscription(ctx context.Context, client *redis.Client, fullChannel, channel string, msgChan chan Message) {
	pubsub := client.Subscribe(ctx, fullChannel)
	defer pubsub.Close()
	
	ch := pubsub.Channel()
	
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			var message Message
			if err := json.Unmarshal([]byte(msg.Payload), &message); err != nil {
				log.Printf("Failed to unmarshal message: %v", err)
				continue
			}
			
			select {
			case msgChan <- message:
			case <-ctx.Done():
				return
			default:
				// Канал переполнен, пропускаем сообщение
				log.Printf("Message channel full for %s, dropping message", channel)
			}
		}
	}
}

// UpdateLeaderboard обновляет лидерборд
func (rm *RedisManager) UpdateLeaderboard(ctx context.Context, leaderboardName string, userID int64, score float64) error {
	client := rm.getPrimaryClient()
	if client == nil {
		return fmt.Errorf("no Redis clients available")
	}
	
	key := fmt.Sprintf("%sleaderboard:%s", rm.config.KeyPrefix, leaderboardName)
	
	// Обновление счета в отсортированном множестве
	err := client.ZAdd(ctx, key, redis.Z{
		Score:  score,
		Member: userID,
	}).Err()
	
	if err != nil {
		rm.incrementFailedCommands()
		return fmt.Errorf("failed to update leaderboard: %w", err)
	}
	
	rm.incrementTotalCommands()
	return nil
}

// GetLeaderboard получает лидерборд
func (rm *RedisManager) GetLeaderboard(ctx context.Context, leaderboardName string, offset, limit int) ([]LeaderboardEntry, error) {
	client := rm.getClient()
	if client == nil {
		return nil, fmt.Errorf("no Redis clients available")
	}
	
	key := fmt.Sprintf("%sleaderboard:%s", rm.config.KeyPrefix, leaderboardName)
	
	// Получение топ игроков (в обратном порядке для высоких счетов)
	results, err := client.ZRevRangeWithScores(ctx, key, int64(offset), int64(offset+limit-1)).Result()
	if err != nil {
		rm.incrementFailedCommands()
		return nil, fmt.Errorf("failed to get leaderboard: %w", err)
	}
	
	entries := make([]LeaderboardEntry, len(results))
	for i, result := range results {
		userID, ok := result.Member.(int64)
		if !ok {
			continue
		}
		
		entries[i] = LeaderboardEntry{
			UserID:    userID,
			Score:     result.Score,
			Rank:      offset + i + 1,
			UpdatedAt: time.Now(),
		}
	}
	
	rm.incrementTotalCommands()
	return entries, nil
}

// SetGlobalOnline устанавливает глобальный онлайн
func (rm *RedisManager) SetGlobalOnline(ctx context.Context, count int64) error {
	key := "global:online"
	return rm.Set(ctx, key, count, 30*time.Second)
}

// GetGlobalOnline получает глобальный онлайн
func (rm *RedisManager) GetGlobalOnline(ctx context.Context) (int64, error) {
	var count int64
	err := rm.Get(ctx, "global:online", &count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// replicateSet реплицирует SET операцию
func (rm *RedisManager) replicateSet(ctx context.Context, key string, value []byte, expiration time.Duration) {
	for _, client := range rm.clients {
		if client == rm.getPrimaryClient() {
			continue
		}
		
		if err := client.Set(ctx, key, value, expiration).Err(); err != nil {
			log.Printf("Failed to replicate SET to node: %v", err)
		}
	}
}

// replicateDelete реплицирует DELETE операцию
func (rm *RedisManager) replicateDelete(ctx context.Context, key string) {
	for _, client := range rm.clients {
		if client == rm.getPrimaryClient() {
			continue
		}
		
		if err := client.Del(ctx, key).Err(); err != nil {
			log.Printf("Failed to replicate DELETE to node: %v", err)
		}
	}
}

// Методы для инкремента метрик
func (rm *RedisManager) incrementTotalCommands() {
	rm.metrics.mu.Lock()
	defer rm.metrics.mu.Unlock()
	rm.metrics.TotalCommands++
}

func (rm *RedisManager) incrementFailedCommands() {
	rm.metrics.mu.Lock()
	defer rm.metrics.mu.Unlock()
	rm.metrics.FailedCommands++
}

func (rm *RedisManager) incrementCacheHits() {
	rm.metrics.mu.Lock()
	defer rm.metrics.mu.Unlock()
	rm.metrics.CacheHits++
}

func (rm *RedisManager) incrementCacheMisses() {
	rm.metrics.mu.Lock()
	defer rm.metrics.mu.Unlock()
	rm.metrics.CacheMisses++
}

func (rm *RedisManager) incrementPubSubMessages() {
	rm.metrics.mu.Lock()
	defer rm.metrics.mu.Unlock()
	rm.metrics.PubSubMessages++
}

// GetMetrics возвращает метрики
func (rm *RedisManager) GetMetrics() RedisMetrics {
	rm.metrics.mu.RLock()
	defer rm.metrics.mu.RUnlock()
	
	metrics := *rm.metrics
	metrics.ActiveConnections = int64(len(rm.clients))
	
	return metrics
}

// Close закрывает все соединения
func (rm *RedisManager) Close() error {
	rm.cancel()
	
	var lastErr error
	for _, client := range rm.clients {
		if err := client.Close(); err != nil {
			lastErr = err
		}
	}
	
	return lastErr
}

// HealthCheck проверяет здоровье Redis
func (rm *RedisManager) HealthCheck(ctx context.Context) map[string]bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	results := make(map[string]bool)
	
	for i, client := range rm.clients {
		if i < len(rm.config.Nodes) {
			node := rm.config.Nodes[i]
			key := fmt.Sprintf("%s:%d", node.Host, node.Port)
			results[key] = client.Ping(ctx).Err() == nil
		}
	}
	
	return results
}
