package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"bkc_coin_v2/internal/config"
	"bkc_coin_v2/internal/cryptopay"
	"bkc_coin_v2/internal/db"
	"bkc_coin_v2/internal/fasttap"
	"bkc_coin_v2/internal/memtap"
	"bkc_coin_v2/internal/security"
	"bkc_coin_v2/internal/telegram"
	"bkc_coin_v2/internal/tgbot"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type API struct {
	Cfg     config.Config
	DB      *db.DB
	Tg      *tgbot.Bot
	FastTap *fasttap.Engine
	MemTap  *memtap.Engine
	Guard   *security.Guard

	walletsMu       sync.RWMutex
	walletsCached   map[string]string
	walletsCachedAt time.Time
}

type envelope struct {
	OK    bool        `json:"ok"`
	Error string      `json:"error,omitempty"`
	Data  interface{} `json:"data,omitempty"`
}

type stateRequest struct {
	InitData string `json:"init_data"`
}

type tapRequest struct {
	InitData string `json:"init_data"`
	Count    int64  `json:"count"`
	Power    int64  `json:"power"`
}

type transferRequest struct {
	InitData  string `json:"init_data"`
	To        string `json:"to"`
	ToAddress string `json:"to_address"`
	Amount    int64  `json:"amount"`
}

type buyRequest struct {
	InitData string `json:"init_data"`
	Item     string `json:"item"`
}

type depositCreateRequest struct {
	InitData  string `json:"init_data"`
	TxHash    string `json:"tx_hash"`
	AmountUSD int64  `json:"amount_usd"`
	Currency  string `json:"currency"`
}

type depositListRequest struct {
	InitData string `json:"init_data"`
	Status   string `json:"status"`
	Limit    int64  `json:"limit"`
}

type depositProcessRequest struct {
	InitData  string `json:"init_data"`
	DepositID int64  `json:"deposit_id"`
	Approve   bool   `json:"approve"`
}

type cryptoPayInvoiceRequest struct {
	InitData  string `json:"init_data"`
	AmountUSD int64  `json:"amount_usd"`
}

type cryptoPayCheckRequest struct {
	InitData  string `json:"init_data"`
	InvoiceID int64  `json:"invoice_id"`
}

type nftsListRequest struct {
	InitData string `json:"init_data"`
}

type nftBuyRequest struct {
	InitData string `json:"init_data"`
	NFTID    int64  `json:"nft_id"`
}

type adminNFTCreateRequest struct {
	InitData    string `json:"init_data"`
	Title       string `json:"title"`
	ImageURL    string `json:"image_url"`
	PriceCoins  int64  `json:"price_coins"`
	SupplyTotal int64  `json:"supply_total"`
}

type adminReserveSendRequest struct {
	InitData string `json:"init_data"`
	ToUserID int64  `json:"to_user_id"`
	Amount   int64  `json:"amount"`
}

type adminDepositWalletsSetRequest struct {
	InitData string            `json:"init_data"`
	Wallets  map[string]string `json:"wallets"`
}

type adminBroadcastRequest struct {
	InitData string `json:"init_data"`
	Text     string `json:"text"`
}

type bankLoanTakeRequest struct {
	InitData string `json:"init_data"`
	Plan     string `json:"plan"`   // "7d" | "30d"
	Amount   int64  `json:"amount"` // principal
}

type bankLoanMyRequest struct {
	InitData string `json:"init_data"`
	Limit    int64  `json:"limit"`
}

type bankLoanRepayRequest struct {
	InitData string `json:"init_data"`
	LoanID   int64  `json:"loan_id"`
}

type p2pLoanRequestRequest struct {
	InitData   string `json:"init_data"`
	Lender     string `json:"lender"` // user id or BKC address
	Amount     int64  `json:"amount"`
	InterestBP int64  `json:"interest_bp"`
	TermDays   int64  `json:"term_days"`
}

type p2pLoanIDRequest struct {
	InitData string `json:"init_data"`
	LoanID   int64  `json:"loan_id"`
}

