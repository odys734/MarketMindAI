ALTER TABLE company_news
ADD COLUMN IF NOT EXISTS relevance_category VARCHAR(20)
    NOT NULL DEFAULT 'unreviewed'
    CHECK (
        relevance_category IN (
            'direct',
            'sector',
            'macro',
            'unrelated',
            'unreviewed'
        )
    );

ALTER TABLE company_news
ADD COLUMN IF NOT EXISTS relevance_score SMALLINT
    NOT NULL DEFAULT 0
    CHECK (relevance_score BETWEEN 0 AND 100);

ALTER TABLE company_news
ADD COLUMN IF NOT EXISTS relevance_reason TEXT;

ALTER TABLE company_news
ADD COLUMN IF NOT EXISTS analyzed_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_company_news_relevance
ON company_news (company_id, relevance_category, relevance_score DESC);
