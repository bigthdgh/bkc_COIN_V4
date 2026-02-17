package marketplace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"bkc_coin_v2/internal/i18n"
)

// ItemType тип товара
type ItemType string

const (
	ItemTypeDigital  ItemType = "digital"  // Цифровые товары (NFT)
	ItemTypePhysical ItemType = "physical" // Физические товары
	ItemTypeFiat     ItemType = "fiat"     // Фиатные товары
)

// ListingStatus статус объявления
type ListingStatus string

const (
	StatusActive    ListingStatus = "active"
	StatusSold      ListingStatus = "sold"
	StatusCancelled ListingStatus = "cancelled"
	StatusExpired   ListingStatus = "expired"
	StatusEscrow    ListingStatus = "escrow"
	StatusDisputed  ListingStatus = "disputed"
)

// EscrowStatus статус Escrow
type EscrowStatus string

const (
	EscrowPending   EscrowStatus = "pending"
	EscrowConfirmed EscrowStatus = "confirmed"
	EscrowReleased  EscrowStatus = "released"
	EscrowRefunded  EscrowStatus = "refunded"
	EscrowDisputed  EscrowStatus = "disputed"
)

// NFTMarketplace маркетплейс NFT
type NFTMarketplace struct {
	// Объявления
	listings map[string]*Listing
	mu       sync.RWMutex

	// Escrow сделки
	escrows  map[string]*EscrowTransaction
	escrowMu sync.RWMutex

	// Пользователи
	users  map[int64]*MarketUser
	userMu sync.RWMutex

	// Конфигурация
	config MarketplaceConfig

	// Метрики
	metrics *MarketplaceMetrics

	// Кэш
	cache   map[string]interface{}
	cacheMu sync.RWMutex
}

// Listing объявление на маркетплейсе
type Listing struct {
	ID            string            `json:"id"`
	SellerID      int64             `json:"seller_id"`
	SellerName    string            `json:"seller_name"`
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	Type          ItemType          `json:"type"`
	Category      string            `json:"category"`
	Price         int64             `json:"price"`
	Currency      string            `json:"currency"`
	Images        []string          `json:"images"`
	Tags          []string          `json:"tags"`
	Attributes    map[string]string `json:"attributes"`
	Status        ListingStatus     `json:"status"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	ExpiresAt     time.Time         `json:"expires_at"`
	ViewCount     int64             `json:"view_count"`
	ContactInfo   string            `json:"contact_info"`
	IsVerified    bool              `json:"is_verified"`
	IsPremium     bool              `json:"is_premium"`
	EscrowEnabled bool              `json:"escrow_enabled"`
}

// EscrowTransaction Escrow транзакция
type EscrowTransaction struct {
	ID              string       `json:"id"`
	ListingID       string       `json:"listing_id"`
	BuyerID         int64        `json:"buyer_id"`
	SellerID        int64        `json:"seller_id"`
	Amount          int64        `json:"amount"`
	Currency        string       `json:"currency"`
	Status          EscrowStatus `json:"status"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
	ConfirmedAt     time.Time    `json:"confirmed_at"`
	ReleasedAt      time.Time    `json:"released_at"`
	Hash            string       `json:"hash"`
	Secret          string       `json:"secret"`
	BuyerConfirmed  bool         `json:"buyer_confirmed"`
	SellerConfirmed bool         `json:"seller_confirmed"`
	DisputeReason   string       `json:"dispute_reason,omitempty"`
	AdminNotes      string       `json:"admin_notes,omitempty"`
}

// MarketUser пользователь маркетплейса
type MarketUser struct {
	UserID         int64      `json:"user_id"`
	Username       string     `json:"username"`
	Rating         float64    `json:"rating"`
	TotalSales     int64      `json:"total_sales"`
	TotalPurchases int64      `json:"total_purchases"`
	IsVerified     bool       `json:"is_verified"`
	IsPremium      bool       `json:"is_premium"`
	Balance        int64      `json:"balance"`
	FrozenBalance  int64      `json:"frozen_balance"`
	CreatedAt      time.Time  `json:"created_at"`
	LastActive     time.Time  `json:"last_active"`
	BannedUntil    *time.Time `json:"banned_until,omitempty"`
	BanReason      string     `json:"ban_reason,omitempty"`
}

