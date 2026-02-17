package games

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// GameType тип игры
type GameType string

const (
	GameTypeCrash GameType = "crash"
	GameTypeChart GameType = "chart"
)

// WebSocketEngine движок для WebSocket игр
type WebSocketEngine struct {
	// Клиенты
	clients map[*Client]bool
	mu      sync.RWMutex
	
	// Игры
	games map[string]*Game
	gameMu sync.RWMutex
	
	// Конфигурация
	config WebSocketConfig
	
	// HTTP upgrader
	upgrader websocket.Upgrader
	
	// Контекст
	ctx    context.Context
	cancel context.CancelFunc
	
	// Метрики
	metrics *GameMetrics
	
	// Provably Fair
	fairGenerator *ProvablyFairGenerator
}

// Client WebSocket клиент
type Client struct {
	ID        int64               `json:"id"`
	Username  string              `json:"username"`
	UserID    int64               `json:"user_id"`
	GameType  GameType            `json:"game_type"`
	Conn      *websocket.Conn    `json:"-"`
	Send      chan []byte         `json:"-"`
	LastPing  time.Time           `json:"last_ping"`
	IsPremium bool                `json:"is_premium"`
	Lang      string              `json:"lang"`
	mu        sync.RWMutex
}

// Game игра
type Game struct {
	ID           string          `json:"id"`
	Type         GameType        `json:"type"`
	Status       GameStatus      `json:"status"`
	Players      map[int64]*Player `json:"players"`
	StartedAt    time.Time       `json:"started_at"`
	EndedAt      time.Time       `json:"ended_at"`
	CrashPoint   float64         `json:"crash_point"`
	CurrentMult  float64         `json:"current_mult"`
	Hash         string          `json:"hash"`
	Secret       string          `json:"secret"`
	TotalBets    int64           `json:"total_bets"`
	TotalWins    int64           `json:"total_wins"`
	mu           sync.RWMutex
}

// Player игрок в игре
type Player struct {
	UserID      int64     `json:"user_id"`
	Username    string    `json:"username"`
	Bet         int64     `json:"bet"`
	CashedOut   bool      `json:"cashed_out"`
	CashOutMult float64   `json:"cash_out_mult"`
	WinAmount   int64     `json:"win_amount"`
	JoinedAt    time.Time `json:"joined_at"`
}

// GameStatus статус игры
type GameStatus string

const (
	GameStatusWaiting   GameStatus = "waiting"
	GameStatusStarting  GameStatus = "starting"
	GameStatusActive    GameStatus = "active"
	GameStatusCrashed   GameStatus = "crashed"
	GameStatusFinished  GameStatus = "finished"
)

// WebSocketConfig конфигурация WebSocket
type WebSocketConfig struct {
	ReadBufferSize    int           `json:"read_buffer_size"`
	WriteBufferSize   int           `json:"write_buffer_size"`
	PingPeriod        time.Duration `json:"ping_period"`
	PongWait          time.Duration `json:"pong_wait"`
	WriteWait         time.Duration `json:"write_wait"`
	MaxMessageSize    int64         `json:"max_message_size"`
	EnableCompression bool          `json:"enable_compression"`
	CrashGameSettings CrashGameSettings `json:"crash_settings"`
	ChartSettings    ChartSettings    `json:"chart_settings"`
}

// CrashGameSettings настройки игры Ракетка
type CrashGameSettings struct {
	MinMultiplier    float64       `json:"min_multiplier"`
	MaxMultiplier    float64       `json:"max_multiplier"`
	GrowthRate       float64       `json:"growth_rate"`
	UpdateInterval   time.Duration `json:"update_interval"`
	MinBet           int64         `json:"min_bet"`
	MaxBet           int64         `json:"max_bet"`
	HouseEdge        float64       `json:"house_edge"`
	InstantCrashProb float64       `json:"instant_crash_prob"`
}

// ChartSettings настройки графика
type ChartSettings struct {
	UpdateInterval   time.Duration `json:"update_interval"`
	PriceSource      string        `json:"price_source"`
	DecimalPlaces    int           `json:"decimal_places"`
	MaxDataPoints    int           `json:"max_data_points"`
}

// GameMetrics метрики игр
type GameMetrics struct {
	TotalGames        int64     `json:"total_games"`
	ActivePlayers     int64     `json:"active_players"`
	TotalBets         int64     `json:"total_bets"`
	TotalWins         int64     `json:"total_wins"`
	HouseProfit       int64     `json:"house_profit"`
	AvgGameDuration   time.Duration `json:"avg_game_duration"`
	LastUpdated       time.Time `json:"last_updated"`
	mu                sync.RWMutex
}

