package backtest

import (
	"math"
	"testing"
	"time"
)

func portfolioBars() []Bar {
	return []Bar{
		{
			Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Open: 10, High: 11, Low: 9, Close: 10,
		},
		{
			Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
			Open: 10, High: 12, Low: 9, Close: 11,
		},
		{
			Date: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
			Open: 12, High: 13, Low: 11, Close: 12,
		},
	}
}

func portfolioSignals() []Signal {
	bars := portfolioBars()

	return []Signal{
		{
			Date:          bars[0].Date,
			ExecutionDate: bars[1].Date,
			Action:        "ENTER_LONG",
		},
		{
			Date:          bars[1].Date,
			ExecutionDate: bars[2].Date,
			Action:        "EXIT_LONG",
		},
	}
}

func TestSimulatePortfolioWithoutCosts(t *testing.T) {
	result, err := SimulatePortfolio(
		portfolioBars(),
		portfolioSignals(),
		PortfolioConfig{
			InitialCapital: 1000,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}

	trade := result.Trades[0]

	if trade.Quantity != 100 {
		t.Fatalf("expected 100 shares, got %d", trade.Quantity)
	}

	if !closeEnough(result.FinalCash, 1200) {
		t.Fatalf("expected final cash 1200, got %.4f", result.FinalCash)
	}

	if !closeEnough(result.FinalEquity, 1200) {
		t.Fatalf("expected final equity 1200, got %.4f", result.FinalEquity)
	}

	if !closeEnough(result.TotalReturnPct, 20) {
		t.Fatalf("expected 20%% return, got %.4f%%", result.TotalReturnPct)
	}

	if result.OpenPosition || result.OpenShares != 0 {
		t.Fatal("expected no open position")
	}
}

func TestSimulatePortfolioWithCosts(t *testing.T) {
	result, err := SimulatePortfolio(
		portfolioBars(),
		portfolioSignals(),
		PortfolioConfig{
			InitialCapital: 1000,
			CommissionBPS:  100,
			SlippageBPS:    100,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(result.Trades))
	}

	trade := result.Trades[0]

	if trade.Quantity != 98 {
		t.Fatalf("expected 98 shares, got %d", trade.Quantity)
	}

	if !closeEnough(trade.EntryPrice, 10.1) {
		t.Fatalf("unexpected entry price: %.4f", trade.EntryPrice)
	}

	if !closeEnough(trade.ExitPrice, 11.88) {
		t.Fatalf("unexpected exit price: %.4f", trade.ExitPrice)
	}

	if !closeEnough(result.FinalCash, 1152.8996) {
		t.Fatalf("unexpected final cash: %.4f", result.FinalCash)
	}

	if !closeEnough(trade.NetPnL, 152.8996) {
		t.Fatalf("unexpected net PnL: %.4f", trade.NetPnL)
	}
}

func TestSimulatePortfolioMarksOpenPosition(t *testing.T) {
	bars := portfolioBars()[:2]
	signals := portfolioSignals()[:1]

	result, err := SimulatePortfolio(
		bars,
		signals,
		PortfolioConfig{InitialCapital: 1000},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.OpenPosition || result.OpenShares != 100 {
		t.Fatal("expected 100 shares in an open position")
	}

	if !closeEnough(result.FinalEquity, 1100) {
		t.Fatalf("expected final equity 1100, got %.4f", result.FinalEquity)
	}

	if len(result.Trades) != 0 {
		t.Fatal("open position must not count as a closed trade")
	}
}

func TestSimulatePortfolioRejectsInvalidInputs(t *testing.T) {
	bars := portfolioBars()
	signals := portfolioSignals()

	tests := []struct {
		name    string
		bars    []Bar
		signals []Signal
		config  PortfolioConfig
	}{
		{
			name:    "zero capital",
			bars:    bars,
			signals: signals,
			config:  PortfolioConfig{},
		},
		{
			name:    "negative commission",
			bars:    bars,
			signals: signals,
			config: PortfolioConfig{
				InitialCapital: 1000,
				CommissionBPS:  -1,
			},
		},
		{
			name: "invalid signal",
			bars: bars,
			signals: []Signal{{
				Date:          bars[0].Date,
				ExecutionDate: bars[1].Date,
				Action:        "BUY",
			}},
			config: PortfolioConfig{InitialCapital: 1000},
		},
		{
			name:    "missing execution bar",
			bars:    bars[:2],
			signals: signals,
			config:  PortfolioConfig{InitialCapital: 1000},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := SimulatePortfolio(
				test.bars,
				test.signals,
				test.config,
			)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func closeEnough(a, b float64) bool {
	return math.Abs(a-b) < 0.0001
}

func TestSimulatePortfolioRejectsQuantityOverflow(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	bars := []Bar{
		{Date: start, Open: 1e-300, High: 2e-300, Low: 5e-301, Close: 1e-300},
		{Date: start.AddDate(0, 0, 1), Open: 1e-300, High: 2e-300, Low: 5e-301, Close: 1e-300},
	}

	signals := []Signal{{
		Date: start, ExecutionDate: bars[1].Date, Action: "ENTER_LONG",
	}}

	_, err := SimulatePortfolio(bars, signals, PortfolioConfig{
		InitialCapital: 1e12,
	})
	if err == nil {
		t.Fatal("expected quantity overflow error")
	}
}

func TestSimulatePortfolioRejectsExitOverflow(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	bars := []Bar{
		{Date: start, Open: 1, High: 2, Low: 0.5, Close: 1},
		{Date: start.AddDate(0, 0, 1), Open: 1, High: 2, Low: 0.5, Close: 1},
		{Date: start.AddDate(0, 0, 2), Open: 1e300, High: 1.1e300, Low: 9e299, Close: 1e300},
	}

	signals := []Signal{
		{Date: bars[0].Date, ExecutionDate: bars[1].Date, Action: "ENTER_LONG"},
		{Date: bars[1].Date, ExecutionDate: bars[2].Date, Action: "EXIT_LONG"},
	}

	_, err := SimulatePortfolio(bars, signals, PortfolioConfig{
		InitialCapital: 1e12,
	})
	if err == nil {
		t.Fatal("expected exit calculation overflow error")
	}
}

func TestSimulatePortfolioRejectsFinalReturnOverflow(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	bars := []Bar{
		{Date: start, Open: 1e-300, High: 2e-300, Low: 5e-301, Close: 1e-300},
		{Date: start.AddDate(0, 0, 1), Open: 1e-300, High: 2e-300, Low: 5e-301, Close: 1e-300},
		{Date: start.AddDate(0, 0, 2), Open: 1e300, High: 1.1e300, Low: 9e299, Close: 1e300},
	}

	signals := []Signal{{
		Date: bars[0].Date, ExecutionDate: bars[1].Date, Action: "ENTER_LONG",
	}}

	_, err := SimulatePortfolio(bars, signals, PortfolioConfig{
		InitialCapital: 1e-300,
	})
	if err == nil {
		t.Fatal("expected final return overflow error")
	}
}

func TestSimulatePortfolioRejectsNonFiniteOHLC(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Bar)
	}{
		{"NaN open", func(b *Bar) { b.Open = math.NaN() }},
		{"infinite high", func(b *Bar) { b.High = math.Inf(1) }},
		{"NaN low", func(b *Bar) { b.Low = math.NaN() }},
		{"infinite close", func(b *Bar) { b.Close = math.Inf(1) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bars := portfolioBars()
			tt.mutate(&bars[0])

			_, err := SimulatePortfolio(bars, nil, PortfolioConfig{
				InitialCapital: 1000,
			})
			if err == nil {
				t.Fatal("expected non-finite OHLC validation error")
			}
		})
	}
}
