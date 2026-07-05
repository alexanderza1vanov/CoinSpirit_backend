CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name VARCHAR(120) NOT NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'user',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS portfolios (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL,
    base_currency VARCHAR(8) NOT NULL DEFAULT 'USD',
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS asset_sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(120) NOT NULL,
    source_type VARCHAR(40) NOT NULL,
    base_url TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id UUID REFERENCES asset_sources(id) ON DELETE SET NULL,
    ticker VARCHAR(32) NOT NULL,
    name VARCHAR(255) NOT NULL,
    asset_type VARCHAR(32) NOT NULL,
    market_type VARCHAR(32) NOT NULL,
    currency_code VARCHAR(8) NOT NULL,
    isin VARCHAR(32),
    figi VARCHAR(32),
    lot_size NUMERIC(18,8) NOT NULL DEFAULT 1,
    is_active BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS portfolio_positions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    portfolio_id UUID NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE RESTRICT,
    quantity NUMERIC(24,8) NOT NULL DEFAULT 0,
    avg_buy_price NUMERIC(24,8) NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (portfolio_id, asset_id)
);

CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    portfolio_id UUID NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE RESTRICT,
    transaction_type VARCHAR(8) NOT NULL CHECK (transaction_type IN ('buy', 'sell')),
    quantity NUMERIC(24,8) NOT NULL CHECK (quantity > 0),
    unit_price NUMERIC(24,8) NOT NULL CHECK (unit_price >= 0),
    fee_amount NUMERIC(24,8) NOT NULL DEFAULT 0,
    total_amount NUMERIC(24,8) NOT NULL,
    transaction_at TIMESTAMPTZ NOT NULL,
    note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS price_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    timeframe VARCHAR(16) NOT NULL,
    open_price NUMERIC(24,8) NOT NULL,
    high_price NUMERIC(24,8) NOT NULL,
    low_price NUMERIC(24,8) NOT NULL,
    close_price NUMERIC(24,8) NOT NULL,
    volume NUMERIC(24,8) NOT NULL DEFAULT 0,
    recorded_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS indicators (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    indicator_type VARCHAR(32) NOT NULL,
    timeframe VARCHAR(16) NOT NULL,
    indicator_value NUMERIC(24,8) NOT NULL,
    meta_value_1 NUMERIC(24,8),
    meta_value_2 NUMERIC(24,8),
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    portfolio_id UUID REFERENCES portfolios(id) ON DELETE CASCADE,
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    rule_type VARCHAR(32) NOT NULL,
    target_value NUMERIC(24,8) NOT NULL,
    comparison_operator VARCHAR(8) NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS alert_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_rule_id UUID NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    trigger_value NUMERIC(24,8) NOT NULL,
    event_status VARCHAR(32) NOT NULL DEFAULT 'created',
    message TEXT NOT NULL,
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS notification_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_event_id UUID NOT NULL REFERENCES alert_events(id) ON DELETE CASCADE,
    channel VARCHAR(32) NOT NULL,
    recipient VARCHAR(255) NOT NULL,
    delivery_status VARCHAR(32) NOT NULL DEFAULT 'pending',
    provider_response TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_portfolios_user_id ON portfolios(user_id);
CREATE INDEX IF NOT EXISTS idx_portfolios_created_at ON portfolios(created_at);
CREATE INDEX IF NOT EXISTS idx_assets_ticker ON assets(ticker);
CREATE INDEX IF NOT EXISTS idx_assets_source_id ON assets(source_id);
CREATE INDEX IF NOT EXISTS idx_positions_portfolio_id ON portfolio_positions(portfolio_id);
CREATE INDEX IF NOT EXISTS idx_positions_asset_id ON portfolio_positions(asset_id);
CREATE INDEX IF NOT EXISTS idx_transactions_portfolio_id ON transactions(portfolio_id);
CREATE INDEX IF NOT EXISTS idx_transactions_asset_id ON transactions(asset_id);
CREATE INDEX IF NOT EXISTS idx_transactions_created_at ON transactions(created_at);
CREATE INDEX IF NOT EXISTS idx_price_history_asset_id ON price_history(asset_id);
CREATE INDEX IF NOT EXISTS idx_price_history_recorded_at ON price_history(recorded_at);
CREATE INDEX IF NOT EXISTS idx_indicators_asset_id ON indicators(asset_id);
CREATE INDEX IF NOT EXISTS idx_alert_rules_user_id ON alert_rules(user_id);
CREATE INDEX IF NOT EXISTS idx_alert_rules_portfolio_id ON alert_rules(portfolio_id);
CREATE INDEX IF NOT EXISTS idx_alert_rules_asset_id ON alert_rules(asset_id);
CREATE INDEX IF NOT EXISTS idx_alert_events_created ON alert_events(triggered_at);
