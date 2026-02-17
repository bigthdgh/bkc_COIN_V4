package memtap

import (
	"context"
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"bkc_coin_v2/internal/config"
	"bkc_coin_v2/internal/db"
)

type Engine struct {
	cfg config.Config
	db  *db.DB

	enabled bool

	flushInterval time.Duration
	systemRefresh time.Duration
	cacheTTL      time.Duration

	startOnce sync.Once

	mu sync.RWMutex

	systemLoadedAt time.Time
	reserve        int64
	initialReserve int64
	startRate      int64
	minRate        int64

	users map[int64]*userState

	pendingUsers   map[int64]pendingUserDelta
	pendingDaily   map[dailyKey]int64
	pendingReserve int64

	flushInFlight atomic.Bool
	lastFlushUnix atomic.Int64
	flushErrors   atomic.Int64
	flushCount    atomic.Int64
}

type userState struct {
	UserID    int64
	Username  string
	FirstName string

	Balance   int64
	TapsTotal int64

	Energy          float64
	EnergyMax       float64
	EnergyUpdatedAt time.Time

	BoostUntil    time.Time
	BoostRegenMul float64
	BoostMaxMul   float64

	Day         string
	DailyTapped int64
	DailyExtra  int64

	LastTouched time.Time
}

type pendingUserDelta struct {
	BalanceDelta int64
	TapsDelta    int64
	Energy       float64
	EnergyAt     time.Time
}

type dailyKey struct {
	UserID int64
	Day    string
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

type UserSnapshot struct {
	Balance        int64
	TapsTotal      int64
	Energy         int64
	EnergyMax      int64
	DailyTapped    int64
	DailyExtra     int64
	DailyRemaining int64
}

func New(cfg config.Config, database *db.DB) *Engine {
	if database == nil {
		return nil
	}
	enabled := strings.TrimSpace(os.Getenv("MEMTAP_ENABLED")) == "1"
	if !enabled {
		return nil
	}

	flushIntervalMs := envInt64("MEMTAP_FLUSH_INTERVAL_MS", 2000)
	if flushIntervalMs < 500 {
		flushIntervalMs = 500
	}
	if flushIntervalMs > 60_000 {
		flushIntervalMs = 60_000
	}
	systemRefreshSec := envInt64("MEMTAP_SYSTEM_REFRESH_SEC", 5)
	if systemRefreshSec < 1 {
		systemRefreshSec = 1
	}
	if systemRefreshSec > 300 {
		systemRefreshSec = 300
	}
	cacheTTLSec := envInt64("MEMTAP_CACHE_TTL_SEC", 900)
	if cacheTTLSec < 60 {
		cacheTTLSec = 60
	}
	if cacheTTLSec > 86_400 {
		cacheTTLSec = 86_400
	}

	return &Engine{
		cfg: cfg,
		db:  database,

		enabled: enabled,

		flushInterval: time.Duration(flushIntervalMs) * time.Millisecond,
		systemRefresh: time.Duration(systemRefreshSec) * time.Second,
		cacheTTL:      time.Duration(cacheTTLSec) * time.Second,

		users:        map[int64]*userState{},
		pendingUsers: map[int64]pendingUserDelta{},
		pendingDaily: map[dailyKey]int64{},
	}
}

func (e *Engine) Enabled() bool {
	return e != nil && e.enabled
}

func (e *Engine) Start(ctx context.Context) {
	if !e.Enabled() {
		return
	}
	e.startOnce.Do(func() {
		go e.loop(ctx)
	})
}

func (e *Engine) loop(ctx context.Context) {
	flushTicker := time.NewTicker(e.flushInterval)
	cleanupTicker := time.NewTicker(60 * time.Second)
	defer flushTicker.Stop()
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = e.Flush(context.Background())
			return
		case <-flushTicker.C:
			_ = e.Flush(ctx)
		case <-cleanupTicker.C:
			e.cleanupStaleUsers()
		}
	}
}

func (e *Engine) ensureSystem(ctx context.Context) error {
	if !e.Enabled() {
		return errors.New("memtap disabled")
	}

	e.mu.RLock()
	loaded := !e.systemLoadedAt.IsZero() && time.Since(e.systemLoadedAt) < e.systemRefresh
	e.mu.RUnlock()
	if loaded {
		return nil
	}

	sys, err := e.db.GetSystem(ctx)
	if err != nil {
		return err
	}

	e.mu.Lock()
	// Keep pending in-memory delta on top of DB reserve value.
	e.reserve = sys.ReserveSupply + e.pendingReserve
	e.initialReserve = sys.InitialReserve
	e.startRate = sys.StartRateCoinsUSD
	e.minRate = sys.MinRateCoinsUSD
	e.systemLoadedAt = time.Now().UTC()
	e.mu.Unlock()
	return nil
}

