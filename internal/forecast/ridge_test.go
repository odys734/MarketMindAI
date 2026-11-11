package forecast

import (
	"math"
	"testing"
	"time"
)

func ridgeTestRows(count int) []DatasetRow {
	rows := make([]DatasetRow, count)
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < count; i++ {
		date := start.AddDate(0, 0, i)
		y := float64(i%9-4) * 0.4
		closePrice := 100 + float64(i)*0.2

		rows[i] = DatasetRow{
			Date: date,
			Features: Features{
				Date:                      date,
				Close:                     closePrice,
				DailyReturnPct:            floatPtr(float64(i%7-3) * 0.2),
				LogReturn:                 floatPtr(float64(i%5-2) * 0.001),
				RSI14:                     floatPtr(float64(35 + i%30)),
				SMA5:                      floatPtr(closePrice * (1 - float64(i%4)*0.001)),
				SMA20:                     floatPtr(closePrice * (1 + float64(i%6)*0.001)),
				EMA12:                     floatPtr(closePrice * (1 - float64(i%3)*0.001)),
				MACD:                      floatPtr(float64(i%5-2) * 0.1),
				Volatility20AnnualizedPct: floatPtr(float64(15 + i%12)),
			},
			Target: Target{
				Date:             date,
				HorizonSessions:  1,
				TargetDate:       date.AddDate(0, 0, 1),
				StartClose:       100,
				TargetClose:      100 * (1 + y/100),
				ForwardReturnPct: y,
			},
		}
	}

	return rows
}

func TestRidgeRegressionFitAndPredict(t *testing.T) {
	model, err := NewRidgeRegression(1)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := model.Predict(Features{}); err == nil {
		t.Fatal("expected prediction error before fitting")
	}

	training := ridgeTestRows(40)
	if err := model.Fit(training, 1); err != nil {
		t.Fatal(err)
	}

	prediction, err := model.Predict(training[len(training)-1].Features)
	if err != nil {
		t.Fatal(err)
	}
	if !finite(prediction) {
		t.Fatalf("prediction is not finite: %v", prediction)
	}
}

func TestRidgeRegressionRejectsInvalidLambda(t *testing.T) {
	for _, lambda := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := NewRidgeRegression(lambda); err == nil {
			t.Errorf("expected invalid lambda error for %v", lambda)
		}
	}
}

func TestRidgeRegressionRequiresEnoughCompleteRows(t *testing.T) {
	model, err := NewRidgeRegression(1)
	if err != nil {
		t.Fatal(err)
	}

	if err := model.Fit(ridgeTestRows(8), 1); err == nil {
		t.Fatal("expected insufficient complete training data error")
	}
}

func TestRidgeRegressionSkipsIncompleteFeatures(t *testing.T) {
	model, err := NewRidgeRegression(1)
	if err != nil {
		t.Fatal(err)
	}

	rows := ridgeTestRows(30)
	for i := 0; i < 25; i++ {
		rows[i].Features.RSI14 = nil
	}

	if err := model.Fit(rows, 1); err == nil {
		t.Fatal("expected insufficient complete observations error")
	}
}

