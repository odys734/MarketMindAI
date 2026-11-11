package forecast

import (
	"errors"
	"fmt"
	"time"
)

// BaselineWalkForwardResult contains aggregated out-of-sample results.
type BaselineWalkForwardResult struct {
	ModelName       string                `json:"model_name"`
	HorizonSessions int                   `json:"horizon_sessions"`
	FoldCount       int                   `json:"fold_count"`
	Folds           []FeatureFoldDiagnostic `json:"folds"`
	Observations    []ForecastObservation `json:"observations"`
	Metrics         EvaluationMetrics     `json:"metrics"`
}

// EvaluateBaselineWalkForward fits the model independently on each
// purged training fold and evaluates only its forward test observations.
func EvaluateBaselineWalkForward(
	dataset Dataset,
	config WalkForwardConfig,
	model BaselineModel,
) (BaselineWalkForwardResult, error) {
	if model == nil {
		return BaselineWalkForwardResult{}, errors.New("baseline model is required")
	}

	folds, err := BuildWalkForwardFolds(dataset, config)
	if err != nil {
		return BaselineWalkForwardResult{}, fmt.Errorf("build walk-forward folds: %w", err)
	}

	result := BaselineWalkForwardResult{
		ModelName:       model.Name(),
		HorizonSessions: dataset.HorizonSessions,
		FoldCount:       len(folds),
		Folds:           make([]FeatureFoldDiagnostic, 0, len(folds)),
		Observations:    make([]ForecastObservation, 0),
	}

	if result.ModelName == "" {
		return BaselineWalkForwardResult{}, errors.New("baseline model name is required")
	}

	for _, fold := range folds {
		result.Folds = append(result.Folds, FeatureFoldDiagnostic{
			FoldNumber:           fold.Number,
			TrainingObservations: len(fold.Train),
			TestObservations:     len(fold.Test),
			PurgedObservations:   fold.PurgedCount,
			TrainEndDate:         fold.TrainEndDate,
			TestStartDate:        fold.TestStartDate,
			TestEndDate:          fold.TestEndDate,
		})
		if err := model.Fit(fold.Train, dataset.HorizonSessions); err != nil {
			return BaselineWalkForwardResult{}, fmt.Errorf(
				"fit %s on fold %d: %w",
				result.ModelName,
				fold.Number,
				err,
			)
		}

		for i, row := range fold.Test {
			prediction, err := model.Predict()
			if err != nil {
				return BaselineWalkForwardResult{}, fmt.Errorf(
					"predict %s on fold %d row %d: %w",
					result.ModelName,
					fold.Number,
					i,
					err,
				)
			}
			if !finite(prediction) {
				return BaselineWalkForwardResult{}, fmt.Errorf(
					"%s produced a non-finite prediction on fold %d row %d",
					result.ModelName,
					fold.Number,
					i,
				)
			}

			result.Observations = append(result.Observations, ForecastObservation{
				Date:            row.Date.Format(time.RFC3339),
				HorizonSessions: dataset.HorizonSessions,
				PredictedReturn: prediction,
				ActualReturn:    row.Target.ForwardReturnPct,
			})
		}
	}

	if len(result.Observations) == 0 {
		return BaselineWalkForwardResult{}, errors.New("walk-forward evaluation produced no observations")
	}

	result.Metrics, err = EvaluateForecasts(result.Observations)
	if err != nil {
		return BaselineWalkForwardResult{}, fmt.Errorf("evaluate %s: %w", result.ModelName, err)
	}

	return result, nil
}
