# MarketMind AI

**MarketMind AI** is a historical financial research and market intelligence platform built with Go, Gin and PostgreSQL.

It combines company information, news collection, AI-assisted news interpretation, historical market data, analytics, index comparisons and data-quality reporting.

> **Important:** This project is intended for educational and historical research only. It does not provide investment advice, buy/sell recommendations, guaranteed predictions or live market quotes.

---

## 1. Features

### Company Management
- Create, list, retrieve, update and delete companies.
- Search and filter companies by name, symbol, exchange and country.
- Store sector, industry, currency and website metadata.

### News Intelligence
- Collect news using GDELT DOC 2.0.
- Associate articles with companies.
- Classify news as direct, sector, macro, unrelated or unreviewed.
- Store relevance scores and explanations.

### AI News Analysis
- Analyze individual news articles.
- Batch-analyze pending company news.
- Store summaries, event types, sentiment, impact interpretation, evidence and uncertainty.
- Retrieve previous AI analyses and intelligence summaries.
- Maintain sentiment history.

### Historical Market Data
- Retrieve historical company prices through TejHQ.
- Retrieve historical index data through the Yahoo Finance chart endpoint.
- Calculate historical analytics.
- Compare indices using base-100 normalization.
- Compare a company with a market index.
- Retrieve a combined market overview.

### Data Quality
- Flag unusually large daily movements.
- Identify long calendar gaps.
- Identify dates with multiple providers.
- Review provider and fetch metadata.

Data-quality flags are review indicators, not automatic proof of incorrect data.

---

## 2. Technology Stack

| Component | Technology |
|---|---|
| Backend language | Go |
| HTTP framework | Gin |
| Database | PostgreSQL |
| Database driver | pgx |
| AI integration | OpenAI-compatible Chat Completions API |
| News source | GDELT DOC 2.0 |
| Stock historical data | TejHQ |
| Index historical data | Yahoo Finance chart endpoint |

---

## 3. Project Structure

```text
MarketMindAI/
├── cmd/
│   ├── api/
│   │   └── main.go
│   └── indexcheck/
│       └── main.go
├── internal/
│   ├── ai/
│   ├── config/
│   ├── database/
│   │   └── migrations/
│   ├── handlers/
│   ├── marketdata/
│   ├── models/
│   └── routes/
├── docs/
│   └── API.md
├── go.mod
├── go.sum
├── marketmind
└── README.md
```

---

## 4. Requirements

- Go compatible with the project's `go.mod`.
- PostgreSQL.
- An OpenAI-compatible AI gateway for AI analysis.
- Internet access for external news and historical-data providers.
- Termux is supported as a development environment.

---

## 5. Environment Configuration

The application reads configuration from environment variables.

| Variable | Default | Description |
|---|---|---|
| `HOST` | `127.0.0.1` | HTTP bind address; localhost by default |
| `PORT` | `8080` | HTTP server port |
| `APP_ENV` | `development` | Environment label |
| `DB_HOST` | `$PREFIX/tmp` | PostgreSQL host/socket directory |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `$USER` | PostgreSQL user |
| `DB_NAME` | `marketmind` | Database name |
| `DB_SSL_MODE` | `disable` | PostgreSQL SSL mode |
| `AI_BASE_URL` | `http://127.0.0.1:8081/v1` | AI gateway base URL |
| `AI_MODEL` | `gemini-3.5-flash` | AI model identifier |
| `AI_API_KEY` | Empty | Optional gateway API key |
| `AI_TIMEOUT_SECONDS` | `45` | AI request timeout |

For remote PostgreSQL connections, prefer `verify-full` with a valid CA certificate and matching hostname. `disable` is intended for local Termux PostgreSQL.

The API binds to `127.0.0.1` by default. Only set `HOST=0.0.0.0` when network access is deliberately required and suitable API authentication and network protections are in place.

Set `APP_ENV=production` to enable Gin release mode.

Invalid or less-than-one AI timeout values fall back to 45 seconds.

---

## 6. Running in Termux

### Step 1: Start PostgreSQL

```bash
pg_ctl -D "$PREFIX/var/lib/postgresql" \
  -l "$PREFIX/var/lib/postgresql/logfile" start
```

### Step 2: Configure environment

