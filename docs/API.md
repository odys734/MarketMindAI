# MarketMind AI — API Reference

**Base URL:** `http://127.0.0.1:8080/api/v1`

All endpoints use JSON unless otherwise stated.

## General conventions

- Company and index IDs must be positive integers.
- Dates use `YYYY-MM-DD`.
- Historical price endpoints return stored records, not live quotes.
- Unless specified otherwise, errors return JSON containing an `error` field.
- The API currently has no authentication middleware. Do not expose it publicly without adding authentication and access controls.

## Health

### GET /health

Checks API and database health.

Example:

```bash
curl http://127.0.0.1:8080/api/v1/health
```

### Companies

### POST /companies

Creates a company.

Request:

```json
{
  "name": "Company Name",
  "symbol": "SYMBOL",
  "exchange": "NSE",
  "country": "India",
  "sector": "Technology",
  "industry": "Software",
  "currency": "INR",
  "website": "https://example.com"
}
```

Required: `name`, `symbol`, `exchange`, `country`.

Success: `201 Created`, with the created company in `data`.

Possible errors: `400` invalid input, `409` duplicate symbol/exchange.

### GET /companies

Lists companies.

Query parameters:

| Parameter | Default | Description       |
| --------- | ------- | ----------------- |
| limit     | 20      | 1–100             |
| offset    | 0       | Pagination offset |
| search    | —       | Company search    |
| exchange  | —       | Exchange filter   |
| country   | —       | Country filter    |

Example:

```bash
curl "http://127.0.0.1:8080/api/v1/companies?limit=20&offset=0"
```

Response contains `data`, `limit`, and `offset`.

### GET /companies/:id

Returns one company.

### PUT /companies/:id

Updates a company using the company request fields.

### DELETE /companies/:id

Deletes a company. Success returns `204 No Content`.

Invalid IDs return `400`; missing companies return `404`.

### News

### POST /news/collect

Collects news using GDELT DOC 2.0 and associates articles with a company.

Query parameters:

| Parameter  | Required | Description                            |
| ---------- | -------- | -------------------------------------- |
| company_id | Yes      | Positive company ID                    |
| query      | No       | Search query; defaults to company name |

Example:

```bash
curl -X POST \
  "http://127.0.0.1:8080/api/v1/news/collect?company_id=1"
```

The current collector requests up to 20 articles over a seven-day window. Requests are spaced by at least five seconds in the running process.

Success response includes:

* `status`

* `company`

* `query`

* `articles_processed`

* `new_company_links`

* `provider`

* `message`

Possible errors: `400`, `404`, `429`, `502`, `500`.

Data caveat: GDELT's `seendate` is an observation date and should not automatically be interpreted as the publisher's verified publication timestamp.

### GET /companies/:id/news

Returns stored news for a company.

| Parameter | Default | Range |
| --------- | ------- | ----- |
| limit     | 20      | 1–100 |

Example:

```bash
curl "http://127.0.0.1:8080/api/v1/companies/1/news?limit=20"
```

Response contains `company_id`, `count`, and `data`.

Article records may include title, URL, domain, language, source country, timestamps, relevance category, relevance score, and relevance reason.

### AI news analysis

### POST /companies/:id/news/analyze

Runs the company's news relevance analysis.

### POST /companies/:id/news/:news_id/analyze-ai

Runs AI analysis for a particular linked news article.

### POST /companies/:id/analyze-ai-batch

Runs AI analysis on a limited batch of relevant, not-yet-analyzed articles.

| Parameter | Default | Range |
| --------- | ------- | ----- |
| limit     | 5       | 1–5   |

Example:

```bash
curl -X POST \
  "http://127.0.0.1:8080/api/v1/companies/1/analyze-ai-batch?limit=5"
```

The batch selects direct, sector, and macro relevance categories. Already analyzed articles are excluded.

### GET /companies/:id/news/:news_id/analysis-ai

Returns the saved AI analysis for a linked article.

### GET /companies/:id/news/analyses-ai

Returns saved AI analyses for a company.

| Parameter | Default | Range        |
| --------- | ------- | ------------ |
| limit     | 20      | 1–100        |
| offset    | 0       | 0 or greater |

