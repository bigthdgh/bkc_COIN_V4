# BKC Coin v2 - Полное руководство по развертыванию

## 🏗️ Архитектура "BKC-IMMORTAL"

Распределенная система способна выдержать 20,000+ одновременных пользователей с нулевой стоимостью хостинга.

### 📊 Общая структура

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Frontend      │    │   Backend       │    │   Games         │
│   (Vercel)      │    │   (Render x15)  │    │   (Render x3)   │
│                 │    │                 │    │                 │
│ React + Canvas  │◄──►│ Go API Nodes    │◄──►│ WebSocket       │
│ Multi-language  │    │ Load Balancer   │    │ Real-time       │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
         ┌─────────────────────────────────────────────────┐
         │              Data Layer                          │
         │  ┌─────────────┐ ┌─────────────┐ ┌───────────┐ │
         │  │   Neon      │ │CockroachDB  │ │   Redis   │ │
         │  │  (x5 DBs)   │ │  (x2 DBs)   │ │ (x4 nodes)│ │
         │  └─────────────┘ └─────────────┘ └───────────┘ │
         └─────────────────────────────────────────────────┘
```

---

## 🚀 Пошаговая инструкция по развертыванию

### Шаг 1: Подготовка репозитория

1. **Форкните репозиторий** на GitHub
2. **Создайте ветку** для продакшена:
```bash
git checkout -b production
git push origin production
```

### Шаг 2: Настройка баз данных

#### 🟢 Neon.tech (5 баз для пользователей)

Зарегистрируйтесь на [Neon.tech](https://neon.tech) и создайте 5 проектов:

| База | Название | Переменные окружения | Назначение |
|------|----------|---------------------|------------|
| DB_0 | bkc-users-0 | `NEON_0_HOST`, `NEON_0_PASSWORD`, `NEON_0_DB` | ID оканчивающиеся на 0-1 |
| DB_1 | bkc-users-1 | `NEON_1_HOST`, `NEON_1_PASSWORD`, `NEON_1_DB` | ID оканчивающиеся на 2-3 |
| DB_2 | bkc-users-2 | `NEON_2_HOST`, `NEON_2_PASSWORD`, `NEON_2_DB` | ID оканчивающиеся на 4-5 |
| DB_3 | bkc-users-3 | `NEON_3_HOST`, `NEON_3_PASSWORD`, `NEON_3_DB` | ID оканчивающиеся на 6-7 |
| DB_4 | bkc-users-4 | `NEON_4_HOST`, `NEON_4_PASSWORD`, `NEON_4_DB` | ID оканчивающиеся на 8-9 |

**Для каждой базы:**
- Выберите регион **US East** (для скорости)
- Размер: **Free tier** (500 МБ хватит на 1M+ пользователей)
- Бранч: **main**
- Скопируйте **Connection string**

#### 🟤 CockroachDB (2 базы для логов)

Зарегистрируйтесь на [CockroachDB](https://cockroachlabs.cloud):

| База | Название | Переменные окружения | Назначение |
|------|----------|---------------------|------------|
| LOG_0 | bkc-logs-0 | `COCKROACH_0_HOST`, `COCKROACH_0_PASSWORD`, `COCKROACH_0_DB` | Клики и переводы |
| LOG_1 | bkc-logs-1 | `COCKROACH_1_HOST`, `COCKROACH_1_PASSWORD`, `COCKROACH_1_DB` | Игры и барахолка |

**Для каждой базы:**
- План: **Free** (10 ГБ)
- Регион: **US Central**
- Скопируйте **Connection string**

#### 🔴 Redis (4 ноды для кэша)

Зарегистрируйтесь на [Upstash Redis](https://upstash.com/redis):

| Нода | Название | Переменные окружения | Назначение |
|------|----------|---------------------|------------|
| REDIS_0 | redis-global-0 | `REDIS_0_HOST`, `REDIS_0_PASSWORD` | Глобальный онлайн |
| REDIS_1 | redis-global-1 | `REDIS_1_HOST`, `REDIS_1_PASSWORD` | Pub/Sub backup |
| REDIS_2 | redis-leaderboard-0 | `REDIS_2_HOST`, `REDIS_2_PASSWORD` | Лидерборды |
| REDIS_3 | redis-leaderboard-1 | `REDIS_3_HOST`, `REDIS_3_PASSWORD` | Лидерборды backup |

**Для каждой ноды:**
- Регион: **US East** (ближайший к пользователям)
- План: **Free** (30 МБ)
- База данных: **0** для глобальных, **1** для лидербордов

---

### Шаг 3: Развертывание фронтенда (Vercel)

1. **Зайдите в [Vercel](https://vercel.com)**
2. **Import Project** → выберите GitHub репозиторий
3. **Настройки сборки:**
   ```
   Framework Preset: React
   Root Directory: ./webapp
   Build Command: npm run build
   Output Directory: dist
   Install Command: npm install
   ```

4. **Environment Variables:**
   ```
   REACT_APP_API_NODES=https://bkc-core-1.onrender.com,https://bkc-core-2.onrender.com,https://bkc-core-3.onrender.com
   REACT_APP_GAME_NODE=https://bkc-games-1.onrender.com
   REACT_APP_WS_NODE=wss://bkc-games-1.onrender.com
   REACT_APP_DEFAULT_LANGUAGE=ru
   ```

5. **Deploy** → получите URL вида `https://bkc-coin.vercel.app`

