package forecast

import (
	"encoding/json"
	"strings"
	"testing"
)

func experimentTestDataset(horizon, count int) Dataset {
	rows := ridgeTestRows(count)
	for i := range rows {
		rows[i].Target.HorizonSessions = horizon
	}
	return Dataset{
		HorizonSessions: horizon,
		Rows:            rows,
		IncompleteCount: horizon,
	}
}

func TestRunForecastExperimentMultipleHorizons(t *testing.T) {
	datasets := map[int]Dataset{
		20: experimentTestDataset(20, 60),
		1:  experimentTestDataset(1, 60),
		5:  experimentTestDataset(5, 60),
	}

	baselineCreations := 0
	ridgeCreations := 0

	factories := []ForecastModelFactory{
		{
			NewBaseline: func() BaselineModel {
				baselineCreations++
				return &ZeroReturnBaseline{}
			},
		},
		{
			NewBaseline: func() BaselineModel {
				return &HistoricalMeanBaseline{}
			},
		},
		{
			NewFeature: func() FeatureForecastModel {
				ridgeCreations++
				model, err := NewRidgeRegression(1)
				if err != nil {
					t.Fatal(err)
				}
				return model
			},
		},
	}

	report, err := RunForecastExperiment(
		datasets,
		WalkForwardConfig{
			InitialTrainSize: 30,
			MinimumTrainSize: 10,
			TestSize:         5,
		},
		factories,
	)
	if err != nil {
		t.Fatal(err)
	}

	if report.SchemaVersion != "1.0" || report.GeneratedAt.IsZero() {
		t.Fatal("report metadata is incomplete")
	}
	if len(report.Horizons) != 3 {
		t.Fatalf("horizon count: got %d, want 3", len(report.Horizons))
	}
	if baselineCreations != 3 || ridgeCreations != 3 {
		t.Fatalf("factory calls: baseline=%d ridge=%d, want 3 each",
			baselineCreations, ridgeCreations)
	}

	expectedHorizons := []int{1, 5, 20}
	for i, result := range report.Horizons {
		if result.HorizonSessions != expectedHorizons[i] {
			t.Errorf("horizon %d: got %d, want %d",
				i, result.HorizonSessions, expectedHorizons[i])
		}
		if result.RowCount != 60 || result.IncompleteCount != result.HorizonSessions {
			t.Errorf("unexpected dataset diagnostics for horizon %d", result.HorizonSessions)
		}
		if result.Comparison.ModelCount != 3 {
			t.Errorf("horizon %d model count: got %d, want 3",
				result.HorizonSessions, result.Comparison.ModelCount)
		}
		for _, model := range result.Comparison.Models {
			if model.Metrics.ObservationCount == 0 {
				t.Errorf("model %q has no evaluated observations", model.ModelName)
			}
		}
	}
}

func TestRunForecastExperimentRejectsInvalidInputs(t *testing.T) {
	validConfig := WalkForwardConfig{
		InitialTrainSize: 30,
		MinimumTrainSize: 10,
		TestSize:         5,
	}
	validFactory := []ForecastModelFactory{
		{NewBaseline: func() BaselineModel { return &ZeroReturnBaseline{} }},
	}

	tests := []struct {
		name      string
		datasets  map[int]Dataset
		factories []ForecastModelFactory
	}{
		{"no datasets", nil, validFactory},
		{"no factories", map[int]Dataset{1: experimentTestDataset(1, 60)}, nil},
		{"unsupported horizon", map[int]Dataset{2: experimentTestDataset(2, 60)}, validFactory},
		{"mismatched horizon", map[int]Dataset{5: experimentTestDataset(1, 60)}, validFactory},
		{"empty rows", map[int]Dataset{1: {HorizonSessions: 1}}, validFactory},
		{"negative incomplete count", map[int]Dataset{
			1: {HorizonSessions: 1, Rows: ridgeTestRows(60), IncompleteCount: -1},
		}, validFactory},
		{"missing constructor", map[int]Dataset{1: experimentTestDataset(1, 60)}, []ForecastModelFactory{{}}},
		{"two constructors", map[int]Dataset{1: experimentTestDataset(1, 60)}, []ForecastModelFactory{
			{
				NewBaseline: func() BaselineModel { return &ZeroReturnBaseline{} },
				NewFeature:  func() FeatureForecastModel { return &featureWalkForwardSpy{} },
			},
		}},
		{"nil model returned", map[int]Dataset{1: experimentTestDataset(1, 60)}, []ForecastModelFactory{
			{NewBaseline: func() BaselineModel { return (*ZeroReturnBaseline)(nil) }},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := RunForecastExperiment(tt.datasets, validConfig, tt.factories)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestMarshalForecastExperimentReport(t *testing.T) {
	report := ForecastExperimentReport{
		SchemaVersion: "1.0",
		GeneratedAt:   experimentTestDataset(1, 2).Rows[0].Date,
		Horizons: []ExperimentHorizonResult{
			{HorizonSessions: 1, RowCount: 2},
		},
	}

	data, err := MarshalForecastExperimentReport(report)
	if err != nil {
		t.Fatal(err)
	}

	var decoded ForecastExperimentReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != report.SchemaVersion || len(decoded.Horizons) != 1 {
		t.Fatal("JSON report did not round-trip correctly")
	}
	if !strings.Contains(string(data), `"horizon_sessions": 1`) {
		t.Fatal("JSON report is missing horizon metadata")
	}
}

func TestMarshalForecastExperimentReportRejectsIncompleteReport(t *testing.T) {
	_, err := MarshalForecastExperimentReport(ForecastExperimentReport{})
	if err == nil {
		t.Fatal("expected incomplete report error")
	}
}
