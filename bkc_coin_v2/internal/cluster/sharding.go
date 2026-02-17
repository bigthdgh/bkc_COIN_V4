package cluster

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// ShardManager управляет распределенными базами данных
type ShardManager struct {
	// Основные базы пользователей (Neon)
	UserShards []*pgxpool.Pool
	// Логи и история (CockroachDB)
	LogShards []*pgxpool.Pool
	// Кэш и синхронизация (Redis)
	RedisClients []RedisClient

	mu     sync.RWMutex
	health map[string]bool
}

type RedisClient interface {
	Ping(ctx context.Context) *redis.StatusCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Publish(ctx context.Context, channel string, message interface{}) *redis.IntCmd
	Subscribe(ctx context.Context, channels ...string) *redis.PubSub
}

// NewShardManager создает новый менеджер шардинга
func NewShardManager() *ShardManager {
	return &ShardManager{
		health: make(map[string]bool),
	}
}

// AddUserShard добавляет шард пользователей
func (sm *ShardManager) AddUserShard(ctx context.Context, connString string) error {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return fmt.Errorf("failed to create user shard pool: %w", err)
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.UserShards = append(sm.UserShards, pool)
	shardID := fmt.Sprintf("user_%d", len(sm.UserShards)-1)
	sm.health[shardID] = true

	return nil
}

// AddLogShard добавляет шард логов
func (sm *ShardManager) AddLogShard(ctx context.Context, connString string) error {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return fmt.Errorf("failed to create log shard pool: %w", err)
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.LogShards = append(sm.LogShards, pool)
	shardID := fmt.Sprintf("log_%d", len(sm.LogShards)-1)
	sm.health[shardID] = true

	return nil
}

// GetUserShard определяет шард для пользователя
func (sm *ShardManager) GetUserShard(userID int64) *pgxpool.Pool {
	if len(sm.UserShards) == 0 {
		return nil
	}

	// Консистентный хешинг на основе ID
	shardIndex := userID % int64(len(sm.UserShards))
	return sm.UserShards[shardIndex]
}

// GetLogShard определяет шард для логов
func (sm *ShardManager) GetLogShard(entityID string) *pgxpool.Pool {
	if len(sm.LogShards) == 0 {
		return nil
	}

	// Хеш на основе ID сущности
	hash := sha256.Sum256([]byte(entityID))
	shardIndex := hash[0] % byte(len(sm.LogShards))
	return sm.LogShards[shardIndex]
}

// GetRandomUserShard возвращает случайный доступный шард пользователей
func (sm *ShardManager) GetRandomUserShard() *pgxpool.Pool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var availableShards []*pgxpool.Pool
	for i, shard := range sm.UserShards {
		if sm.health[fmt.Sprintf("user_%d", i)] {
			availableShards = append(availableShards, shard)
		}
	}

	if len(availableShards) == 0 {
		return nil
	}

	return availableShards[rand.Intn(len(availableShards))]
}

// BroadcastMessage рассылает сообщение всем Redis нодам
func (sm *ShardManager) BroadcastMessage(ctx context.Context, channel, message string) error {
	for _, redis := range sm.RedisClients {
		if err := redis.Publish(ctx, channel, message).Err(); err != nil {
			// Логируем ошибку, но продолжаем рассылку
			continue
		}
	}
	return nil
}

// GetGlobalCache получает значение из глобального кэша
func (sm *ShardManager) GetGlobalCache(ctx context.Context, key string) (string, error) {
	// Пробуем получить из первой доступной Redis ноды
	for _, redis := range sm.RedisClients {
		val, err := redis.Get(ctx, key).Result()
		if err == nil {
			return val, nil
		}
	}
	return "", fmt.Errorf("key not found in any Redis node")
}

// SetGlobalCache устанавливает значение во всех Redis нодах
func (sm *ShardManager) SetGlobalCache(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	for _, redis := range sm.RedisClients {
		if err := redis.Set(ctx, key, value, expiration).Err(); err != nil {
			return err
		}
	}
	return nil
}

// HealthCheck проверяет здоровье всех шардов
func (sm *ShardManager) HealthCheck(ctx context.Context) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Проверяем пользовательские шарды
	for i, shard := range sm.UserShards {
		shardID := fmt.Sprintf("user_%d", i)
		if err := shard.Ping(ctx); err != nil {
			sm.health[shardID] = false
		} else {
			sm.health[shardID] = true
		}
	}

	// Проверяем шарды логов
	for i, shard := range sm.LogShards {
		shardID := fmt.Sprintf("log_%d", i)
		if err := shard.Ping(ctx); err != nil {
			sm.health[shardID] = false
		} else {
			sm.health[shardID] = true
		}
	}

	// Проверяем Redis
	for i, redis := range sm.RedisClients {
		redisID := fmt.Sprintf("redis_%d", i)
		if err := redis.Ping(ctx).Err(); err != nil {
			sm.health[redisID] = false
		} else {
			sm.health[redisID] = true
		}
	}
}

// GetStats возвращает статистику по шардам
func (sm *ShardManager) GetStats() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["user_shards"] = len(sm.UserShards)
	stats["log_shards"] = len(sm.LogShards)
	stats["redis_nodes"] = len(sm.RedisClients)
	stats["healthy_shards"] = 0

	for _, healthy := range sm.health {
		if healthy {
			stats["healthy_shards"] = stats["healthy_shards"].(int) + 1
		}
	}

	return stats
}

// Close закрывает все соединения
func (sm *ShardManager) Close() {
	for _, shard := range sm.UserShards {
		shard.Close()
	}
	for _, shard := range sm.LogShards {
		shard.Close()
	}
}

// ShardConfig содержит конфигурацию шардинга
type ShardConfig struct {
	UserShards []string `json:"user_shards"`
	LogShards  []string `json:"log_shards"`
	RedisNodes []string `json:"redis_nodes"`
}

// LoadFromConfig загружает шарды из конфигурации
func (sm *ShardManager) LoadFromConfig(ctx context.Context, cfg ShardConfig) error {
	// Добавляем пользовательские шарды
	for _, connStr := range cfg.UserShards {
		if err := sm.AddUserShard(ctx, connStr); err != nil {
			return fmt.Errorf("failed to add user shard: %w", err)
		}
	}

	// Добавляем шарды логов
	for _, connStr := range cfg.LogShards {
		if err := sm.AddLogShard(ctx, connStr); err != nil {
			return fmt.Errorf("failed to add log shard: %w", err)
		}
	}

	return nil
}