type marketListingCreateRequest struct {
	InitData    string `json:"init_data"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	PriceCoins  int64  `json:"price_coins"`
	Contact     string `json:"contact"`
	ImageBase64 string `json:"image_base64"` // optional
	ImageMime   string `json:"image_mime"`   // optional
}

type marketListingListRequest struct {
	InitData string `json:"init_data"`
	Status   string `json:"status"`
	Limit    int64  `json:"limit"`
}

type marketListingBuyRequest struct {
	InitData  string `json:"init_data"`
	ListingID int64  `json:"listing_id"`
}

type marketListingCancelRequest struct {
	InitData  string `json:"init_data"`
	ListingID int64  `json:"listing_id"`
}

type bankAmountRequest struct {
	InitData string `json:"init_data"`
	Amount   int64  `json:"amount"`
}

type adminMarketListingDeleteRequest struct {
	InitData  string `json:"init_data"`
	ListingID int64  `json:"listing_id"`
}

func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(a.corsMiddleware)
	r.Use(a.securityMiddleware)
	r.Use(a.memtapConsistencyMiddleware)
	r.Use(a.apiProfileMiddleware)

	r.Get("/health", a.health)
	r.Get("/blockchain", a.blockchain)
	// WebApp
	r.Post("/state", a.state)
	r.Post("/tap", a.tap)
	r.Post("/transfer", a.transfer)
	r.Post("/buy", a.buy)
	// Manual deposits
	r.Post("/deposit/create", a.depositCreate)
	r.Post("/deposit/list", a.depositList)
	r.Post("/deposit/process", a.depositProcess)
	// CryptoPay (CryptoBot)
	r.Post("/deposit/cryptopay/invoice", a.cryptoPayInvoice)
	r.Post("/deposit/cryptopay/check", a.cryptoPayCheck)
	r.Post("/cryptopay/webhook", a.cryptoPayWebhook)
	r.Post("/cryptopay/webhook/{secret}", a.cryptoPayWebhook)
	// NFTs
	r.Post("/nfts/list", a.nftsList)
	r.Post("/nfts/my", a.nftsMy)
	r.Post("/nfts/buy", a.nftBuy)
	r.Post("/admin/nfts/create", a.adminNFTCreate)
	// Bank
	r.Post("/bank/freeze", a.bankFreeze)
	r.Post("/bank/unfreeze", a.bankUnfreeze)
	r.Post("/bank/loan/take", a.bankLoanTake)
	r.Post("/bank/loan/my", a.bankLoanMy)
	r.Post("/bank/loan/repay", a.bankLoanRepay)
	// P2P loans
	r.Post("/p2p/loan/request", a.p2pLoanRequest)
	r.Post("/p2p/loan/incoming", a.p2pLoanIncoming)
	r.Post("/p2p/loan/my", a.p2pLoanMy)
	r.Post("/p2p/loan/accept", a.p2pLoanAccept)
	r.Post("/p2p/loan/reject", a.p2pLoanReject)
	r.Post("/p2p/loan/repay", a.p2pLoanRepay)
	r.Post("/p2p/loan/recall", a.p2pLoanRecall)
	// Marketplace
	r.Post("/market/listings/create", a.marketListingCreate)
	r.Post("/market/listings/list", a.marketListingList)
	r.Post("/market/listings/my", a.marketListingMy)
	r.Post("/market/listings/buy", a.marketListingBuy)
	r.Post("/market/listings/cancel", a.marketListingCancel)
	r.Get("/assets/listings/{id}", a.marketListingImage)
	// Admin
	r.Post("/admin/reserve/send", a.adminReserveSend)
	r.Post("/admin/deposit_wallets/set", a.adminDepositWalletsSet)
	r.Post("/admin/broadcast", a.adminBroadcast)
	r.Post("/admin/market/listings/delete", a.adminMarketListingDelete)

	return r
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (a *API) securityMiddleware(next http.Handler) http.Handler {
	if a.Guard == nil || !a.Guard.Enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := a.Guard.ClientIP(r)
		if a.Guard.IsBanned(ip) {
			writeJSON(w, http.StatusTooManyRequests, envelope{OK: false, Error: "too many requests"})
			return
		}

		// Body size guard for JSON endpoints (keeps memory stable under spam).
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			if n := a.Guard.MaxBodyBytes(); n > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, n)
			}
		}

		path := strings.ToLower(strings.TrimSpace(r.URL.Path))
		isPublic := strings.HasSuffix(path, "/health") || strings.HasSuffix(path, "/blockchain") || path == "/healthz"
		if isPublic {
			if !a.Guard.AllowPublic(ip) {
				writeJSON(w, http.StatusTooManyRequests, envelope{OK: false, Error: "rate limited"})
				return
			}
		} else {
			if !a.Guard.AllowAPI(ip) {
				writeJSON(w, http.StatusTooManyRequests, envelope{OK: false, Error: "rate limited"})
				return
			}
			if strings.HasSuffix(path, "/tap") && !a.Guard.AllowTapIP(ip) {
				writeJSON(w, http.StatusTooManyRequests, envelope{OK: false, Error: "tap rate limited"})
				return
			}
		}

		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == http.StatusUnauthorized {
			a.Guard.RecordAuthFail(ip)
		}
	})
}

func (a *API) corsMiddleware(next http.Handler) http.Handler {
	allowed := map[string]struct{}{}
	for _, origin := range a.Cfg.CORSOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		allowed[origin] = struct{}{}
	}
	if strings.TrimSpace(a.Cfg.WebappURL) != "" {
		allowed[strings.TrimRight(strings.TrimSpace(a.Cfg.WebappURL), "/")] = struct{}{}
	}
	if strings.TrimSpace(a.Cfg.PublicBaseURL) != "" {
		allowed[strings.TrimRight(strings.TrimSpace(a.Cfg.PublicBaseURL), "/")] = struct{}{}
	}
	_, allowAll := allowed["*"]

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/")
		if origin != "" && (allowAll || hasOrigin(allowed, origin)) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) memtapConsistencyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.MemTap != nil && a.MemTap.Enabled() && r.Method == http.MethodPost {
			path := strings.ToLower(strings.TrimSpace(r.URL.Path))
			if !strings.HasSuffix(path, "/tap") &&
				!strings.HasSuffix(path, "/state") &&
				!strings.Contains(path, "/cryptopay/webhook") {
				ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
				defer cancel()
				if err := a.MemTap.Flush(ctx); err != nil {
					writeJSON(w, http.StatusServiceUnavailable, envelope{OK: false, Error: "tap queue busy, retry"})
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) apiProfileMiddleware(next http.Handler) http.Handler {
	profile := strings.ToLower(strings.TrimSpace(a.Cfg.APIProfile))
	if profile == "" || profile == "full" || profile == "all" {
		return next
	}
	isAllowed := func(path string) bool {
		p := strings.ToLower(strings.TrimSpace(path))
		if p == "" {
			return false
		}
		// Defensive: handle both mounted and full paths.
		p = strings.TrimPrefix(p, "/api/v1")
		if p == "" {
			p = "/"
		}
		// Always-allowed health endpoints.
		if p == "/health" || p == "/blockchain" {
			return true
		}
		switch profile {
		case "tap":
			return p == "/state" || p == "/tap" || p == "/buy"
		case "market":
			return p == "/state" ||
				strings.HasPrefix(p, "/nfts/") ||
				strings.HasPrefix(p, "/market/")
		case "bank":
			return p == "/state" ||
				p == "/transfer" ||
				strings.HasPrefix(p, "/bank/") ||
				strings.HasPrefix(p, "/p2p/") ||
				strings.HasPrefix(p, "/deposit/") ||
				strings.HasPrefix(p, "/cryptopay/")
		case "admin":
			return p == "/state" || strings.HasPrefix(p, "/admin/")
		default:
			return true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAllowed(r.URL.Path) {
			writeJSON(w, http.StatusNotFound, envelope{OK: false, Error: "endpoint disabled on this node profile"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func hasOrigin(allowed map[string]struct{}, origin string) bool {
	if _, ok := allowed[origin]; ok {
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	return dec.Decode(dst)
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	dbOK := true
	var one int
	if err := a.DB.Pool.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil || one != 1 {
		dbOK = false
	}

	data := map[string]any{
		"service":         "bkc_coin_v2",
		"ts":              time.Now().Unix(),
		"db_ok":           dbOK,
		"fasttap_enabled": a.FastTap != nil && a.FastTap.Enabled(),
		"memtap_enabled":  a.MemTap != nil && a.MemTap.Enabled(),
	}
	if a.FastTap != nil && a.FastTap.Enabled() {
		data["fasttap"] = a.FastTap.QueueStats(ctx)
	}
	if a.MemTap != nil && a.MemTap.Enabled() {
		data["memtap"] = a.MemTap.Stats()
	}

	status := http.StatusOK
	ok := true
	if !dbOK {
		status = http.StatusServiceUnavailable
		ok = false
	}
	writeJSON(w, status, envelope{OK: ok, Data: data})
}

func (a *API) blockchain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sys, err := a.DB.GetSystem(ctx)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}

	var users int64
	_ = a.DB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&users)
	var txs int64
	_ = a.DB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM ledger`).Scan(&txs)
	var tapsMinted int64
	_ = a.DB.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(amount),0) FROM ledger WHERE kind='tap'`).Scan(&tapsMinted)

	rate := coinsPerUSD(sys.ReserveSupply, sys.InitialReserve, sys.StartRateCoinsUSD, sys.MinRateCoinsUSD)

	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{
		"total_supply":    sys.TotalSupply,
		"reserve_supply":  sys.ReserveSupply,
		"reserved_supply": sys.ReservedSupply,
		"users":           users,
		"txs":             txs,
		"taps_minted":     tapsMinted,
		"coins_per_usd":   rate,
		"ts":              time.Now().Unix(),
	}})
}

func (a *API) authUserFrom(initData string) (telegram.AuthUser, bool) {
	return telegram.VerifyWebAppInitData(initData, a.Cfg.BotToken)
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		kk := strings.ToUpper(strings.TrimSpace(k))
		vv := strings.TrimSpace(v)
		if kk == "" || vv == "" {
			continue
		}
		out[kk] = vv
	}
	return out
}

func (a *API) getDepositWallets(ctx context.Context) map[string]string {
	const ttl = 30 * time.Second
	now := time.Now()

	a.walletsMu.RLock()
	if a.walletsCached != nil && now.Sub(a.walletsCachedAt) < ttl {
		out := copyStringMap(a.walletsCached)
		a.walletsMu.RUnlock()
		return out
	}
	a.walletsMu.RUnlock()

	wallets, err := a.DB.GetDepositWallets(ctx)
	if err != nil {
		wallets = map[string]string{}
	}
	if len(wallets) == 0 {
		wallets = a.Cfg.DepositWallets
	}
	wallets = copyStringMap(wallets)

	a.walletsMu.Lock()
	a.walletsCached = wallets
	a.walletsCachedAt = now
	a.walletsMu.Unlock()

	return copyStringMap(wallets)
}

func (a *API) buildUserState(ctx context.Context, user telegram.AuthUser) (map[string]any, error) {
	// Ensure user exists.
	u, err := a.DB.EnsureUser(ctx, user.ID, user.Username, user.FirstName, float64(a.Cfg.EnergyMax))
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var (
		eMax      float64
		newEnergy float64
		memSnap   memtap.UserSnapshot
		hasMemTap bool
	)
	if a.MemTap != nil && a.MemTap.Enabled() {
		memSnap, hasMemTap = a.MemTap.SnapshotIfPending(user.ID, now)
	}

	if hasMemTap {
		u.Balance = memSnap.Balance
		u.TapsTotal = memSnap.TapsTotal
		eMax = float64(memSnap.EnergyMax)
		newEnergy = float64(memSnap.Energy)
	} else {
		// Regen energy and persist only when there is no pending in-memory tap state.
		var regen float64
		eMax, regen = a.effectiveEnergyParams(u, now)
		newEnergy = regenEnergy(u.Energy, eMax, regen, u.EnergyUpdatedAt, now)
		if math.Abs(newEnergy-u.Energy) > 0.0001 || now.Sub(u.EnergyUpdatedAt) > 2*time.Second {
			_, _ = a.DB.Pool.Exec(ctx, `UPDATE users SET energy=$1, energy_updated_at=$2 WHERE user_id=$3`, newEnergy, now, user.ID)
			u.Energy = newEnergy
			u.EnergyUpdatedAt = now
		}
	}

	sys, err := a.DB.GetSystem(ctx)
	if err != nil {
		return nil, err
	}

	rate := coinsPerUSD(sys.ReserveSupply, sys.InitialReserve, sys.StartRateCoinsUSD, sys.MinRateCoinsUSD)

	ud, err := a.DB.GetUserDaily(ctx, user.ID, now)
	if err != nil {
		return nil, err
	}
	dailyLimit := a.Cfg.TapDailyLimit
	if dailyLimit < 0 {
		dailyLimit = 0
	}
	dailyMax := dailyLimit + ud.ExtraQuota
	dailyRemaining := dailyMax - ud.Tapped
	if dailyRemaining < 0 {
		dailyRemaining = 0
	}

	if hasMemTap {
		ud.Tapped = memSnap.DailyTapped
		ud.ExtraQuota = memSnap.DailyExtra
		dailyRemaining = memSnap.DailyRemaining
	}
	if a.MemTap != nil && a.MemTap.Enabled() {
		if reserve, initialReserve, startRate, minRate, ok := a.MemTap.ReserveSnapshotIfPending(); ok {
			rate = coinsPerUSD(reserve, initialReserve, startRate, minRate)
			sys.ReserveSupply = reserve
		}
	}

	data := map[string]any{
		"user_id":         user.ID,
		"username":        user.Username,
		"first_name":      user.FirstName,
		"is_admin":        user.ID == a.Cfg.AdminID,
		"address":         fmtAddress(user.ID),
		"balance":         u.Balance,
		"frozen_balance":  u.FrozenBalance,
		"taps_total":      u.TapsTotal,
		"energy":          int64(math.Floor(u.Energy)),
		"energy_max":      int64(math.Floor(eMax)),
		"coins_per_usd":   rate,
		"referrals":       u.ReferralsCount,
		"ref_bonus_total": u.ReferralBonusTotal,
		"tap": map[string]any{
			"daily_limit":       dailyLimit,
			"daily_tapped":      ud.Tapped,
			"daily_extra_quota": ud.ExtraQuota,
			"daily_remaining":   dailyRemaining,
			"pack_size":         a.Cfg.ExtraTapsPackSize,
		},
		"bank": map[string]any{
			"loan_max_amount":        a.Cfg.BankLoanMaxAmount,
			"loan_7d_interest_bp":    a.Cfg.BankLoan7DInterestBP,
			"loan_30d_interest_bp":   a.Cfg.BankLoan30DInterestBP,
			"p2p_recall_min_days":    a.Cfg.P2PRecallMinDays,
			"market_listing_fee_bkc": a.Cfg.MarketListingFeeCoins,
		},
		"prices": map[string]any{
			"ENERGY_1H": a.Cfg.EnergyBoost1HPriceCoins,
			"TAP_PACK":  a.Cfg.ExtraTapsPackPriceCoins,
		},
		"ts": time.Now().Unix(),
	}
	if strings.TrimSpace(a.Cfg.CoinImageURL) != "" {
		data["coin_image_url"] = a.Cfg.CoinImageURL
	}
	if wallets := a.getDepositWallets(ctx); len(wallets) > 0 {
		data["deposit_wallets"] = wallets
	}

	// Hide reserve from regular users.
	if user.ID == a.Cfg.AdminID {
		data["reserve_supply"] = sys.ReserveSupply
		data["reserved_supply"] = sys.ReservedSupply
		data["total_supply"] = sys.TotalSupply
	}

	return data, nil
}

func (a *API) state(w http.ResponseWriter, r *http.Request) {
	var req stateRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	data, err := a.buildUserState(r.Context(), user)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "server error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: data})
}

func (a *API) tap(w http.ResponseWriter, r *http.Request) {
	var req tapRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if a.Guard != nil && a.Guard.Enabled() {
		if !a.Guard.AllowTapUser(user.ID) {
			writeJSON(w, http.StatusTooManyRequests, envelope{OK: false, Error: "tap rate limited"})
			return
		}
	}

	requested := req.Count
	if requested <= 0 {
		requested = req.Power
	}
	if requested <= 0 {
		requested = 1
	}
	if requested > a.Cfg.TapMaxPerRequest {
		requested = a.Cfg.TapMaxPerRequest
	}

	ctx := r.Context()
	now := time.Now().UTC()

	// Fast path (Redis): returns a compact response used by the webapp to reconcile optimistic taps.
	if a.FastTap != nil && a.FastTap.Enabled() {
		if err := a.FastTap.EnsureSystemCached(ctx); err != nil {
			writeJSON(w, 500, envelope{OK: false, Error: "fasttap system error"})
			return
		}
		if err := a.FastTap.EnsureUserCached(ctx, user.ID, user.Username, user.FirstName, now); err != nil {
			writeJSON(w, 500, envelope{OK: false, Error: "fasttap user error"})
			return
		}
		res, err := a.FastTap.Tap(ctx, user.ID, requested, now)
		if err != nil {
			writeJSON(w, 500, envelope{OK: false, Error: "fasttap failed"})
			return
		}
		writeJSON(w, 200, envelope{OK: true, Data: map[string]any{
			"gained":     res.Gained,
			"tap_reason": res.Reason,
			"energy":     res.Energy,
			"energy_max": res.EnergyMax,
			"tap": map[string]any{
				"daily_limit":       a.Cfg.TapDailyLimit,
				"daily_tapped":      res.DailyTapped,
				"daily_extra_quota": res.DailyExtra,
				"daily_remaining":   res.DailyRemaining,
				"pack_size":         a.Cfg.ExtraTapsPackSize,
			},
			"ts": time.Now().Unix(),
		}})
		return
	}

	// Fast path (in-memory + periodic Postgres flush): no external Redis required.
	if a.MemTap != nil && a.MemTap.Enabled() {
		res, err := a.MemTap.Tap(ctx, user.ID, user.Username, user.FirstName, requested, now)
		if err != nil {
			writeJSON(w, 500, envelope{OK: false, Error: "memtap failed"})
			return
		}
		writeJSON(w, 200, envelope{OK: true, Data: map[string]any{
			"gained":     res.Gained,
			"tap_reason": res.Reason,
			"energy":     res.Energy,
			"energy_max": res.EnergyMax,
			"tap": map[string]any{
				"daily_limit":       a.Cfg.TapDailyLimit,
				"daily_tapped":      res.DailyTapped,
				"daily_extra_quota": res.DailyExtra,
				"daily_remaining":   res.DailyRemaining,
				"pack_size":         a.Cfg.ExtraTapsPackSize,
			},
			"ts": time.Now().Unix(),
		}})
		return
	}

	// DB path (fallback): tap is applied immediately in Postgres.
	_, err := a.DB.EnsureUser(ctx, user.ID, user.Username, user.FirstName, float64(a.Cfg.EnergyMax))
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}

	var gained int64
	tapReason := "ok"

	err = a.DB.WithTx(ctx, func(tx pgx.Tx) error {
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

		// Lock system reserve first (avoid deadlocks with other reserve ops).
		var reserve int64
		var reserved int64
		if err := tx.QueryRow(ctx, `SELECT reserve_supply, reserved_supply FROM system_state WHERE id=1 FOR UPDATE`).Scan(&reserve, &reserved); err != nil {
			return err
		}
		availableReserve := reserve - reserved
		if availableReserve < 0 {
			availableReserve = 0
		}

		// Lock user
		var bal int64
		var energy float64
		var energyMax float64
		var updatedAt time.Time
		var boostUntil time.Time
		var regenMult float64
		var maxMult float64
		if err := tx.QueryRow(ctx, `
