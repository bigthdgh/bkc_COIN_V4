package cluster

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// DatabaseConfig полная конфигурация баз данных
type DatabaseConfig struct {
	// Neon базы для пользователей
	NeonShards []NeonShardConfig `json:"neon_shards"`

	// CockroachDB для логов
	CockroachShards []CockroachShardConfig `json:"cockroach_shards"`

	// Redis ноды
	RedisNodes []RedisConfig `json:"redis_nodes"`

	// Настройки пула соединений
	PoolSettings PoolSettings `json:"pool_settings"`

	// Настройки репликации
	ReplicationSettings ReplicationSettings `json:"replication_settings"`
}

// NeonShardConfig конфигурация Neon шарда
type NeonShardConfig struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Database    string `json:"database"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	SSLMode     string `json:"ssl_mode"`
	MaxSize     int    `json:"max_size"`
	MinSize     int    `json:"min_size"`
	Region      string `json:"region"`
	Description string `json:"description"`
}

// CockroachShardConfig конфигурация CockroachDB шарда
type CockroachShardConfig struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Database    string `json:"database"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	SSLMode     string `json:"ssl_mode"`
	MaxSize     int    `json:"max_size"`
	MinSize     int    `json:"min_size"`
	Region      string `json:"region"`
	Description string `json:"description"`
}

// RedisConfig конфигурация Redis ноды
type RedisConfig struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Password    string `json:"password"`
	Database    int    `json:"database"`
	MaxRetries  int    `json:"max_retries"`
	PoolSize    int    `json:"pool_size"`
	Region      string `json:"region"`
	Description string `json:"description"`
}

// PoolSettings настройки пулов соединений
type PoolSettings struct {
	MaxConnectionsPerShard int           `json:"max_connections_per_shard"`
	MinConnectionsPerShard int           `json:"min_connections_per_shard"`
	ConnectionTimeout      time.Duration `json:"connection_timeout"`
	IdleTimeout            time.Duration `json:"idle_timeout"`
	MaxLifetime            time.Duration `json:"max_lifetime"`
	HealthCheckPeriod      time.Duration `json:"health_check_period"`
}

// ReplicationSettings настройки репликации
type ReplicationSettings struct {
	EnableReplication     bool          `json:"enable_replication"`
	ReplicationFactor     int           `json:"replication_factor"`
	ReplicationTimeout    time.Duration `json:"replication_timeout"`
	ConsistencyLevel      string        `json:"consistency_level"`
	EnableBackups         bool          `json:"enable_backups"`
	BackupInterval        time.Duration `json:"backup_interval"`
	BackupRetentionPeriod time.Duration `json:"backup_retention_period"`
}

