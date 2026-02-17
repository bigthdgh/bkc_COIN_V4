package admin

import (
	"context"
	"fmt"
	"log"
	"time"

	"bkc_coin_v2/internal/db"
)

// AdminManager управляет админ-панелью
type AdminManager struct {
	db *db.DB
}

// NewAdminManager создает новый менеджер админ-панели
func NewAdminManager(database *db.DB) *AdminManager {
	return &AdminManager{db: database}
}

// AdminUser представляет администратора
type AdminUser struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`        // super_admin, admin, moderator
	Permissions []string   `json:"permissions"` // god_mode, users, market, bank, games, analytics
	IsActive    bool       `json:"is_active"`
	LastLogin   *time.Time `json:"last_login"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// UserManagement управление пользователями
type UserManagement struct {
	UserID       int64      `json:"user_id"`
	Username     string     `json:"username"`
	Balance      int64      `json:"balance"`
	Level        int        `json:"level"`
	IsSubscribed bool       `json:"is_subscribed"`
	IsBanned     bool       `json:"is_banned"`
	BanReason    string     `json:"ban_reason"`
	BanExpiresAt *time.Time `json:"ban_expires_at"`
	LastActive   time.Time  `json:"last_active"`
	Referrals    int        `json:"referrals"`
	TapsTotal    int64      `json:"taps_total"`
	CreatedAt    time.Time  `json:"created_at"`
}

// MarketManagement управление маркетплейсом
type MarketManagement struct {
	ListingID    int64      `json:"listing_id"`
	UserID       int64      `json:"user_id"`
	Username     string     `json:"username"`
	ItemType     string     `json:"item_type"` // nft, physical
	ItemName     string     `json:"item_name"`
	Price        int64      `json:"price"`
	Status       string     `json:"status"` // active, pending, rejected, sold
	IsApproved   bool       `json:"is_approved"`
	ApprovedBy   *int64     `json:"approved_by"`
	ApprovedAt   *time.Time `json:"approved_at"`
	RejectedBy   *int64     `json:"rejected_by"`
	RejectedAt   *time.Time `json:"rejected_at"`
	RejectReason string     `json:"reject_reason"`
	CreatedAt    time.Time  `json:"created_at"`
}

