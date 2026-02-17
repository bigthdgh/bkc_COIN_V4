package marketplace

import (
	"context"
	"fmt"
	"log"
	"time"

	"bkc_coin_v2/internal/db"
)

// MarketplaceManager управляет маркетплейсом NFT и физических товаров
type MarketplaceManager struct {
	db *db.DB
}

// NewMarketplaceManager создает новый менеджер маркетплейса
func NewMarketplaceManager(database *db.DB) *MarketplaceManager {
	return &MarketplaceManager{db: database}
}

// NFTItem информация о NFT
type NFTItem struct {
	ID          int64                  `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	ImageURL    string                 `json:"image_url"`
	Rarity      string                 `json:"rarity"`
	PriceCoins  int64                  `json:"price_coins"`
	SupplyTotal int64                  `json:"supply_total"`
	SupplyLeft  int64                  `json:"supply_left"`
	Perks       map[string]interface{} `json:"perks"`
	IsTradeable bool                   `json:"is_tradeable"`
	IsCollateral bool                  `json:"is_collateral"`
	CreatedAt   time.Time              `json:"created_at"`
}

// UserNFT NFT пользователя
type UserNFT struct {
	UserID      int64     `json:"user_id"`
	NFTID       int64     `json:"nft_id"`
	Title       string    `json:"title"`
	ImageURL    string    `json:"image_url"`
	Qty         int64     `json:"qty"`
	IsCollateral bool     `json:"is_collateral"`
	LoanID      int64     `json:"loan_id"`
	AcquiredAt  time.Time `json:"acquired_at"`
}

// MarketListing объявление на барахолке
type MarketListing struct {
	ID          int64      `json:"id"`
	SellerID    int64      `json:"seller_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Category    string     `json:"category"`
	PriceCoins  int64      `json:"price_coins"`
	ContactInfo string     `json:"contact_info"`
	Location    string     `json:"location"`
	Condition   string     `json:"condition"`
	Status      string     `json:"status"`
	ViewsCount  int        `json:"views_count"`
	CreatedAt   time.Time  `json:"created_at"`
	SoldAt      *time.Time `json:"sold_at"`
	BuyerID     *int64     `json:"buyer_id"`
}