// ProvablyFairGenerator генератор честной игры
type ProvablyFairGenerator struct {
	serverSeed string
	clientSeed string
	nonce      int64
	mu         sync.RWMutex
}

// WebSocketMessage сообщение WebSocket
type WebSocketMessage struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
	GameID    string      `json:"game_id,omitempty"`
}

// CrashGameData данные игры Ракетка
type CrashGameData struct {
	GameID       string    `json:"game_id"`
	Multiplier   float64   `json:"multiplier"`
	Status       GameStatus `json:"status"`
	CrashPoint   float64   `json:"crash_point,omitempty"`
	TimeLeft     float64   `json:"time_left,omitempty"`
	PlayerCount  int       `json:"player_count"`
	TotalBets    int64     `json:"total_bets"`
	Hash         string    `json:"hash"`
}

// ChartPoint точка графика
type ChartPoint struct {
	Timestamp int64   `json:"timestamp"`
	Price     float64 `json:"price"`
	Volume    float64 `json:"volume"`
}

// ChartData данные графика
type ChartData struct {
	Symbol     string       `json:"symbol"`
	Points     []ChartPoint `json:"points"`
	LastUpdate time.Time    `json:"last_update"`
	Price      float64      `json:"price"`
	Change24h  float64      `json:"change_24h"`
}

// DefaultWebSocketConfig конфигурация по умолчанию
func DefaultWebSocketConfig() WebSocketConfig {
	return WebSocketConfig{
		ReadBufferSize:    1024,
		WriteBufferSize:   1024,
		PingPeriod:        30 * time.Second,
		PongWait:          35 * time.Second,
		WriteWait:         10 * time.Second,
		MaxMessageSize:    512,
		EnableCompression: true,
		CrashGameSettings: CrashGameSettings{
			MinMultiplier:    1.00,
			MaxMultiplier:    100.0,
			GrowthRate:       0.01,
			UpdateInterval:   100 * time.Millisecond,
			MinBet:           1000,
			MaxBet:           1000000,
			HouseEdge:        0.03,
			InstantCrashProb: 0.03,
		},
		ChartSettings: ChartSettings{
			UpdateInterval: 5 * time.Second,
			PriceSource:    "p2p_average",
			DecimalPlaces:  4,
			MaxDataPoints:  100,
		},
	}
}

// NewWebSocketEngine создает новый WebSocket движок
func NewWebSocketEngine(config WebSocketConfig) *WebSocketEngine {
	ctx, cancel := context.WithCancel(context.Background())
	
	wse := &WebSocketEngine{
		clients:      make(map[*Client]bool),
		games:        make(map[string]*Game),
		config:       config,
		ctx:          ctx,
		cancel:       cancel,
		metrics:      &GameMetrics{},
		fairGenerator: NewProvablyFairGenerator(),
	}
	
	// Настройка upgrader
	wse.upgrader = websocket.Upgrader{
		ReadBufferSize:  config.ReadBufferSize,
		WriteBufferSize: config.WriteBufferSize,
		CheckOrigin: func(r *http.Request) bool {
			// В продакшене здесь должна быть проверка origin
			return true
		},
		EnableCompression: config.EnableCompression,
	}
	
	return wse
}

// Start запускает WebSocket движок
func (wse *WebSocketEngine) Start() {
	// Запуск пингера
	go wse.pinger()
	
	// Запуск обработчика игр
	go wse.gameHandler()
	
	// Запуск обновления графика
	go wse.chartUpdater()
}

// Stop останавливает WebSocket движок
func (wse *WebSocketEngine) Stop() {
	wse.cancel()
	
	// Закрытие всех соединений
	wse.mu.Lock()
	for client := range wse.clients {
		client.Conn.Close()
	}
	wse.mu.Unlock()
}

// HandleWebSocket обрабатывает WebSocket соединение
func (wse *WebSocketEngine) HandleWebSocket(w http.ResponseWriter, r *http.Request, userID int64, username string, gameType GameType, isPremium bool, lang string) {
	conn, err := wse.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket: %v", err)
		return
	}
	
	client := &Client{
		ID:        time.Now().UnixNano(),
		UserID:    userID,
		Username:  username,
		GameType:  gameType,
		Conn:      conn,
		Send:      make(chan []byte, 256),
		LastPing:  time.Now(),
		IsPremium: isPremium,
		Lang:      lang,
	}
	
	// Добавление клиента
	wse.addClient(client)
	
	// Запуск горутин для клиента
	go client.writePump(wse)
	go client.readPump(wse)
	
	// Отправка начальных данных
	wse.sendInitialData(client)
	
	// Обновление метрик
	atomic.AddInt64(&wse.metrics.ActivePlayers, 1)
}

