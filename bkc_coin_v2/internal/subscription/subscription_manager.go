package subscription

import (
	"context"
	"fmt"
	"sync"
	"time"

	"bkc_coin_v2/internal/i18n"
)

// SubscriptionType тип подписки
type SubscriptionType string

const (
	SubscriptionBasic  SubscriptionType = "basic"
	SubscriptionSilver SubscriptionType = "silver"
	SubscriptionGold   SubscriptionType = "gold"
)

// SubscriptionStatus статус подписки
type SubscriptionStatus string

const (
	StatusActive    SubscriptionStatus = "active"
	StatusExpired   SubscriptionStatus = "expired"
	StatusCancelled SubscriptionStatus = "cancelled"
	StatusPending   SubscriptionStatus = "pending"
)

// SubscriptionManager управляет подписками
type SubscriptionManager struct {
	// Подписки пользователей
	subscriptions map[int64]*Subscription
	mu            sync.RWMutex

	// План подписок
	plans map[SubscriptionType]*SubscriptionPlan

	// Конфигурация
	config SubscriptionConfig

	// Метрики
	metrics *SubscriptionMetrics

	// Кэш
	cache   map[string]interface{}
	cacheMu sync.RWMutex
}

// Subscription подписка пользователя
type Subscription struct {
	ID            string             `json:"id"`
	UserID        int64              `json:"user_id"`
	Type          SubscriptionType   `json:"type"`
	Status        SubscriptionStatus `json:"status"`
	StartedAt     time.Time          `json:"started_at"`
	EndsAt        time.Time          `json:"ends_at"`
	AutoRenew     bool               `json:"auto_renew"`
	LastPaymentAt time.Time          `json:"last_payment_at"`
	NextPaymentAt time.Time          `json:"next_payment_at"`
	TrialUsed     bool               `json:"trial_used"`
	CancelledAt   *time.Time         `json:"cancelled_at,omitempty"`
	CancelReason  string             `json:"cancel_reason,omitempty"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

// SubscriptionPlan план подписки
type SubscriptionPlan struct {
	Type          SubscriptionType `json:"type"`
	Name          string           `json:"name"`
	NameRu        string           `json:"name_ru"`
	Description   string           `json:"description"`
	DescriptionRu string           `json:"description_ru"`
	Price         int64            `json:"price"`
	Currency      string           `json:"currency"`
	Duration      time.Duration    `json:"duration"`
	TaxRate       float64          `json:"tax_rate"`

	// Лимиты и привилегии
	DailyTapLimit     int64   `json:"daily_tap_limit"`
	EnergyMax         int     `json:"energy_max"`
	EnergyRegenRate   float64 `json:"energy_regen_rate"`
	ReferralBonus     float64 `json:"referral_bonus"`
	MarketplaceAccess bool    `json:"marketplace_access"`
	EarlyAccess       bool    `json:"early_access"`
	RealTimeChart     bool    `json:"real_time_chart"`
	NoTransferFee     bool    `json:"no_transfer_fee"`
	PrioritySupport   bool    `json:"priority_support"`
	CustomProfile     bool    `json:"custom_profile"`
	MaxListings       int     `json:"max_listings"`
	EscrowDiscount    float64 `json:"escrow_discount"`

	// NFT привилегии
	NFTTrading   bool    `json:"nft_trading"`
	ExclusiveNFT bool    `json:"exclusive_nft"`
	NFTBonus     float64 `json:"nft_bonus"`

	// Игровые привилегии
	GameBonus       float64 `json:"game_bonus"`
	ExclusiveGames  bool    `json:"exclusive_games"`
	EarlyGameAccess bool    `json:"early_game_access"`
}

// SubscriptionConfig конфигурация подписок
type SubscriptionConfig struct {
	TrialDuration     time.Duration `json:"trial_duration"`
	GracePeriod       time.Duration `json:"grace_period"`
	RenewalReminder   time.Duration `json:"renewal_reminder"`
	MaxFailedPayments int           `json:"max_failed_payments"`
	TaxBasic          float64       `json:"tax_basic"`
	TaxSilver         float64       `json:"tax_silver"`
	TaxGold           float64       `json:"tax_gold"`
}

// SubscriptionMetrics метрики подписок
type SubscriptionMetrics struct {
	TotalSubscriptions  int64     `json:"total_subscriptions"`
	ActiveSubscriptions int64     `json:"active_subscriptions"`
	BasicSubscriptions  int64     `json:"basic_subscriptions"`
	SilverSubscriptions int64     `json:"silver_subscriptions"`
	GoldSubscriptions   int64     `json:"gold_subscriptions"`
	TotalRevenue        int64     `json:"total_revenue"`
	MonthlyRevenue      int64     `json:"monthly_revenue"`
	ChurnRate           float64   `json:"churn_rate"`
	LastUpdated         time.Time `json:"last_updated"`
	mu                  sync.RWMutex
}

// CreateSubscriptionRequest запрос на создание подписки
type CreateSubscriptionRequest struct {
	UserID        int64            `json:"user_id"`
	Type          SubscriptionType `json:"type"`
	UseTrial      bool             `json:"use_trial"`
	AutoRenew     bool             `json:"auto_renew"`
	PaymentMethod string           `json:"payment_method"`
}

// UpgradeSubscriptionRequest запрос на апгрейд подписки
type UpgradeSubscriptionRequest struct {
	UserID    int64            `json:"user_id"`
	NewType   SubscriptionType `json:"new_type"`
	Immediate bool             `json:"immediate"`
}

// DefaultSubscriptionConfig конфигурация по умолчанию
func DefaultSubscriptionConfig() SubscriptionConfig {
	return SubscriptionConfig{
		TrialDuration:     7 * 24 * time.Hour, // 7 дней
		GracePeriod:       3 * 24 * time.Hour, // 3 дня
		RenewalReminder:   7 * 24 * time.Hour, // 7 дней до окончания
		MaxFailedPayments: 3,
		TaxBasic:          0.10, // 10%
		TaxSilver:         0.05, // 5%
		TaxGold:           0.02, // 2%
	}
}

// DefaultSubscriptionPlans планы подписок по умолчанию
func DefaultSubscriptionPlans() map[SubscriptionType]*SubscriptionPlan {
	return map[SubscriptionType]*SubscriptionPlan{
		SubscriptionBasic: {
			Type:          SubscriptionBasic,
			Name:          "Basic",
			NameRu:        "Базовый",
			Description:   "Free plan with basic features",
			DescriptionRu: "Бесплатный план с базовыми функциями",
			Price:         0,
			Currency:      "BKC",
			Duration:      30 * 24 * time.Hour,
			TaxRate:       0.10,

			DailyTapLimit:     5000,
			EnergyMax:         300,
			EnergyRegenRate:   1.0,
			ReferralBonus:     1.0,
			MarketplaceAccess: false,
			EarlyAccess:       false,
			RealTimeChart:     false,
			NoTransferFee:     false,
			PrioritySupport:   false,
			CustomProfile:     false,
			MaxListings:       3,
			EscrowDiscount:    0.0,

			NFTTrading:   false,
			ExclusiveNFT: false,
			NFTBonus:     0.0,

			GameBonus:       1.0,
			ExclusiveGames:  false,
			EarlyGameAccess: false,
		},

		SubscriptionSilver: {
			Type:          SubscriptionSilver,
			Name:          "Silver",
			NameRu:        "Серебряный",
			Description:   "Enhanced features with lower taxes",
			DescriptionRu: "Расширенные функции с пониженными налогами",
			Price:         50000,
			Currency:      "BKC",
			Duration:      30 * 24 * time.Hour,
			TaxRate:       0.05,

			DailyTapLimit:     15000,
			EnergyMax:         500,
			EnergyRegenRate:   1.5,
			ReferralBonus:     1.5,
			MarketplaceAccess: true,
			EarlyAccess:       true,
			RealTimeChart:     false,
			NoTransferFee:     false,
			PrioritySupport:   true,
			CustomProfile:     true,
			MaxListings:       20,
			EscrowDiscount:    0.1,

			NFTTrading:   true,
			ExclusiveNFT: false,
			NFTBonus:     1.1,

			GameBonus:       1.1,
			ExclusiveGames:  false,
			EarlyGameAccess: true,
		},

		SubscriptionGold: {
			Type:          SubscriptionGold,
			Name:          "Gold",
			NameRu:        "Золотой",
			Description:   "Premium features with maximum benefits",
			DescriptionRu: "Премиум функции с максимальными преимуществами",
			Price:         200000,
			Currency:      "BKC",
			Duration:      30 * 24 * time.Hour,
			TaxRate:       0.02,

			DailyTapLimit:     50000,
			EnergyMax:         1000,
			EnergyRegenRate:   2.0,
			ReferralBonus:     2.0,
			MarketplaceAccess: true,
			EarlyAccess:       true,
			RealTimeChart:     true,
			NoTransferFee:     true,
			PrioritySupport:   true,
			CustomProfile:     true,
			MaxListings:       100,
			EscrowDiscount:    0.2,

			NFTTrading:   true,
			ExclusiveNFT: true,
			NFTBonus:     1.25,

			GameBonus:       1.25,
			ExclusiveGames:  true,
			EarlyGameAccess: true,
		},
	}
}

// NewSubscriptionManager создает новый менеджер подписок
func NewSubscriptionManager(config SubscriptionConfig) *SubscriptionManager {
	return &SubscriptionManager{
		subscriptions: make(map[int64]*Subscription),
		plans:         DefaultSubscriptionPlans(),
		config:        config,
		metrics:       &SubscriptionMetrics{},
		cache:         make(map[string]interface{}),
	}
}

// CreateSubscription создает новую подписку
func (sm *SubscriptionManager) CreateSubscription(ctx context.Context, req *CreateSubscriptionRequest, lang i18n.Language) (*Subscription, error) {
	// Проверка существующей подписки
	sm.mu.RLock()
	existing, hasExisting := sm.subscriptions[req.UserID]
	sm.mu.RUnlock()

	if hasExisting && existing.Status == StatusActive {
		return nil, fmt.Errorf(i18n.T(lang, "error_subscription_already_active"))
	}

	// Получение плана подписки
	plan, exists := sm.plans[req.Type]
	if !exists {
		return nil, fmt.Errorf(i18n.T(lang, "error_subscription_plan_not_found"))
	}

	// Проверка триала
	if req.UseTrial && existing != nil && existing.TrialUsed {
		return nil, fmt.Errorf(i18n.T(lang, "error_trial_already_used"))
	}

	// Создание подписки
	now := time.Now()
	subscription := &Subscription{
		ID:        sm.generateID("sub"),
		UserID:    req.UserID,
		Type:      req.Type,
		Status:    StatusActive,
		StartedAt: now,
		EndsAt:    now.Add(plan.Duration),
		AutoRenew: req.AutoRenew,
		TrialUsed: req.UseTrial,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Если используется триал
	if req.UseTrial {
		subscription.EndsAt = now.Add(sm.config.TrialDuration)
	}

	// Расчет следующего платежа
	if plan.Price > 0 && !req.UseTrial {
		subscription.NextPaymentAt = subscription.EndsAt
	}

	// Сохранение подписки
	sm.mu.Lock()
	sm.subscriptions[req.UserID] = subscription
	sm.mu.Unlock()

	// Обновление метрик
	sm.incrementTotalSubscriptions()
	sm.incrementActiveSubscriptions()
	sm.incrementTypeSubscription(req.Type)

	if plan.Price > 0 && !req.UseTrial {
		sm.incrementTotalRevenue(plan.Price)
	}

	// Очистка кэша
	sm.clearCache("subscriptions")

	return subscription, nil
}

// UpgradeSubscription апгрейд подписки
func (sm *SubscriptionManager) UpgradeSubscription(ctx context.Context, req *UpgradeSubscriptionRequest, lang i18n.Language) (*Subscription, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	subscription, exists := sm.subscriptions[req.UserID]
	if !exists {
		return nil, fmt.Errorf(i18n.T(lang, "error_subscription_not_found"))
	}

	if subscription.Status != StatusActive {
		return nil, fmt.Errorf(i18n.T(lang, "error_subscription_not_active"))
	}

	// Проверка, что это действительно апгрейд
	if !sm.isUpgrade(subscription.Type, req.NewType) {
		return nil, fmt.Errorf(i18n.T(lang, "error_not_an_upgrade"))
	}

	// Получение планов
	oldPlan := sm.plans[subscription.Type]
	newPlan := sm.plans[req.NewType]

	if newPlan == nil {
		return nil, fmt.Errorf(i18n.T(lang, "error_subscription_plan_not_found"))
	}

	// Расчет стоимости апгрейда
	var upgradeCost int64
	if req.Immediate {
		// Немедленный апгрейд с пропорциональной доплатой
		remainingDays := time.Until(subscription.EndsAt).Hours() / 24
		dailyPriceDiff := float64(newPlan.Price-oldPlan.Price) / 30.0
		upgradeCost = int64(dailyPriceDiff * remainingDays)
	}

	// Обновление подписки
	subscription.Type = req.NewType
	subscription.UpdatedAt = time.Now()

	if req.Immediate {
		subscription.NextPaymentAt = time.Now().Add(newPlan.Duration)
	}

	// Обновление метрик
	sm.decrementTypeSubscription(subscription.Type)
	sm.incrementTypeSubscription(req.NewType)

	if upgradeCost > 0 {
		sm.incrementTotalRevenue(upgradeCost)
	}

	sm.clearCache("subscriptions")

	return subscription, nil
}

// CancelSubscription отмена подписки
func (sm *SubscriptionManager) CancelSubscription(ctx context.Context, userID int64, reason string, lang i18n.Language) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	subscription, exists := sm.subscriptions[userID]
	if !exists {
		return fmt.Errorf(i18n.T(lang, "error_subscription_not_found"))
	}

	if subscription.Status != StatusActive {
		return fmt.Errorf(i18n.T(lang, "error_subscription_not_active"))
	}

	now := time.Now()
	subscription.Status = StatusCancelled
	subscription.AutoRenew = false
	subscription.CancelledAt = &now
	subscription.CancelReason = reason
	subscription.UpdatedAt = now

	// Обновление метрик
	sm.decrementActiveSubscriptions()
	sm.decrementTypeSubscription(subscription.Type)

	sm.clearCache("subscriptions")

	return nil
}

// GetUserSubscription получает подписку пользователя
func (sm *SubscriptionManager) GetUserSubscription(ctx context.Context, userID int64) (*Subscription, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	subscription, exists := sm.subscriptions[userID]
	if !exists {
		// Создание базовой подписки по умолчанию
		return sm.createDefaultSubscription(userID), nil
	}

	// Проверка истечения подписки
	if subscription.Status == StatusActive && time.Now().After(subscription.EndsAt) {
		subscription.Status = StatusExpired
		subscription.UpdatedAt = time.Now()

		// Обновление метрик
		sm.decrementActiveSubscriptions()
		sm.decrementTypeSubscription(subscription.Type)
	}

	return subscription, nil
}

// GetUserPrivileges получает привилегии пользователя
func (sm *SubscriptionManager) GetUserPrivileges(ctx context.Context, userID int64) (*SubscriptionPlan, error) {
	subscription, err := sm.GetUserSubscription(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Если подписка не активна, возвращаем базовый план
	if subscription.Status != StatusActive {
		return sm.plans[SubscriptionBasic], nil
	}

	plan, exists := sm.plans[subscription.Type]
	if !exists {
		return sm.plans[SubscriptionBasic], nil
	}

	return plan, nil
}

// CheckPrivilege проверяет привилегию пользователя
func (sm *SubscriptionManager) CheckPrivilege(ctx context.Context, userID int64, privilege string) bool {
	plan, err := sm.GetUserPrivileges(ctx, userID)
	if err != nil {
		return false
	}

	switch privilege {
	case "marketplace_access":
		return plan.MarketplaceAccess
	case "real_time_chart":
		return plan.RealTimeChart
	case "no_transfer_fee":
		return plan.NoTransferFee
	case "nft_trading":
		return plan.NFTTrading
	case "exclusive_games":
		return plan.ExclusiveGames
	case "priority_support":
		return plan.PrioritySupport
	default:
		return false
	}
}

// GetUserTaxRate получает налоговую ставку пользователя
func (sm *SubscriptionManager) GetUserTaxRate(ctx context.Context, userID int64) float64 {
	plan, err := sm.GetUserPrivileges(ctx, userID)
	if err != nil {
		return sm.config.TaxBasic
	}

	return plan.TaxRate
}

// GetUserLimits получает лимиты пользователя
func (sm *SubscriptionManager) GetUserLimits(ctx context.Context, userID int64) map[string]interface{} {
	plan, err := sm.GetUserPrivileges(ctx, userID)
	if err != nil {
		plan = sm.plans[SubscriptionBasic]
	}

	return map[string]interface{}{
		"daily_tap_limit":   plan.DailyTapLimit,
		"energy_max":        plan.EnergyMax,
		"energy_regen_rate": plan.EnergyRegenRate,
		"max_listings":      plan.MaxListings,
		"referral_bonus":    plan.ReferralBonus,
		"game_bonus":        plan.GameBonus,
		"nft_bonus":         plan.NFTBonus,
		"escrow_discount":   plan.EscrowDiscount,
	}
}

// ProcessRenewals обработка продлений подписок
func (sm *SubscriptionManager) ProcessRenewals(ctx context.Context) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	var renewedSubscriptions []*Subscription

	for _, subscription := range sm.subscriptions {
		if subscription.Status != StatusActive {
			continue
		}

		// Проверка необходимости продления
		if subscription.AutoRenew && now.After(subscription.EndsAt.Add(-sm.config.RenewalReminder)) {
			plan := sm.plans[subscription.Type]
			if plan != nil && plan.Price > 0 {
				// Продление подписки
				subscription.EndsAt = subscription.EndsAt.Add(plan.Duration)
				subscription.NextPaymentAt = subscription.EndsAt
				subscription.LastPaymentAt = now
				subscription.UpdatedAt = now

				renewedSubscriptions = append(renewedSubscriptions, subscription)

				// Обновление метрик
				sm.incrementTotalRevenue(plan.Price)
			}
		}
	}

	if len(renewedSubscriptions) > 0 {
		sm.clearCache("subscriptions")
	}

	return nil
}

// GetSubscriptionStats получает статистику подписок
func (sm *SubscriptionManager) GetSubscriptionStats(ctx context.Context) map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stats := make(map[string]interface{})

	typeCount := make(map[SubscriptionType]int64)
	statusCount := make(map[SubscriptionStatus]int64)

	for _, subscription := range sm.subscriptions {
		typeCount[subscription.Type]++
		statusCount[subscription.Status]++
	}

	stats["total_subscriptions"] = len(sm.subscriptions)
	stats["by_type"] = typeCount
	stats["by_status"] = statusCount
	stats["metrics"] = sm.GetMetrics()

	return stats
}

// Вспомогательные методы

func (sm *SubscriptionManager) createDefaultSubscription(userID int64) *Subscription {
	now := time.Now()
	return &Subscription{
		ID:        sm.generateID("sub_basic"),
		UserID:    userID,
		Type:      SubscriptionBasic,
		Status:    StatusActive,
		StartedAt: now,
		EndsAt:    now.Add(365 * 24 * time.Hour), // Базовая подписка на год
		AutoRenew: false,
		TrialUsed: false,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (sm *SubscriptionManager) isUpgrade(oldType, newType SubscriptionType) bool {
	upgradeOrder := map[SubscriptionType]int{
		SubscriptionBasic:  0,
		SubscriptionSilver: 1,
		SubscriptionGold:   2,
	}

	return upgradeOrder[newType] > upgradeOrder[oldType]
}

func (sm *SubscriptionManager) generateID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

// Кэш методы
func (sm *SubscriptionManager) getFromCache(key string) interface{} {
	sm.cacheMu.RLock()
	defer sm.cacheMu.RUnlock()

	return sm.cache[key]
}

func (sm *SubscriptionManager) setToCache(key string, value interface{}, ttl time.Duration) {
	sm.cacheMu.Lock()
	defer sm.cacheMu.Unlock()

	sm.cache[key] = value

	// Удаление из кэша через TTL
	go func() {
		time.Sleep(ttl)
		sm.cacheMu.Lock()
		delete(sm.cache, key)
		sm.cacheMu.Unlock()
	}()
}

func (sm *SubscriptionManager) clearCache(prefix string) {
	sm.cacheMu.Lock()
	defer sm.cacheMu.Unlock()

	for key := range sm.cache {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(sm.cache, key)
		}
	}
}

// Метрики
func (sm *SubscriptionManager) incrementTotalSubscriptions() {
	sm.metrics.mu.Lock()
	defer sm.metrics.mu.Unlock()
	sm.metrics.TotalSubscriptions++
	sm.metrics.LastUpdated = time.Now()
}

func (sm *SubscriptionManager) incrementActiveSubscriptions() {
	sm.metrics.mu.Lock()
	defer sm.metrics.mu.Unlock()
	sm.metrics.ActiveSubscriptions++
	sm.metrics.LastUpdated = time.Now()
}

func (sm *SubscriptionManager) decrementActiveSubscriptions() {
	sm.metrics.mu.Lock()
	defer sm.metrics.mu.Unlock()
	if sm.metrics.ActiveSubscriptions > 0 {
		sm.metrics.ActiveSubscriptions--
	}
	sm.metrics.LastUpdated = time.Now()
}

func (sm *SubscriptionManager) incrementTypeSubscription(subType SubscriptionType) {
	sm.metrics.mu.Lock()
	defer sm.metrics.mu.Unlock()

	switch subType {
	case SubscriptionBasic:
		sm.metrics.BasicSubscriptions++
	case SubscriptionSilver:
		sm.metrics.SilverSubscriptions++
	case SubscriptionGold:
		sm.metrics.GoldSubscriptions++
	}
	sm.metrics.LastUpdated = time.Now()
}

func (sm *SubscriptionManager) decrementTypeSubscription(subType SubscriptionType) {
	sm.metrics.mu.Lock()
	defer sm.metrics.mu.Unlock()

	switch subType {
	case SubscriptionBasic:
		if sm.metrics.BasicSubscriptions > 0 {
			sm.metrics.BasicSubscriptions--
		}
	case SubscriptionSilver:
		if sm.metrics.SilverSubscriptions > 0 {
			sm.metrics.SilverSubscriptions--
		}
	case SubscriptionGold:
		if sm.metrics.GoldSubscriptions > 0 {
			sm.metrics.GoldSubscriptions--
		}
	}
	sm.metrics.LastUpdated = time.Now()
}

func (sm *SubscriptionManager) incrementTotalRevenue(amount int64) {
	sm.metrics.mu.Lock()
	defer sm.metrics.mu.Unlock()
	sm.metrics.TotalRevenue += amount
	sm.metrics.MonthlyRevenue += amount
	sm.metrics.LastUpdated = time.Now()
}

// GetMetrics возвращает метрики
func (sm *SubscriptionManager) GetMetrics() SubscriptionMetrics {
	sm.metrics.mu.RLock()
	defer sm.metrics.mu.RUnlock()

	return *sm.metrics
}