func TestRidgeRegressionRejectsInvalidPredictionFeatures(t *testing.T) {
	model, err := NewRidgeRegression(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Fit(ridgeTestRows(30), 1); err != nil {
		t.Fatal(err)
	}

	if _, err := model.Predict(Features{}); err == nil {
		t.Fatal("expected incomplete feature error")
	}
}

func TestRidgeFeatureVectorRejectsInvalidIndicators(t *testing.T) {
	base := ridgeTestRows(1)[0].Features

	tests := []struct {
		name   string
		mutate func(*Features)
	}{
		{"negative RSI", func(f *Features) { *f.RSI14 = -1 }},
		{"RSI above 100", func(f *Features) { *f.RSI14 = 101 }},
		{"zero SMA5", func(f *Features) { *f.SMA5 = 0 }},
		{"negative SMA20", func(f *Features) { *f.SMA20 = -1 }},
		{"zero EMA12", func(f *Features) { *f.EMA12 = 0 }},
		{"negative volatility", func(f *Features) { *f.Volatility20AnnualizedPct = -1 }},
		{"NaN MACD", func(f *Features) { *f.MACD = math.NaN() }},
		{"infinite daily return", func(f *Features) { *f.DailyReturnPct = math.Inf(1) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			features := base
			tt.mutate(&features)

			if _, ok := ridgeFeatureVector(features); ok {
				t.Fatal("expected invalid feature vector to be rejected")
			}
		})
	}
}

func TestRidgeRegressionFailedRefitClearsPreviousModel(t *testing.T) {
	model, err := NewRidgeRegression(1)
	if err != nil {
		t.Fatal(err)
	}

	training := ridgeTestRows(30)
	if err := model.Fit(training, 1); err != nil {
		t.Fatal(err)
	}

	if _, err := model.Predict(training[0].Features); err != nil {
		t.Fatalf("expected initial prediction to succeed: %v", err)
	}

	if err := model.Fit(ridgeTestRows(8), 1); err == nil {
		t.Fatal("expected refit with insufficient data to fail")
	}

	if _, err := model.Predict(training[0].Features); err == nil {
		t.Fatal("expected prediction to fail after unsuccessful refit")
	}
}

func TestRidgeRegressionRejectsOverflowingPrediction(t *testing.T) {
	model, err := NewRidgeRegression(1)
	if err != nil {
		t.Fatal(err)
	}

	training := ridgeTestRows(30)
	if err := model.Fit(training, 1); err != nil {
		t.Fatal(err)
	}

	features := training[0].Features
	features.Close = math.MaxFloat64

	if _, err := model.Predict(features); err == nil {
		t.Fatal("expected overflowing prediction features to be rejected")
	}
}

func TestSolveRidgeSystemDiagonalMatrix(t *testing.T) {
	var matrix [ridgeFeatureCount][ridgeFeatureCount]float64
	var vector [ridgeFeatureCount]float64

	for i := 0; i < ridgeFeatureCount; i++ {
		matrix[i][i] = 2
		vector[i] = float64(i+1) * 2
	}

	result, err := solveRidgeSystem(matrix, vector)
	if err != nil {
		t.Fatal(err)
	}

	for i, value := range result {
		expected := float64(i + 1)
		if math.Abs(value-expected) > 1e-10 {
			t.Errorf("coefficient %d: got %v, want %v", i, value, expected)
		}
	}
}

func TestSolveRidgeSystemRejectsInvalidInputs(t *testing.T) {
	t.Run("NaN matrix", func(t *testing.T) {
		var matrix [ridgeFeatureCount][ridgeFeatureCount]float64
		var vector [ridgeFeatureCount]float64
		for i := range matrix {
			matrix[i][i] = 1
		}
		matrix[0][0] = math.NaN()

		if _, err := solveRidgeSystem(matrix, vector); err == nil {
			t.Fatal("expected non-finite matrix error")
		}
	})

	t.Run("infinite vector", func(t *testing.T) {
		var matrix [ridgeFeatureCount][ridgeFeatureCount]float64
		var vector [ridgeFeatureCount]float64
		for i := range matrix {
			matrix[i][i] = 1
		}
		vector[0] = math.Inf(1)

		if _, err := solveRidgeSystem(matrix, vector); err == nil {
			t.Fatal("expected non-finite vector error")
		}
	})

	t.Run("singular matrix", func(t *testing.T) {
		var matrix [ridgeFeatureCount][ridgeFeatureCount]float64
		var vector [ridgeFeatureCount]float64

		if _, err := solveRidgeSystem(matrix, vector); err == nil {
			t.Fatal("expected singular matrix error")
		}
	})
}