// MarketplaceConfig конфигурация маркетплейса
type MarketplaceConfig struct {
	ListingFee           int64         `json:"listing_fee"`
	EscrowFee            float64       `json:"escrow_fee"`
	MaxListingDuration   time.Duration `json:"max_listing_duration"`
	MaxImagesPerListing  int           `json:"max_images_per_listing"`
	MaxTitleLength       int           `json:"max_title_length"`
	MaxDescriptionLength int           `json:"max_description_length"`
	VerificationCost     int64         `json:"verification_cost"`
	PremiumCost          int64         `json:"premium_cost"`
	MinRating            float64       `json:"min_rating"`
	MaxActiveListings    int           `json:"max_active_listings"`
	EscrowTimeout        time.Duration `json:"escrow_timeout"`
	DisputeTimeout       time.Duration `json:"dispute_timeout"`
}

// MarketplaceMetrics метрики маркетплейса
type MarketplaceMetrics struct {
	TotalListings        int64     `json:"total_listings"`
	ActiveListings       int64     `json:"active_listings"`
	TotalSales           int64     `json:"total_sales"`
	TotalRevenue         int64     `json:"total_revenue"`
	TotalUsers           int64     `json:"total_users"`
	VerifiedUsers        int64     `json:"verified_users"`
	PremiumUsers         int64     `json:"premium_users"`
	EscrowTransactions   int64     `json:"escrow_transactions"`
	DisputedTransactions int64     `json:"disputed_transactions"`
	LastUpdated          time.Time `json:"last_updated"`
	mu                   sync.RWMutex
}

// CreateListingRequest запрос на создание объявления
type CreateListingRequest struct {
	SellerID      int64             `json:"seller_id"`
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	Type          ItemType          `json:"type"`
	Category      string            `json:"category"`
	Price         int64             `json:"price"`
	Currency      string            `json:"currency"`
	Images        []string          `json:"images"`
	Tags          []string          `json:"tags"`
	Attributes    map[string]string `json:"attributes"`
	ContactInfo   string            `json:"contact_info"`
	EscrowEnabled bool              `json:"escrow_enabled"`
	Duration      time.Duration     `json:"duration"`
}

// PurchaseRequest запрос на покупку
type PurchaseRequest struct {
	ListingID string `json:"listing_id"`
	BuyerID   int64  `json:"buyer_id"`
	UseEscrow bool   `json:"use_escrow"`
}

// DefaultMarketplaceConfig конфигурация по умолчанию
func DefaultMarketplaceConfig() MarketplaceConfig {
	return MarketplaceConfig{
		ListingFee:           2000,                // 2000 BKC
		EscrowFee:            0.02,                // 2%
		MaxListingDuration:   30 * 24 * time.Hour, // 30 дней
		MaxImagesPerListing:  10,
		MaxTitleLength:       100,
		MaxDescriptionLength: 2000,
		VerificationCost:     100000, // 100k BKC
		PremiumCost:          50000,  // 50k BKC
		MinRating:            3.0,
		MaxActiveListings:    50,
		EscrowTimeout:        24 * time.Hour,
		DisputeTimeout:       7 * 24 * time.Hour,
	}
}

// NewNFTMarketplace создает новый маркетплейс
func NewNFTMarketplace(config MarketplaceConfig) *NFTMarketplace {
	return &NFTMarketplace{
		listings: make(map[string]*Listing),
		escrows:  make(map[string]*EscrowTransaction),
		users:    make(map[int64]*MarketUser),
		config:   config,
		metrics:  &MarketplaceMetrics{},
		cache:    make(map[string]interface{}),
	}
}

