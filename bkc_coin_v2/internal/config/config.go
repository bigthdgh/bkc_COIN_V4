package config

import (
	"encoding/json"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	BotToken       string
	AdminID        int64
	DatabaseURL    string
	RedisURL       string
	PublicBaseURL  string
	WebappURL      string
	CORSOrigins    []string
	CoinImageURL   string
	DepositWallets map[string]string
	APIProfile     string
	RunAPI         bool
	RunBot         bool
	RunOverdue     bool
	RunFasttap     bool

	TotalSupply          int64
	AdminAllocationPct   int64
	StartRateCoinsPerUSD int64
	MinRateCoinsPerUSD   int64

	BankLoan7DInterestBP  int64
	BankLoan30DInterestBP int64
	BankLoanMaxAmount     int64
	P2PRecallMinDays      int64
	MarketListingFeeCoins int64

	EnergyMax         int64
	EnergyRegenPerSec float64
	TapMaxPerRequest  int64
	TapMaxMultiTouch  int64

	TapDailyLimit           int64
	ExtraTapsPackSize       int64
	ExtraTapsPackPriceCoins int64

	EnergyBoost1HPriceCoins      int64
	EnergyBoost1HRegenMultiplier float64
	EnergyBoost1HMaxMultiplier   float64

	CryptoPayToken         string
	CryptoPayWebhookSecret string
}

func mustEnv(key string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		log.Printf("missing env: %s, using default", key)
		return ""
	}
	return val
}

func normalizeDatabaseURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return s
	}

	// Neon sometimes shows `psql 'postgresql://...'` examples. Accept them too.
	if i := strings.Index(s, "postgresql://"); i >= 0 {
		s = s[i:]
	} else if i := strings.Index(s, "postgres://"); i >= 0 {
		s = s[i:]
	}

	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	if i := strings.IndexAny(s, " \t\r\n"); i >= 0 {
		s = strings.Trim(s[:i], `"'`)
	}

	u, err := url.Parse(s)
	if err != nil {
		return s
	}
	q := u.Query()
	// pgx does not need channel_binding and may treat it as a runtime param.
	q.Del("channel_binding")
	u.RawQuery = q.Encode()
	return u.String()
}

func normalizeRedisURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return s
	}

	// Some consoles show `redis-cli -u redis://...` examples. Accept them too.
	// Also allow rediss:// (TLS).
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

