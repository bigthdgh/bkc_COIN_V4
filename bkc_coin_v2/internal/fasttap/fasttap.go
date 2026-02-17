package fasttap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"bkc_coin_v2/internal/config"
	"bkc_coin_v2/internal/db"

	"github.com/redis/go-redis/v9"
)

// Engine accelerates the "tap" hot path by using Redis for:
// - energy regen + multi-tap limits (per user)
// - daily tap limit counters (per user per day)
// - reserve checks (global system state)
// and then emits events into a Redis Stream for async Postgres persistence.
//
// Important: this is a performance experiment. On free hosting/DB tiers, large "concurrent tappers"
// numbers are still limited by CPU, network and provider quotas.
type Engine struct {
	Cfg config.Config
	DB  *db.DB
	Rdb *redis.Client

	SysKey         string
	StreamKey      string
	StreamGroup    string
	StreamConsumer string
	StreamMaxLen   int64

	WorkerCount       int
	ReadCount         int64
	ReadBlock         time.Duration
	ApplyBatchSize    int
	ClaimMinIdle      time.Duration
	ClaimCount        int64
	ClaimEvery        time.Duration
	ClaimMaxRounds    int
	HealthPendingScan int64

	scriptTap *redis.Script
}

type TapResult struct {
	Gained         int64
	Reason         string
	Energy         int64
	EnergyMax      int64
	DailyTapped    int64
	DailyExtra     int64
	DailyRemaining int64
}

func Connect(ctx context.Context, redisURL string) (*redis.Client, error) {
	redisURL = normalizeRedisURL(redisURL)
	if redisURL == "" {
		return nil, nil
	}
	// Accept both "redis://..." and host:port formats.
	if !strings.Contains(redisURL, "://") {
		redisURL = "redis://" + redisURL
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	rdb := redis.NewClient(opt)
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, err
	}
	return rdb, nil
}

func normalizeRedisURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return s
	}

	// Redis providers often show connection as a CLI command, e.g.:
	//   redis-cli -u redis://default:<pass>@host:port
	// Accept that too by extracting the URL portion.
	if i := strings.Index(s, "rediss://"); i >= 0 {
		s = s[i:]
	} else if i := strings.Index(s, "redis://"); i >= 0 {
		s = s[i:]
	}

	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	if i := strings.IndexAny(s, " \t\r\n"); i >= 0 {
		s = strings.Trim(s[:i], `"'`)
	}
	return s
}

func New(cfg config.Config, database *db.DB, rdb *redis.Client) *Engine {
	if rdb == nil {
		return nil
	}
	consumer := strings.TrimSpace(os.Getenv("REDIS_STREAM_CONSUMER"))
	if consumer == "" {
		consumer = "c-" + randomHex(6)
	}
	streamKey := strings.TrimSpace(os.Getenv("REDIS_STREAM_KEY"))
	if streamKey == "" {
		streamKey = "bkc:stream:taps"
	}
	group := strings.TrimSpace(os.Getenv("REDIS_STREAM_GROUP"))
	if group == "" {
		group = "bkc"
	}
	maxLen := envInt64("REDIS_STREAM_MAXLEN", 500_000)
	if maxLen < 10_000 {
		maxLen = 10_000
	}
	workerCount := int(envInt64("REDIS_WORKER_COUNT", 2))
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > 32 {
		workerCount = 32
	}
	readCount := envInt64("REDIS_STREAM_READ_COUNT", 500)
	if readCount < 50 {
		readCount = 50
	}
	if readCount > 5000 {
		readCount = 5000
	}
	readBlockMs := envInt64("REDIS_STREAM_READ_BLOCK_MS", 1500)
	if readBlockMs < 100 {
		readBlockMs = 100
	}
	applyBatch := int(envInt64("REDIS_STREAM_APPLY_BATCH", 500))
	if applyBatch < 50 {
		applyBatch = 50
	}
	if applyBatch > 5000 {
		applyBatch = 5000
	}
	claimMinIdleSec := envInt64("REDIS_STREAM_CLAIM_MIN_IDLE_SEC", 60)
	if claimMinIdleSec < 5 {
		claimMinIdleSec = 5
	}
	claimCount := envInt64("REDIS_STREAM_CLAIM_COUNT", 200)
	if claimCount < 10 {
		claimCount = 10
	}
	if claimCount > 2000 {
		claimCount = 2000
	}
	claimEverySec := envInt64("REDIS_STREAM_CLAIM_EVERY_SEC", 30)
	if claimEverySec < 5 {
		claimEverySec = 5
	}
	claimMaxRounds := int(envInt64("REDIS_STREAM_CLAIM_MAX_ROUNDS", 4))
	if claimMaxRounds < 1 {
		claimMaxRounds = 1
	}
	if claimMaxRounds > 50 {
		claimMaxRounds = 50
	}
	healthPendingScan := envInt64("REDIS_HEALTH_PENDING_SCAN", 20)
	if healthPendingScan < 0 {
		healthPendingScan = 0
	}
	if healthPendingScan > 1000 {
		healthPendingScan = 1000
	}

	e := &Engine{
		Cfg:               cfg,
		DB:                database,
		Rdb:               rdb,
		SysKey:            "bkc:sys",
		StreamKey:         streamKey,
		StreamGroup:       group,
		StreamConsumer:    consumer,
		StreamMaxLen:      maxLen,
		WorkerCount:       workerCount,
		ReadCount:         readCount,
		ReadBlock:         time.Duration(readBlockMs) * time.Millisecond,
		ApplyBatchSize:    applyBatch,
		ClaimMinIdle:      time.Duration(claimMinIdleSec) * time.Second,
		ClaimCount:        claimCount,
		ClaimEvery:        time.Duration(claimEverySec) * time.Second,
		ClaimMaxRounds:    claimMaxRounds,
		HealthPendingScan: healthPendingScan,
		scriptTap:         redis.NewScript(tapLua),
	}
	return e
}