### GET /companies/:id/intelligence

Returns the company's combined intelligence view.

### GET /companies/:id/sentiment-history

Returns historical counts of saved AI news sentiment classifications.

| Parameter | Default | Range |
| --------- | ------- | ----- |
| days      | 30      | 1–365 |

The sentiment counts describe analyzed news, not stock returns or price performance.

### Company historical market data

### POST /companies/:id/prices/sync

Fetches and stores historical EOD prices through TejHQ.

Required query parameters:

| Parameter | Format     |
| --------- | ---------- |
| from      | YYYY-MM-DD |
| to        | YYYY-MM-DD |

Maximum requested range: 365 days. Currently supported exchanges: NSE and BSE.

Example:

```bash
curl -X POST \
  "http://127.0.0.1:8080/api/v1/companies/1/prices/sync?from=2026-11-01&to=2026-11-10"
```

Success response includes company, symbol, exchange, provider, data type, latest trading date, provider publication timestamp, requested dates, and records saved.

Provider: `TejHQ`.

### GET /companies/:id/prices

Returns stored historical company prices.

| Parameter | Default | Description         |
| --------- | ------- | ------------------- |
| from      | —       | Optional start date |
| to        | —       | Optional end date   |
| limit     | 100     | 1–500               |

Example:

```bash
curl \
  "http://127.0.0.1:8080/api/v1/companies/1/prices?from=2026-11-01&to=2026-11-10&limit=100"
```

Response includes company, symbol, exchange, `data_type`, count, requested date filters, and `data`.

Price records include trading date, OHLC, last, previous close, daily change percentage, volume, turnover, trades, provider, and fetched timestamp.

### Market indices

### POST /indices/:id/prices/sync

Fetches and stores historical daily index prices using the Yahoo Finance chart endpoint.

Required query parameters: `from`, `to`, both in `YYYY-MM-DD` format.

Maximum requested range: 365 days.

Example:

```bash
curl -X POST \
  "http://127.0.0.1:8080/api/v1/indices/1/prices/sync?from=2026-11-01&to=2026-11-10"
```

Success response includes index, symbol, provider symbol, exchange, currency, provider, data type, requested dates, and records saved.

### GET /indices/:id/prices

Returns stored historical daily index prices.

| Parameter | Default | Description         |
| --------- | ------- | ------------------- |
| from      | —       | Optional start date |
| to        | —       | Optional end date   |
| limit     | 100     | 1–1000              |

Example:

```bash
curl \
  "http://127.0.0.1:8080/api/v1/indices/1/prices?limit=100"
```

Response includes index, symbol, exchange, currency, count, date filters, limit, and `data`.

Each price record includes trading date, OHLC, daily change percentage, nullable volume, provider, and fetched timestamp.

### Analytics and comparisons

### GET /companies/:id/market-analytics

Returns stored company market analytics.

`days`: default 30; valid range 1–365.

### GET /indices/:id/analytics

Returns stored index analytics.

`days`: default 30; valid range 2–3650.

### GET /indices/compare

Compares two different indices.

Required: `left_id`, `right_id`.

Optional: `days`, default 30; valid range 2–3650.

Example:

```bash
curl \
  "http://127.0.0.1:8080/api/v1/indices/compare?left_id=1&right_id=2&days=30"
```

### GET /companies/:id/compare/indices/:index_id

Compares a company's stored historical performance with an index.

`days`: default 30; valid range 2–3650.

### News and historical price observations

### GET /companies/:id/news-price-correlation

Returns observations pairing analyzed company news with nearby historical stock-price sessions.

Query parameters:

| Parameter | Default | Range |
|---|---:|---:|
| `limit` | 50 | 1–100 |

Example:

```bash
curl "http://127.0.0.1:8080/api/v1/companies/1/news-price-correlation?limit=50"
```

Notes:

- News dates currently represent GDELT observation dates, not verified publication timestamps.
- A price movement after an observation does not establish that the news caused it.
- Missing event-session prices are returned as empty values.
- Small samples cannot establish reliable statistical correlation.
- This endpoint is an exploratory historical comparison, not a prediction or investment recommendation.

