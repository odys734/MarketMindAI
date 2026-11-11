package forecast

import (
	"errors"
	"fmt"
	"math"
)

const ridgeFeatureCount = 8

// RidgeRegression is a regularized linear model for forward returns.
// All preprocessing parameters are learned from training observations only.
type RidgeRegression struct {
	Lambda float64

	means        [ridgeFeatureCount]float64
	scales       [ridgeFeatureCount]float64
	coefficients [ridgeFeatureCount]float64
	intercept    float64
	horizon      int
	fitted       bool
	sampleCount  int
}

func NewRidgeRegression(lambda float64) (*RidgeRegression, error) {
	if !finite(lambda) || lambda <= 0 {
		return nil, errors.New("ridge lambda must be finite and positive")
	}
	return &RidgeRegression{Lambda: lambda}, nil
}

func (m *RidgeRegression) Name() string {
	return "ridge_regression"
}

// Metadata returns the configuration needed to identify this model run.
func (m *RidgeRegression) Metadata() map[string]any {
	if m == nil {
		return nil
	}
	return map[string]any{
		"lambda":                        m.Lambda,
		"feature_count":                 ridgeFeatureCount,
		"minimum_complete_observations": ridgeFeatureCount + 2,
	}
}

func (m *RidgeRegression) Fit(training []DatasetRow, horizon int) error {
	if m == nil {
		return errors.New("ridge model is nil")
	}

	// Never retain stale parameters after a failed refit.
	m.fitted = false
	m.means = [ridgeFeatureCount]float64{}
	m.scales = [ridgeFeatureCount]float64{}
	m.coefficients = [ridgeFeatureCount]float64{}
	m.intercept = 0
	m.horizon = 0
	m.sampleCount = 0

	if !finite(m.Lambda) || m.Lambda <= 0 {
		return errors.New("ridge lambda must be finite and positive")
	}
	if err := validateBaselineTraining(training, horizon); err != nil {
		return err
	}

	type sample struct {
		x [ridgeFeatureCount]float64
		y float64
	}

	samples := make([]sample, 0, len(training))
	for i, row := range training {
		if !row.Features.Date.Equal(row.Date) {
			return fmt.Errorf("feature date mismatch at training row %d", i)
		}

		x, ok := ridgeFeatureVector(row.Features)
		if !ok {
			continue
		}
		samples = append(samples, sample{x: x, y: row.Target.ForwardReturnPct})
	}

	if len(samples) < ridgeFeatureCount+2 {
		return fmt.Errorf(
			"ridge requires at least %d complete training observations; got %d",
			ridgeFeatureCount+2, len(samples),
		)
	}

	var means [ridgeFeatureCount]float64
	meanY := 0.0

	for i, sample := range samples {
		n := float64(i + 1)
		deltaY := sample.y - meanY
		if !finite(deltaY) {
			return errors.New("ridge target mean calculation overflowed")
		}
		meanY += deltaY / n
		if !finite(meanY) {
			return errors.New("ridge target mean is not finite")
		}

		for j, value := range sample.x {
			delta := value - means[j]
			if !finite(delta) {
				return fmt.Errorf("ridge feature mean %d calculation overflowed", j)
			}
			means[j] += delta / n
			if !finite(means[j]) {
				return fmt.Errorf("ridge feature mean %d is not finite", j)
			}
		}
	}

	var scales [ridgeFeatureCount]float64
	for _, sample := range samples {
		for j, value := range sample.x {
			difference := value - means[j]
			squared := difference * difference
			if !finite(difference) || !finite(squared) {
				return fmt.Errorf("ridge feature variance %d overflowed", j)
			}
			scales[j] += squared
			if !finite(scales[j]) {
				return fmt.Errorf("ridge feature variance %d is not finite", j)
			}
		}
	}

	for j := range scales {
		scales[j] = math.Sqrt(scales[j] / float64(len(samples)))
		if !finite(scales[j]) {
			return fmt.Errorf("ridge feature scale %d is not finite", j)
		}
		if scales[j] < 1e-12 {
			scales[j] = 1
		}
	}

	var matrix [ridgeFeatureCount][ridgeFeatureCount]float64
	var vector [ridgeFeatureCount]float64

	for _, sample := range samples {
		var normalized [ridgeFeatureCount]float64

		for j, value := range sample.x {
			normalized[j] = (value - means[j]) / scales[j]
			if !finite(normalized[j]) {
				return fmt.Errorf("ridge normalized feature %d is not finite", j)
			}
		}

		centeredY := sample.y - meanY
		if !finite(centeredY) {
			return errors.New("ridge centered target is not finite")
		}

		for j := 0; j < ridgeFeatureCount; j++ {
			contribution := normalized[j] * centeredY
			if !finite(contribution) {
				return errors.New("ridge target matrix contribution overflowed")
			}
			vector[j] += contribution
			if !finite(vector[j]) {
				return errors.New("ridge target vector overflowed")
			}

			for k := 0; k < ridgeFeatureCount; k++ {
				contribution := normalized[j] * normalized[k]
				if !finite(contribution) {
					return errors.New("ridge feature matrix contribution overflowed")
				}
				matrix[j][k] += contribution
				if !finite(matrix[j][k]) {
					return errors.New("ridge feature matrix overflowed")
				}
			}
		}
	}

	for j := 0; j < ridgeFeatureCount; j++ {
		matrix[j][j] += m.Lambda
		if !finite(matrix[j][j]) {
			return errors.New("ridge regularized matrix is not finite")
		}
	}

	coefficients, err := solveRidgeSystem(matrix, vector)
	if err != nil {
		return fmt.Errorf("solve ridge regression: %w", err)
	}

	intercept := meanY
	if !finite(intercept) {
		return errors.New("ridge intercept is not finite")
	}

	m.means = means
	m.scales = scales
	m.coefficients = coefficients
	m.intercept = intercept
	m.horizon = horizon
	m.sampleCount = len(samples)
	m.fitted = true

	return nil
}

