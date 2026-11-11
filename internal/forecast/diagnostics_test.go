package forecast

import "testing"

func TestEvaluateForecastsExcludesDirectionalTies(t *testing.T) {
	metrics, err := EvaluateForecasts([]ForecastObservation{
		{HorizonSessions: 1, PredictedReturn: 0, ActualReturn: 1},
		{HorizonSessions: 1, PredictedReturn: 1, ActualReturn: 1},
		{HorizonSessions: 1, PredictedReturn: -1, ActualReturn: 1},
		{HorizonSessions: 1, PredictedReturn: 1, ActualReturn: 0},
	})
	if err != nil {
		t.Fatal(err)
	}

	if metrics.DirectionalObservationCount != 2 {
		t.Fatalf("directional observation count: got %d, want 2", metrics.DirectionalObservationCount)
	}
	if metrics.DirectionalTieCount != 2 {
		t.Fatalf("directional tie count: got %d, want 2", metrics.DirectionalTieCount)
	}
	if !metrics.DirectionalAccuracyDefined {
		t.Fatal("directional accuracy should be defined when directional observations exist")
	}
	assertNear(t, "directional accuracy", metrics.DirectionalAccuracy, 50, 1e-12)
}

func TestEvaluateForecastsZeroReturnHasUndefinedDirectionalAccuracy(t *testing.T) {
	metrics, err := EvaluateForecasts([]ForecastObservation{
		{HorizonSessions: 1, PredictedReturn: 0, ActualReturn: 1},
		{HorizonSessions: 1, PredictedReturn: 0, ActualReturn: -1},
		{HorizonSessions: 1, PredictedReturn: 0, ActualReturn: 0},
	})
	if err != nil {
		t.Fatal(err)
	}

	if metrics.DirectionalAccuracyDefined {
		t.Fatal("directional accuracy must be undefined for an all-zero forecast")
	}
	if metrics.DirectionalObservationCount != 0 {
		t.Fatalf("directional observation count: got %d, want 0", metrics.DirectionalObservationCount)
	}
	if metrics.DirectionalTieCount != 3 {
		t.Fatalf("directional tie count: got %d, want 3", metrics.DirectionalTieCount)
	}
	if metrics.DirectionalAccuracy != 0 {
		t.Fatalf("undefined directional accuracy placeholder: got %v, want 0", metrics.DirectionalAccuracy)
	}
}

func TestWalkForwardResultsIncludePerFoldMetrics(t *testing.T) {
	result, err := EvaluateBaselineWalkForward(
		baselineTestDataset(40),
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

	if len(result.Folds) != result.FoldCount {
		t.Fatalf("fold diagnostics: got %d, want %d", len(result.Folds), result.FoldCount)
	}
	for i, fold := range result.Folds {
		if fold.Metrics.ObservationCount != fold.TestObservations {
			t.Errorf("fold %d metric observations: got %d, want %d",
				i+1, fold.Metrics.ObservationCount, fold.TestObservations)
		}
		if fold.Metrics.MAE < 0 || fold.Metrics.RMSE < 0 {
			t.Errorf("fold %d has negative error metrics", i+1)
		}
	}
}
