package tgbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bkc_coin_v2/internal/config"
	"bkc_coin_v2/internal/db"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	Cfg config.Config
	DB  *db.DB
	Bot *tgbotapi.BotAPI
}

func New(cfg config.Config, d *db.DB) (*Bot, error) {
	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, err
	}
	bot.Debug = false
	return &Bot{Cfg: cfg, DB: d, Bot: bot}, nil
}

func (b *Bot) StartPolling(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := b.Bot.GetUpdatesChan(u)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case upd := <-updates:
				b.handleUpdate(ctx, upd)
			}
		}
	}()
}

func (b *Bot) handleUpdate(ctx context.Context, upd tgbotapi.Update) {
	if upd.Message != nil {
		b.handleMessage(ctx, upd.Message)
		return
	}
	if upd.CallbackQuery != nil {
		b.handleCallback(ctx, upd.CallbackQuery)
		return
	}
}

// HandleUpdate is used by webhook mode. It reuses the same logic as polling mode.
func (b *Bot) HandleUpdate(ctx context.Context, upd tgbotapi.Update) {
	b.handleUpdate(ctx, upd)
}

func (b *Bot) SetWebhook(url string) error {
	params := tgbotapi.Params{"url": url}
	_, err := b.Bot.MakeRequest("setWebhook", params)
	return err
}

// StartBroadcast triggers a background broadcast job from the bot.
// adminChatID is used for progress messages.
func (b *Bot) StartBroadcast(ctx context.Context, adminChatID int64, text string) {
	go b.broadcast(ctx, adminChatID, text)
}

func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	if msg.From == nil {
		return
	}
	if !msg.IsCommand() {
		return
	}

	switch msg.Command() {
	case "start":
		payload := strings.TrimSpace(msg.CommandArguments())
		_ = b.onStart(ctx, msg, payload)
	case "reserve_send":
		if int64(msg.From.ID) != b.Cfg.AdminID {
			return
		}
		parts := strings.Fields(msg.CommandArguments())
		if len(parts) != 2 {
			_ = b.sendMessage(msg.Chat.ID, "Формат: /reserve_send <user_id> <amount>", "")
			return
		}
		toID, _ := strconv.ParseInt(parts[0], 10, 64)
		amount, _ := strconv.ParseInt(parts[1], 10, 64)
		if toID <= 0 || amount <= 0 {
			_ = b.sendMessage(msg.Chat.ID, "Неверные параметры", "")
			return
		}
		_ = b.reserveSend(ctx, msg.Chat.ID, toID, amount)
	case "broadcast":
		if int64(msg.From.ID) != b.Cfg.AdminID {
			return
		}
		text := strings.TrimSpace(msg.CommandArguments())
		if text == "" {
			_ = b.sendMessage(msg.Chat.ID, "Формат: /broadcast <текст>", "")
			return
		}
		go b.broadcast(ctx, msg.Chat.ID, text)
	default:
		return
	}
}

