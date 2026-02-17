package mining

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"bkc_coin_v2/internal/db"
)

// MiningManager управляет логикой майнинга и тапов
type MiningManager struct {
	db *db.DB
}

// NewMiningManager создает новый менеджер майнинга
func NewMiningManager(database *db.DB) *MiningManager {
	return &MiningManager{db: database}
}

// UserMiningState состояние майнинга пользователя
type UserMiningState struct {
	UserID              int64     `json:"user_id"`
	Balance             int64     `json:"balance"`
	Energy              float64   `json:"energy"`
	EnergyMax           float64   `json:"energy_max"`
	EnergyUpdatedAt     time.Time `json:"energy_updated_at"`
	Level               int       `json:"level"`
	TapsPower           int       `json:"taps_power"`
	DailyTapsLimit      int       `json:"daily_taps_limit"`
	DailyTapsUsed       int       `json:"daily_taps_used"`
	LastTapDate         time.Time `json:"last_tap_date"`
	IsPremium           bool      `json:"is_premium"`
	PremiumType         string    `json:"premium_type"`
	IsSubscribed        bool      `json:"is_subscribed"`
	CollectorMode       bool      `json:"collector_mode"`
	LoanDebt            int64     `json:"loan_debt"`
	TapsTotal           int64     `json:"taps_total"`
	ReferralsCount      int64     `json:"referrals_count"`
}

// TapRequest запрос на тапы
type TapRequest struct {
	UserID int64 `json:"user_id"`
	Taps   int64 `json:"taps"`
}

// TapResult результат обработки тапов
type TapResult struct {
	UserID         int64   `json:"user_id"`
	Taps           int64   `json:"taps"`
	Reward         int64   `json:"reward"`
	EnergyUsed     float64 `json:"energy_used"`
	EnergyLeft     float64 `json:"energy_left"`
	DailyTapsLeft  int     `json:"daily_taps_left"`
	Level          int     `json:"level"`
	TapsPower      int     `json:"taps_power"`
	CollectorMode  bool    `json:"collector_mode"`
	Success        bool    `json:"success"`
	Message        string  `json:"message"`
}

// PremiumPlan планы подписки
type PremiumPlan struct {
	Type         string  `json:"type"`
	Name         string  `json:"name"`
	Price        int64   `json:"price"`
	DailyLimit   int     `json:"daily_limit"`
	TaxRate      float64 `json:"tax_rate"`
	Features     []string `json:"features"`
}

// LevelCost стоимость уровня
type LevelCost struct {
	Level int   `json:"level"`
	Cost  int64 `json:"cost"`
	Power int   `json:"power"`
}

// GetPremiumPlans получает доступные планы подписок
func GetPremiumPlans() map[string]PremiumPlan {
	return map[string]PremiumPlan{
		"basic": {
			Type:       "basic",
			Name:       "BKC Basic",
			Price:      0,
			DailyLimit:  5000,
			TaxRate:    10.0,
			Features:   []string{"Базовый майнинг", "5к тапов в день"},
		},
		"silver": {
			Type:       "silver",
			Name:       "BKC Silver",
			Price:      50000,
			DailyLimit:  15000,
			TaxRate:    5.0,
			Features:   []string{"15к тапов в день", "Комиссия 5%", "Доступ к редким NFT"},
		},
		"gold": {
			Type:       "gold",
			Name:       "BKC Gold",
			Price:      200000,
			DailyLimit:  50000,
			TaxRate:    2.0,
			Features:   []string{"50к тапов в день", "Комиссия 2%", "Доступ к элитным NFT", "Приоритетная поддержка"},
		},
	}
}

// GetLevelCost рассчитывает стоимость уровня по формуле: 500 * 1.6^(Level-1)
func GetLevelCost(level int) LevelCost {
	if level <= 1 {
		return LevelCost{Level: 1, Cost: 0, Power: 1}
	}
	
	cost := int64(float64(500) * math.Pow(1.6, float64(level-1)))
	power := 1 + (level-1)/2 // Каждые 2 уровня +1 к силе тапа
	
	return LevelCost{
		Level: level,
		Cost:  cost,
		Power: power,
	}
}

