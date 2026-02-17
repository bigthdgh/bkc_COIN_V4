package antiabuse

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"bkc_coin_v2/internal/db"
)

// AntiAbuseManager управляет защитой от читов и злоупотреблений
type AntiAbuseManager struct {
	db            *db.DB
	rateLimiter   *RateLimiter
	cheatDetector *CheatDetector
	ipTracker     *IPTracker
	deviceTracker *DeviceTracker
}

// NewAntiAbuseManager создает новый менеджер анти-abuse
func NewAntiAbuseManager(database *db.DB) *AntiAbuseManager {
	return &AntiAbuseManager{
		db:            database,
		rateLimiter:   NewRateLimiter(),
		cheatDetector: NewCheatDetector(),
		ipTracker:     NewIPTracker(),
		deviceTracker: NewDeviceTracker(),
	}
}

// RateLimiter ограничивает частоту запросов
type RateLimiter struct {
	requests map[string][]time.Time // key -> timestamps
	mu       sync.RWMutex
}

// CheatDetector детектирует подозрительную активность
type CheatDetector struct {
	userStats map[int64]*UserStats
	mu        sync.RWMutex
	alerts    []CheatAlert
}

// IPTracker отслеживает IP адреса
type IPTracker struct {
	ipToUsers map[string][]int64 // IP -> userIDs
	userToIPs map[int64][]string // userID -> IPs
	mu        sync.RWMutex
}

// DeviceTracker отслеживает устройства
type DeviceTracker struct {
	deviceToUsers map[string][]int64 // deviceID -> userIDs
	userToDevices map[int64][]string // userID -> deviceIDs
	mu            sync.RWMutex
}

// UserStats статистика пользователя для детекции читов
type UserStats struct {
	UserID          int64
	TapsPerSecond   float64
	TapsPerMinute   float64
	TapsPerHour     float64
	AvgTapInterval  time.Duration
	MaxTapInterval  time.Duration
	MinTapInterval  time.Duration
	LastTapTime     time.Time
	TotalTaps       int64
	SessionStart    time.Time
	SuspiciousScore float64
}

// CheatAlert оповещение о подозрительной активности
type CheatAlert struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	AlertType   string     `json:"alert_type"` // rapid_taps, impossible_speed, multi_account, bot_behavior
	Severity    string     `json:"severity"`   // low, medium, high, critical
	Description string     `json:"description"`
	IP          string     `json:"ip"`
	DeviceID    string     `json:"device_id"`
	Metadata    string     `json:"metadata"`
	CreatedAt   time.Time  `json:"created_at"`
	IsResolved  bool       `json:"is_resolved"`
	ResolvedAt  *time.Time `json:"resolved_at"`
	ResolvedBy  *int64     `json:"resolved_by"`
}

// AbuseReport репорт о злоупотреблении
type AbuseReport struct {
	ID          int64     `json:"id"`
	ReporterID  int64     `json:"reporter_id"`
	TargetID    int64     `json:"target_id"`
	Reason      string    `json:"reason"` // cheating, harassment, spam, multi_account
	Description string    `json:"description"`
	Evidence    string    `json:"evidence"`
	Status      string    `json:"status"` // pending, reviewing, resolved, dismissed
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// NewRateLimiter создает новый rate limiter
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
	}
}

// NewCheatDetector создает новый детектор читов
func NewCheatDetector() *CheatDetector {
	return &CheatDetector{
		userStats: make(map[int64]*UserStats),
		alerts:    make([]CheatAlert, 0),
	}
}

// NewIPTracker создает новый IP трекер
func NewIPTracker() *IPTracker {
	return &IPTracker{
		ipToUsers: make(map[string][]int64),
		userToIPs: make(map[int64][]string),
	}
}

// NewDeviceTracker создает новый device трекер
func NewDeviceTracker() *DeviceTracker {
	return &DeviceTracker{
		deviceToUsers: make(map[string][]int64),
		userToDevices: make(map[int64][]string),
	}
}

// CheckRateLimit проверяет ограничение частоты запросов
func (aam *AntiAbuseManager) CheckRateLimit(ctx context.Context, userID int64, action string, limit int, window time.Duration) bool {
	key := fmt.Sprintf("%d_%s", userID, action)

	aam.rateLimiter.mu.Lock()
	defer aam.rateLimiter.mu.Unlock()

	now := time.Now()
	timestamps, exists := aam.rateLimiter.requests[key]

	if !exists {
		aam.rateLimiter.requests[key] = []time.Time{now}
		return true
	}

	// Удаляем старые запросы
	var validTimestamps []time.Time
	for _, ts := range timestamps {
		if now.Sub(ts) <= window {
			validTimestamps = append(validTimestamps, ts)
		}
	}

	// Проверяем лимит
	if len(validTimestamps) >= limit {
		return false
	}

	// Добавляем новый запрос
	validTimestamps = append(validTimestamps, now)
	aam.rateLimiter.requests[key] = validTimestamps

	return true
}