// CreateListing создает новое объявление
func (nm *NFTMarketplace) CreateListing(ctx context.Context, req *CreateListingRequest, lang i18n.Language) (*Listing, error) {
	// Валидация запроса
	if err := nm.validateCreateListingRequest(req, lang); err != nil {
		return nil, err
	}

	// Проверка пользователя
	user, err := nm.getOrCreateUser(req.SellerID, "")
	if err != nil {
		return nil, fmt.Errorf(i18n.T(lang, "error_user_not_found"))
	}

	// Проверка лимитов
	if user.BannedUntil != nil && user.BannedUntil.After(time.Now()) {
		return nil, fmt.Errorf(i18n.T(lang, "error_user_banned"))
	}

	activeCount := nm.getUserActiveListings(req.SellerID)
	if activeCount >= nm.config.MaxActiveListings {
		return nil, fmt.Errorf(i18n.T(lang, "error_max_listings_exceeded"))
	}

	// Проверка баланса для комиссии
	if user.Balance < nm.config.ListingFee {
		return nil, fmt.Errorf(i18n.T(lang, "error_insufficient_balance"))
	}

	// Создание объявления
	listing := &Listing{
		ID:            nm.generateID("listing"),
		SellerID:      req.SellerID,
		SellerName:    user.Username,
		Title:         req.Title,
		Description:   req.Description,
		Type:          req.Type,
		Category:      req.Category,
		Price:         req.Price,
		Currency:      req.Currency,
		Images:        req.Images,
		Tags:          req.Tags,
		Attributes:    req.Attributes,
		Status:        StatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(req.Duration),
		ViewCount:     0,
		ContactInfo:   req.ContactInfo,
		IsVerified:    user.IsVerified,
		IsPremium:     user.IsPremium,
		EscrowEnabled: req.EscrowEnabled,
	}

	// Списание комиссии за размещение
	user.Balance -= nm.config.ListingFee
	nm.updateUser(user)

	// Сохранение объявления
	nm.mu.Lock()
	nm.listings[listing.ID] = listing
	nm.mu.Unlock()

	// Обновление метрик
	nm.incrementTotalListings()
	nm.incrementActiveListings()
	nm.incrementTotalRevenue(nm.config.ListingFee)

	// Очистка кэша
	nm.clearCache("listings")

	return listing, nil
}

// PurchaseListing покупка товара
func (nm *NFTMarketplace) PurchaseListing(ctx context.Context, req *PurchaseRequest, lang i18n.Language) (*EscrowTransaction, error) {
	nm.mu.RLock()
	listing, exists := nm.listings[req.ListingID]
	nm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf(i18n.T(lang, "error_listing_not_found"))
	}

	if listing.Status != StatusActive {
		return nil, fmt.Errorf(i18n.T(lang, "error_listing_not_active"))
	}

	// Проверка, что покупатель не продавец
	if listing.SellerID == req.BuyerID {
		return nil, fmt.Errorf(i18n.T(lang, "error_cannot_buy_own_listing"))
	}

	// Получение пользователей
	buyer, err := nm.getOrCreateUser(req.BuyerID, "")
	if err != nil {
		return nil, fmt.Errorf(i18n.T(lang, "error_user_not_found"))
	}

	seller, _ := nm.getOrCreateUser(listing.SellerID, listing.SellerName)
	_ = seller // Используется для проверки существования пользователя

	// Проверка баланса покупателя
	totalCost := listing.Price
	if req.UseEscrow {
		totalCost += int64(float64(listing.Price) * nm.config.EscrowFee)
	}

	if buyer.Balance < totalCost {
		return nil, fmt.Errorf(i18n.T(lang, "error_insufficient_balance"))
	}

	// Создание Escrow транзакции
	escrow := &EscrowTransaction{
		ID:              nm.generateID("escrow"),
		ListingID:       req.ListingID,
		BuyerID:         req.BuyerID,
		SellerID:        listing.SellerID,
		Amount:          listing.Price,
		Currency:        listing.Currency,
		Status:          EscrowPending,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Hash:            nm.generateEscrowHash(listing.ID, req.BuyerID, listing.SellerID),
		BuyerConfirmed:  false,
		SellerConfirmed: false,
	}

	// Списание средств с покупателя
	buyer.Balance -= totalCost
	buyer.FrozenBalance += listing.Price
	nm.updateUser(buyer)

	// Сохранение транзакции
	nm.escrowMu.Lock()
	nm.escrows[escrow.ID] = escrow
	nm.escrowMu.Unlock()

	// Обновление статуса объявления
	listing.Status = StatusEscrow
	nm.updateListing(listing)

	// Обновление метрик
	nm.incrementEscrowTransactions()
	nm.incrementTotalSales()

	return escrow, nil
}

