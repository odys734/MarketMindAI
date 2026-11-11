package forecast

import (
	"errors"
	"fmt"
	"math"
)

// ForecastObservation represents one out-of-sample forecast and its
// subsequently observed result. All returns are percentages.
type ForecastObservation struct {
	Date            string   `json:"date"`
	HorizonSessions int      `json:"horizon_sessions"`
	PredictedReturn float64  `json:"predicted_return_pct"`
	ActualReturn    float64  `json:"actual_return_pct"`
	UpProbability   *float64 `json:"up_probability,omitempty"`
	LowerBound      *float64 `json:"lower_bound_pct,omitempty"`
	UpperBound      *float64 `json:"upper_bound_pct,omitempty"`
}

// EvaluationMetrics summarizes out-of-sample forecast performance.
type EvaluationMetrics struct {
	ObservationCount     int      `json:"observation_count"`
	MAE                  float64  `json:"mae"`
	RMSE                 float64  `json:"rmse"`
	MeanError            float64  `json:"mean_error"`
	DirectionalAccuracy  float64  `json:"directional_accuracy_pct"`
	BrierScore           *float64 `json:"brier_score,omitempty"`
	IntervalCoverage     *float64 `json:"interval_coverage_pct,omitempty"`
	AverageIntervalWidth *float64 `json:"average_interval_width_pct,omitempty"`
}

// EvaluateForecasts calculates metrics from completed out-of-sample
// forecast observations. It does not train a model.
func EvaluateForecasts(observations []ForecastObservation) (EvaluationMetrics, error) {
	if len(observations) == 0 {
		return EvaluationMetrics{}, errors.New("at least one forecast observation is required")
	}

	var absoluteErrorSum, squaredErrorSum, errorSum float64
	var directionCorrect int

	var brierSum float64
	brierCount := 0

	var coveredCount int
	var intervalWidthSum float64
	intervalCount := 0

	for i, observation := range observations {
		if observation.HorizonSessions <= 0 {
			return EvaluationMetrics{}, fmt.Errorf("observation %d has invalid horizon", i)
		}
		if !finite(observation.PredictedReturn) || !finite(observation.ActualReturn) {
			return EvaluationMetrics{}, fmt.Errorf("observation %d contains non-finite returns", i)
		}

		errValue := observation.PredictedReturn - observation.ActualReturn
		squaredError := errValue * errValue
		if !finite(errValue) || !finite(squaredError) {
			return EvaluationMetrics{}, fmt.Errorf("observation %d produces non-finite forecast error", i)
		}
		absoluteErrorSum += math.Abs(errValue)
		squaredErrorSum += squaredError
		errorSum += errValue
		if !finite(absoluteErrorSum) || !finite(squaredErrorSum) || !finite(errorSum) {
			return EvaluationMetrics{}, errors.New("forecast error accumulation overflowed")
		}

		if (observation.PredictedReturn > 0 && observation.ActualReturn > 0) ||
			(observation.PredictedReturn < 0 && observation.ActualReturn < 0) ||
			(observation.PredictedReturn == 0 && observation.ActualReturn == 0) {
			directionCorrect++
		}

		if observation.UpProbability != nil {
			probability := *observation.UpProbability
			if !finite(probability) || probability < 0 || probability > 1 {
				return EvaluationMetrics{}, fmt.Errorf("observation %d has invalid up probability", i)
			}
			actualUp := 0.0
			if observation.ActualReturn > 0 {
				actualUp = 1
			}
			difference := probability - actualUp
			brierSum += difference * difference
			if !finite(brierSum) {
				return EvaluationMetrics{}, errors.New("Brier score accumulation overflowed")
			}
			brierCount++
		}

		if observation.LowerBound != nil || observation.UpperBound != nil {
			if observation.LowerBound == nil || observation.UpperBound == nil {
				return EvaluationMetrics{}, fmt.Errorf("observation %d has incomplete interval", i)
			}
			lower := *observation.LowerBound
			upper := *observation.UpperBound
			if !finite(lower) || !finite(upper) || lower > upper {
				return EvaluationMetrics{}, fmt.Errorf("observation %d has invalid interval", i)
			}

			if observation.ActualReturn >= lower && observation.ActualReturn <= upper {
				coveredCount++
			}
			width := upper - lower
			if !finite(width) {
				return EvaluationMetrics{}, fmt.Errorf("observation %d has non-finite interval width", i)
			}
			intervalWidthSum += width
			if !finite(intervalWidthSum) {
				return EvaluationMetrics{}, errors.New("interval width accumulation overflowed")
			}
			intervalCount++
		}
	}

	count := float64(len(observations))
	metrics := EvaluationMetrics{
		ObservationCount:    len(observations),
		MAE:                 absoluteErrorSum / count,
		RMSE:                math.Sqrt(squaredErrorSum / count),
		MeanError:           errorSum / count,
		DirectionalAccuracy: float64(directionCorrect) / count * 100,
	}

	if brierCount > 0 {
		score := brierSum / float64(brierCount)
		metrics.BrierScore = &score
	}
	if intervalCount > 0 {
		coverage := float64(coveredCount) / float64(intervalCount) * 100
		width := intervalWidthSum / float64(intervalCount)
		metrics.IntervalCoverage = &coverage
		metrics.AverageIntervalWidth = &width
	}

	if !finite(metrics.MAE) || !finite(metrics.RMSE) || !finite(metrics.MeanError) ||
		!finite(metrics.DirectionalAccuracy) {
		return EvaluationMetrics{}, errors.New("evaluation produced non-finite metrics")
	}

	if metrics.BrierScore != nil && !finite(*metrics.BrierScore) {
		return EvaluationMetrics{}, errors.New("evaluation produced non-finite Brier score")
	}
	if metrics.IntervalCoverage != nil && !finite(*metrics.IntervalCoverage) {
		return EvaluationMetrics{}, errors.New("evaluation produced non-finite interval coverage")
	}
	if metrics.AverageIntervalWidth != nil && !finite(*metrics.AverageIntervalWidth) {
		return EvaluationMetrics{}, errors.New("evaluation produced non-finite average interval width")
	}

	return metrics, nil
}