// RecordTap записывает тап для анализа
func (aam *AntiAbuseManager) RecordTap(ctx context.Context, userID int64, ip string, deviceID string) error {
	// Обновляем статистику пользователя
	aam.updateUserStats(userID)

	// Отслеживаем IP и устройство
	aam.ipTracker.TrackIP(userID, ip)
	aam.deviceTracker.TrackDevice(userID, deviceID)

	// Проверяем на подозрительную активность
	alert := aam.cheatDetector.AnalyzeTap(userID, ip, deviceID)
	if alert != nil {
		aam.saveCheatAlert(ctx, alert)
	}

	// Проверяем мультиаккаунты
	multiAccountAlert := aam.detectMultiAccount(userID, ip, deviceID)
	if multiAccountAlert != nil {
		aam.saveCheatAlert(ctx, multiAccountAlert)
	}

	return nil
}

// updateUserStats обновляет статистику пользователя
func (aam *AntiAbuseManager) updateUserStats(userID int64) {
	aam.cheatDetector.mu.Lock()
	defer aam.cheatDetector.mu.Unlock()

	now := time.Now()
	stats, exists := aam.cheatDetector.userStats[userID]

	if !exists {
		stats = &UserStats{
			UserID:         userID,
			SessionStart:   now,
			LastTapTime:    now,
			MinTapInterval: time.Hour, // начальное значение
		}
		aam.cheatDetector.userStats[userID] = stats
	}

	// Обновляем интервалы
	if !stats.LastTapTime.IsZero() {
		interval := now.Sub(stats.LastTapTime)

		if stats.MinTapInterval == 0 || interval < stats.MinTapInterval {
			stats.MinTapInterval = interval
		}
		if interval > stats.MaxTapInterval {
			stats.MaxTapInterval = interval
		}

		// Рассчитываем средний интервал
		sessionDuration := now.Sub(stats.SessionStart)
		if sessionDuration > 0 && stats.TotalTaps > 0 {
			stats.AvgTapInterval = sessionDuration / time.Duration(stats.TotalTaps)
		}
	}

	stats.LastTapTime = now
	stats.TotalTaps++

	// Рассчитываем тапы в секунду/минуту/час
	sessionDuration := now.Sub(stats.SessionStart)
	if sessionDuration > 0 {
		seconds := sessionDuration.Seconds()
		stats.TapsPerSecond = float64(stats.TotalTaps) / seconds
		stats.TapsPerMinute = stats.TapsPerSecond * 60
		stats.TapsPerHour = stats.TapsPerMinute * 60
	}
}

// AnalyzeTap анализирует тап на предмет читерства
func (cd *CheatDetector) AnalyzeTap(userID int64, ip string, deviceID string) *CheatAlert {
	cd.mu.RLock()
	stats, exists := cd.userStats[userID]
	cd.mu.RUnlock()

	if !exists {
		return nil
	}

	// Проверка на невозможную скорость тапов
	if stats.TapsPerSecond > 20 { // больше 20 тапов в секунду
		return &CheatAlert{
			UserID:      userID,
			AlertType:   "impossible_speed",
			Severity:    "high",
			Description: fmt.Sprintf("Impossible tap speed: %.2f taps/sec", stats.TapsPerSecond),
			IP:          ip,
			DeviceID:    deviceID,
			CreatedAt:   time.Now(),
		}
	}

	// Проверка на ботов (слишком постоянные интервалы)
	if stats.TotalTaps > 100 && stats.MinTapInterval > 0 && stats.MaxTapInterval > 0 {
		variance := stats.MaxTapInterval - stats.MinTapInterval
		avgInterval := stats.AvgTapInterval

		if avgInterval > 0 {
			coefficient := float64(variance) / float64(avgInterval)
			if coefficient < 0.1 { // слишком маленькая вариация
				return &CheatAlert{
					UserID:      userID,
					AlertType:   "bot_behavior",
					Severity:    "medium",
					Description: fmt.Sprintf("Bot-like behavior detected: variance coefficient %.3f", coefficient),
					IP:          ip,
					DeviceID:    deviceID,
					CreatedAt:   time.Now(),
				}
			}
		}
	}

	// Проверка на слишком быструю серию тапов
	if stats.TapsPerMinute > 1000 { // больше 1000 тапов в минуту
		return &CheatAlert{
			UserID:      userID,
			AlertType:   "rapid_taps",
			Severity:    "medium",
			Description: fmt.Sprintf("Rapid tapping detected: %.2f taps/min", stats.TapsPerMinute),
			IP:          ip,
			DeviceID:    deviceID,
			CreatedAt:   time.Now(),
		}
	}

	return nil
}