// BankManagement управление банком
type BankManagement struct {
	LoanID        int64     `json:"loan_id"`
	UserID        int64     `json:"user_id"`
	Username      string    `json:"username"`
	Principal     int64     `json:"principal"`
	InterestTotal int64     `json:"interest_total"`
	TotalDue      int64     `json:"total_due"`
	Status        string    `json:"status"` // active, repaid, defaulted, collector
	DueAt         time.Time `json:"due_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// Analytics аналитика
type Analytics struct {
	TotalUsers    int64 `json:"total_users"`
	ActiveUsers   int64 `json:"active_users"`
	TotalBalance  int64 `json:"total_balance"`
	TotalTaps     int64 `json:"total_taps"`
	TotalNFTs     int64 `json:"total_nfts"`
	ActiveLoans   int64 `json:"active_loans"`
	TotalDebt     int64 `json:"total_debt"`
	MarketVolume  int64 `json:"market_volume"`
	GamesRevenue  int64 `json:"games_revenue"`
	Subscribers   int64 `json:"subscribers"`
	NewUsersToday int64 `json:"new_users_today"`
	TapsToday     int64 `json:"taps_today"`
	RevenueToday  int64 `json:"revenue_today"`
}

// GodModeAction действие God Mode
type GodModeAction struct {
	AdminID    int64     `json:"admin_id"`
	ActionType string    `json:"action_type"` // add_balance, remove_balance, ban_user, unban_user, approve_market, reject_market
	TargetID   int64     `json:"target_id"`
	Amount     int64     `json:"amount"`
	Reason     string    `json:"reason"`
	Metadata   string    `json:"metadata"`
	CreatedAt  time.Time `json:"created_at"`
}

// SubscriptionCheck проверка подписки
type SubscriptionCheck struct {
	UserID        int64      `json:"user_id"`
	Username      string     `json:"username"`
	IsSubscribed  bool       `json:"is_subscribed"`
	PlanType      string     `json:"plan_type"` // basic, silver, gold
	ExpiresAt     *time.Time `json:"expires_at"`
	DaysRemaining int        `json:"days_remaining"`
	LastChecked   time.Time  `json:"last_checked"`
	IsValid       bool       `json:"is_valid"`
}

// AuthenticateAdmin аутентифицирует администратора
func (am *AdminManager) AuthenticateAdmin(ctx context.Context, username, password string) (*AdminUser, error) {
	var admin AdminUser
	err := am.db.Pool.QueryRow(ctx, `
		SELECT id, username, email, role, permissions, is_active, last_login, created_at, updated_at
		FROM admin_users 
		WHERE username = $1 AND password_hash = crypt($2, password_hash) AND is_active = true
	`, username, password).Scan(&admin.ID, &admin.Username, &admin.Email,
		&admin.Role, &admin.Permissions, &admin.IsActive, &admin.LastLogin,
		&admin.CreatedAt, &admin.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	// Обновляем время последнего входа
	_, err = am.db.Pool.Exec(ctx, "UPDATE admin_users SET last_login = $1 WHERE id = $2", time.Now(), admin.ID)
	if err != nil {
		log.Printf("Failed to update admin last login: %v", err)
	}

	return &admin, nil
}

// HasPermission проверяет наличие разрешения
func (am *AdminManager) HasPermission(admin *AdminUser, permission string) bool {
	for _, p := range admin.Permissions {
		if p == permission || p == "god_mode" {
			return true
		}
	}
	return false
}

// GetUsersList получает список пользователей
func (am *AdminManager) GetUsersList(ctx context.Context, limit, offset int, filter string) ([]UserManagement, error) {
	query := `
		SELECT u.user_id, u.username, u.balance, u.level, u.is_subscribed, 
		       u.is_banned, u.ban_reason, u.ban_expires_at, u.last_active,
		       (SELECT COUNT(*) FROM referrals WHERE referrer_id = u.user_id) as referrals,
		       u.taps_total, u.created_at
		FROM users u
		WHERE 1=1
	`
	args := []interface{}{}
	argIndex := 1

	if filter != "" {
		query += fmt.Sprintf(" AND (u.username ILIKE $%d OR u.email ILIKE $%d)", argIndex, argIndex+1)
		args = append(args, "%"+filter+"%", "%"+filter+"%")
		argIndex += 2
	}

	query += fmt.Sprintf(" ORDER BY u.created_at DESC LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, limit, offset)

	rows, err := am.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get users list: %w", err)
	}
	defer rows.Close()

	var users []UserManagement
	for rows.Next() {
		var user UserManagement
		err := rows.Scan(
			&user.UserID, &user.Username, &user.Balance, &user.Level,
			&user.IsSubscribed, &user.IsBanned, &user.BanReason,
			&user.BanExpiresAt, &user.LastActive, &user.Referrals,
			&user.TapsTotal, &user.CreatedAt,
		)
		if err != nil {
			continue
		}
		users = append(users, user)
	}

	return users, rows.Err()
}

// BanUser блокирует пользователя
func (am *AdminManager) BanUser(ctx context.Context, adminID, userID int64, reason string, days int) error {
	var expiresAt *time.Time
	if days > 0 {
		exp := time.Now().AddDate(0, 0, days)
		expiresAt = &exp
	}

	tx, err := am.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Блокируем пользователя
	_, err = tx.Exec(ctx, `
		UPDATE users 
		SET is_banned = true, ban_reason = $1, ban_expires_at = $2
		WHERE user_id = $3
	`, reason, expiresAt, userID)
	if err != nil {
		return fmt.Errorf("failed to ban user: %w", err)
	}

	// Записываем действие God Mode
	_, err = tx.Exec(ctx, `
		INSERT INTO god_mode_actions(admin_id, action_type, target_id, reason, created_at)
		VALUES($1, 'ban_user', $2, $3, $4)
	`, adminID, userID, reason, time.Now())
	if err != nil {
		return fmt.Errorf("failed to record god mode action: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit ban: %w", err)
	}

	log.Printf("Admin %d banned user %d: %s (expires: %v)", adminID, userID, reason, expiresAt)

	return nil
}

// UnbanUser разблокирует пользователя
func (am *AdminManager) UnbanUser(ctx context.Context, adminID, userID int64) error {
	tx, err := am.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Разблокируем пользователя
	_, err = tx.Exec(ctx, `
		UPDATE users 
		SET is_banned = false, ban_reason = NULL, ban_expires_at = NULL
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to unban user: %w", err)
	}

	// Записываем действие God Mode
	_, err = tx.Exec(ctx, `
		INSERT INTO god_mode_actions(admin_id, action_type, target_id, created_at)
		VALUES($1, 'unban_user', $2, $3)
	`, adminID, userID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to record god mode action: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit unban: %w", err)
	}

	log.Printf("Admin %d unbanned user %d", adminID, userID)

	return nil
}

