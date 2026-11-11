package backtest

import (
	"math"
	"testing"
	"time"
)

func makeBars(closes []float64) []Bar {
	bars := make([]Bar, len(closes))

	for i, close := range closes {
		bars[i] = Bar{
			Date:  time.Date(2026, 1, i+1, 0, 0, 0, 0, time.UTC),
			Open:  close,
			High:  close + 1,
			Low:   close - 1,
			Close: close,
		}
	}

	return bars
}

func TestRunSMACrossoverSignals(t *testing.T) {
	bars := makeBars([]float64{
		10, 10, 10, 10, 20, 20, 20,
	})

	result, err := RunSMACrossover(bars, Config{
		FastPeriod: 2,
		SlowPeriod: 3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Signals) == 0 {
		t.Fatal("expected at least one crossover signal")
	}

	signal := result.Signals[0]

	if signal.Action != "ENTER_LONG" {
		t.Fatalf("expected ENTER_LONG, got %s", signal.Action)
	}

	if !signal.ExecutionDate.After(signal.Date) {
		t.Fatal("execution must happen after signal date")
	}

	expectedExecution := bars[5].Date
	if !signal.ExecutionDate.Equal(expectedExecution) {
		t.Fatalf("expected execution date %v, got %v",
			expectedExecution, signal.ExecutionDate)
	}
}

func TestRunSMACrossoverExitSignal(t *testing.T) {
	bars := makeBars([]float64{
		20, 20, 20, 20, 10, 10, 10,
	})

	result, err := RunSMACrossover(bars, Config{
		FastPeriod: 2,
		SlowPeriod: 3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Signals) == 0 {
		t.Fatal("expected an exit crossover signal")
	}

	if result.Signals[0].Action != "EXIT_LONG" {
		t.Fatalf("expected EXIT_LONG, got %s",
			result.Signals[0].Action)
	}
}

func TestRunSMACrossoverRejectsInvalidConfig(t *testing.T) {
	bars := makeBars([]float64{10, 11, 12, 13})

	tests := []Config{
		{FastPeriod: 0, SlowPeriod: 3},
		{FastPeriod: 3, SlowPeriod: 3},
		{FastPeriod: 4, SlowPeriod: 2},
	}

	for _, cfg := range tests {
		_, err := RunSMACrossover(bars, cfg)
		if err == nil {
			t.Errorf("expected error for config %+v", cfg)
		}
	}
}

func TestRunSMACrossoverRejectsInsufficientData(t *testing.T) {
	bars := makeBars([]float64{10, 11, 12})

	_, err := RunSMACrossover(bars, Config{
		FastPeriod: 2,
		SlowPeriod: 3,
	})

	if err == nil {
		t.Fatal("expected insufficient data error")
	}
}

func TestRunSMACrossoverRejectsUnorderedDates(t *testing.T) {
	bars := makeBars([]float64{10, 11, 12, 13})
	bars[2].Date = bars[1].Date

	_, err := RunSMACrossover(bars, Config{
		FastPeriod: 2,
		SlowPeriod: 3,
	})

	if err == nil {
		t.Fatal("expected unordered date error")
	}
}

func TestRunSMACrossoverRejectsInvalidOHLC(t *testing.T) {
	bars := makeBars([]float64{10, 11, 12, 13})
	bars[2].High = 5

	_, err := RunSMACrossover(bars, Config{
		FastPeriod: 2,
		SlowPeriod: 3,
	})

	if err == nil {
		t.Fatal("expected invalid OHLC error")
	}
}

func TestRunSMACrossoverRejectsNonFiniteOHLC(t *testing.T) {
	tests := []struct {
		name string
		set  func(*Bar)
	}{
		{"NaN close", func(b *Bar) { b.Close = math.NaN() }},
		{"infinite open", func(b *Bar) { b.Open = math.Inf(1) }},
		{"infinite high", func(b *Bar) { b.High = math.Inf(1) }},
		{"infinite low", func(b *Bar) { b.Low = math.Inf(-1) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bars := makeBars([]float64{10, 11, 12, 13})
			tt.set(&bars[2])

			_, err := RunSMACrossover(bars, Config{
				FastPeriod: 2,
				SlowPeriod: 3,
			})
			if err == nil {
				t.Fatal("expected non-finite OHLC validation error")
			}
		})
	}
}

func TestRunSMACrossoverRejectsSMAOverflow(t *testing.T) {
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	bars := make([]Bar, 3)

	for i := range bars {
		bars[i] = Bar{
			Date:  date.AddDate(0, 0, i),
			Open:  math.MaxFloat64,
			High:  math.MaxFloat64,
			Low:   math.MaxFloat64,
			Close: math.MaxFloat64,
		}
	}

	_, err := RunSMACrossover(bars, Config{
		FastPeriod: 1,
		SlowPeriod: 2,
	})
	if err == nil {
		t.Fatal("expected SMA overflow error")
	}
}
