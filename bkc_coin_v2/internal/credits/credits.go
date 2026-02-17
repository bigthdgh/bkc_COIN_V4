package credits

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"bkc_coin_v2/internal/db"
)

// CreditsManager управляет кредитной системой
type CreditsManager struct {
	db *db.DB
}

// NewCreditsManager создает новый менеджер кредитов
func NewCreditsManager(database *db.DB) *CreditsManager {
	return &CreditsManager{db: database}
}

// BankLoan системный кредит
type BankLoan struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	Principal      int64      `json:"principal"`
	InterestRate   float64    `json:"interest_rate"` // % в день
	InterestTotal  int64      `json:"interest_total"`
	TotalDue       int64      `json:"total_due"`
	TermDays       int        `json:"term_days"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	DueAt          time.Time  `json:"due_at"`
	ClosedAt       *time.Time `json:"closed_at"`
	CollectorStartedAt *time.Time `json:"collector_started_at"`
	DailyCollected int64      `json:"daily_collected"`
}

// P2PLoan P2P кредит между пользователями
type P2PLoan struct {
	ID              int64      `json:"id"`
	LenderID        int64      `json:"lender_id"`
	BorrowerID      int64      `json:"borrower_id"`
	Principal       int64      `json:"principal"`
	InterestRate    float64    `json:"interest_rate"`
	InterestTotal   int64      `json:"interest_total"`
	TotalDue        int64      `json:"total_due"`
	CollateralType  string     `json:"collateral_type"` // nft, bkc
	CollateralValue int64      `json:"collateral_value"`
	CollateralNFTID int        `json:"collateral_nft_id"`
	TermDays        int        `json:"term_days"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	AcceptedAt      *time.Time `json:"accepted_at"`
	DueAt           *time.Time `json:"due_at"`
	ClosedAt        *time.Time `json:"closed_at"`
	DefaultedAt     *time.Time `json:"defaulted_at"`
}

// LoanRequest запрос на кредит
type LoanRequest struct {
	UserID    int64 `json:"user_id"`
	Amount    int64 `json:"amount"`
	TermDays  int   `json:"term_days"`
	LoanType  string `json:"loan_type"` // bank, p2p
}

// LoanEligibility проверка eligibility для кредита
type LoanEligibility struct {
	IsEligible      bool    `json:"is_eligible"`
	MaxAmount       int64   `json:"max_amount"`
	Reason          string  `json:"reason"`
	UserLevel       int     `json:"user_level"`
	LifetimeTaps    int64   `json:"lifetime_taps"`
	RequiredLevel   int     `json:"required_level"`
	RequiredTaps    int64   `json:"required_taps"`
	IsSubscribed    bool    `json:"is_subscribed"`
	ExistingDebt    int64   `json:"existing_debt"`
}

