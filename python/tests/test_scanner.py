import pandas as pd
from nse_scanner.scanner import scan
from nse_scanner.universe import NIFTY50, NIFTY_TICKER, yahoo_symbol

def make_bulk_frame():
    idx = pd.date_range("2025-01-01", periods=80, freq="B")
    columns = {}
    for item in NIFTY50:
        ticker = yahoo_symbol(item.symbol)
        close = pd.Series([100.0 + i * 0.8 for i in range(80)], index=idx)
        columns[(ticker, "Close")] = close
        columns[(ticker, "High")] = close + 1
        columns[(ticker, "Low")] = close - 1
    nifty = pd.Series([100.0 + i * 0.2 for i in range(80)], index=idx)
    columns[(NIFTY_TICKER, "Close")] = nifty
    for item in NIFTY50:
        columns[(item.sector_index, "Close")] = pd.Series([100.0 + i * 0.3 for i in range(80)], index=idx)
    return pd.DataFrame(columns)

def test_scan_reports_all_50_and_strict_rejections():
    result = scan(make_bulk_frame(), budget=10000)
    assert result["universe_size"] == 50
    assert result["qualified_count"] + result["rejected_count"] == 50
    assert result["rejected_count"] == 50  # synthetic RSI is >68: hard rejection

def test_invalid_budget_rejected():
    try:
        scan(make_bulk_frame(), budget=0)
    except ValueError as exc:
        assert "budget" in str(exc)
    else:
        raise AssertionError("expected invalid budget")