// AddBalance добавляет баланс пользователю (God Mode)
func (am *AdminManager) AddBalance(ctx context.Context, adminID, userID int64, amount int64, reason string) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	tx, err := am.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Добавляем баланс
	_, err = tx.Exec(ctx, "UPDATE users SET balance = balance + $1 WHERE user_id = $2", amount, userID)
	if err != nil {
		return fmt.Errorf("failed to add balance: %w", err)
	}

	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, from_id, to_id, amount, meta)
		VALUES('admin_add', NULL, $1, $2, $3::jsonb)
	`, userID, amount, fmt.Sprintf(`{
		"admin_id": %d,
		"reason": "%s"
	}`, adminID, reason))
	if err != nil {
		return fmt.Errorf("failed to record ledger: %w", err)
	}

	// Записываем действие God Mode
	_, err = tx.Exec(ctx, `
		INSERT INTO god_mode_actions(admin_id, action_type, target_id, amount, reason, created_at)
		VALUES($1, 'add_balance', $2, $3, $4, $5)
	`, adminID, userID, amount, reason, time.Now())
	if err != nil {
		return fmt.Errorf("failed to record god mode action: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit add balance: %w", err)
	}

	log.Printf("Admin %d added %d BKC to user %d: %s", adminID, amount, userID, reason)

	return nil
}

// RemoveBalance удаляет баланс у пользователя (God Mode)
func (am *AdminManager) RemoveBalance(ctx context.Context, adminID, userID int64, amount int64, reason string) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	tx, err := am.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Проверяем баланс
	var currentBalance int64
	err = tx.QueryRow(ctx, "SELECT balance FROM users WHERE user_id = $1 FOR UPDATE", userID).Scan(&currentBalance)
	if err != nil {
		return fmt.Errorf("failed to get user balance: %w", err)
	}

	if currentBalance < amount {
		return fmt.Errorf("insufficient balance: current %d, trying to remove %d", currentBalance, amount)
	}

	// Удаляем баланс
	_, err = tx.Exec(ctx, "UPDATE users SET balance = balance - $1 WHERE user_id = $2", amount, userID)
	if err != nil {
		return fmt.Errorf("failed to remove balance: %w", err)
	}

	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, from_id, to_id, amount, meta)
		VALUES('admin_remove', $1, NULL, $2, $3::jsonb)
	`, userID, amount, fmt.Sprintf(`{
		"admin_id": %d,
		"reason": "%s"
	}`, adminID, reason))
	if err != nil {
		return fmt.Errorf("failed to record ledger: %w", err)
	}

	// Записываем действие God Mode
	_, err = tx.Exec(ctx, `
		INSERT INTO god_mode_actions(admin_id, action_type, target_id, amount, reason, created_at)
		VALUES($1, 'remove_balance', $2, $3, $4, $5)
	`, adminID, userID, amount, reason, time.Now())
	if err != nil {
		return fmt.Errorf("failed to record god mode action: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit remove balance: %w", err)
	}

	log.Printf("Admin %d removed %d BKC from user %d: %s", adminID, amount, userID, reason)

	return nil
}

// GetMarketListings получает объявления маркетплейса
func (am *AdminManager) GetMarketListings(ctx context.Context, status string, limit, offset int) ([]MarketManagement, error) {
	query := `
		SELECT ml.id, ml.user_id, u.username, ml.item_type, ml.item_name, 
		       ml.price, ml.status, ml.is_approved, ml.approved_by, ml.approved_at,
		       ml.rejected_by, ml.rejected_at, ml.reject_reason, ml.created_at
		FROM market_listings ml
		JOIN users u ON ml.user_id = u.user_id
	`
	args := []interface{}{}

	if status != "" {
		query += " WHERE ml.status = $1"
		args = append(args, status)
	}

	query += " ORDER BY ml.created_at DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
		args = append(args, limit, offset)
	}

	rows, err := am.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get market listings: %w", err)
	}
	defer rows.Close()

	var listings []MarketManagement
	for rows.Next() {
		var listing MarketManagement
		err := rows.Scan(
			&listing.ListingID, &listing.UserID, &listing.Username,
			&listing.ItemType, &listing.ItemName, &listing.Price,
			&listing.Status, &listing.IsApproved, &listing.ApprovedBy,
			&listing.ApprovedAt, &listing.RejectedBy, &listing.RejectedAt,
			&listing.RejectReason, &listing.CreatedAt,
		)
		if err != nil {
			continue
		}
		listings = append(listings, listing)
	}

	return listings, rows.Err()
}

