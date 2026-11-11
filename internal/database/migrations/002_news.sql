CREATE TABLE IF NOT EXISTS news_articles (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title TEXT NOT NULL,
    url TEXT NOT NULL UNIQUE,
    domain VARCHAR(255),
    language VARCHAR(30),
    source_country VARCHAR(10),
    published_at TIMESTAMPTZ,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS company_news (
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    news_id BIGINT NOT NULL REFERENCES news_articles(id) ON DELETE CASCADE,
    matched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (company_id, news_id)
);

CREATE INDEX IF NOT EXISTS idx_news_published
ON news_articles(published_at DESC);

CREATE INDEX IF NOT EXISTS idx_company_news_company
ON company_news(company_id);