## Market overview and data quality

### GET /market/overview

Returns a summary based on stored market observations.

```bash
curl http://127.0.0.1:8080/api/v1/market/overview
```

This is a stored historical-data overview, not a live market feed.

### GET /market/data-quality

Returns data-quality checks for stored market records.

```bash
curl http://127.0.0.1:8080/api/v1/market/data-quality
```

Current checks include unusually large daily moves, calendar gaps, and dates reported by multiple providers. These checks identify records for review; they do not certify source accuracy.

### Common HTTP status codes

| Status | Meaning                                 |
| ------ | --------------------------------------- |
| 200    | Request successful                      |
| 201    | Resource created                        |
| 204    | Resource deleted successfully           |
| 400    | Invalid request or parameters           |
| 404    | Resource not found                      |
| 409    | Duplicate/conflicting resource          |
| 429    | Provider rate limit                     |
| 500    | Internal server/database error          |
| 502    | External provider or AI gateway failure |

### Data source limitations

* GDELT DOC 2.0: News discovery; observation timestamps are not necessarily verified publication timestamps.

* TejHQ: Historical NSE/BSE stock EOD data. Provider status metadata may contain a publication timestamp.

* Yahoo Finance chart endpoint: Unofficial historical index data. Index volume can be unavailable or incomparable with stock volume.

* AI gateway: AI-generated summaries and classifications can be incomplete or incorrect. Review source articles and uncertainty fields.

### Security and deployment notes

* The current API does not register authentication middleware.

* Do not expose the development API directly to the public internet.

* Review database SSL settings before using a remote database.

* Keep API keys and credentials in environment variables, never in source control.

* Apply request authentication, authorization, rate limits, and input validation before production deployment.

### Educational-use notice

MarketMind AI is a historical financial research and educational project. Historical correlations, news sentiment, AI summaries, and past price movements do not establish causation or predict future results. Nothing in this API is a personalized investment recommendation.

## Historical Strategy Backtesting

### POST /companies/:id/backtest

Runs an educational historical SMA crossover simulation using stored company EOD price records.

This endpoint does not place real orders, connect to a broker, or provide investment recommendations.

**Request body:**

```json
{
  "from": "2026-01-01",
  "to": "2026-11-09",
  "fast_period": 10,
  "slow_period": 30,
  "initial_capital": 100000,
  "commission_bps": 10,
  "slippage_bps": 5,
  "annual_risk_free_rate": 0.05,
  "provider": "tejhq"
}
```

**Simulation behavior:**

- Simulated execution occurs at the next available trading session's open.
- Entry and exit slippage and commission are included in the hypothetical portfolio calculation.
- The simulator supports long/flat positions, whole shares, and no leverage or short selling.
- An open position at the end is marked to the last available closing price; it is not counted as a closed trade.
- The engine detects crossovers after its first valid SMA comparison. It does not automatically enter a position simply because the fast SMA is already above the slow SMA at the first valid comparison.

**Example:**

```bash
curl -X POST \
  "http://127.0.0.1:8080/api/v1/companies/1/backtest" \
  -H "Content-Type: application/json" \
  -d '{
    "from": "2026-01-01",
    "to": "2026-11-09",
    "fast_period": 10,
    "slow_period": 30,
    "initial_capital": 100000,
    "commission_bps": 10,
    "slippage_bps": 5,
    "annual_risk_free_rate": 0.05,
    "provider": "tejhq"
  }'
```

**Responses:**

| Status | Meaning |
|---|---|
| 200 | Backtest completed |
| 400 | Invalid request, parameters or session limit |
| 404 | Company not found |
| 409 | Duplicate-date provider records or mixed providers |
| 422 | No usable historical data or strategy/simulation validation failure |
| 500 | Database or stored-data error |

> Historical backtests are hypothetical. Results depend on data quality, strategy assumptions, transaction costs and sample period. They do not predict future performance or constitute personalized investment advice.


## Historical Model Forecast

### POST /companies/:id/forecast

Generates an experimental multi-session forecast from the company's stored historical EOD prices.