// ConfirmDelivery подтверждение доставки (для цифровых товаров)
func (nm *NFTMarketplace) ConfirmDelivery(ctx context.Context, escrowID string, userID int64, isBuyer bool, lang i18n.Language) error {
	nm.escrowMu.Lock()
	escrow, exists := nm.escrows[escrowID]
	if !exists {
		nm.escrowMu.Unlock()
		return fmt.Errorf(i18n.T(lang, "error_escrow_not_found"))
	}

	if isBuyer {
		escrow.BuyerConfirmed = true
	} else {
		escrow.SellerConfirmed = true
	}

	// Если обе стороны подтвердили,释放 средства
	if escrow.BuyerConfirmed && escrow.SellerConfirmed {
		escrow.Status = EscrowReleased
		escrow.ReleasedAt = time.Now()
		escrow.UpdatedAt = time.Now()

		// Выплата продавцу за вычетом комиссии
		sellerFee := int64(float64(escrow.Amount) * nm.config.EscrowFee)
		sellerAmount := escrow.Amount - sellerFee

		seller, _ := nm.getOrCreateUser(escrow.SellerID, "")
		seller.Balance += sellerAmount
		seller.FrozenBalance -= escrow.Amount
		seller.TotalSales++
		nm.updateUser(seller)

		// Разморозка средств покупателя
		buyer, _ := nm.getOrCreateUser(escrow.BuyerID, "")
		buyer.FrozenBalance -= escrow.Amount
		buyer.TotalPurchases++
		nm.updateUser(buyer)

		// Обновление объявления
		nm.mu.RLock()
		listing, exists := nm.listings[escrow.ListingID]
		nm.mu.RUnlock()
		if exists {
			listing.Status = StatusSold
			listing.UpdatedAt = time.Now()
			nm.updateListing(listing)
		}

		nm.incrementTotalRevenue(sellerFee)
	}

	nm.escrowMu.Unlock()
	return nil
}

// CreateDispute создание спора
func (nm *NFTMarketplace) CreateDispute(ctx context.Context, escrowID string, userID int64, reason string, lang i18n.Language) error {
	nm.escrowMu.Lock()
	defer nm.escrowMu.Unlock()

	escrow, exists := nm.escrows[escrowID]
	if !exists {
		return fmt.Errorf(i18n.T(lang, "error_escrow_not_found"))
	}

	if escrow.Status != EscrowPending && escrow.Status != EscrowConfirmed {
		return fmt.Errorf(i18n.T(lang, "error_cannot_dispute"))
	}

	// Проверка, что пользователь является участником сделки
	if escrow.BuyerID != userID && escrow.SellerID != userID {
		return fmt.Errorf(i18n.T(lang, "error_not_participant"))
	}

	escrow.Status = EscrowDisputed
	escrow.DisputeReason = reason
	escrow.UpdatedAt = time.Now()

	nm.incrementDisputedTransactions()
	return nil
}

// ResolveDispute разрешение спора (админ)
func (nm *NFTMarketplace) ResolveDispute(ctx context.Context, escrowID string, favorBuyer bool, adminNotes string, lang i18n.Language) error {
	nm.escrowMu.Lock()
	defer nm.escrowMu.Unlock()

	escrow, exists := nm.escrows[escrowID]
	if !exists {
		return fmt.Errorf(i18n.T(lang, "error_escrow_not_found"))
	}

	if escrow.Status != EscrowDisputed {
		return fmt.Errorf(i18n.T(lang, "error_not_disputed"))
	}

	escrow.AdminNotes = adminNotes
	escrow.UpdatedAt = time.Now()

	if favorBuyer {
		// Возврат средств покупателю
		escrow.Status = EscrowRefunded

		buyer, _ := nm.getOrCreateUser(escrow.BuyerID, "")
		buyer.Balance += escrow.Amount
		buyer.FrozenBalance -= escrow.Amount
		nm.updateUser(buyer)

		// Разморозка средств продавца
		seller, _ := nm.getOrCreateUser(escrow.SellerID, "")
		seller.FrozenBalance -= escrow.Amount
		nm.updateUser(seller)
	} else {
		// Выплата продавцу
		escrow.Status = EscrowReleased
		escrow.ReleasedAt = time.Now()

		sellerFee := int64(float64(escrow.Amount) * nm.config.EscrowFee)
		sellerAmount := escrow.Amount - sellerFee

		seller, _ := nm.getOrCreateUser(escrow.SellerID, "")
		seller.Balance += sellerAmount
		seller.FrozenBalance -= escrow.Amount
		seller.TotalSales++
		nm.updateUser(seller)

		// Разморозка средств покупателя
		buyer, _ := nm.getOrCreateUser(escrow.BuyerID, "")
		buyer.FrozenBalance -= escrow.Amount
		nm.updateUser(buyer)

		nm.incrementTotalRevenue(sellerFee)
	}

	return nil
}