// ApproveMarketListing одобряет объявление
func (am *AdminManager) ApproveMarketListing(ctx context.Context, adminID, listingID int64) error {
	tx, err := am.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Одобряем объявление
	now := time.Now()
	_, err = tx.Exec(ctx, `
		UPDATE market_listings 
		SET status = 'active', is_approved = true, approved_by = $1, approved_at = $2
		WHERE id = $3
	`, adminID, now, listingID)
	if err != nil {
		return fmt.Errorf("failed to approve listing: %w", err)
	}

	// Записываем действие God Mode
	_, err = tx.Exec(ctx, `
		INSERT INTO god_mode_actions(admin_id, action_type, target_id, created_at)
		VALUES($1, 'approve_market', $2, $3)
	`, adminID, listingID, now)
	if err != nil {
		return fmt.Errorf("failed to record god mode action: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit approval: %w", err)
	}

	log.Printf("Admin %d approved market listing %d", adminID, listingID)

	return nil
}

// RejectMarketListing отклоняет объявление
func (am *AdminManager) RejectMarketListing(ctx context.Context, adminID, listingID int64, reason string) error {
	tx, err := am.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Отклоняем объявление
	now := time.Now()
	_, err = tx.Exec(ctx, `
		UPDATE market_listings 
		SET status = 'rejected', is_approved = false, rejected_by = $1, rejected_at = $2, reject_reason = $3
		WHERE id = $4
	`, adminID, now, reason, listingID)
	if err != nil {
		return fmt.Errorf("failed to reject listing: %w", err)
	}

	// Записываем действие God Mode
	_, err = tx.Exec(ctx, `
		INSERT INTO god_mode_actions(admin_id, action_type, target_id, reason, created_at)
		VALUES($1, 'reject_market', $2, $3, $4)
	`, adminID, listingID, reason, now)
	if err != nil {
		return fmt.Errorf("failed to record god mode action: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit rejection: %w", err)
	}

	log.Printf("Admin %d rejected market listing %d: %s", adminID, listingID, reason)

	return nil
}

// GetBankLoans получает кредиты банка
func (am *AdminManager) GetBankLoans(ctx context.Context, status string, limit, offset int) ([]BankManagement, error) {
	query := `
		SELECT bl.id, bl.user_id, u.username, bl.principal, bl.interest_total,
		       bl.total_due, bl.status, bl.due_at, bl.created_at
		FROM bank_loans bl
		JOIN users u ON bl.user_id = u.user_id
	`
	args := []interface{}{}

	if status != "" {
		query += " WHERE bl.status = $1"
		args = append(args, status)
	}

	query += " ORDER BY bl.created_at DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
		args = append(args, limit, offset)
	}

	rows, err := am.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get bank loans: %w", err)
	}
	defer rows.Close()

	var loans []BankManagement
	for rows.Next() {
		var loan BankManagement
		err := rows.Scan(
			&loan.LoanID, &loan.UserID, &loan.Username,
			&loan.Principal, &loan.InterestTotal, &loan.TotalDue,
			&loan.Status, &loan.DueAt, &loan.CreatedAt,
		)
		if err != nil {
			continue
		}
		loans = append(loans, loan)
	}

	return loans, rows.Err()
}