func (e *Engine) Enabled() bool { return e != nil && e.Rdb != nil }

func (e *Engine) EnsureSystemCached(ctx context.Context) error {
	if !e.Enabled() {
		return nil
	}
	exists, err := e.Rdb.Exists(ctx, e.SysKey).Result()
	if err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	sys, err := e.DB.GetSystem(ctx)
	if err != nil {
		return err
	}
	// Seed from DB once. After that, taps and other operations should adjust the hash by delta.
	return e.Rdb.HSet(ctx, e.SysKey,
		"total_supply", sys.TotalSupply,
		"reserve_supply", sys.ReserveSupply,
		"reserved_supply", sys.ReservedSupply,
		"initial_reserve", sys.InitialReserve,
		"start_rate", sys.StartRateCoinsUSD,
		"min_rate", sys.MinRateCoinsUSD,
	).Err()
}

func (e *Engine) userKey(userID int64) string {
	return fmt.Sprintf("bkc:u:%d", userID)
}

func (e *Engine) dailyKey(userID int64, day time.Time) string {
	day = day.UTC()
	return fmt.Sprintf("bkc:ud:%d:%s", userID, day.Format("2006-01-02"))
}

func dayUTC(t time.Time) time.Time {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func (e *Engine) EnsureUserCached(ctx context.Context, userID int64, username, firstName string, now time.Time) error {
	if !e.Enabled() {
		return nil
	}
	if userID <= 0 {
		return errors.New("bad user_id")
	}
	key := e.userKey(userID)
	exists, err := e.Rdb.Exists(ctx, key).Result()
	if err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}

	u, err := e.DB.EnsureUser(ctx, userID, username, firstName, float64(e.Cfg.EnergyMax))
	if err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	// Cache energy state.
	if err := e.Rdb.HSet(ctx, key,
		"energy", u.Energy,
		"energy_max", u.EnergyMax,
		"energy_updated_at", u.EnergyUpdatedAt.UTC().Unix(),
		"boost_until", u.EnergyBoostUntil.UTC().Unix(),
		"boost_regen_mult", u.EnergyBoostRegenMultiplier,
		"boost_max_mult", u.EnergyBoostMaxMultiplier,
	).Err(); err != nil {
		return err
	}

	// Cache daily counters for today (prevents easy bypass if Redis restarts mid-day).
	if e.Cfg.TapDailyLimit > 0 {
		ud, err := e.DB.GetUserDaily(ctx, userID, now)
		if err != nil {
			return err
		}
		dk := e.dailyKey(userID, dayUTC(now))
		ttlSec := int64(72 * 3600)
		pipe := e.Rdb.Pipeline()
		pipe.HSet(ctx, dk, "tapped", ud.Tapped, "extra_quota", ud.ExtraQuota)
		pipe.Expire(ctx, dk, time.Duration(ttlSec)*time.Second)
		_, err = pipe.Exec(ctx)
		return err
	}

	return nil
}