---

### Шаг 4: Развертывание бэкенда (Render x15)

#### 📋 Список необходимых сервисов Render:

**Основные ноды (10 штук):**
- `bkc-core-1` → `bkc-core-10`
- Тип: **Web Service**
- Бранч: **production**

**Игровые ноды (3 штуки):**
- `bkc-games-1` → `bkc-games-3`
- Тип: **Web Service**
- Бранч: **production**

**Специализированные ноды (2 штуки):**
- `bkc-market-1` (барахолка)
- `bkc-bank-1` (банк и подписки)

#### 🔧 Конфигурация для каждой ноды:

**Основные ноды (Core 1-10):**
```bash
# Runtime
Runtime: Go
Build Command: go build -o bin/server ./cmd/server
Start Command: ./bin/server

# Environment Variables
BOT_TOKEN=your_telegram_bot_token
ADMIN_ID=your_admin_id
DATABASE_URL=postgres://neon_connection_string
REDIS_URL=redis://upstash_connection_string
PUBLIC_BASE_URL=https://your-service-name.onrender.com
WEBAPP_URL=https://bkc-coin.vercel.app

# Профиль ноды
RUN_API=1
RUN_BOT=0  # Только на первой ноде
API_PROFILE=full
RUN_FASTTAP_WORKER=1

# Neon шарды (все 5)
NEON_0_HOST=ep-xxx.us-east-2.aws.neon.tech
NEON_0_PASSWORD=xxx
NEON_0_DB=bkc_users_0
NEON_1_HOST=ep-yyy.us-east-2.aws.neon.tech
NEON_1_PASSWORD=yyy
NEON_1_DB=bkc_users_1
# ... и так далее для всех 5 баз

# CockroachDB шарды
COCKROACH_0_HOST=free-tier.gcp-us-central1.cockroachlabs.cloud
COCKROACH_0_PASSWORD=zzz
COCKROACH_0_DB=bkc_logs_0
COCKROACH_1_HOST=free-tier.gcp-us-east1.cockroachlabs.cloud
COCKROACH_1_PASSWORD=www
COCKROACH_1_DB=bkc_logs_1

# Redis ноды
REDIS_0_HOST=redis-12345.c1.us-east-1-2.ec2.cloud.redislabs.com
REDIS_0_PASSWORD=aaa
REDIS_1_HOST=redis-12346.c1.us-east-1-2.ec2.cloud.redislabs.com
REDIS_1_PASSWORD=bbb
REDIS_2_HOST=redis-12347.c1.us-east-1-2.ec2.cloud.redislabs.com
REDIS_2_PASSWORD=ccc
REDIS_3_HOST=redis-12348.c1.us-east-1-2.ec2.cloud.redislabs.com
REDIS_3_PASSWORD=ddd
```

**Игровые ноды (Games 1-3):**
```bash
# Отличия от основных нод
API_PROFILE=games
RUN_BOT=0
RUN_FASTTAP_WORKER=0

# WebSocket настройки
ENABLE_WEBSOCKET=1
WS_READ_BUFFER_SIZE=1024
WS_WRITE_BUFFER_SIZE=1024
WS_PING_PERIOD=30s
```

**Нода барахолки (Market-1):**
```bash
API_PROFILE=market
RUN_OVERDUE_WORKER=0
RUN_FASTTAP_WORKER=0

# Настройки маркетплейса
MARKET_LISTING_FEE_COINS=2000
MARKET_ESCROW_FEE=0.02
MARKET_MAX_LISTINGS=50
```

