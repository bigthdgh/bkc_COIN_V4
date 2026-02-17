-- =============================================================================
-- BKC COIN V2 - ПОЛНАЯ СТРУКТУРА БАЗ ДАННЫХ
-- Архитектура: 5 Neon (профили), 2 Supabase (P2P/кредиты), 2 Cockroach (логи), 6 Redis
-- =============================================================================

-- БАЗА 1-5: NEON (ПРОФИЛИ ПОЛЬЗОВАТЕЛЕЙ - ШАРДИНГ ПО user_id % 5)
-- =============================================================================

-- Основная таблица пользователей (для каждой из 5 баз Neon)
CREATE TABLE IF NOT EXISTS users (
    user_id BIGINT PRIMARY KEY,
    username TEXT,
    first_name TEXT,
    balance BIGINT NOT NULL DEFAULT 0,
    frozen_balance BIGINT NOT NULL DEFAULT 0,
    taps_total BIGINT NOT NULL DEFAULT 0,
    energy DOUBLE PRECISION NOT NULL DEFAULT 1000,
    energy_max DOUBLE PRECISION NOT NULL DEFAULT 1000,
    energy_updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    energy_boost_until TIMESTAMPTZ,
    energy_boost_regen_multiplier DOUBLE PRECISION NOT NULL DEFAULT 1,
    energy_boost_max_multiplier DOUBLE PRECISION NOT NULL DEFAULT 1,
    level INT NOT NULL DEFAULT 1,
    taps_power INT NOT NULL DEFAULT 1,
    referrals_count BIGINT NOT NULL DEFAULT 0,
    referred_by BIGINT,
    is_admin BOOLEAN DEFAULT FALSE,
    is_premium BOOLEAN DEFAULT FALSE,
    premium_type TEXT DEFAULT 'basic', -- basic, silver, gold
    premium_until TIMESTAMPTZ,
    subscription_required BOOLEAN DEFAULT TRUE,
    is_subscribed BOOLEAN DEFAULT FALSE,
    daily_taps_limit INT NOT NULL DEFAULT 5000,
    daily_taps_used INT NOT NULL DEFAULT 0,
    last_tap_date DATE DEFAULT CURRENT_DATE,
    collector_mode BOOLEAN DEFAULT FALSE,
    loan_debt BIGINT DEFAULT 0,
    banned_until TIMESTAMPTZ,
    ban_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Индексы для оптимизации
CREATE INDEX IF NOT EXISTS users_balance_idx ON users(balance DESC);
CREATE INDEX IF NOT EXISTS users_level_idx ON users(level DESC);
CREATE INDEX IF NOT EXISTS users_taps_total_idx ON users(taps_total DESC);
CREATE INDEX IF NOT EXISTS users_referrals_idx ON users(referrals_count DESC);
CREATE INDEX IF NOT EXISTS users_premium_idx ON users(is_premium, premium_until);
CREATE INDEX IF NOT EXISTS users_collector_idx ON users(collector_mode, loan_debt);

-- Таблица ежедневных лимитов тапов
CREATE TABLE IF NOT EXISTS user_daily (
    user_id BIGINT NOT NULL,
    day DATE NOT NULL,
    tapped BIGINT NOT NULL DEFAULT 0,
    extra_quota BIGINT NOT NULL DEFAULT 0,
    energy_used DOUBLE PRECISION DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, day)
);

CREATE INDEX IF NOT EXISTS user_daily_day_idx ON user_daily(day, tapped DESC);

-- Реферальная система
CREATE TABLE IF NOT EXISTS referrals (
    id BIGSERIAL PRIMARY KEY,
    referrer_id BIGINT NOT NULL,
    referred_id BIGINT NOT NULL UNIQUE,
    bonus BIGINT NOT NULL DEFAULT 0,
    milestone_earned BIGINT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS referrals_referrer_idx ON referrals(referrer_id, created_at DESC);

-- NFT, которыми владеют пользователи
CREATE TABLE IF NOT EXISTS user_nfts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    nft_id INT NOT NULL,
    qty INT NOT NULL DEFAULT 1,
    acquired_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    is_collateral BOOLEAN DEFAULT FALSE, -- В залоге для P2P кредита
    loan_id BIGINT, -- ID кредита, если в залоге
    UNIQUE(user_id, nft_id)
);

