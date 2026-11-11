import math
import pandas as pd
from nse_scanner.indicators import atr_wilder, return_pct, rsi_wilder, sma

def test_sma_and_return():
    values = pd.Series([100, 101, 102, 103, 105, 106])
    assert round(sma(values, 3), 6) == round((103+105+106)/3, 6)
    assert round(return_pct(values, 5), 3) == 6.0

def test_rsi_uptrend_is_100():
    values = pd.Series(range(100, 121), dtype=float)
    assert rsi_wilder(values, 14) == 100.0

def test_atr_constant_range():
    close = pd.Series([10.0] * 20)
    high = pd.Series([11.0] * 20)
    low = pd.Series([9.0] * 20)
    assert atr_wilder(high, low, close, 14) == 2.0

def test_insufficient_history_returns_nan():
    assert math.isnan(sma(pd.Series([1, 2]), 20))