// TakeBankLoan берет системный кредит
func (cm *CreditsManager) TakeBankLoan(ctx context.Context, req *LoanRequest) (*BankLoan, error) {
	if req.LoanType != "bank" {
		return nil, fmt.Errorf("invalid loan type")
	}
	
	// Проверяем eligibility
	eligibility, err := cm.CheckBankLoanEligibility(ctx, req.UserID, req.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to check eligibility: %w", err)
	}
	
	if !eligibility.IsEligible {
		return nil, fmt.Errorf("not eligible for loan: %s", eligibility.Reason)
	}
	
	if req.Amount > eligibility.MaxAmount {
		return nil, fmt.Errorf("amount %d exceeds maximum %d", req.Amount, eligibility.MaxAmount)
	}
	
	// Параметры кредита
	interestRate := 5.0 // 5% в день
	if req.TermDays > 7 {
		interestRate = 7.0 // 7% для кредитов больше 7 дней
	}
	
	interestTotal := int64(math.Floor(float64(req.Amount) * interestRate * float64(req.TermDays) / 100))
	totalDue := req.Amount + interestTotal
	dueAt := time.Now().AddDate(0, 0, req.TermDays)
	
	tx, err := cm.db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	
	// Проверяем и резервируем монеты
	var reserveSupply int64
	err = tx.QueryRow(ctx, "SELECT reserve_supply FROM system_state WHERE id = 1 FOR UPDATE").Scan(&reserveSupply)
	if err != nil {
		return nil, fmt.Errorf("failed to get reserve: %w", err)
	}
	
	if reserveSupply < req.Amount {
		return nil, fmt.Errorf("insufficient reserve supply")
	}
	
	// Создаем кредит
	var loanID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO bank_loans(
			user_id, principal, interest_rate, interest_total, total_due, term_days, due_at
		) VALUES($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`, req.UserID, req.Amount, interestRate, interestTotal, totalDue, req.TermDays, dueAt).
		Scan(&loanID, &dueAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create loan: %w", err)
	}
	
	// Уменьшаем резерв
	_, err = tx.Exec(ctx, "UPDATE system_state SET reserve_supply = reserve_supply - $1 WHERE id = 1", req.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to update reserve: %w", err)
	}
	
	// Выдаем монеты пользователю
	_, err = tx.Exec(ctx, "UPDATE users SET balance = balance + $1, loan_debt = COALESCE(loan_debt, 0) + $2 WHERE user_id = $3", 
		req.Amount, totalDue, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to credit user: %w", err)
	}
	
	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, from_id, to_id, amount, meta)
		VALUES('bank_loan', NULL, $1, $2, $3::jsonb)
	`, req.UserID, req.Amount, fmt.Sprintf(`{
		"loan_id": %d,
		"principal": %d,
		"interest": %d,
		"term_days": %d,
		"interest_rate": %.2f
	}`, loanID, req.Amount, interestTotal, req.TermDays, interestRate))
	if err != nil {
		return nil, fmt.Errorf("failed to record loan: %w", err)
	}
	
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit loan: %w", err)
	}
	
	loan := &BankLoan{
		ID:            loanID,
		UserID:        req.UserID,
		Principal:     req.Amount,
		InterestRate:  interestRate,
		InterestTotal: interestTotal,
		TotalDue:      totalDue,
		TermDays:      req.TermDays,
		Status:        "active",
		CreatedAt:     dueAt,
		DueAt:         dueAt,
	}
	
	log.Printf("Bank loan %d created: user %d, amount %d, total due %d, rate %.2f%%, term %d days", 
		loanID, req.UserID, req.Amount, totalDue, interestRate, req.TermDays)
	
	return loan, nil
}

// CheckBankLoanEligibility проверяет eligibility для системного кредита
func (cm *CreditsManager) CheckBankLoanEligibility(ctx context.Context, userID, amount int64) (*LoanEligibility, error) {
	var userLevel, tapsTotal int64
	var isSubscribed bool
	var existingDebt int64
	
	err := cm.db.Pool.QueryRow(ctx, `
		SELECT level, taps_total, is_subscribed, COALESCE(loan_debt, 0)
		FROM users WHERE user_id = $1
	`, userID).Scan(&userLevel, &tapsTotal, &isSubscribed, &existingDebt)
	if err != nil {
		return nil, fmt.Errorf("failed to get user data: %w", err)
	}
	
	eligibility := &LoanEligibility{
		UserLevel:    int(userLevel),
		LifetimeTaps: tapsTotal,
		IsSubscribed: isSubscribed,
		ExistingDebt: existingDebt,
		RequiredLevel: 5,
		RequiredTaps: tapsTotal * 3, // Нужно натапать в 3 раза больше чем кредит
	}
	
	// Проверяем уровень
	if userLevel < 5 {
		eligibility.IsEligible = false
		eligibility.Reason = fmt.Sprintf("Need level 5+, current level: %d", userLevel)
		return eligibility, nil
	}
	
	// Проверяем подписку
	if !isSubscribed {
		eligibility.IsEligible = false
		eligibility.Reason = "Subscription required"
		return eligibility, nil
	}
	
	// Проверяем существующий долг
	if existingDebt > 0 {
		eligibility.IsEligible = false
		eligibility.Reason = fmt.Sprintf("Existing debt: %d BKC", existingDebt)
		return eligibility, nil
	}
	
	// Рассчитываем максимальную сумму (30% от lifetime taps)
	maxAmount := tapsTotal * 30 / 100
	if maxAmount > 2_000_000 { // Максимальный лимит
		maxAmount = 2_000_000
	}
	
	eligibility.MaxAmount = maxAmount
	
	// Проверяем запрошенную сумму
	if amount > maxAmount {
		eligibility.IsEligible = false
		eligibility.Reason = fmt.Sprintf("Amount %d exceeds maximum %d", amount, maxAmount)
		return eligibility, nil
	}
	
	eligibility.IsEligible = true
	return eligibility, nil
}

