package db

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

type SystemState struct {
	TotalSupply       int64
	ReserveSupply     int64
	ReservedSupply    int64
	InitialReserve    int64
	AdminUserID       int64
	AdminAllocated    int64
	StartRateCoinsUSD int64
	MinRateCoinsUSD   int64
	ReferralStep      int64
	ReferralBonus     int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type UserState struct {
	UserID                     int64
	Username                   string
	FirstName                  string
	Balance                    int64
	FrozenBalance              int64
	TapsTotal                  int64
	Energy                     float64
	EnergyMax                  float64
	EnergyUpdatedAt            time.Time
	EnergyBoostUntil           time.Time
	EnergyBoostRegenMultiplier float64
	EnergyBoostMaxMultiplier   float64
	ReferralsCount             int64
	ReferralBonusTotal         int64
}

type NFT struct {
	NFTID      int64     `json:"nft_id"`
	Title      string    `json:"title"`
	ImageURL   string    `json:"image_url"`
	PriceCoins int64     `json:"price_coins"`
	SupplyLeft int64     `json:"supply_left"`
	CreatedAt  time.Time `json:"created_at"`
}

type UserNFT struct {
	NFTID    int64  `json:"nft_id"`
	Title    string `json:"title"`
	ImageURL string `json:"image_url"`
	Qty      int64  `json:"qty"`
}

type CryptoPayInvoice struct {
	InvoiceID  int64      `json:"invoice_id"`
	UserID     int64      `json:"user_id"`
	AmountUSD  int64      `json:"amount_usd"`
	Coins      int64      `json:"coins"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	PaidAt     *time.Time `json:"paid_at"`
	CreditedAt *time.Time `json:"credited_at"`
	ReleasedAt *time.Time `json:"released_at"`
}

type Deposit struct {
	DepositID  int64      `json:"deposit_id"`
	UserID     int64      `json:"user_id"`
	TxHash     string     `json:"tx_hash"`
	AmountUSD  int64      `json:"amount_usd"`
	Currency   string     `json:"currency"`
	Coins      int64      `json:"coins"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ApprovedAt *time.Time `json:"approved_at"`
	ApprovedBy *int64     `json:"approved_by"`
}

type BankLoan struct {
	LoanID    int64      `json:"loan_id"`
	UserID    int64      `json:"user_id"`
	Principal int64      `json:"principal"`
	Interest  int64      `json:"interest"`
	TotalDue  int64      `json:"total_due"`
	TermDays  int64      `json:"term_days"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	DueAt     time.Time  `json:"due_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

type P2PLoan struct {
	LoanID     int64      `json:"loan_id"`
	LenderID   int64      `json:"lender_id"`
	BorrowerID int64      `json:"borrower_id"`
	Principal  int64      `json:"principal"`
	Interest   int64      `json:"interest"`
	TotalDue   int64      `json:"total_due"`
	InterestBP int64      `json:"interest_bp"`
	TermDays   int64      `json:"term_days"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	AcceptedAt *time.Time `json:"accepted_at"`
	DueAt      *time.Time `json:"due_at"`
	ClosedAt   *time.Time `json:"closed_at"`
}

type MarketListing struct {
	ListingID   int64      `json:"listing_id"`
	SellerID    int64      `json:"seller_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Category    string     `json:"category"`
	PriceCoins  int64      `json:"price_coins"`
	Contact     string     `json:"contact"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	SoldAt      *time.Time `json:"sold_at"`
	BuyerID     *int64     `json:"buyer_id"`
	ImageID     *int64     `json:"image_id,omitempty"`
}

type MarketListingImage struct {
	ImageID   int64     `json:"image_id"`
	ListingID int64     `json:"listing_id"`
	Mime      string    `json:"mime"`
	Data      []byte    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type UserDaily struct {
	UserID     int64
	Day        time.Time
	Tapped     int64
	ExtraQuota int64
}

// TapEvent is produced by the Redis fasttap queue and persisted into Postgres by a background worker.
// Day is formatted as YYYY-MM-DD in UTC.
type TapEvent struct {
	EventID string
	UserID  int64
	Coins   int64
	Taps    int64
	Day     string
	Req     int64
}

// UserTapAggregate is an in-memory tap flush batch item.
// BalanceDelta/TapsDelta are additive deltas, while Energy/EnergyUpdatedAt
// represent the latest absolute energy state for the user.
type UserTapAggregate struct {
	UserID          int64
	BalanceDelta    int64
	TapsDelta       int64
	Energy          float64
	EnergyUpdatedAt time.Time
}

// DailyTapAggregate is an in-memory daily tap counter flush item.
type DailyTapAggregate struct {
	UserID      int64
	Day         string // YYYY-MM-DD (UTC)
	TappedDelta int64
}

func Connect(ctx context.Context, databaseURL string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 5
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &DB{Pool: pool}, nil
}

func (d *DB) Close() {
	if d.Pool != nil {
		d.Pool.Close()
	}
}

func (d *DB) Migrate(ctx context.Context) error {
	sql := `
CREATE TABLE IF NOT EXISTS system_state (
  id INT PRIMARY KEY DEFAULT 1,
  total_supply BIGINT NOT NULL,
  reserve_supply BIGINT NOT NULL,
  reserved_supply BIGINT NOT NULL DEFAULT 0,
  initial_reserve BIGINT NOT NULL,
  admin_user_id BIGINT NOT NULL,
  admin_allocated BIGINT NOT NULL,
  start_rate_coins_usd BIGINT NOT NULL,
  min_rate_coins_usd BIGINT NOT NULL,
  referral_step BIGINT NOT NULL,
  referral_bonus BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE system_state ADD COLUMN IF NOT EXISTS reserved_supply BIGINT NOT NULL DEFAULT 0;

	CREATE TABLE IF NOT EXISTS users (
	  user_id BIGINT PRIMARY KEY,
	  username TEXT,
	  first_name TEXT,
	  balance BIGINT NOT NULL DEFAULT 0,
	  frozen_balance BIGINT NOT NULL DEFAULT 0,
	  taps_total BIGINT NOT NULL DEFAULT 0,
	  energy DOUBLE PRECISION NOT NULL DEFAULT 0,
	  energy_max DOUBLE PRECISION NOT NULL DEFAULT 0,
	  energy_updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	  energy_boost_until TIMESTAMPTZ,
  energy_boost_regen_multiplier DOUBLE PRECISION NOT NULL DEFAULT 1,
  energy_boost_max_multiplier DOUBLE PRECISION NOT NULL DEFAULT 1,
  referrals_count BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

	ALTER TABLE users ADD COLUMN IF NOT EXISTS energy_boost_until TIMESTAMPTZ;
	ALTER TABLE users ADD COLUMN IF NOT EXISTS energy_boost_regen_multiplier DOUBLE PRECISION NOT NULL DEFAULT 1;
	ALTER TABLE users ADD COLUMN IF NOT EXISTS energy_boost_max_multiplier DOUBLE PRECISION NOT NULL DEFAULT 1;
	ALTER TABLE users ADD COLUMN IF NOT EXISTS taps_total BIGINT NOT NULL DEFAULT 0;
	ALTER TABLE users ADD COLUMN IF NOT EXISTS frozen_balance BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS referrals (
  id BIGSERIAL PRIMARY KEY,
  referrer_id BIGINT NOT NULL,
  referred_id BIGINT NOT NULL UNIQUE,
  bonus BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ledger (
  id BIGSERIAL PRIMARY KEY,
  event_id TEXT,
  ts TIMESTAMPTZ NOT NULL DEFAULT now(),
  kind TEXT NOT NULL,
  from_id BIGINT,
  to_id BIGINT,
  amount BIGINT NOT NULL,
  meta JSONB NOT NULL DEFAULT '{}'::jsonb
);

ALTER TABLE ledger ADD COLUMN IF NOT EXISTS event_id TEXT;

CREATE INDEX IF NOT EXISTS ledger_ts_idx ON ledger(ts DESC);
CREATE INDEX IF NOT EXISTS ledger_to_idx ON ledger(to_id);
CREATE INDEX IF NOT EXISTS ledger_from_idx ON ledger(from_id);
CREATE UNIQUE INDEX IF NOT EXISTS ledger_event_id_uniq ON ledger(event_id);

CREATE TABLE IF NOT EXISTS cryptopay_invoices (
  invoice_id BIGINT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  amount_usd BIGINT NOT NULL,
  coins BIGINT NOT NULL,
  status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  paid_at TIMESTAMPTZ,
  credited_at TIMESTAMPTZ,
  released_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS cryptopay_invoices_user_idx ON cryptopay_invoices(user_id);

CREATE TABLE IF NOT EXISTS deposits (
  deposit_id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  tx_hash TEXT NOT NULL,
  amount_usd BIGINT NOT NULL,
  currency TEXT NOT NULL,
  coins BIGINT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  approved_at TIMESTAMPTZ,
  approved_by BIGINT
);

CREATE INDEX IF NOT EXISTS deposits_status_idx ON deposits(status, created_at DESC);

CREATE TABLE IF NOT EXISTS nfts (
  nft_id BIGSERIAL PRIMARY KEY,
  title TEXT NOT NULL,
  image_url TEXT NOT NULL,
  price_coins BIGINT NOT NULL,
  supply_total BIGINT NOT NULL,
  supply_left BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS nft_owns (
  user_id BIGINT NOT NULL,
  nft_id BIGINT NOT NULL,
  qty BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, nft_id)
);

-- Bank loans (reserve -> user)
CREATE TABLE IF NOT EXISTS bank_loans (
  loan_id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  principal BIGINT NOT NULL,
  interest BIGINT NOT NULL,
  total_due BIGINT NOT NULL,
  term_days INT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active', -- active|repaid|overdue
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  due_at TIMESTAMPTZ NOT NULL,
  closed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS bank_loans_user_idx ON bank_loans(user_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS bank_loans_status_due_idx ON bank_loans(status, due_at);

-- P2P loans (user -> user)
CREATE TABLE IF NOT EXISTS p2p_loans (
  loan_id BIGSERIAL PRIMARY KEY,
  lender_id BIGINT NOT NULL,
  borrower_id BIGINT NOT NULL,
  principal BIGINT NOT NULL,
  interest BIGINT NOT NULL,
  total_due BIGINT NOT NULL,
  interest_bp INT NOT NULL,
  term_days INT NOT NULL,
  status TEXT NOT NULL DEFAULT 'requested', -- requested|active|rejected|cancelled|repaid
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  accepted_at TIMESTAMPTZ,
  due_at TIMESTAMPTZ,
  closed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS p2p_loans_lender_idx ON p2p_loans(lender_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS p2p_loans_borrower_idx ON p2p_loans(borrower_id, status, created_at DESC);

-- Marketplace (bazaar)
CREATE TABLE IF NOT EXISTS market_listings (
  listing_id BIGSERIAL PRIMARY KEY,
  seller_id BIGINT NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL,
  category TEXT NOT NULL DEFAULT 'other',
  price_coins BIGINT NOT NULL,
  contact TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active', -- active|sold|cancelled
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  sold_at TIMESTAMPTZ,
  buyer_id BIGINT
);
CREATE INDEX IF NOT EXISTS market_listings_status_idx ON market_listings(status, created_at DESC);
CREATE INDEX IF NOT EXISTS market_listings_seller_idx ON market_listings(seller_id, created_at DESC);

CREATE TABLE IF NOT EXISTS market_listing_images (
  image_id BIGSERIAL PRIMARY KEY,
  listing_id BIGINT NOT NULL REFERENCES market_listings(listing_id) ON DELETE CASCADE,
  mime TEXT NOT NULL,
  data BYTEA NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS market_listing_images_listing_idx ON market_listing_images(listing_id, created_at DESC);

-- Daily tap limits / quotas
CREATE TABLE IF NOT EXISTS user_daily (
  user_id BIGINT NOT NULL,
  day DATE NOT NULL,
  tapped BIGINT NOT NULL DEFAULT 0,
  extra_quota BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, day)
);
CREATE INDEX IF NOT EXISTS user_daily_day_idx ON user_daily(day, tapped DESC);

-- Deposit wallets (manual top-up instructions)
CREATE TABLE IF NOT EXISTS deposit_wallets (
  currency TEXT PRIMARY KEY,
  address TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`
	_, err := d.Pool.Exec(ctx, sql)
	return err
}

func (d *DB) EnsureSystemState(ctx context.Context, totalSupply, adminUserID, adminAllocated, reserveSupply, startRate, minRate, refStep, refBonus int64) (SystemState, error) {
	_, err := d.Pool.Exec(ctx, `
INSERT INTO system_state (id, total_supply, reserve_supply, reserved_supply, initial_reserve, admin_user_id, admin_allocated, start_rate_coins_usd, min_rate_coins_usd, referral_step, referral_bonus)
VALUES (1, $1, $2, 0, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO NOTHING
`, totalSupply, reserveSupply, adminUserID, adminAllocated, startRate, minRate, refStep, refBonus)
	if err != nil {
		return SystemState{}, err
	}
	return d.GetSystem(ctx)
}

func (d *DB) EnsureUser(ctx context.Context, userID int64, username, firstName string, energyMax float64) (UserState, error) {
	_, err := d.Pool.Exec(ctx, `
INSERT INTO users (user_id, username, first_name, balance, taps_total, energy, energy_max)
VALUES ($1, $2, $3, 0, 0, $4, $4)
ON CONFLICT (user_id) DO UPDATE SET
  username = EXCLUDED.username,
  first_name = EXCLUDED.first_name
`, userID, username, firstName, energyMax)
	if err != nil {
		return UserState{}, err
	}
	return d.GetUser(ctx, userID)
}

func (d *DB) GetUser(ctx context.Context, userID int64) (UserState, error) {
	var u UserState
	row := d.Pool.QueryRow(ctx, `
SELECT user_id, COALESCE(username,''), COALESCE(first_name,''), balance, frozen_balance, taps_total, energy, energy_max, energy_updated_at,
       COALESCE(energy_boost_until, to_timestamp(0)), energy_boost_regen_multiplier, energy_boost_max_multiplier,
       referrals_count
	FROM users
	WHERE user_id=$1
	`, userID)
	if err := row.Scan(
		&u.UserID, &u.Username, &u.FirstName, &u.Balance, &u.FrozenBalance, &u.TapsTotal, &u.Energy, &u.EnergyMax, &u.EnergyUpdatedAt,
		&u.EnergyBoostUntil, &u.EnergyBoostRegenMultiplier, &u.EnergyBoostMaxMultiplier,
		&u.ReferralsCount,
	); err != nil {
		return UserState{}, err
	}
	row2 := d.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(bonus),0) FROM referrals WHERE referrer_id=$1`, userID)
	_ = row2.Scan(&u.ReferralBonusTotal)
	return u, nil
}

func (d *DB) GetSystem(ctx context.Context) (SystemState, error) {
	var s SystemState
	row := d.Pool.QueryRow(ctx, `
SELECT total_supply, reserve_supply, reserved_supply, initial_reserve, admin_user_id, admin_allocated, start_rate_coins_usd, min_rate_coins_usd, referral_step, referral_bonus, created_at, updated_at
FROM system_state
WHERE id=1
`)
	if err := row.Scan(&s.TotalSupply, &s.ReserveSupply, &s.ReservedSupply, &s.InitialReserve, &s.AdminUserID, &s.AdminAllocated, &s.StartRateCoinsUSD, &s.MinRateCoinsUSD, &s.ReferralStep, &s.ReferralBonus, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return SystemState{}, err
	}
	return s, nil
}

func (d *DB) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := d.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var ErrNotEnough = errors.New("not enough")
var ErrAlreadyExists = errors.New("already exists")
var ErrForbidden = errors.New("forbidden")

func (d *DB) ApplyTapEvents(ctx context.Context, events []TapEvent) error {
	if len(events) == 0 {
		return nil
	}
	ids := make([]string, 0, len(events))
	uids := make([]int64, 0, len(events))
	coins := make([]int64, 0, len(events))
	taps := make([]int64, 0, len(events))
	days := make([]string, 0, len(events))
	reqs := make([]int64, 0, len(events))
	for _, ev := range events {
		if strings.TrimSpace(ev.EventID) == "" || ev.UserID <= 0 || ev.Coins <= 0 || ev.Taps <= 0 || strings.TrimSpace(ev.Day) == "" {
			continue
		}
		ids = append(ids, strings.TrimSpace(ev.EventID))
		uids = append(uids, ev.UserID)
		coins = append(coins, ev.Coins)
		taps = append(taps, ev.Taps)
		days = append(days, strings.TrimSpace(ev.Day))
		reqs = append(reqs, ev.Req)
	}
	if len(ids) == 0 {
		return nil
	}

	return d.WithTx(ctx, func(tx pgx.Tx) error {
		// Insert into ledger with idempotency (event_id unique).
		// Then apply aggregates to users + daily counters + reserve.
		_, err := tx.Exec(ctx, `
WITH data AS (
  SELECT * FROM UNNEST($1::text[], $2::bigint[], $3::bigint[], $4::bigint[], $5::text[], $6::bigint[])
  AS t(event_id, user_id, coins, taps, day, req)
),
ins AS (
  INSERT INTO ledger(event_id, kind, from_id, to_id, amount, meta)
  SELECT event_id, 'tap', NULL, user_id, coins,
         jsonb_build_object('taps', taps, 'req', req, 'day', day)
  FROM data
  ON CONFLICT (event_id) DO NOTHING
  RETURNING to_id AS user_id, amount AS coins, (meta->>'taps')::bigint AS taps, (meta->>'day')::date AS day
),
agg_user AS (
  SELECT user_id, SUM(coins) AS coins, SUM(taps) AS taps
  FROM ins
  GROUP BY user_id
),
agg_day AS (
  SELECT user_id, day, SUM(taps) AS taps
  FROM ins
  GROUP BY user_id, day
),
up_user AS (
  UPDATE users
  SET balance = users.balance + agg_user.coins,
      taps_total = users.taps_total + agg_user.taps
  FROM agg_user
  WHERE users.user_id = agg_user.user_id
  RETURNING 1
),
up_daily AS (
  INSERT INTO user_daily(user_id, day, tapped)
  SELECT user_id, day, taps
  FROM agg_day
  ON CONFLICT (user_id, day) DO UPDATE
    SET tapped = user_daily.tapped + EXCLUDED.tapped,
        updated_at = now()
  RETURNING 1
)
UPDATE system_state
SET reserve_supply = reserve_supply - (SELECT COALESCE(SUM(coins),0) FROM ins),
    updated_at = now()
WHERE id=1
`, ids, uids, coins, taps, days, reqs)
		return err
	})
}

// ApplyTapAggregates persists in-memory tap deltas in one transactional batch.
// reserveDelta should be negative for tap mints (reserve decreases).
func (d *DB) ApplyTapAggregates(ctx context.Context, users []UserTapAggregate, daily []DailyTapAggregate, reserveDelta int64, source string) error {
	if len(users) == 0 && len(daily) == 0 && reserveDelta == 0 {
		return nil
	}

	userIDs := make([]int64, 0, len(users))
	userBalance := make([]int64, 0, len(users))
	userTaps := make([]int64, 0, len(users))
	userEnergy := make([]float64, 0, len(users))
	userEnergyAt := make([]time.Time, 0, len(users))
	var totalCoins int64

	for _, u := range users {
		if u.UserID <= 0 {
			continue
		}
		if u.EnergyUpdatedAt.IsZero() {
			u.EnergyUpdatedAt = time.Now().UTC()
		}
		userIDs = append(userIDs, u.UserID)
		userBalance = append(userBalance, u.BalanceDelta)
		userTaps = append(userTaps, u.TapsDelta)
		userEnergy = append(userEnergy, u.Energy)
		userEnergyAt = append(userEnergyAt, u.EnergyUpdatedAt.UTC())
		if u.BalanceDelta > 0 {
			totalCoins += u.BalanceDelta
		}
	}

	dailyUserIDs := make([]int64, 0, len(daily))
	dailyDays := make([]string, 0, len(daily))
	dailyTapped := make([]int64, 0, len(daily))
	for _, dly := range daily {
		if dly.UserID <= 0 || strings.TrimSpace(dly.Day) == "" || dly.TappedDelta == 0 {
			continue
		}
		dailyUserIDs = append(dailyUserIDs, dly.UserID)
		dailyDays = append(dailyDays, strings.TrimSpace(dly.Day))
		dailyTapped = append(dailyTapped, dly.TappedDelta)
	}

	return d.WithTx(ctx, func(tx pgx.Tx) error {
		if len(userIDs) > 0 {
			_, err := tx.Exec(ctx, `
WITH data AS (
  SELECT * FROM UNNEST($1::bigint[], $2::bigint[], $3::bigint[], $4::double precision[], $5::timestamptz[])
  AS t(user_id, balance_delta, taps_delta, energy, energy_updated_at)
)
UPDATE users
SET balance = users.balance + data.balance_delta,
    taps_total = users.taps_total + data.taps_delta,
    energy = data.energy,
    energy_updated_at = data.energy_updated_at
FROM data
WHERE users.user_id = data.user_id
`, userIDs, userBalance, userTaps, userEnergy, userEnergyAt)
			if err != nil {
				return err
			}
		}

		if len(dailyUserIDs) > 0 {
			_, err := tx.Exec(ctx, `
WITH data AS (
  SELECT * FROM UNNEST($1::bigint[], $2::text[], $3::bigint[])
  AS t(user_id, day, tapped_delta)
)
INSERT INTO user_daily(user_id, day, tapped)
SELECT user_id, day::date, tapped_delta
FROM data
ON CONFLICT (user_id, day) DO UPDATE
SET tapped = user_daily.tapped + EXCLUDED.tapped,
    updated_at = now()
`, dailyUserIDs, dailyDays, dailyTapped)
			if err != nil {
				return err
			}
		}

		if reserveDelta != 0 {
			_, err := tx.Exec(ctx, `
UPDATE system_state
SET reserve_supply = GREATEST(reserve_supply + $1, 0),
    updated_at = now()
WHERE id=1
`, reserveDelta)
			if err != nil {
				return err
			}
		}

		if totalCoins > 0 {
			meta := toJSON(map[string]any{
				"source": source,
				"users":  len(userIDs),
				"daily":  len(dailyUserIDs),
			})
			_, err := tx.Exec(ctx, `
INSERT INTO ledger(kind, from_id, to_id, amount, meta)
VALUES('tap_flush_batch', NULL, NULL, $1, $2::jsonb)
`, totalCoins, meta)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (d *DB) CreditFromReserve(ctx context.Context, userID int64, amount int64, kind string, meta any) error {
	if amount <= 0 {
		return nil
	}
	return d.WithTx(ctx, func(tx pgx.Tx) error {
		var reserve int64
		var reserved int64
		if err := tx.QueryRow(ctx, `SELECT reserve_supply, reserved_supply FROM system_state WHERE id=1 FOR UPDATE`).Scan(&reserve, &reserved); err != nil {
			return err
		}
		available := reserve - reserved
		if available < amount {
			return ErrNotEnough
		}
		if _, err := tx.Exec(ctx, `UPDATE system_state SET reserve_supply = reserve_supply - $1, updated_at=now() WHERE id=1`, amount); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET balance = balance + $1 WHERE user_id=$2`, amount, userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES($1, NULL, $2, $3, $4::jsonb)`, kind, userID, amount, toJSON(meta))
		return err
	})
}

func (d *DB) DebitToReserve(ctx context.Context, userID int64, amount int64, kind string, meta any) error {
	if amount <= 0 {
		return nil
	}
	return d.WithTx(ctx, func(tx pgx.Tx) error {
		var bal int64
		if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE user_id=$1 FOR UPDATE`, userID).Scan(&bal); err != nil {
			return err
		}
		if bal < amount {
			return ErrNotEnough
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET balance = balance - $1 WHERE user_id=$2`, amount, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE system_state SET reserve_supply = reserve_supply + $1, updated_at=now() WHERE id=1`, amount); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES($1, $2, NULL, $3, $4::jsonb)`, kind, userID, amount, toJSON(meta))
		return err
	})
}

func (d *DB) Transfer(ctx context.Context, fromID, toID, amount int64) error {
	if amount <= 0 || fromID == toID {
		return nil
	}
	return d.WithTx(ctx, func(tx pgx.Tx) error {
		var fromBal int64
		if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE user_id=$1 FOR UPDATE`, fromID).Scan(&fromBal); err != nil {
			return err
		}
		if fromBal < amount {
			return ErrNotEnough
		}
		// Ensure receiver exists and lock
		var toBal int64
		if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE user_id=$1 FOR UPDATE`, toID).Scan(&toBal); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET balance = balance - $1 WHERE user_id=$2`, amount, fromID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET balance = balance + $1 WHERE user_id=$2`, amount, toID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount) VALUES('transfer', $1, $2, $3)`, fromID, toID, amount)
		return err
	})
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

func (d *DB) WasReferred(ctx context.Context, referredID int64) (bool, error) {
	var ok bool
	err := d.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM referrals WHERE referred_id=$1)`, referredID).Scan(&ok)
	return ok, err
}

// RegisterReferral increments referrer count once per referred user.
// Returns bonus credited to referrer (0 or referralBonus when milestone reached).
func (d *DB) RegisterReferral(ctx context.Context, referrerID, referredID, step, referralBonus int64) (int64, error) {
	if referrerID == 0 || referredID == 0 || referrerID == referredID {
		return 0, nil
	}
	if step <= 0 || referralBonus < 0 {
		return 0, nil
	}

	var bonus int64
	err := d.WithTx(ctx, func(tx pgx.Tx) error {
		// If referred already registered, skip.
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM referrals WHERE referred_id=$1)`, referredID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil
		}

		// Lock system + referrer rows
		var reserve int64
		var reserved int64
		if err := tx.QueryRow(ctx, `SELECT reserve_supply, reserved_supply FROM system_state WHERE id=1 FOR UPDATE`).Scan(&reserve, &reserved); err != nil {
			return err
		}
		available := reserve - reserved

		var refCount int64
		var refBal int64
		if err := tx.QueryRow(ctx, `SELECT referrals_count, balance FROM users WHERE user_id=$1 FOR UPDATE`, referrerID).Scan(&refCount, &refBal); err != nil {
			return err
		}

		nextCount := refCount + 1
		if nextCount%step == 0 && available >= referralBonus {
			bonus = referralBonus
		} else {
			bonus = 0
		}

		_, err := tx.Exec(ctx, `INSERT INTO referrals(referrer_id, referred_id, bonus) VALUES($1, $2, $3)`, referrerID, referredID, bonus)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `UPDATE users SET referrals_count = referrals_count + 1 WHERE user_id=$1`, referrerID)
		if err != nil {
			return err
		}

		if bonus > 0 {
			_, err = tx.Exec(ctx, `UPDATE system_state SET reserve_supply = reserve_supply - $1, updated_at=now() WHERE id=1`, bonus)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE users SET balance = balance + $1 WHERE user_id=$2`, bonus, referrerID)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('ref_bonus', NULL, $1, $2, $3::jsonb)`, referrerID, bonus, toJSON(map[string]any{
				"step":  step,
				"count": nextCount,
			}))
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return 0, err
	}
	return bonus, nil
}

func (d *DB) ListUserIDs(ctx context.Context) ([]int64, error) {
	rows, err := d.Pool.Query(ctx, `SELECT user_id FROM users ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (d *DB) CreateCryptoPayInvoice(ctx context.Context, invoiceID, userID, amountUSD, coins int64, status string) error {
	if invoiceID <= 0 || userID <= 0 || amountUSD <= 0 || coins <= 0 {
		return errors.New("bad params")
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = "active"
	}

	return d.WithTx(ctx, func(tx pgx.Tx) error {
		var reserve int64
		var reserved int64
		if err := tx.QueryRow(ctx, `SELECT reserve_supply, reserved_supply FROM system_state WHERE id=1 FOR UPDATE`).Scan(&reserve, &reserved); err != nil {
			return err
		}
		available := reserve - reserved
		if available < coins {
			return ErrNotEnough
		}

		// Insert invoice row once; reserve coins only if insertion succeeded.
		var inserted int
		err := tx.QueryRow(ctx, `
INSERT INTO cryptopay_invoices(invoice_id, user_id, amount_usd, coins, status)
VALUES($1, $2, $3, $4, $5)
ON CONFLICT (invoice_id) DO NOTHING
RETURNING 1
`, invoiceID, userID, amountUSD, coins, status).Scan(&inserted)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Already exists, nothing to do.
				return nil
			}
			return err
		}

		if _, err := tx.Exec(ctx, `UPDATE system_state SET reserved_supply = reserved_supply + $1, updated_at=now() WHERE id=1`, coins); err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('cryptopay_invoice', $1, NULL, 0, $2::jsonb)`,
			userID,
			toJSON(map[string]any{"invoice_id": invoiceID, "usd": amountUSD, "coins": coins, "status": status}),
		)
		return err
	})
}

func (d *DB) GetCryptoPayInvoice(ctx context.Context, invoiceID int64) (CryptoPayInvoice, error) {
	var inv CryptoPayInvoice
	row := d.Pool.QueryRow(ctx, `
SELECT invoice_id, user_id, amount_usd, coins, status, created_at, paid_at, credited_at, released_at
FROM cryptopay_invoices
WHERE invoice_id=$1
`, invoiceID)
	if err := row.Scan(&inv.InvoiceID, &inv.UserID, &inv.AmountUSD, &inv.Coins, &inv.Status, &inv.CreatedAt, &inv.PaidAt, &inv.CreditedAt, &inv.ReleasedAt); err != nil {
		return CryptoPayInvoice{}, err
	}
	return inv, nil
}

// ProcessCryptoPayStatus updates invoice status and, if needed, credits or releases reserved coins.
// Returns credited coins (0 if none) and final status.
func (d *DB) ProcessCryptoPayStatus(ctx context.Context, invoiceID int64, newStatus string, paidAt time.Time) (int64, string, error) {
	if invoiceID <= 0 {
		return 0, "", errors.New("bad invoice_id")
	}
	newStatus = strings.ToLower(strings.TrimSpace(newStatus))
	if newStatus == "" {
		return 0, "", errors.New("missing status")
	}

	var credited int64
	var finalStatus string

	err := d.WithTx(ctx, func(tx pgx.Tx) error {
		var userID int64
		var coins int64
		var status string
		var paid *time.Time
		var cred *time.Time
		var rel *time.Time

		err := tx.QueryRow(ctx, `
SELECT user_id, coins, status, paid_at, credited_at, released_at
FROM cryptopay_invoices
WHERE invoice_id=$1
FOR UPDATE
`, invoiceID).Scan(&userID, &coins, &status, &paid, &cred, &rel)
		if err != nil {
			// If invoice unknown, ignore (webhook for other projects).
			if errors.Is(err, pgx.ErrNoRows) {
				finalStatus = "unknown"
				return nil
			}
			return err
		}

		finalStatus = newStatus

		// Always update status, but keep timestamps idempotent.
		if _, err := tx.Exec(ctx, `UPDATE cryptopay_invoices SET status=$1 WHERE invoice_id=$2`, newStatus, invoiceID); err != nil {
			return err
		}

		isPaid := newStatus == "paid"
		isExpired := newStatus == "expired" || newStatus == "canceled" || newStatus == "cancelled"

		if isPaid && paid == nil {
			_, _ = tx.Exec(ctx, `UPDATE cryptopay_invoices SET paid_at=$1 WHERE invoice_id=$2`, paidAt, invoiceID)
		}

		// Credit once.
		if isPaid && cred == nil {
			// Lock system to move coins out of reserve and out of reserved.
			var reserve int64
			var reserved int64
			if err := tx.QueryRow(ctx, `SELECT reserve_supply, reserved_supply FROM system_state WHERE id=1 FOR UPDATE`).Scan(&reserve, &reserved); err != nil {
				return err
			}
			if reserved < coins {
				return errors.New("reserved underflow")
			}
			if reserve < coins {
				return ErrNotEnough
			}
			if _, err := tx.Exec(ctx, `UPDATE system_state SET reserve_supply = reserve_supply - $1, reserved_supply = reserved_supply - $1, updated_at=now() WHERE id=1`, coins); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE users SET balance = balance + $1 WHERE user_id=$2`, coins, userID); err != nil {
				return err
			}
			now := time.Now().UTC()
			if _, err := tx.Exec(ctx, `UPDATE cryptopay_invoices SET credited_at=$1 WHERE invoice_id=$2`, now, invoiceID); err != nil {
				return err
			}
			credited = coins
			_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('cryptopay_deposit', NULL, $1, $2, $3::jsonb)`,
				userID, coins, toJSON(map[string]any{"invoice_id": invoiceID}),
			)
			return err
		}

		// Release reservation once on expiry/cancel if not credited.
		if isExpired && cred == nil && rel == nil {
			var reserved int64
			if err := tx.QueryRow(ctx, `SELECT reserved_supply FROM system_state WHERE id=1 FOR UPDATE`).Scan(&reserved); err != nil {
				return err
			}
			if reserved < coins {
				// Should not happen, but don't make it worse.
				coins = reserved
			}
			if coins > 0 {
				if _, err := tx.Exec(ctx, `UPDATE system_state SET reserved_supply = reserved_supply - $1, updated_at=now() WHERE id=1`, coins); err != nil {
					return err
				}
			}
			now := time.Now().UTC()
			if _, err := tx.Exec(ctx, `UPDATE cryptopay_invoices SET released_at=$1 WHERE invoice_id=$2`, now, invoiceID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('cryptopay_release', $1, NULL, 0, $2::jsonb)`,
				userID, toJSON(map[string]any{"invoice_id": invoiceID, "status": newStatus}),
			)
			return err
		}

		return nil
	})
	if err != nil {
		return 0, "", err
	}
	return credited, finalStatus, nil
}

func (d *DB) ListNFTs(ctx context.Context) ([]NFT, error) {
	rows, err := d.Pool.Query(ctx, `
SELECT nft_id, title, image_url, price_coins, supply_left, created_at
FROM nfts
ORDER BY nft_id DESC
LIMIT 200
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []NFT
	for rows.Next() {
		var n NFT
		if err := rows.Scan(&n.NFTID, &n.Title, &n.ImageURL, &n.PriceCoins, &n.SupplyLeft, &n.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	return items, rows.Err()
}

func (d *DB) ListUserNFTs(ctx context.Context, userID int64) ([]UserNFT, error) {
	rows, err := d.Pool.Query(ctx, `
SELECT o.nft_id, n.title, n.image_url, o.qty
FROM nft_owns o
JOIN nfts n ON n.nft_id = o.nft_id
WHERE o.user_id=$1 AND o.qty > 0
ORDER BY o.qty DESC, o.nft_id DESC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserNFT
	for rows.Next() {
		var u UserNFT
		if err := rows.Scan(&u.NFTID, &u.Title, &u.ImageURL, &u.Qty); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (d *DB) CreateNFT(ctx context.Context, title, imageURL string, priceCoins, supply int64) (int64, error) {
	title = strings.TrimSpace(title)
	imageURL = strings.TrimSpace(imageURL)
	if title == "" || imageURL == "" || priceCoins <= 0 || supply <= 0 {
		return 0, errors.New("bad params")
	}

	var id int64
	err := d.Pool.QueryRow(ctx, `
INSERT INTO nfts(title, image_url, price_coins, supply_total, supply_left)
VALUES($1, $2, $3, $4, $4)
RETURNING nft_id
`, title, imageURL, priceCoins, supply).Scan(&id)
	if err != nil {
		return 0, err
	}
	_, _ = d.Pool.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('nft_create', NULL, NULL, 0, $1::jsonb)`,
		toJSON(map[string]any{"nft_id": id, "title": title, "price": priceCoins, "supply": supply}),
	)
	return id, nil
}

func (d *DB) BuyNFT(ctx context.Context, buyerID, nftID int64) error {
	if buyerID <= 0 || nftID <= 0 {
		return errors.New("bad params")
	}

	return d.WithTx(ctx, func(tx pgx.Tx) error {
		var price int64
		var left int64
		if err := tx.QueryRow(ctx, `SELECT price_coins, supply_left FROM nfts WHERE nft_id=$1 FOR UPDATE`, nftID).Scan(&price, &left); err != nil {
			return err
		}
		if left <= 0 {
			return ErrNotEnough
		}

		var bal int64
		if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE user_id=$1 FOR UPDATE`, buyerID).Scan(&bal); err != nil {
			return err
		}
		if bal < price {
			return ErrNotEnough
		}

		// Debit buyer -> reserve
		if _, err := tx.Exec(ctx, `UPDATE users SET balance = balance - $1 WHERE user_id=$2`, price, buyerID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE system_state SET reserve_supply = reserve_supply + $1, updated_at=now() WHERE id=1`, price); err != nil {
			return err
		}

		// Decrement supply
		if _, err := tx.Exec(ctx, `UPDATE nfts SET supply_left = supply_left - 1 WHERE nft_id=$1`, nftID); err != nil {
			return err
		}

		// Upsert ownership
		if _, err := tx.Exec(ctx, `
INSERT INTO nft_owns(user_id, nft_id, qty) VALUES($1, $2, 1)
ON CONFLICT (user_id, nft_id) DO UPDATE SET qty = nft_owns.qty + 1
`, buyerID, nftID); err != nil {
			return err
		}

		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('nft_buy', $1, NULL, $2, $3::jsonb)`,
			buyerID, price, toJSON(map[string]any{"nft_id": nftID}),
		)
		return err
	})
}

func (d *DB) CreateDeposit(ctx context.Context, userID int64, txHash string, amountUSD int64, currency string, coins int64) (int64, error) {
	txHash = strings.TrimSpace(txHash)
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if userID <= 0 || txHash == "" || amountUSD <= 0 || coins <= 0 || currency == "" {
		return 0, errors.New("bad params")
	}

	var id int64
	err := d.WithTx(ctx, func(tx pgx.Tx) error {
		var reserve int64
		var reserved int64
		if err := tx.QueryRow(ctx, `SELECT reserve_supply, reserved_supply FROM system_state WHERE id=1 FOR UPDATE`).Scan(&reserve, &reserved); err != nil {
			return err
		}
		available := reserve - reserved
		if available < coins {
			return ErrNotEnough
		}

		if err := tx.QueryRow(ctx, `
INSERT INTO deposits(user_id, tx_hash, amount_usd, currency, coins, status)
VALUES($1,$2,$3,$4,$5,'pending')
RETURNING deposit_id
`, userID, txHash, amountUSD, currency, coins).Scan(&id); err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `UPDATE system_state SET reserved_supply = reserved_supply + $1, updated_at=now() WHERE id=1`, coins); err != nil {
			return err
		}

		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('deposit_create', $1, NULL, 0, $2::jsonb)`,
			userID,
			toJSON(map[string]any{"deposit_id": id, "tx_hash": txHash, "usd": amountUSD, "currency": currency, "coins": coins}),
		)
		return err
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (d *DB) ListDeposits(ctx context.Context, status string, limit int64) ([]Deposit, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		status = "pending"
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := d.Pool.Query(ctx, `
SELECT deposit_id, user_id, tx_hash, amount_usd, currency, coins, status, created_at, approved_at, approved_by
FROM deposits
WHERE status=$1
ORDER BY created_at DESC
LIMIT $2
`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Deposit
	for rows.Next() {
		var dps Deposit
		if err := rows.Scan(&dps.DepositID, &dps.UserID, &dps.TxHash, &dps.AmountUSD, &dps.Currency, &dps.Coins, &dps.Status, &dps.CreatedAt, &dps.ApprovedAt, &dps.ApprovedBy); err != nil {
			return nil, err
		}
		out = append(out, dps)
	}
	return out, rows.Err()
}

func (d *DB) GetDeposit(ctx context.Context, depositID int64) (Deposit, error) {
	if depositID <= 0 {
		return Deposit{}, errors.New("bad deposit_id")
	}
	var out Deposit
	row := d.Pool.QueryRow(ctx, `
SELECT deposit_id, user_id, tx_hash, amount_usd, currency, coins, status, created_at, approved_at, approved_by
FROM deposits
WHERE deposit_id=$1
`, depositID)
	if err := row.Scan(&out.DepositID, &out.UserID, &out.TxHash, &out.AmountUSD, &out.Currency, &out.Coins, &out.Status, &out.CreatedAt, &out.ApprovedAt, &out.ApprovedBy); err != nil {
		return Deposit{}, err
	}
	return out, nil
}

func (d *DB) ProcessDeposit(ctx context.Context, depositID int64, adminID int64, approve bool) error {
	if depositID <= 0 || adminID <= 0 {
		return errors.New("bad params")
	}

	return d.WithTx(ctx, func(tx pgx.Tx) error {
		var userID int64
		var coins int64
		var status string
		if err := tx.QueryRow(ctx, `
SELECT user_id, coins, status
FROM deposits
WHERE deposit_id=$1
FOR UPDATE
`, depositID).Scan(&userID, &coins, &status); err != nil {
			return err
		}

		status = strings.ToLower(strings.TrimSpace(status))
		if status != "pending" {
			return nil
		}

		if approve {
			var reserve int64
			var reserved int64
			if err := tx.QueryRow(ctx, `SELECT reserve_supply, reserved_supply FROM system_state WHERE id=1 FOR UPDATE`).Scan(&reserve, &reserved); err != nil {
				return err
			}
			if reserved < coins {
				return errors.New("reserved underflow")
			}
			if reserve < coins {
				return ErrNotEnough
			}

			if _, err := tx.Exec(ctx, `UPDATE system_state SET reserve_supply=reserve_supply-$1, reserved_supply=reserved_supply-$1, updated_at=now() WHERE id=1`, coins); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance+$1 WHERE user_id=$2`, coins, userID); err != nil {
				return err
			}
			now := time.Now().UTC()
			if _, err := tx.Exec(ctx, `UPDATE deposits SET status='approved', approved_at=$1, approved_by=$2 WHERE deposit_id=$3`, now, adminID, depositID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('deposit_approve', NULL, $1, $2, $3::jsonb)`,
				userID, coins, toJSON(map[string]any{"deposit_id": depositID, "by": adminID}),
			)
			return err
		}

		// reject -> release reserved
		var reserved int64
		if err := tx.QueryRow(ctx, `SELECT reserved_supply FROM system_state WHERE id=1 FOR UPDATE`).Scan(&reserved); err != nil {
			return err
		}
		if reserved < coins {
			coins = reserved
		}
		if coins > 0 {
			if _, err := tx.Exec(ctx, `UPDATE system_state SET reserved_supply=reserved_supply-$1, updated_at=now() WHERE id=1`, coins); err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		if _, err := tx.Exec(ctx, `UPDATE deposits SET status='rejected', approved_at=$1, approved_by=$2 WHERE deposit_id=$3`, now, adminID, depositID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('deposit_reject', $1, NULL, 0, $2::jsonb)`,
			userID, toJSON(map[string]any{"deposit_id": depositID, "by": adminID}),
		)
		return err
	})
}

func interestFromBP(amount int64, bp int64) int64 {
	if amount <= 0 || bp <= 0 {
		return 0
	}
	return (amount * bp) / 10_000
}

func (d *DB) Burn(ctx context.Context, userID int64, amount int64, kind string, meta any) error {
	if amount <= 0 {
		return nil
	}
	return d.WithTx(ctx, func(tx pgx.Tx) error {
		// Lock user
		var bal int64
		if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE user_id=$1 FOR UPDATE`, userID).Scan(&bal); err != nil {
			return err
		}
		if bal < amount {
			return ErrNotEnough
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance-$1 WHERE user_id=$2`, amount, userID); err != nil {
			return err
		}
		// Reduce total supply (burn)
		if _, err := tx.Exec(ctx, `UPDATE system_state SET total_supply=GREATEST(total_supply-$1, 0), updated_at=now() WHERE id=1`, amount); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES($1, $2, NULL, $3, $4::jsonb)`, kind, userID, amount, toJSON(meta))
		return err
	})
}

// CreateBankLoan issues a loan from reserve to user balance (principal) and creates a loan record.
func (d *DB) CreateBankLoan(ctx context.Context, userID int64, principal int64, interestBP int64, termDays int64) (BankLoan, error) {
	if userID <= 0 || principal <= 0 || termDays <= 0 {
		return BankLoan{}, errors.New("bad params")
	}
	interest := interestFromBP(principal, interestBP)
	totalDue := principal + interest
	now := time.Now().UTC()
	dueAt := now.Add(time.Duration(termDays) * 24 * time.Hour)

	var out BankLoan
	err := d.WithTx(ctx, func(tx pgx.Tx) error {
		// Only one active bank loan per user.
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bank_loans WHERE user_id=$1 AND status='active')`, userID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrAlreadyExists
		}

		// Lock system reserve (respect reserved_supply).
		var reserve int64
		var reserved int64
		if err := tx.QueryRow(ctx, `SELECT reserve_supply, reserved_supply FROM system_state WHERE id=1 FOR UPDATE`).Scan(&reserve, &reserved); err != nil {
			return err
		}
		available := reserve - reserved
		if available < principal {
			return ErrNotEnough
		}

		// Lock user.
		{
			var tmp int
			if err := tx.QueryRow(ctx, `SELECT 1 FROM users WHERE user_id=$1 FOR UPDATE`, userID).Scan(&tmp); err != nil {
				return err
			}
		}

		if _, err := tx.Exec(ctx, `UPDATE system_state SET reserve_supply=reserve_supply-$1, updated_at=now() WHERE id=1`, principal); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance+$1 WHERE user_id=$2`, principal, userID); err != nil {
			return err
		}

		if err := tx.QueryRow(ctx, `
INSERT INTO bank_loans (user_id, principal, interest, total_due, term_days, status, created_at, due_at)
VALUES ($1,$2,$3,$4,$5,'active',$6,$7)
RETURNING loan_id
`, userID, principal, interest, totalDue, termDays, now, dueAt).Scan(&out.LoanID); err != nil {
			return err
		}

		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('bank_loan_issue', NULL, $1, $2, $3::jsonb)`,
			userID, principal, toJSON(map[string]any{"loan_id": out.LoanID, "principal": principal, "interest": interest, "total_due": totalDue, "term_days": termDays, "due_at": dueAt.Unix()}),
		)
		return err
	})
	if err != nil {
		return BankLoan{}, err
	}
	out.UserID = userID
	out.Principal = principal
	out.Interest = interest
	out.TotalDue = totalDue
	out.TermDays = termDays
	out.Status = "active"
	out.CreatedAt = now
	out.DueAt = dueAt
	return out, nil
}

func (d *DB) ListBankLoansByUser(ctx context.Context, userID int64, limit int64) ([]BankLoan, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := d.Pool.Query(ctx, `
SELECT loan_id, user_id, principal, interest, total_due, term_days, status, created_at, due_at, closed_at
FROM bank_loans
WHERE user_id=$1
ORDER BY created_at DESC
LIMIT $2
`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BankLoan
	for rows.Next() {
		var l BankLoan
		if err := rows.Scan(&l.LoanID, &l.UserID, &l.Principal, &l.Interest, &l.TotalDue, &l.TermDays, &l.Status, &l.CreatedAt, &l.DueAt, &l.ClosedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (d *DB) RepayBankLoan(ctx context.Context, userID int64, loanID int64) error {
	if userID <= 0 || loanID <= 0 {
		return errors.New("bad params")
	}
	return d.WithTx(ctx, func(tx pgx.Tx) error {
		var principal, interest, totalDue int64
		var status string
		if err := tx.QueryRow(ctx, `
SELECT principal, interest, total_due, status
FROM bank_loans
WHERE loan_id=$1 AND user_id=$2
FOR UPDATE
`, loanID, userID).Scan(&principal, &interest, &totalDue, &status); err != nil {
			return err
		}
		status = strings.ToLower(strings.TrimSpace(status))
		if status != "active" {
			return nil
		}

		// Lock user
		var bal int64
		if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE user_id=$1 FOR UPDATE`, userID).Scan(&bal); err != nil {
			return err
		}
		if bal < totalDue {
			return ErrNotEnough
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance-$1 WHERE user_id=$2`, totalDue, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE system_state SET reserve_supply=reserve_supply+$1, updated_at=now() WHERE id=1`, totalDue); err != nil {
			return err
		}
		now := time.Now().UTC()
		if _, err := tx.Exec(ctx, `UPDATE bank_loans SET status='repaid', closed_at=$1 WHERE loan_id=$2`, now, loanID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('bank_loan_repay', $1, NULL, $2, $3::jsonb)`,
			userID, totalDue, toJSON(map[string]any{"loan_id": loanID, "principal": principal, "interest": interest}),
		)
		return err
	})
}

// MarkOverdueBankLoans marks all expired active loans as "overdue" and applies a penalty to user balance (can go negative).
func (d *DB) MarkOverdueBankLoans(ctx context.Context, now time.Time) (int64, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rows, err := d.Pool.Query(ctx, `
SELECT loan_id
FROM bank_loans
WHERE status='active' AND due_at <= $1
ORDER BY due_at ASC
LIMIT 500
`, now)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	var processed int64
	for _, loanID := range ids {
		err := d.WithTx(ctx, func(tx pgx.Tx) error {
			var userID int64
			var totalDue int64
			var status string
			if err := tx.QueryRow(ctx, `
SELECT user_id, total_due, status
FROM bank_loans
WHERE loan_id=$1
FOR UPDATE
`, loanID).Scan(&userID, &totalDue, &status); err != nil {
				return err
			}
			if strings.ToLower(strings.TrimSpace(status)) != "active" {
				return nil
			}

			// Apply penalty (balance can go negative)
			if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance-$1 WHERE user_id=$2`, totalDue, userID); err != nil {
				return err
			}
			// Return the debt to reserve to keep reserve accounting consistent even if user goes negative.
			if _, err := tx.Exec(ctx, `UPDATE system_state SET reserve_supply=reserve_supply+$1, updated_at=now() WHERE id=1`, totalDue); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE bank_loans SET status='overdue', closed_at=$1 WHERE loan_id=$2`, now, loanID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('bank_loan_overdue', $1, NULL, $2, $3::jsonb)`,
				userID, totalDue, toJSON(map[string]any{"loan_id": loanID, "ts": now.Unix()}),
			)
			return err
		})
		if err == nil {
			processed++
		}
	}
	return processed, nil
}

func (d *DB) CreateP2PLoanRequest(ctx context.Context, borrowerID, lenderID int64, principal int64, interestBP int64, termDays int64) (P2PLoan, error) {
	if borrowerID <= 0 || lenderID <= 0 || borrowerID == lenderID || principal <= 0 || termDays <= 0 {
		return P2PLoan{}, errors.New("bad params")
	}
	interest := interestFromBP(principal, interestBP)
	totalDue := principal + interest
	now := time.Now().UTC()
	var out P2PLoan
	err := d.Pool.QueryRow(ctx, `
INSERT INTO p2p_loans (lender_id, borrower_id, principal, interest, total_due, interest_bp, term_days, status, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,'requested',$8)
RETURNING loan_id
`, lenderID, borrowerID, principal, interest, totalDue, interestBP, termDays, now).Scan(&out.LoanID)
	if err != nil {
		return P2PLoan{}, err
	}
	out.LenderID = lenderID
	out.BorrowerID = borrowerID
	out.Principal = principal
	out.Interest = interest
	out.TotalDue = totalDue
	out.InterestBP = interestBP
	out.TermDays = termDays
	out.Status = "requested"
	out.CreatedAt = now
	return out, nil
}

func (d *DB) ListIncomingP2PRequests(ctx context.Context, lenderID int64, limit int64) ([]P2PLoan, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := d.Pool.Query(ctx, `
SELECT loan_id, lender_id, borrower_id, principal, interest, total_due, interest_bp, term_days, status, created_at, accepted_at, due_at, closed_at
FROM p2p_loans
WHERE lender_id=$1 AND status='requested'
ORDER BY created_at ASC
LIMIT $2
`, lenderID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []P2PLoan
	for rows.Next() {
		var l P2PLoan
		if err := rows.Scan(&l.LoanID, &l.LenderID, &l.BorrowerID, &l.Principal, &l.Interest, &l.TotalDue, &l.InterestBP, &l.TermDays, &l.Status, &l.CreatedAt, &l.AcceptedAt, &l.DueAt, &l.ClosedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (d *DB) ListP2PLoansByUser(ctx context.Context, userID int64, limit int64) ([]P2PLoan, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := d.Pool.Query(ctx, `
SELECT loan_id, lender_id, borrower_id, principal, interest, total_due, interest_bp, term_days, status, created_at, accepted_at, due_at, closed_at
FROM p2p_loans
WHERE lender_id=$1 OR borrower_id=$1
ORDER BY created_at DESC
LIMIT $2
`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []P2PLoan
	for rows.Next() {
		var l P2PLoan
		if err := rows.Scan(&l.LoanID, &l.LenderID, &l.BorrowerID, &l.Principal, &l.Interest, &l.TotalDue, &l.InterestBP, &l.TermDays, &l.Status, &l.CreatedAt, &l.AcceptedAt, &l.DueAt, &l.ClosedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (d *DB) AcceptP2PLoan(ctx context.Context, lenderID int64, loanID int64) error {
	if lenderID <= 0 || loanID <= 0 {
		return errors.New("bad params")
	}
	now := time.Now().UTC()
	return d.WithTx(ctx, func(tx pgx.Tx) error {
		var lender int64
		var borrower int64
		var principal int64
		var interest int64
		var totalDue int64
		var termDays int64
		var status string
		if err := tx.QueryRow(ctx, `
SELECT lender_id, borrower_id, principal, interest, total_due, term_days, status
FROM p2p_loans
WHERE loan_id=$1
FOR UPDATE
`, loanID).Scan(&lender, &borrower, &principal, &interest, &totalDue, &termDays, &status); err != nil {
			return err
		}
		if lender != lenderID {
			return ErrForbidden
		}
		if strings.ToLower(strings.TrimSpace(status)) != "requested" {
			return nil
		}

		// Lock balances
		var lenderBal int64
		if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE user_id=$1 FOR UPDATE`, lenderID).Scan(&lenderBal); err != nil {
			return err
		}
		if lenderBal < principal {
			return ErrNotEnough
		}
		{
			var tmp int
			if err := tx.QueryRow(ctx, `SELECT 1 FROM users WHERE user_id=$1 FOR UPDATE`, borrower).Scan(&tmp); err != nil {
				return err
			}
		}

		if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance-$1 WHERE user_id=$2`, principal, lenderID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance+$1 WHERE user_id=$2`, principal, borrower); err != nil {
			return err
		}
		dueAt := now.Add(time.Duration(termDays) * 24 * time.Hour)
		if _, err := tx.Exec(ctx, `UPDATE p2p_loans SET status='active', accepted_at=$1, due_at=$2 WHERE loan_id=$3`, now, dueAt, loanID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('p2p_loan_issue', $1, $2, $3, $4::jsonb)`,
			lenderID, borrower, principal, toJSON(map[string]any{"loan_id": loanID, "total_due": totalDue, "interest": interest, "due_at": dueAt.Unix()}),
		)
		return err
	})
}

func (d *DB) RejectP2PLoan(ctx context.Context, lenderID int64, loanID int64) error {
	if lenderID <= 0 || loanID <= 0 {
		return errors.New("bad params")
	}
	now := time.Now().UTC()
	_, err := d.Pool.Exec(ctx, `
UPDATE p2p_loans
SET status='rejected', closed_at=$1
WHERE loan_id=$2 AND lender_id=$3 AND status='requested'
`, now, loanID, lenderID)
	return err
}

func (d *DB) RepayP2PLoan(ctx context.Context, borrowerID int64, loanID int64) error {
	if borrowerID <= 0 || loanID <= 0 {
		return errors.New("bad params")
	}
	now := time.Now().UTC()
	return d.WithTx(ctx, func(tx pgx.Tx) error {
		var lender int64
		var borrower int64
		var totalDue int64
		var status string
		if err := tx.QueryRow(ctx, `
SELECT lender_id, borrower_id, total_due, status
FROM p2p_loans
WHERE loan_id=$1
FOR UPDATE
`, loanID).Scan(&lender, &borrower, &totalDue, &status); err != nil {
			return err
		}
		if borrower != borrowerID {
			return ErrForbidden
		}
		if strings.ToLower(strings.TrimSpace(status)) != "active" {
			return nil
		}

		var bal int64
		if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE user_id=$1 FOR UPDATE`, borrowerID).Scan(&bal); err != nil {
			return err
		}
		if bal < totalDue {
			return ErrNotEnough
		}
		{
			var tmp int
			if err := tx.QueryRow(ctx, `SELECT 1 FROM users WHERE user_id=$1 FOR UPDATE`, lender).Scan(&tmp); err != nil {
				return err
			}
		}

		if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance-$1 WHERE user_id=$2`, totalDue, borrowerID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance+$1 WHERE user_id=$2`, totalDue, lender); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE p2p_loans SET status='repaid', closed_at=$1 WHERE loan_id=$2`, now, loanID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('p2p_loan_repay', $1, $2, $3, $4::jsonb)`,
			borrowerID, lender, totalDue, toJSON(map[string]any{"loan_id": loanID}),
		)
		return err
	})
}

func (d *DB) RecallP2PLoan(ctx context.Context, lenderID int64, loanID int64, minDays int64) error {
	if lenderID <= 0 || loanID <= 0 {
		return errors.New("bad params")
	}
	if minDays <= 0 {
		minDays = 5
	}
	now := time.Now().UTC()
	return d.WithTx(ctx, func(tx pgx.Tx) error {
		var lender int64
		var borrower int64
		var totalDue int64
		var termDays int64
		var status string
		var acceptedAt time.Time
		var dueAt time.Time
		if err := tx.QueryRow(ctx, `
SELECT lender_id, borrower_id, total_due, term_days, status, accepted_at, due_at
FROM p2p_loans
WHERE loan_id=$1
FOR UPDATE
`, loanID).Scan(&lender, &borrower, &totalDue, &termDays, &status, &acceptedAt, &dueAt); err != nil {
			return err
		}
		if lender != lenderID {
			return ErrForbidden
		}
		if strings.ToLower(strings.TrimSpace(status)) != "active" {
			return nil
		}
		// Allowed if overdue OR, for long loans (termDays > minDays), after minDays since accepted.
		// For short loans (termDays <= minDays) recall is allowed only after due_at.
		if now.Before(dueAt) {
			if termDays <= minDays {
				return errors.New("too early")
			}
			if now.Before(acceptedAt.Add(time.Duration(minDays) * 24 * time.Hour)) {
				return errors.New("too early")
			}
		}

		// Collect only if borrower has enough positive balance.
		var borrowerBal int64
		if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE user_id=$1 FOR UPDATE`, borrower).Scan(&borrowerBal); err != nil {
			return err
		}
		if borrowerBal < totalDue {
			return ErrNotEnough
		}
		{
			var tmp int
			if err := tx.QueryRow(ctx, `SELECT 1 FROM users WHERE user_id=$1 FOR UPDATE`, lenderID).Scan(&tmp); err != nil {
				return err
			}
		}

		if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance-$1 WHERE user_id=$2`, totalDue, borrower); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance+$1 WHERE user_id=$2`, totalDue, lenderID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE p2p_loans SET status='repaid', closed_at=$1 WHERE loan_id=$2`, now, loanID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('p2p_loan_recall', $1, $2, $3, $4::jsonb)`,
			borrower, lenderID, totalDue, toJSON(map[string]any{"loan_id": loanID}),
		)
		return err
	})
}

func (d *DB) CreateMarketListing(ctx context.Context, sellerID int64, title, description, category string, priceCoins int64, contact string, listingFee int64) (MarketListing, error) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	category = strings.ToLower(strings.TrimSpace(category))
	contact = strings.TrimSpace(contact)
	if sellerID <= 0 || title == "" || description == "" || contact == "" || priceCoins <= 0 {
		return MarketListing{}, errors.New("bad params")
	}
	if category == "" {
		category = "other"
	}
	if listingFee < 0 {
		listingFee = 0
	}
	now := time.Now().UTC()
	var out MarketListing
	err := d.WithTx(ctx, func(tx pgx.Tx) error {
		// fee burn
		if listingFee > 0 {
			var bal int64
			if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE user_id=$1 FOR UPDATE`, sellerID).Scan(&bal); err != nil {
				return err
			}
			if bal < listingFee {
				return ErrNotEnough
			}
			if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance-$1 WHERE user_id=$2`, listingFee, sellerID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE system_state SET total_supply=GREATEST(total_supply-$1,0), updated_at=now() WHERE id=1`, listingFee); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('market_listing_fee_burn', $1, NULL, $2, $3::jsonb)`,
				sellerID, listingFee, toJSON(map[string]any{"fee": listingFee}),
			); err != nil {
				return err
			}
		}

		if err := tx.QueryRow(ctx, `
INSERT INTO market_listings (seller_id, title, description, category, price_coins, contact, status, created_at)
VALUES ($1,$2,$3,$4,$5,$6,'active',$7)
RETURNING listing_id
`, sellerID, title, description, category, priceCoins, contact, now).Scan(&out.ListingID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return MarketListing{}, err
	}
	out.SellerID = sellerID
	out.Title = title
	out.Description = description
	out.Category = category
	out.PriceCoins = priceCoins
	out.Contact = contact
	out.Status = "active"
	out.CreatedAt = now
	return out, nil
}

func (d *DB) AddMarketListingImage(ctx context.Context, listingID int64, mime string, data []byte) (int64, error) {
	mime = strings.TrimSpace(mime)
	if listingID <= 0 || mime == "" || len(data) == 0 {
		return 0, errors.New("bad params")
	}
	var imageID int64
	err := d.Pool.QueryRow(ctx, `
INSERT INTO market_listing_images (listing_id, mime, data)
VALUES ($1,$2,$3)
RETURNING image_id
`, listingID, mime, data).Scan(&imageID)
	return imageID, err
}

func (d *DB) GetMarketListingImage(ctx context.Context, imageID int64) (MarketListingImage, error) {
	var img MarketListingImage
	row := d.Pool.QueryRow(ctx, `
SELECT image_id, listing_id, mime, data, created_at
FROM market_listing_images
WHERE image_id=$1
`, imageID)
	if err := row.Scan(&img.ImageID, &img.ListingID, &img.Mime, &img.Data, &img.CreatedAt); err != nil {
		return MarketListingImage{}, err
	}
	return img, nil
}

func (d *DB) ListMarketListings(ctx context.Context, status string, limit int64) ([]MarketListing, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		status = "active"
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := d.Pool.Query(ctx, `
SELECT l.listing_id, l.seller_id, l.title, l.description, l.category, l.price_coins, l.contact, l.status, l.created_at, l.sold_at, l.buyer_id,
       (SELECT image_id FROM market_listing_images WHERE listing_id=l.listing_id ORDER BY created_at ASC LIMIT 1) AS image_id
FROM market_listings l
WHERE l.status=$1
ORDER BY l.created_at DESC
LIMIT $2
`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MarketListing
	for rows.Next() {
		var l MarketListing
		if err := rows.Scan(&l.ListingID, &l.SellerID, &l.Title, &l.Description, &l.Category, &l.PriceCoins, &l.Contact, &l.Status, &l.CreatedAt, &l.SoldAt, &l.BuyerID, &l.ImageID); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (d *DB) ListMyMarketListings(ctx context.Context, sellerID int64, limit int64) ([]MarketListing, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := d.Pool.Query(ctx, `
SELECT l.listing_id, l.seller_id, l.title, l.description, l.category, l.price_coins, l.contact, l.status, l.created_at, l.sold_at, l.buyer_id,
       (SELECT image_id FROM market_listing_images WHERE listing_id=l.listing_id ORDER BY created_at ASC LIMIT 1) AS image_id
FROM market_listings l
WHERE l.seller_id=$1
ORDER BY l.created_at DESC
LIMIT $2
`, sellerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MarketListing
	for rows.Next() {
		var l MarketListing
		if err := rows.Scan(&l.ListingID, &l.SellerID, &l.Title, &l.Description, &l.Category, &l.PriceCoins, &l.Contact, &l.Status, &l.CreatedAt, &l.SoldAt, &l.BuyerID, &l.ImageID); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (d *DB) BuyMarketListing(ctx context.Context, buyerID int64, listingID int64) error {
	if buyerID <= 0 || listingID <= 0 {
		return errors.New("bad params")
	}
	now := time.Now().UTC()
	return d.WithTx(ctx, func(tx pgx.Tx) error {
		var sellerID int64
		var price int64
		var status string
		var category string
		if err := tx.QueryRow(ctx, `
	SELECT seller_id, price_coins, status, category
	FROM market_listings
	WHERE listing_id=$1
	FOR UPDATE
	`, listingID).Scan(&sellerID, &price, &status, &category); err != nil {
			return err
		}
		if strings.ToLower(strings.TrimSpace(status)) != "active" {
			return nil
		}
		if sellerID == buyerID {
			return errors.New("cant buy own listing")
		}

		cat := strings.ToLower(strings.TrimSpace(category))
		isFiat := cat == "exchange" || cat == "fiat"
		if isFiat {
			// Fiat/exchange listing: mark as sold, no in-app coin transfer.
			if _, err := tx.Exec(ctx, `UPDATE market_listings SET status='sold', sold_at=$1, buyer_id=$2 WHERE listing_id=$3`, now, buyerID, listingID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('market_buy_fiat', $1, $2, 0, $3::jsonb)`,
				buyerID, sellerID, toJSON(map[string]any{"listing_id": listingID, "category": cat, "price_coins": price}),
			)
			return err
		}

		var buyerBal int64
		if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE user_id=$1 FOR UPDATE`, buyerID).Scan(&buyerBal); err != nil {
			return err
		}
		if buyerBal < price {
			return ErrNotEnough
		}
		{
			var tmp int64
			if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE user_id=$1 FOR UPDATE`, sellerID).Scan(&tmp); err != nil {
				return err
			}
		}

		if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance-$1 WHERE user_id=$2`, price, buyerID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance+$1 WHERE user_id=$2`, price, sellerID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE market_listings SET status='sold', sold_at=$1, buyer_id=$2 WHERE listing_id=$3`, now, buyerID, listingID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('market_buy', $1, $2, $3, $4::jsonb)`,
			buyerID, sellerID, price, toJSON(map[string]any{"listing_id": listingID}),
		)
		return err
	})
}