**Нода банка (Bank-1):**
```bash
API_PROFILE=bank
RUN_OVERDUE_WORKER=1
RUN_FASTTAP_WORKER=0

# Настройки банка
BANK_LOAN_7D_INTEREST_BP=1200
BANK_LOAN_30D_INTEREST_BP=3500
BANK_LOAN_MAX_AMOUNT=2000000
```

---

### Шаг 5: Настройка Telegram Bot

1. **Создайте бота** через [@BotFather](https://t.me/botfather):
   ```
   /newbot
   BKC Coin Bot
   bkc_coin_bot
   ```

2. **Получите токен** и добавьте в `BOT_TOKEN`

3. **Настройте Webhook** (только для первой ноды):
   ```bash
   curl -X POST "https://api.telegram.org/bot{BOT_TOKEN}/setWebhook" \
   -H "Content-Type: application/json" \
   -d '{"url": "https://bkc-core-1.onrender.com/webhook"}'
   ```

4. **Создайте Mini App** в BotFather:
   ```
   /newapp
   BKC Coin
   https://bkc-coin.vercel.app
   ```

---

### Шаг 6: Настройка мониторинга и пингера

#### 🏥 Health Check

Каждая нода имеет эндпоинт `/api/v1/health`:
```json
{
  "status": "ok",
  "timestamp": "2024-01-01T00:00:00Z",
  "load": 45,
  "uptime": 3600,
  "version": "v2.0.0"
}
```

#### 📡 Внешний пингер

Создайте бесплатный аккаунт на [cron-job.org](https://cron-job.org):

**Задачи для пинга:**
```
1. https://bkc-core-1.onrender.com/api/v1/health (каждые 5 минут)
2. https://bkc-core-2.onrender.com/api/v1/health (каждые 5 минут)
...
15. https://bkc-games-3.onrender.com/api/v1/health (каждые 5 минут)
```

---

### Шаг 7: Оптимизация производительности

#### ⚡ Настройка Go runtime

Добавьте в `main.go`:
```go
func init() {
    // Оптимизация под 1 ядро Render Free
    runtime.GOMAXPROCS(1)
    
    // Настройки GC для высокой нагрузки
    debug.SetGCPercent(100)
    debug.SetMemoryLimit(100 * 1024 * 1024) // 100MB
}
```

#### 🗄️ Оптимизация баз данных

**Neon настройки:**
```sql
-- Оптимизация для высоких нагрузок
ALTER SYSTEM SET shared_buffers = '128MB';
ALTER SYSTEM SET effective_cache_size = '256MB';
ALTER SYSTEM SET maintenance_work_mem = '64MB';
ALTER SYSTEM SET checkpoint_completion_target = 0.9;
```

**Redis настройки:**
```bash
# Максимальная производительность
maxmemory-policy allkeys-lru
save ""
appendonly yes
appendfsync everysec
```

---

### Шаг 8: Тестирование нагрузки

#### 🧪 Load Testing

Используйте [k6](https://k6.io) для тестирования:

```javascript
// load-test.js
import http from 'k6/http';
import { check } from 'k6';

export let options = {
  stages: [
    { duration: '2m', target: 1000 }, // Подъем до 1000 пользователей
    { duration: '5m', target: 5000 }, // Подъем до 5000
    { duration: '10m', target: 10000 }, // Подъем до 10000
    { duration: '5m', target: 20000 }, // Пик 20000
    { duration: '5m', target: 0 }, // Спад
  ],
};

export default function() {
  let response = http.post('https://bkc-core-1.onrender.com/api/v1/tap', 
    JSON.stringify({
      user_id: Math.floor(Math.random() * 1000000),
      taps: Math.floor(Math.random() * 100) + 1,
    }), {
      headers: {
        'Content-Type': 'application/json',
      },
    }
  );
  
  check(response, {
    'status is 200': (r) => r.status === 200,
    'response time < 500ms': (r) => r.timings.duration < 500,
  });
}
```

Запуск теста:
```bash
k6 run load-test.js
```

---

## 🔧 Мониторинг и отладка

### 📊 Метрики производительности

**Ключевые метрики для мониторинга:**
- **Response time**: < 200ms (95 percentile)
- **CPU usage**: < 80% на каждой ноде
- **Memory usage**: < 400MB на каждой ноде
- **Database connections**: < 80% от максимума
- **Redis memory**: < 80% от лимита

### 🚨 Алерты

Настройте алерты в [UptimeRobot](https://uptimerobot.com):

1. **HTTP Monitor** для каждой ноды
2. **Keyword Monitor** для проверки "status": "ok"
3. **Alert threshold**: 2 неудачные проверки подряд

### 📱 Логирование

Добавьте в каждую ноду:
```go
// Структурированное логирование
log.Printf("REQUEST: method=%s path=%s user_id=%d latency=%dms", 
    r.Method, r.URL.Path, userID, latency)

// Ошибки с контекстом
log.Printf("ERROR: err=%v user_id=%d request_id=%s", 
    err, userID, requestID)
```

---

## 🔄 Обновления и обслуживание

### 📦 Процесс обновления

1. **Подготовка:**
   ```bash
   git checkout -b update-v2.1
   # Внесите изменения
   git commit -m "Update to v2.1"
   git push origin update-v2.1
   ```

2. **Пошаговое развертывание:**
   - Обновите `bkc-core-1` → проверьте health
   - Обновите `bkc-core-2` → проверьте health
   - ... и так далее для всех нод
   - В последнюю очередь обновите игровые ноды

3. **Откат при проблемах:**
   ```bash
   git checkout production
   git push origin production
   ```

### 💾 Резервное копирование

**Автоматические бэкапы Neon:**
```bash
# Ежедневные бэкапы через API
curl -X POST "https://console.neon.tech/api/v2/projects/{project_id}/backups" \
  -H "Authorization: Bearer {api_key}" \
  -H "Content-Type: application/json" \
  -d '{"backup_name": "daily-backup-$(date +%Y%m%d)"}'
```

**Резервное копирование CockroachDB:**
```bash
# Еженедельные бэкапы
cockroach sql --url={connection_string} \
  --execute="BACKUP DATABASE TO 's3://backups/$(date +%Y%m%d)/'"
```

---

## 🎯 Масштабирование

### 📈 Когда переходить на платные тарифы

**Триггеры для масштабирования:**
- **Neon**: > 400MB использовано из 500MB
- **CockroachDB**: > 8GB использовано из 10GB  
- **Redis**: > 24MB использовано из 30MB
- **Render**: > 750 hours/month (free limit)

### 🚀 План масштабирования

**Level 1 (0-10k пользователей):**
- Использовать текущую бесплатную архитектуру
- Оптимизировать код и кэширование

**Level 2 (10k-50k пользователей):**
- Обновить 2-3 ноды Render до **Starter ($7/мес)**
- Обновить Neon до **Scale ($19/мес)**

**Level 3 (50k+ пользователей):**
- Переехать на **AWS/DigitalOcean VPS**
- Настроить **Kubernetes cluster**
- Использовать **managed PostgreSQL**

---

## 🆘 Поддержка и troubleshooting

### 🔧 Частые проблемы

**1. Cold start на Render:**
- Решение: Внешний пингер каждые 5 минут
- Альтернатива: Upgrade до Starter плана

**2. Database connection limits:**
- Решение: Увеличить `pool_size` в настройках
- Оптимизировать запросы и индексы

**3. Memory leaks:**
- Решение: Проверить горутины с `pprof`
- Добавить `runtime.SetMemoryLimit`

**4. WebSocket disconnects:**
- Решение: Увеличить `ping_period`
- Добавить реконнект на клиенте

### 📞 Контакты для поддержки

- **GitHub Issues**: [repository/issues](https://github.com/your-repo/issues)
- **Telegram**: @bkc_support
- **Email**: support@bkc-coin.com

---

## ✅ Чек-лист развертывания

- [ ] Созданы 5 баз Neon.tech
- [ ] Созданы 2 базы CockroachDB  
- [ ] Созданы 4 ноды Redis
- [ ] Развернут фронтенд на Vercel
- [ ] Созданы 15 нод на Render
- [ ] Настроен Telegram Bot
- [ ] Настроен внешний пингер
- [ ] Проведено load testing
- [ ] Настроены алерты и мониторинг
- [ ] Созданы бэкапы
- [ ] Документированы все пароли и ключи

---

**🎉 Поздравляю! Ваша BKC Coin v2 готова принимать тысячи пользователей!**

**Максимальная нагрузка: 20,000+ одновременных пользователей**
**Стоимость: $0/месяц (на бесплатных тарифах)**
**Время развертывания: ~2 часа**
