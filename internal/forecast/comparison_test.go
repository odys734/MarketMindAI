package forecast

import (
	"strings"
	"testing"
)

func TestCompareForecastModels(t *testing.T) {
	dataset := Dataset{
		HorizonSessions: 1,
		Rows:            ridgeTestRows(50),
	}
	config := WalkForwardConfig{
		InitialTrainSize: 25,
		MinimumTrainSize: 10,
		TestSize:         5,
	}

	ridge, err := NewRidgeRegression(1)
	if err != nil {
		t.Fatal(err)
	}

	result, err := CompareForecastModels(
		dataset,
		config,
		[]BaselineModel{
			&ZeroReturnBaseline{},
			&HistoricalMeanBaseline{},
		},
		[]FeatureForecastModel{ridge},
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.ModelCount != 3 || len(result.Models) != 3 {
		t.Fatalf("model count: got %d/%d, want 3", result.ModelCount, len(result.Models))
	}
	if result.HorizonSessions != 1 {
		t.Fatalf("horizon: got %d, want 1", result.HorizonSessions)
	}

	expectedNames := []string{"zero_return", "historical_mean", "ridge_regression"}
	expectedTypes := []string{"baseline", "baseline", "feature"}

	for i, model := range result.Models {
		if model.ModelName != expectedNames[i] {
			t.Errorf("model %d name: got %q, want %q", i, model.ModelName, expectedNames[i])
		}
		if model.ModelType != expectedTypes[i] {
			t.Errorf("model %d type: got %q, want %q", i, model.ModelType, expectedTypes[i])
		}
		if model.FoldCount != 5 {
			t.Errorf("model %q fold count: got %d, want 5", model.ModelName, model.FoldCount)
		}
		if len(model.Observations) != 25 {
			t.Errorf("model %q observations: got %d, want 25", model.ModelName, len(model.Observations))
		}
		if model.Metrics.ObservationCount != 25 {
			t.Errorf("model %q metric count: got %d, want 25", model.ModelName, model.Metrics.ObservationCount)
		}
	}

	ridgeResult := result.Models[2]
	if ridgeResult.ModelMetadata["lambda"] != float64(1) {
		t.Errorf("ridge lambda metadata: got %v, want 1", ridgeResult.ModelMetadata["lambda"])
	}

	for modelIndex := 1; modelIndex < len(result.Models); modelIndex++ {
		for i, observation := range result.Models[modelIndex].Observations {
			reference := result.Models[0].Observations[i]
			if observation.Date != reference.Date ||
				observation.ActualReturn != reference.ActualReturn {
				t.Fatalf("model %q is not aligned at observation %d",
					result.Models[modelIndex].ModelName, i)
			}
		}
	}
}

func TestCompareForecastModelsRejectsNoModels(t *testing.T) {
	_, err := CompareForecastModels(Dataset{}, WalkForwardConfig{}, nil, nil)
	if err == nil {
		t.Fatal("expected missing models error")
	}
}

func TestCompareForecastModelsRejectsDuplicateNames(t *testing.T) {
	_, err := CompareForecastModels(
		Dataset{
			HorizonSessions: 1,
			Rows:            ridgeTestRows(50),
		},
		WalkForwardConfig{
			InitialTrainSize: 25,
			MinimumTrainSize: 10,
			TestSize:         5,
		},
		[]BaselineModel{
			&ZeroReturnBaseline{},
			&ZeroReturnBaseline{},
		},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate model name error, got %v", err)
	}
}

func TestCompareForecastModelsRejectsTypedNil(t *testing.T) {
	var model *ZeroReturnBaseline

	_, err := CompareForecastModels(
		Dataset{},
		WalkForwardConfig{},
		[]BaselineModel{model},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("expected typed nil model error, got %v", err)
	}
}

func TestValidateComparisonAlignmentRejectsDifferentObservations(t *testing.T) {
	models := []ModelComparisonEntry{
		{
			ModelName: "first",
			Observations: []ForecastObservation{
				{Date: "2024-01-01T00:00:00Z", HorizonSessions: 1, ActualReturn: 1},
			},
		},
		{
			ModelName: "second",
			Observations: []ForecastObservation{
				{Date: "2024-01-02T00:00:00Z", HorizonSessions: 1, ActualReturn: 1},
			},
		},
	}

	if err := validateComparisonAlignment(models); err == nil {
		t.Fatal("expected alignment error")
	}
}