// addClient добавляет клиента
func (wse *WebSocketEngine) addClient(client *Client) {
	wse.mu.Lock()
	defer wse.mu.Unlock()
	
	wse.clients[client] = true
}

// removeClient удаляет клиента
func (wse *WebSocketEngine) removeClient(client *Client) {
	wse.mu.Lock()
	defer wse.mu.Unlock()
	
	if _, ok := wse.clients[client]; ok {
		delete(wse.clients, client)
		close(client.Send)
		atomic.AddInt64(&wse.metrics.ActivePlayers, -1)
	}
}

// sendInitialData отправляет начальные данные клиенту
func (wse *WebSocketEngine) sendInitialData(client *Client) {
	switch client.GameType {
	case GameTypeCrash:
		wse.sendCrashGameInfo(client)
	case GameTypeChart:
		wse.sendChartData(client)
	}
}

// sendCrashGameInfo отправляет информацию об игре Ракетка
func (wse *WebSocketEngine) sendCrashGameInfo(client *Client) {
	game := wse.getCurrentCrashGame()
	
	data := CrashGameData{
		GameID:      game.ID,
		Multiplier:  game.CurrentMult,
		Status:      game.Status,
		CrashPoint:  game.CrashPoint,
		PlayerCount: len(game.Players),
		TotalBets:   game.TotalBets,
		Hash:        game.Hash,
	}
	
	message := WebSocketMessage{
		Type:      "crash_update",
		Data:      data,
		Timestamp: time.Now(),
		GameID:    game.ID,
	}
	
	wse.sendToClient(client, message)
}

// sendChartData отправляет данные графика
func (wse *WebSocketEngine) sendChartData(client *Client) {
	// Генерация тестовых данных (в реальном приложении здесь будет реальный источник)
	points := make([]ChartPoint, 50)
	basePrice := 60000.0
	
	for i := range points {
		points[i] = ChartPoint{
			Timestamp: time.Now().Add(-time.Duration(50-i) * time.Minute).Unix(),
			Price:     basePrice + (rand.Float64()-0.5)*1000,
			Volume:    rand.Float64() * 100000,
		}
	}
	
	data := ChartData{
		Symbol:     "BKC/USDT",
		Points:     points,
		LastUpdate: time.Now(),
		Price:      points[len(points)-1].Price,
		Change24h:  (points[len(points)-1].Price - points[0].Price) / points[0].Price * 100,
	}
	
	message := WebSocketMessage{
		Type:      "chart_update",
		Data:      data,
		Timestamp: time.Now(),
	}
	
	wse.sendToClient(client, message)
}

// getCurrentCrashGame получает текущую игру Ракетка
func (wse *WebSocketEngine) getCurrentCrashGame() *Game {
	wse.gameMu.RLock()
	defer wse.gameMu.RUnlock()
	
	// Поиск активной игры
	for _, game := range wse.games {
		if game.Type == GameTypeCrash && (game.Status == GameStatusWaiting || game.Status == GameStatusStarting || game.Status == GameStatusActive) {
			return game
		}
	}
	
	// Создание новой игры
	return wse.createNewCrashGame()
}

// createNewCrashGame создает новую игру Ракетка
func (wse *WebSocketEngine) createNewCrashGame() *Game {
	wse.gameMu.Lock()
	defer wse.gameMu.Unlock()
	
	gameID := fmt.Sprintf("crash_%d", time.Now().UnixNano())
	
	// Генерация честной игры
	hash, secret := wse.fairGenerator.GenerateGame()
	
	// Генерация точки взрыва
	crashPoint := wse.generateCrashPoint(hash)
	
	game := &Game{
		ID:          gameID,
		Type:        GameTypeCrash,
		Status:      GameStatusWaiting,
		Players:     make(map[int64]*Player),
		StartedAt:   time.Now(),
		CrashPoint:  crashPoint,
		CurrentMult: 1.00,
		Hash:        hash,
		Secret:      secret,
	}
	
	wse.games[gameID] = game
	
	// Запуск игры через 3 секунды
	go func() {
		time.Sleep(3 * time.Second)
		wse.startCrashGame(game)
	}()
	
	return game
}

