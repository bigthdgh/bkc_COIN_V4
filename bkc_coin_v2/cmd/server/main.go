package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"bkc_coin_v2/internal/api"
	"bkc_coin_v2/internal/config"
	"bkc_coin_v2/internal/db"
	"bkc_coin_v2/internal/fasttap"
	"bkc_coin_v2/internal/memtap"
	"bkc_coin_v2/internal/security"
	"bkc_coin_v2/internal/tgbot"

	"github.com/go-chi/chi/v5"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	database, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(ctx); err != nil {
		log.Fatalf("db migrate: %v", err)
	}
	if err := database.EnsureDepositWalletsIfEmpty(ctx, cfg.DepositWallets); err != nil {
		log.Fatalf("ensure deposit wallets: %v", err)
	}

	// Init system state if missing.
	_, err = database.GetSystem(ctx)
	if err != nil {
		if err == pgx.ErrNoRows {
			initSystem(ctx, cfg, database)
		} else {
			log.Fatalf("get system: %v", err)
		}
	}

	// Start bot (optional by role/env).
	var bot *tgbot.Bot
	if cfg.RunBot {
		bot, err = tgbot.New(cfg, database)
		if err != nil {
			log.Fatalf("bot init: %v", err)
		}
	}

	// Optional: fast tap pipeline (Redis + stream worker -> Postgres).
	var ft *fasttap.Engine
	if strings.TrimSpace(cfg.RedisURL) != "" {
		rdb, err := fasttap.Connect(ctx, cfg.RedisURL)
		if err != nil {
			log.Fatalf("redis connect: %v", err)
		}
		defer func() {
			if rdb != nil {
				_ = rdb.Close()
			}
		}()
		ft = fasttap.New(cfg, database, rdb)
		if ft != nil && ft.Enabled() && cfg.RunFasttap {
			if err := ft.EnsureSystemCached(ctx); err != nil {
				log.Fatalf("fasttap system warmup: %v", err)
			}
			ft.StartWorker(ctx)
			log.Printf("fasttap enabled (stream=%s group=%s)", ft.StreamKey, ft.StreamGroup)
		} else if ft != nil && ft.Enabled() {
			log.Printf("fasttap enabled (worker disabled by RUN_FASTTAP_WORKER=0)")
		}
	}

	// Optional: mem tap pipeline (in-memory tap cache + periodic Postgres flush).
	// Used when MEMTAP_ENABLED=1. If Redis fasttap is enabled, it remains the primary hot path.
	var mt *memtap.Engine
	if ft == nil || !ft.Enabled() {
		mt = memtap.New(cfg, database)
		if mt != nil && mt.Enabled() {
			mt.Start(ctx)
			log.Printf("memtap enabled (flush in-memory to postgres)")
		}
	}
	useWebhook := cfg.RunBot && shouldUseTelegramWebhook(cfg.PublicBaseURL)
	webhookSecret := strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_SECRET"))
	if webhookSecret == "" {
		webhookSecret = randomHex(16)
	}
	webhookPath := "/telegram/webhook/" + webhookSecret

	// Background maintenance (optional by role/env).
	if cfg.RunOverdue {
		go func() {
			ticker := time.NewTicker(60 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					n, err := database.MarkOverdueBankLoans(ctx, time.Now().UTC())
					if err != nil {
						log.Printf("bank_loans overdue: %v", err)
						continue
					}
					if n > 0 {
						log.Printf("bank_loans overdue processed: %d", n)
					}
				}
			}
		}()
	}

	// HTTP server
	guard := security.NewFromEnv()
	apiSrv := &api.API{Cfg: cfg, DB: database, Tg: bot, FastTap: ft, MemTap: mt, Guard: guard}
	root := chi.NewRouter()
	root.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"run_api":     cfg.RunAPI,
			"run_bot":     cfg.RunBot,
			"run_overdue": cfg.RunOverdue,
			"api_profile": cfg.APIProfile,
			"ts":          time.Now().Unix(),
		})
	})
	if cfg.RunAPI {
		root.Mount("/api/v1", apiSrv.Router())
	}

	if useWebhook {
		root.Post(webhookPath, func(w http.ResponseWriter, r *http.Request) {
			var upd tgbotapi.Update
			if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			bot.HandleUpdate(ctx, upd)
			w.WriteHeader(http.StatusOK)
		})
		webhookURL := strings.TrimRight(cfg.PublicBaseURL, "/") + webhookPath
		if err := bot.SetWebhook(webhookURL); err != nil {
			log.Printf("telegram setWebhook error: %v", err)
		} else {
			log.Printf("telegram webhook enabled")
		}
	} else if cfg.RunBot {
		// If a webhook is configured on Telegram side, polling won't work.
		_ = bot.SetWebhook("")
		bot.StartPolling(ctx)
		log.Printf("telegram polling enabled")
	}

	// Static webapp (optional local hosting)
	if cfg.RunAPI {
		fs := http.FileServer(http.Dir("webapp"))
		root.Handle("/*", fs)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	_, _ = strconv.Atoi(port)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           root,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("http listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	// Graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	cancel()

	ctxShut, cancelShut := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShut()
	_ = srv.Shutdown(ctxShut)
}

func initSystem(ctx context.Context, cfg config.Config, database *db.DB) {
	adminAllocated := (cfg.TotalSupply * cfg.AdminAllocationPct) / 100
	reserve := cfg.TotalSupply - adminAllocated

	_, err := database.EnsureSystemState(ctx, cfg.TotalSupply, cfg.AdminID, adminAllocated, reserve, cfg.StartRateCoinsPerUSD, cfg.MinRateCoinsPerUSD, 3, 30_000)
	if err != nil {
		log.Fatalf("ensure system: %v", err)
	}

	// Create admin user and set premine balance directly (not taken from reserve)
	_, err = database.Pool.Exec(ctx, `
INSERT INTO users (user_id, username, first_name, balance, energy, energy_max)
VALUES ($1, 'admin', 'Admin', $2, $3, $3)
ON CONFLICT (user_id) DO UPDATE SET balance = EXCLUDED.balance
`, cfg.AdminID, adminAllocated, float64(cfg.EnergyMax))
	if err != nil {
		log.Fatalf("ensure admin user: %v", err)
	}

	_, _ = database.Pool.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('genesis_admin', NULL, $1, $2, '{}'::jsonb)`, cfg.AdminID, adminAllocated)

	log.Printf("initialized system: total=%d, admin=%d, reserve=%d", cfg.TotalSupply, adminAllocated, reserve)
}

func shouldUseTelegramWebhook(publicBaseURL string) bool {
	if strings.TrimSpace(os.Getenv("TELEGRAM_FORCE_POLLING")) == "1" {
		return false
	}
	if strings.TrimSpace(os.Getenv("TELEGRAM_FORCE_WEBHOOK")) == "1" {
		return true
	}
	u := strings.TrimSpace(publicBaseURL)
	if !strings.HasPrefix(u, "https://") {
		return false
	}
	if strings.Contains(u, "127.0.0.1") || strings.Contains(u, "localhost") {
		return false
	}
	return true
}

func randomHex(n int) string {
	if n <= 0 {
		n = 16
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "dev"
	}
	return hex.EncodeToString(b)
}
