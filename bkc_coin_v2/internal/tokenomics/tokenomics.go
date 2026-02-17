package tokenomics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"

	"bkc_coin_v2/internal/db"
)

// TokenomicsManager управляет всей экономикой BKC Coin
type TokenomicsManager struct {
	db *db.DB
}

// NewTokenomicsManager создает новый менеджер токеномики
func NewTokenomicsManager(database *db.DB) *TokenomicsManager {
	return &TokenomicsManager{db: database}
}

// SystemState представляет состояние системы токеномики
type SystemState struct {
	TotalSupply      int64         `json:"total_supply"`
	ReserveSupply    int64         `json:"reserve_supply"`
	FrozenSupply     int64         `json:"frozen_supply"`
	AdminAllocated   int64         `json:"admin_allocated"`
	TotalMined       int64         `json:"total_mined"`
	TotalBurned      int64         `json:"total_burned"`
	CurrentTapReward int64         `json:"current_tap_reward"`
	CurrentHalving   int           `json:"current_halving"`
	HalvingThreshold int64         `json:"halving_threshold"`
	TaxRateBurn      float64       `json:"tax_rate_burn"`
	TaxRateSystem    float64       `json:"tax_rate_system"`
	UnfrozenSchedule map[int]int64 `json:"unfrozen_schedule"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
}

// TapRewardResult результат обработки тапов
type TapRewardResult struct {
	UserID     int64   `json:"user_id"`
	Taps       int64   `json:"taps"`
	Reward     int64   `json:"reward"`
	EnergyUsed float64 `json:"energy_used"`
	TaxBurned  int64   `json:"tax_burned"`
	TaxSystem  int64   `json:"tax_system"`
	NetReward  int64   `json:"net_reward"`
}

// TransactionTax результат расчета налога на транзакцию
type TransactionTax struct {
	Amount    int64 `json:"amount"`
	TaxBurned int64 `json:"tax_burned"`
	TaxSystem int64 `json:"tax_system"`
	NetAmount int64 `json:"net_amount"`
}

// GetSystemState получает текущее состояние системы
func (tm *TokenomicsManager) GetSystemState(ctx context.Context) (*SystemState, error) {
	var state SystemState
	var scheduleJSON json.RawMessage

	err := tm.db.Pool.QueryRow(ctx, `
		SELECT 
			total_supply, reserve_supply, frozen_supply, admin_allocated,
			total_mined, total_burned, current_tap_reward, current_halving,
			halving_threshold, tax_rate_burn, tax_rate_system,
			unfrozen_schedule, created_at, updated_at
		FROM system_state WHERE id = 1
	`).Scan(
		&state.TotalSupply, &state.ReserveSupply, &state.FrozenSupply,
		&state.AdminAllocated, &state.TotalMined, &state.TotalBurned,
		&state.CurrentTapReward, &state.CurrentHalving,
		&state.HalvingThreshold, &state.TaxRateBurn, &state.TaxRateSystem,
		&scheduleJSON, &state.CreatedAt, &state.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get system state: %w", err)
	}

	// Десериализуем график разблокировки
	if len(scheduleJSON) > 0 {
		if err := json.Unmarshal(scheduleJSON, &state.UnfrozenSchedule); err != nil {
			log.Printf("Warning: failed to parse unfrozen schedule: %v", err)
			state.UnfrozenSchedule = make(map[int]int64)
		}
	} else {
		state.UnfrozenSchedule = make(map[int]int64)
	}

	return &state, nil
}

// ProcessTapReward обрабатывает награду за тапы с учетом халвинга
func (tm *TokenomicsManager) ProcessTapReward(ctx context.Context, userID int64, taps int64, energyCost float64) (*TapRewardResult, error) {
	if taps <= 0 {
		return nil, fmt.Errorf("taps must be positive")
	}

	// Получаем текущую награду за тап
	var currentReward int64
	err := tm.db.Pool.QueryRow(ctx,
		"SELECT current_tap_reward FROM system_state WHERE id = 1",
	).Scan(&currentReward)
	if err != nil {
		return nil, fmt.Errorf("failed to get tap reward: %w", err)
	}

	// Рассчитываем общую награду
	grossReward := taps * currentReward

	// Рассчитываем налоги (только для награды за тапы)
	taxBurned := int64(math.Floor(float64(grossReward) * 0.05)) // 5% сжигается
	taxSystem := int64(math.Floor(float64(grossReward) * 0.02)) // 2% системе
	netReward := grossReward - taxBurned - taxSystem

	result := &TapRewardResult{
		UserID:     userID,
		Taps:       taps,
		Reward:     grossReward,
		EnergyUsed: energyCost,
		TaxBurned:  taxBurned,
		TaxSystem:  taxSystem,
		NetReward:  netReward,
	}

	return result, nil
}

// CalculateTransactionTax рассчитывает налог на транзакцию
func (tm *TokenomicsManager) CalculateTransactionTax(ctx context.Context, amount int64) (*TransactionTax, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}

	var taxRateBurn, taxRateSystem float64
	err := tm.db.Pool.QueryRow(ctx,
		"SELECT tax_rate_burn, tax_rate_system FROM system_state WHERE id = 1",
	).Scan(&taxRateBurn, &taxRateSystem)
	if err != nil {
		return nil, fmt.Errorf("failed to get tax rates: %w", err)
	}

	taxBurned := int64(math.Floor(float64(amount) * taxRateBurn / 100))
	taxSystem := int64(math.Floor(float64(amount) * taxRateSystem / 100))
	netAmount := amount - taxBurned - taxSystem

	return &TransactionTax{
		Amount:    amount,
		TaxBurned: taxBurned,
		TaxSystem: taxSystem,
		NetAmount: netAmount,
	}, nil
}

// CheckAndProcessHalving проверяет и обрабатывает халвинг
func (tm *TokenomicsManager) CheckAndProcessHalving(ctx context.Context) (bool, error) {
	var totalMined, halvingThreshold, currentHalving, currentReward int64

	err := tm.db.Pool.QueryRow(ctx, `
		SELECT total_mined, halving_threshold, current_halving, current_tap_reward 
		FROM system_state WHERE id = 1
	`).Scan(&totalMined, &halvingThreshold, &currentHalving, &currentReward)
	if err != nil {
		return false, fmt.Errorf("failed to get halving data: %w", err)
	}

	// Проверяем нужен ли халвинг
	expectedHalving := totalMined / halvingThreshold
	if expectedHalving <= currentHalving {
		return false, nil // Халвинг еще не нужен
	}

	// Выполняем халвинг
	newReward := currentReward / 2
	if newReward < 1 {
		newReward = 1 // Минимальная награда 1 BKC
	}

	tx, err := tm.db.Pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Обновляем состояние системы
	_, err = tx.Exec(ctx, `
		UPDATE system_state 
		SET current_tap_reward = $1, current_halving = $2, updated_at = now()
		WHERE id = 1
	`, newReward, currentHalving+1)
	if err != nil {
		return false, fmt.Errorf("failed to update halving: %w", err)
	}

	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, amount, meta)
		VALUES('halving', $1, $2::jsonb)
	`, newReward, fmt.Sprintf(`{
		"old_reward": %d,
		"new_reward": %d,
		"halving_number": %d,
		"total_mined": %d
	}`, currentReward, newReward, currentHalving+1, totalMined))
	if err != nil {
		return false, fmt.Errorf("failed to record halving in ledger: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("failed to commit halving: %w", err)
	}

	log.Printf("HALVING COMPLETED: reward %d -> %d, halving #%d",
		currentReward, newReward, currentHalving+1)

	return true, nil
}