// TrackIP отслеживает IP адрес пользователя
func (it *IPTracker) TrackIP(userID int64, ip string) {
	it.mu.Lock()
	defer it.mu.Unlock()

	// Добавляем IP пользователю
	if it.userToIPs[userID] == nil {
		it.userToIPs[userID] = make([]string, 0)
	}

	// Проверяем, не добавлен ли уже этот IP
	found := false
	for _, existingIP := range it.userToIPs[userID] {
		if existingIP == ip {
			found = true
			break
		}
	}

	if !found {
		it.userToIPs[userID] = append(it.userToIPs[userID], ip)
	}

	// Добавляем пользователя к IP
	if it.ipToUsers[ip] == nil {
		it.ipToUsers[ip] = make([]int64, 0)
	}

	found = false
	for _, existingUserID := range it.ipToUsers[ip] {
		if existingUserID == userID {
			found = true
			break
		}
	}

	if !found {
		it.ipToUsers[ip] = append(it.ipToUsers[ip], userID)
	}
}

// TrackDevice отслеживает устройство пользователя
func (dt *DeviceTracker) TrackDevice(userID int64, deviceID string) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	// Добавляем устройство пользователю
	if dt.userToDevices[userID] == nil {
		dt.userToDevices[userID] = make([]string, 0)
	}

	found := false
	for _, existingDevice := range dt.userToDevices[userID] {
		if existingDevice == deviceID {
			found = true
			break
		}
	}

	if !found {
		dt.userToDevices[userID] = append(dt.userToDevices[userID], deviceID)
	}

	// Добавляем пользователя к устройству
	if dt.deviceToUsers[deviceID] == nil {
		dt.deviceToUsers[deviceID] = make([]int64, 0)
	}

	found = false
	for _, existingUserID := range dt.deviceToUsers[deviceID] {
		if existingUserID == userID {
			found = true
			break
		}
	}

	if !found {
		dt.deviceToUsers[deviceID] = append(dt.deviceToUsers[deviceID], userID)
	}
}

// detectMultiAccount детектирует мультиаккаунты
func (aam *AntiAbuseManager) detectMultiAccount(userID int64, ip string, deviceID string) *CheatAlert {
	// Проверяем IP
	aam.ipTracker.mu.RLock()
	usersWithSameIP := aam.ipTracker.ipToUsers[ip]
	aam.ipTracker.mu.RUnlock()

	if len(usersWithSameIP) > 3 { // больше 3 аккаунтов с одного IP
		return &CheatAlert{
			UserID:      userID,
			AlertType:   "multi_account",
			Severity:    "medium",
			Description: fmt.Sprintf("Multiple accounts from same IP: %d accounts", len(usersWithSameIP)),
			IP:          ip,
			DeviceID:    deviceID,
			CreatedAt:   time.Now(),
		}
	}

	// Проверяем устройство
	aam.deviceTracker.mu.RLock()
	usersWithSameDevice := aam.deviceTracker.deviceToUsers[deviceID]
	aam.deviceTracker.mu.RUnlock()

	if len(usersWithSameDevice) > 2 { // больше 2 аккаунтов на одном устройстве
		return &CheatAlert{
			UserID:      userID,
			AlertType:   "multi_account",
			Severity:    "high",
			Description: fmt.Sprintf("Multiple accounts on same device: %d accounts", len(usersWithSameDevice)),
			IP:          ip,
			DeviceID:    deviceID,
			CreatedAt:   time.Now(),
		}
	}

	return nil
}