// generateCrashPoint генерирует точку взрыва
func (wse *WebSocketEngine) generateCrashPoint(hash string) float64 {
	// Использование хэша для генерации честной точки взрыва
	h := sha256.Sum256([]byte(hash))
	
	// Преобразование хэша в число от 0 до 1
	hashFloat := float64(h[0]) / 255.0
	
	// House edge - 3% игр взрываются на 1.00x
	if hashFloat < wse.config.CrashGameSettings.HouseEdge {
		return 1.00
	}
	
	// Генерация множителя от 1.01 до 100
	multiplier := 1.01 + (hashFloat * 98.99)
	
	// Ограничение максимального множителя
	if multiplier > wse.config.CrashGameSettings.MaxMultiplier {
		multiplier = wse.config.CrashGameSettings.MaxMultiplier
	}
	
	// Округление до 2 знаков
	return float64(int(multiplier*100)) / 100
}

// startCrashGame запускает игру Ракетка
func (wse *WebSocketEngine) startCrashGame(game *Game) {
	wse.gameMu.Lock()
	game.Status = GameStatusActive
	game.StartedAt = time.Now()
	wse.gameMu.Unlock()
	
	atomic.AddInt64(&wse.metrics.TotalGames, 1)
	
	// Основной цикл игры
	ticker := time.NewTicker(wse.config.CrashGameSettings.UpdateInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-wse.ctx.Done():
			return
		case <-ticker.C:
			game.mu.Lock()
			
			// Увеличение множителя
			game.CurrentMult += wse.config.CrashGameSettings.GrowthRate
			
			// Проверка взрыва
			if game.CurrentMult >= game.CrashPoint {
				game.Status = GameStatusCrashed
				game.EndedAt = time.Now()
				game.mu.Unlock()
				
				// Обработка взрыва
				wse.handleCrash(game)
				return
			}
			
			game.mu.Unlock()
			
			// Отправка обновлений клиентам
			wse.broadcastCrashUpdate(game)
		}
	}
}

// handleCrash обрабатывает взрыв игры
func (wse *WebSocketEngine) handleCrash(game *Game) {
	game.mu.Lock()
	defer game.mu.Unlock()
	
	// Расчет выигрышей и проигрышей
	for _, player := range game.Players {
		if !player.CashedOut {
			// Игрок не вывел деньги - проиграл
			player.WinAmount = 0
		}
	}
	
	// Отправка финального обновления
	wse.broadcastCrashUpdate(game)
	
	// Создание новой игры через 5 секунд
	go func() {
		time.Sleep(5 * time.Second)
		wse.createNewCrashGame()
	}()
}

// broadcastCrashUpdate рассылает обновление игры Ракетка
func (wse *WebSocketEngine) broadcastCrashUpdate(game *Game) {
	data := CrashGameData{
		GameID:      game.ID,
		Multiplier:  game.CurrentMult,
		Status:      game.Status,
		CrashPoint:  game.CrashPoint,
		PlayerCount: len(game.Players),
		TotalBets:   game.TotalBets,
		Hash:        game.Hash,
	}
	
	message := WebSocketMessage{
		Type:      "crash_update",
		Data:      data,
		Timestamp: time.Now(),
		GameID:    game.ID,
	}
	
	wse.broadcastToGameType(GameTypeCrash, message)
}

// broadcastToGameType рассылает сообщение всем клиентам определенного типа игры
func (wse *WebSocketEngine) broadcastToGameType(gameType GameType, message WebSocketMessage) {
	wse.mu.RLock()
	defer wse.mu.RUnlock()
	
	for client := range wse.clients {
		if client.GameType == gameType {
			wse.sendToClient(client, message)
		}
	}
}

// sendToClient отправляет сообщение клиенту
func (wse *WebSocketEngine) sendToClient(client *Client, message WebSocketMessage) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}
	
	select {
	case client.Send <- data:
	default:
		// Канал переполнен
		wse.removeClient(client)
	}
}

// pinger проверяет соединения с клиентами
func (wse *WebSocketEngine) pinger() {
	ticker := time.NewTicker(wse.config.PingPeriod)
	defer ticker.Stop()
	
	for {
		select {
		case <-wse.ctx.Done():
			return
		case <-ticker.C:
			wse.pingClients()
		}
	}
}

// pingClients отправляет пинг клиентам
func (wse *WebSocketEngine) pingClients() {
	wse.mu.RLock()
	clients := make([]*Client, 0, len(wse.clients))
	for client := range wse.clients {
		clients = append(clients, client)
	}
	wse.mu.RUnlock()
	
	for _, client := range clients {
		if time.Since(client.LastPing) > wse.config.PongWait {
			wse.removeClient(client)
			continue
		}
		
		if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			wse.removeClient(client)
		}
	}
}

