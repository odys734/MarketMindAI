package eventstudy

import (
	"math"
	"testing"
	"time"
)

func testDate(day int) time.Time {
	return time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC)
}

func TestCalculateMarketAdjustedCAR(t *testing.T) {
	stock := []PriceBar{
		{testDate(1), 100},
		{testDate(2), 101},
		{testDate(3), 102},
		{testDate(4), 105},
		{testDate(5), 106},
		{testDate(6), 107},
		{testDate(7), 108},
	}

	benchmark := []PriceBar{
		{testDate(1), 100},
		{testDate(2), 100},
		{testDate(3), 100},
		{testDate(4), 100},
		{testDate(5), 100},
		{testDate(6), 100},
		{testDate(7), 100},
	}

	result, err := Calculate(stock, benchmark, testDate(4), Config{
		PreSessions:  1,
		PostSessions: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Observations) != 3 {
		t.Fatalf("expected 3 observations, got %d", len(result.Observations))
	}

	if result.Observations[0].SessionOffset != -1 ||
		result.Observations[1].SessionOffset != 0 ||
		result.Observations[2].SessionOffset != 1 {
		t.Fatal("incorrect event-session offsets")
	}

	wantCAR := (102.0/101.0-1)*100 +
		(105.0/102.0-1)*100 +
		(106.0/105.0-1)*100

	if math.Abs(result.CAR-wantCAR) > 1e-9 {
		t.Fatalf("CAR got %.10f, want %.10f", result.CAR, wantCAR)
	}

	if result.EventSession != testDate(4) {
		t.Fatal("incorrect event session")
	}
}

func TestCalculateMapsHolidayToNextTradingSession(t *testing.T) {
	stock := []PriceBar{
		{testDate(1), 100},
		{testDate(2), 101},
		{testDate(3), 102},
		{testDate(5), 103},
		{testDate(6), 104},
		{testDate(7), 105},
	}

	benchmark := []PriceBar{
		{testDate(1), 100},
		{testDate(2), 100},
		{testDate(3), 100},
		{testDate(5), 100},
		{testDate(6), 100},
		{testDate(7), 100},
	}

	result, err := Calculate(stock, benchmark, testDate(4), Config{
		PreSessions:  1,
		PostSessions: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.EventSession != testDate(5) {
		t.Fatalf("expected next session Jan 5, got %s", result.EventSession)
	}
}

func TestCalculateRejectsInvalidInputs(t *testing.T) {
	valid := []PriceBar{
		{testDate(1), 100},
		{testDate(2), 101},
		{testDate(3), 102},
		{testDate(4), 103},
		{testDate(5), 104},
	}

	tests := []struct {
		name      string
		stock     []PriceBar
		benchmark []PriceBar
		event     time.Time
		cfg       Config
	}{
		{"missing event date", valid, valid, time.Time{}, Config{}},
		{"empty stock", nil, valid, testDate(3), Config{}},
		{"empty benchmark", valid, nil, testDate(3), Config{}},
		{"negative window", valid, valid, testDate(3), Config{PreSessions: -1}},
		{"oversized window", valid, valid, testDate(3), Config{PostSessions: 501}},
		{"insufficient history", valid, valid, testDate(1), Config{PreSessions: 1}},
		{"event after history", valid, valid, testDate(10), Config{}},
		{"invalid stock price", []PriceBar{
			{testDate(1), 100},
			{testDate(2), math.NaN()},
		}, valid, testDate(2), Config{}},
		{"duplicate stock dates", []PriceBar{
			{testDate(1), 100},
			{testDate(1), 101},
		}, valid, testDate(1), Config{}},
		{"unordered benchmark", valid, []PriceBar{
			{testDate(2), 100},
			{testDate(1), 101},
		}, testDate(3), Config{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Calculate(tt.stock, tt.benchmark, tt.event, tt.cfg); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestCalculateRejectsMissingBenchmarkSession(t *testing.T) {
	stock := []PriceBar{
		{testDate(1), 100},
		{testDate(2), 101},
		{testDate(3), 102},
		{testDate(4), 103},
		{testDate(5), 104},
	}

	benchmark := []PriceBar{
		{testDate(1), 100},
		{testDate(2), 100},
		{testDate(4), 100},
		{testDate(5), 100},
	}

	_, err := Calculate(stock, benchmark, testDate(3), Config{})
	if err == nil {
		t.Fatal("expected missing benchmark session error")
	}
}

func TestCalculateRejectsMissingBenchmarkBaseline(t *testing.T) {
	stock := []PriceBar{
		{testDate(1), 100},
		{testDate(2), 101},
		{testDate(3), 102},
		{testDate(4), 103},
	}

	benchmark := []PriceBar{
		{testDate(2), 100},
		{testDate(3), 101},
		{testDate(4), 102},
	}

	_, err := Calculate(stock, benchmark, testDate(2), Config{})
	if err == nil {
		t.Fatal("expected missing benchmark baseline error")
	}
}

func TestCalculateRejectsReturnOverflow(t *testing.T) {
	stock := []PriceBar{
		{testDate(1), math.SmallestNonzeroFloat64},
		{testDate(2), math.MaxFloat64},
		{testDate(3), 100},
	}

	benchmark := []PriceBar{
		{testDate(1), 100},
		{testDate(2), 100},
		{testDate(3), 100},
	}

	_, err := Calculate(stock, benchmark, testDate(2), Config{})
	if err == nil {
		t.Fatal("expected invalid return calculation error")
	}
}

func TestCalculateUsesMatchingBenchmarkDates(t *testing.T) {
	stock := []PriceBar{
		{testDate(1), 100},
		{testDate(2), 101},
		// Stock market is closed on Jan 3.
		{testDate(4), 104},
		{testDate(5), 105},
	}

	benchmark := []PriceBar{
		{testDate(1), 100},
		{testDate(2), 100},
		// Benchmark trades on Jan 3 while stock market is closed.
		{testDate(3), 105},
		{testDate(4), 110},
		{testDate(5), 110},
	}

	result, err := Calculate(stock, benchmark, testDate(4), Config{
		PreSessions:  0,
		PostSessions: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Both returns use Jan 2 -> Jan 4.
	stockReturn := (104.0/101.0 - 1) * 100
	benchmarkReturn := (110.0/100.0 - 1) * 100
	want := stockReturn - benchmarkReturn

	if math.Abs(result.Observations[0].AbnormalReturnPct-want) > 1e-9 {
		t.Fatalf("abnormal return got %.10f, want %.10f",
			result.Observations[0].AbnormalReturnPct, want)
	}
}