// saveCheatAlert сохраняет оповещение о читерстве
func (aam *AntiAbuseManager) saveCheatAlert(ctx context.Context, alert *CheatAlert) error {
	var alertID int64
	err := aam.db.Pool.QueryRow(ctx, `
		INSERT INTO cheat_alerts(
			user_id, alert_type, severity, description, ip, device_id, metadata, created_at
		) VALUES($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, alert.UserID, alert.AlertType, alert.Severity, alert.Description,
		alert.IP, alert.DeviceID, alert.Metadata, alert.CreatedAt).Scan(&alertID)

	if err != nil {
		return fmt.Errorf("failed to save cheat alert: %w", err)
	}

	alert.ID = alertID

	// Добавляем в локальный кэш
	aam.cheatDetector.mu.Lock()
	aam.cheatDetector.alerts = append(aam.cheatDetector.alerts, *alert)
	aam.cheatDetector.mu.Unlock()

	log.Printf("Cheat alert created: ID %d, User %d, Type %s, Severity %s",
		alertID, alert.UserID, alert.AlertType, alert.Severity)

	return nil
}

// CreateAbuseReport создает репорт о злоупотреблении
func (aam *AntiAbuseManager) CreateAbuseReport(ctx context.Context, reporterID, targetID int64, reason, description, evidence string) error {
	var reportID int64
	err := aam.db.Pool.QueryRow(ctx, `
		INSERT INTO abuse_reports(
			reporter_id, target_id, reason, description, evidence, status, created_at, updated_at
		) VALUES($1, $2, $3, $4, $5, 'pending', $6, $6)
		RETURNING id
	`, reporterID, targetID, reason, description, evidence, time.Now()).Scan(&reportID)

	if err != nil {
		return fmt.Errorf("failed to create abuse report: %w", err)
	}

	log.Printf("Abuse report created: ID %d, Reporter %d, Target %d, Reason %s",
		reportID, reporterID, targetID, reason)

	return nil
}

// GetAbuseReports получает репорты о злоупотреблениях
func (aam *AntiAbuseManager) GetAbuseReports(ctx context.Context, status string, limit, offset int) ([]AbuseReport, error) {
	query := `
		SELECT id, reporter_id, target_id, reason, description, evidence, 
		       status, created_at, updated_at
		FROM abuse_reports
	`
	args := []interface{}{}

	if status != "" {
		query += " WHERE status = $1"
		args = append(args, status)
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
		args = append(args, limit, offset)
	}

	rows, err := aam.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get abuse reports: %w", err)
	}
	defer rows.Close()

	var reports []AbuseReport
	for rows.Next() {
		var report AbuseReport
		err := rows.Scan(
			&report.ID, &report.ReporterID, &report.TargetID,
			&report.Reason, &report.Description, &report.Evidence,
			&report.Status, &report.CreatedAt, &report.UpdatedAt,
		)
		if err != nil {
			continue
		}
		reports = append(reports, report)
	}

	return reports, rows.Err()
}

// GetCheatAlerts получает оповещения о читерстве
func (aam *AntiAbuseManager) GetCheatAlerts(ctx context.Context, severity string, limit, offset int) ([]CheatAlert, error) {
	query := `
		SELECT id, user_id, alert_type, severity, description, ip, device_id, 
		       metadata, created_at, is_resolved, resolved_at, resolved_by
		FROM cheat_alerts
	`
	args := []interface{}{}

	if severity != "" {
		query += " WHERE severity = $1"
		args = append(args, severity)
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
		args = append(args, limit, offset)
	}

	rows, err := aam.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get cheat alerts: %w", err)
	}
	defer rows.Close()

	var alerts []CheatAlert
	for rows.Next() {
		var alert CheatAlert
		err := rows.Scan(
			&alert.ID, &alert.UserID, &alert.AlertType, &alert.Severity,
			&alert.Description, &alert.IP, &alert.DeviceID, &alert.Metadata,
			&alert.CreatedAt, &alert.IsResolved, &alert.ResolvedAt, &alert.ResolvedBy,
		)
		if err != nil {
			continue
		}
		alerts = append(alerts, alert)
	}

	return alerts, rows.Err()
}

// ResolveCheatAlert решает проблему с читерством
func (aam *AntiAbuseManager) ResolveCheatAlert(ctx context.Context, alertID, resolvedBy int64) error {
	_, err := aam.db.Pool.Exec(ctx, `
		UPDATE cheat_alerts 
		SET is_resolved = true, resolved_at = $1, resolved_by = $2
		WHERE id = $3
	`, time.Now(), resolvedBy, alertID)

	if err != nil {
		return fmt.Errorf("failed to resolve cheat alert: %w", err)
	}

	log.Printf("Cheat alert %d resolved by admin %d", alertID, resolvedBy)

	return nil
}

// GetAntiAbuseStats получает статистику анти-abuse
func (aam *AntiAbuseManager) GetAntiAbuseStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Статистика алертов
	var totalAlerts, criticalAlerts, highAlerts, mediumAlerts, lowAlerts, resolvedAlerts int64
	aam.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM cheat_alerts").Scan(&totalAlerts)
	aam.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM cheat_alerts WHERE severity = 'critical'").Scan(&criticalAlerts)
	aam.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM cheat_alerts WHERE severity = 'high'").Scan(&highAlerts)
	aam.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM cheat_alerts WHERE severity = 'medium'").Scan(&mediumAlerts)
	aam.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM cheat_alerts WHERE severity = 'low'").Scan(&lowAlerts)
	aam.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM cheat_alerts WHERE is_resolved = true").Scan(&resolvedAlerts)

	// Статистика репортов
	var totalReports, pendingReports, resolvedReports int64
	aam.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM abuse_reports").Scan(&totalReports)
	aam.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM abuse_reports WHERE status = 'pending'").Scan(&pendingReports)
	aam.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM abuse_reports WHERE status = 'resolved'").Scan(&resolvedReports)

	// Алерты за сегодня
	today := time.Now().Truncate(24 * time.Hour)
	var alertsToday int64
	aam.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM cheat_alerts WHERE created_at >= $1", today).Scan(&alertsToday)

	// Уникальные IP и устройства
	aam.ipTracker.mu.RLock()
	uniqueIPs := len(aam.ipTracker.ipToUsers)
	aam.ipTracker.mu.RUnlock()

	aam.deviceTracker.mu.RLock()
	uniqueDevices := len(aam.deviceTracker.deviceToUsers)
	aam.deviceTracker.mu.RUnlock()

	stats["total_alerts"] = totalAlerts
	stats["critical_alerts"] = criticalAlerts
	stats["high_alerts"] = highAlerts
	stats["medium_alerts"] = mediumAlerts
	stats["low_alerts"] = lowAlerts
	stats["resolved_alerts"] = resolvedAlerts
	stats["total_reports"] = totalReports
	stats["pending_reports"] = pendingReports
	stats["resolved_reports"] = resolvedReports
	stats["alerts_today"] = alertsToday
	stats["unique_ips"] = uniqueIPs
	stats["unique_devices"] = uniqueDevices
	stats["resolution_rate"] = float64(resolvedAlerts) / float64(totalAlerts) * 100

	return stats, nil
}

// CleanupOldData очищает старые данные
func (aam *AntiAbuseManager) CleanupOldData(ctx context.Context) error {
	// Удаляем старые алерты (старше 30 дней)
	cutoffDate := time.Now().AddDate(0, 0, -30)

	result, err := aam.db.Pool.Exec(ctx, "DELETE FROM cheat_alerts WHERE created_at < $1 AND is_resolved = true", cutoffDate)
	if err != nil {
		return fmt.Errorf("failed to cleanup old cheat alerts: %w", err)
	}

	deletedAlerts := result.RowsAffected()
	log.Printf("Cleaned up %d old cheat alerts", deletedAlerts)

	// Очищаем rate limiter
	aam.cleanupRateLimiter()

	// Очищаем статистику пользователей старше 1 часа
	aam.cleanupUserStats()

	return nil
}

// cleanupRateLimiter очищает rate limiter
func (aam *AntiAbuseManager) cleanupRateLimiter() {
	aam.rateLimiter.mu.Lock()
	defer aam.rateLimiter.mu.Unlock()

	now := time.Now()
	for key, timestamps := range aam.rateLimiter.requests {
		var validTimestamps []time.Time
		for _, ts := range timestamps {
			if now.Sub(ts) <= time.Hour { // оставляем только за последний час
				validTimestamps = append(validTimestamps, ts)
			}
		}

		if len(validTimestamps) == 0 {
			delete(aam.rateLimiter.requests, key)
		} else {
			aam.rateLimiter.requests[key] = validTimestamps
		}
	}
}

// cleanupUserStats очищает старую статистику пользователей
func (aam *AntiAbuseManager) cleanupUserStats() {
	aam.cheatDetector.mu.Lock()
	defer aam.cheatDetector.mu.Unlock()

	now := time.Now()
	for userID, stats := range aam.cheatDetector.userStats {
		if now.Sub(stats.LastTapTime) > time.Hour {
			delete(aam.cheatDetector.userStats, userID)
		}
	}
}
