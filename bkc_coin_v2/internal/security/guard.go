package security

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Enabled bool

	MaxBodyBytes int64

	APIRate  float64
	APIBurst float64

	PublicRate  float64
	PublicBurst float64

	TapIPRate  float64
	TapIPBurst float64

	TapUserRate  float64
	TapUserBurst float64

	AuthFailWindowSec int64
	AuthFailThreshold int
	BanSec            int64

	EntryTTLMin int64
}

type Guard struct {
	cfg Config

	mu sync.Mutex

	ipBuckets      map[string]*bucket
	tapUserBuckets map[int64]*bucket
	authFails      map[string]*failState
	bannedUntil    map[string]time.Time

	lastCleanup time.Time
}

type bucket struct {
	Tokens   float64
	LastSeen time.Time
}

type failState struct {
	Count      int
	WindowFrom time.Time
	LastSeen   time.Time
}

func NewFromEnv() *Guard {
	cfg := Config{
		Enabled: envBool("SECURITY_ENABLED", true),

		MaxBodyBytes: envInt64("SECURITY_MAX_BODY_BYTES", 64*1024),

		APIRate:  envFloat64("SECURITY_API_RATE", 120),
		APIBurst: envFloat64("SECURITY_API_BURST", 240),

		PublicRate:  envFloat64("SECURITY_PUBLIC_RATE", 40),
		PublicBurst: envFloat64("SECURITY_PUBLIC_BURST", 80),

		TapIPRate:  envFloat64("SECURITY_TAP_IP_RATE", 80),
		TapIPBurst: envFloat64("SECURITY_TAP_IP_BURST", 200),

		TapUserRate:  envFloat64("SECURITY_TAP_USER_RATE", 35),
		TapUserBurst: envFloat64("SECURITY_TAP_USER_BURST", 120),

		AuthFailWindowSec: envInt64("SECURITY_AUTH_FAIL_WINDOW_SEC", 20),
		AuthFailThreshold: int(envInt64("SECURITY_AUTH_FAIL_THRESHOLD", 40)),
		BanSec:            envInt64("SECURITY_BAN_SEC", 120),

		EntryTTLMin: envInt64("SECURITY_ENTRY_TTL_MIN", 15),
	}
	if cfg.MaxBodyBytes < 4096 {
		cfg.MaxBodyBytes = 4096
	}
	if cfg.APIRate < 1 {
		cfg.APIRate = 1
	}
	if cfg.APIBurst < cfg.APIRate {
		cfg.APIBurst = cfg.APIRate * 2
	}
	if cfg.PublicRate < 1 {
		cfg.PublicRate = 1
	}
	if cfg.PublicBurst < cfg.PublicRate {
		cfg.PublicBurst = cfg.PublicRate * 2
	}
	if cfg.TapIPRate < 1 {
		cfg.TapIPRate = 1
	}
	if cfg.TapIPBurst < cfg.TapIPRate {
		cfg.TapIPBurst = cfg.TapIPRate * 2
	}
	if cfg.TapUserRate < 1 {
		cfg.TapUserRate = 1
	}
	if cfg.TapUserBurst < cfg.TapUserRate {
		cfg.TapUserBurst = cfg.TapUserRate * 2
	}
	if cfg.AuthFailWindowSec < 1 {
		cfg.AuthFailWindowSec = 1
	}
	if cfg.AuthFailThreshold < 1 {
		cfg.AuthFailThreshold = 1
	}
	if cfg.BanSec < 1 {
		cfg.BanSec = 1
	}
	if cfg.EntryTTLMin < 1 {
		cfg.EntryTTLMin = 1
	}

	return &Guard{
		cfg:            cfg,
		ipBuckets:      map[string]*bucket{},
		tapUserBuckets: map[int64]*bucket{},
		authFails:      map[string]*failState{},
		bannedUntil:    map[string]time.Time{},
		lastCleanup:    time.Now().UTC(),
	}
}

func (g *Guard) Enabled() bool {
	return g != nil && g.cfg.Enabled
}

func (g *Guard) MaxBodyBytes() int64 {
	if g == nil {
		return 0
	}
	return g.cfg.MaxBodyBytes
}

func (g *Guard) ClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	get := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}
		if strings.Contains(s, ",") {
			s = strings.TrimSpace(strings.Split(s, ",")[0])
		}
		return s
	}
	if ip := get(r.Header.Get("CF-Connecting-IP")); ip != "" {
		return ip
	}
	if ip := get(r.Header.Get("X-Forwarded-For")); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func (g *Guard) IsBanned(ip string) bool {
	if !g.Enabled() {
		return false
	}
	if strings.TrimSpace(ip) == "" {
		return false
	}
	now := time.Now().UTC()
	g.mu.Lock()
	defer g.mu.Unlock()
	until, ok := g.bannedUntil[ip]
	if !ok {
		return false
	}
	if now.After(until) {
		delete(g.bannedUntil, ip)
		return false
	}
	return true
}

