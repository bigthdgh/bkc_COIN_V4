# BKC Coin - Complete Implementation

## Overview

BKC Coin - это полноценная криптовалютная система с игровыми элементами, построенная на Go. Проект включает в себя токеномику, майнинг, маркетплейс, кредитную систему, игры и масштабируемую кластерную архитектуру.

## Architecture

### Core Components

1. **Tokenomics** (`internal/tokenomics/`)
   - Эмиссия 1 млрд BKC
   - Халвинг каждые 100 млн добытых монет
   - 10% налог на все транзакции (burn)
   - Ежемесячная разблокировка замороженных монет

2. **Mining** (`internal/mining/`)
   - Ежедневные лимиты тапов
   - Прогрессия уровней с экспоненциальной стоимостью
   - Премиум подписки (Basic, Silver, Gold)
   - Энергия и бусты

3. **Marketplace** (`internal/marketplace/`)
   - NFT каталог и покупки
   - Барахолка для физических товаров
   - P2P торги с эскроу
   - Комиссии и рейтинги

4. **Credits** (`internal/credits/`)
   - Системные кредиты под 5-7% в день
   - P2P кредиты с залогом (NFT/BKC)
   - Коллекторский режим для просроченных кредитов
   - Проверка eligibility

5. **Games** (`internal/games/`)
   - Игра "Ракетка" (Crash) с Provably Fair
   - Внутренняя биржа с графиками
   - Статистика и аналитика

6. **Cluster** (`internal/cluster/`)
   - 15 нод на Render
   - Шардинг баз данных (5 Neon, 2 Supabase, 2 CockroachDB)
   - 6 экземпляров Redis для разных задач
   - Load balancing и health checking

7. **Admin Panel** (`internal/admin/`)
   - God Mode для администраторов
   - Управление пользователями и банами
   - Модерация маркетплейса
   - Аналитика и статистика

8. **Anti-Abuse** (`internal/antiabuse/`)
   - Rate limiting
   - Детекция ботов и читов
   - Отслеживание IP и устройств
   - Система репортов

## Database Structure

### Primary Databases

1. **Neon (5 shards)** - Профили пользователей, тапы, энергия
2. **Supabase (2 shards)** - P2P, кредиты, маркетплейс
3. **CockroachDB (2 shards)** - Логи, аналитика
4. **Redis (6 instances)** - Кэш, anti-cheat, очереди, лидерборды

### Key Tables

- `users` - Основная информация о пользователях
- `system_state` - Состояние токеномики
- `user_nfts` - NFT коллекции
- `market_listings` - Объявления маркетплейса
- `p2p_loans` - P2P кредиты
- `crash_games` - Игры Ракетка
- `cheat_alerts` - Оповещения о читерстве

## API Endpoints

### User Management
- `POST /api/auth/login` - Авторизация
- `POST /api/auth/register` - Регистрация
- `GET /api/user/profile` - Профиль пользователя
- `PUT /api/user/profile` - Обновление профиля

### Mining
- `POST /api/mining/tap` - Совершить тап
- `GET /api/mining/state` - Состояние майнинга
- `POST /api/mining/upgrade` - Повысить уровень
- `POST /api/mining/subscribe` - Купить подписку

### Marketplace
- `GET /api/market/nft` - NFT каталог
- `POST /api/market/nft/buy` - Купить NFT
- `GET /api/market/listings` - Объявления
- `POST /api/market/listings` - Создать объявление

### Credits
- `POST /api/credits/bank/loan` - Взять системный кредит
- `POST /api/credits/bank/repay` - Погасить кредит
- `POST /api/credits/p2p/request` - Создать P2P запрос
- `POST /api/credits/p2p/accept` - Принять P2P кредит

### Games
- `POST /api/games/crash/start` - Начать игру Ракетка
- `POST /api/games/crash/bet` - Сделать ставку
- `POST /api/games/crash/cashout` - Вывести ставку
- `GET /api/games/exchange/prices` - Цены биржи

### Admin Panel
- `POST /api/admin/login` - Вход админа
- `GET /api/admin/users` - Список пользователей
- `POST /api/admin/users/ban` - Забанить пользователя
- `GET /api/admin/analytics` - Аналитика

## Configuration

### Environment Variables