// GetListings получает объявления с фильтрацией
func (nm *NFTMarketplace) GetListings(ctx context.Context, filters ListingFilters, lang i18n.Language) ([]*Listing, int64, error) {
	cacheKey := fmt.Sprintf("listings_%s", nm.hashFilters(filters))

	// Проверка кэша
	if cached := nm.getFromCache(cacheKey); cached != nil {
		if result, ok := cached.([]*Listing); ok {
			return result, int64(len(result)), nil
		}
	}

	nm.mu.RLock()
	defer nm.mu.RUnlock()

	var filtered []*Listing

	for _, listing := range nm.listings {
		if nm.matchesFilters(listing, filters) {
			filtered = append(filtered, listing)
		}
	}

	// Сохранение в кэш
	nm.setToCache(cacheKey, filtered, 5*time.Minute)

	return filtered, int64(len(filtered)), nil
}

// GetUserListings получает объявления пользователя
func (nm *NFTMarketplace) GetUserListings(ctx context.Context, userID int64, status ListingStatus) ([]*Listing, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	var listings []*Listing

	for _, listing := range nm.listings {
		if listing.SellerID == userID && (status == "" || listing.Status == status) {
			listings = append(listings, listing)
		}
	}

	return listings, nil
}

// GetUserEscrows получает Escrow транзакции пользователя
func (nm *NFTMarketplace) GetUserEscrows(ctx context.Context, userID int64) ([]*EscrowTransaction, error) {
	nm.escrowMu.RLock()
	defer nm.escrowMu.RUnlock()

	var escrows []*EscrowTransaction

	for _, escrow := range nm.escrows {
		if escrow.BuyerID == userID || escrow.SellerID == userID {
			escrows = append(escrows, escrow)
		}
	}

	return escrows, nil
}

// Вспомогательные методы

func (nm *NFTMarketplace) validateCreateListingRequest(req *CreateListingRequest, lang i18n.Language) error {
	if req.Title == "" {
		return fmt.Errorf(i18n.T(lang, "error_title_required"))
	}

	if len(req.Title) > nm.config.MaxTitleLength {
		return fmt.Errorf(i18n.T(lang, "error_title_too_long"))
	}

	if len(req.Description) > nm.config.MaxDescriptionLength {
		return fmt.Errorf(i18n.T(lang, "error_description_too_long"))
	}

	if req.Price <= 0 {
		return fmt.Errorf(i18n.T(lang, "error_invalid_price"))
	}

	if len(req.Images) > nm.config.MaxImagesPerListing {
		return fmt.Errorf(i18n.T(lang, "error_too_many_images"))
	}

	return nil
}

func (nm *NFTMarketplace) getOrCreateUser(userID int64, username string) (*MarketUser, error) {
	nm.userMu.Lock()
	defer nm.userMu.Unlock()

	if user, exists := nm.users[userID]; exists {
		return user, nil
	}

	user := &MarketUser{
		UserID:     userID,
		Username:   username,
		Rating:     5.0,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}

	nm.users[userID] = user
	nm.incrementTotalUsers()

	return user, nil
}

func (nm *NFTMarketplace) getUserActiveListings(userID int64) int {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	count := 0
	for _, listing := range nm.listings {
		if listing.SellerID == userID && listing.Status == StatusActive {
			count++
		}
	}

	return count
}

func (nm *NFTMarketplace) updateUser(user *MarketUser) {
	nm.userMu.Lock()
	defer nm.userMu.Unlock()

	user.LastActive = time.Now()
	nm.users[user.UserID] = user
}

func (nm *NFTMarketplace) updateListing(listing *Listing) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	nm.listings[listing.ID] = listing
	nm.clearCache("listings")
}

func (nm *NFTMarketplace) generateID(prefix string) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())))
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(hash[:8]))
}