SELECT balance, energy, energy_max, energy_updated_at,
       COALESCE(energy_boost_until, to_timestamp(0)), energy_boost_regen_multiplier, energy_boost_max_multiplier
FROM users
WHERE user_id=$1
FOR UPDATE
`, user.ID).Scan(&bal, &energy, &energyMax, &updatedAt, &boostUntil, &regenMult, &maxMult); err != nil {
			return err
		}

		eMax := energyMax
		eRegen := a.Cfg.EnergyRegenPerSec
		if now.Before(boostUntil) {
			eMax = eMax * maxMult
			eRegen = eRegen * regenMult
		}
		energy = regenEnergy(energy, eMax, eRegen, updatedAt, now)

		mintable := int64(math.Floor(energy))
		if mintable < 0 {
			mintable = 0
		}

		remainingDaily := int64(1 << 62)
		if a.Cfg.TapDailyLimit > 0 {
			if _, err := tx.Exec(ctx, `INSERT INTO user_daily(user_id, day) VALUES($1,$2) ON CONFLICT DO NOTHING`, user.ID, day); err != nil {
				return err
			}
			var tapped int64
			var extraQuota int64
			if err := tx.QueryRow(ctx, `SELECT tapped, extra_quota FROM user_daily WHERE user_id=$1 AND day=$2 FOR UPDATE`, user.ID, day).Scan(&tapped, &extraQuota); err != nil {
				return err
			}
			remainingDaily = (a.Cfg.TapDailyLimit + extraQuota) - tapped
			if remainingDaily < 0 {
				remainingDaily = 0
			}
		}

		gained = min3(requested, mintable, remainingDaily)
		if gained > availableReserve {
			gained = availableReserve
		}
		if gained < 0 {
			gained = 0
		}

		if gained == 0 {
			if a.Cfg.TapDailyLimit > 0 && remainingDaily == 0 && mintable > 0 {
				tapReason = "daily_limit"
			} else if availableReserve == 0 && mintable > 0 && remainingDaily > 0 {
				tapReason = "reserve_empty"
			} else if mintable <= 0 {
				tapReason = "no_energy"
			} else {
				tapReason = "zero"
			}
		}
		energy = energy - float64(gained)
		if energy < 0 {
			energy = 0
		}

		if gained > 0 {
			// Move coins out of reserve.
			if _, err := tx.Exec(ctx, `UPDATE system_state SET reserve_supply = reserve_supply - $1, updated_at=now() WHERE id=1`, gained); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE users SET balance = balance + $1, taps_total = taps_total + $1 WHERE user_id=$2`, gained, user.ID); err != nil {
				return err
			}
			if a.Cfg.TapDailyLimit > 0 {
				if _, err := tx.Exec(ctx, `UPDATE user_daily SET tapped=tapped+$1, updated_at=now() WHERE user_id=$2 AND day=$3`, gained, user.ID, day); err != nil {
					return err
				}
			}
			_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('tap', NULL, $1, $2, $3::jsonb)`, user.ID, gained, toJSON(map[string]any{"req": requested}))
			if err != nil {
				return err
			}
		}

		_, err := tx.Exec(ctx, `UPDATE users SET energy=$1, energy_updated_at=$2 WHERE user_id=$3`, energy, now, user.ID)
		return err
	})
	if err != nil {
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"gained": 0}})
			return
		}
		writeJSON(w, 500, envelope{OK: false, Error: "tap failed"})
		return
	}

	data, err := a.buildUserState(ctx, user)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "server error"})
		return
	}
	data["gained"] = gained
	data["tap_reason"] = tapReason
	writeJSON(w, 200, envelope{OK: true, Data: data})
}

func (a *API) transfer(w http.ResponseWriter, r *http.Request) {
	var req transferRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}

	amount := req.Amount
	if amount <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "amount must be > 0"})
		return
	}

	toRaw := strings.TrimSpace(req.To)
	if toRaw == "" {
		toRaw = strings.TrimSpace(req.ToAddress)
	}
	toID, err := parseUserID(toRaw)
	if err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "invalid recipient"})
		return
	}

	ctx := r.Context()
	_, err = a.DB.EnsureUser(ctx, user.ID, user.Username, user.FirstName, float64(a.Cfg.EnergyMax))
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}

	// receiver must exist
	_, err = a.DB.GetUser(ctx, toID)
	if err != nil {
		writeJSON(w, 404, envelope{OK: false, Error: "recipient not found"})
		return
	}

	if err := a.DB.Transfer(ctx, user.ID, toID, amount); err != nil {
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 400, envelope{OK: false, Error: "not enough balance"})
			return
		}
		writeJSON(w, 500, envelope{OK: false, Error: "transfer failed"})
		return
	}

	data, err := a.buildUserState(ctx, user)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "server error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: data})
}

func (a *API) depositCreate(w http.ResponseWriter, r *http.Request) {
	var req depositCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}

	amountUSD := req.AmountUSD
	if amountUSD <= 0 || amountUSD > 1_000_000 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad amount_usd"})
		return
	}

	txHash := strings.TrimSpace(req.TxHash)
	if len(txHash) < 6 || len(txHash) > 200 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad tx_hash"})
		return
	}

	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "USDT"
	}

	ctx := r.Context()
	_, err := a.DB.EnsureUser(ctx, user.ID, user.Username, user.FirstName, float64(a.Cfg.EnergyMax))
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}

	sys, err := a.DB.GetSystem(ctx)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}
	rate := coinsPerUSD(sys.ReserveSupply, sys.InitialReserve, sys.StartRateCoinsUSD, sys.MinRateCoinsUSD)
	coins := amountUSD * rate
	if coins <= 0 {
		writeJSON(w, 500, envelope{OK: false, Error: "rate error"})
		return
	}

	depositID, err := a.DB.CreateDeposit(ctx, user.ID, txHash, amountUSD, currency, coins)
	if err != nil {
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 400, envelope{OK: false, Error: "not enough reserve"})
			return
		}
		writeJSON(w, 500, envelope{OK: false, Error: "deposit create failed"})
		return
	}
	if a.FastTap != nil && a.FastTap.Enabled() {
		_ = a.FastTap.AdjustReserved(ctx, coins)
	}

	state, err := a.buildUserState(ctx, user)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "server error"})
		return
	}
	state["deposit"] = map[string]any{
		"deposit_id": depositID,
		"amount_usd": amountUSD,
		"currency":   currency,
		"amount_bkc": coins,
		"status":     "pending",
	}
	writeJSON(w, 200, envelope{OK: true, Data: state})
}

func (a *API) depositList(w http.ResponseWriter, r *http.Request) {
	var req depositListRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if user.ID != a.Cfg.AdminID {
		writeJSON(w, 403, envelope{OK: false, Error: "forbidden"})
		return
	}

	items, err := a.DB.ListDeposits(r.Context(), req.Status, req.Limit)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"items": items}})
}

func (a *API) depositProcess(w http.ResponseWriter, r *http.Request) {
	var req depositProcessRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if user.ID != a.Cfg.AdminID {
		writeJSON(w, 403, envelope{OK: false, Error: "forbidden"})
		return
	}
	if req.DepositID <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad deposit_id"})
		return
	}

	ctx := r.Context()
	var before db.Deposit
	if a.FastTap != nil && a.FastTap.Enabled() {
		// For Redis reserve accounting we need to know how many coins were reserved.
		if dps, err := a.DB.GetDeposit(ctx, req.DepositID); err == nil {
			before = dps
		}
	}

	err := a.DB.ProcessDeposit(ctx, req.DepositID, user.ID, req.Approve)
	if err != nil {
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 400, envelope{OK: false, Error: "not enough reserve"})
			return
		}
		writeJSON(w, 500, envelope{OK: false, Error: "process failed"})
		return
	}
	if a.FastTap != nil && a.FastTap.Enabled() {
		if strings.ToLower(strings.TrimSpace(before.Status)) == "pending" && before.Coins > 0 {
			if req.Approve {
				_ = a.FastTap.AdjustReserve(ctx, -before.Coins)
				_ = a.FastTap.AdjustReserved(ctx, -before.Coins)
			} else {
				_ = a.FastTap.AdjustReserved(ctx, -before.Coins)
			}
		}
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"ok": true}})
}

func (a *API) cryptoPayInvoice(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(a.Cfg.CryptoPayToken) == "" {
		writeJSON(w, 400, envelope{OK: false, Error: "cryptopay disabled"})
		return
	}

	var req cryptoPayInvoiceRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	usd := req.AmountUSD
	if usd <= 0 || usd > 10_000 {
		writeJSON(w, 400, envelope{OK: false, Error: "amount_usd must be 1..10000"})
		return
	}

	ctx := r.Context()
	_, err := a.DB.EnsureUser(ctx, user.ID, user.Username, user.FirstName, float64(a.Cfg.EnergyMax))
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}

	sys, err := a.DB.GetSystem(ctx)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}

	rate := coinsPerUSD(sys.ReserveSupply, sys.InitialReserve, sys.StartRateCoinsUSD, sys.MinRateCoinsUSD)
	coins := usd * rate
	if coins <= 0 {
		writeJSON(w, 500, envelope{OK: false, Error: "rate error"})
		return
	}
	available := sys.ReserveSupply - sys.ReservedSupply
	if available < coins {
		writeJSON(w, 400, envelope{OK: false, Error: "not enough reserve"})
		return
	}

	client := cryptopay.New(a.Cfg.CryptoPayToken)
	payload := fmt.Sprintf("uid:%d;usd:%d;coins:%d;ts:%d", user.ID, usd, coins, time.Now().Unix())

	inv, err := client.CreateInvoice(ctx, cryptopay.CreateInvoiceRequest{
		CurrencyType:   "fiat",
		Fiat:           "USD",
		Amount:         fmt.Sprintf("%d", usd),
		AcceptedAssets: "TON,USDT,BTC,ETH",
		Description:    "BKC COIN top up",
		Payload:        payload,
		ExpiresIn:      3600,
		AllowComments:  false,
		AllowAnonymous: true,
	})
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "cryptopay createInvoice failed"})
		return
	}

	if err := a.DB.CreateCryptoPayInvoice(ctx, inv.InvoiceID, user.ID, usd, coins, inv.Status); err != nil {
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 400, envelope{OK: false, Error: "not enough reserve"})
			return
		}
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}
	if a.FastTap != nil && a.FastTap.Enabled() {
		_ = a.FastTap.AdjustReserved(ctx, coins)
	}

	url := strings.TrimSpace(inv.MiniAppInvoiceURL)
	if url == "" {
		url = strings.TrimSpace(inv.WebAppInvoiceURL)
	}
	if url == "" {
		url = strings.TrimSpace(inv.BotInvoiceURL)
	}

	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{
		"invoice_id": inv.InvoiceID,
		"status":     inv.Status,
		"url":        url,
		"amount_usd": usd,
		"coins":      coins,
		"rate":       rate,
	}})
}

func (a *API) cryptoPayCheck(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(a.Cfg.CryptoPayToken) == "" {
		writeJSON(w, 400, envelope{OK: false, Error: "cryptopay disabled"})
		return
	}

	var req cryptoPayCheckRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if req.InvoiceID <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad invoice_id"})
		return
	}

	ctx := r.Context()
	invRow, err := a.DB.GetCryptoPayInvoice(ctx, req.InvoiceID)
	if err != nil {
		writeJSON(w, 404, envelope{OK: false, Error: "invoice not found"})
		return
	}
	if invRow.UserID != user.ID && user.ID != a.Cfg.AdminID {
		writeJSON(w, 403, envelope{OK: false, Error: "forbidden"})
		return
	}

	client := cryptopay.New(a.Cfg.CryptoPayToken)
	items, err := client.GetInvoices(ctx, fmt.Sprintf("%d", req.InvoiceID))
	if err != nil || len(items) == 0 {
		writeJSON(w, 500, envelope{OK: false, Error: "cryptopay getInvoices failed"})
		return
	}

	status := items[0].Status
	credited, finalStatus, err := a.DB.ProcessCryptoPayStatus(ctx, req.InvoiceID, status, time.Now().UTC())
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "process failed"})
		return
	}
	if a.FastTap != nil && a.FastTap.Enabled() {
		if credited > 0 {
			_ = a.FastTap.AdjustReserve(ctx, -credited)
			_ = a.FastTap.AdjustReserved(ctx, -credited)
		} else {
			// If the invoice expired/cancelled and coins were released, reserved_supply changed.
			s := strings.ToLower(strings.TrimSpace(finalStatus))
			if s == "expired" || s == "canceled" || s == "cancelled" {
				if sys, err := a.DB.GetSystem(ctx); err == nil {
					_ = a.FastTap.Rdb.HSet(ctx, a.FastTap.SysKey, "reserved_supply", sys.ReservedSupply).Err()
				}
			}
		}
	}

	state, err := a.buildUserState(ctx, user)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "server error"})
		return
	}
	state["cryptopay"] = map[string]any{
		"invoice_id": req.InvoiceID,
		"status":     finalStatus,
		"credited":   credited,
	}
	writeJSON(w, 200, envelope{OK: true, Data: state})
}

func (a *API) cryptoPayWebhook(w http.ResponseWriter, r *http.Request) {
	// Optional secret in path
	secret := chi.URLParam(r, "secret")
	if strings.TrimSpace(a.Cfg.CryptoPayWebhookSecret) != "" && secret != a.Cfg.CryptoPayWebhookSecret {
		writeJSON(w, 403, envelope{OK: false, Error: "forbidden"})
		return
	}

	if strings.TrimSpace(a.Cfg.CryptoPayToken) == "" {
		writeJSON(w, 404, envelope{OK: false, Error: "not configured"})
		return
	}

	raw, err := io.ReadAll(r.Body)
	if err != nil || len(raw) == 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad body"})
		return
	}

	sig := r.Header.Get("crypto-pay-api-signature")
	if !cryptopay.VerifyWebhookSignature(a.Cfg.CryptoPayToken, raw, sig) {
		writeJSON(w, 401, envelope{OK: false, Error: "bad signature"})
		return
	}

	var upd cryptopay.WebhookUpdate
	if err := json.Unmarshal(raw, &upd); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}

	inv := upd.Payload
	credited, finalStatus, _ := a.DB.ProcessCryptoPayStatus(r.Context(), inv.InvoiceID, inv.Status, time.Now().UTC())
	if a.FastTap != nil && a.FastTap.Enabled() {
		ctx := r.Context()
		if credited > 0 {
			_ = a.FastTap.AdjustReserve(ctx, -credited)
			_ = a.FastTap.AdjustReserved(ctx, -credited)
		} else {
			s := strings.ToLower(strings.TrimSpace(finalStatus))
			if s == "expired" || s == "canceled" || s == "cancelled" {
				if sys, err := a.DB.GetSystem(ctx); err == nil {
					_ = a.FastTap.Rdb.HSet(ctx, a.FastTap.SysKey, "reserved_supply", sys.ReservedSupply).Err()
				}
			}
		}
	}

	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"ok": true}})
}

func (a *API) nftsList(w http.ResponseWriter, r *http.Request) {
	var req nftsListRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	_, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	items, err := a.DB.ListNFTs(r.Context())
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"items": items}})
}

func (a *API) nftsMy(w http.ResponseWriter, r *http.Request) {
	var req nftsListRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	items, err := a.DB.ListUserNFTs(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"items": items}})
}

func (a *API) nftBuy(w http.ResponseWriter, r *http.Request) {
	var req nftBuyRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if req.NFTID <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad nft_id"})
		return
	}
	ctx := r.Context()
	_, err := a.DB.EnsureUser(ctx, user.ID, user.Username, user.FirstName, float64(a.Cfg.EnergyMax))
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}
	if err := a.DB.BuyNFT(ctx, user.ID, req.NFTID); err != nil {
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 400, envelope{OK: false, Error: "not enough balance or sold out"})
			return
		}
		writeJSON(w, 500, envelope{OK: false, Error: "buy failed"})
		return
	}
	state, err := a.buildUserState(ctx, user)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "server error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: state})
}

func (a *API) adminNFTCreate(w http.ResponseWriter, r *http.Request) {
	var req adminNFTCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if user.ID != a.Cfg.AdminID {
		writeJSON(w, 403, envelope{OK: false, Error: "forbidden"})
		return
	}

	id, err := a.DB.CreateNFT(r.Context(), req.Title, req.ImageURL, req.PriceCoins, req.SupplyTotal)
	if err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad params"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"nft_id": id}})
}

func (a *API) adminReserveSend(w http.ResponseWriter, r *http.Request) {
	var req adminReserveSendRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if user.ID != a.Cfg.AdminID {
		writeJSON(w, 403, envelope{OK: false, Error: "forbidden"})
		return
	}
	if req.ToUserID <= 0 || req.Amount <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad params"})
		return
	}
	if _, err := a.DB.GetUser(r.Context(), req.ToUserID); err != nil {
		writeJSON(w, 404, envelope{OK: false, Error: "recipient not found"})
		return
	}
	err := a.DB.CreditFromReserve(r.Context(), req.ToUserID, req.Amount, "admin_reserve_send", map[string]any{"by": user.ID})
	if err != nil {
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 400, envelope{OK: false, Error: "not enough reserve"})
			return
		}
		writeJSON(w, 500, envelope{OK: false, Error: "send failed"})
		return
	}
	if a.FastTap != nil && a.FastTap.Enabled() {
		_ = a.FastTap.AdjustReserve(r.Context(), -req.Amount)
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"ok": true}})
}

func (a *API) adminDepositWalletsSet(w http.ResponseWriter, r *http.Request) {
	var req adminDepositWalletsSetRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if user.ID != a.Cfg.AdminID {
		writeJSON(w, 403, envelope{OK: false, Error: "forbidden"})
		return
	}
	if len(req.Wallets) > 50 {
		writeJSON(w, 400, envelope{OK: false, Error: "too many wallets"})
		return
	}
	wallets := copyStringMap(req.Wallets)
	for k, v := range wallets {
		if len(k) > 24 || len(v) > 256 {
			writeJSON(w, 400, envelope{OK: false, Error: "bad wallet value"})
			return
		}
	}

	if err := a.DB.SetDepositWallets(r.Context(), wallets); err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}

	a.walletsMu.Lock()
	a.walletsCached = copyStringMap(wallets)
	a.walletsCachedAt = time.Now()
	a.walletsMu.Unlock()

	state, err := a.buildUserState(r.Context(), user)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "server error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: state})
}

func (a *API) adminBroadcast(w http.ResponseWriter, r *http.Request) {
	var req adminBroadcastRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if user.ID != a.Cfg.AdminID {
		writeJSON(w, 403, envelope{OK: false, Error: "forbidden"})
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeJSON(w, 400, envelope{OK: false, Error: "empty text"})
		return
	}
	if len(text) > 3500 {
		writeJSON(w, 400, envelope{OK: false, Error: "text too long"})
		return
	}
	if a.Tg == nil {
		writeJSON(w, 500, envelope{OK: false, Error: "bot not configured"})
		return
	}

	a.Tg.StartBroadcast(context.Background(), user.ID, text)
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"started": true}})
}

func (a *API) adminMarketListingDelete(w http.ResponseWriter, r *http.Request) {
	var req adminMarketListingDeleteRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if user.ID != a.Cfg.AdminID {
		writeJSON(w, 403, envelope{OK: false, Error: "forbidden"})
		return
	}
	if req.ListingID <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad listing_id"})
		return
	}
	res, err := a.DB.Pool.Exec(r.Context(), `
