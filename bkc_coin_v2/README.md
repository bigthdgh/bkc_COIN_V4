# BKC COIN v2 (Go)

Новая версия в отдельной папке. Старую папку `bkc_coin` не трогаем.

## Что будет в v2
- Go backend (быстро): API для Mini App + Telegram bot (webhook на хостинге, polling локально)
- Postgres (внешняя БД), чтобы данные не слетали (даже если хостинг перезапустится)
- Без BIO (вообще)
- Энергия + лимиты, чтобы нельзя было бесконечно быстро тапать
- Рефы: каждые 3 приглашенных = +30 000 BKC рефереру
- Total supply: 500 000 000, админ получает 30% при инициализации
- Курс: старт 60 000 BKC за $1, минимум 50 000; меняется от резерва
- CryptoBot (CryptoPay) пополнение (USD) + резервирование монет под инвойсы
- Пополнение по кошелькам (manual): пользователь вводит TX hash, админ подтверждает
- NFT магазин (админ добавляет, пользователи покупают за BKC)
- Банк: кредит 7 дней и 30 дней (проценты в bp), просрочка уводит баланс в минус (автоматически)
- Заморозка средств: перенос BKC в `frozen_balance` (нельзя тратить, пока не разморозишь)
- P2P долги: заемщик отправляет заявку, кредитор Accept/Reject; возврат/Recall
- Барахолка: объявления (вирт/физ/фиат), контакт, фото; комиссия за размещение сжигается; админ может удалять объявления
- Рассылка /broadcast (админ)
- Рассылка из WebApp (админ)

## ENV
- BOT_TOKEN
- ADMIN_ID
- DATABASE_URL (Postgres, например Neon)
- REDIS_URL (опционально, но рекомендую для скорости /tap: Redis для энергии/лимитов + очередь (Redis Streams) + асинхронная фиксация в Postgres)
- MEMTAP_ENABLED=1 (опционально: in-memory tap очередь + пакетный flush в Postgres, если Redis не используешь)
- PUBLIC_BASE_URL (опционально на Render: возьмется из RENDER_EXTERNAL_URL/RENDER_EXTERNAL_HOSTNAME)
- WEBAPP_URL (опционально: по умолчанию = PUBLIC_BASE_URL)
- CORS_ALLOWED_ORIGINS (опционально: список через запятую, если WebApp и API на разных доменах, пример: `https://app.example.com,https://node1.onrender.com`)
- API_PROFILE (опционально: `full` | `tap` | `market` | `bank` | `admin`; ограничивает эндпоинты на конкретной ноде)
- RUN_API (default `1`)
- RUN_BOT (default `1`)
- RUN_OVERDUE_WORKER (default `1`)
- RUN_FASTTAP_WORKER (default `1`)
- COIN_IMAGE_URL (необязательно, по умолчанию = PUBLIC_BASE_URL/assets/coin.svg)
- CRYPTOPAY_API_TOKEN (CryptoBot/CryptoPay)
- CRYPTOPAY_WEBHOOK_SECRET (необязательно, дополнительная защита webhook URL)
- TELEGRAM_WEBHOOK_SECRET (необязательно, защита Telegram webhook URL; если не задан, генерируется на старте)
- TELEGRAM_FORCE_POLLING=1 (необязательно, принудительно отключить webhook)
- DEPOSIT_WALLETS_JSON (необязательно, первоначальные кошельки для manual topup; админ может менять в WebApp, пример: {"USDT_TRC20":"...","TRX":"..."} )

Настройки fasttap (Redis, необязательно):
- REDIS_STREAM_KEY (default `bkc:stream:taps`)
- REDIS_STREAM_GROUP (default `bkc`)
- REDIS_STREAM_CONSUMER (default генерируется)
- REDIS_STREAM_MAXLEN (default 500000)
- REDIS_WORKER_COUNT (default 2)
- REDIS_STREAM_READ_COUNT (default 500)
- REDIS_STREAM_READ_BLOCK_MS (default 1500)
- REDIS_STREAM_APPLY_BATCH (default 500)
- REDIS_STREAM_CLAIM_MIN_IDLE_SEC (default 60)
- REDIS_STREAM_CLAIM_COUNT (default 200)
- REDIS_STREAM_CLAIM_EVERY_SEC (default 30)
- REDIS_STREAM_CLAIM_MAX_ROUNDS (default 4)
- REDIS_HEALTH_PENDING_SCAN (default 20)

Настройки memtap (in-memory, необязательно):
- MEMTAP_ENABLED (default `0`)
- MEMTAP_FLUSH_INTERVAL_MS (default 2000)
- MEMTAP_SYSTEM_REFRESH_SEC (default 5)
- MEMTAP_CACHE_TTL_SEC (default 900)

