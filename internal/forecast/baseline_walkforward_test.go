package forecast

import (
	"errors"
	"math"
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

func TestEvaluateRidgeRegressionWalkForward(t *testing.T) {
	rows := ridgeTestRows(50)
	dataset := Dataset{
		HorizonSessions: 1,
		Rows:            rows,
	}

	model, err := NewRidgeRegression(1)
	if err != nil {
		t.Fatal(err)
	}

	result, err := EvaluateFeatureWalkForward(
		dataset,
		WalkForwardConfig{
			InitialTrainSize: 25,
			MinimumTrainSize: 10,
			TestSize:         5,
		},
		model,
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.ModelName != "ridge_regression" {
		t.Fatalf("model name: got %q", result.ModelName)
	}

	if result.ModelMetadata["lambda"] != float64(1) {
		t.Fatalf("model lambda metadata: got %v, want 1", result.ModelMetadata["lambda"])
	}
	if result.ModelMetadata["feature_count"] != ridgeFeatureCount {
		t.Fatalf("feature count metadata: got %v, want %d",
			result.ModelMetadata["feature_count"], ridgeFeatureCount)
	}
	if result.FoldCount != 5 {
		t.Fatalf("fold count: got %d, want 5", result.FoldCount)
	}
	if len(result.Observations) != 25 {
		t.Fatalf("observations: got %d, want 25", len(result.Observations))
	}
	if result.Metrics.ObservationCount != 25 {
		t.Fatalf("metric observation count: got %d, want 25", result.Metrics.ObservationCount)
	}

	for i, observation := range result.Observations {
		if observation.HorizonSessions != 1 {
			t.Fatalf("observation %d has wrong horizon", i)
		}
		if !finite(observation.PredictedReturn) {
			t.Fatalf("observation %d has non-finite prediction", i)
		}
	}
}

func TestEvaluateFeatureWalkForwardRejectsNilModel(t *testing.T) {
	_, err := EvaluateFeatureWalkForward(
		Dataset{},
		WalkForwardConfig{},
		nil,
	)
	if err == nil {
		t.Fatal("expected nil feature model error")
	}
}

type featureWalkForwardSpy struct {
	fitCalls     int
	predictCalls int
	failPredict  bool
	nonFinite    bool
}

func (m *featureWalkForwardSpy) Name() string {
	return "feature_spy"
}

func (m *featureWalkForwardSpy) Fit(training []DatasetRow, horizon int) error {
	m.fitCalls++
	if len(training) == 0 {
		return errors.New("empty training data")
	}
	return nil
}

func (m *featureWalkForwardSpy) Predict(features Features) (float64, error) {
	m.predictCalls++
	if m.failPredict {
		return 0, errors.New("synthetic prediction failure")
	}
	if m.nonFinite {
		return math.NaN(), nil
	}
	return features.Close, nil
}

func TestFeatureWalkForwardFitsEachFoldAndPredictsEachRow(t *testing.T) {
	model := &featureWalkForwardSpy{}
	result, err := EvaluateFeatureWalkForward(
		Dataset{
			HorizonSessions: 1,
			Rows:            ridgeTestRows(50),
		},
		WalkForwardConfig{
			InitialTrainSize: 25,
			MinimumTrainSize: 10,
			TestSize:         5,
		},
		model,
	)
	if err != nil {
		t.Fatal(err)
	}

	if model.fitCalls != result.FoldCount {
		t.Fatalf("fit calls: got %d, want %d", model.fitCalls, result.FoldCount)
	}
	if model.predictCalls != len(result.Observations) {
		t.Fatalf("predict calls: got %d, want %d", model.predictCalls, len(result.Observations))
	}
	if model.predictCalls != 25 {
		t.Fatalf("predict calls: got %d, want 25", model.predictCalls)
	}

	for i, observation := range result.Observations {
		expected := ridgeTestRows(50)[25+i].Features.Close
		if observation.PredictedReturn != expected {
			t.Fatalf("row %d: got prediction %v, want %v",
				i, observation.PredictedReturn, expected)
		}
	}
}

func TestFeatureWalkForwardRejectsFeatureDateMismatch(t *testing.T) {
	rows := ridgeTestRows(40)
	rows[20].Features.Date = rows[20].Date.AddDate(0, 0, -1)

	model := &featureWalkForwardSpy{}
	_, err := EvaluateFeatureWalkForward(
		Dataset{HorizonSessions: 1, Rows: rows},
		WalkForwardConfig{
			InitialTrainSize: 20,
			MinimumTrainSize: 10,
			TestSize:         5,
		},
		model,
	)
	if err == nil {
		t.Fatal("expected feature date mismatch error")
	}
}

func TestFeatureWalkForwardRejectsPredictionErrors(t *testing.T) {
	model := &featureWalkForwardSpy{failPredict: true}

	_, err := EvaluateFeatureWalkForward(
		Dataset{
			HorizonSessions: 1,
			Rows:            ridgeTestRows(40),
		},
		WalkForwardConfig{
			InitialTrainSize: 20,
			MinimumTrainSize: 10,
			TestSize:         5,
		},
		model,
	)
	if err == nil {
		t.Fatal("expected prediction error")
	}
}

func TestFeatureWalkForwardRejectsNonFinitePredictions(t *testing.T) {
	model := &featureWalkForwardSpy{nonFinite: true}

	_, err := EvaluateFeatureWalkForward(
		Dataset{
			HorizonSessions: 1,
			Rows:            ridgeTestRows(40),
		},
		WalkForwardConfig{
			InitialTrainSize: 20,
			MinimumTrainSize: 10,
			TestSize:         5,
		},
		model,
	)
	if err == nil {
		t.Fatal("expected non-finite prediction error")
	}
}

func TestFeatureWalkForwardReportsFoldDiagnostics(t *testing.T) {
	rows := ridgeTestRows(50)
	model, err := NewRidgeRegression(1)
	if err != nil {
		t.Fatal(err)
	}

	result, err := EvaluateFeatureWalkForward(
		Dataset{HorizonSessions: 1, Rows: rows},
		WalkForwardConfig{
			InitialTrainSize: 25,
			MinimumTrainSize: 10,
			TestSize:         5,
		},
		model,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Folds) != result.FoldCount {
		t.Fatalf("diagnostic folds: got %d, want %d", len(result.Folds), result.FoldCount)
	}

	expectedTraining := []int{24, 29, 34, 39, 44}

	for i, fold := range result.Folds {
		if fold.FoldNumber != i+1 {
			t.Errorf("fold number: got %d, want %d", fold.FoldNumber, i+1)
		}
		if fold.TrainingObservations != expectedTraining[i] {
			t.Errorf("fold %d training count: got %d, want %d",
				i+1, fold.TrainingObservations, expectedTraining[i])
		}
		if fold.TestObservations != 5 {
			t.Errorf("fold %d test count: got %d, want 5", i+1, fold.TestObservations)
		}
		if fold.PurgedObservations != 1 {
			t.Errorf("fold %d purged count: got %d, want 1", i+1, fold.PurgedObservations)
		}
		if !fold.TrainEndDate.Before(fold.TestStartDate) {
			t.Errorf("fold %d training end must precede test start", i+1)
		}
		if fold.TestEndDate.Before(fold.TestStartDate) {
			t.Errorf("fold %d test end precedes test start", i+1)
		}
	}
}
