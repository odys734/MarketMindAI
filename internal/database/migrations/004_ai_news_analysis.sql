CREATE TABLE IF NOT EXISTS ai_news_analysis (
    company_id BIGINT NOT NULL,
    news_id BIGINT NOT NULL,

    summary TEXT NOT NULL,
    event_type VARCHAR(80) NOT NULL,

    sentiment VARCHAR(20) NOT NULL
        CHECK (sentiment IN (
            'positive', 'negative', 'mixed', 'neutral', 'unknown'
        )),

    impact_direction VARCHAR(20) NOT NULL
        CHECK (impact_direction IN (
            'positive', 'negative', 'mixed', 'neutral', 'unknown'
        )),

    impact_summary TEXT NOT NULL,

    confidence SMALLINT NOT NULL
        CHECK (confidence BETWEEN 0 AND 100),

    evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
    uncertainty TEXT NOT NULL,

    model VARCHAR(120) NOT NULL,
    analyzed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (company_id, news_id),

    FOREIGN KEY (company_id, news_id)
        REFERENCES company_news(company_id, news_id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_ai_news_sentiment
ON ai_news_analysis(company_id, sentiment);

CREATE INDEX IF NOT EXISTS idx_ai_news_analyzed
ON ai_news_analysis(analyzed_at DESC);