Безопасность API (anti-abuse, необязательно):
- SECURITY_ENABLED (default `1`)
- SECURITY_MAX_BODY_BYTES (default `65536`)
- SECURITY_API_RATE / SECURITY_API_BURST (default `120` / `240`)
- SECURITY_PUBLIC_RATE / SECURITY_PUBLIC_BURST (default `40` / `80`)
- SECURITY_TAP_IP_RATE / SECURITY_TAP_IP_BURST (default `80` / `200`)
- SECURITY_TAP_USER_RATE / SECURITY_TAP_USER_BURST (default `35` / `120`)
- SECURITY_AUTH_FAIL_WINDOW_SEC (default `20`)
- SECURITY_AUTH_FAIL_THRESHOLD (default `40`)
- SECURITY_BAN_SEC (default `120`)
- SECURITY_ENTRY_TTL_MIN (default `15`)

Настройки экономики (необязательно):
- TOTAL_SUPPLY (default 500000000)
- ADMIN_ALLOCATION_PCT (default 30)
- START_RATE_COINS_PER_USD (default 60000)
- MIN_RATE_COINS_PER_USD (default 50000)

Кредиты/рынок (необязательно):
- BANK_LOAN_7D_INTEREST_BP (default 1200 = 12%)
- BANK_LOAN_30D_INTEREST_BP (default 3500 = 35%)
- BANK_LOAN_MAX_AMOUNT (default 2000000)
- P2P_RECALL_MIN_DAYS (default 5)
- MARKET_LISTING_FEE_COINS (default 2000)

Тапалка (необязательно):
- ENERGY_MAX (default 300)
- ENERGY_REGEN_PER_SEC (default 1.0)
- TAP_MAX_PER_REQUEST (default 500)
- TAP_MAX_MULTITOUCH (default 13)
- TAP_DAILY_LIMIT (default 100000)
- EXTRA_TAPS_PACK_SIZE (default 13000)
- EXTRA_TAPS_PACK_PRICE_COINS (default 15000)

## Запуск локально
```powershell
cd bkc_coin_v2
$env:BOT_TOKEN='...'
$env:ADMIN_ID='...'
$env:DATABASE_URL='postgres://...'
$env:PUBLIC_BASE_URL='http://127.0.0.1:8080'
$env:WEBAPP_URL='http://127.0.0.1:5500'

go run .\cmd\server
```

## Примечание про хостинг
На Render и подобных хостингах бот работает стабильнее через webhook: входящее сообщение само "будит" сервис.
На free-тарифах возможны cold start задержки. 100% "без сна" обычно только на paid-плане или при внешнем пинге (uptime монитор).

## Режимы tap hot-path
- При наличии `REDIS_URL` используется `fasttap` (Redis Lua + Stream + async worker).
- Если `REDIS_URL` не задан, но `MEMTAP_ENABLED=1`, используется `memtap` (in-memory + batch flush в Postgres).
- Если оба выключены, включается прямой DB path (самый медленный, только для dev/малой нагрузки).

## Разделение по Render-нодам (практика)
Пример профилей для нескольких сервисов из одного репо:

1. `bkc-bot-core`
   - `RUN_BOT=1`
   - `RUN_API=1`
   - `API_PROFILE=full`
   - `RUN_OVERDUE_WORKER=1`
   - `RUN_FASTTAP_WORKER=1`
2. `bkc-tap-1..N`
   - `RUN_BOT=0`
   - `RUN_API=1`
   - `API_PROFILE=tap`
   - `RUN_OVERDUE_WORKER=0`
   - `RUN_FASTTAP_WORKER=1` (или `0`, если отдельные worker-ноды)
3. `bkc-market-1..M`
   - `RUN_BOT=0`
   - `RUN_API=1`
   - `API_PROFILE=market`
   - `RUN_OVERDUE_WORKER=0`
4. `bkc-bank-1..K`
   - `RUN_BOT=0`
   - `RUN_API=1`
   - `API_PROFILE=bank`
   - `RUN_OVERDUE_WORKER=1` только на 1 ноде, на остальных `0`

Каждая нода имеет `/healthz` (root) и `/api/v1/health` (если `RUN_API=1`).

## Клиентский пул API (опционально)
WebApp поддерживает список нод в query-параметре `nodes`:

`https://<webapp-host>/?nodes=https://node1.onrender.com,https://node2.onrender.com`

- нода выбирается sticky в рамках сессии;
- при сетевой ошибке клиент автоматически переключится на следующую;
- для кросс-доменных запросов не забудьте задать `CORS_ALLOWED_ORIGINS`.

Для профильных нод можно задавать отдельные пулы:
- `nodes_tap=...`
- `nodes_market=...`
- `nodes_bank=...`
- `nodes_admin=...`
- `nodes_full=...`

Пример:

`https://<webapp-host>/?nodes_tap=https://tap1.onrender.com,https://tap2.onrender.com&nodes_market=https://market1.onrender.com&nodes_bank=https://bank1.onrender.com`

## Пингер для free-хостингов (опционально)
В `tools/pinger.py` есть простой внешний пингер `/api/v1/health` раз в N секунд.
Запуск:

```powershell
$env:PING_URLS='https://node1.onrender.com/api/v1/health,https://node2.onrender.com/api/v1/health'
$env:PING_INTERVAL_SEC='600'
python .\tools\pinger.py
```