// RepayBankLoan погашает системный кредит
func (cm *CreditsManager) RepayBankLoan(ctx context.Context, userID, loanID, amount int64) error {
	tx, err := cm.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	
	// Получаем информацию о кредите
	var loan BankLoan
	err = tx.QueryRow(ctx, `
		SELECT id, user_id, principal, interest_total, total_due, status, due_at
		FROM bank_loans 
		WHERE id = $1 AND user_id = $2 AND status = 'active' FOR UPDATE
	`, loanID, userID).Scan(&loan.ID, &loan.UserID, &loan.Principal, 
		&loan.InterestTotal, &loan.TotalDue, &loan.Status, &loan.DueAt)
	
	if err != nil {
		return fmt.Errorf("loan not found or not active: %w", err)
	}
	
	// Проверяем баланс пользователя
	var userBalance int64
	err = tx.QueryRow(ctx, "SELECT balance FROM users WHERE user_id = $1 FOR UPDATE", userID).Scan(&userBalance)
	if err != nil {
		return fmt.Errorf("failed to get user balance: %w", err)
	}
	
	if userBalance < amount {
		return fmt.Errorf("insufficient balance: need %d, have %d", amount, userBalance)
	}
	
	// Определяем сумму погашения
	repayAmount := amount
	if repayAmount > loan.TotalDue {
		repayAmount = loan.TotalDue
	}
	
	// Списываем монеты
	_, err = tx.Exec(ctx, "UPDATE users SET balance = balance - $1 WHERE user_id = $2", repayAmount, userID)
	if err != nil {
		return fmt.Errorf("failed to deduct balance: %w", err)
	}
	
	// Возвращаем монеты в резерв
	_, err = tx.Exec(ctx, "UPDATE system_state SET reserve_supply = reserve_supply + $1 WHERE id = 1", repayAmount)
	if err != nil {
		return fmt.Errorf("failed to return to reserve: %w", err)
	}
	
	// Обновляем долг
	remainingDebt := loan.TotalDue - repayAmount
	if remainingDebt <= 0 {
		// Кредит погашен
		_, err = tx.Exec(ctx, `
			UPDATE bank_loans 
			SET status = 'repaid', closed_at = $1
			WHERE id = $2
		`, time.Now(), loanID)
		if err != nil {
			return fmt.Errorf("failed to update loan status: %w", err)
		}
		
		// Обнуляем долг пользователя
		_, err = tx.Exec(ctx, "UPDATE users SET loan_debt = 0 WHERE user_id = $1", userID)
		if err != nil {
			return fmt.Errorf("failed to clear user debt: %w", err)
		}
	} else {
		// Частичное погашение
		_, err = tx.Exec(ctx, "UPDATE users SET loan_debt = $1 WHERE user_id = $2", remainingDebt, userID)
		if err != nil {
			return fmt.Errorf("failed to update user debt: %w", err)
		}
	}
	
	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, from_id, to_id, amount, meta)
		VALUES('bank_loan_repay', $1, NULL, $2, $3::jsonb)
	`, userID, repayAmount, fmt.Sprintf(`{
		"loan_id": %d,
		"repaid_amount": %d,
		"remaining_debt": %d
	}`, loanID, repayAmount, remainingDebt))
	if err != nil {
		return fmt.Errorf("failed to record repayment: %w", err)
	}
	
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit repayment: %w", err)
	}
	
	log.Printf("User %d repaid %d BKC of bank loan %d (remaining: %d)", 
		userID, repayAmount, loanID, remainingDebt)
	
	return nil
}

// ProcessOverdueLoans обрабатывает просроченные кредиты
func (cm *CreditsManager) ProcessOverdueLoans(ctx context.Context) (int, error) {
	now := time.Now()
	
	// Находим просроченные кредиты
	rows, err := cm.db.Pool.Query(ctx, `
		SELECT id, user_id, total_due, daily_collected
		FROM bank_loans 
		WHERE status = 'active' AND due_at < $1
	`, now)
	if err != nil {
		return 0, fmt.Errorf("failed to get overdue loans: %w", err)
	}
	defer rows.Close()
	
	processed := 0
	for rows.Next() {
		var loanID, userID, totalDue, dailyCollected int64
		if err := rows.Scan(&loanID, &userID, &totalDue, &dailyCollected); err != nil {
			continue
		}
		
		// Включаем режим коллектора
		err := cm.EnableCollectorMode(ctx, userID, loanID)
		if err != nil {
			log.Printf("Failed to enable collector mode for user %d: %v", userID, err)
			continue
		}
		
		processed++
	}
	
	return processed, rows.Err()
}

// EnableCollectorMode включает режим коллектора
func (cm *CreditsManager) EnableCollectorMode(ctx context.Context, userID, loanID int64) error {
	tx, err := cm.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	
	// Включаем режим коллектора пользователю
	_, err = tx.Exec(ctx, `
		UPDATE users 
		SET collector_mode = true
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to enable collector mode: %w", err)
	}
	
	// Обновляем статус кредита
	_, err = tx.Exec(ctx, `
		UPDATE bank_loans 
		SET status = 'collector', collector_started_at = $1
		WHERE id = $2
	`, time.Now(), loanID)
	if err != nil {
		return fmt.Errorf("failed to update loan status: %w", err)
	}
	
	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, amount, meta)
		VALUES('collector_mode', 0, $1::jsonb)
	`, fmt.Sprintf(`{
		"user_id": %d,
		"loan_id": %d,
		"started_at": "%s"
	}`, userID, loanID, time.Now().Format(time.RFC3339)))
	if err != nil {
		return fmt.Errorf("failed to record collector mode: %w", err)
	}
	
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit collector mode: %w", err)
	}
	
	log.Printf("Collector mode enabled for user %d (loan %d)", userID, loanID)
	
	return nil
}

// CreateP2PLoanRequest создает запрос на P2P кредит
func (cm *CreditsManager) CreateP2PLoanRequest(ctx context.Context, req *LoanRequest, interestRate float64, collateralType string, collateralValue int64, collateralNFTID int) (*P2PLoan, error) {
	if req.LoanType != "p2p" {
		return nil, fmt.Errorf("invalid loan type")
	}
	
	// Рассчитываем общую сумму к возврату
	interestTotal := int64(math.Floor(float64(req.Amount) * interestRate * float64(req.TermDays) / 100))
	totalDue := req.Amount + interestTotal
	dueAt := time.Now().AddDate(0, 0, req.TermDays)
	
	tx, err := cm.db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	
	// Если залог в виде NFT, проверяем и блокируем его
	if collateralType == "nft" && collateralNFTID > 0 {
		var userNFTCount int64
		err = tx.QueryRow(ctx, `
			SELECT qty FROM user_nfts 
			WHERE user_id = $1 AND nft_id = $2 AND qty > 0 AND is_collateral = false FOR UPDATE
		`, req.UserID, collateralNFTID).Scan(&userNFTCount)
		
		if err != nil || userNFTCount <= 0 {
			return nil, fmt.Errorf("NFT not available for collateral")
		}
		
		// Блокируем NFT как залог
		_, err = tx.Exec(ctx, `
			UPDATE user_nfts 
			SET is_collateral = true 
			WHERE user_id = $1 AND nft_id = $2
		`, req.UserID, collateralNFTID)
		if err != nil {
			return nil, fmt.Errorf("failed to lock NFT collateral: %w", err)
		}
	}
	
	// Создаем P2P кредит
	var loanID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO p2p_loans(
			lender_id, borrower_id, principal, interest_rate, interest_total, total_due,
			collateral_type, collateral_value, collateral_nft_id, term_days, due_at
		) VALUES(0, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at
	`, req.UserID, req.Amount, interestRate, interestTotal, totalDue,
		collateralType, collateralValue, collateralNFTID, req.TermDays, dueAt).
		Scan(&loanID, &dueAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create P2P loan: %w", err)
	}
	
	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, amount, meta)
		VALUES('p2p_loan_request', 0, $1::jsonb)
	`, fmt.Sprintf(`{
		"loan_id": %d,
		"borrower_id": %d,
		"principal": %d,
		"interest_rate": %.2f,
		"collateral_type": "%s",
		"collateral_value": %d
	}`, loanID, req.UserID, req.Amount, interestRate, collateralType, collateralValue))
	if err != nil {
		return nil, fmt.Errorf("failed to record loan request: %w", err)
	}
	
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit P2P loan: %w", err)
	}
	
	loan := &P2PLoan{
		ID:              loanID,
		LenderID:        0, // Пока не назначен
		BorrowerID:      req.UserID,
		Principal:       req.Amount,
		InterestRate:    interestRate,
		InterestTotal:   interestTotal,
		TotalDue:        totalDue,
		CollateralType:  collateralType,
		CollateralValue: collateralValue,
		CollateralNFTID: collateralNFTID,
		TermDays:        req.TermDays,
		Status:          "requested",
		CreatedAt:       dueAt,
		DueAt:           &dueAt,
	}
	
	log.Printf("P2P loan request %d created: borrower %d, amount %d, collateral %s", 
		loanID, req.UserID, req.Amount, collateralType)
	
	return loan, nil
}

// AcceptP2PLoan принимает P2P кредит (кредитор подтверждает)
func (cm *CreditsManager) AcceptP2PLoan(ctx context.Context, loanID, lenderID int64) error {
	tx, err := cm.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	
	// Получаем информацию о кредите
	var loan P2PLoan
	err = tx.QueryRow(ctx, `
		SELECT id, borrower_id, principal, total_due, status, collateral_type, collateral_nft_id
		FROM p2p_loans 
		WHERE id = $1 AND status = 'requested' FOR UPDATE
	`, loanID).Scan(&loan.ID, &loan.BorrowerID, &loan.Principal, 
		&loan.TotalDue, &loan.Status, &loan.CollateralType, &loan.CollateralNFTID)
	
	if err != nil {
		return fmt.Errorf("loan not found or not requested: %w", err)
	}
	
	if loan.BorrowerID == lenderID {
		return fmt.Errorf("cannot accept own loan request")
	}
	
	// Проверяем баланс кредитора
	var lenderBalance int64
	err = tx.QueryRow(ctx, "SELECT balance FROM users WHERE user_id = $1 FOR UPDATE", lenderID).Scan(&lenderBalance)
	if err != nil {
		return fmt.Errorf("failed to get lender balance: %w", err)
	}
	
	if lenderBalance < loan.Principal {
		return fmt.Errorf("insufficient balance: need %d, have %d", loan.Principal, lenderBalance)
	}
	
	// Переводим монеты кредитора -> заемщику
	_, err = tx.Exec(ctx, "UPDATE users SET balance = balance - $1 WHERE user_id = $2", loan.Principal, lenderID)
	if err != nil {
		return fmt.Errorf("failed to deduct from lender: %w", err)
	}
	
	_, err = tx.Exec(ctx, "UPDATE users SET balance = balance + $1 WHERE user_id = $2", loan.Principal, loan.BorrowerID)
	if err != nil {
		return fmt.Errorf("failed to credit borrower: %w", err)
	}
	
	// Обновляем статус кредита
	now := time.Now()
	_, err = tx.Exec(ctx, `
		UPDATE p2p_loans 
		SET lender_id = $1, status = 'active', accepted_at = $2
		WHERE id = $3
	`, lenderID, now, loanID)
	if err != nil {
		return fmt.Errorf("failed to update loan status: %w", err)
	}
	
	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, from_id, to_id, amount, meta)
		VALUES('p2p_loan', $1, $2, $3, $4::jsonb)
	`, lenderID, loan.BorrowerID, loan.Principal, fmt.Sprintf(`{
		"loan_id": %d,
		"total_due": %d,
		"collateral_type": "%s"
	}`, loanID, loan.TotalDue, loan.CollateralType))
	if err != nil {
		return fmt.Errorf("failed to record loan: %w", err)
	}
	
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit P2P loan: %w", err)
	}
	
	log.Printf("P2P loan %d accepted: lender %d -> borrower %d, amount %d BKC", 
		loanID, lenderID, loan.BorrowerID, loan.Principal)
	
	return nil
}

