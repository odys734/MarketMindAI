-- Market index master data
CREATE TABLE IF NOT EXISTS market_indices (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    name VARCHAR(100) NOT NULL,
    symbol VARCHAR(50) NOT NULL UNIQUE,
    provider_symbol VARCHAR(50) NOT NULL,

    exchange VARCHAR(20) NOT NULL,
    country VARCHAR(80) NOT NULL,
    currency VARCHAR(10) NOT NULL,

    provider VARCHAR(80) NOT NULL,
    instrument_type VARCHAR(30) NOT NULL DEFAULT 'equity_index',

    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Historical daily index prices
CREATE TABLE IF NOT EXISTS index_prices (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    index_id BIGINT NOT NULL
        REFERENCES market_indices(id) ON DELETE CASCADE,

    trading_date DATE NOT NULL,

    open NUMERIC(20,4) NOT NULL,
    high NUMERIC(20,4) NOT NULL,
    low NUMERIC(20,4) NOT NULL,
    close NUMERIC(20,4) NOT NULL,

    volume BIGINT,
    provider VARCHAR(80) NOT NULL,

    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT index_prices_unique
        UNIQUE (index_id, trading_date, provider),

    CONSTRAINT index_prices_valid_ohlc
        CHECK (
            open > 0 AND high > 0 AND low > 0 AND close > 0
            AND high >= low
            AND open BETWEEN low AND high
            AND close BETWEEN low AND high
        ),

    CONSTRAINT index_prices_nonnegative_volume
        CHECK (volume IS NULL OR volume >= 0)
);

CREATE INDEX IF NOT EXISTS idx_index_prices_index_date
    ON index_prices (index_id, trading_date DESC);

CREATE INDEX IF NOT EXISTS idx_market_indices_exchange
    ON market_indices (exchange, country);

-- Initial supported indices
INSERT INTO market_indices (
    name, symbol, provider_symbol,
    exchange, country, currency,
    provider, instrument_type
)
VALUES
(
    'NIFTY 50', 'NIFTY50', '^NSEI',
    'NSE', 'India', 'INR',
    'yahoo_chart', 'equity_index'
),
(
    'SENSEX', 'SENSEX', '^BSESN',
    'BSE', 'India', 'INR',
    'yahoo_chart', 'equity_index'
)
ON CONFLICT (symbol) DO UPDATE SET
    name = EXCLUDED.name,
    provider_symbol = EXCLUDED.provider_symbol,
    exchange = EXCLUDED.exchange,
    country = EXCLUDED.country,
    currency = EXCLUDED.currency,
    provider = EXCLUDED.provider,
    instrument_type = EXCLUDED.instrument_type,
    updated_at = NOW();