func (e *Engine) Tap(ctx context.Context, userID int64, requested int64, now time.Time) (TapResult, error) {
	if !e.Enabled() {
		return TapResult{}, errors.New("fasttap disabled")
	}
	if requested <= 0 {
		requested = 1
	}
	if requested > e.Cfg.TapMaxPerRequest {
		requested = e.Cfg.TapMaxPerRequest
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	day := dayUTC(now)

	userKey := e.userKey(userID)
	dailyKey := e.dailyKey(userID, day)

	ttlSec := int64(72 * 3600)
	coinPerTap := int64(1)

	out, err := e.scriptTap.Run(ctx, e.Rdb,
		[]string{userKey, dailyKey, e.SysKey, e.StreamKey},
		now.Unix(),
		requested,
		fmt.Sprintf("%.6f", e.Cfg.EnergyRegenPerSec),
		e.Cfg.TapDailyLimit,
		e.Cfg.EnergyMax,
		e.StreamMaxLen,
		coinPerTap,
		userID,
		day.Format("2006-01-02"),
		ttlSec,
	).Result()
	if err != nil {
		return TapResult{}, err
	}

	parts, ok := out.([]interface{})
	if !ok || len(parts) < 7 {
		return TapResult{}, errors.New("bad tap response")
	}

	getI64 := func(i int) int64 {
		if i < 0 || i >= len(parts) {
			return 0
		}
		switch v := parts[i].(type) {
		case int64:
			return v
		case string:
			n, _ := strconv.ParseInt(v, 10, 64)
			return n
		case []byte:
			n, _ := strconv.ParseInt(string(v), 10, 64)
			return n
		default:
			return 0
		}
	}
	getStr := func(i int) string {
		if i < 0 || i >= len(parts) {
			return ""
		}
		switch v := parts[i].(type) {
		case string:
			return v
		case []byte:
			return string(v)
		default:
			return ""
		}
	}

	res := TapResult{
		Gained:         getI64(0),
		Reason:         getStr(1),
		Energy:         getI64(2),
		EnergyMax:      getI64(3),
		DailyTapped:    getI64(4),
		DailyExtra:     getI64(5),
		DailyRemaining: getI64(6),
	}
	if res.Gained < 0 {
		res.Gained = 0
	}
	if res.Energy < 0 {
		res.Energy = 0
	}
	if res.EnergyMax < 0 {
		res.EnergyMax = 0
	}
	if res.Energy > res.EnergyMax && res.EnergyMax > 0 {
		res.Energy = res.EnergyMax
	}
	if res.DailyRemaining < 0 {
		res.DailyRemaining = 0
	}
	return res, nil
}

func (e *Engine) UpdateEnergyBoost(ctx context.Context, userID int64, until time.Time, regenMult, maxMult float64, energy float64, energyMax float64, now time.Time) error {
	if !e.Enabled() {
		return nil
	}
	key := e.userKey(userID)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if until.IsZero() {
		until = time.Unix(0, 0).UTC()
	}
	if regenMult <= 0 {
		regenMult = 1
	}
	if maxMult <= 0 {
		maxMult = 1
	}
	if energyMax <= 0 {
		energyMax = float64(e.Cfg.EnergyMax)
	}
	energy = math.Max(0, math.Min(energy, energyMax*maxMult))
	return e.Rdb.HSet(ctx, key,
		"boost_until", until.UTC().Unix(),
		"boost_regen_mult", regenMult,
		"boost_max_mult", maxMult,
		"energy", energy,
		"energy_max", energyMax,
		"energy_updated_at", now.UTC().Unix(),
	).Err()
}

func (e *Engine) AddDailyExtraQuota(ctx context.Context, userID int64, day time.Time, extra int64) error {
	if !e.Enabled() || extra == 0 || userID <= 0 {
		return nil
	}
	day = dayUTC(day)
	dk := e.dailyKey(userID, day)
	pipe := e.Rdb.Pipeline()
	pipe.HIncrBy(ctx, dk, "extra_quota", extra)
	pipe.Expire(ctx, dk, 72*time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}

func (e *Engine) AdjustReserve(ctx context.Context, delta int64) error {
	if !e.Enabled() || delta == 0 {
		return nil
	}
	return e.Rdb.HIncrBy(ctx, e.SysKey, "reserve_supply", delta).Err()
}

func (e *Engine) AdjustReserved(ctx context.Context, delta int64) error {
	if !e.Enabled() || delta == 0 {
		return nil
	}
	return e.Rdb.HIncrBy(ctx, e.SysKey, "reserved_supply", delta).Err()
}

func (e *Engine) QueueStats(ctx context.Context) map[string]any {
	out := map[string]any{
		"enabled":      e.Enabled(),
		"stream_key":   e.StreamKey,
		"stream_group": e.StreamGroup,
	}
	if !e.Enabled() {
		return out
	}
	if xlen, err := e.Rdb.XLen(ctx, e.StreamKey).Result(); err == nil {
		out["stream_len"] = xlen
	}
	if p, err := e.Rdb.XPending(ctx, e.StreamKey, e.StreamGroup).Result(); err == nil {
		out["pending_count"] = p.Count
		out["pending_consumers"] = len(p.Consumers)
	}
	if e.HealthPendingScan > 0 {
		if pe, err := e.Rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream: e.StreamKey,
			Group:  e.StreamGroup,
			Start:  "-",
			End:    "+",
			Count:  e.HealthPendingScan,
		}).Result(); err == nil {
			out["pending_sample"] = len(pe)
		}
	}
	return out
}

func envInt64(key string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func randomHex(n int) string {
	if n <= 0 {
		n = 6
	}
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
