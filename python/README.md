# NSE Strict Quantitative Scanner

Pure Python technical-analysis research engine. It does not use AI/LLMs for predictions.

## Mandatory filters (AND logic)

1. Latest adjusted close is strictly above SMA20 and SMA50.
2. Wilder RSI(14) is strictly greater than 40 and strictly less than 68.
3. Stock five-session return is at least 1.5 percentage points above Nifty 50 five-session return.
4. Mapped sector-index five-session return is strictly greater than Nifty 50 five-session return.
5. Latest stock price must be within the supplied budget.

The strict 40-to-68 RSI band takes precedence over the looser oversold note: values at or below 40 are rejected, as are values at or above 68.

## Market regime

Raise "DANGER: Market Overbought" only when Nifty 50 is more than 8% above SMA20. In that regime, the already budget- and risk-constrained paper quantity is halved (rounded down), and the output advises half size or waiting.

## Risk calculations

- Entry reference = latest adjusted close.
- Stop = entry - 1.5 × Wilder ATR(14).
- Target = entry + 3.0 × Wilder ATR(14).
- Reward/risk = 3 ATR / 1.5 ATR = 2:1 reward-to-risk (risk:reward 1:2).
- Risk capital cap = budget × risk percentage (default 2%).
- Quantity = min(floor(budget / entry), floor(risk capital cap / (entry - stop))).
- Overbought regime halves that quantity.

The budget is treated as the capital base for the 2% cap. Costs, taxes, slippage and overnight gaps are excluded.

## Bulk data

One yfinance download call fetches all 50 constituent tickers, Nifty 50, and all distinct mapped sector indices. No per-stock request loop is used. Data is adjusted daily OHLC, not a live feed.

The constituent list and sector mappings are a maintained snapshot in nse_scanner/universe.py. Verify against current official NSE constituents and sector-index methodology before relying on a scan. Yahoo Finance is unofficial; observations may be missing or revised. Sector mappings are proxies, not guaranteed official classifications.

## Termux / Linux

    cd ~/MarketMindAI/python
    pkg install python
    python -m venv .venv
    . .venv/bin/activate
    pip install -r requirements.txt
    python -m nse_scanner.cli --budget 10000 --risk-pct 2 --period 9mo --output scan.json

First run requires internet access. At least 51 stock closes and 21 Nifty closes are required.

Run tests:

    python -m pytest -q

Output is JSON with qualified candidates, every rejection reason, indicator values, risk-constrained quantities, regime warning and limitations. Zero candidates is a valid outcome.

Educational historical paper research only. No real orders, execution, or investment recommendation.
