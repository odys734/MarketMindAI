# MarketMind AI — Research Scanner & Paper-Trading Platform Plan

## Product direction

Extend MarketMind AI from a historical market-intelligence and experimental forecasting backend into an educational **market research, stock screening, trade-plan simulation, and paper-trading platform**.

The product must explain its evidence and uncertainty. It is not an investment adviser, does not issue personalized buy/sell instructions, does not guarantee returns, and must not place real orders. All entry/exit levels in this phase are hypothetical research scenarios for paper trading.

## Preserve the existing foundation

Keep the current Go + Gin + PostgreSQL architecture and existing modules:
- Company master data and company search.
- GDELT news collection, company relevance, AI news analysis and sentiment history.
- TejHQ historical EOD company prices and Yahoo Finance historical index data.
- Market analytics, index comparisons, news/price correlation and data-quality reports.
- Experimental 1/5/10/20-session forecasting, baselines, purged walk-forward evaluation and historical backtesting.

Do not promote Ridge Regression to a winning model: current Reliance evaluation shows it does not consistently beat the zero-return and historical-mean baselines. Model selection must be evidence-based and may return “no model qualified”.

## Product workflow

1. **Research profile** — user selects market universe, research mode (intraday research, swing research, long-term research), virtual budget, maximum concurrent paper positions, and a conservative per-position risk budget.
2. **Universe discovery** — obtain eligible symbols from a maintained exchange instrument master. Record exchange, symbol, series, lot size where relevant, tick size, instrument status and source timestamp. Never pretend the current company table is a complete exchange universe.
3. **Data readiness gate** — check freshness, missing sessions, duplicate providers, corporate-action concerns, liquidity/volume coverage and minimum history. Exclude or clearly mark symbols that fail.
4. **Screening** — apply explainable mode-specific filters using price/volume, volatility, trend, technical indicators, sector/index context, fundamentals when a reliable provider exists, and relevant news. Every factor records its value, source, timestamp and rule.
5. **Candidate ranking** — rank eligible research candidates with transparent factor contributions and a reason list. Ranking is not a probability of profit. Show “insufficient evidence / no candidate” when appropriate.
6. **Scenario builder** — for each candidate, create a hypothetical setup with a trigger zone, invalidation level, one or more scenario targets, time horizon, assumptions and conditions that cancel the setup. Levels must come from documented deterministic rules and historical data; do not invent precision or call them guaranteed entry/exit points.
7. **Budget and risk calculator** — calculate whole-share quantity from virtual capital, scenario entry-to-invalidation distance, configured risk cap, available cash, liquidity cap and estimated costs. Enforce quantity >= 0, no negative cash, no leverage by default, and no position when costs/data make it infeasible.
8. **Paper execution** — simulate next eligible bar execution, gaps, slippage, brokerage/taxes assumptions, partial exits, stop/target ordering ambiguity, and market closures. No broker integration or live order placement.
9. **Portfolio and journal** — track virtual cash, open/closed paper positions, exposure, drawdown, realized/unrealized P&L, costs, setup rationale and user notes.
10. **Evaluation** — compare scanner rules and forecast models with chronological, purged walk-forward tests; report MAE/RMSE, directional metrics where defined, turnover, exposure, drawdown, win/loss distribution and cost sensitivity. Avoid look-ahead and survivorship bias where source data permits.
11. **Research report** — produce a reproducible report containing data freshness, screened universe size, exclusions, candidate explanations, hypothetical scenarios, risk calculations, assumptions, model diagnostics and limitations.

## Research modes

### Intraday research
- Requires intraday bars and reliable timestamped volume; EOD-only data is insufficient.
- Session-aware rules for market hours, opening gaps, liquidity, spread/slippage and end-of-day position handling.
- If intraday data is unavailable or stale, disable this mode and explain why.

### Swing research
- Daily bars; multi-session holding horizon.
- Trend, momentum, volatility, support/resistance context, volume and event/news risk.
- Scenario invalidation and time-based exit conditions.

### Long-term research
- Multi-year price history and reliable fundamentals/financial statements.
- Business quality, growth, profitability, leverage, valuation context, sector and risk factors.
- Do not fabricate missing fundamentals; show unavailable fields.

## Core domain modules (proposed)

- `internal/universe`: exchange instrument master, symbol identity, eligibility and refresh metadata.
- `internal/screener`: mode-specific filters, factor values, exclusions and explainable ranking.
- `internal/tradeplan`: hypothetical setup scenarios, trigger/invalidation/target rules and provenance.
- `internal/risk`: budget, risk cap, quantity, costs, exposure and feasibility calculations.
- `internal/papertrade`: virtual orders, deterministic execution simulation, positions, cash and journal.
- `internal/researchreport`: reproducible report schema and export.
- `internal/marketdata`: provider adapters, freshness and coverage contracts; add intraday/fundamental providers only after validating availability, terms and data quality.

