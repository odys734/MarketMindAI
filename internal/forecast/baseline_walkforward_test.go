package forecast

import (
	"testing"
	"time"
)

func baselineTestDataset(count int) Dataset {
	rows := make([]DatasetRow, count)
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < count; i++ {
		date := start.AddDate(0, 0, i)
		actualReturn := float64((i % 5) - 2) // -2%, -1%, 0%, 1%, 2%
		rows[i] = DatasetRow{
			Date: date,
			Target: Target{
				Date:             date,
				HorizonSessions:  1,
				TargetDate:       date.AddDate(0, 0, 1),
				StartClose:       100,
				TargetClose:      100 * (1 + actualReturn/100),
				ForwardReturnPct: actualReturn,
			},
		}
	}

	return Dataset{
		HorizonSessions: 1,
		Rows:            rows,
	}
}

func TestEvaluateZeroReturnBaselineWalkForward(t *testing.T) {
	dataset := baselineTestDataset(40)

	result, err := EvaluateBaselineWalkForward(
		dataset,
		WalkForwardConfig{
			InitialTrainSize: 20,
			MinimumTrainSize: 10,
			TestSize:         5,
		},
		&ZeroReturnBaseline{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.ModelName != "zero_return" {
		t.Fatalf("model name: got %q", result.ModelName)
	}
	if result.FoldCount != 4 {
		t.Fatalf("fold count: got %d, want 4", result.FoldCount)
	}
	if len(result.Observations) != 20 {
		t.Fatalf("observations: got %d, want 20", len(result.Observations))
	}
	if result.Metrics.ObservationCount != 20 {
		t.Fatalf("metric observation count: got %d, want 20", result.Metrics.ObservationCount)
	}

	for i, observation := range result.Observations {
		if observation.PredictedReturn != 0 {
			t.Fatalf("prediction %d: got %v, want 0", i, observation.PredictedReturn)
		}
		if observation.HorizonSessions != 1 {
			t.Fatalf("observation %d has wrong horizon", i)
		}
	}
}

func TestEvaluateHistoricalMeanBaselineWalkForward(t *testing.T) {
	dataset := baselineTestDataset(40)

	result, err := EvaluateBaselineWalkForward(
		dataset,
		WalkForwardConfig{
			InitialTrainSize: 20,
			MinimumTrainSize: 10,
			TestSize:         5,
		},
		&HistoricalMeanBaseline{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.ModelName != "historical_mean" {
		t.Fatalf("model name: got %q", result.ModelName)
	}
	if result.FoldCount != 4 || len(result.Observations) != 20 {
		t.Fatalf("unexpected folds/observations: %d/%d", result.FoldCount, len(result.Observations))
	}

	// First fold trains on rows 0..18 after purging row 19.
	// The first 19 values sum to -2 after the final candidate is purged.
	for i := 0; i < 5; i++ {
		assertNear(t, "first-fold prediction", result.Observations[i].PredictedReturn, -2.0/19.0, 1e-12)
	}

	if result.Metrics.ObservationCount != 20 {
		t.Fatalf("metric observation count: got %d, want 20", result.Metrics.ObservationCount)
	}
}

func TestEvaluateBaselineWalkForwardRejectsNilModel(t *testing.T) {
	_, err := EvaluateBaselineWalkForward(
		baselineTestDataset(40),
		WalkForwardConfig{
			InitialTrainSize: 20,
			MinimumTrainSize: 10,
			TestSize:         5,
		},
		nil,
	)
	if err == nil {
		t.Fatal("expected nil model error")
	}
}