func (e *Engine) ensureUser(ctx context.Context, userID int64, username, firstName string, now time.Time) (*userState, error) {
	e.mu.RLock()
	if u := e.users[userID]; u != nil {
		e.mu.RUnlock()
		return u, nil
	}
	e.mu.RUnlock()

	dbUser, err := e.db.EnsureUser(ctx, userID, username, firstName, float64(e.cfg.EnergyMax))
	if err != nil {
		return nil, err
	}
	ud, err := e.db.GetUserDaily(ctx, userID, now)
	if err != nil {
		return nil, err
	}

	day := now.UTC().Format("2006-01-02")
	loaded := &userState{
		UserID:    dbUser.UserID,
		Username:  dbUser.Username,
		FirstName: dbUser.FirstName,

		Balance:   dbUser.Balance,
		TapsTotal: dbUser.TapsTotal,

		Energy:          dbUser.Energy,
		EnergyMax:       dbUser.EnergyMax,
		EnergyUpdatedAt: dbUser.EnergyUpdatedAt.UTC(),

		BoostUntil:    dbUser.EnergyBoostUntil.UTC(),
		BoostRegenMul: dbUser.EnergyBoostRegenMultiplier,
		BoostMaxMul:   dbUser.EnergyBoostMaxMultiplier,

		Day:         day,
		DailyTapped: ud.Tapped,
		DailyExtra:  ud.ExtraQuota,

		LastTouched: now.UTC(),
	}
	if loaded.EnergyMax <= 0 {
		loaded.EnergyMax = float64(e.cfg.EnergyMax)
	}
	if loaded.BoostRegenMul <= 0 {
		loaded.BoostRegenMul = 1
	}
	if loaded.BoostMaxMul <= 0 {
		loaded.BoostMaxMul = 1
	}

	e.mu.Lock()
	if existing := e.users[userID]; existing != nil {
		if strings.TrimSpace(username) != "" {
			existing.Username = username
		}
		if strings.TrimSpace(firstName) != "" {
			existing.FirstName = firstName
		}
		existing.LastTouched = now.UTC()
		e.mu.Unlock()
		return existing, nil
	}
	e.users[userID] = loaded
	e.mu.Unlock()
	return loaded, nil
}