Keep HTTP handlers thin. Domain logic must be deterministic and unit-testable. Persist schema changes through ordered SQL migrations.

## Proposed API surface (versioned)

- `GET /api/v1/universe` — available instruments and coverage metadata.
- `POST /api/v1/research/scans` — create a scan from market, mode, virtual budget and constraints.
- `GET /api/v1/research/scans/:id` — scan status, data readiness, exclusions and ranked candidates.
- `GET /api/v1/research/scans/:id/candidates/:candidate_id` — evidence and factor explanation.
- `POST /api/v1/research/scans/:id/candidates/:candidate_id/scenario` — create a hypothetical paper-trade scenario.
- `POST /api/v1/paper/portfolios` — create a virtual portfolio.
- `GET /api/v1/paper/portfolios/:id` — cash, exposure, positions and performance.
- `POST /api/v1/paper/portfolios/:id/orders` — submit a simulated order only.
- `GET /api/v1/paper/portfolios/:id/journal` — paper-trade journal.
- `POST /api/v1/research/evaluations` — evaluate scanner/model rules on historical data.
- `GET /api/v1/research/reports/:id` — retrieve reproducible research report.

These are target contracts, not claims that the routes already exist. Final request/response schemas must be documented before implementation.

## Phased delivery

### Phase 0 — Contract and safety foundation
- Confirm supported exchanges and instrument universe.
- Define profile, scan, candidate, scenario, risk and paper portfolio schemas.
- Define provider capability/freshness contracts and explicit “unavailable” states.
- Add migrations, validation, API docs and test fixtures.

### Phase 1 — Budget-aware research scanner (daily/EOD)
- Start with the verified historical EOD provider and the currently stored company universe.
- Add market/mode/budget scan request and data-readiness checks.
- Add explainable technical screening and ranked candidates.
- Return no candidate when coverage or criteria are insufficient.
- Add unit, handler, database and integration tests.

### Phase 2 — Scenario and risk calculator
- Build deterministic hypothetical trigger/invalidation/target scenarios.
- Add budget, quantity, costs, risk-cap and cash-feasibility calculations.
- Include detailed rationale and scenario invalidation conditions.
- Test gaps, zero/negative values, fractional shares, cost edge cases and insufficient budget.

### Phase 3 — Paper trading
- Add virtual portfolio, simulated order lifecycle, execution assumptions, cash ledger and journal.
- Add no-look-ahead historical simulation and explicit gap/slippage behavior.
- Never connect this API to broker order endpoints.

### Phase 4 — Robust evaluation
- Evaluate scanner rules and scenarios with purged walk-forward validation.
- Add aggregate and per-fold stability, worst/median fold, dispersion and cost sensitivity.
- Add benchmark comparisons and data-quality exclusions.
- Do not automatically deploy a model based on a single metric.

### Phase 5 — Data expansion
- Validate an exchange instrument master for NSE/BSE.
- Add intraday feed only after confirming supported interval, timestamps, history, rate limits and licensing.
- Add fundamentals only through a documented reliable source.
- Make each mode unavailable when its required data is missing.

### Phase 6 — User interface
- Research profile and budget setup.
- Scan progress, data coverage, candidate list and explanation view.
- Scenario details with hypothetical trigger, invalidation, targets and quantity/cost breakdown.
- Paper portfolio, journal, reports and model diagnostics.
- Responsive mobile-first interface; API remains independently usable.

### Phase 7 — Hardening
- Authentication/authorization before multi-user portfolios.
- Rate limits, request IDs, audit logs, provider retries/timeouts and observability.
- Migration/backup strategy, CI coverage, security review and deployment documentation.

## Initial defaults and non-goals

- Start with historical EOD research; do not label it intraday.
- No real-money trading, broker credentials, order routing, leverage or derivatives in the initial release.
- No guaranteed returns, certainty scores, fabricated data, or personalized investment advice.
- A scan is allowed to return zero candidates.
- Budget is a simulation constraint, not a recommendation about how much money a person should invest.
- Exchange universe coverage and data entitlements must be verified before claiming “all stocks”.

## Definition of done for the first MVP

- User can request a daily-data research scan with a virtual budget and mode.
- API reports data freshness and exact eligible/excluded universe counts.
- Each candidate has traceable factor values, explanations and source timestamps.
- A hypothetical scenario includes trigger, invalidation, targets, assumptions and cancellation rules.
- Quantity and costs are calculated with a configurable risk cap and no negative virtual cash.
- Paper simulation is reproducible and cannot place real orders.
- Tests cover validation, data gaps, stale data, ties, costs, position sizing, look-ahead prevention and no-candidate cases.
- Documentation clearly separates existing features from planned features.