UPDATE market_listings
SET status='cancelled'
WHERE listing_id=$1 AND status='active'
`, req.ListingID)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}
	if res.RowsAffected() == 0 {
		writeJSON(w, 404, envelope{OK: false, Error: "listing not found"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"ok": true}})
}

func (a *API) bankFreeze(w http.ResponseWriter, r *http.Request) {
	var req bankAmountRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	amount := req.Amount
	if amount <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad amount"})
		return
	}
	if err := a.DB.FreezeBalance(r.Context(), user.ID, amount); err != nil {
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 400, envelope{OK: false, Error: "not enough balance"})
			return
		}
		writeJSON(w, 500, envelope{OK: false, Error: "freeze failed"})
		return
	}
	state, err := a.buildUserState(r.Context(), user)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "server error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: state})
}

func (a *API) bankUnfreeze(w http.ResponseWriter, r *http.Request) {
	var req bankAmountRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	amount := req.Amount
	if amount <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad amount"})
		return
	}
	if err := a.DB.UnfreezeBalance(r.Context(), user.ID, amount); err != nil {
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 400, envelope{OK: false, Error: "not enough frozen balance"})
			return
		}
		writeJSON(w, 500, envelope{OK: false, Error: "unfreeze failed"})
		return
	}
	state, err := a.buildUserState(r.Context(), user)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "server error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: state})
}

func (a *API) bankLoanTake(w http.ResponseWriter, r *http.Request) {
	var req bankLoanTakeRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}

	plan := strings.ToLower(strings.TrimSpace(req.Plan))
	var termDays int64
	var interestBP int64
	switch plan {
	case "7d", "7", "week":
		termDays = 7
		interestBP = a.Cfg.BankLoan7DInterestBP
	case "30d", "30", "month":
		termDays = 30
		interestBP = a.Cfg.BankLoan30DInterestBP
	default:
		writeJSON(w, 400, envelope{OK: false, Error: "bad plan"})
		return
	}

	amount := req.Amount
	if amount <= 0 || amount > a.Cfg.BankLoanMaxAmount {
		writeJSON(w, 400, envelope{OK: false, Error: "bad amount"})
		return
	}

	ctx := r.Context()
	if _, err := a.DB.EnsureUser(ctx, user.ID, user.Username, user.FirstName, float64(a.Cfg.EnergyMax)); err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}

	loan, err := a.DB.CreateBankLoan(ctx, user.ID, amount, interestBP, termDays)
	if err != nil {
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 400, envelope{OK: false, Error: "not enough reserve"})
			return
		}
		if errors.Is(err, db.ErrAlreadyExists) {
			writeJSON(w, 400, envelope{OK: false, Error: "active loan exists"})
			return
		}
		writeJSON(w, 500, envelope{OK: false, Error: "loan create failed"})
		return
	}
	if a.FastTap != nil && a.FastTap.Enabled() {
		_ = a.FastTap.AdjustReserve(ctx, -loan.Principal)
	}

	state, err := a.buildUserState(ctx, user)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "server error"})
		return
	}
	state["bank_loan"] = loan
	writeJSON(w, 200, envelope{OK: true, Data: state})
}

func (a *API) bankLoanMy(w http.ResponseWriter, r *http.Request) {
	var req bankLoanMyRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	ctx := r.Context()
	items, err := a.DB.ListBankLoansByUser(ctx, user.ID, req.Limit)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"items": items}})
}

func (a *API) bankLoanRepay(w http.ResponseWriter, r *http.Request) {
	var req bankLoanRepayRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if req.LoanID <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad loan_id"})
		return
	}
	ctx := r.Context()
	if err := a.DB.RepayBankLoan(ctx, user.ID, req.LoanID); err != nil {
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 400, envelope{OK: false, Error: "not enough balance"})
			return
		}
		writeJSON(w, 500, envelope{OK: false, Error: "repay failed"})
		return
	}
	state, err := a.buildUserState(ctx, user)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "server error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: state})
}

func (a *API) p2pLoanRequest(w http.ResponseWriter, r *http.Request) {
	var req p2pLoanRequestRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	lenderID, err := parseUserID(strings.TrimSpace(req.Lender))
	if err != nil || lenderID <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad lender"})
		return
	}
	if lenderID == user.ID {
		writeJSON(w, 400, envelope{OK: false, Error: "self lender"})
		return
	}
	amount := req.Amount
	if amount <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad amount"})
		return
	}
	termDays := req.TermDays
	if termDays <= 0 || termDays > 365 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad term_days"})
		return
	}
	interestBP := req.InterestBP
	if interestBP < 0 || interestBP > 50_000 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad interest_bp"})
		return
	}

	ctx := r.Context()
	if _, err := a.DB.EnsureUser(ctx, user.ID, user.Username, user.FirstName, float64(a.Cfg.EnergyMax)); err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}
	if _, err := a.DB.GetUser(ctx, lenderID); err != nil {
		writeJSON(w, 404, envelope{OK: false, Error: "lender not found"})
		return
	}

	loan, err := a.DB.CreateP2PLoanRequest(ctx, user.ID, lenderID, amount, interestBP, termDays)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "request failed"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"loan": loan}})
}

func (a *API) p2pLoanIncoming(w http.ResponseWriter, r *http.Request) {
	var req bankLoanMyRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	ctx := r.Context()
	items, err := a.DB.ListIncomingP2PRequests(ctx, user.ID, req.Limit)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"items": items}})
}

func (a *API) p2pLoanMy(w http.ResponseWriter, r *http.Request) {
	var req bankLoanMyRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	ctx := r.Context()
	items, err := a.DB.ListP2PLoansByUser(ctx, user.ID, req.Limit)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"items": items}})
}

func (a *API) p2pLoanAccept(w http.ResponseWriter, r *http.Request) {
	var req p2pLoanIDRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if req.LoanID <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad loan_id"})
		return
	}
	if err := a.DB.AcceptP2PLoan(r.Context(), user.ID, req.LoanID); err != nil {
		if errors.Is(err, db.ErrForbidden) {
			writeJSON(w, 403, envelope{OK: false, Error: "forbidden"})
			return
		}
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 400, envelope{OK: false, Error: "not enough balance"})
			return
		}
		writeJSON(w, 500, envelope{OK: false, Error: "accept failed"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"ok": true}})
}

func (a *API) p2pLoanReject(w http.ResponseWriter, r *http.Request) {
	var req p2pLoanIDRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if req.LoanID <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad loan_id"})
		return
	}
	if err := a.DB.RejectP2PLoan(r.Context(), user.ID, req.LoanID); err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "reject failed"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"ok": true}})
}

func (a *API) p2pLoanRepay(w http.ResponseWriter, r *http.Request) {
	var req p2pLoanIDRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if req.LoanID <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad loan_id"})
		return
	}
	if err := a.DB.RepayP2PLoan(r.Context(), user.ID, req.LoanID); err != nil {
		if errors.Is(err, db.ErrForbidden) {
			writeJSON(w, 403, envelope{OK: false, Error: "forbidden"})
			return
		}
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 400, envelope{OK: false, Error: "not enough balance"})
			return
		}
		writeJSON(w, 500, envelope{OK: false, Error: "repay failed"})
		return
	}
	state, err := a.buildUserState(r.Context(), user)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "server error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: state})
}

func (a *API) p2pLoanRecall(w http.ResponseWriter, r *http.Request) {
	var req p2pLoanIDRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if req.LoanID <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad loan_id"})
		return
	}
	if err := a.DB.RecallP2PLoan(r.Context(), user.ID, req.LoanID, a.Cfg.P2PRecallMinDays); err != nil {
		if errors.Is(err, db.ErrForbidden) {
			writeJSON(w, 403, envelope{OK: false, Error: "forbidden"})
			return
		}
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 400, envelope{OK: false, Error: "not enough borrower balance"})
			return
		}
		writeJSON(w, 400, envelope{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"ok": true}})
}

func (a *API) marketListingCreate(w http.ResponseWriter, r *http.Request) {
	var req marketListingCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	ctx := r.Context()
	if _, err := a.DB.EnsureUser(ctx, user.ID, user.Username, user.FirstName, float64(a.Cfg.EnergyMax)); err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}

	// Create listing and burn fee.
	listing, err := a.DB.CreateMarketListing(ctx, user.ID, req.Title, req.Description, req.Category, req.PriceCoins, req.Contact, a.Cfg.MarketListingFeeCoins)
	if err != nil {
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 400, envelope{OK: false, Error: "not enough balance for fee"})
			return
		}
		writeJSON(w, 400, envelope{OK: false, Error: "bad params"})
		return
	}

	// Optional image.
	rawB64 := strings.TrimSpace(req.ImageBase64)
	if rawB64 != "" {
		mime := strings.TrimSpace(req.ImageMime)
		if i := strings.Index(rawB64, "base64,"); i >= 0 {
			rawB64 = rawB64[i+7:]
		}
		buf, err := base64.StdEncoding.DecodeString(rawB64)
		if err != nil {
			writeJSON(w, 400, envelope{OK: false, Error: "bad image_base64"})
			return
		}
		if len(buf) > 800_000 {
			writeJSON(w, 400, envelope{OK: false, Error: "image too large"})
			return
		}
		if mime == "" {
			mime = "image/jpeg"
		}
		id, err := a.DB.AddMarketListingImage(ctx, listing.ListingID, mime, buf)
		if err == nil && id > 0 {
			listing.ImageID = &id
		}
	}

	state, err := a.buildUserState(ctx, user)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "server error"})
		return
	}
	state["listing"] = listing
	writeJSON(w, 200, envelope{OK: true, Data: state})
}

func (a *API) marketListingList(w http.ResponseWriter, r *http.Request) {
	var req marketListingListRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	_ = user
	items, err := a.DB.ListMarketListings(r.Context(), req.Status, req.Limit)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"items": items}})
}

func (a *API) marketListingMy(w http.ResponseWriter, r *http.Request) {
	var req marketListingListRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	items, err := a.DB.ListMyMarketListings(r.Context(), user.ID, req.Limit)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"items": items}})
}

func (a *API) marketListingBuy(w http.ResponseWriter, r *http.Request) {
	var req marketListingBuyRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if req.ListingID <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad listing_id"})
		return
	}
	if err := a.DB.BuyMarketListing(r.Context(), user.ID, req.ListingID); err != nil {
		if errors.Is(err, db.ErrNotEnough) {
			writeJSON(w, 400, envelope{OK: false, Error: "not enough balance"})
			return
		}
		writeJSON(w, 500, envelope{OK: false, Error: "buy failed"})
		return
	}
	state, err := a.buildUserState(r.Context(), user)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "server error"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: state})
}

func (a *API) marketListingCancel(w http.ResponseWriter, r *http.Request) {
	var req marketListingCancelRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}
	if req.ListingID <= 0 {
		writeJSON(w, 400, envelope{OK: false, Error: "bad listing_id"})
		return
	}
	res, err := a.DB.Pool.Exec(r.Context(), `