// GetProductionConfig возвращает конфигурацию для продакшена
func GetProductionConfig() DatabaseConfig {
	return DatabaseConfig{
		NeonShards: []NeonShardConfig{
			{
				ID:          0,
				Name:        "neon-users-0",
				Host:        getEnv("NEON_0_HOST", "ep-cool-darkness-123456.us-east-2.aws.neon.tech"),
				Port:        5432,
				Database:    getEnv("NEON_0_DB", "bkc_users_0"),
				Username:    getEnv("NEON_0_USER", "bkc_user"),
				Password:    getEnv("NEON_0_PASSWORD", ""),
				SSLMode:     "require",
				MaxSize:     20,
				MinSize:     5,
				Region:      "us-east-2",
				Description: "Users with ID ending in 0-1",
			},
			{
				ID:          1,
				Name:        "neon-users-1",
				Host:        getEnv("NEON_1_HOST", "ep-blue-silence-123456.us-east-2.aws.neon.tech"),
				Port:        5432,
				Database:    getEnv("NEON_1_DB", "bkc_users_1"),
				Username:    getEnv("NEON_1_USER", "bkc_user"),
				Password:    getEnv("NEON_1_PASSWORD", ""),
				SSLMode:     "require",
				MaxSize:     20,
				MinSize:     5,
				Region:      "us-east-2",
				Description: "Users with ID ending in 2-3",
			},
			{
				ID:          2,
				Name:        "neon-users-2",
				Host:        getEnv("NEON_2_HOST", "ep-green-forest-123456.us-east-2.aws.neon.tech"),
				Port:        5432,
				Database:    getEnv("NEON_2_DB", "bkc_users_2"),
				Username:    getEnv("NEON_2_USER", "bkc_user"),
				Password:    getEnv("NEON_2_PASSWORD", ""),
				SSLMode:     "require",
				MaxSize:     20,
				MinSize:     5,
				Region:      "us-east-2",
				Description: "Users with ID ending in 4-5",
			},
			{
				ID:          3,
				Name:        "neon-users-3",
				Host:        getEnv("NEON_3_HOST", "ep-red-ocean-123456.us-east-2.aws.neon.tech"),
				Port:        5432,
				Database:    getEnv("NEON_3_DB", "bkc_users_3"),
				Username:    getEnv("NEON_3_USER", "bkc_user"),
				Password:    getEnv("NEON_3_PASSWORD", ""),
				SSLMode:     "require",
				MaxSize:     20,
				MinSize:     5,
				Region:      "us-east-2",
				Description: "Users with ID ending in 6-7",
			},
			{
				ID:          4,
				Name:        "neon-users-4",
				Host:        getEnv("NEON_4_HOST", "ep-yellow-mountain-123456.us-east-2.aws.neon.tech"),
				Port:        5432,
				Database:    getEnv("NEON_4_DB", "bkc_users_4"),
				Username:    getEnv("NEON_4_USER", "bkc_user"),
				Password:    getEnv("NEON_4_PASSWORD", ""),
				SSLMode:     "require",
				MaxSize:     20,
				MinSize:     5,
				Region:      "us-east-2",
				Description: "Users with ID ending in 8-9",
			},
		},
		CockroachShards: []CockroachShardConfig{
			{
				ID:          0,
				Name:        "cockroach-logs-0",
				Host:        getEnv("COCKROACH_0_HOST", "free-tier.gcp-us-central1.cockroachlabs.cloud"),
				Port:        26257,
				Database:    getEnv("COCKROACH_0_DB", "bkc_logs_0"),
				Username:    getEnv("COCKROACH_0_USER", "bkc_user"),
				Password:    getEnv("COCKROACH_0_PASSWORD", ""),
				SSLMode:     "require",
				MaxSize:     15,
				MinSize:     3,
				Region:      "us-central1",
				Description: "Click history and transfers",
			},
			{
				ID:          1,
				Name:        "cockroach-logs-1",
				Host:        getEnv("COCKROACH_1_HOST", "free-tier.gcp-us-east1.cockroachlabs.cloud"),
				Port:        26257,
				Database:    getEnv("COCKROACH_1_DB", "bkc_logs_1"),
				Username:    getEnv("COCKROACH_1_USER", "bkc_user"),
				Password:    getEnv("COCKROACH_1_PASSWORD", ""),
				SSLMode:     "require",
				MaxSize:     15,
				MinSize:     3,
				Region:      "us-east1",
				Description: "Games history and marketplace logs",
			},
		},
		RedisNodes: []RedisConfig{
			{
				ID:          0,
				Name:        "redis-global-0",
				Host:        getEnv("REDIS_0_HOST", "redis-12345.c1.us-east-1-2.ec2.cloud.redislabs.com"),
				Port:        12345,
				Password:    getEnv("REDIS_0_PASSWORD", ""),
				Database:    0,
				MaxRetries:  3,
				PoolSize:    50,
				Region:      "us-east-1",
				Description: "Global online and Pub/Sub",
			},
			{
				ID:          1,
				Name:        "redis-global-1",
				Host:        getEnv("REDIS_1_HOST", "redis-12346.c1.us-east-1-2.ec2.cloud.redislabs.com"),
				Port:        12346,
				Password:    getEnv("REDIS_1_PASSWORD", ""),
				Database:    0,
				MaxRetries:  3,
				PoolSize:    50,
				Region:      "us-east-1",
				Description: "Global online and Pub/Sub backup",
			},
			{
				ID:          2,
				Name:        "redis-leaderboard-0",
				Host:        getEnv("REDIS_2_HOST", "redis-12347.c1.us-east-1-2.ec2.cloud.redislabs.com"),
				Port:        12347,
				Password:    getEnv("REDIS_2_PASSWORD", ""),
				Database:    1,
				MaxRetries:  3,
				PoolSize:    30,
				Region:      "us-east-1",
				Description: "Fast leaderboards",
			},
			{
				ID:          3,
				Name:        "redis-leaderboard-1",
				Host:        getEnv("REDIS_3_HOST", "redis-12348.c1.us-east-1-2.ec2.cloud.redislabs.com"),
				Port:        12348,
				Password:    getEnv("REDIS_3_PASSWORD", ""),
				Database:    1,
				MaxRetries:  3,
				PoolSize:    30,
				Region:      "us-east-1",
				Description: "Fast leaderboards backup",
			},
		},
		PoolSettings: PoolSettings{
			MaxConnectionsPerShard: 50,
			MinConnectionsPerShard: 5,
			ConnectionTimeout:      30 * time.Second,
			IdleTimeout:            5 * time.Minute,
			MaxLifetime:            1 * time.Hour,
			HealthCheckPeriod:      30 * time.Second,
		},
		ReplicationSettings: ReplicationSettings{
			EnableReplication:     true,
			ReplicationFactor:     2,
			ReplicationTimeout:    5 * time.Second,
			ConsistencyLevel:      "eventual",
			EnableBackups:         true,
			BackupInterval:        6 * time.Hour,
			BackupRetentionPeriod: 7 * 24 * time.Hour,
		},
	}
}