// ProcessUnfrozenVesting обрабатывает разблокировку замороженных монет
func (tm *TokenomicsManager) ProcessUnfrozenVesting(ctx context.Context) (int64, error) {
	var frozenSupply int64
	var createdAt time.Time
	var scheduleJSON json.RawMessage

	err := tm.db.Pool.QueryRow(ctx, `
		SELECT frozen_supply, created_at, unfrozen_schedule 
		FROM system_state WHERE id = 1
	`).Scan(&frozenSupply, &createdAt, &scheduleJSON)
	if err != nil {
		return 0, fmt.Errorf("failed to get vesting data: %w", err)
	}

	if frozenSupply <= 0 {
		return 0, nil // Нечего разблокировать
	}

	// Десериализуем график
	var schedule map[int]int64
	if len(scheduleJSON) > 0 {
		if err := json.Unmarshal(scheduleJSON, &schedule); err != nil {
			return 0, fmt.Errorf("failed to parse unfrozen schedule: %w", err)
		}
	} else {
		return 0, nil // График не задан
	}

	// Определяем текущий месяц от создания
	now := time.Now()
	monthsPassed := int(now.Sub(createdAt).Hours() / 24 / 30)

	// Ищем разблокировку на текущий месяц
	unfreezeAmount, exists := schedule[monthsPassed]
	if !exists || unfreezeAmount <= 0 {
		return 0, nil // Нет разблокировки в этом месяце
	}

	// Проверяем что есть что разблокировать
	if unfreezeAmount > frozenSupply {
		unfreezeAmount = frozenSupply
	}

	tx, err := tm.db.Pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Обновляем запасы
	_, err = tx.Exec(ctx, `
		UPDATE system_state 
		SET frozen_supply = frozen_supply - $1,
		    reserve_supply = reserve_supply + $1,
		    updated_at = now()
		WHERE id = 1
	`, unfreezeAmount)
	if err != nil {
		return 0, fmt.Errorf("failed to update supplies: %w", err)
	}

	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, amount, meta)
		VALUES('unfreeze', $1, $2::jsonb)
	`, unfreezeAmount, fmt.Sprintf(`{
		"month": %d,
		"amount": %d,
		"frozen_remaining": %d
	}`, monthsPassed, unfreezeAmount, frozenSupply-unfreezeAmount))
	if err != nil {
		return 0, fmt.Errorf("failed to record unfreeze in ledger: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit unfreeze: %w", err)
	}

	log.Printf("UNFROZEN: %d BKC unlocked for month %d", unfreezeAmount, monthsPassed)

	return unfreezeAmount, nil
}

// BurnCoins сжигает указанное количество монет
func (tm *TokenomicsManager) BurnCoins(ctx context.Context, amount int64, reason string, metadata map[string]interface{}) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	tx, err := tm.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Уменьшаем total supply (сжигание)
	_, err = tx.Exec(ctx, `
		UPDATE system_state 
		SET total_supply = total_supply - $1,
		    total_burned = total_burned + $1,
		    updated_at = now()
		WHERE id = 1 AND total_supply >= $1
	`, amount)
	if err != nil {
		return fmt.Errorf("failed to update total supply: %w", err)
	}

	// Записываем в ledger
	metaJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, amount, meta)
		VALUES('burn', $1, $2::jsonb)
	`, amount, fmt.Sprintf(`{
		"reason": "%s",
		"metadata": %s
	}`, reason, string(metaJSON)))
	if err != nil {
		return fmt.Errorf("failed to record burn in ledger: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit burn: %w", err)
	}

	return nil
}