```bash
cd ~/MarketMindAI

export HOST="127.0.0.1"
export APP_ENV="development"
export DB_USER="$(id -un)"
export DB_HOST="$PREFIX/tmp"
export DB_PORT="5432"
export DB_NAME="marketmind"
export DB_SSL_MODE="disable"

export AI_BASE_URL="http://127.0.0.1:8081/v1"
export AI_MODEL="gemini-3.5-flash"
export AI_TIMEOUT_SECONDS="45"
```

Set `AI_API_KEY` if your gateway requires authentication.

### Step 3: Check PostgreSQL

```bash
psql -h "$DB_HOST" \
  -U "$DB_USER" \
  -d "$DB_NAME" \
  -c "\dt"
```

### Step 4: Build

```bash
go build -o marketmind ./cmd/api
```

### Step 5: Start the API

```bash
./marketmind
```

The API listens on the configured port, defaulting to `8080`.

Keep the AI gateway running separately when AI analysis is required.

---

## 7. Testing and Code Quality

Run the complete Go test suite:

```bash
go test ./...
```

Run static analysis:

```bash
go vet ./...
```

Build all packages:

```bash
go build ./...
```

The current test suite includes coverage for configuration, AI client behavior, historical-data providers, index analytics, comparisons, market overview and data-quality reporting.

Some packages do not yet have dedicated tests. A successful test run does not mean every code path has been tested.

---

## 8. API Overview

Base URL for local development:

```text
http://127.0.0.1:8080
```

API prefix:

```text
/api/v1
```

### Health and Root

| Method | Endpoint | Purpose |
|---|---|---|
| GET | `/health` | Health check |
| GET | `/` | Project information |
| GET | `/api/v1/health` | Versioned health check |

### Company Management

| Method | Endpoint | Purpose |
|---|---|---|
| POST | `/api/v1/companies` | Create company |
| GET | `/api/v1/companies` | List companies |
| GET | `/api/v1/companies/:id` | Retrieve company |
| PUT | `/api/v1/companies/:id` | Update company |
| DELETE | `/api/v1/companies/:id` | Delete company |

### News and AI

| Method | Endpoint | Purpose |
|---|---|---|
| POST | `/api/v1/news/collect` | Collect news |
| GET | `/api/v1/companies/:id/news` | List company news |
| POST | `/api/v1/companies/:id/news/analyze` | Analyze news relevance |
| POST | `/api/v1/companies/:id/news/:news_id/analyze-ai` | Analyze one article with AI |
| POST | `/api/v1/companies/:id/analyze-ai-batch` | Batch AI analysis |
| GET | `/api/v1/companies/:id/news/:news_id/analysis-ai` | Retrieve one AI analysis |
| GET | `/api/v1/companies/:id/news/analyses-ai` | List AI analyses |
| GET | `/api/v1/companies/:id/intelligence` | Company intelligence |
| GET | `/api/v1/companies/:id/sentiment-history` | Sentiment history |

### Historical Stock Data

| Method | Endpoint | Purpose |
|---|---|---|
| POST | `/api/v1/companies/:id/prices/sync` | Sync historical prices |
| GET | `/api/v1/companies/:id/prices` | Retrieve historical prices |
| GET | `/api/v1/companies/:id/market-analytics` | Historical stock analytics |
| GET | `/api/v1/companies/:id/news-price-correlation` | Descriptive news-price analysis |

### Historical Strategy Backtesting

| Method | Endpoint | Purpose |
|---|---|---|
| POST | `/api/v1/companies/:id/backtest` | Run a hypothetical historical SMA crossover simulation |
| POST | `/api/v1/companies/:id/forecast` | Generate experimental 1, 5, 10 or 20-session model forecasts |

The backtesting engine includes next-session execution, simulated commission and slippage, portfolio equity tracking, and historical performance metrics. It does not place real orders or provide investment recommendations.

### Historical Index Data

| Method | Endpoint | Purpose |
|---|---|---|
| POST | `/api/v1/indices/:id/prices/sync` | Sync historical index prices |
| GET | `/api/v1/indices/:id/prices` | Retrieve historical index prices |
| GET | `/api/v1/indices/:id/analytics` | Index analytics |
| GET | `/api/v1/indices/compare` | Compare two indices |
| GET | `/api/v1/companies/:id/compare/indices/:index_id` | Compare company and index |

### Market Monitoring

| Method | Endpoint | Purpose |
|---|---|---|
| GET | `/api/v1/market/overview` | Combined market overview |
| GET | `/api/v1/market/data-quality` | Data-quality report |

See [docs/API.md](docs/API.md) for detailed endpoint documentation.

---

## 9. Database