func (g *Guard) AllowPublic(ip string) bool {
	return g.allowIP(ip, g.cfg.PublicRate, g.cfg.PublicBurst)
}

func (g *Guard) AllowAPI(ip string) bool {
	return g.allowIP(ip, g.cfg.APIRate, g.cfg.APIBurst)
}

func (g *Guard) AllowTapIP(ip string) bool {
	return g.allowIP(ip, g.cfg.TapIPRate, g.cfg.TapIPBurst)
}

func (g *Guard) AllowTapUser(userID int64) bool {
	if !g.Enabled() || userID <= 0 {
		return true
	}
	now := time.Now().UTC()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cleanupLocked(now)
	b := g.tapUserBuckets[userID]
	if b == nil {
		b = &bucket{Tokens: g.cfg.TapUserBurst, LastSeen: now}
		g.tapUserBuckets[userID] = b
	}
	allow := allowBucket(b, now, g.cfg.TapUserRate, g.cfg.TapUserBurst, 1)
	b.LastSeen = now
	return allow
}

func (g *Guard) RecordAuthFail(ip string) {
	if !g.Enabled() {
		return
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return
	}
	now := time.Now().UTC()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cleanupLocked(now)

	fs := g.authFails[ip]
	if fs == nil {
		fs = &failState{
			Count:      0,
			WindowFrom: now,
			LastSeen:   now,
		}
		g.authFails[ip] = fs
	}
	if now.Sub(fs.WindowFrom) > time.Duration(g.cfg.AuthFailWindowSec)*time.Second {
		fs.Count = 0
		fs.WindowFrom = now
	}
	fs.Count++
	fs.LastSeen = now
	if fs.Count >= g.cfg.AuthFailThreshold {
		g.bannedUntil[ip] = now.Add(time.Duration(g.cfg.BanSec) * time.Second)
		fs.Count = 0
		fs.WindowFrom = now
	}
}

func (g *Guard) allowIP(ip string, rate, burst float64) bool {
	if !g.Enabled() {
		return true
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return true
	}
	now := time.Now().UTC()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cleanupLocked(now)

	if until, ok := g.bannedUntil[ip]; ok {
		if now.Before(until) {
			return false
		}
		delete(g.bannedUntil, ip)
	}

	b := g.ipBuckets[ip]
	if b == nil {
		b = &bucket{Tokens: burst, LastSeen: now}
		g.ipBuckets[ip] = b
	}
	allow := allowBucket(b, now, rate, burst, 1)
	b.LastSeen = now
	return allow
}

func allowBucket(b *bucket, now time.Time, ratePerSec float64, burst float64, need float64) bool {
	if b == nil {
		return false
	}
	if ratePerSec <= 0 {
		return true
	}
	if burst <= 0 {
		burst = ratePerSec
	}
	if b.Tokens > burst {
		b.Tokens = burst
	}
	elapsed := now.Sub(b.LastSeen).Seconds()
	if elapsed > 0 {
		b.Tokens += elapsed * ratePerSec
		if b.Tokens > burst {
			b.Tokens = burst
		}
	}
	if b.Tokens < need {
		return false
	}
	b.Tokens -= need
	return true
}

func (g *Guard) cleanupLocked(now time.Time) {
	if now.Sub(g.lastCleanup) < 30*time.Second {
		return
	}
	g.lastCleanup = now
	ttl := time.Duration(g.cfg.EntryTTLMin) * time.Minute

	for ip, b := range g.ipBuckets {
		if b == nil || now.Sub(b.LastSeen) > ttl {
			delete(g.ipBuckets, ip)
		}
	}
	for uid, b := range g.tapUserBuckets {
		if b == nil || now.Sub(b.LastSeen) > ttl {
			delete(g.tapUserBuckets, uid)
		}
	}
	for ip, fs := range g.authFails {
		if fs == nil || now.Sub(fs.LastSeen) > ttl {
			delete(g.authFails, ip)
		}
	}
	for ip, until := range g.bannedUntil {
		if now.After(until) {
			delete(g.bannedUntil, ip)
		}
	}
}

func envBool(key string, def bool) bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if val == "" {
		return def
	}
	switch val {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func envInt64(key string, def int64) int64 {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func envFloat64(key string, def float64) float64 {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	n, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return def
	}
	return n
}