func (d *DB) FreezeBalance(ctx context.Context, userID int64, amount int64) error {
	if userID <= 0 || amount <= 0 {
		return errors.New("bad params")
	}
	return d.WithTx(ctx, func(tx pgx.Tx) error {
		var bal int64
		var frozen int64
		if err := tx.QueryRow(ctx, `SELECT balance, frozen_balance FROM users WHERE user_id=$1 FOR UPDATE`, userID).Scan(&bal, &frozen); err != nil {
			return err
		}
		if bal < amount {
			return ErrNotEnough
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance-$1, frozen_balance=frozen_balance+$1 WHERE user_id=$2`, amount, userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('balance_freeze', $1, NULL, $2, $3::jsonb)`,
			userID, amount, toJSON(map[string]any{"amount": amount}),
		)
		return err
	})
}

func (d *DB) UnfreezeBalance(ctx context.Context, userID int64, amount int64) error {
	if userID <= 0 || amount <= 0 {
		return errors.New("bad params")
	}
	return d.WithTx(ctx, func(tx pgx.Tx) error {
		var bal int64
		var frozen int64
		if err := tx.QueryRow(ctx, `SELECT balance, frozen_balance FROM users WHERE user_id=$1 FOR UPDATE`, userID).Scan(&bal, &frozen); err != nil {
			return err
		}
		if frozen < amount {
			return ErrNotEnough
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET balance=balance+$1, frozen_balance=frozen_balance-$1 WHERE user_id=$2`, amount, userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('balance_unfreeze', $1, NULL, $2, $3::jsonb)`,
			userID, amount, toJSON(map[string]any{"amount": amount}),
		)
		return err
	})
}