// GetDevelopmentConfig возвращает конфигурацию для разработки
func GetDevelopmentConfig() DatabaseConfig {
	return DatabaseConfig{
		NeonShards: []NeonShardConfig{
			{
				ID:          0,
				Name:        "dev-neon-0",
				Host:        getEnv("DEV_NEON_HOST", "localhost"),
				Port:        5432,
				Database:    getEnv("DEV_NEON_DB", "bkc_dev"),
				Username:    getEnv("DEV_NEON_USER", "postgres"),
				Password:    getEnv("DEV_NEON_PASSWORD", "postgres"),
				SSLMode:     "disable",
				MaxSize:     5,
				MinSize:     1,
				Region:      "local",
				Description: "Development single shard",
			},
		},
		CockroachShards: []CockroachShardConfig{
			{
				ID:          0,
				Name:        "dev-cockroach-0",
				Host:        getEnv("DEV_COCKROACH_HOST", "localhost"),
				Port:        26257,
				Database:    getEnv("DEV_COCKROACH_DB", "bkc_logs_dev"),
				Username:    getEnv("DEV_COCKROACH_USER", "root"),
				Password:    getEnv("DEV_COCKROACH_PASSWORD", ""),
				SSLMode:     "disable",
				MaxSize:     3,
				MinSize:     1,
				Region:      "local",
				Description: "Development logs",
			},
		},
		RedisNodes: []RedisConfig{
			{
				ID:          0,
				Name:        "dev-redis-0",
				Host:        getEnv("DEV_REDIS_HOST", "localhost"),
				Port:        6379,
				Password:    getEnv("DEV_REDIS_PASSWORD", ""),
				Database:    0,
				MaxRetries:  3,
				PoolSize:    10,
				Region:      "local",
				Description: "Development cache",
			},
		},
		PoolSettings: PoolSettings{
			MaxConnectionsPerShard: 10,
			MinConnectionsPerShard: 1,
			ConnectionTimeout:      10 * time.Second,
			IdleTimeout:            1 * time.Minute,
			MaxLifetime:            30 * time.Minute,
			HealthCheckPeriod:      10 * time.Second,
		},
		ReplicationSettings: ReplicationSettings{
			EnableReplication:     false,
			ReplicationFactor:     1,
			ReplicationTimeout:    1 * time.Second,
			ConsistencyLevel:      "strong",
			EnableBackups:         false,
			BackupInterval:        1 * time.Hour,
			BackupRetentionPeriod: 24 * time.Hour,
		},
	}
}

// BuildConnectionString строит строку подключения для Neon
func (nsc *NeonShardConfig) BuildConnectionString() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		nsc.Username,
		nsc.Password,
		nsc.Host,
		nsc.Port,
		nsc.Database,
		nsc.SSLMode,
	)
}

// BuildConnectionString строит строку подключения для CockroachDB
func (csc *CockroachShardConfig) BuildConnectionString() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		csc.Username,
		csc.Password,
		csc.Host,
		csc.Port,
		csc.Database,
		csc.SSLMode,
	)
}

// BuildRedisOptions строит опции для Redis
func (rc *RedisConfig) BuildRedisOptions() *redis.Options {
	return &redis.Options{
		Addr:       fmt.Sprintf("%s:%d", rc.Host, rc.Port),
		Password:   rc.Password,
		DB:         rc.Database,
		MaxRetries: rc.MaxRetries,
		PoolSize:   rc.PoolSize,
	}
}

// InitializeShardManager инициализирует ShardManager с конфигурацией
func InitializeShardManager(ctx context.Context, config DatabaseConfig) (*ShardManager, error) {
	sm := NewShardManager()

	// Добавление Neon шардов
	for _, shardConfig := range config.NeonShards {
		connString := shardConfig.BuildConnectionString()
		if err := sm.AddUserShard(ctx, connString); err != nil {
			log.Printf("Failed to add Neon shard %s: %v", shardConfig.Name, err)
			return nil, fmt.Errorf("failed to add Neon shard %s: %w", shardConfig.Name, err)
		}
		log.Printf("Added Neon shard: %s", shardConfig.Name)
	}

	// Добавление CockroachDB шардов
	for _, shardConfig := range config.CockroachShards {
		connString := shardConfig.BuildConnectionString()
		if err := sm.AddLogShard(ctx, connString); err != nil {
			log.Printf("Failed to add CockroachDB shard %s: %v", shardConfig.Name, err)
			return nil, fmt.Errorf("failed to add CockroachDB shard %s: %w", shardConfig.Name, err)
		}
		log.Printf("Added CockroachDB shard: %s", shardConfig.Name)
	}

	// Добавление Redis нод
	for _, redisConfig := range config.RedisNodes {
		client := redis.NewClient(redisConfig.BuildRedisOptions())
		if err := client.Ping(ctx).Err(); err != nil {
			log.Printf("Failed to connect to Redis %s: %v", redisConfig.Name, err)
			return nil, fmt.Errorf("failed to connect to Redis %s: %w", redisConfig.Name, err)
		}
		sm.RedisClients = append(sm.RedisClients, client)
		log.Printf("Added Redis node: %s", redisConfig.Name)
	}

	// Запуск health check
	go func() {
		ticker := time.NewTicker(config.PoolSettings.HealthCheckPeriod)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sm.HealthCheck(ctx)
			}
		}
	}()

	return sm, nil
}