UPDATE market_listings
SET status='cancelled'
WHERE listing_id=$1 AND seller_id=$2 AND status='active'
`, req.ListingID, user.ID)
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}
	if res.RowsAffected() == 0 {
		writeJSON(w, 404, envelope{OK: false, Error: "listing not found"})
		return
	}
	writeJSON(w, 200, envelope{OK: true, Data: map[string]any{"ok": true}})
}

func (a *API) marketListingImage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	img, err := a.DB.GetMarketListingImage(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", img.Mime)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(img.Data)
}

func (a *API) buy(w http.ResponseWriter, r *http.Request) {
	var req buyRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, envelope{OK: false, Error: "bad json"})
		return
	}
	user, ok := a.authUserFrom(req.InitData)
	if !ok {
		writeJSON(w, 401, envelope{OK: false, Error: "unauthorized"})
		return
	}

	item := strings.ToLower(strings.TrimSpace(req.Item))
	if item == "" {
		writeJSON(w, 400, envelope{OK: false, Error: "missing item"})
		return
	}

	ctx := r.Context()
	_, err := a.DB.EnsureUser(ctx, user.ID, user.Username, user.FirstName, float64(a.Cfg.EnergyMax))
	if err != nil {
		writeJSON(w, 500, envelope{OK: false, Error: "db error"})
		return
	}

	now := time.Now().UTC()

	switch item {
	case "energy_1h":
		price := a.Cfg.EnergyBoost1HPriceCoins
		var (
			boostUntil time.Time
			baseMax    float64
			effMax     float64
		)
		err = a.DB.WithTx(ctx, func(tx pgx.Tx) error {
			// lock user
			var bal int64
			var energyMax float64
			if err := tx.QueryRow(ctx, `SELECT balance, energy_max FROM users WHERE user_id=$1 FOR UPDATE`, user.ID).Scan(&bal, &energyMax); err != nil {
				return err
			}
			if bal < price {
				return db.ErrNotEnough
			}
			// debit to reserve
			if _, err := tx.Exec(ctx, `UPDATE users SET balance = balance - $1 WHERE user_id=$2`, price, user.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE system_state SET reserve_supply = reserve_supply + $1, updated_at=now() WHERE id=1`, price); err != nil {
				return err
			}
			until := now.Add(1 * time.Hour)
			boostUntil = until
			baseMax = energyMax
			regenMult := a.Cfg.EnergyBoost1HRegenMultiplier
			maxMult := a.Cfg.EnergyBoost1HMaxMultiplier
			effMax = energyMax * maxMult
			if _, err := tx.Exec(ctx, `UPDATE users SET energy_boost_until=$1, energy_boost_regen_multiplier=$2, energy_boost_max_multiplier=$3, energy=$4, energy_updated_at=$5 WHERE user_id=$6`, until, regenMult, maxMult, effMax, now, user.ID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('buy_energy_1h', $1, NULL, $2, $3::jsonb)`, user.ID, price, toJSON(map[string]any{"until": until.Unix()}))
			return err
		})
		if err != nil {
			if errors.Is(err, db.ErrNotEnough) {
				writeJSON(w, 400, envelope{OK: false, Error: "not enough balance"})
				return
			}
			writeJSON(w, 500, envelope{OK: false, Error: "buy failed"})
			return
		}
		if a.FastTap != nil && a.FastTap.Enabled() {
			_ = a.FastTap.AdjustReserve(ctx, price)
			_ = a.FastTap.UpdateEnergyBoost(ctx, user.ID, boostUntil, a.Cfg.EnergyBoost1HRegenMultiplier, a.Cfg.EnergyBoost1HMaxMultiplier, effMax, baseMax, now)
		}
		data, err := a.buildUserState(ctx, user)
		if err != nil {
			writeJSON(w, 500, envelope{OK: false, Error: "server error"})
			return
		}
		data["item"] = item
		data["cost"] = price
		writeJSON(w, 200, envelope{OK: true, Data: data})
		return
	case "tap_pack":
		packSize := a.Cfg.ExtraTapsPackSize
		price := a.Cfg.ExtraTapsPackPriceCoins
		if packSize <= 0 || price <= 0 {
			writeJSON(w, 400, envelope{OK: false, Error: "tap pack disabled"})
			return
		}
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		err = a.DB.WithTx(ctx, func(tx pgx.Tx) error {
			// lock user
			var bal int64
			if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE user_id=$1 FOR UPDATE`, user.ID).Scan(&bal); err != nil {
				return err
			}
			if bal < price {
				return db.ErrNotEnough
			}
			if _, err := tx.Exec(ctx, `UPDATE users SET balance = balance - $1 WHERE user_id=$2`, price, user.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE system_state SET reserve_supply = reserve_supply + $1, updated_at=now() WHERE id=1`, price); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO user_daily(user_id, day) VALUES($1,$2) ON CONFLICT DO NOTHING`, user.ID, day); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE user_daily SET extra_quota = extra_quota + $1, updated_at=now() WHERE user_id=$2 AND day=$3`, packSize, user.ID, day); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('buy_tap_pack', $1, NULL, $2, $3::jsonb)`,
				user.ID, price, toJSON(map[string]any{"day": day.Format("2006-01-02"), "pack_size": packSize}),
			)
			return err
		})
		if err != nil {
			if errors.Is(err, db.ErrNotEnough) {
				writeJSON(w, 400, envelope{OK: false, Error: "not enough balance"})
				return
			}
			writeJSON(w, 500, envelope{OK: false, Error: "buy failed"})
			return
		}
		if a.FastTap != nil && a.FastTap.Enabled() {
			_ = a.FastTap.AdjustReserve(ctx, price)
			_ = a.FastTap.AddDailyExtraQuota(ctx, user.ID, day, packSize)
		}
		data, err := a.buildUserState(ctx, user)
		if err != nil {
			writeJSON(w, 500, envelope{OK: false, Error: "server error"})
			return
		}
		data["item"] = item
		data["cost"] = price
		data["pack_size"] = packSize
		writeJSON(w, 200, envelope{OK: true, Data: data})
		return
	default:
		writeJSON(w, 400, envelope{OK: false, Error: "unknown item"})
		return
	}
}

