package i18n

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Language поддерживаемые языки
type Language string

const (
	Russian Language = "ru"
	English Language = "en"
)

// LocaleManager управляет переводами
type LocaleManager struct {
	mu       sync.RWMutex
	messages map[Language]map[string]string
	defaultLang Language
}

// NewLocaleManager создает новый менеджер локализации
func NewLocaleManager() *LocaleManager {
	lm := &LocaleManager{
		messages:    make(map[Language]map[string]string),
		defaultLang: Russian,
	}
	
	// Загружаем переводы по умолчанию
	lm.loadDefaultMessages()
	
	return lm
}

// loadDefaultMessages загружает переводы по умолчанию
func (lm *LocaleManager) loadDefaultMessages() {
	// Русские переводы
	ruMessages := map[string]string{
		// Общие
		"app_name": "BKC Coin",
		"loading": "Загрузка...",
		"error": "Ошибка",
		"success": "Успешно",
		"cancel": "Отмена",
		"confirm": "Подтвердить",
		"save": "Сохранить",
		"back": "Назад",
		"next": "Далее",
		"close": "Закрыть",
		
		// Главное меню
		"tap_earn": "Тапать и зарабатывать",
		"balance": "Баланс",
		"energy": "Энергия",
		"earn_per_tap": "Заработок за тап",
		"level": "Уровень",
		"referrals": "Рефералы",
		"tasks": "Задания",
		"shop": "Магазин",
		"bank": "Банк",
		"market": "Барахолка",
		"games": "Игры",
		"stats": "Статистика",
		
		// Тапалка
		"tap_to_earn": "Тапай чтобы зарабатывать",
		"energy_full": "Энергия полная",
		"energy_recharging": "Энергия восстанавливается",
		"daily_limit_reached": "Дневной лимит достигнут",
		"boost_available": "Буст доступен",
		"multitap_enabled": "Мультитап включен",
		
		// Рефералы
		"invite_friends": "Пригласи друзей",
		"referral_link": "Реферальная ссылка",
	 "referral_reward": "Награда за реферала",
		"referrals_count": "Количество рефералов",
		"referral_bonus": "Бонус: %d BKC за каждые 3 реферала",
		
		// Магазин
		"buy_energy": "Купить энергию",
		"buy_taps": "Купить тапы",
		"nft_shop": "NFT магазин",
		"premium": "Премиум",
		"subscription_basic": "Basic",
		"subscription_silver": "Silver", 
		"subscription_gold": "Gold",
		"subscription_basic_desc": "Бесплатно\nЛимит тапов: 5,000\nНалог: 10%",
		"subscription_silver_desc": "50,000 BKC/мес\nЛимит тапов: 15,000\nРанняя барахолка\nНалог: 5%",
		"subscription_gold_desc": "200,000 BKC/мес\nЛимит тапов: 50,000\nГрафик в реальном времени\nНалог: 2%",
		
		// Банк
		"take_loan": "Взять кредит",
		"loan_7_days": "Кредит на 7 дней",
		"loan_30_days": "Кредит на 30 дней",
		"interest_rate": "Ставка",
		"max_loan_amount": "Максимальная сумма",
		"loan_active": "Кредит активен",
		"loan_overdue": "Просрочен",
		"repay_loan": "Погасить кредит",
		
		// Барахолка
		"marketplace": "Барахолка",
		"create_listing": "Создать объявление",
		"my_listings": "Мои объявления",
		"digital_items": "Цифровые товары",
		"physical_items": "Физические товары",
		"fiat_items": "Фиатные товары",
		"listing_fee": "Комиссия за размещение",
		"contact_seller": "Связаться с продавцом",
		"verified_seller": "Проверенный продавец",
		"seller_rating": "Рейтинг продавца",
		"escrow_protected": "Защита Escrow",
		
		// Игры
		"crash_game": "Ракетка",
		"place_bet": "Сделать ставку",
		"cash_out": "Забрать",
		"multiplier": "Множитель",
		"crashed_at": "Взрыв на",
		"provably_fair": "Честная игра",
		"game_history": "История игр",
		
		// График
		"price_chart": "График цены",
		"current_price": "Текущая цена",
		"24h_change": "Изменение за 24ч",
		"market_cap": "Капитализация",
		"buy_bkc": "Купить BKC",
		"sell_bkc": "Продать BKC",
		"p2p_market": "P2P биржа",
		
		// P2P долги
		"p2p_loans": "P2P долги",
		"lend_money": "Дать в долг",
		"borrow_money": "Взять в долг",
		"collateral": "Залог",
		"interest": "Процент",
		"loan_term": "Срок займа",
		"active_loans": "Активные займы",
		"loan_requests": "Заявки на займ",
		
		// NFT привилегии
		"nft_privileges": "NFT привилегии",
		"magnat_nft": "Магнат NFT",
		"sheikh_nft": "Шейх NFT", 
		"hacker_nft": "Хакер NFT",
		"magnat_desc": "Убирает комиссию на переводы друзьям",
		"sheikh_desc": "+500% к реферальным отчислениям",
		"hacker_desc": "Видит график Ракетки на 0.5 сек быстрее",
		
		// Ошибки
		"error_insufficient_balance": "Недостаточно средств",
		"error_insufficient_energy": "Недостаточно энергии",
		"error_daily_limit": "Дневной лимит исчерпан",
		"error_invalid_amount": "Неверная сумма",
		"error_network": "Ошибка сети",
		"error_server": "Ошибка сервера",
		"error_unauthorized": "Не авторизован",
		"error_forbidden": "Доступ запрещен",
		"error_not_found": "Не найдено",
		"error_already_exists": "Уже существует",
		
		// Успешные сообщения
		"success_tap": "Тап засчитан",
		"success_purchase": "Покупка выполнена",
		"success_loan_taken": "Кредит получен",
		"success_loan_repaid": "Кредит погашен",
		"success_listing_created": "Объявление создано",
		"success_bet_placed": "Ставка сделана",
		"success_withdrawal": "Вывод выполнен",
	}
	
	// Английские переводы
	enMessages := map[string]string{
		// Общие
		"app_name": "BKC Coin",
		"loading": "Loading...",
		"error": "Error",
		"success": "Success",
		"cancel": "Cancel",
		"confirm": "Confirm",
		"save": "Save",
		"back": "Back",
		"next": "Next",
		"close": "Close",
		
		// Главное меню
		"tap_earn": "Tap & Earn",
		"balance": "Balance",
		"energy": "Energy",
		"earn_per_tap": "Earn per tap",
		"level": "Level",
		"referrals": "Referrals",
		"tasks": "Tasks",
		"shop": "Shop",
		"bank": "Bank",
		"market": "Marketplace",
		"games": "Games",
		"stats": "Statistics",
		
		// Тапалка
		"tap_to_earn": "Tap to earn",
		"energy_full": "Energy full",
		"energy_recharging": "Energy recharging",
		"daily_limit_reached": "Daily limit reached",
		"boost_available": "Boost available",
		"multitap_enabled": "Multitap enabled",
		
		// Рефералы
		"invite_friends": "Invite friends",
		"referral_link": "Referral link",
		"referral_reward": "Referral reward",
		"referrals_count": "Referrals count",
		"referral_bonus": "Bonus: %d BKC for every 3 referrals",
		
		// Магазин
		"buy_energy": "Buy energy",
		"buy_taps": "Buy taps",
		"nft_shop": "NFT shop",
		"premium": "Premium",
		"subscription_basic": "Basic",
		"subscription_silver": "Silver",
		"subscription_gold": "Gold",
		"subscription_basic_desc": "Free\nTap limit: 5,000\nTax: 10%",
		"subscription_silver_desc": "50,000 BKC/month\nTap limit: 15,000\nEarly marketplace\nTax: 5%",
		"subscription_gold_desc": "200,000 BKC/month\nTap limit: 50,000\nReal-time chart\nTax: 2%",
		
		// Банк
		"take_loan": "Take loan",
		"loan_7_days": "7 days loan",
		"loan_30_days": "30 days loan",
		"interest_rate": "Interest rate",
		"max_loan_amount": "Max amount",
		"loan_active": "Loan active",
		"loan_overdue": "Overdue",
		"repay_loan": "Repay loan",
		
		// Барахолка
		"marketplace": "Marketplace",
		"create_listing": "Create listing",
		"my_listings": "My listings",
		"digital_items": "Digital items",
		"physical_items": "Physical items",
		"fiat_items": "Fiat items",
		"listing_fee": "Listing fee",
		"contact_seller": "Contact seller",
		"verified_seller": "Verified seller",
		"seller_rating": "Seller rating",
		"escrow_protected": "Escrow protected",
		
		// Игры
		"crash_game": "Crash Game",
		"place_bet": "Place bet",
		"cash_out": "Cash out",
		"multiplier": "Multiplier",
		"crashed_at": "Crashed at",
		"provably_fair": "Provably Fair",
		"game_history": "Game history",
		
		// График
		"price_chart": "Price Chart",
		"current_price": "Current price",
		"24h_change": "24h change",
		"market_cap": "Market cap",
		"buy_bkc": "Buy BKC",
		"sell_bkc": "Sell BKC",
		"p2p_market": "P2P Market",
		
		// P2P долги
		"p2p_loans": "P2P Loans",
		"lend_money": "Lend money",
		"borrow_money": "Borrow money",
		"collateral": "Collateral",
		"interest": "Interest",
		"loan_term": "Loan term",
		"active_loans": "Active loans",
		"loan_requests": "Loan requests",
		
		// NFT привилегии
		"nft_privileges": "NFT Privileges",
		"magnat_nft": "Magnat NFT",
		"sheikh_nft": "Sheikh NFT",
		"hacker_nft": "Hacker NFT",
		"magnat_desc": "Removes commission on transfers to friends",
		"sheikh_desc": "+500% to referral earnings",
		"hacker_desc": "Sees crash chart 0.5 sec earlier",
		
		// Ошибки
		"error_insufficient_balance": "Insufficient balance",
		"error_insufficient_energy": "Insufficient energy",
		"error_daily_limit": "Daily limit exceeded",
		"error_invalid_amount": "Invalid amount",
		"error_network": "Network error",
		"error_server": "Server error",
		"error_unauthorized": "Unauthorized",
		"error_forbidden": "Forbidden",
		"error_not_found": "Not found",
		"error_already_exists": "Already exists",
		
		// Успешные сообщения
		"success_tap": "Tap counted",
		"success_purchase": "Purchase completed",
		"success_loan_taken": "Loan received",
		"success_loan_repaid": "Loan repaid",
		"success_listing_created": "Listing created",
		"success_bet_placed": "Bet placed",
		"success_withdrawal": "Withdrawal completed",
	}
	
	lm.messages[Russian] = ruMessages
	lm.messages[English] = enMessages
}