func (b *Bot) onStart(ctx context.Context, msg *tgbotapi.Message, payload string) error {
	user := msg.From
	if user == nil {
		return nil
	}

	// Check if new user
	var existed bool
	_ = b.DB.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE user_id=$1)`, int64(user.ID)).Scan(&existed)

	sys, err := b.DB.GetSystem(ctx)
	if err != nil {
		return err
	}

	_, err = b.DB.EnsureUser(ctx, int64(user.ID), user.UserName, user.FirstName, float64(b.Cfg.EnergyMax))
	if err != nil {
		return err
	}

	refID := parseRef(payload)
	if !existed && refID > 0 && refID != int64(user.ID) {
		if _, err := b.DB.GetUser(ctx, refID); err == nil {
			bonus, err := b.DB.RegisterReferral(ctx, refID, int64(user.ID), sys.ReferralStep, sys.ReferralBonus)
			if err == nil {
				note := "👥 Новый реферал!"
				if bonus > 0 {
					note = fmt.Sprintf("👥 Новый реферал! +%d BKC", bonus)
				} else {
					note = "👥 Новый реферал! Каждые 3 приглашенных = +30 000 BKC."
				}
				_ = b.sendMessage(refID, note, "")
			}
		}
	}

	u, _ := b.DB.GetUser(ctx, int64(user.ID))
	rate := coinsPerUSD(sys.ReserveSupply, sys.InitialReserve, sys.StartRateCoinsUSD, sys.MinRateCoinsUSD)
	refLink := fmt.Sprintf("https://t.me/%s?start=%d", b.Bot.Self.UserName, user.ID)

	uname := strings.TrimSpace(user.UserName)
	if uname != "" && !strings.HasPrefix(uname, "@") {
		uname = "@" + uname
	}
	nameLine := strings.TrimSpace(user.FirstName)
	if uname != "" {
		nameLine = fmt.Sprintf("%s %s", nameLine, uname)
	}

	text := fmt.Sprintf(
		"BKC COIN\n\n👤 Игрок: %s\n🆔 ID: %d\n💰 Баланс: %d BKC\n🏷 Адрес: %s\n💱 Курс: %d BKC = $1\n\n👥 Реф-ссылка:\n%s\n\nОткрой ⚡ MINI APP: тап, кошелёк, банк, P2P, барахолка.",
		nameLine,
		int64(user.ID),
		u.Balance,
		fmtAddress(int64(user.ID)),
		rate,
		refLink,
	)

	return b.sendMessage(msg.Chat.ID, text, b.mainKeyboardJSON(int64(user.ID) == b.Cfg.AdminID))
}

func (b *Bot) handleCallback(ctx context.Context, q *tgbotapi.CallbackQuery) {
	_ = b.answerCallback(q.ID)
	user := q.From
	if user == nil || q.Message == nil {
		return
	}

	isAdmin := int64(user.ID) == b.Cfg.AdminID
	kb := b.mainKeyboardJSON(isAdmin)

	switch q.Data {
	case "wallet":
		u, err := b.DB.GetUser(ctx, int64(user.ID))
		if err != nil {
			return
		}
		sys, _ := b.DB.GetSystem(ctx)
		rate := coinsPerUSD(sys.ReserveSupply, sys.InitialReserve, sys.StartRateCoinsUSD, sys.MinRateCoinsUSD)
		text := fmt.Sprintf("💰 Кошелек\n\nБаланс: %d BKC\nАдрес: %s\nКурс: %d BKC = $1", u.Balance, fmtAddress(int64(user.ID)), rate)
		_ = b.editMessageText(q.Message.Chat.ID, q.Message.MessageID, text, kb)
	case "invite":
		refLink := fmt.Sprintf("https://t.me/%s?start=%d", b.Bot.Self.UserName, user.ID)
		text := "👥 Рефералы\n\nТвоя ссылка:\n" + refLink + "\n\nБонус: 30 000 BKC за каждые 3 приглашенных."
		_ = b.editMessageText(q.Message.Chat.ID, q.Message.MessageID, text, kb)
	case "store":
		text := fmt.Sprintf("🛒 Магазин\n\n• Energy 1h: %d BKC\n• CryptoBot пополнение (USD)\n• Пополнение по TX hash (админ подтверждает)\n• NFT магазин\n• Банк: кредиты 7/30 дней\n• Барахолка: объявления + фото\n\nВсе покупки и функции внутри ⚡ MINI APP.", b.Cfg.EnergyBoost1HPriceCoins)
		_ = b.editMessageText(q.Message.Chat.ID, q.Message.MessageID, text, kb)
	case "admin":
		if !isAdmin {
			return
		}
		text := "👑 Админ\n\n/reserve_send <user_id> <amount>\n/broadcast <text>"
		_ = b.editMessageText(q.Message.Chat.ID, q.Message.MessageID, text, kb)
	default:
		return
	}
}

func (b *Bot) broadcast(ctx context.Context, adminChatID int64, text string) {
	ids, err := b.DB.ListUserIDs(ctx)
	if err != nil {
		_ = b.sendMessage(adminChatID, "Ошибка БД (users)", "")
		return
	}
	if len(ids) == 0 {
		_ = b.sendMessage(adminChatID, "Нет пользователей для рассылки.", "")
		return
	}

	_ = b.sendMessage(adminChatID, fmt.Sprintf("Рассылка запущена. Пользователей: %d", len(ids)), "")

	ticker := time.NewTicker(60 * time.Millisecond) // ~16 msg/sec
	defer ticker.Stop()

	var okCount int
	var failCount int
	for _, id := range ids {
		select {
		case <-ctx.Done():
			_ = b.sendMessage(adminChatID, fmt.Sprintf("Рассылка остановлена. OK=%d FAIL=%d", okCount, failCount), "")
			return
		case <-ticker.C:
		}

		if err := b.sendMessage(id, text, ""); err != nil {
			failCount++
			log.Printf("broadcast to %d failed: %v", id, err)
			continue
		}
		okCount++
	}

	_ = b.sendMessage(adminChatID, fmt.Sprintf("Рассылка готова. OK=%d FAIL=%d", okCount, failCount), "")
}

type webAppInfo struct {
	URL string `json:"url"`
}

type inlineButton struct {
	Text         string      `json:"text"`
	CallbackData *string     `json:"callback_data,omitempty"`
	WebApp       *webAppInfo `json:"web_app,omitempty"`
}

type inlineMarkup struct {
	InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
}

func (b *Bot) mainKeyboardJSON(isAdmin bool) string {
	webappURL := strings.TrimRight(b.Cfg.WebappURL, "/")
	apiParam := strings.TrimRight(b.Cfg.PublicBaseURL, "/")
	// If WEBAPP_URL already contains client-side node pool (nodes=...), do not force api=
	// so Mini App can pick a node from the pool.
	if !strings.Contains(webappURL, "api=") && !strings.Contains(webappURL, "nodes=") {
		sep := "?"
		if strings.Contains(webappURL, "?") {
			sep = "&"
		}
		webappURL = webappURL + sep + "api=" + url.QueryEscape(apiParam)
	}

	wallet := "wallet"
	invite := "invite"
	store := "store"
	admin := "admin"

	rows := [][]inlineButton{
		{{Text: "⚡ MINI APP", WebApp: &webAppInfo{URL: webappURL}}},
		{{Text: "💰 Кошелек", CallbackData: &wallet}, {Text: "👥 Рефы", CallbackData: &invite}},
		{{Text: "🛒 Магазин", CallbackData: &store}},
	}
	if isAdmin {
		rows = append(rows, []inlineButton{{Text: "👑 Админ", CallbackData: &admin}})
	}

	bts, err := json.Marshal(inlineMarkup{InlineKeyboard: rows})
	if err != nil {
		return ""
	}
	return string(bts)
}

func fmtAddress(userID int64) string {
	return "BKC" + strconv.FormatInt(userID, 10)
}

func parseRef(payload string) int64 {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return 0
	}
	payload = strings.TrimPrefix(payload, "ref_")
	id, _ := strconv.ParseInt(payload, 10, 64)
	if id <= 0 {
		return 0
	}
	return id
}

func coinsPerUSD(reserve, initialReserve, startRate, minRate int64) int64 {
	if initialReserve <= 0 {
		return startRate
	}
	if reserve < 0 {
		reserve = 0
	}
	if reserve > initialReserve {
		reserve = initialReserve
	}
	span := startRate - minRate
	return minRate + (span*reserve)/initialReserve
}

func (b *Bot) reserveSend(ctx context.Context, adminChatID int64, toID int64, amount int64) error {
	if _, err := b.DB.GetUser(ctx, toID); err != nil {
		_ = b.sendMessage(adminChatID, "Получатель не найден в БД", "")
		return err
	}
	err := b.DB.CreditFromReserve(ctx, toID, amount, "admin_reserve_send", map[string]any{"by": b.Cfg.AdminID})
	if err != nil {
		if errors.Is(err, db.ErrNotEnough) {
			_ = b.sendMessage(adminChatID, "В резерве недостаточно", "")
			return err
		}
		_ = b.sendMessage(adminChatID, "Ошибка перевода из резерва", "")
		return err
	}
	_ = b.sendMessage(adminChatID, fmt.Sprintf("Отправлено %d BKC пользователю %d", amount, toID), "")
	_ = b.sendMessage(toID, fmt.Sprintf("Админ начислил %d BKC", amount), "")
	return nil
}

func (b *Bot) sendMessage(chatID int64, text string, replyMarkup string) error {
	params := tgbotapi.Params{
		"chat_id": strconv.FormatInt(chatID, 10),
		"text":    text,
	}
	if replyMarkup != "" {
		params["reply_markup"] = replyMarkup
	}
	_, err := b.Bot.MakeRequest("sendMessage", params)
	return err
}

func (b *Bot) editMessageText(chatID int64, messageID int, text string, replyMarkup string) error {
	params := tgbotapi.Params{
		"chat_id":    strconv.FormatInt(chatID, 10),
		"message_id": strconv.Itoa(messageID),
		"text":       text,
	}
	if replyMarkup != "" {
		params["reply_markup"] = replyMarkup
	}
	_, err := b.Bot.MakeRequest("editMessageText", params)
	return err
}

func (b *Bot) answerCallback(callbackQueryID string) error {
	params := tgbotapi.Params{"callback_query_id": callbackQueryID}
	_, err := b.Bot.MakeRequest("answerCallbackQuery", params)
	return err
}
