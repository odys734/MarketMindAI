package forecast

import (
	"math"
	"testing"
	"time"
)

func baselineTestRows(values ...float64) []DatasetRow {
	rows := make([]DatasetRow, len(values))

	for i, value := range values {
		date := time.Date(2025, 1, 1+i, 0, 0, 0, 0, time.UTC)
		targetDate := date.AddDate(0, 0, 1)

		rows[i] = DatasetRow{
			Date: date,
			Target: Target{
				Date:             date,
				HorizonSessions:  1,
				TargetDate:       targetDate,
				StartClose:       100,
				TargetClose:      100 * (1 + value/100),
				ForwardReturnPct: value,
			},
		}
	}

	return rows
}

func TestZeroReturnBaseline(t *testing.T) {
	model := &ZeroReturnBaseline{}

	if _, err := model.Predict(); err == nil {
		t.Fatal("expected error before fitting")
	}

	if err := model.Fit(baselineTestRows(2, -1, 4), 1); err != nil {
		t.Fatal(err)
	}

	prediction, err := model.Predict()
	if err != nil {
		t.Fatal(err)
	}
	if prediction != 0 {
		t.Fatalf("prediction: got %v, want 0", prediction)
	}
	if model.Name() != "zero_return" {
		t.Fatalf("unexpected model name: %s", model.Name())
	}
}

func TestHistoricalMeanBaseline(t *testing.T) {
	model := &HistoricalMeanBaseline{}

	if _, err := model.Predict(); err == nil {
		t.Fatal("expected error before fitting")
	}

	if err := model.Fit(baselineTestRows(2, -1, 4), 1); err != nil {
		t.Fatal(err)
	}

	prediction, err := model.Predict()
	if err != nil {
		t.Fatal(err)
	}

	assertNear(t, "historical mean", prediction, 5.0/3.0, 1e-12)

	if model.Name() != "historical_mean" {
		t.Fatalf("unexpected model name: %s", model.Name())
	}
}

func TestHistoricalMeanBaselineUsesOnlyTrainingRows(t *testing.T) {
	training := baselineTestRows(2, 4)
	model := &HistoricalMeanBaseline{}

	if err := model.Fit(training, 1); err != nil {
		t.Fatal(err)
	}

	prediction, err := model.Predict()
	if err != nil {
		t.Fatal(err)
	}

	assertNear(t, "training-only mean", prediction, 3, 1e-12)
}

func TestBaselineRejectsInvalidTrainingData(t *testing.T) {
	tests := []struct {
		name    string
		rows    []DatasetRow
		horizon int
	}{
		{name: "empty training", rows: nil, horizon: 1},
		{name: "unsupported horizon", rows: baselineTestRows(1), horizon: 2},
		{name: "target horizon mismatch", rows: baselineTestRows(1), horizon: 5},
		{name: "invalid target", rows: []DatasetRow{{Date: time.Now()}}, horizon: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &HistoricalMeanBaseline{}
			if err := model.Fit(tt.rows, tt.horizon); err == nil {
				t.Fatal("expected training validation error")
			}
		})
	}
}

func TestHistoricalMeanRejectsNonFinitePrediction(t *testing.T) {
	model := &HistoricalMeanBaseline{
		mean:   math.NaN(),
		count:  1,
		fitted: true,
	}

	if _, err := model.Predict(); err == nil {
		t.Fatal("expected non-finite mean error")
	}
}
