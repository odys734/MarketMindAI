package forecast

import (
	"math"
	"testing"
)

func floatPointer(value float64) *float64 {
	return &value
}

func TestEvaluateForecastsMetrics(t *testing.T) {
	observations := []ForecastObservation{
		{
			HorizonSessions: 1,
			PredictedReturn: 2,
			ActualReturn:    1,
			UpProbability:   floatPointer(0.8),
			LowerBound:      floatPointer(0),
			UpperBound:      floatPointer(2),
		},
		{
			HorizonSessions: 1,
			PredictedReturn: -1,
			ActualReturn:    -2,
			UpProbability:   floatPointer(0.2),
			LowerBound:      floatPointer(-3),
			UpperBound:      floatPointer(-1),
		},
		{
			HorizonSessions: 1,
			PredictedReturn: 1,
			ActualReturn:    -1,
			UpProbability:   floatPointer(0.7),
			LowerBound:      floatPointer(-2),
			UpperBound:      floatPointer(2),
		},
	}

	metrics, err := EvaluateForecasts(observations)
	if err != nil {
		t.Fatal(err)
	}

	if metrics.ObservationCount != 3 {
		t.Fatalf("observation count: got %d, want 3", metrics.ObservationCount)
	}

	assertNear(t, "MAE", metrics.MAE, 4.0/3.0, 1e-9)
	assertNear(t, "RMSE", metrics.RMSE, math.Sqrt(2), 1e-9)
	assertNear(t, "mean error", metrics.MeanError, 4.0/3.0, 1e-9)
	assertNear(t, "direction accuracy", metrics.DirectionalAccuracy, 200.0/3.0, 1e-9)

	if metrics.BrierScore == nil {
		t.Fatal("expected Brier score")
	}
	assertNear(t, "Brier score", *metrics.BrierScore, 0.19, 1e-9)

	if metrics.IntervalCoverage == nil || metrics.AverageIntervalWidth == nil {
		t.Fatal("expected interval metrics")
	}
	assertNear(t, "interval coverage", *metrics.IntervalCoverage, 100, 1e-9)
	assertNear(t, "average interval width", *metrics.AverageIntervalWidth, 8.0/3.0, 1e-9)
}

func TestEvaluateForecastsOptionalMetrics(t *testing.T) {
	metrics, err := EvaluateForecasts([]ForecastObservation{
		{
			HorizonSessions: 5,
			PredictedReturn: 1,
			ActualReturn:    2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if metrics.BrierScore != nil {
		t.Fatal("Brier score should be nil without probabilities")
	}
	if metrics.IntervalCoverage != nil || metrics.AverageIntervalWidth != nil {
		t.Fatal("interval metrics should be nil without intervals")
	}
}

func TestEvaluateForecastsRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name        string
		observation ForecastObservation
	}{
		{
			name: "invalid horizon",
			observation: ForecastObservation{
				HorizonSessions: 0,
				PredictedReturn: 1,
				ActualReturn:    2,
			},
		},
		{
			name: "invalid probability",
			observation: ForecastObservation{
				HorizonSessions: 1,
				PredictedReturn: 1,
				ActualReturn:    2,
				UpProbability:   floatPointer(1.5),
			},
		},
		{
			name: "incomplete interval",
			observation: ForecastObservation{
				HorizonSessions: 1,
				PredictedReturn: 1,
				ActualReturn:    2,
				LowerBound:      floatPointer(0),
			},
		},
		{
			name: "reversed interval",
			observation: ForecastObservation{
				HorizonSessions: 1,
				PredictedReturn: 1,
				ActualReturn:    2,
				LowerBound:      floatPointer(3),
				UpperBound:      floatPointer(1),
			},
		},
		{
			name: "non-finite return",
			observation: ForecastObservation{
				HorizonSessions: 1,
				PredictedReturn: math.Inf(1),
				ActualReturn:    2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := EvaluateForecasts([]ForecastObservation{tt.observation}); err == nil {
				t.Fatal("expected invalid input error")
			}
		})
	}
}

func TestEvaluateForecastsRequiresObservations(t *testing.T) {
	if _, err := EvaluateForecasts(nil); err == nil {
		t.Fatal("expected empty observation error")
	}
}

func TestEvaluateForecastsRejectsOverflow(t *testing.T) {
	max := math.MaxFloat64
	tests := []struct {
		name        string
		observation ForecastObservation
	}{
		{
			name: "forecast error overflow",
			observation: ForecastObservation{
				HorizonSessions: 1,
				PredictedReturn: max,
				ActualReturn:    -max,
			},
		},
		{
			name: "interval width overflow",
			observation: ForecastObservation{
				HorizonSessions: 1,
				PredictedReturn: 1,
				ActualReturn:    1,
				LowerBound:      floatPointer(-max),
				UpperBound:      floatPointer(max),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := EvaluateForecasts([]ForecastObservation{tt.observation}); err == nil {
				t.Fatal("expected overflow error")
			}
		})
	}
}
