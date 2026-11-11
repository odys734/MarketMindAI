"""Bulk-download and strict all-filters NSE historical EOD scanner."""
from __future__ import annotations
import math
from typing import Any
import pandas as pd
from .indicators import atr_wilder, return_pct, rsi_wilder, sma
from .universe import NIFTY50, NIFTY_TICKER, yahoo_symbol

class ScanDataError(RuntimeError):
    pass

def bulk_fetch(period: str = "9mo", interval: str = "1d") -> pd.DataFrame:
    """One provider call for 50 stocks, Nifty 50 and all mapped sector indices."""
    try:
        import yfinance as yf
    except ImportError as exc:
        raise ScanDataError("Install dependencies: pip install -r requirements.txt") from exc
    tickers = {NIFTY_TICKER}
    tickers.update(yahoo_symbol(x.symbol) for x in NIFTY50)
    tickers.update(x.sector_index for x in NIFTY50)
    try:
        data = yf.download(tickers=sorted(tickers), period=period, interval=interval,
                           group_by="ticker", auto_adjust=True, actions=False,
                           threads=True, progress=False, timeout=25)
    except Exception as exc:
        raise ScanDataError(f"Bulk historical-data request failed: {exc}") from exc
    if data is None or data.empty:
        raise ScanDataError("Bulk provider returned no historical observations")
    return data.sort_index()

def _series(data: pd.DataFrame, ticker: str, field: str) -> pd.Series:
    if data.empty: return pd.Series(dtype=float)
    if isinstance(data.columns, pd.MultiIndex):
        for key in ((ticker, field), (field, ticker)):
            if key in data.columns:
                return data[key].dropna().astype(float).sort_index()
        return pd.Series(dtype=float)
    if ticker == NIFTY_TICKER and field in data.columns:
        return data[field].dropna().astype(float).sort_index()
    return pd.Series(dtype=float)