// GetEconomyStats получает статистику экономики
func (tm *TokenomicsManager) GetEconomyStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Базовая статистика
	var totalSupply, reserveSupply, frozenSupply, totalMined, totalBurned, currentReward int64
	var burnPercentage, reservePercentage, frozenPercentage float64

	err := tm.db.Pool.QueryRow(ctx, `
		SELECT 
			total_supply, reserve_supply, frozen_supply, total_mined, total_burned, current_tap_reward
		FROM system_state WHERE id = 1
	`).Scan(&totalSupply, &reserveSupply, &frozenSupply, &totalMined, &totalBurned, &currentReward)
	if err != nil {
		return nil, fmt.Errorf("failed to get basic stats: %w", err)
	}

	if totalSupply > 0 {
		burnPercentage = float64(totalBurned) / float64(totalSupply) * 100
		reservePercentage = float64(reserveSupply) / float64(totalSupply) * 100
		frozenPercentage = float64(frozenSupply) / float64(totalSupply) * 100
	}

	// Активные пользователи
	var activeUsers int64
	err = tm.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM users 
		WHERE created_at > NOW() - INTERVAL '7 days'
	`).Scan(&activeUsers)
	if err != nil {
		activeUsers = 0
	}

	// Всего пользователей
	var totalUsers int64
	err = tm.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&totalUsers)
	if err != nil {
		totalUsers = 0
	}

	// P2P объем за последние 24 часа
	var p2pVolume int64
	err = tm.db.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_bkc), 0) FROM p2p_orders 
		WHERE status = 'completed' AND completed_at > NOW() - INTERVAL '24 hours'
	`).Scan(&p2pVolume)
	if err != nil {
		p2pVolume = 0
	}

	// Объем сжигания за последние 24 часа
	var dailyBurn int64
	err = tm.db.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0) FROM ledger 
		WHERE kind = 'burn' AND ts > NOW() - INTERVAL '24 hours'
	`).Scan(&dailyBurn)
	if err != nil {
		dailyBurn = 0
	}

	stats["total_supply"] = totalSupply
	stats["reserve_supply"] = reserveSupply
	stats["frozen_supply"] = frozenSupply
	stats["total_mined"] = totalMined
	stats["total_burned"] = totalBurned
	stats["current_tap_reward"] = currentReward
	stats["burn_percentage"] = burnPercentage
	stats["reserve_percentage"] = reservePercentage
	stats["frozen_percentage"] = frozenPercentage
	stats["active_users_7d"] = activeUsers
	stats["total_users"] = totalUsers
	stats["p2p_volume_24h"] = p2pVolume
	stats["daily_burn_24h"] = dailyBurn

	return stats, nil
}

// InitializeVestingSchedule инициализирует график разблокировки (30% на 6 месяцев)
func (tm *TokenomicsManager) InitializeVestingSchedule(ctx context.Context, totalSupply int64) error {
	frozenAmount := totalSupply * 30 / 100 // 30% заморозка

	// График: 6 месяцев, разблокировка по 5% в месяц
	schedule := make(map[int]int64)
	for i := 1; i <= 6; i++ {
		schedule[i] = totalSupply * 5 / 100 // 5% в месяц
	}

	scheduleJSON, err := json.Marshal(schedule)
	if err != nil {
		return fmt.Errorf("failed to marshal schedule: %w", err)
	}

	_, err = tm.db.Pool.Exec(ctx, `
		UPDATE system_state 
		SET frozen_supply = $1,
		    unfrozen_schedule = $2::jsonb,
		    updated_at = now()
		WHERE id = 1
	`, frozenAmount, string(scheduleJSON))

	return err
}

// ValidateSupply проверяет целостность эмиссии
func (tm *TokenomicsManager) ValidateSupply(ctx context.Context) error {
	var totalSupply, reserveSupply, frozenSupply, adminAllocated, totalBurned int64

	err := tm.db.Pool.QueryRow(ctx, `
		SELECT total_supply, reserve_supply, frozen_supply, admin_allocated, total_burned
		FROM system_state WHERE id = 1
	`).Scan(&totalSupply, &reserveSupply, &frozenSupply, &adminAllocated, &totalBurned)
	if err != nil {
		return fmt.Errorf("failed to get supply data: %w", err)
	}

	// Проверяем что сумма всех частей равна total supply
	calculatedTotal := reserveSupply + frozenSupply + adminAllocated + totalBurned
	if calculatedTotal != totalSupply {
		return fmt.Errorf("supply validation failed: calculated %d != stored %d",
			calculatedTotal, totalSupply)
	}

	// Проверяем что баланс всех пользователей + reserve = оставшаяся часть
	var userBalanceSum int64
	err = tm.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(balance), 0) FROM users").Scan(&userBalanceSum)
	if err != nil {
		return fmt.Errorf("failed to get user balance sum: %w", err)
	}

	if userBalanceSum != reserveSupply {
		return fmt.Errorf("user balance mismatch: users have %d, reserve has %d",
			userBalanceSum, reserveSupply)
	}

	return nil
}
