package backtest

import (
	"math"
	"testing"
	"time"
)

func TestCalculateMetrics(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	result := PortfolioResult{
		InitialCapital: 1000,
		FinalEquity:    1100,
		Trades: []Trade{
			{NetPnL: 100},
			{NetPnL: -50},
			{NetPnL: 25},
		},
		EquityCurve: []EquityPoint{
			{Date: start, Equity: 1000},
			{Date: start.AddDate(0, 0, 100), Equity: 900},
			{Date: start.AddDate(0, 0, 200), Equity: 1100},
		},
	}

	m, err := CalculateMetrics(result, 0)
	if err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		name string
		got  float64
		want float64
	}{
		{"total return", m.TotalReturnPct, 10},
		{"maximum drawdown", m.MaxDrawdownPct, 10},
		{"win rate", m.WinRatePct, 200.0 / 3},
		{"profit factor", m.ProfitFactor, 2.5},
	}

	for _, c := range checks {
		if math.Abs(c.got-c.want) > 0.0001 {
			t.Errorf("%s: got %.4f, want %.4f", c.name, c.got, c.want)
		}
	}

	if m.ClosedTrades != 3 || m.WinningTrades != 2 || m.LosingTrades != 1 {
		t.Fatal("incorrect trade counts")
	}

	if !m.CAGRAvailable || !m.SharpeAvailable || !m.ProfitFactorAvailable {
		t.Fatal("expected metrics to be available")
	}
}

func TestCalculateMetricsNoClosedTrades(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	result := PortfolioResult{
		InitialCapital: 1000,
		FinalEquity:    1000,
		EquityCurve: []EquityPoint{
			{Date: start, Equity: 1000},
			{Date: start.AddDate(0, 0, 1), Equity: 1000},
		},
	}

	m, err := CalculateMetrics(result, 0)
	if err != nil {
		t.Fatal(err)
	}

	if m.ClosedTrades != 0 || m.WinRatePct != 0 {
		t.Fatal("expected zero closed trades")
	}
	if m.SharpeAvailable {
		t.Fatal("Sharpe should be unavailable for zero variance")
	}
	if m.ProfitFactorAvailable {
		t.Fatal("profit factor should be unavailable without losses")
	}
}

func TestCalculateMetricsRejectsInvalidData(t *testing.T) {
	result := PortfolioResult{
		InitialCapital: 1000,
		FinalEquity:    1000,
		EquityCurve: []EquityPoint{{
			Date:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Equity: 1000,
		}},
	}

	if _, err := CalculateMetrics(result, -1); err == nil {
		t.Fatal("expected invalid risk-free rate error")
	}

	result.EquityCurve = nil
	if _, err := CalculateMetrics(result, 0); err == nil {
		t.Fatal("expected missing equity curve error")
	}
}

func TestCalculateMetricsRejectsFinalEquityMismatch(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := PortfolioResult{
		InitialCapital: 1000,
		FinalEquity:    1100,
		EquityCurve: []EquityPoint{
			{Date: start, Equity: 1000},
			{Date: start.AddDate(0, 0, 1), Equity: 1050},
		},
	}

	if _, err := CalculateMetrics(result, 0); err == nil {
		t.Fatal("expected final equity mismatch error")
	}
}

func TestCalculateMetricsRejectsProfitAccumulationOverflow(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := PortfolioResult{
		InitialCapital: 1000,
		FinalEquity:    1000,
		Trades: []Trade{
			{NetPnL: math.MaxFloat64},
			{NetPnL: math.MaxFloat64},
		},
		EquityCurve: []EquityPoint{
			{Date: start, Equity: 1000},
		},
	}

	if _, err := CalculateMetrics(result, 0); err == nil {
		t.Fatal("expected gross profit accumulation overflow error")
	}
}

func TestCalculateMetricsRejectsProfitFactorOverflow(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := PortfolioResult{
		InitialCapital: 1000,
		FinalEquity:    1000,
		Trades: []Trade{
			{NetPnL: math.MaxFloat64},
			{NetPnL: -math.SmallestNonzeroFloat64},
		},
		EquityCurve: []EquityPoint{
			{Date: start, Equity: 1000},
		},
	}

	if _, err := CalculateMetrics(result, 0); err == nil {
		t.Fatal("expected profit factor overflow error")
	}
}