// GetUserMiningState получает состояние майнинга пользователя
func (mm *MiningManager) GetUserMiningState(ctx context.Context, userID int64) (*UserMiningState, error) {
	var state UserMiningState
	
	err := mm.db.Pool.QueryRow(ctx, `
		SELECT 
			user_id, balance, energy, energy_max, energy_updated_at,
			level, taps_power, daily_taps_limit, daily_taps_used, 
			COALESCE(last_tap_date, '1970-01-01'), is_premium, 
			COALESCE(premium_type, 'basic'), is_subscribed,
			collector_mode, COALESCE(loan_debt, 0), taps_total, referrals_count
		FROM users WHERE user_id = $1
	`, userID).Scan(
		&state.UserID, &state.Balance, &state.Energy, &state.EnergyMax,
		&state.EnergyUpdatedAt, &state.Level, &state.TapsPower,
		&state.DailyTapsLimit, &state.DailyTapsUsed, &state.LastTapDate,
		&state.IsPremium, &state.PremiumType, &state.IsSubscribed,
		&state.CollectorMode, &state.LoanDebt, &state.TapsTotal, &state.ReferralsCount,
	)
	
	if err != nil {
		return nil, fmt.Errorf("failed to get user mining state: %w", err)
	}
	
	return &state, nil
}

// UpdateEnergy обновляет энергию пользователя с учетом регенерации
func (mm *MiningManager) UpdateEnergy(ctx context.Context, userID int64) (float64, error) {
	var userEnergy, userEnergyMax float64
	var energyUpdatedAt time.Time
	var energyBoostUntil time.Time
	var regenMultiplier float64
	
	err := mm.db.Pool.QueryRow(ctx, `
		SELECT energy, energy_max, energy_updated_at, 
		       COALESCE(energy_boost_until, '1970-01-01'),
		       COALESCE(energy_boost_regen_multiplier, 1.0)
		FROM users WHERE user_id = $1
	`, userID).Scan(&userEnergy, &userEnergyMax, &energyUpdatedAt, &energyBoostUntil, &regenMultiplier)
	
	if err != nil {
		return userEnergy, fmt.Errorf("failed to get energy data: %w", err)
	}
	
	now := time.Now()
	
	// Проверяем активен ли буст энергии
	isBoostActive := now.Before(energyBoostUntil)
	if !isBoostActive {
		regenMultiplier = 1.0
	}
	
	// Рассчитываем прошедшее время
	timeDiff := now.Sub(energyUpdatedAt).Seconds()
	
	// Базовая регенерация: 1 энергия в секунду
	baseRegen := timeDiff * 1.0
	
	// Применяем множитель
	actualRegen := baseRegen * regenMultiplier
	
	// Если пользователь в режиме коллектора, регенерация в 3 раза медленнее
	var collectorMode bool
	err = mm.db.Pool.QueryRow(ctx, "SELECT collector_mode FROM users WHERE user_id = $1", userID).Scan(&collectorMode)
	if err == nil && collectorMode {
		actualRegen /= 3.0
	}
	
	// Обновляем энергию, но не выше максимума
	newEnergy := userEnergy + actualRegen
	if newEnergy > userEnergyMax {
		newEnergy = userEnergyMax
	}
	
	// Сохраняем обновленную энергию
	_, err = mm.db.Pool.Exec(ctx, `
		UPDATE users 
		SET energy = $1, energy_updated_at = $2
		WHERE user_id = $3
	`, newEnergy, now, userID)
	
	return newEnergy, err
}