// gameHandler обрабатывает игровые события
func (wse *WebSocketEngine) gameHandler() {
	// Здесь будет логика обработки игровых событий
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-wse.ctx.Done():
			return
		case <-ticker.C:
			// Обновление метрик
			wse.updateMetrics()
		}
	}
}

// chartUpdater обновляет данные графика
func (wse *WebSocketEngine) chartUpdater() {
	ticker := time.NewTicker(wse.config.ChartSettings.UpdateInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-wse.ctx.Done():
			return
		case <-ticker.C:
			wse.updateChart()
		}
	}
}

// updateChart обновляет данные графика
func (wse *WebSocketEngine) updateChart() {
	// Генерация новой точки графика
	newPoint := ChartPoint{
		Timestamp: time.Now().Unix(),
		Price:     60000 + (rand.Float64()-0.5)*1000,
		Volume:    rand.Float64() * 100000,
	}
	
	data := ChartData{
		Symbol:     "BKC/USDT",
		Points:     []ChartPoint{newPoint},
		LastUpdate: time.Now(),
		Price:      newPoint.Price,
		Change24h:  (rand.Float64() - 0.5) * 10,
	}
	
	message := WebSocketMessage{
		Type:      "chart_update",
		Data:      data,
		Timestamp: time.Now(),
	}
	
	wse.broadcastToGameType(GameTypeChart, message)
}

// updateMetrics обновляет метрики
func (wse *WebSocketEngine) updateMetrics() {
	wse.metrics.mu.Lock()
	defer wse.metrics.mu.Unlock()
	
	wse.metrics.LastUpdated = time.Now()
}

// GetMetrics возвращает метрики
func (wse *WebSocketEngine) GetMetrics() GameMetrics {
	wse.metrics.mu.RLock()
	defer wse.metrics.mu.RUnlock()
	
	return *wse.metrics
}

// NewProvablyFairGenerator создает новый генератор честной игры
func NewProvablyFairGenerator() *ProvablyFairGenerator {
	return &ProvablyFairGenerator{
		serverSeed: fmt.Sprintf("%d", time.Now().UnixNano()),
		nonce:      0,
	}
}

// GenerateGame генерирует честную игру
func (pfg *ProvablyFairGenerator) GenerateGame() (hash, secret string) {
	pfg.mu.Lock()
	defer pfg.mu.Unlock()
	
	// Генерация секретного числа
	secret = fmt.Sprintf("%d_%d", time.Now().UnixNano(), pfg.nonce)
	pfg.nonce++
	
	// Создание хэша
	h := sha256.Sum256([]byte(secret + pfg.serverSeed))
	hash = fmt.Sprintf("%x", h)
	
	return hash, secret
}

// Методы клиента
func (c *Client) readPump(wse *WebSocketEngine) {
	defer func() {
		wse.removeClient(c)
		c.Conn.Close()
	}()
	
	c.Conn.SetReadLimit(wse.config.MaxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(wse.config.PongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.LastPing = time.Now()
		c.Conn.SetReadDeadline(time.Now().Add(wse.config.PongWait))
		return nil
	})
	
	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
		
		// Обработка сообщения от клиента
		wse.handleClientMessage(c, message)
	}
}

func (c *Client) writePump(wse *WebSocketEngine) {
	ticker := time.NewTicker(wse.config.PingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()
	
	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(wse.config.WriteWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
			
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(wse.config.WriteWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (wse *WebSocketEngine) handleClientMessage(client *Client, message []byte) {
	var msg WebSocketMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Printf("Failed to unmarshal client message: %v", err)
		return
	}
	
	switch msg.Type {
	case "place_bet":
		wse.handlePlaceBet(client, msg.Data)
	case "cash_out":
		wse.handleCashOut(client, msg.Data)
	case "ping":
		// Ответ на пинг
		response := WebSocketMessage{
			Type:      "pong",
			Timestamp: time.Now(),
		}
		wse.sendToClient(client, response)
	}
}

func (wse *WebSocketEngine) handlePlaceBet(client *Client, data interface{}) {
	// Логика обработки ставки
	// TODO: Реализовать логику ставок
}

func (wse *WebSocketEngine) handleCashOut(client *Client, data interface{}) {
	// Логика обработки вывода средств
	// TODO: Реализовать логику вывода
}