func dayUTC(t time.Time) time.Time {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func (d *DB) GetUserDaily(ctx context.Context, userID int64, day time.Time) (UserDaily, error) {
	day = dayUTC(day)
	out := UserDaily{UserID: userID, Day: day}
	err := d.Pool.QueryRow(ctx, `SELECT tapped, extra_quota FROM user_daily WHERE user_id=$1 AND day=$2`, userID, day).Scan(&out.Tapped, &out.ExtraQuota)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, nil
		}
		return UserDaily{}, err
	}
	return out, nil
}

func (d *DB) GetDepositWallets(ctx context.Context) (map[string]string, error) {
	rows, err := d.Pool.Query(ctx, `SELECT currency, address FROM deposit_wallets ORDER BY currency ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var c string
		var a string
		if err := rows.Scan(&c, &a); err != nil {
			return nil, err
		}
		c = strings.ToUpper(strings.TrimSpace(c))
		a = strings.TrimSpace(a)
		if c == "" || a == "" {
			continue
		}
		out[c] = a
	}
	return out, rows.Err()
}

func (d *DB) EnsureDepositWalletsIfEmpty(ctx context.Context, wallets map[string]string) error {
	if len(wallets) == 0 {
		return nil
	}
	var n int64
	if err := d.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM deposit_wallets`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	return d.SetDepositWallets(ctx, wallets)
}

func (d *DB) SetDepositWallets(ctx context.Context, wallets map[string]string) error {
	return d.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM deposit_wallets`); err != nil {
			return err
		}
		for k, v := range wallets {
			kk := strings.ToUpper(strings.TrimSpace(k))
			vv := strings.TrimSpace(v)
			if kk == "" || vv == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO deposit_wallets(currency, address) VALUES($1,$2) ON CONFLICT (currency) DO UPDATE SET address=EXCLUDED.address, updated_at=now()`, kk, vv); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO ledger(kind, from_id, to_id, amount, meta) VALUES('admin_set_deposit_wallets', NULL, NULL, 0, '{}'::jsonb)`)
		return err
	})
}