// ProcessTaps обрабатывает тапы пользователя
func (mm *MiningManager) ProcessTaps(ctx context.Context, req *TapRequest) (*TapResult, error) {
	if req.Taps <= 0 || req.Taps > 100 {
		return &TapResult{
			Success: false,
			Message: "Invalid taps count",
		}, nil
	}
	
	// Обновляем энергию
	currentEnergy, err := mm.UpdateEnergy(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to update energy: %w", err)
	}
	
	// Получаем состояние пользователя
	state, err := mm.GetUserMiningState(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user state: %w", err)
	}
	
	// Проверяем подписку на канал
	if !state.IsSubscribed {
		return &TapResult{
			Success: false,
			Message: "Требуется подписка на канал для начала майнинга",
		}, nil
	}
	
	// Проверяем лимит тапов на день
	today := time.Now().Format("2006-01-02")
	if state.LastTapDate.Format("2006-01-02") != today {
		// Новый день - сбрасываем счетчик
		state.DailyTapsUsed = 0
		_, err = mm.db.Pool.Exec(ctx, `
			UPDATE users 
			SET daily_taps_used = 0, last_tap_date = $1
			WHERE user_id = $2
		`, today, req.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to reset daily taps: %w", err)
		}
	}
	
	dailyTapsLeft := state.DailyTapsLimit - state.DailyTapsUsed
	if req.Taps > int64(dailyTapsLeft) {
		return &TapResult{
			Success:       false,
			Message:       fmt.Sprintf("Достигнут дневной лимит. Осталось: %d", dailyTapsLeft),
			DailyTapsLeft: dailyTapsLeft,
		}, nil
	}
	
	// Проверяем энергию
	energyCost := float64(req.Taps) // 1 энергия за 1 тап
	if currentEnergy < energyCost {
		return &TapResult{
			Success:    false,
			Message:    "Недостаточно энергии",
			EnergyLeft: currentEnergy,
		}, nil
	}
	
	// Рассчитываем награду
	tapsPower := state.TapsPower
	baseReward := req.Taps * int64(tapsPower)
	
	// Если пользователь в режиме коллектора, вся награда идет на погашение долга
	var finalReward int64
	if state.CollectorMode && state.LoanDebt > 0 {
		// 100% тапов идет на погашение долга
		if baseReward > state.LoanDebt {
			finalReward = state.LoanDebt
		} else {
			finalReward = baseReward
		}
		
		// Обновляем долг
		newDebt := state.LoanDebt - finalReward
		_, err = mm.db.Pool.Exec(ctx, `
			UPDATE users 
			SET loan_debt = GREATEST($1, 0), collector_mode = CASE WHEN $1 <= 0 THEN false ELSE collector_mode END
			WHERE user_id = $2
		`, newDebt, req.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to update loan debt: %w", err)
		}
	} else {
		finalReward = baseReward
	}
	
	// Обновляем пользователя
	newEnergy := currentEnergy - energyCost
	newDailyUsed := state.DailyTapsUsed + int(req.Taps)
	
	tx, err := mm.db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	
	// Обновляем баланс и статистику
	_, err = tx.Exec(ctx, `
		UPDATE users 
		SET balance = balance + $1,
		    energy = $2,
		    energy_updated_at = $3,
		    daily_taps_used = $4,
		    taps_total = taps_total + $5
		WHERE user_id = $6
	`, finalReward, newEnergy, time.Now(), newDailyUsed, req.Taps, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}
	
	// Записываем в daily статистику
	_, err = tx.Exec(ctx, `
		INSERT INTO user_daily(user_id, day, tapped)
		VALUES($1, CURRENT_DATE, $2)
		ON CONFLICT (user_id, day) DO UPDATE
		SET tapped = user_daily.tapped + EXCLUDED.tapped,
		    updated_at = now()
	`, req.UserID, req.Taps)
	if err != nil {
		return nil, fmt.Errorf("failed to update daily stats: %w", err)
	}
	
	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, from_id, to_id, amount, meta)
		VALUES('tap', NULL, $1, $2, $3::jsonb)
	`, req.UserID, finalReward, fmt.Sprintf(`{
		"taps": %d,
		"power": %d,
		"energy_used": %.2f,
		"collector_mode": %t
	}`, req.Taps, tapsPower, energyCost, state.CollectorMode))
	if err != nil {
		return nil, fmt.Errorf("failed to record in ledger: %w", err)
	}
	
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit taps: %w", err)
	}
	
	return &TapResult{
		UserID:        req.UserID,
		Taps:          req.Taps,
		Reward:        finalReward,
		EnergyUsed:    energyCost,
		EnergyLeft:    newEnergy,
		DailyTapsLeft: state.DailyTapsLimit - newDailyUsed,
		Level:         state.Level,
		TapsPower:     tapsPower,
		CollectorMode: state.CollectorMode,
		Success:       true,
		Message:       fmt.Sprintf("Заработано +%d BKC", finalReward),
	}, nil
}

// UpgradeLevel повышает уровень пользователя
func (mm *MiningManager) UpgradeLevel(ctx context.Context, userID int64) error {
	state, err := mm.GetUserMiningState(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user state: %w", err)
	}
	
	newLevel := state.Level + 1
	cost := GetLevelCost(newLevel)
	
	// Проверяем достаточно ли монет
	if state.Balance < cost.Cost {
		return fmt.Errorf("insufficient balance: need %d, have %d", cost.Cost, state.Balance)
	}
	
	tx, err := mm.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	
	// Списываем стоимость
	_, err = tx.Exec(ctx, `
		UPDATE users 
		SET balance = balance - $1
		WHERE user_id = $2
	`, cost.Cost, userID)
	if err != nil {
		return fmt.Errorf("failed to deduct cost: %w", err)
	}
	
	// Обновляем уровень и силу тапа
	_, err = tx.Exec(ctx, `
		UPDATE users 
		SET level = $1, taps_power = $2
		WHERE user_id = $3
	`, newLevel, cost.Power, userID)
	if err != nil {
		return fmt.Errorf("failed to update level: %w", err)
	}
	
	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, from_id, to_id, amount, meta)
		VALUES('upgrade', $1, NULL, $2, $3::jsonb)
	`, userID, cost.Cost, fmt.Sprintf(`{
		"new_level": %d,
		"new_power": %d,
		"cost": %d
	}`, newLevel, cost.Power, cost.Cost))
	if err != nil {
		return fmt.Errorf("failed to record upgrade: %w", err)
	}
	
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit upgrade: %w", err)
	}
	
	log.Printf("User %d upgraded to level %d (power: %d) for %d BKC", 
		userID, newLevel, cost.Power, cost.Cost)
	
	return nil
}