def scan(data: pd.DataFrame, budget: float, capital_risk_pct: float = 2.0,
         max_rs_spread_pct: float = 1.5) -> dict[str, Any]:
    """Strict AND filters. Returns paper quantities, never execution instructions."""
    if not math.isfinite(budget) or budget <= 0:
        raise ValueError("budget must be a finite positive number")
    if not math.isfinite(capital_risk_pct) or not 0 < capital_risk_pct <= 2:
        raise ValueError("capital_risk_pct must be > 0 and <= 2")
    if not math.isfinite(max_rs_spread_pct) or max_rs_spread_pct < 0:
        raise ValueError("max_rs_spread_pct must be finite and nonnegative")

    nifty = _series(data, NIFTY_TICKER, "Close")
    if len(nifty) < 21:
        raise ScanDataError("Nifty 50 requires at least 21 daily closes")
    nifty_ma20 = sma(nifty, 20)
    nifty_distance = (float(nifty.iloc[-1]) / nifty_ma20 - 1) * 100
    danger = nifty_distance > 8.0
    nifty_5d = return_pct(nifty, 5)
    if not math.isfinite(nifty_5d):
        raise ScanDataError("Nifty 50 requires at least 6 closes for 5-session return")

    qualified, rejected = [], []
    for item in NIFTY50:
        ticker = yahoo_symbol(item.symbol)
        close = _series(data, ticker, "Close")
        high, low = _series(data, ticker, "High"), _series(data, ticker, "Low")
        reasons = []
        if len(close) < 51 or len(high) < 15 or len(low) < 15:
            rejected.append({"symbol": item.symbol, "reasons": ["insufficient_history: require 51 closes and 15 OHLC rows"]})
            continue
        price = float(close.iloc[-1])
        ma20, ma50 = sma(close, 20), sma(close, 50)
        rsi, stock_5d = rsi_wilder(close, 14), return_pct(close, 5)
        sector = _series(data, item.sector_index, "Close")
        sector_5d, atr = return_pct(sector, 5), atr_wilder(high, low, close, 14)
        if not all(math.isfinite(x) for x in (price, ma20, ma50, rsi, stock_5d, sector_5d, atr)):
            rejected.append({"symbol": item.symbol, "reasons": ["invalid_or_missing_indicator_data"]})
            continue
        if price > budget: reasons.append("price_above_budget")
        if not (price > ma20 and price > ma50):
            reasons.append("trend_failed: close must be above MA20 and MA50")
        if not (40.0 < rsi < 68.0):
            reasons.append("rsi_failed: Wilder RSI14 must be strictly between 40 and 68")
        if stock_5d < nifty_5d + max_rs_spread_pct:
            reasons.append("relative_strength_failed: stock 5-session return must exceed Nifty by required spread")
        if sector_5d <= nifty_5d:
            reasons.append("sector_strength_failed: sector index must outperform Nifty")
        if atr <= 0: reasons.append("atr_invalid")
        if reasons:
            rejected.append({"symbol": item.symbol, "sector": item.sector, "reasons": reasons})
            continue

        entry = price
        stop, target = entry - 1.5 * atr, entry + 3.0 * atr
        per_share_risk = entry - stop
        budget_qty = math.floor(budget / entry)
        risk_cap = budget * capital_risk_pct / 100.0
        risk_qty = math.floor(risk_cap / per_share_risk) if per_share_risk > 0 else 0
        quantity = max(0, min(budget_qty, risk_qty))
        if danger: quantity //= 2
        qualified.append({
            "symbol": item.symbol, "sector": item.sector, "sector_index": item.sector_index,
            "currency": "INR", "price": round(price, 2), "ma20": round(ma20, 2),
            "ma50": round(ma50, 2), "rsi14": round(rsi, 2),
            "stock_5d_return_pct": round(stock_5d, 3), "nifty_5d_return_pct": round(nifty_5d, 3),
            "sector_5d_return_pct": round(sector_5d, 3),
            "relative_strength_spread_pct": round(stock_5d - nifty_5d, 3),
            "atr14": round(atr, 2), "entry_reference": round(entry, 2),
            "stop_loss": round(stop, 2), "target": round(target, 2),
            "risk_reward": "1:2", "risk_per_share": round(per_share_risk, 2),
            "budget_quantity": budget_qty, "risk_limited_quantity": risk_qty,
            "suggested_paper_quantity": quantity, "estimated_capital_used": round(quantity * entry, 2),
            "estimated_max_loss": round(quantity * per_share_risk, 2),
            "risk_capital_limit": round(risk_cap, 2),
            "regime_action": "half_position_or_wait" if danger else "standard_risk_cap",
        })
    qualified.sort(key=lambda x: (x["relative_strength_spread_pct"], x["sector_5d_return_pct"]), reverse=True)
    return {
        "system": "NSE strict technical paper-research scanner",
        "data_type": "historical_daily_adjusted_ohlc", "universe_size": len(NIFTY50),
        "budget_inr": round(budget, 2), "capital_risk_pct": capital_risk_pct,
        "market_regime": {
            "nifty_last_close": round(float(nifty.iloc[-1]), 2), "nifty_ma20": round(nifty_ma20, 2),
            "distance_from_ma20_pct": round(nifty_distance, 3),
            "warning": "DANGER: Market Overbought" if danger else None,
            "action": "Trade with half position size or wait" if danger else "No >8% MA20 overbought warning",
        },
        "qualified_count": len(qualified), "rejected_count": len(rejected),
        "qualified": qualified, "rejected": rejected,
        "methodology": {
            "trend": "close > SMA20 AND SMA50",
            "rsi": "40 < Wilder RSI(14) < 68",
            "relative_strength": f"stock 5-session return >= Nifty return + {max_rs_spread_pct:.2f} percentage points",
            "sector": "mapped sector index 5-session return > Nifty 5-session return",
            "risk": "quantity = min(budget affordability, capital risk cap); danger regime halves quantity",
        },
        "limitations": [
            "Historical adjusted daily data only; not live or intraday quotes.",
            "Nifty 50 constituents and sector mappings are a maintained snapshot; verify against official NSE files.",
            "Yahoo Finance is unofficial; missing or revised observations can change results.",
            "Sector-index mappings are proxies, not guaranteed official sector classifications.",
            "Risk excludes brokerage, taxes, slippage, gaps and liquidity; stop prices are not guaranteed.",
            "Educational paper research only; no orders or investment recommendations.",
        ],
    }