// P2POrders P2P ордер на покупку/продажу BKC
type P2POrder struct {
	ID           int64      `json:"id"`
	SellerID     int64      `json:"seller_id"`
	BuyerID      *int64     `json:"buyer_id"`
	AmountBKC    int64      `json:"amount_bkc"`
	PriceTON     float64    `json:"price_ton"`
	PriceUSD     *float64   `json:"price_usd"`
	Status       string     `json:"status"`
	EscrowBKC    int64      `json:"escrow_bkc"`
	ContactMethod string    `json:"contact_method"`
	Description  string     `json:"description"`
	CreatedAt    time.Time  `json:"created_at"`
	LockedAt     *time.Time `json:"locked_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	DisputeReason string    `json:"dispute_reason"`
}

// GetNFTCatalog получает каталог NFT
func (mp *MarketplaceManager) GetNFTCatalog(ctx context.Context, rarity string) ([]NFTItem, error) {
	query := `
		SELECT id, title, description, image_url, rarity, price_coins, 
		       supply_total, supply_left, perks, is_tradeable, is_collateral, created_at
		FROM nfts 
		WHERE supply_left > 0
	`
	args := []interface{}{}
	
	if rarity != "" && rarity != "all" {
		query += " AND rarity = $1"
		args = append(args, rarity)
	}
	
	query += " ORDER BY price_coins ASC"
	
	rows, err := mp.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get NFT catalog: %w", err)
	}
	defer rows.Close()
	
	var items []NFTItem
	for rows.Next() {
		var item NFTItem
		var perksJSON string
		
		err := rows.Scan(
			&item.ID, &item.Title, &item.Description, &item.ImageURL,
			&item.Rarity, &item.PriceCoins, &item.SupplyTotal, &item.SupplyLeft,
			&perksJSON, &item.IsTradeable, &item.IsCollateral, &item.CreatedAt,
		)
		if err != nil {
			continue
		}
		
		// Десериализуем perks
		if perksJSON != "" && perksJSON != "{}" {
			// Простая десериализация для базовых типов
			item.Perks = make(map[string]interface{})
			// Здесь можно добавить JSON парсинг
		}
		
		items = append(items, item)
	}
	
	return items, rows.Err()
}

// PurchaseNFT покупает NFT
func (mp *MarketplaceManager) PurchaseNFT(ctx context.Context, userID, nftID int64, qty int64) error {
	if qty <= 0 {
		return fmt.Errorf("quantity must be positive")
	}
	
	tx, err := mp.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	
	// Получаем информацию о NFT и блокируем запись
	var nft NFTItem
	err = tx.QueryRow(ctx, `
		SELECT id, title, price_coins, supply_left, is_tradeable
		FROM nfts 
		WHERE id = $1 AND supply_left >= $2 FOR UPDATE
	`, nftID, qty).Scan(&nft.ID, &nft.Title, &nft.PriceCoins, &nft.SupplyLeft, &nft.IsTradeable)
	
	if err != nil {
		return fmt.Errorf("NFT not available: %w", err)
	}
	
	if !nft.IsTradeable {
		return fmt.Errorf("NFT is not tradeable")
	}
	
	totalCost := nft.PriceCoins * qty
	
	// Проверяем баланс пользователя
	var userBalance int64
	err = tx.QueryRow(ctx, "SELECT balance FROM users WHERE user_id = $1 FOR UPDATE", userID).Scan(&userBalance)
	if err != nil {
		return fmt.Errorf("failed to get user balance: %w", err)
	}
	
	if userBalance < totalCost {
		return fmt.Errorf("insufficient balance: need %d, have %d", totalCost, userBalance)
	}
	
	// Списываем монеты
	_, err = tx.Exec(ctx, "UPDATE users SET balance = balance - $1 WHERE user_id = $2", totalCost, userID)
	if err != nil {
		return fmt.Errorf("failed to deduct balance: %w", err)
	}
	
	// Обновляем количество NFT
	_, err = tx.Exec(ctx, "UPDATE nfts SET supply_left = supply_left - $1 WHERE id = $2", qty, nftID)
	if err != nil {
		return fmt.Errorf("failed to update NFT supply: %w", err)
	}
	
	// Добавляем NFT пользователю
	_, err = tx.Exec(ctx, `
		INSERT INTO user_nfts(user_id, nft_id, qty, acquired_at)
		VALUES($1, $2, $3, $4)
		ON CONFLICT (user_id, nft_id) DO UPDATE
		SET qty = user_nfts.qty + EXCLUDED.qty
	`, userID, nftID, qty, time.Now())
	if err != nil {
		return fmt.Errorf("failed to add NFT to user: %w", err)
	}
	
	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, from_id, to_id, amount, meta)
		VALUES('nft_purchase', $1, NULL, $2, $3::jsonb)
	`, userID, totalCost, fmt.Sprintf(`{
		"nft_id": %d,
		"title": "%s",
		"qty": %d,
		"price_per_item": %d
	}`, nftID, nft.Title, qty, nft.PriceCoins))
	if err != nil {
		return fmt.Errorf("failed to record purchase: %w", err)
	}
	
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit purchase: %w", err)
	}
	
	log.Printf("User %d purchased %dx NFT %d (%s) for %d BKC", 
		userID, qty, nftID, nft.Title, totalCost)
	
	return nil
}