// GetAnalytics получает аналитику
func (am *AdminManager) GetAnalytics(ctx context.Context) (*Analytics, error) {
	var analytics Analytics

	// Общая статистика
	am.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&analytics.TotalUsers)
	am.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE last_active > NOW() - INTERVAL '24 hours'").Scan(&analytics.ActiveUsers)
	am.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(balance), 0) FROM users").Scan(&analytics.TotalBalance)
	am.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(taps_total), 0) FROM users").Scan(&analytics.TotalTaps)

	// NFT статистика
	am.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM user_nfts").Scan(&analytics.TotalNFTs)

	// Кредиты
	am.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM bank_loans WHERE status = 'active'").Scan(&analytics.ActiveLoans)
	am.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(total_due), 0) FROM bank_loans WHERE status = 'active'").Scan(&analytics.TotalDebt)

	// Маркетплейс
	am.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(price), 0) FROM market_listings WHERE status = 'sold'").Scan(&analytics.MarketVolume)

	// Игры
	am.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(system_profit), 0) FROM crash_games").Scan(&analytics.GamesRevenue)

	// Подписки
	am.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE is_subscribed = true").Scan(&analytics.Subscribers)

	// Статистика за сегодня
	today := time.Now().Truncate(24 * time.Hour)
	am.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE created_at >= $1", today).Scan(&analytics.NewUsersToday)
	am.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(taps_today), 0) FROM user_daily WHERE date = $1", today).Scan(&analytics.TapsToday)

	// Доход за сегодня (сумма всех транзакций с налогом)
	am.db.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount * 0.1), 0) 
		FROM ledger 
		WHERE created_at >= $1 AND kind IN ('transfer', 'nft_purchase', 'market_sale')
	`, today).Scan(&analytics.RevenueToday)

	return &analytics, nil
}

// CheckSubscription проверяет подписку пользователя
func (am *AdminManager) CheckSubscription(ctx context.Context, userID int64) (*SubscriptionCheck, error) {
	var check SubscriptionCheck
	var expiresAt *time.Time

	err := am.db.Pool.QueryRow(ctx, `
		SELECT u.user_id, u.username, u.is_subscribed, s.plan_type, s.expires_at
		FROM users u
		LEFT JOIN user_subscriptions s ON u.user_id = s.user_id AND s.is_active = true
		WHERE u.user_id = $1
	`, userID).Scan(&check.UserID, &check.Username, &check.IsSubscribed,
		&check.PlanType, &expiresAt)

	if err != nil {
		return nil, fmt.Errorf("failed to check subscription: %w", err)
	}

	check.ExpiresAt = expiresAt
	check.LastChecked = time.Now()

	// Проверяем валидность подписки
	if check.IsSubscribed && check.ExpiresAt != nil {
		check.IsValid = time.Now().Before(*check.ExpiresAt)
		check.DaysRemaining = int(check.ExpiresAt.Sub(time.Now()).Hours() / 24)
		if check.DaysRemaining < 0 {
			check.DaysRemaining = 0
		}
	} else {
		check.IsValid = false
		check.DaysRemaining = 0
	}

	// Если подписка истекла, деактивируем
	if !check.IsValid && check.IsSubscribed {
		_, err = am.db.Pool.Exec(ctx, `
			UPDATE users SET is_subscribed = false WHERE user_id = $1
		`, userID)
		if err != nil {
			log.Printf("Failed to deactivate expired subscription for user %d: %v", userID, err)
		}
		check.IsSubscribed = false
	}

	return &check, nil
}

// GetGodModeActions получает действия God Mode
func (am *AdminManager) GetGodModeActions(ctx context.Context, adminID int64, limit, offset int) ([]GodModeAction, error) {
	query := `
		SELECT gma.admin_id, au.username, gma.action_type, gma.target_id, 
		       gma.amount, gma.reason, gma.metadata, gma.created_at
		FROM god_mode_actions gma
		JOIN admin_users au ON gma.admin_id = au.id
	`
	args := []interface{}{}

	if adminID > 0 {
		query += " WHERE gma.admin_id = $1"
		args = append(args, adminID)
	}

	query += " ORDER BY gma.created_at DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
		args = append(args, limit, offset)
	}

	rows, err := am.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get god mode actions: %w", err)
	}
	defer rows.Close()

	var actions []GodModeAction
	for rows.Next() {
		var action GodModeAction
		var adminUsername string
		err := rows.Scan(
			&action.AdminID, &adminUsername, &action.ActionType,
			&action.TargetID, &action.Amount, &action.Reason,
			&action.Metadata, &action.CreatedAt,
		)
		if err != nil {
			continue
		}
		actions = append(actions, action)
	}

	return actions, rows.Err()
}

// GetAdminStats получает статистику администраторов
func (am *AdminManager) GetAdminStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Количество администраторов по ролям
	var superAdmins, admins, moderators int64
	am.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM admin_users WHERE role = 'super_admin' AND is_active = true").Scan(&superAdmins)
	am.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM admin_users WHERE role = 'admin' AND is_active = true").Scan(&admins)
	am.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM admin_users WHERE role = 'moderator' AND is_active = true").Scan(&moderators)

	// Действия God Mode за сегодня
	today := time.Now().Truncate(24 * time.Hour)
	var godModeActionsToday int64
	am.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM god_mode_actions WHERE created_at >= $1", today).Scan(&godModeActionsToday)

	// Ожидающие объявления
	var pendingListings int64
	am.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM market_listings WHERE status = 'pending'").Scan(&pendingListings)

	// Просроченные кредиты
	var overdueLoans int64
	am.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM bank_loans WHERE status = 'active' AND due_at < NOW()").Scan(&overdueLoans)

	stats["super_admins"] = superAdmins
	stats["admins"] = admins
	stats["moderators"] = moderators
	stats["total_admins"] = superAdmins + admins + moderators
	stats["god_mode_actions_today"] = godModeActionsToday
	stats["pending_listings"] = pendingListings
	stats["overdue_loans"] = overdueLoans

	return stats, nil
}