The current database contains seven tables:

| Table | Purpose |
|---|---|
| `companies` | Company master records |
| `news_articles` | Collected news articles |
| `company_news` | Company-news mapping and relevance |
| `ai_news_analysis` | AI analysis results |
| `market_prices` | Historical stock prices |
| `market_indices` | Index master records |
| `index_prices` | Historical index prices |

Migration files currently present:

- `002_news.sql`
- `003_news_relevance.sql`
- `004_ai_news_analysis.sql`
- `005_market_prices.sql`
- `006_market_indices.sql`

The base `companies` table must exist before applying the news migration.

Important database constraints include:

- Unique news article URLs.
- Unique company-news relationships.
- Valid sentiment and impact categories.
- Confidence scores between 0 and 100.
- OHLC consistency checks.
- Non-negative volume and turnover.
- Unique instrument, date and provider combinations.

---

## 10. Data Sources and Limitations

### GDELT DOC 2.0
Used for news collection. Its `seendate` is an observation timestamp and should not automatically be interpreted as the original publication time.

### TejHQ
Used for historical company price records. Provider data should be reviewed independently.

### Yahoo Finance Chart Endpoint
Used for historical index OHLC data. This is an unofficial integration, not an official exchange data feed.

### Historical Data
The application reports stored historical records, not live market quotes. Missing dates can reflect weekends, exchange holidays or unavailable provider records.

### Analytics
- Historical returns describe past observations, not future performance.
- Sentiment is an interpretation of news, not a forecast of stock movement.
- Correlation does not establish causation.
- Provider-reported index volume may not represent standardized traded volume.
- Comparisons use available common historical dates.

---

## 11. Security and Deployment Notes

- Authentication middleware is not currently registered in the API routes.
- Do not expose this API publicly without authentication, authorization and rate limiting.
- The current database connection uses `sslmode=disable`; review this before remote database deployment.
- Keep API keys out of source code and version control.
- Configure appropriate firewall and network access restrictions.
- Review database backups, logging and provider terms before production deployment.

---

## Forecasting Research

The forecasting package currently provides:

- Historical feature engineering using returns, moving averages, RSI, MACD and volatility.
- Forward-return targets for 1, 5, 10 and 20 sessions.
- Zero-return and historical-mean baseline models.
- Ridge Regression using eight normalized technical features.
- Expanding-window walk-forward evaluation with purging of training labels that overlap the test period.
- Out-of-sample MAE, RMSE, mean error and directional accuracy; optional probability and interval metrics are supported by the evaluation layer.
- Fold diagnostics including training/test observation counts, purged observations and date boundaries.
- Ridge model metadata including regularization lambda and feature count.
- Unified comparison of baseline and feature-based models on identical walk-forward test observations.
- Comparison validation for duplicate model names, missing models and mismatched evaluation dates or actual returns.

Ridge preprocessing statistics are learned from each fold's training observations only. Predictions are generated separately from each test row's features. Incomplete feature rows are excluded from Ridge training; incomplete prediction features are rejected.

- Multi-horizon experiment runner for supported 1, 5, 10 and 20-session datasets.
- Fresh model instances per horizon, shared walk-forward configuration and canonical horizon ordering.
- Experiment reports with schema version, UTC generation time, dataset date boundaries, row counts and model comparisons.
- Indented JSON serialization through `MarshalForecastExperimentReport`.
- Company-level historical EOD forecast endpoint using Ridge Regression, Zero Return and Historical Mean models.

**Limitations:** These are research models, not validated production forecasts. Passing software tests does not establish predictive skill. Model performance must be assessed on sufficiently large, representative, point-in-time datasets against naive baselines. Results are sensitive to data quality, market regime, feature choices and model configuration. No accuracy or return is guaranteed.

## 12. Development Status

The backend currently includes:

- Company management.
- News collection and relevance classification.
- AI news analysis.
- Historical stock and index data.
- Market analytics and comparisons.
- Historical SMA crossover backtesting with hypothetical portfolio simulation.
- Performance metrics including CAGR, drawdown, Sharpe ratio, win rate, and profit factor.
- Market overview.
- Data-quality reporting.
- Automated tests for selected components.

A complete Web Dashboard and Android application are separate development deliverables. Their completion should be verified independently.

---

## 13. License and Usage

No license has been declared in this repository yet. Do not assume that the project is open-source or that unrestricted redistribution is permitted.

---

**MarketMind AI**
*Historical research and market intelligence.*