// GetMessage получает сообщение для указанного языка
func (lm *LocaleManager) GetMessage(lang Language, key string, args ...interface{}) string {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	
	// Если язык не поддерживается, используем язык по умолчанию
	if _, exists := lm.messages[lang]; !exists {
		lang = lm.defaultLang
	}
	
	// Ищем перевод
	if message, exists := lm.messages[lang][key]; exists {
		if len(args) > 0 {
			return fmt.Sprintf(message, args...)
		}
		return message
	}
	
	// Если перевод не найден, пробуем язык по умолчанию
	if lang != lm.defaultLang {
		if message, exists := lm.messages[lm.defaultLang][key]; exists {
			if len(args) > 0 {
				return fmt.Sprintf(message, args...)
			}
			return message
		}
	}
	
	// Если ничего не найдено, возвращаем ключ
	return key
}

// SetDefaultLanguage устанавливает язык по умолчанию
func (lm *LocaleManager) SetDefaultLanguage(lang Language) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.defaultLang = lang
}

// AddMessages добавляет переводы для языка
func (lm *LocaleManager) AddMessages(lang Language, messages map[string]string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	
	if lm.messages[lang] == nil {
		lm.messages[lang] = make(map[string]string)
	}
	
	for key, value := range messages {
		lm.messages[lang][key] = value
	}
}