func coinsPerUSD(reserve, initialReserve, startRate, minRate int64) int64 {
	if initialReserve <= 0 {
		return startRate
	}
	if reserve < 0 {
		reserve = 0
	}
	if reserve > initialReserve {
		reserve = initialReserve
	}
	span := startRate - minRate
	return minRate + (span*reserve)/initialReserve
}

func regenEnergy(current float64, eMax float64, regenPerSec float64, updatedAt time.Time, now time.Time) float64 {
	if eMax <= 0 {
		return 0
	}
	if current < 0 {
		current = 0
	}
	dt := now.Sub(updatedAt).Seconds()
	if dt <= 0 {
		if current > eMax {
			return eMax
		}
		return current
	}
	current = current + dt*regenPerSec
	if current > eMax {
		current = eMax
	}
	return current
}

func (a *API) effectiveEnergyParams(u db.UserState, now time.Time) (float64, float64) {
	eMax := u.EnergyMax
	eRegen := a.Cfg.EnergyRegenPerSec
	if now.Before(u.EnergyBoostUntil) {
		eMax = eMax * u.EnergyBoostMaxMultiplier
		eRegen = eRegen * u.EnergyBoostRegenMultiplier
	}
	if eMax < 0 {
		eMax = 0
	}
	if eRegen < 0 {
		eRegen = 0
	}
	return eMax, eRegen
}

func min3(a, b, c int64) int64 {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

func parseUserID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("empty")
	}
	if strings.HasPrefix(raw, "BKC") {
		raw = strings.TrimPrefix(raw, "BKC")
	}
	if !isDigits(raw) {
		return 0, errors.New("bad")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("bad")
	}
	return id, nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func fmtAddress(userID int64) string {
	return "BKC" + strconv.FormatInt(userID, 10)
}

func toJSON(v any) string {
	if v == nil {
		return `{}`
	}
	b, err := json.Marshal(v)
	if err != nil {
		return `{}`
	}
	return string(b)
}