// GetUserNFTs получает NFT пользователя
func (mp *MarketplaceManager) GetUserNFTs(ctx context.Context, userID int64) ([]UserNFT, error) {
	rows, err := mp.db.Pool.Query(ctx, `
		SELECT un.user_id, un.nft_id, n.title, n.image_url, un.qty,
		       un.is_collateral, COALESCE(un.loan_id, 0), un.acquired_at
		FROM user_nfts un
		JOIN nfts n ON un.nft_id = n.id
		WHERE un.user_id = $1 AND un.qty > 0
		ORDER BY un.acquired_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user NFTs: %w", err)
	}
	defer rows.Close()
	
	var nfts []UserNFT
	for rows.Next() {
		var nft UserNFT
		err := rows.Scan(
			&nft.UserID, &nft.NFTID, &nft.Title, &nft.ImageURL,
			&nft.Qty, &nft.IsCollateral, &nft.LoanID, &nft.AcquiredAt,
		)
		if err != nil {
			continue
		}
		nfts = append(nfts, nft)
	}
	
	return nfts, rows.Err()
}

// CreateMarketListing создает объявление на барахолке
func (mp *MarketplaceManager) CreateMarketListing(ctx context.Context, listing *MarketListing) error {
	const listingFee = 1000 // 1000 BKC за размещение
	
	tx, err := mp.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	
	// Проверяем баланс продавца
	var sellerBalance int64
	err = tx.QueryRow(ctx, "SELECT balance FROM users WHERE user_id = $1 FOR UPDATE", listing.SellerID).Scan(&sellerBalance)
	if err != nil {
		return fmt.Errorf("failed to get seller balance: %w", err)
	}
	
	if sellerBalance < listingFee {
		return fmt.Errorf("insufficient balance for listing fee: need %d, have %d", listingFee, sellerBalance)
	}
	
	// Списываем плату за размещение (сжигается)
	_, err = tx.Exec(ctx, "UPDATE users SET balance = balance - $1 WHERE user_id = $2", listingFee, listing.SellerID)
	if err != nil {
		return fmt.Errorf("failed to deduct listing fee: %w", err)
	}
	
	// Создаем объявление
	err = tx.QueryRow(ctx, `
		INSERT INTO market_listings(
			seller_id, title, description, category, price_coins,
			contact_info, location, condition, listing_fee_paid
		) VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
	`, listing.SellerID, listing.Title, listing.Description, listing.Category,
		listing.PriceCoins, listing.ContactInfo, listing.Location, 
		listing.Condition, listingFee).Scan(&listing.ID, &listing.CreatedAt)
	
	if err != nil {
		return fmt.Errorf("failed to create listing: %w", err)
	}
	
	// Записываем в ledger (плата за размещение сжигается)
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, from_id, to_id, amount, meta)
		VALUES('market_listing_fee', $1, NULL, $2, $3::jsonb)
	`, listing.SellerID, listingFee, fmt.Sprintf(`{
		"listing_id": %d,
		"title": "%s",
		"fee": %d
	}`, listing.ID, listing.Title, listingFee))
	if err != nil {
		return fmt.Errorf("failed to record listing fee: %w", err)
	}
	
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit listing: %w", err)
	}
	
	log.Printf("User %d created market listing %d (%s) for %d BKC", 
		listing.SellerID, listing.ID, listing.Title, listing.PriceCoins)
	
	return nil
}

// GetMarketListings получает объявления на барахолке
func (mp *MarketplaceManager) GetMarketListings(ctx context.Context, category string, limit, offset int) ([]MarketListing, error) {
	query := `
		SELECT id, seller_id, title, description, category, price_coins,
		       contact_info, location, condition, status, views_count,
		       created_at, sold_at, buyer_id
		FROM market_listings 
		WHERE status = 'active'
	`
	args := []interface{}{}
	
	if category != "" && category != "all" {
		query += " AND category = $1"
		args = append(args, category)
	}
	
	query += " ORDER BY created_at DESC"
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", offset)
	}
	
	rows, err := mp.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get market listings: %w", err)
	}
	defer rows.Close()
	
	var listings []MarketListing
	for rows.Next() {
		var listing MarketListing
		err := rows.Scan(
			&listing.ID, &listing.SellerID, &listing.Title, &listing.Description,
			&listing.Category, &listing.PriceCoins, &listing.ContactInfo,
			&listing.Location, &listing.Condition, &listing.Status,
			&listing.ViewsCount, &listing.CreatedAt, &listing.SoldAt, &listing.BuyerID,
		)
		if err != nil {
			continue
		}
		listings = append(listings, listing)
	}
	
	return listings, rows.Err()
}