// GetSupportedLanguages возвращает поддерживаемые языки
func (lm *LocaleManager) GetSupportedLanguages() []Language {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	
	languages := make([]Language, 0, len(lm.messages))
	for lang := range lm.messages {
		languages = append(languages, lang)
	}
	
	return languages
}

// ToJSON экспортирует все переводы в JSON
func (lm *LocaleManager) ToJSON() ([]byte, error) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	
	return json.MarshalIndent(lm.messages, "", "  ")
}

// DetectLanguage определяет язык из HTTP заголовка или параметра
func DetectLanguage(acceptLanguage string, langParam string) Language {
	// Сначала проверяем явный параметр
	if langParam != "" {
		switch langParam {
		case "ru", "russian":
			return Russian
		case "en", "english":
			return English
		}
	}
	
	// Затем анализируем Accept-Language header
	if acceptLanguage != "" {
		// Простая логика - если начинается с "ru", то русский
		if len(acceptLanguage) >= 2 && acceptLanguage[0:2] == "ru" {
			return Russian
		}
	}
	
	// По умолчанию русский
	return Russian
}

// Глобальный менеджер локализации
var DefaultLocaleManager = NewLocaleManager()

// Удобные функции для получения переводов
func T(lang Language, key string, args ...interface{}) string {
	return DefaultLocaleManager.GetMessage(lang, key, args...)
}

func TR(key string, args ...interface{}) string {
	return DefaultLocaleManager.GetMessage(Russian, key, args...)
}

func TE(key string, args ...interface{}) string {
	return DefaultLocaleManager.GetMessage(English, key, args...)
}