// PrintConfiguration выводит конфигурацию в лог
func PrintConfiguration(config DatabaseConfig) {
	log.Println("=== Database Configuration ===")

	log.Printf("Neon Shards (%d):", len(config.NeonShards))
	for _, shard := range config.NeonShards {
		log.Printf("  - %s: %s@%s:%d/%s (%s)",
			shard.Name, shard.Username, shard.Host, shard.Port, shard.Database, shard.Description)
	}

	log.Printf("CockroachDB Shards (%d):", len(config.CockroachShards))
	for _, shard := range config.CockroachShards {
		log.Printf("  - %s: %s@%s:%d/%s (%s)",
			shard.Name, shard.Username, shard.Host, shard.Port, shard.Database, shard.Description)
	}

	log.Printf("Redis Nodes (%d):", len(config.RedisNodes))
	for _, node := range config.RedisNodes {
		log.Printf("  - %s: %s@%s:%d/%d (%s)",
			node.Name, node.Host, node.Port, node.Database, node.Description)
	}

	log.Printf("Pool Settings:")
	log.Printf("  Max connections per shard: %d", config.PoolSettings.MaxConnectionsPerShard)
	log.Printf("  Min connections per shard: %d", config.PoolSettings.MinConnectionsPerShard)
	log.Printf("  Connection timeout: %v", config.PoolSettings.ConnectionTimeout)

	log.Printf("Replication Settings:")
	log.Printf("  Enabled: %v", config.ReplicationSettings.EnableReplication)
	log.Printf("  Factor: %d", config.ReplicationSettings.ReplicationFactor)
	log.Printf("  Consistency: %s", config.ReplicationSettings.ConsistencyLevel)

	log.Println("=== End Configuration ===")
}

// getEnv получает переменную окружения или значение по умолчанию
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt получает переменную окружения как int или значение по умолчанию
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvBool получает переменную окружения как bool или значение по умолчанию
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

// ValidateConfig проверяет конфигурацию на валидность
func ValidateConfig(config DatabaseConfig) error {
	// Проверка Neon шардов
	if len(config.NeonShards) == 0 {
		return fmt.Errorf("at least one Neon shard is required")
	}

	for i, shard := range config.NeonShards {
		if shard.Host == "" {
			return fmt.Errorf("Neon shard %d: host is required", i)
		}
		if shard.Username == "" {
			return fmt.Errorf("Neon shard %d: username is required", i)
		}
		if shard.Database == "" {
			return fmt.Errorf("Neon shard %d: database is required", i)
		}
	}

	// Проверка CockroachDB шардов
	if len(config.CockroachShards) == 0 {
		return fmt.Errorf("at least one CockroachDB shard is required")
	}

	for i, shard := range config.CockroachShards {
		if shard.Host == "" {
			return fmt.Errorf("CockroachDB shard %d: host is required", i)
		}
		if shard.Username == "" {
			return fmt.Errorf("CockroachDB shard %d: username is required", i)
		}
		if shard.Database == "" {
			return fmt.Errorf("CockroachDB shard %d: database is required", i)
		}
	}

	// Проверка Redis нод
	if len(config.RedisNodes) == 0 {
		return fmt.Errorf("at least one Redis node is required")
	}

	for i, node := range config.RedisNodes {
		if node.Host == "" {
			return fmt.Errorf("Redis node %d: host is required", i)
		}
	}

	// Проверка настроек пула
	if config.PoolSettings.MaxConnectionsPerShard <= 0 {
		return fmt.Errorf("MaxConnectionsPerShard must be positive")
	}
	if config.PoolSettings.MinConnectionsPerShard < 0 {
		return fmt.Errorf("MinConnectionsPerShard cannot be negative")
	}
	if config.PoolSettings.MinConnectionsPerShard > config.PoolSettings.MaxConnectionsPerShard {
		return fmt.Errorf("MinConnectionsPerShard cannot be greater than MaxConnectionsPerShard")
	}

	return nil
}