CREATE INDEX IF NOT EXISTS user_nfts_user_idx ON user_nfts(user_id);
CREATE INDEX IF NOT EXISTS user_nfts_collateral_idx ON user_nfts(is_collateral, loan_id);

-- Подписки (BKC Premium)
CREATE TABLE IF NOT EXISTS subscriptions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    plan_type TEXT NOT NULL, -- basic, silver, gold
    price_paid BIGINT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    auto_renew BOOLEAN DEFAULT FALSE,
    UNIQUE(user_id, is_active)
);

CREATE INDEX IF NOT EXISTS subscriptions_user_idx ON subscriptions(user_id, is_active);
CREATE INDEX IF NOT EXISTS subscriptions_expires_idx ON subscriptions(expires_at, is_active);

-- Системное состояние (только в первой базе Neon)
CREATE TABLE IF NOT EXISTS system_state (
    id INT PRIMARY KEY DEFAULT 1,
    total_supply BIGINT NOT NULL DEFAULT 1_000_000_000,
    reserve_supply BIGINT NOT NULL DEFAULT 700_000_000,
    reserved_supply BIGINT NOT NULL DEFAULT 0,
    initial_reserve BIGINT NOT NULL DEFAULT 700_000_000,
    admin_user_id BIGINT NOT NULL,
    admin_allocated BIGINT NOT NULL DEFAULT 300_000_000,
    frozen_supply BIGINT NOT NULL DEFAULT 300_000_000, -- Замороженные на 6 месяцев
    unfrozen_schedule JSONB DEFAULT '{}', -- График разблокировки
    start_rate_coins_usd BIGINT NOT NULL DEFAULT 60_000,
    min_rate_coins_usd BIGINT NOT NULL DEFAULT 50_000,
    current_tap_reward BIGINT NOT NULL DEFAULT 1,
    total_mined BIGINT NOT NULL DEFAULT 0,
    total_burned BIGINT NOT NULL DEFAULT 0,
    halving_threshold BIGINT NOT NULL DEFAULT 100_000_000,
    current_halving INT NOT NULL DEFAULT 0,
    referral_step BIGINT NOT NULL DEFAULT 3,
    referral_bonus BIGINT NOT NULL DEFAULT 30_000,
    tax_rate_burn DECIMAL(5,2) NOT NULL DEFAULT 10.0, -- % сжигания
    tax_rate_system DECIMAL(5,2) NOT NULL DEFAULT 5.0, -- % системе
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Бухгалтерская книга (все транзакции)
CREATE TABLE IF NOT EXISTS ledger (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT UNIQUE,
    ts TIMESTAMPTZ NOT NULL DEFAULT now(),
    kind TEXT NOT NULL, -- tap, transfer, burn, market, loan, nft, premium, etc.
    from_id BIGINT,
    to_id BIGINT,
    amount BIGINT NOT NULL,
    tax_burned BIGINT DEFAULT 0,
    tax_system BIGINT DEFAULT 0,
    meta JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS ledger_ts_idx ON ledger(ts DESC);
CREATE INDEX IF NOT EXISTS ledger_to_idx ON ledger(to_id);
CREATE INDEX IF NOT EXISTS ledger_from_idx ON ledger(from_id);
CREATE INDEX IF NOT EXISTS ledger_kind_idx ON ledger(kind, ts DESC);
CREATE INDEX IF NOT EXISTS ledger_event_id_uniq ON ledger(event_id);

-- БАЗА 6-7: SUPABASE (P2P МАРКЕТ И КРЕДИТЫ)
-- =============================================================================

-- P2P заказы на покупку/продажу BKC
CREATE TABLE IF NOT EXISTS p2p_orders (
    id BIGSERIAL PRIMARY KEY,
    seller_id BIGINT NOT NULL,
    buyer_id BIGINT,
    amount_bkc BIGINT NOT NULL,
    price_ton DECIMAL(10,2) NOT NULL,
    price_usd DECIMAL(10,2),
    status TEXT NOT NULL DEFAULT 'open', -- open, locked, completed, cancelled, dispute
    escrow_bkc BIGINT DEFAULT 0, -- Залог в BKC
    contact_method TEXT, -- Как связаться (telegram, etc)
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    dispute_reason TEXT
);

CREATE INDEX IF NOT EXISTS p2p_orders_status_idx ON p2p_orders(status, created_at DESC);
CREATE INDEX IF NOT EXISTS p2p_orders_seller_idx ON p2p_orders(seller_id, status);
CREATE INDEX IF NOT EXISTS p2p_orders_buyer_idx ON p2p_orders(buyer_id, status);

-- Системные кредиты (от банка BKC)
CREATE TABLE IF NOT EXISTS bank_loans (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    principal BIGINT NOT NULL,
    interest_rate DECIMAL(5,2) NOT NULL DEFAULT 5.0, -- % в день
    interest_total BIGINT NOT NULL,
    total_due BIGINT NOT NULL,
    term_days INT NOT NULL DEFAULT 3,
    status TEXT NOT NULL DEFAULT 'active', -- active, repaid, overdue, collector
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    due_at TIMESTAMPTZ NOT NULL,
    closed_at TIMESTAMPTZ,
    collector_started_at TIMESTAMPTZ,
    daily_collected BIGINT DEFAULT 0
);

CREATE INDEX IF NOT EXISTS bank_loans_user_idx ON bank_loans(user_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS bank_loans_status_due_idx ON bank_loans(status, due_at);
CREATE INDEX IF NOT EXISTS bank_loans_collector_idx ON bank_loans(status, collector_started_at);

-- P2P кредиты (между пользователями)
CREATE TABLE IF NOT EXISTS p2p_loans (
    id BIGSERIAL PRIMARY KEY,
    lender_id BIGINT NOT NULL,
    borrower_id BIGINT NOT NULL,
    principal BIGINT NOT NULL,
    interest_rate DECIMAL(5,2) NOT NULL, -- % 
    interest_total BIGINT NOT NULL,
    total_due BIGINT NOT NULL,
    collateral_type TEXT, -- nft, bkc
    collateral_value BIGINT, -- Стоимость залога
    collateral_nft_id INT, -- ID NFT в залоге
    term_days INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'requested', -- requested, active, repaid, defaulted, cancelled
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    accepted_at TIMESTAMPTZ,
    due_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,
    defaulted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS p2p_loans_lender_idx ON p2p_loans(lender_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS p2p_loans_borrower_idx ON p2p_loans(borrower_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS p2p_loans_collateral_idx ON p2p_loans(collateral_nft_id, status);

-- NFT каталог (виртуальные предметы)
CREATE TABLE IF NOT EXISTS nfts (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT,
    image_url TEXT NOT NULL,
    rarity TEXT NOT NULL DEFAULT 'common', -- common, rare, epic, legendary, mythic
    price_coins BIGINT NOT NULL,
    supply_total BIGINT NOT NULL,
    supply_left BIGINT NOT NULL,
    perks JSONB DEFAULT '{}', -- Привилегии: буст энергии, крит. тапы, снижение налогов
    is_tradeable BOOLEAN DEFAULT TRUE,
    is_collateral BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS nfts_rarity_idx ON nfts(rarity, price_coins);
CREATE INDEX IF NOT EXISTS nfts_supply_idx ON nfts(supply_left > 0, price_coins);

-- Барахолка (физические товары)
CREATE TABLE IF NOT EXISTS market_listings (
    id BIGSERIAL PRIMARY KEY,
    seller_id BIGINT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'other',
    price_coins BIGINT NOT NULL,
    contact_info TEXT NOT NULL, -- Как связаться с продавцом
    location TEXT, -- Город/страна для доставки
    condition TEXT DEFAULT 'used', -- new, used, refurbished
    images JSONB DEFAULT '[]', -- ID изображений
    status TEXT NOT NULL DEFAULT 'active', -- active, sold, cancelled, removed
    listing_fee_paid BIGINT DEFAULT 1000, -- Плата за размещение
    views_count INT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sold_at TIMESTAMPTZ,
    buyer_id BIGINT,
    moderated_by BIGINT, -- Админ, который проверил
    moderated_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS market_listings_status_idx ON market_listings(status, created_at DESC);
CREATE INDEX IF NOT EXISTS market_listings_seller_idx ON market_listings(seller_id, created_at DESC);
CREATE INDEX IF NOT EXISTS market_listings_category_idx ON market_listings(category, status);
CREATE INDEX IF NOT EXISTS market_listings_price_idx ON market_listings(price_coins, status);

-- Изображения для барахолки
CREATE TABLE IF NOT EXISTS market_images (
    id BIGSERIAL PRIMARY KEY,
    listing_id BIGINT NOT NULL REFERENCES market_listings(id) ON DELETE CASCADE,
    mime TEXT NOT NULL,
    data BYTEA NOT NULL,
    thumbnail BYTEA, -- Превью
    file_size INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS market_images_listing_idx ON market_images(listing_id, created_at DESC);

-- БАЗА 8-9: COCKROACHDB (ЛОГИ И АНАЛИТИКА)
-- =============================================================================

-- Детальные логи всех действий
CREATE TABLE IF NOT EXISTS activity_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT,
    session_id TEXT,
    action TEXT NOT NULL, -- tap, buy, sell, transfer, etc.
    entity_type TEXT, -- nft, loan, order, etc.
    entity_id BIGINT,
    amount BIGINT,
    currency TEXT, -- bkc, ton, usdt
    ip_address INET,
    user_agent TEXT,
    device_info JSONB,
    location_country TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS activity_logs_user_idx ON activity_logs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS activity_logs_action_idx ON activity_logs(action, created_at DESC);
CREATE INDEX IF NOT EXISTS activity_logs_ip_idx ON activity_logs(ip_address, created_at DESC);

-- Логи игр (Ракетка, etc)
CREATE TABLE IF NOT EXISTS game_logs (
    id BIGSERIAL PRIMARY KEY,
    game_type TEXT NOT NULL, -- crash, roulette, etc
    game_id TEXT NOT NULL,
    user_id BIGINT,
    bet_amount BIGINT,
    multiplier DECIMAL(10,4),
    win_amount BIGINT,
    result TEXT, -- win, lose, crash
    provablyfair_hash TEXT,
    provablyfair_salt TEXT,
    session_duration_ms INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS game_logs_user_idx ON game_logs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS game_logs_type_idx ON game_logs(game_type, created_at DESC);
CREATE INDEX IF NOT EXISTS game_logs_result_idx ON game_logs(result, created_at DESC);

-- Чаты/переписка в барахолке
CREATE TABLE IF NOT EXISTS market_chats (
    id BIGSERIAL PRIMARY KEY,
    listing_id BIGINT NOT NULL,
    sender_id BIGINT NOT NULL,
    receiver_id BIGINT NOT NULL,
    message TEXT NOT NULL,
    message_type TEXT DEFAULT 'text', -- text, image, location
    is_read BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS market_chats_listing_idx ON market_chats(listing_id, created_at DESC);
CREATE INDEX IF NOT EXISTS market_chats_participants_idx ON market_chats(sender_id, receiver_id, created_at DESC);

-- Аналитика и статистика
CREATE TABLE IF NOT EXISTS analytics_daily (
    date DATE PRIMARY KEY,
    active_users BIGINT DEFAULT 0,
    new_users BIGINT DEFAULT 0,
    total_taps BIGINT DEFAULT 0,
    total_burned BIGINT DEFAULT 0,
    total_transactions BIGINT DEFAULT 0,
    p2p_volume_bkc BIGINT DEFAULT 0,
    p2p_volume_usd DECIMAL(15,2) DEFAULT 0,
    nft_sales_count INT DEFAULT 0,
    nft_sales_volume BIGINT DEFAULT 0,
    loans_issued_count INT DEFAULT 0,
    loans_issued_volume BIGINT DEFAULT 0,
    revenue_burn_tax BIGINT DEFAULT 0, -- Доход от сжигания
    revenue_system_tax BIGINT DEFAULT 0, -- Доход системе
    revenue_fees BIGINT DEFAULT 0, -- Комиссии
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS analytics_daily_date_idx ON analytics_daily(date DESC);

-- Ошибки и инциденты
CREATE TABLE IF NOT EXISTS error_logs (
    id BIGSERIAL PRIMARY KEY,
    error_type TEXT NOT NULL, -- validation, system, security, etc
    severity TEXT NOT NULL DEFAULT 'error', -- warning, error, critical
    user_id BIGINT,
    request_path TEXT,
    request_method TEXT,
    error_message TEXT,
    stack_trace TEXT,
    ip_address INET,
    user_agent TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS error_logs_type_idx ON error_logs(error_type, created_at DESC);
CREATE INDEX IF NOT EXISTS error_logs_severity_idx ON error_logs(severity, created_at DESC);

-- БАЗА 10-15: REDIS (КЭШ И СИНХРОНИЗАЦИЯ)
-- =============================================================================

-- Redis структуры данных (создаются программно):
-- 
-- user:balance:{user_id} - текущий баланс в RAM
-- user:energy:{user_id} - текущая энергия
-- user:session:{user_id} - данные сессии
-- global:stats - глобальная статистика
-- anti:cheat:{user_id} - защита от читов
-- queue:taps - очередь тапов для обработки
-- queue:notifications - очередь уведомлений
-- leaderboard:taps - топ по тапам
-- leaderboard:balance - топ по балансу
-- online:users - онлайн пользователи
-- locks:user:{user_id} - локи для транзакций
-- cache:nft:catalog - кэш NFT каталога
-- cache:market:active - активные лоты на рынке
-- cache:system:config - конфигурация системы

-- Триггеры и процедуры для PostgreSQL
-- =============================================================================

-- Триггер для обновления updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Применяем ко всем таблицам с updated_at
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_system_state_updated_at BEFORE UPDATE ON system_state FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_user_daily_updated_at BEFORE UPDATE ON user_daily FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_market_listings_updated_at BEFORE UPDATE ON market_listings FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_analytics_daily_updated_at BEFORE UPDATE ON analytics_daily FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Процедура для обработки халвинга
CREATE OR REPLACE FUNCTION process_halving()
RETURNS void AS $$
DECLARE
    current_mined BIGINT;
    current_reward BIGINT;
    new_reward BIGINT;
BEGIN
    -- Получаем текущие значения
    SELECT total_mined, current_tap_reward INTO current_mined, current_reward
    FROM system_state WHERE id = 1;
    
    -- Проверяем нужен ли халвинг
    IF current_mined >= (current_halving + 1) * halving_threshold THEN
        new_reward := GREATEST(current_reward / 2, 1);
        
        UPDATE system_state 
        SET 
            current_tap_reward = new_reward,
            current_halving = current_halving + 1,
            updated_at = now()
        WHERE id = 1;
        
        -- Записываем в ledger
        INSERT INTO ledger(kind, amount, meta)
        VALUES ('halving', new_reward, jsonb_build_object(
            'old_reward', current_reward,
            'new_reward', new_reward,
            'halving_number', current_halving + 1
        ));
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Процедура для разблокировки замороженных монет
CREATE OR REPLACE FUNCTION unfrozen_vesting()
RETURNS void AS $$
DECLARE
    today DATE := CURRENT_DATE;
    unfreeze_amount BIGINT;
    schedule JSONB;
    month_number INT;
BEGIN
    SELECT unfrozen_schedule INTO schedule 
    FROM system_state WHERE id = 1;
    
    -- Проверяем разблокировку на текущий месяц
    month_number := EXTRACT(MONTH FROM age(today, created_at)) + 1;
    
    IF schedule ? month_number::text THEN
        unfreeze_amount := (schedule -> month_number::text) #>> '{}');
        
        IF unfreeze_amount > 0 THEN
            UPDATE system_state 
            SET 
                frozen_supply = frozen_supply - unfreeze_amount,
                reserve_supply = reserve_supply + unfreeze_amount,
                updated_at = now()
            WHERE id = 1;
            
            -- Записываем в ledger
            INSERT INTO ledger(kind, amount, meta)
            VALUES ('unfreeze', unfreeze_amount, jsonb_build_object(
                'month', month_number,
                'amount', unfreeze_amount
            ));
        END IF;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Вьюхи для удобной работы с данными
-- =============================================================================

-- Полная информация о пользователе
CREATE OR REPLACE VIEW user_full AS
SELECT 
    u.*,
    COALESCE(ref.total_bonus, 0) as referral_bonus_total,
    COALESCE(sub.plan_type, 'basic') as current_plan,
    CASE WHEN sub.expires_at > now() THEN true ELSE false END as is_premium_active,
    COALESCE(nft_count.nft_count, 0) as nft_owned_count,
    COALESCE(loan_total.total_debt, 0) as total_loan_debt
FROM users u
LEFT JOIN (
    SELECT referrer_id, SUM(bonus) as total_bonus
    FROM referrals 
    GROUP BY referrer_id
) ref ON u.user_id = ref.referrer_id
LEFT JOIN (
    SELECT user_id, plan_type, expires_at
    FROM subscriptions 
    WHERE is_active = true
) sub ON u.user_id = sub.user_id
LEFT JOIN (
    SELECT user_id, COUNT(*) as nft_count
    FROM user_nfts 
    GROUP BY user_id
) nft_count ON u.user_id = nft_count.user_id
LEFT JOIN (
    SELECT user_id, SUM(total_due) as total_debt
    FROM bank_loans 
    WHERE status = 'active'
    GROUP BY user_id
) loan_total ON u.user_id = loan_total.user_id;

-- Топ пользователей
CREATE OR REPLACE VIEW leaderboard_users AS
SELECT 
    user_id,
    username,
    balance,
    taps_total,
    level,
    referrals_count,
    RANK() OVER (ORDER BY balance DESC) as rank_balance,
    RANK() OVER (ORDER BY taps_total DESC) as rank_taps,
    RANK() OVER (ORDER BY level DESC) as rank_level,
    RANK() OVER (ORDER BY referrals_count DESC) as rank_referrals
FROM users 
WHERE is_admin = false AND banned_until IS NULL;

-- Экономическая статистика
CREATE OR REPLACE VIEW economy_stats AS
SELECT 
    total_supply,
    reserve_supply,
    frozen_supply,
    admin_allocated,
    total_mined,
    total_burned,
    current_tap_reward,
    current_halving,
    ROUND((total_burned::decimal / total_supply) * 100, 2) as burn_percentage,
    ROUND((reserve_supply::decimal / total_supply) * 100, 2) as reserve_percentage,
    ROUND((frozen_supply::decimal / total_supply) * 100, 2) as frozen_percentage
FROM system_state 
WHERE id = 1;

-- Активные P2P сделки
CREATE OR REPLACE VIEW p2p_active AS
SELECT 
    o.*,
    s.username as seller_name,
    b.username as buyer_name
FROM p2p_orders o
LEFT JOIN users s ON o.seller_id = s.user_id
LEFT JOIN users b ON o.buyer_id = b.user_id
WHERE o.status IN ('open', 'locked');

-- Просроченные кредиты
CREATE OR REPLACE VIEW overdue_loans AS
SELECT 
    l.*,
    u.username,
    u.balance as user_balance,
    CASE WHEN l.due_at < now() THEN true ELSE false END as is_overdue
FROM bank_loans l
JOIN users u ON l.user_id = u.user_id
WHERE l.status = 'active' AND l.due_at < now();

-- Начальные данные для системы
-- =============================================================================

-- Создаем админ пользователя (если его нет)
INSERT INTO users (user_id, username, first_name, balance, energy_max, is_admin, is_premium, premium_type)
VALUES (
    0, -- Будет заменен на реальный ADMIN_ID
    'admin', 
    'Administrator', 
    300_000_000, -- 30% от эмиссии
    10000,
    true, 
    true, 
    'gold'
) ON CONFLICT (user_id) DO NOTHING;

-- Создаем базовые NFT
INSERT INTO nfts (title, description, image_url, rarity, price_coins, supply_total, supply_left, perks)
VALUES 
    ('Energy Boost I', 'Увеличивает регенерацию энергии на 25%', '/assets/nfts/energy1.png', 'common', 5000, 10000, 10000, '{"energy_regen": 1.25}'),
    ('Tap Power I', 'Увеличивает силу тапа на 2', '/assets/nfts/tap1.png', 'common', 7500, 5000, 5000, '{"tap_power": 2}'),
    ('Critical Master', 'Шанс критического тапа x10', '/assets/nfts/critical.png', 'rare', 25000, 1000, 1000, '{"critical_chance": 0.05, "critical_multiplier": 10}'),
    ('Tax Shield', 'Снижает налог на 5%', '/assets/nfts/taxshield.png', 'epic', 50000, 500, 500, '{"tax_reduction": 5}'),
    ('Dragon NFT', 'Легендарный дракон с максимальными бонусами', '/assets/nfts/dragon.png', 'mythic', 500000, 100, 100, '{"energy_regen": 2, "tap_power": 5, "critical_chance": 0.1, "critical_multiplier": 15, "tax_reduction": 10}')
) ON CONFLICT DO NOTHING;

-- Создаем планы подписок
INSERT INTO subscriptions (user_id, plan_type, price_paid, expires_at)
VALUES 
    (0, 'basic', 0, '2030-01-01'),
    (0, 'silver', 50000, '2030-01-01'),
    (0, 'gold', 200000, '2030-01-01')
) ON CONFLICT DO NOTHING;

COMMIT;