// CreateP2POrder создает P2P ордер
func (mp *MarketplaceManager) CreateP2POrder(ctx context.Context, order *P2POrder) error {
	tx, err := mp.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	
	// Проверяем баланс продавца для escrow
	var sellerBalance int64
	err = tx.QueryRow(ctx, "SELECT balance FROM users WHERE user_id = $1 FOR UPDATE", order.SellerID).Scan(&sellerBalance)
	if err != nil {
		return fmt.Errorf("failed to get seller balance: %w", err)
	}
	
	if sellerBalance < order.AmountBKC {
		return fmt.Errorf("insufficient balance for escrow: need %d, have %d", order.AmountBKC, sellerBalance)
	}
	
	// Блокируем монеты в escrow
	_, err = tx.Exec(ctx, "UPDATE users SET balance = balance - $1 WHERE user_id = $2", order.AmountBKC, order.SellerID)
	if err != nil {
		return fmt.Errorf("failed to lock escrow: %w", err)
	}
	
	// Создаем ордер
	err = tx.QueryRow(ctx, `
		INSERT INTO p2p_orders(
			seller_id, amount_bkc, price_ton, price_usd, status,
			escrow_bkc, contact_method, description
		) VALUES($1, $2, $3, $4, 'open', $5, $6, $7)
		RETURNING id, created_at
	`, order.SellerID, order.AmountBKC, order.PriceTON, order.PriceUSD,
		order.AmountBKC, order.ContactMethod, order.Description).Scan(&order.ID, &order.CreatedAt)
	
	if err != nil {
		return fmt.Errorf("failed to create P2P order: %w", err)
	}
	
	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, from_id, to_id, amount, meta)
		VALUES('p2p_escrow', $1, NULL, $2, $3::jsonb)
	`, order.SellerID, order.AmountBKC, fmt.Sprintf(`{
		"order_id": %d,
		"amount_bkc": %d,
		"price_ton": %.2f
	}`, order.ID, order.AmountBKC, order.PriceTON))
	if err != nil {
		return fmt.Errorf("failed to record escrow: %w", err)
	}
	
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit P2P order: %w", err)
	}
	
	log.Printf("User %d created P2P order %d: %d BKC for %.2f TON", 
		order.SellerID, order.ID, order.AmountBKC, order.PriceTON)
	
	return nil
}

// LockP2POrder блокирует P2P ордер для покупателя
func (mp *MarketplaceManager) LockP2POrder(ctx context.Context, orderID, buyerID int64) error {
	tx, err := mp.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	
	// Получаем информацию ордере
	var order P2POrder
	err = tx.QueryRow(ctx, `
		SELECT id, seller_id, buyer_id, amount_bkc, price_ton, status, escrow_bkc
		FROM p2p_orders 
		WHERE id = $1 AND status = 'open' FOR UPDATE
	`, orderID).Scan(&order.ID, &order.SellerID, &order.BuyerID, 
		&order.AmountBKC, &order.PriceTON, &order.Status, &order.EscrowBKC)
	
	if err != nil {
		return fmt.Errorf("order not available: %w", err)
	}
	
	if order.SellerID == buyerID {
		return fmt.Errorf("cannot buy own order")
	}
	
	// Обновляем статус ордера
	now := time.Now()
	_, err = tx.Exec(ctx, `
		UPDATE p2p_orders 
		SET buyer_id = $1, status = 'locked', locked_at = $2
		WHERE id = $3
	`, buyerID, now, orderID)
	if err != nil {
		return fmt.Errorf("failed to lock order: %w", err)
	}
	
	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, amount, meta)
		VALUES('p2p_lock', 0, $1::jsonb)
	`, fmt.Sprintf(`{
		"order_id": %d,
		"buyer_id": %d,
		"seller_id": %d,
		"locked_at": "%s"
	}`, orderID, buyerID, order.SellerID, now.Format(time.RFC3339)))
	if err != nil {
		return fmt.Errorf("failed to record lock: %w", err)
	}
	
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit lock: %w", err)
	}
	
	log.Printf("P2P order %d locked by buyer %d", orderID, buyerID)
	
	return nil
}