func (nm *NFTMarketplace) generateEscrowHash(listingID string, buyerID, sellerID int64) string {
	data := fmt.Sprintf("%s_%d_%d_%d", listingID, buyerID, sellerID, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func (nm *NFTMarketplace) matchesFilters(listing *Listing, filters ListingFilters) bool {
	if filters.Type != "" && listing.Type != filters.Type {
		return false
	}

	if filters.Category != "" && listing.Category != filters.Category {
		return false
	}

	if filters.MinPrice > 0 && listing.Price < filters.MinPrice {
		return false
	}

	if filters.MaxPrice > 0 && listing.Price > filters.MaxPrice {
		return false
	}

	if filters.SellerID > 0 && listing.SellerID != filters.SellerID {
		return false
	}

	if filters.Status != "" && listing.Status != filters.Status {
		return false
	}

	if filters.IsVerified && !listing.IsVerified {
		return false
	}

	return true
}

func (nm *NFTMarketplace) hashFilters(filters ListingFilters) string {
	data, _ := json.Marshal(filters)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:8])
}

// Кэш методы
func (nm *NFTMarketplace) getFromCache(key string) interface{} {
	nm.cacheMu.RLock()
	defer nm.cacheMu.RUnlock()

	return nm.cache[key]
}

func (nm *NFTMarketplace) setToCache(key string, value interface{}, ttl time.Duration) {
	nm.cacheMu.Lock()
	defer nm.cacheMu.Unlock()

	nm.cache[key] = value

	// Удаление из кэша через TTL
	go func() {
		time.Sleep(ttl)
		nm.cacheMu.Lock()
		delete(nm.cache, key)
		nm.cacheMu.Unlock()
	}()
}

func (nm *NFTMarketplace) clearCache(prefix string) {
	nm.cacheMu.Lock()
	defer nm.cacheMu.Unlock()

	for key := range nm.cache {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(nm.cache, key)
		}
	}
}

// Метрики
func (nm *NFTMarketplace) incrementTotalListings() {
	nm.metrics.mu.Lock()
	defer nm.metrics.mu.Unlock()
	nm.metrics.TotalListings++
	nm.metrics.LastUpdated = time.Now()
}

func (nm *NFTMarketplace) incrementActiveListings() {
	nm.metrics.mu.Lock()
	defer nm.metrics.mu.Unlock()
	nm.metrics.ActiveListings++
	nm.metrics.LastUpdated = time.Now()
}

func (nm *NFTMarketplace) incrementTotalSales() {
	nm.metrics.mu.Lock()
	defer nm.metrics.mu.Unlock()
	nm.metrics.TotalSales++
	nm.metrics.LastUpdated = time.Now()
}

func (nm *NFTMarketplace) incrementTotalRevenue(amount int64) {
	nm.metrics.mu.Lock()
	defer nm.metrics.mu.Unlock()
	nm.metrics.TotalRevenue += amount
	nm.metrics.LastUpdated = time.Now()
}

func (nm *NFTMarketplace) incrementTotalUsers() {
	nm.metrics.mu.Lock()
	defer nm.metrics.mu.Unlock()
	nm.metrics.TotalUsers++
	nm.metrics.LastUpdated = time.Now()
}

func (nm *NFTMarketplace) incrementEscrowTransactions() {
	nm.metrics.mu.Lock()
	defer nm.metrics.mu.Unlock()
	nm.metrics.EscrowTransactions++
	nm.metrics.LastUpdated = time.Now()
}

func (nm *NFTMarketplace) incrementDisputedTransactions() {
	nm.metrics.mu.Lock()
	defer nm.metrics.mu.Unlock()
	nm.metrics.DisputedTransactions++
	nm.metrics.LastUpdated = time.Now()
}

// GetMetrics возвращает метрики
func (nm *NFTMarketplace) GetMetrics() MarketplaceMetrics {
	nm.metrics.mu.RLock()
	defer nm.metrics.mu.RUnlock()

	return *nm.metrics
}

// ListingFilters фильтры для поиска объявлений
type ListingFilters struct {
	Type       ItemType      `json:"type"`
	Category   string        `json:"category"`
	MinPrice   int64         `json:"min_price"`
	MaxPrice   int64         `json:"max_price"`
	SellerID   int64         `json:"seller_id"`
	Status     ListingStatus `json:"status"`
	IsVerified bool          `json:"is_verified"`
	Tags       []string      `json:"tags"`
}
