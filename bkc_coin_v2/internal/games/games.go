package games

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	mathrand "math/rand"
	"time"

	"bkc_coin_v2/internal/db"
)

// GamesManager управляет играми и биржей
type GamesManager struct {
	db *db.DB
}

// NewGamesManager создает новый менеджер игр
func NewGamesManager(database *db.DB) *GamesManager {
	return &GamesManager{db: database}
}

// CrashGame игра "Ракетка"
type CrashGame struct {
	ID           int64      `json:"id"`
	GameID       string     `json:"game_id"`
	Hash         string     `json:"hash"`
	Salt         string     `json:"salt"`
	CrashPoint   float64    `json:"crash_point"`
	Status       string     `json:"status"` // waiting, active, crashed
	StartedAt    time.Time  `json:"started_at"`
	CrashedAt    *time.Time `json:"crashed_at"`
	TotalBets    int64      `json:"total_bets"`
	TotalWinners int        `json:"total_winners"`
	SystemProfit int64      `json:"system_profit"`
}

// CrashBet ставка в игре Ракетка
type CrashBet struct {
	BetID       string    `json:"bet_id"`
	GameID      string    `json:"game_id"`
	UserID      int64     `json:"user_id"`
	Amount      int64     `json:"amount"`
	AutoCashout float64   `json:"auto_cashout"`
	CashedOutAt float64   `json:"cashed_out_at"`
	WinAmount   int64     `json:"win_amount"`
	Status      string    `json:"status"` // active, cashed_out, lost
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ExchangePrice цена на бирже
type ExchangePrice struct {
	ID        int64     `json:"id"`
	Pair      string    `json:"pair"`       // BKC/TON, BKC/USDT
	Price     float64   `json:"price"`      // цена за 1 BKC
	Volume24h float64   `json:"volume_24h"` // объем за 24ч
	Change24h float64   `json:"change_24h"` // изменение за 24ч
	High24h   float64   `json:"high_24h"`   // максимум за 24ч
	Low24h    float64   `json:"low_24h"`    // минимум за 24ч
	LastTrade time.Time `json:"last_trade"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Trade сделка на бирже
type Trade struct {
	ID        int64     `json:"id"`
	Pair      string    `json:"pair"`
	OrderType string    `json:"order_type"` // buy, sell
	Amount    float64   `json:"amount"`     // количество BKC
	Price     float64   `json:"price"`      // цена
	Total     float64   `json:"total"`      // total = amount * price
	UserID    int64     `json:"user_id"`
	Status    string    `json:"status"` // pending, completed, cancelled
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProvablyFair данные для честной игры
type ProvablyFair struct {
	GameID     string `json:"game_id"`
	ServerSeed string `json:"server_seed"`
	ClientSeed string `json:"client_seed"`
	Nonce      int    `json:"nonce"`
	Hash       string `json:"hash"`
	Salt       string `json:"salt"`
}

// GenerateCrashHash генерирует хеш для игры Ракетка
func GenerateCrashHash() (string, string, float64, error) {
	// Генерируем случайную соль
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", "", 0, err
	}
	salt := hex.EncodeToString(saltBytes)

	// Генерируем точку краха (1.00 - 10.00)
	// 3% раундов взрываются на 1.00x
	crashPoint := 1.00
	r := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	if r.Float64() > 0.03 { // 97% шанс на точку выше 1.00
		// Генерируем точку от 1.01 до 10.00
		crashPoint = 1.01 + (r.Float64() * 8.99)      // 1.01 - 10.00
		crashPoint = math.Round(crashPoint*100) / 100 // Округляем до 2 знаков
	}

	// Создаем хеш
	combined := fmt.Sprintf("%s-%.2f-%d", salt, crashPoint, time.Now().Unix())
	hash := sha256.Sum256([]byte(combined))
	hashStr := hex.EncodeToString(hash[:])

	return hashStr, salt, crashPoint, nil
}

// StartCrashGame начинает новую игру Ракетка
func (gm *GamesManager) StartCrashGame(ctx context.Context) (*CrashGame, error) {
	hash, salt, crashPoint, err := GenerateCrashHash()
	if err != nil {
		return nil, fmt.Errorf("failed to generate crash hash: %w", err)
	}

	gameID := fmt.Sprintf("crash_%d", time.Now().Unix())

	game := &CrashGame{
		GameID:     gameID,
		Hash:       hash,
		Salt:       salt,
		CrashPoint: crashPoint,
		Status:     "waiting",
		StartedAt:  time.Now(),
	}

	// Сохраняем игру в базу
	err = gm.db.Pool.QueryRow(ctx, `
		INSERT INTO crash_games(
			game_id, hash, salt, crash_point, status, started_at
		) VALUES($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, game.GameID, game.Hash, game.Salt, game.CrashPoint, game.Status, game.StartedAt).Scan(&game.ID)

	if err != nil {
		return nil, fmt.Errorf("failed to create crash game: %w", err)
	}

	log.Printf("Crash game %s started with crash point %.2f", gameID, crashPoint)

	return game, nil
}

// PlaceCrashBet делает ставку в игре Ракетка
func (gm *GamesManager) PlaceCrashBet(ctx context.Context, userID int64, gameID string, amount int64, autoCashout float64) (*CrashBet, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("bet amount must be positive")
	}

	if autoCashout < 1.01 || autoCashout > 10.00 {
		return nil, fmt.Errorf("auto cashout must be between 1.01 and 10.00")
	}

	tx, err := gm.db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Проверяем баланс пользователя
	var userBalance int64
	err = tx.QueryRow(ctx, "SELECT balance FROM users WHERE user_id = $1 FOR UPDATE", userID).Scan(&userBalance)
	if err != nil {
		return nil, fmt.Errorf("failed to get user balance: %w", err)
	}

	if userBalance < amount {
		return nil, fmt.Errorf("insufficient balance: need %d, have %d", amount, userBalance)
	}

	// Проверяем статус игры
	var gameStatus string
	err = tx.QueryRow(ctx, "SELECT status FROM crash_games WHERE game_id = $1 FOR UPDATE", gameID).Scan(&gameStatus)
	if err != nil {
		return nil, fmt.Errorf("game not found: %w", err)
	}

	if gameStatus != "waiting" && gameStatus != "active" {
		return nil, fmt.Errorf("game is not accepting bets")
	}

	// Списываем монеты
	_, err = tx.Exec(ctx, "UPDATE users SET balance = balance - $1 WHERE user_id = $2", amount, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to deduct balance: %w", err)
	}

	// Создаем ставку
	betID := fmt.Sprintf("bet_%d_%d", userID, time.Now().Unix())
	now := time.Now()

	var newBetID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO crash_bets(
			bet_id, game_id, user_id, amount, auto_cashout, status, created_at
		) VALUES($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, betID, gameID, userID, amount, autoCashout, "active", now).Scan(&newBetID)

	if err != nil {
		return nil, fmt.Errorf("failed to create bet: %w", err)
	}

	// Обновляем общую сумму ставок в игре
	_, err = tx.Exec(ctx, "UPDATE crash_games SET total_bets = total_bets + $1 WHERE game_id = $2", amount, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to update game totals: %w", err)
	}

	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, from_id, to_id, amount, meta)
		VALUES('crash_bet', $1, NULL, $2, $3::jsonb)
	`, userID, amount, fmt.Sprintf(`{
		"game_id": "%s",
		"bet_id": "%s",
		"auto_cashout": %.2f
	}`, gameID, betID, autoCashout))
	if err != nil {
		return nil, fmt.Errorf("failed to record bet: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit bet: %w", err)
	}

	bet := &CrashBet{
		BetID:       betID,
		GameID:      gameID,
		UserID:      userID,
		Amount:      amount,
		AutoCashout: autoCashout,
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	log.Printf("User %d placed bet %d BKC in crash game %s (auto cashout: %.2f)",
		userID, amount, gameID, autoCashout)

	return bet, nil
}

// CashoutBet выводит ставку из игры
func (gm *GamesManager) CashoutBet(ctx context.Context, betID string, currentMultiplier float64) (*CrashBet, error) {
	if currentMultiplier < 1.01 {
		return nil, fmt.Errorf("multiplier too low for cashout")
	}

	tx, err := gm.db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Получаем информацию о ставке
	var bet CrashBet
	err = tx.QueryRow(ctx, `
		SELECT bet_id, game_id, user_id, amount, auto_cashout, status, created_at
		FROM crash_bets 
		WHERE bet_id = $1 AND status = 'active' FOR UPDATE
	`, betID).Scan(&bet.BetID, &bet.GameID, &bet.UserID, &bet.Amount,
		&bet.AutoCashout, &bet.Status, &bet.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("bet not found or not active: %w", err)
	}

	// Рассчитываем выигрыш
	winAmount := int64(math.Floor(float64(bet.Amount) * currentMultiplier))

	// Обновляем ставку
	now := time.Now()
	_, err = tx.Exec(ctx, `
		UPDATE crash_bets 
		SET cashed_out_at = $1, win_amount = $2, status = 'cashed_out', updated_at = $3
		WHERE bet_id = $4
	`, currentMultiplier, winAmount, now, betID)
	if err != nil {
		return nil, fmt.Errorf("failed to update bet: %w", err)
	}

	// Выдаем выигрыш пользователю
	_, err = tx.Exec(ctx, "UPDATE users SET balance = balance + $1 WHERE user_id = $2", winAmount, bet.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to credit winnings: %w", err)
	}

	// Обновляем статистику игры
	_, err = tx.Exec(ctx, "UPDATE crash_games SET total_winners = total_winners + 1 WHERE game_id = $1", bet.GameID)
	if err != nil {
		return nil, fmt.Errorf("failed to update game stats: %w", err)
	}

	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, from_id, to_id, amount, meta)
		VALUES('crash_win', NULL, $1, $2, $3::jsonb)
	`, bet.UserID, winAmount, fmt.Sprintf(`{
		"game_id": "%s",
		"bet_id": "%s",
		"multiplier": %.2f,
		"bet_amount": %d
	}`, bet.GameID, betID, currentMultiplier, bet.Amount))
	if err != nil {
		return nil, fmt.Errorf("failed to record win: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit cashout: %w", err)
	}

	bet.CashedOutAt = currentMultiplier
	bet.WinAmount = winAmount
	bet.Status = "cashed_out"
	bet.UpdatedAt = now

	log.Printf("User %d cashed out bet %s at %.2fx: won %d BKC",
		bet.UserID, betID, currentMultiplier, winAmount)

	return &bet, nil
}