func envBool(key string, def bool) bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if val == "" {
		return def
	}
	switch val {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func Load() Config {
	// PUBLIC_BASE_URL and WEBAPP_URL are required for local development, but on Render we can
	// derive them from platform-provided env vars.
	publicBase := strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL"))
	if publicBase == "" {
		publicBase = strings.TrimSpace(os.Getenv("RENDER_EXTERNAL_URL"))
	}
	if publicBase == "" {
		host := strings.TrimSpace(os.Getenv("RENDER_EXTERNAL_HOSTNAME"))
		if host != "" {
			publicBase = "https://" + host
		}
	}
	if publicBase == "" {
		port := strings.TrimSpace(os.Getenv("PORT"))
		if port == "" {
			port = "8080"
		}
		publicBase = "http://127.0.0.1:" + port
	}

	webappURL := strings.TrimSpace(os.Getenv("WEBAPP_URL"))
	if webappURL == "" {
		webappURL = publicBase
	}

	publicBase = strings.TrimRight(publicBase, "/")
	webappURL = strings.TrimRight(webappURL, "/")

	cfg := Config{
		BotToken:       mustEnv("BOT_TOKEN"),
		DatabaseURL:    normalizeDatabaseURL(mustEnv("DATABASE_URL")),
		RedisURL:       normalizeRedisURL(os.Getenv("REDIS_URL")),
		PublicBaseURL:  publicBase,
		WebappURL:      webappURL,
		CORSOrigins:    parseCSV(strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))),
		CoinImageURL:   strings.TrimSpace(os.Getenv("COIN_IMAGE_URL")),
		DepositWallets: map[string]string{},
		APIProfile:     strings.ToLower(strings.TrimSpace(os.Getenv("API_PROFILE"))),
		RunAPI:         envBool("RUN_API", true),
		RunBot:         envBool("RUN_BOT", true),
		RunOverdue:     envBool("RUN_OVERDUE_WORKER", true),
		RunFasttap:     envBool("RUN_FASTTAP_WORKER", true),

		AdminID:              envInt64("ADMIN_ID", 0),
		TotalSupply:          envInt64("TOTAL_SUPPLY", 500_000_000),
		AdminAllocationPct:   envInt64("ADMIN_ALLOCATION_PCT", 30),
		StartRateCoinsPerUSD: envInt64("START_RATE_COINS_PER_USD", 60_000),
		MinRateCoinsPerUSD:   envInt64("MIN_RATE_COINS_PER_USD", 50_000),

		BankLoan7DInterestBP:  envInt64("BANK_LOAN_7D_INTEREST_BP", 1200),  // 12%
		BankLoan30DInterestBP: envInt64("BANK_LOAN_30D_INTEREST_BP", 3500), // 35%
		BankLoanMaxAmount:     envInt64("BANK_LOAN_MAX_AMOUNT", 2_000_000),
		P2PRecallMinDays:      envInt64("P2P_RECALL_MIN_DAYS", 5),
		MarketListingFeeCoins: envInt64("MARKET_LISTING_FEE_COINS", 2_000),

		EnergyMax:         envInt64("ENERGY_MAX", 300),
		EnergyRegenPerSec: envFloat64("ENERGY_REGEN_PER_SEC", 1.0),
		TapMaxPerRequest:  envInt64("TAP_MAX_PER_REQUEST", 500),
		TapMaxMultiTouch:  envInt64("TAP_MAX_MULTITOUCH", 13),

		TapDailyLimit:           envInt64("TAP_DAILY_LIMIT", 100_000),
		ExtraTapsPackSize:       envInt64("EXTRA_TAPS_PACK_SIZE", 13_000),
		ExtraTapsPackPriceCoins: envInt64("EXTRA_TAPS_PACK_PRICE_COINS", 15_000),

		EnergyBoost1HPriceCoins:      envInt64("ENERGY_BOOST_1H_PRICE_COINS", 25_000),
		EnergyBoost1HRegenMultiplier: envFloat64("ENERGY_BOOST_1H_REGEN_MULT", 5.0),
		EnergyBoost1HMaxMultiplier:   envFloat64("ENERGY_BOOST_1H_MAX_MULT", 5.0),

		CryptoPayToken:         strings.TrimSpace(os.Getenv("CRYPTOPAY_API_TOKEN")),
		CryptoPayWebhookSecret: strings.TrimSpace(os.Getenv("CRYPTOPAY_WEBHOOK_SECRET")),
	}

	if cfg.CoinImageURL == "" {
		cfg.CoinImageURL = cfg.PublicBaseURL + "/assets/coin.svg"
	}

	// Optional: show deposit wallets in the Mini App (manual top-up instructions).
	// Example:
	//   DEPOSIT_WALLETS_JSON={"USDT":"T...","TRX":"T...","SOL":"..."}
	if raw := strings.TrimSpace(os.Getenv("DEPOSIT_WALLETS_JSON")); raw != "" {
		var m map[string]string
		if err := json.Unmarshal([]byte(raw), &m); err == nil {
			for k, v := range m {
				kk := strings.ToUpper(strings.TrimSpace(k))
				vv := strings.TrimSpace(v)
				if kk == "" || vv == "" {
					continue
				}
				cfg.DepositWallets[kk] = vv
			}
		}
	}

	if cfg.AdminID == 0 {
		panic("missing/invalid env: ADMIN_ID")
	}

	if cfg.AdminAllocationPct < 0 || cfg.AdminAllocationPct > 100 {
		panic("ADMIN_ALLOCATION_PCT must be 0..100")
	}

	if cfg.APIProfile == "" {
		cfg.APIProfile = "full"
	}

	if cfg.MinRateCoinsPerUSD <= 0 || cfg.StartRateCoinsPerUSD <= 0 {
		panic("rate must be > 0")
	}
	if cfg.MinRateCoinsPerUSD > cfg.StartRateCoinsPerUSD {
		panic("MIN_RATE_COINS_PER_USD must be <= START_RATE_COINS_PER_USD")
	}

	if cfg.TapDailyLimit < 0 {
		panic("TAP_DAILY_LIMIT must be >= 0")
	}
	if cfg.ExtraTapsPackSize < 0 || cfg.ExtraTapsPackPriceCoins < 0 {
		panic("EXTRA_TAPS_* must be >= 0")
	}

	return cfg
}

func parseCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
