"""Pure numerical indicators; input series are chronological."""
import math
import numpy as np
import pandas as pd

def sma(values: pd.Series, period: int) -> float:
    return float(values.astype(float).tail(period).mean()) if len(values) >= period else math.nan

def rsi_wilder(close: pd.Series, period: int = 14) -> float:
    delta = close.astype(float).diff().dropna()
    if len(delta) < period: return math.nan
    gains, losses = delta.clip(lower=0), -delta.clip(upper=0)
    avg_gain, avg_loss = float(gains.iloc[:period].mean()), float(losses.iloc[:period].mean())
    for gain, loss in zip(gains.iloc[period:], losses.iloc[period:]):
        avg_gain = ((period - 1) * avg_gain + float(gain)) / period
        avg_loss = ((period - 1) * avg_loss + float(loss)) / period
    if avg_loss == 0: return 100.0 if avg_gain > 0 else 50.0
    if avg_gain == 0: return 0.0
    return 100.0 - 100.0 / (1.0 + avg_gain / avg_loss)

def atr_wilder(high: pd.Series, low: pd.Series, close: pd.Series, period: int = 14) -> float:
    h, l, c = high.astype(float), low.astype(float), close.astype(float)
    if len(c) < period + 1: return math.nan
    prev = c.shift(1)
    tr = pd.concat([h-l, (h-prev).abs(), (l-prev).abs()], axis=1).max(axis=1).dropna().to_numpy(float)
    if len(tr) < period: return math.nan
    result = float(np.mean(tr[:period]))
    for value in tr[period:]: result = ((period-1)*result + float(value))/period
    return result

def return_pct(close: pd.Series, sessions: int = 5) -> float:
    values = close.astype(float)
    if len(values) < sessions + 1: return math.nan
    start, end = float(values.iloc[-sessions-1]), float(values.iloc[-1])
    return (end/start - 1.0)*100.0 if start > 0 else math.nan