// PurchasePremium покупает подписку
func (mm *MiningManager) PurchasePremium(ctx context.Context, userID int64, planType string) error {
	plans := GetPremiumPlans()
	plan, exists := plans[planType]
	if !exists {
		return fmt.Errorf("invalid plan type: %s", planType)
	}
	
	state, err := mm.GetUserMiningState(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user state: %w", err)
	}
	
	// Проверяем достаточно ли монет
	if state.Balance < plan.Price {
		return fmt.Errorf("insufficient balance: need %d, have %d", plan.Price, state.Balance)
	}
	
	// Рассчитываем дату окончания (1 месяц)
	expiresAt := time.Now().AddDate(0, 1, 0)
	
	tx, err := mm.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	
	// Списываем стоимость
	if plan.Price > 0 {
		_, err = tx.Exec(ctx, `
			UPDATE users 
			SET balance = balance - $1
			WHERE user_id = $2
		`, plan.Price, userID)
		if err != nil {
			return fmt.Errorf("failed to deduct cost: %w", err)
		}
	}
	
	// Обновляем подписку
	_, err = tx.Exec(ctx, `
		UPDATE users 
		SET is_premium = true, premium_type = $1, premium_until = $2,
		    daily_taps_limit = $3
		WHERE user_id = $4
	`, planType, expiresAt, plan.DailyLimit, userID)
	if err != nil {
		return fmt.Errorf("failed to update premium: %w", err)
	}
	
	// Записываем в subscriptions
	_, err = tx.Exec(ctx, `
		INSERT INTO subscriptions(user_id, plan_type, price_paid, expires_at, is_active)
		VALUES($1, $2, $3, $4, true)
		ON CONFLICT (user_id, is_active) DO UPDATE
		SET plan_type = EXCLUDED.plan_type,
		    price_paid = EXCLUDED.price_paid,
		    expires_at = EXCLUDED.expires_at
	`, userID, planType, plan.Price, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to record subscription: %w", err)
	}
	
	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, from_id, to_id, amount, meta)
		VALUES('premium', $1, NULL, $2, $3::jsonb)
	`, userID, plan.Price, fmt.Sprintf(`{
		"plan_type": "%s",
		"expires_at": "%s",
		"daily_limit": %d
	}`, planType, expiresAt.Format(time.RFC3339), plan.DailyLimit))
	if err != nil {
		return fmt.Errorf("failed to record premium: %w", err)
	}
	
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit premium: %w", err)
	}
	
	log.Printf("User %d purchased %s premium for %d BKC", userID, planType, plan.Price)
	
	return nil
}

// CheckSubscriptionStatus проверяет статус подписки
func (mm *MiningManager) CheckSubscriptionStatus(ctx context.Context, userID int64) error {
	var premiumUntil time.Time
	var isPremium bool
	
	err := mm.db.Pool.QueryRow(ctx, `
		SELECT COALESCE(premium_until, '1970-01-01'), is_premium
		FROM users WHERE user_id = $1
	`, userID).Scan(&premiumUntil, &isPremium)
	
	if err != nil {
		return fmt.Errorf("failed to get subscription status: %w", err)
	}
	
	// Если подписка истекла, деактивируем
	if isPremium && time.Now().After(premiumUntil) {
		plans := GetPremiumPlans()
		basicPlan := plans["basic"]
		
		_, err = mm.db.Pool.Exec(ctx, `
			UPDATE users 
			SET is_premium = false, premium_type = 'basic',
			    daily_taps_limit = $1
			WHERE user_id = $2
		`, basicPlan.DailyLimit, userID)
		
		if err != nil {
			return fmt.Errorf("failed to deactivate expired premium: %w", err)
		}
		
		log.Printf("User %d premium subscription expired", userID)
	}
	
	return nil
}

// GetMiningStats получает статистику майнинга
func (mm *MiningManager) GetMiningStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Общая статистика
	var totalUsers, activeToday, activeWeekly int64
	var totalTaps, totalBalance int64
	
	mm.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&totalUsers)
	mm.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM users 
		WHERE last_tap_date >= CURRENT_DATE
	`).Scan(&activeToday)
	mm.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM users 
		WHERE last_tap_date >= CURRENT_DATE - INTERVAL '7 days'
	`).Scan(&activeWeekly)
	mm.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(taps_total), 0) FROM users").Scan(&totalTaps)
	mm.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(balance), 0) FROM users").Scan(&totalBalance)
	
	// Статистика по подпискам
	var basicCount, silverCount, goldCount int64
	mm.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM users WHERE premium_type = 'basic' AND is_premium = true
	`).Scan(&basicCount)
	mm.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM users WHERE premium_type = 'silver' AND is_premium = true
	`).Scan(&silverCount)
	mm.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM users WHERE premium_type = 'gold' AND is_premium = true
	`).Scan(&goldCount)
	
	// Статистика по уровням
	var avgLevel float64
	mm.db.Pool.QueryRow(ctx, "SELECT AVG(level) FROM users").Scan(&avgLevel)
	
	stats["total_users"] = totalUsers
	stats["active_today"] = activeToday
	stats["active_weekly"] = activeWeekly
	stats["total_taps"] = totalTaps
	stats["total_balance"] = totalBalance
	stats["avg_level"] = avgLevel
	stats["premium_basic"] = basicCount
	stats["premium_silver"] = silverCount
	stats["premium_gold"] = goldCount
	stats["total_premium"] = basicCount + silverCount + goldCount
	
	return stats, nil
}