// CrashGame завершает игру Ракетка
func (gm *GamesManager) CrashGame(ctx context.Context, gameID string) error {
	tx, err := gm.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Получаем информацию об игре
	var game CrashGame
	err = tx.QueryRow(ctx, `
		SELECT game_id, crash_point, status, total_bets
		FROM crash_games 
		WHERE game_id = $1 AND status = 'active' FOR UPDATE
	`, gameID).Scan(&game.GameID, &game.CrashPoint, &game.Status, &game.TotalBets)

	if err != nil {
		return fmt.Errorf("game not found or not active: %w", err)
	}

	now := time.Now()

	// Обновляем статус игры
	_, err = tx.Exec(ctx, `
		UPDATE crash_games 
		SET status = 'crashed', crashed_at = $1
		WHERE game_id = $2
	`, now, gameID)
	if err != nil {
		return fmt.Errorf("failed to update game status: %w", err)
	}

	// Обрабатываем все активные ставки (проигравшие)
	rows, err := tx.Query(ctx, `
		SELECT bet_id, user_id, amount
		FROM crash_bets 
		WHERE game_id = $1 AND status = 'active'
	`, gameID)
	if err != nil {
		return fmt.Errorf("failed to get active bets: %w", err)
	}
	defer rows.Close()

	var totalLost int64
	for rows.Next() {
		var betID string
		var userID int64
		var amount int64

		if err := rows.Scan(&betID, &userID, &amount); err != nil {
			continue
		}

		// Обновляем ставку как проигравшую
		_, err = tx.Exec(ctx, `
			UPDATE crash_bets 
			SET status = 'lost', updated_at = $1
			WHERE bet_id = $2
		`, now, betID)
		if err != nil {
			continue
		}

		totalLost += amount
	}

	// Рассчитываем прибыль системы (5% от проигравших ставок)
	systemProfit := int64(math.Floor(float64(totalLost) * 0.05))

	// Обновляем прибыль системы
	_, err = tx.Exec(ctx, "UPDATE crash_games SET system_profit = $1 WHERE game_id = $2", systemProfit, gameID)
	if err != nil {
		return fmt.Errorf("failed to update system profit: %w", err)
	}

	// Записываем прибыль системы в ledger
	if systemProfit > 0 {
		_, err = tx.Exec(ctx, `
			INSERT INTO ledger(kind, amount, meta)
			VALUES('crash_profit', $1, $2::jsonb)
		`, systemProfit, fmt.Sprintf(`{
			"game_id": "%s",
			"crash_point": %.2f,
			"total_bets": %d,
			"total_lost": %d
		}`, gameID, game.CrashPoint, game.TotalBets, totalLost))
		if err != nil {
			return fmt.Errorf("failed to record system profit: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit crash: %w", err)
	}

	log.Printf("Crash game %s crashed at %.2fx: system profit %d BKC",
		gameID, game.CrashPoint, systemProfit)

	return nil
}

// GetActiveCrashGame получает активную игру Ракетка
func (gm *GamesManager) GetActiveCrashGame(ctx context.Context) (*CrashGame, error) {
	var game CrashGame

	err := gm.db.Pool.QueryRow(ctx, `
		SELECT game_id, hash, salt, crash_point, status, started_at, crashed_at, total_bets, total_winners, system_profit
		FROM crash_games 
		WHERE status IN ('waiting', 'active')
		ORDER BY started_at DESC
		LIMIT 1
	`).Scan(&game.GameID, &game.Hash, &game.Salt, &game.CrashPoint,
		&game.Status, &game.StartedAt, &game.CrashedAt, &game.TotalBets, &game.TotalWinners, &game.SystemProfit)

	if err != nil {
		return nil, fmt.Errorf("no active crash game: %w", err)
	}

	return &game, nil
}

// GetCrashBets получает ставки игры
func (gm *GamesManager) GetCrashBets(ctx context.Context, gameID string) ([]CrashBet, error) {
	rows, err := gm.db.Pool.Query(ctx, `
		SELECT bet_id, game_id, user_id, amount, auto_cashout, cashed_out_at, win_amount, status, created_at, updated_at
		FROM crash_bets 
		WHERE game_id = $1
		ORDER BY created_at DESC
	`, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get crash bets: %w", err)
	}
	defer rows.Close()

	var bets []CrashBet
	for rows.Next() {
		var bet CrashBet
		err := rows.Scan(
			&bet.BetID, &bet.GameID, &bet.UserID, &bet.Amount,
			&bet.AutoCashout, &bet.CashedOutAt, &bet.WinAmount,
			&bet.Status, &bet.CreatedAt, &bet.UpdatedAt,
		)
		if err != nil {
			continue
		}
		bets = append(bets, bet)
	}

	return bets, rows.Err()
}

// UpdateExchangePrice обновляет цену на бирже
func (gm *GamesManager) UpdateExchangePrice(ctx context.Context, pair string, price float64, volume float64) error {
	tx, err := gm.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Получаем предыдущую цену
	var prevPrice float64
	var prevHigh24h, prevLow24h float64
	var volume24h float64

	err = tx.QueryRow(ctx, `
		SELECT price, high_24h, low_24h, volume_24h
		FROM exchange_prices 
		WHERE pair = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, pair).Scan(&prevPrice, &prevHigh24h, &prevLow24h, &volume24h)

	if err != nil {
		// Первая цена
		prevHigh24h = price
		prevLow24h = price
		volume24h = volume
	} else {
		// Обновляем 24ч статистику
		if price > prevHigh24h {
			prevHigh24h = price
		}
		if price < prevLow24h {
			prevLow24h = price
		}
		volume24h += volume
	}

	// Рассчитываем изменение за 24ч
	change24h := 0.0
	if prevPrice > 0 {
		change24h = ((price - prevPrice) / prevPrice) * 100
	}

	// Вставляем новую цену
	now := time.Now()
	_, err = tx.Exec(ctx, `
		INSERT INTO exchange_prices(
			pair, price, volume_24h, change_24h, high_24h, low_24h, last_trade, created_at, updated_at
		) VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, pair, price, volume24h, change24h, prevHigh24h, prevLow24h, now, now, now)
	if err != nil {
		return fmt.Errorf("failed to insert price: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit price update: %w", err)
	}

	log.Printf("Exchange price updated: %s = %.6f (24h change: %.2f%%)", pair, price, change24h)

	return nil
}

// GetExchangePrices получает цены на бирже
func (gm *GamesManager) GetExchangePrices(ctx context.Context) ([]ExchangePrice, error) {
	rows, err := gm.db.Pool.Query(ctx, `
		SELECT DISTINCT ON (pair) 
		       id, pair, price, volume_24h, change_24h, high_24h, low_24h, last_trade, created_at, updated_at
		FROM exchange_prices 
		ORDER BY pair, created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get exchange prices: %w", err)
	}
	defer rows.Close()

	var prices []ExchangePrice
	for rows.Next() {
		var price ExchangePrice
		err := rows.Scan(
			&price.ID, &price.Pair, &price.Price, &price.Volume24h,
			&price.Change24h, &price.High24h, &price.Low24h,
			&price.LastTrade, &price.CreatedAt, &price.UpdatedAt,
		)
		if err != nil {
			continue
		}
		prices = append(prices, price)
	}

	return prices, rows.Err()
}

// GetPriceHistory получает историю цен
func (gm *GamesManager) GetPriceHistory(ctx context.Context, pair string, hours int) ([]map[string]interface{}, error) {
	rows, err := gm.db.Pool.Query(ctx, `
		SELECT created_at, price, volume_24h
		FROM exchange_prices 
		WHERE pair = $1 AND created_at > NOW() - INTERVAL '%d hours'
		ORDER BY created_at ASC
	`, pair, hours)
	if err != nil {
		return nil, fmt.Errorf("failed to get price history: %w", err)
	}
	defer rows.Close()

	var history []map[string]interface{}
	for rows.Next() {
		var createdAt time.Time
		var price, volume float64

		if err := rows.Scan(&createdAt, &price, &volume); err != nil {
			continue
		}

		history = append(history, map[string]interface{}{
			"time":   createdAt.Unix() * 1000, // для графиков
			"price":  price,
			"volume": volume,
		})
	}

	return history, rows.Err()
}

// GetGamesStats получает статистику игр
func (gm *GamesManager) GetGamesStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Статистика Ракетки
	var totalCrashGames, activeCrashGames, totalCrashBets, totalCrashVolume, totalCrashProfit int64

	gm.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM crash_games").Scan(&totalCrashGames)
	gm.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM crash_games WHERE status IN ('waiting', 'active')").Scan(&activeCrashGames)
	gm.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM crash_bets").Scan(&totalCrashBets)
	gm.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(amount), 0) FROM crash_bets").Scan(&totalCrashVolume)
	gm.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(system_profit), 0) FROM crash_games").Scan(&totalCrashProfit)

	// Статистика биржи
	var exchangePairs int
	var totalExchangeVolume float64

	gm.db.Pool.QueryRow(ctx, "SELECT COUNT(DISTINCT pair) FROM exchange_prices").Scan(&exchangePairs)
	gm.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(volume_24h), 0) FROM exchange_prices").Scan(&totalExchangeVolume)

	stats["total_crash_games"] = totalCrashGames
	stats["active_crash_games"] = activeCrashGames
	stats["total_crash_bets"] = totalCrashBets
	stats["total_crash_volume"] = totalCrashVolume
	stats["total_crash_profit"] = totalCrashProfit
	stats["exchange_pairs"] = exchangePairs
	stats["total_exchange_volume"] = totalExchangeVolume

	return stats, nil
}

// GetProvablyFairData получает данные для проверки честности игры
func (gm *GamesManager) GetProvablyFairData(ctx context.Context, gameID string) (*ProvablyFair, error) {
	var pf ProvablyFair

	err := gm.db.Pool.QueryRow(ctx, `
		SELECT game_id, hash, salt
		FROM crash_games 
		WHERE game_id = $1
	`, gameID).Scan(&pf.GameID, &pf.Hash, &pf.Salt)

	if err != nil {
		return nil, fmt.Errorf("game not found: %w", err)
	}

	pf.ServerSeed = pf.Salt
	pf.ClientSeed = gameID
	pf.Nonce = 1
	pf.Hash = pf.Hash

	return &pf, nil
}

// VerifyProvablyFair проверяет честность игры
func VerifyProvablyFair(serverSeed, clientSeed string, nonce int, expectedHash string) bool {
	combined := fmt.Sprintf("%s-%s-%d", serverSeed, clientSeed, nonce)
	hash := sha256.Sum256([]byte(combined))
	hashStr := hex.EncodeToString(hash[:])

	return hashStr == expectedHash
}
