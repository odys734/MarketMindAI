package forecast

import (
	"errors"
	"fmt"
	"time"
)

// FeatureForecastModel predicts each observation using its own features.
type FeatureForecastModel interface {
	Name() string
	Fit(training []DatasetRow, horizon int) error
	Predict(features Features) (float64, error)
}

// FeatureFoldDiagnostic describes the training and test data used by one fold.
type FeatureFoldDiagnostic struct {
	FoldNumber           int             `json:"fold_number"`
	TrainingObservations int             `json:"training_observations"`
	TestObservations     int             `json:"test_observations"`
	PurgedObservations   int             `json:"purged_observations"`
	TrainEndDate         time.Time       `json:"train_end_date"`
	TestStartDate        time.Time       `json:"test_start_date"`
	TestEndDate          time.Time       `json:"test_end_date"`
	Metrics              EvaluationMetrics `json:"metrics"`
}

// FeatureModelMetadataProvider optionally exposes reproducibility metadata.
type FeatureModelMetadataProvider interface {
	Metadata() map[string]any
}

// FeatureWalkForwardResult contains aggregated out-of-sample results
// for models that require row-specific features.
type FeatureWalkForwardResult struct {
	ModelName       string                  `json:"model_name"`
	ModelMetadata   map[string]any          `json:"model_metadata,omitempty"`
	HorizonSessions int                     `json:"horizon_sessions"`
	FoldCount       int                     `json:"fold_count"`
	Folds           []FeatureFoldDiagnostic `json:"folds"`
	Observations    []ForecastObservation   `json:"observations"`
	Metrics         EvaluationMetrics       `json:"metrics"`
}

// EvaluateFeatureWalkForward evaluates a feature-based model using
// independently fitted, purged, chronological walk-forward folds.
func EvaluateFeatureWalkForward(
	dataset Dataset,
	config WalkForwardConfig,
	model FeatureForecastModel,
) (FeatureWalkForwardResult, error) {
	if model == nil {
		return FeatureWalkForwardResult{}, errors.New("feature forecast model is required")
	}

	modelName := model.Name()
	if modelName == "" {
		return FeatureWalkForwardResult{}, errors.New("feature forecast model name is required")
	}

	folds, err := BuildWalkForwardFolds(dataset, config)
	if err != nil {
		return FeatureWalkForwardResult{}, fmt.Errorf("build walk-forward folds: %w", err)
	}

	result := FeatureWalkForwardResult{
		ModelName:       modelName,
		HorizonSessions: dataset.HorizonSessions,
		FoldCount:       len(folds),
		Folds:           make([]FeatureFoldDiagnostic, 0, len(folds)),
		Observations:    make([]ForecastObservation, 0),
	}

	if provider, ok := model.(FeatureModelMetadataProvider); ok {
		result.ModelMetadata = provider.Metadata()
	}

	for _, fold := range folds {
		diagnostic := FeatureFoldDiagnostic{
			FoldNumber:           fold.Number,
			TrainingObservations: len(fold.Train),
			TestObservations:     len(fold.Test),
			PurgedObservations:   fold.PurgedCount,
			TrainEndDate:         fold.TrainEndDate,
			TestStartDate:        fold.TestStartDate,
			TestEndDate:          fold.TestEndDate,
		}

		if err := model.Fit(fold.Train, dataset.HorizonSessions); err != nil {
			return FeatureWalkForwardResult{}, fmt.Errorf(
				"fit %s on fold %d: %w", modelName, fold.Number, err,
			)
		}

		foldObservations := make([]ForecastObservation, 0, len(fold.Test))
		for i, row := range fold.Test {
			if !row.Features.Date.Equal(row.Date) {
				return FeatureWalkForwardResult{}, fmt.Errorf(
					"feature date mismatch for %s on fold %d row %d",
					modelName, fold.Number, i,
				)
			}

			prediction, err := model.Predict(row.Features)
			if err != nil {
				return FeatureWalkForwardResult{}, fmt.Errorf(
					"predict %s on fold %d row %d: %w",
					modelName, fold.Number, i, err,
				)
			}
			if !finite(prediction) {
				return FeatureWalkForwardResult{}, fmt.Errorf(
					"%s produced a non-finite prediction on fold %d row %d",
					modelName, fold.Number, i,
				)
			}

			observation := ForecastObservation{
				Date:            row.Date.Format(time.RFC3339),
				HorizonSessions: dataset.HorizonSessions,
				PredictedReturn: prediction,
				ActualReturn:    row.Target.ForwardReturnPct,
			}
			result.Observations = append(result.Observations, observation)
			foldObservations = append(foldObservations, observation)
		}

		diagnostic.Metrics, err = EvaluateForecasts(foldObservations)
		if err != nil {
			return FeatureWalkForwardResult{}, fmt.Errorf(
				"evaluate %s fold %d: %w", modelName, fold.Number, err,
			)
		}
		result.Folds = append(result.Folds, diagnostic)
	}

	if len(result.Observations) == 0 {
		return FeatureWalkForwardResult{}, errors.New(
			"feature walk-forward evaluation produced no observations",
		)
	}

	result.Metrics, err = EvaluateForecasts(result.Observations)
	if err != nil {
		return FeatureWalkForwardResult{}, fmt.Errorf(
			"evaluate %s: %w", modelName, err,
		)
	}

	return result, nil
}