func (m *RidgeRegression) Predict(features Features) (float64, error) {
	if m == nil || !m.fitted {
		return 0, errors.New("ridge model has not been fitted")
	}

	x, ok := ridgeFeatureVector(features)
	if !ok {
		return 0, errors.New("prediction features are incomplete or invalid")
	}

	prediction := m.intercept
	for j, value := range x {
		normalized := (value - m.means[j]) / m.scales[j]
		prediction += normalized * m.coefficients[j]
	}

	if !finite(prediction) {
		return 0, errors.New("ridge prediction is not finite")
	}

	return prediction, nil
}

func ridgeFeatureVector(f Features) ([ridgeFeatureCount]float64, bool) {
	var result [ridgeFeatureCount]float64

	if !finitePositive(f.Close) ||
		f.DailyReturnPct == nil ||
		f.LogReturn == nil ||
		f.RSI14 == nil ||
		f.SMA5 == nil ||
		f.SMA20 == nil ||
		f.EMA12 == nil ||
		f.MACD == nil ||
		f.Volatility20AnnualizedPct == nil {
		return result, false
	}

	if !finite(*f.DailyReturnPct) ||
		!finite(*f.LogReturn) ||
		!finite(*f.RSI14) ||
		*f.RSI14 < 0 || *f.RSI14 > 100 ||
		!finitePositive(*f.SMA5) ||
		!finitePositive(*f.SMA20) ||
		!finitePositive(*f.EMA12) ||
		!finite(*f.MACD) ||
		!finite(*f.Volatility20AnnualizedPct) ||
		*f.Volatility20AnnualizedPct < 0 {
		return result, false
	}

	values := [ridgeFeatureCount]float64{
		*f.DailyReturnPct / 100,
		*f.LogReturn,
		(*f.RSI14 - 50) / 50,
		f.Close / *f.SMA5 - 1,
		f.Close / *f.SMA20 - 1,
		f.Close / *f.EMA12 - 1,
		*f.MACD / f.Close,
		*f.Volatility20AnnualizedPct / 100,
	}

	for i, value := range values {
		if !finite(value) {
			return [ridgeFeatureCount]float64{}, false
		}
		result[i] = value
	}

	return result, true
}

func solveRidgeSystem(
	matrix [ridgeFeatureCount][ridgeFeatureCount]float64,
	vector [ridgeFeatureCount]float64,
) ([ridgeFeatureCount]float64, error) {
	var result [ridgeFeatureCount]float64

	// Reject invalid input before performing elimination.
	for i := 0; i < ridgeFeatureCount; i++ {
		if !finite(vector[i]) {
			return result, errors.New("ridge solver input vector is not finite")
		}
		for j := 0; j < ridgeFeatureCount; j++ {
			if !finite(matrix[i][j]) {
				return result, errors.New("ridge solver input matrix is not finite")
			}
		}
	}

	for column := 0; column < ridgeFeatureCount; column++ {
		pivot := column

		for row := column + 1; row < ridgeFeatureCount; row++ {
			if math.Abs(matrix[row][column]) > math.Abs(matrix[pivot][column]) {
				pivot = row
			}
		}

		pivotValue := matrix[pivot][column]
		if !finite(pivotValue) || math.Abs(pivotValue) < 1e-14 {
			return result, errors.New("singular regularized feature matrix")
		}

		matrix[column], matrix[pivot] = matrix[pivot], matrix[column]
		vector[column], vector[pivot] = vector[pivot], vector[column]

		divisor := matrix[column][column]
		if !finite(divisor) || divisor == 0 {
			return result, errors.New("invalid ridge solver pivot")
		}

		for j := column; j < ridgeFeatureCount; j++ {
			matrix[column][j] /= divisor
			if !finite(matrix[column][j]) {
				return result, errors.New("ridge solver normalization overflowed")
			}
		}

		vector[column] /= divisor
		if !finite(vector[column]) {
			return result, errors.New("ridge solver vector normalization overflowed")
		}

		for row := 0; row < ridgeFeatureCount; row++ {
			if row == column {
				continue
			}

			factor := matrix[row][column]
			if !finite(factor) {
				return result, errors.New("ridge solver elimination factor is not finite")
			}

			for j := column; j < ridgeFeatureCount; j++ {
				product := factor * matrix[column][j]
				if !finite(product) {
					return result, errors.New("ridge solver elimination overflowed")
				}

				matrix[row][j] -= product
				if !finite(matrix[row][j]) {
					return result, errors.New("ridge solver matrix became non-finite")
				}
			}

			product := factor * vector[column]
			if !finite(product) {
				return result, errors.New("ridge solver vector elimination overflowed")
			}

			vector[row] -= product
			if !finite(vector[row]) {
				return result, errors.New("ridge solver vector became non-finite")
			}
		}
	}

	for i := range result {
		if !finite(vector[i]) {
			return result, errors.New("ridge coefficient is not finite")
		}
		result[i] = vector[i]
	}

	return result, nil
}