**Request body:**

```json
{
  "horizon_sessions": 5,
  "provider": "tejhq",
  "ridge_lambda": 1.0
}
```

- horizon_sessions is required and must be 1, 5, 10 or 20 trading sessions.
- provider is optional. If omitted, the stored history must resolve to one provider; mixed-provider history returns 409. Specify a provider to select one consistent series.
- ridge_lambda is optional and defaults to 1.0; accepted values are finite, positive and no greater than 1,000,000.
- The endpoint uses at most 5,000 historical sessions.

The response includes the latest stored session date and close, provider, dataset row count, Ridge model metadata and three model outputs: Ridge Regression, Zero Return and Historical Mean. Each output includes a predicted simple return and, when mathematically valid, an indicative price calculated from the latest stored close.

**Responses:**

| Status | Meaning |
|---|---|
| 200 | Forecast generated |
| 400 | Invalid company ID, JSON or forecast configuration |
| 404 | Company not found |
| 409 | Duplicate dates or mixed providers |
| 422 | Insufficient history, invalid stored data or unavailable model features |
| 500 | Database or stored-data error |

The endpoint fits Ridge Regression using historical rows whose forward-return labels are available by the latest stored session, then predicts from technical features at that latest session. It does not return calibrated probabilities, confidence intervals or walk-forward accuracy metrics. The baseline models are comparisons, not evidence that any model has predictive skill.

> Forecast outputs are experimental historical-data model estimates, not guaranteed prices, trading signals or personalized investment advice.


## Walk-forward Forecast Evaluation

### POST /companies/:id/forecast/evaluate

Runs a chronological, expanding-window out-of-sample comparison for Ridge Regression, Zero Return and Historical Mean using stored historical EOD prices. Unlike the single forecast endpoint, this endpoint evaluates predictions against subsequently observed forward returns.

**Request body:**

```json
{
  "provider": "tejhq",
  "horizons": [1, 5, 10, 20],
  "initial_train_size": 80,
  "minimum_train_size": 30,
  "test_size": 20,
  "step_size": 20,
  "ridge_lambda": 1.0
}
```

All fields are optional:
- `provider`: selects one consistent stored data provider. If omitted, mixed-provider or duplicate-date data returns 409.
- `horizons`: one or more unique values from 1, 5, 10 and 20. Defaults to all four.
- `initial_train_size`: initial number of dataset rows before the first test fold; defaults to 80.
- `minimum_train_size`: minimum rows remaining after purging overlapping target labels; defaults to 30.
- `test_size`: observations per test fold; defaults to 20.
- `step_size`: distance between test fold starts; defaults to `test_size`. Overlapping test windows are rejected.
- `ridge_lambda`: positive finite Ridge regularization; defaults to 1.0.

The endpoint processes at most 5,000 historical sessions and requires at least 100 sessions. Each fold trains only on earlier observations; training labels whose target dates reach or overlap the test period are purged. Ridge preprocessing is fitted inside each training fold. Model metrics are calculated from aligned out-of-sample observations only.

The experiment report includes each horizon's dataset date range, complete/incomplete row counts, model comparisons, fold counts, MAE, RMSE, mean error, directional accuracy and fold diagnostics. It does not automatically select a winning model.

**Example:**

```bash
curl -X POST \
  "http://127.0.0.1:8080/api/v1/companies/1/forecast/evaluate" \
  -H "Content-Type: application/json" \
  -d '{"provider":"tejhq","horizons":[5,20],"initial_train_size":80,"minimum_train_size":30,"test_size":20,"step_size":20,"ridge_lambda":1.0}'
```

**Responses:**

| Status | Meaning |
|---|---|
| 200 | Walk-forward evaluation completed |
| 400 | Invalid ID, JSON, horizons or validation configuration |
| 404 | Company not found |
| 409 | Duplicate dates or mixed providers |
| 422 | Insufficient history or a fold/model could not be evaluated |
| 500 | Database or stored-data error |

> Historical out-of-sample performance is descriptive, not a guarantee of future accuracy. A lower error in one sample does not establish predictive skill or provide an investment recommendation.
