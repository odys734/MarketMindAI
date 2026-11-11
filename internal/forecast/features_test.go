package forecast

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func makeBars(closes []float64) []Bar {
	bars := make([]Bar, len(closes))
	for i, closePrice := range closes {
		bars[i] = Bar{
			Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			Open:   closePrice,
			High:   closePrice + 1,
			Low:    closePrice - 1,
			Close:  closePrice,
			Volume: int64(1000 + i*10),
		}
	}
	return bars
}

func assertNear(t *testing.T, name string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Errorf("%s: got %.10f, want %.10f", name, got, want)
	}
}

func TestCalculateFeaturesMovingAverages(t *testing.T) {
	bars := makeBars([]float64{
		10, 11, 12, 13, 14, 15, 16, 17, 18, 19,
	})

	features, err := CalculateFeatures(bars)
	if err != nil {
		t.Fatal(err)
	}

	last := features[len(features)-1]

	if last.SMA5 == nil || last.SMA10 == nil {
		t.Fatal("expected SMA5 and SMA10")
	}

	assertNear(t, "SMA5", *last.SMA5, 17, 1e-9)
	assertNear(t, "SMA10", *last.SMA10, 14.5, 1e-9)

	if features[3].SMA5 != nil {
		t.Error("SMA5 should be unavailable before five observations")
	}
}

func TestCalculateFeaturesEMA(t *testing.T) {
	shortHistory, err := CalculateFeatures(makeBars([]float64{10, 11, 12, 13, 14}))
	if err != nil {
		t.Fatal(err)
	}
	if shortHistory[4].EMA12 != nil {
		t.Fatal("EMA12 must remain unavailable before 12 observations")
	}

	features, err := CalculateFeatures(makeBars(increasingCloses(30)))
	if err != nil {
		t.Fatal(err)
	}

	last := features[len(features)-1]
	if last.EMA12 == nil || last.EMA26 == nil {
		t.Fatal("expected EMA12 and EMA26 after sufficient history")
	}
}

func TestCalculateFeaturesRSI(t *testing.T) {
	tests := []struct {
		name   string
		closes []float64
		want   float64
	}{
		{
			name:   "all gains",
			closes: increasingCloses(20),
			want:   100,
		},
		{
			name:   "all losses",
			closes: decreasingCloses(20),
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			features, err := CalculateFeatures(makeBars(tt.closes))
			if err != nil {
				t.Fatal(err)
			}

			got := features[len(features)-1].RSI14
			if got == nil {
				t.Fatal("expected RSI14")
			}
			assertNear(t, "RSI14", *got, tt.want, 1e-9)
		})
	}
}

func TestCalculateFeaturesMACD(t *testing.T) {
	bars := makeBars(increasingCloses(60))

	features, err := CalculateFeatures(bars)
	if err != nil {
		t.Fatal(err)
	}

	last := features[len(features)-1]

	if last.MACD == nil || last.MACDSignal == nil || last.MACDHistogram == nil {
		t.Fatal("expected complete MACD values after sufficient history")
	}

	assertNear(
		t,
		"MACD histogram",
		*last.MACDHistogram,
		*last.MACD-*last.MACDSignal,
		1e-9,
	)
}

func TestCalculateFeaturesVolatilityAndVolume(t *testing.T) {
	bars := makeBars(increasingCloses(30))

	features, err := CalculateFeatures(bars)
	if err != nil {
		t.Fatal(err)
	}

	first := features[19]
	if first.VolumeAverage20 == nil {
		t.Fatal("expected volume average after 20 observations")
	}
	if first.VolumeRatio20 == nil {
		t.Fatal("expected volume ratio for positive average volume")
	}
	if first.Volatility20AnnualizedPct != nil {
		t.Error("volatility should require 20 daily returns")
	}

	last := features[20]
	if last.Volatility20AnnualizedPct == nil {
		t.Fatal("expected volatility after 20 daily returns")
	}
	if *last.Volatility20AnnualizedPct < 0 {
		t.Error("volatility cannot be negative")
	}
}

func TestCalculateFeaturesInsufficientHistory(t *testing.T) {
	features, err := CalculateFeatures(makeBars([]float64{10, 11, 12}))
	if err != nil {
		t.Fatal(err)
	}

	last := features[len(features)-1]

	if last.SMA5 != nil || last.SMA10 != nil ||
		last.SMA20 != nil || last.SMA50 != nil {
		t.Error("insufficient SMA values must remain nil")
	}
	if last.EMA12 != nil || last.EMA26 != nil ||
		last.RSI14 != nil || last.MACD != nil {
		t.Error("insufficient indicator values must remain nil")
	}
}

func TestCalculateFeaturesRejectsInvalidData(t *testing.T) {
	tests := []struct {
		name string
		bars []Bar
	}{
		{
			name: "invalid OHLC",
			bars: []Bar{
				{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Open: 10, High: 8, Low: 9, Close: 10},
				{Date: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), Open: 10, High: 11, Low: 9, Close: 10},
			},
		},
		{
			name: "negative volume",
			bars: []Bar{
				{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Open: 10, High: 11, Low: 9, Close: 10, Volume: -1},
				{Date: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), Open: 10, High: 11, Low: 9, Close: 10},
			},
		},
		{
			name: "unordered dates",
			bars: []Bar{
				{Date: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), Open: 10, High: 11, Low: 9, Close: 10},
				{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Open: 10, High: 11, Low: 9, Close: 10},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := CalculateFeatures(tt.bars); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func increasingCloses(n int) []float64 {
	values := make([]float64, n)
	for i := range values {
		values[i] = 100 + float64(i)
	}
	return values
}

func decreasingCloses(n int) []float64 {
	values := make([]float64, n)
	for i := range values {
		values[i] = 200 - float64(i)
	}
	return values
}

func TestCalculateFeaturesNoLookAheadBias(t *testing.T) {
	bars := makeBars(increasingCloses(60))
	prefixLength := 35

	prefixFeatures, err := CalculateFeatures(bars[:prefixLength])
	if err != nil {
		t.Fatal(err)
	}

	fullFeatures, err := CalculateFeatures(bars)
	if err != nil {
		t.Fatal(err)
	}

	prefixLast := prefixFeatures[prefixLength-1]
	fullAtSameDate := fullFeatures[prefixLength-1]

	if !reflect.DeepEqual(prefixLast, fullAtSameDate) {
		t.Fatal("features changed after future bars were added; possible look-ahead bias")
	}
}

func TestCalculateFeaturesRSIExtremeValues(t *testing.T) {
	bars := make([]Bar, 15)
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := range bars {
		closePrice := 1.0
		low := 0.5
		high := 1.1

		if i%2 == 1 {
			closePrice = 1e308
			low = 5e307
			high = 1e308
		}

		bars[i] = Bar{
			Date:   start.AddDate(0, 0, i),
			Open:   closePrice,
			High:   high,
			Low:    low,
			Close:  closePrice,
			Volume: 1000,
		}
	}

	features, err := CalculateFeatures(bars)
	if err != nil {
		t.Fatal(err)
	}

	rsi := features[len(features)-1].RSI14
	if rsi == nil {
		t.Fatal("RSI14 should remain available for finite extreme price changes")
	}
	if !finite(*rsi) || *rsi < 0 || *rsi > 100 {
		t.Fatalf("RSI14 is outside its valid range: %v", *rsi)
	}
}