func (e *Engine) Tap(ctx context.Context, userID int64, username, firstName string, requested int64, now time.Time) (TapResult, error) {
	if !e.Enabled() {
		return TapResult{}, errors.New("memtap disabled")
	}
	if userID <= 0 {
		return TapResult{}, errors.New("bad user_id")
	}
	if requested <= 0 {
		requested = 1
	}
	if requested > e.cfg.TapMaxPerRequest {
		requested = e.cfg.TapMaxPerRequest
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	if err := e.ensureSystem(ctx); err != nil {
		return TapResult{}, err
	}
	if _, err := e.ensureUser(ctx, userID, username, firstName, now); err != nil {
		return TapResult{}, err
	}

	day := now.Format("2006-01-02")

	e.mu.Lock()
	defer e.mu.Unlock()

	u := e.users[userID]
	if u == nil {
		return TapResult{}, errors.New("user not cached")
	}

	if u.Day != day {
		u.Day = day
		u.DailyTapped = 0
		u.DailyExtra = 0
	}

	regen, eMax := energyParams(u, now, e.cfg.EnergyRegenPerSec)
	u.Energy = regenEnergy(u.Energy, eMax, regen, u.EnergyUpdatedAt, now)
	u.EnergyUpdatedAt = now

	mintable := int64(math.Floor(u.Energy))
	if mintable < 0 {
		mintable = 0
	}

	dailyMax := e.cfg.TapDailyLimit + u.DailyExtra
	dailyRemaining := int64(1 << 60)
	if e.cfg.TapDailyLimit > 0 {
		dailyRemaining = dailyMax - u.DailyTapped
		if dailyRemaining < 0 {
			dailyRemaining = 0
		}
	}

	availableReserve := e.reserve
	if availableReserve < 0 {
		availableReserve = 0
	}

	gained := min4(requested, mintable, dailyRemaining, availableReserve)
	if gained < 0 {
		gained = 0
	}

	reason := "ok"
	if gained == 0 {
		switch {
		case e.cfg.TapDailyLimit > 0 && dailyRemaining == 0 && mintable > 0:
			reason = "daily_limit"
		case availableReserve == 0 && mintable > 0:
			reason = "reserve_empty"
		case mintable <= 0:
			reason = "no_energy"
		default:
			reason = "zero"
		}
	}

	if gained > 0 {
		u.Energy -= float64(gained)
		if u.Energy < 0 {
			u.Energy = 0
		}
		u.Balance += gained
		u.TapsTotal += gained
		u.DailyTapped += gained
		u.LastTouched = now

		e.reserve -= gained
		e.pendingReserve -= gained

		pu := e.pendingUsers[userID]
		pu.BalanceDelta += gained
		pu.TapsDelta += gained
		pu.Energy = u.Energy
		pu.EnergyAt = now
		e.pendingUsers[userID] = pu

		dk := dailyKey{UserID: userID, Day: day}
		e.pendingDaily[dk] += gained
	}

	dailyRemainingOut := dailyMax - u.DailyTapped
	if dailyRemainingOut < 0 {
		dailyRemainingOut = 0
	}

	return TapResult{
		Gained:         gained,
		Reason:         reason,
		Energy:         int64(math.Floor(u.Energy)),
		EnergyMax:      int64(math.Floor(eMax)),
		DailyTapped:    u.DailyTapped,
		DailyExtra:     u.DailyExtra,
		DailyRemaining: dailyRemainingOut,
	}, nil
}

// SnapshotIfPending returns current in-memory state only when user has
// unflushed tap deltas. This avoids overriding fresh DB data after flush.
func (e *Engine) SnapshotIfPending(userID int64, now time.Time) (UserSnapshot, bool) {
	if !e.Enabled() || userID <= 0 {
		return UserSnapshot{}, false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.pendingUsers[userID]; !ok {
		return UserSnapshot{}, false
	}
	u := e.users[userID]
	if u == nil {
		return UserSnapshot{}, false
	}
	day := now.Format("2006-01-02")
	if u.Day != day {
		u.Day = day
		u.DailyTapped = 0
		u.DailyExtra = 0
	}
	regen, eMax := energyParams(u, now, e.cfg.EnergyRegenPerSec)
	u.Energy = regenEnergy(u.Energy, eMax, regen, u.EnergyUpdatedAt, now)
	u.EnergyUpdatedAt = now

	dailyMax := e.cfg.TapDailyLimit + u.DailyExtra
	dailyRemaining := dailyMax - u.DailyTapped
	if dailyRemaining < 0 {
		dailyRemaining = 0
	}

	return UserSnapshot{
		Balance:        u.Balance,
		TapsTotal:      u.TapsTotal,
		Energy:         int64(math.Floor(u.Energy)),
		EnergyMax:      int64(math.Floor(eMax)),
		DailyTapped:    u.DailyTapped,
		DailyExtra:     u.DailyExtra,
		DailyRemaining: dailyRemaining,
	}, true
}

// ReserveSnapshotIfPending returns reserve/rate params when there is an
// unflushed reserve delta.
func (e *Engine) ReserveSnapshotIfPending() (reserve, initialReserve, startRate, minRate int64, ok bool) {
	if !e.Enabled() {
		return 0, 0, 0, 0, false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.pendingReserve == 0 {
		return 0, 0, 0, 0, false
	}
	return e.reserve, e.initialReserve, e.startRate, e.minRate, true
}

func (e *Engine) Flush(ctx context.Context) error {
	if !e.Enabled() {
		return nil
	}
	if !e.flushInFlight.CompareAndSwap(false, true) {
		return nil
	}
	defer e.flushInFlight.Store(false)

	users, daily, reserveDelta := e.snapshotPending()
	if len(users) == 0 && len(daily) == 0 && reserveDelta == 0 {
		return nil
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctxTimeout, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	if err := e.db.ApplyTapAggregates(ctxTimeout, users, daily, reserveDelta, "memtap"); err != nil {
		e.mergePending(users, daily, reserveDelta)
		e.flushErrors.Add(1)
		return err
	}

	e.lastFlushUnix.Store(time.Now().UTC().Unix())
	e.flushCount.Add(1)
	return nil
}

func (e *Engine) snapshotPending() ([]db.UserTapAggregate, []db.DailyTapAggregate, int64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	users := make([]db.UserTapAggregate, 0, len(e.pendingUsers))
	for userID, delta := range e.pendingUsers {
		if userID <= 0 {
			continue
		}
		users = append(users, db.UserTapAggregate{
			UserID:          userID,
			BalanceDelta:    delta.BalanceDelta,
			TapsDelta:       delta.TapsDelta,
			Energy:          delta.Energy,
			EnergyUpdatedAt: delta.EnergyAt,
		})
	}

	daily := make([]db.DailyTapAggregate, 0, len(e.pendingDaily))
	for k, tapped := range e.pendingDaily {
		if k.UserID <= 0 || strings.TrimSpace(k.Day) == "" || tapped == 0 {
			continue
		}
		daily = append(daily, db.DailyTapAggregate{
			UserID:      k.UserID,
			Day:         k.Day,
			TappedDelta: tapped,
		})
	}

	reserveDelta := e.pendingReserve
	e.pendingUsers = map[int64]pendingUserDelta{}
	e.pendingDaily = map[dailyKey]int64{}
	e.pendingReserve = 0

	return users, daily, reserveDelta
}

func (e *Engine) mergePending(users []db.UserTapAggregate, daily []db.DailyTapAggregate, reserveDelta int64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, u := range users {
		if u.UserID <= 0 {
			continue
		}
		d := e.pendingUsers[u.UserID]
		d.BalanceDelta += u.BalanceDelta
		d.TapsDelta += u.TapsDelta
		d.Energy = u.Energy
		if u.EnergyUpdatedAt.After(d.EnergyAt) {
			d.EnergyAt = u.EnergyUpdatedAt
		}
		e.pendingUsers[u.UserID] = d
	}

	for _, d := range daily {
		if d.UserID <= 0 || strings.TrimSpace(d.Day) == "" || d.TappedDelta == 0 {
			continue
		}
		key := dailyKey{UserID: d.UserID, Day: d.Day}
		e.pendingDaily[key] += d.TappedDelta
	}

	e.pendingReserve += reserveDelta
}

func (e *Engine) InvalidateUser(userID int64) {
	if !e.Enabled() || userID <= 0 {
		return
	}
	e.mu.Lock()
	delete(e.users, userID)
	delete(e.pendingUsers, userID)
	for k := range e.pendingDaily {
		if k.UserID == userID {
			delete(e.pendingDaily, k)
		}
	}
	e.mu.Unlock()
}

func (e *Engine) MarkSystemDirty() {
	if !e.Enabled() {
		return
	}
	e.mu.Lock()
	e.systemLoadedAt = time.Time{}
	e.mu.Unlock()
}

func (e *Engine) cleanupStaleUsers() {
	if !e.Enabled() {
		return
	}
	cutoff := time.Now().UTC().Add(-e.cacheTTL)
	e.mu.Lock()
	for userID, u := range e.users {
		if u == nil {
			delete(e.users, userID)
			continue
		}
		if _, hasPending := e.pendingUsers[userID]; hasPending {
			continue
		}
		if u.LastTouched.Before(cutoff) {
			delete(e.users, userID)
		}
	}
	e.mu.Unlock()
}

func (e *Engine) Stats() map[string]any {
	out := map[string]any{
		"enabled": e.Enabled(),
	}
	if !e.Enabled() {
		return out
	}

	e.mu.RLock()
	out["cached_users"] = len(e.users)
	out["pending_users"] = len(e.pendingUsers)
	out["pending_daily"] = len(e.pendingDaily)
	out["pending_reserve_delta"] = e.pendingReserve
	out["reserve_cached"] = e.reserve
	out["system_loaded"] = !e.systemLoadedAt.IsZero()
	if !e.systemLoadedAt.IsZero() {
		out["system_loaded_at"] = e.systemLoadedAt.Unix()
	}
	e.mu.RUnlock()

	out["flush_in_flight"] = e.flushInFlight.Load()
	out["last_flush_ts"] = e.lastFlushUnix.Load()
	out["flush_count"] = e.flushCount.Load()
	out["flush_errors"] = e.flushErrors.Load()
	return out
}

func energyParams(u *userState, now time.Time, baseRegen float64) (regen float64, eMax float64) {
	eMax = u.EnergyMax
	if eMax <= 0 {
		eMax = 0
	}
	regen = baseRegen
	if regen < 0 {
		regen = 0
	}
	if now.Before(u.BoostUntil) {
		regen = regen * u.BoostRegenMul
		eMax = eMax * u.BoostMaxMul
	}
	if regen < 0 {
		regen = 0
	}
	if eMax < 0 {
		eMax = 0
	}
	return regen, eMax
}

func regenEnergy(current float64, eMax float64, regenPerSec float64, updatedAt, now time.Time) float64 {
	if eMax <= 0 {
		return 0
	}
	if current < 0 {
		current = 0
	}
	dt := now.Sub(updatedAt).Seconds()
	if dt > 0 {
		current = current + dt*regenPerSec
	}
	if current > eMax {
		current = eMax
	}
	return current
}

func min4(a, b, c, d int64) int64 {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	if d < m {
		m = d
	}
	return m
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