```bash
# Database
DATABASE_URL=postgresql://user:pass@localhost:5432/bkc_coin
REDIS_URL=redis://localhost:6379

# Tokenomics
TOTAL_SUPPLY=1000000000000
ADMIN_ALLOCATION_PCT=10
BURN_TAX_PCT=10

# Mining
TAP_REWARD=100
MAX_DAILY_TAPS=10000
ENERGY_REGEN_RATE=1

# Credits
BANK_INTEREST_RATE=5
MAX_LOAN_AMOUNT=2000000

# Cluster
NODE_COUNT=15
SHARD_COUNT=9
REDIS_INSTANCES=6
```

## Deployment

### Local Development

1. **Клонировать репозиторий**
```bash
git clone <repository-url>
cd bkc_coin_v2
```

2. **Установить зависимости**
```bash
go mod download
```

3. **Настроить базу данных**
```bash
psql -U postgres -c "CREATE DATABASE bkc_coin;"
psql -U postgres -d bkc_coin -f database_schema_complete.sql
```

4. **Запустить Redis**
```bash
redis-server
```

5. **Запустить приложение**
```bash
go run cmd/server/main.go
```

### Production Deployment (Render)

1. **Подготовить 15 нод**
```bash
# Main Core (10 нод)
render deploy --service node-main-1 --service-type web
# ... повторить для node-main-2 ... node-main-10

# Market (1 нода)
render deploy --service node-market --service-type web

# Bank (1 нода)
render deploy --service node-bank --service-type web

# Games (3 ноды)
render deploy --service node-games-1 --service-type web
# ... повторить для node-games-2, node-games-3
```

2. **Настроить базы данных**
```bash
# Neon (5 шардов)
for i in {1..5}; do
  neon create --name bkc_users_$i --region us-east-1
done

# Supabase (2 шарда)
for i in {1..2}; do
  supabase create --name bkc_market_$i --region us-east-1
done

# CockroachDB (2 шарда)
for i in {1..2}; do
  cockroach create --name bkc_logs_$i --region us-east-1
done

# Redis (6 экземпляров)
for i in {1..6}; do
  redis create --name redis-$i --region us-east-1
done
```

3. **Настроить Load Balancer**
```bash
# Установить NGINX или использовать Render Load Balancer
# Настроить routing по ролям нод
```

## Monitoring

### Health Checks

Каждая нода предоставляет `/health` endpoint:
```json
{
  "status": "healthy",
  "timestamp": "2024-01-01T00:00:00Z",
  "uptime": "24h30m15s",
  "memory_usage": "45%",
  "cpu_usage": "23%"
}
```

### Metrics

- Prometheus metrics на `/metrics`
- Grafana dashboard для визуализации
- Алерты для критических ситуаций

### Logging

Структурированные логи в JSON формате:
```json
{
  "level": "info",
  "timestamp": "2024-01-01T00:00:00Z",
  "service": "node-main-1",
  "user_id": 12345,
  "action": "tap",
  "amount": 100,
  "duration": "15ms"
}
```

## Security

### Anti-Cheat Measures

1. **Rate Limiting**
   - Максимум 20 тапов в секунду
   - Максимум 1000 тапов в минуту
   - IP-based throttling

2. **Bot Detection**
   - Анализ интервалов между тапами
   - Детекция слишком постоянных паттернов
   - Device fingerprinting

3. **Multi-Account Prevention**
   - Отслеживание IP адресов
   - Отслеживание устройств
   - Алерты при подозрительной активности

### Data Protection

- Все пароли хешируются bcrypt
- TLS 1.3 для всех соединений
- Регулярные бэкапы баз данных
- Аудит всех действий администраторов

## Performance

### Scalability

- Горизонтальное масштабирование через добавление нод
- Шардинг баз данных для распределения нагрузки
- Redis для кэширования частых запросов
- Connection pooling для баз данных

### Optimization

- Индексы для всех часто используемых запросов
- Batch операции для массовых обновлений
- Асинхронная обработка фоновых задач
- Сжатие ответов API

## Testing

### Unit Tests
```bash
go test ./...
```

### Integration Tests
```bash
go test -tags=integration ./...
```

### Load Testing
```bash
k6 run scripts/load_test.js
```

## Contributing

1. Fork репозитория
2. Создать feature branch
3. Сделать изменения
4. Добавить тесты
5. Запустить CI/CD pipeline
6. Создать Pull Request

## License

MIT License - см. файл LICENSE

## Support

- Telegram: @bkc_coin_support
- Email: support@bkc-coin.com
- Documentation: https://docs.bkc-coin.com

---

**Status**: ✅ Complete Implementation Ready for Production

Все основные компоненты реализованы согласно техническому заданию. Система готова к развертыванию на production инфраструктуре.