// CompleteP2POrder завершает P2P сделку
func (mp *MarketplaceManager) CompleteP2POrder(ctx context.Context, orderID int64) error {
	tx, err := mp.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	
	// Получаем информацию ордере
	var order P2POrder
	err = tx.QueryRow(ctx, `
		SELECT id, seller_id, buyer_id, amount_bkc, price_ton, status, escrow_bkc
		FROM p2p_orders 
		WHERE id = $1 AND status = 'locked' FOR UPDATE
	`, orderID).Scan(&order.ID, &order.SellerID, &order.BuyerID, 
		&order.AmountBKC, &order.PriceTON, &order.Status, &order.EscrowBKC)
	
	if err != nil {
		return fmt.Errorf("order not in locked state: %w", err)
	}
	
	if order.BuyerID == nil {
		return fmt.Errorf("order has no buyer")
	}
	
	// Переводим монеты escrow покупателю
	_, err = tx.Exec(ctx, "UPDATE users SET balance = balance + $1 WHERE user_id = $2", 
		order.EscrowBKC, *order.BuyerID)
	if err != nil {
		return fmt.Errorf("failed to transfer to buyer: %w", err)
	}
	
	// Обновляем статус ордера
	now := time.Now()
	_, err = tx.Exec(ctx, `
		UPDATE p2p_orders 
		SET status = 'completed', completed_at = $1
		WHERE id = $2
	`, now, orderID)
	if err != nil {
		return fmt.Errorf("failed to complete order: %w", err)
	}
	
	// Записываем в ledger
	_, err = tx.Exec(ctx, `
		INSERT INTO ledger(kind, from_id, to_id, amount, meta)
		VALUES('p2p_complete', $1, $2, $3, $4::jsonb)
	`, order.SellerID, *order.BuyerID, order.EscrowBKC, fmt.Sprintf(`{
		"order_id": %d,
		"price_ton": %.2f,
		"completed_at": "%s"
	}`, orderID, order.PriceTON, now.Format(time.RFC3339)))
	if err != nil {
		return fmt.Errorf("failed to record completion: %w", err)
	}
	
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit completion: %w", err)
	}
	
	log.Printf("P2P order %d completed: %d BKC transferred to buyer %d", 
		orderID, order.EscrowBKC, *order.BuyerID)
	
	return nil
}

// GetP2POrders получает P2P ордера
func (mp *MarketplaceManager) GetP2POrders(ctx context.Context, status string, limit, offset int) ([]P2POrder, error) {
	query := `
		SELECT id, seller_id, buyer_id, amount_bkc, price_ton, price_usd,
		       status, escrow_bkc, contact_method, description,
		       created_at, locked_at, completed_at, dispute_reason
		FROM p2p_orders
	`
	args := []interface{}{}
	
	if status != "" && status != "all" {
		query += " WHERE status = $1"
		args = append(args, status)
	}
	
	query += " ORDER BY created_at DESC"
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", offset)
	}
	
	rows, err := mp.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get P2P orders: %w", err)
	}
	defer rows.Close()
	
	var orders []P2POrder
	for rows.Next() {
		var order P2POrder
		err := rows.Scan(
			&order.ID, &order.SellerID, &order.BuyerID, &order.AmountBKC,
			&order.PriceTON, &order.PriceUSD, &order.Status, &order.EscrowBKC,
			&order.ContactMethod, &order.Description, &order.CreatedAt,
			&order.LockedAt, &order.CompletedAt, &order.DisputeReason,
		)
		if err != nil {
			continue
		}
		orders = append(orders, order)
	}
	
	return orders, rows.Err()
}

// GetMarketplaceStats получает статистику маркетплейса
func (mp *MarketplaceManager) GetMarketplaceStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Статистика NFT
	var totalNFT, soldNFT, activeListings int64
	var totalNFTVolume int64
	
	mp.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM nfts").Scan(&totalNFT)
	mp.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM nfts WHERE supply_left > 0").Scan(&soldNFT)
	mp.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM market_listings WHERE status = 'active'").Scan(&activeListings)
	mp.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(price_coins), 0) FROM nfts").Scan(&totalNFTVolume)
	
	// Статистика P2P
	var p2pOrders, p2pVolume int64
	mp.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM p2p_orders").Scan(&p2pOrders)
	mp.db.Pool.QueryRow(ctx, "SELECT COALESCE(SUM(amount_bkc), 0) FROM p2p_orders WHERE status = 'completed'").Scan(&p2pVolume)
	
	// Популярные категории
	var topCategory string
	var categoryCount int64
	mp.db.Pool.QueryRow(ctx, `
		SELECT category, COUNT(*) as cnt 
		FROM market_listings 
		WHERE status = 'active' 
		GROUP BY category 
		ORDER BY cnt DESC 
		LIMIT 1
	`).Scan(&topCategory, &categoryCount)
	
	stats["total_nft"] = totalNFT
	stats["sold_nft"] = soldNFT
	stats["active_listings"] = activeListings
	stats["total_nft_volume"] = totalNFTVolume
	stats["p2p_orders"] = p2pOrders
	stats["p2p_volume"] = p2pVolume
	stats["top_category"] = topCategory
	stats["top_category_count"] = categoryCount
	
	return stats, nil
}
