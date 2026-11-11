CREATE TABLE IF NOT EXISTS market_prices (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    company_id BIGINT NOT NULL
        REFERENCES companies(id) ON DELETE CASCADE,

    symbol VARCHAR(50) NOT NULL,
    exchange VARCHAR(20) NOT NULL,
    trading_date DATE NOT NULL,

    open NUMERIC(20,4) NOT NULL,
    high NUMERIC(20,4) NOT NULL,
    low NUMERIC(20,4) NOT NULL,
    close NUMERIC(20,4) NOT NULL,

    last NUMERIC(20,4),
    prev_close NUMERIC(20,4),

    volume BIGINT NOT NULL DEFAULT 0,
    turnover NUMERIC(24,4) NOT NULL DEFAULT 0,
    trades BIGINT NOT NULL DEFAULT 0,

    provider VARCHAR(80) NOT NULL,
    source_published_at TIMESTAMPTZ,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT market_prices_unique
        UNIQUE (company_id, trading_date, provider),

    CONSTRAINT market_prices_valid_prices
        CHECK (
            open > 0 AND high > 0 AND low > 0 AND close > 0
            AND high >= low
            AND open BETWEEN low AND high
            AND close BETWEEN low AND high
        ),

    CONSTRAINT market_prices_nonnegative_volume
        CHECK (volume >= 0 AND turnover >= 0 AND trades >= 0)
);

CREATE INDEX IF NOT EXISTS idx_market_prices_company_date
    ON market_prices (company_id, trading_date DESC);

CREATE INDEX IF NOT EXISTS idx_market_prices_symbol_exchange
    ON market_prices (symbol, exchange, trading_date DESC);