// GetBankLoans получает кредиты пользователя
func (cm *CreditsManager) GetBankLoans(ctx context.Context, userID int64) ([]BankLoan, error) {
	rows, err := cm.db.Pool.Query(ctx, `
		SELECT id, user_id, principal, interest_rate, interest_total, total_due,
		       term_days, status, created_at, due_at, closed_at, collector_started_at, daily_collected
		FROM bank_loans 
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bank loans: %w", err)
	}
	defer rows.Close()
	
	var loans []BankLoan
	for rows.Next() {
		var loan BankLoan
		err := rows.Scan(
			&loan.ID, &loan.UserID, &loan.Principal, &loan.InterestRate,
			&loan.InterestTotal, &loan.TotalDue, &loan.TermDays, &loan.Status,
			&loan.CreatedAt, &loan.DueAt, &loan.ClosedAt, &loan.CollectorStartedAt, &loan.DailyCollected,
		)
		if err != nil {
			continue
		}
		loans = append(loans, loan)
	}
	
	return loans, rows.Err()
}

// GetP2PLoans получает P2P кредиты пользователя
func (cm *CreditsManager) GetP2PLoans(ctx context.Context, userID int64, role string) ([]P2PLoan, error) {
	var whereClause string
	if role == "lender" {
		whereClause = "WHERE lender_id = $1"
	} else if role == "borrower" {
		whereClause = "WHERE borrower_id = $1"
	} else {
		whereClause = "WHERE lender_id = $1 OR borrower_id = $1"
	}
	
	rows, err := cm.db.Pool.Query(ctx, fmt.Sprintf(`
		SELECT id, lender_id, borrower_id, principal, interest_rate, interest_total, total_due,
		       collateral_type, collateral_value, collateral_nft_id, term_days, status,
		       created_at, accepted_at, due_at, closed_at, defaulted_at
		FROM p2p_loans 
		%s
		ORDER BY created_at DESC
	`, whereClause), userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get P2P loans: %w", err)
	}
	defer rows.Close()
	
	var loans []P2PLoan
	for rows.Next() {
		var loan P2PLoan
		err := rows.Scan(
			&loan.ID, &loan.LenderID, &loan.BorrowerID, &loan.Principal, &loan.InterestRate,
			&loan.InterestTotal, &loan.TotalDue, &loan.CollateralType, &loan.CollateralValue,
			&loan.CollateralNFTID, &loan.TermDays, &loan.Status, &loan.CreatedAt,
			&loan.AcceptedAt, &loan.DueAt, &loan.ClosedAt, &loan.DefaultedAt,
		)
		if err != nil {
			continue
		}
		loans = append(loans, loan)
	}
	
	return loans, rows.Err()
}

// GetCreditsStats получает статистику кредитов
func (cm *CreditsManager) GetCreditsStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Статистика банковских кредитов
	var activeBankLoans, totalBankLoaned, totalBankDebt int64
	cm.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM bank_loans WHERE status = 'active'").Scan(&activeBankLoans)
	cm.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(principal), 0) FROM bank_loans WHERE status = 'active'").Scan(&totalBankLoaned)
	cm.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(total_due), 0) FROM bank_loans WHERE status = 'active'").Scan(&totalBankDebt)
	
	// Статистика P2P кредитов
	var activeP2PLoans, totalP2PLoaned, totalP2PDebt int64
	cm.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM p2p_loans WHERE status = 'active'").Scan(&activeP2PLoans)
	cm.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(principal), 0) FROM p2p_loans WHERE status = 'active'").Scan(&totalP2PLoaned)
	cm.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(total_due), 0) FROM p2p_loans WHERE status = 'active'").Scan(&totalP2PDebt)
	
	// Статистика коллекторов
	var collectorModeUsers int64
	cm.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE collector_mode = true").Scan(&collectorModeUsers)
	
	stats["active_bank_loans"] = activeBankLoans
	stats["total_bank_loaned"] = totalBankLoaned
	stats["total_bank_debt"] = totalBankDebt
	stats["active_p2p_loans"] = activeP2PLoans
	stats["total_p2p_loaned"] = totalP2PLoaned
	stats["total_p2p_debt"] = totalP2PDebt
	stats["collector_mode_users"] = collectorModeUsers
	stats["total_active_loans"] = activeBankLoans + activeP2PLoans
	stats["total_loaned"] = totalBankLoaned + totalP2PLoaned
	stats["total_debt"] = totalBankDebt + totalP2PDebt
	
	return stats, nil
}
