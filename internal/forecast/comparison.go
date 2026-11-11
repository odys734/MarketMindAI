package forecast

import (
	"errors"
	"fmt"
	"reflect"
)

// ModelComparisonEntry contains one model's out-of-sample evaluation.
type ModelComparisonEntry struct {
	ModelName       string                `json:"model_name"`
	ModelType       string                `json:"model_type"`
	ModelMetadata   map[string]any        `json:"model_metadata,omitempty"`
	HorizonSessions int                   `json:"horizon_sessions"`
	FoldCount       int                   `json:"fold_count"`
	Folds           []FeatureFoldDiagnostic `json:"folds"`
	Observations    []ForecastObservation `json:"observations"`
	Metrics         EvaluationMetrics     `json:"metrics"`
}

// ModelComparisonResult contains comparable results from the same dataset
// and walk-forward configuration.
type ModelComparisonResult struct {
	HorizonSessions int                    `json:"horizon_sessions"`
	ModelCount      int                    `json:"model_count"`
	Models          []ModelComparisonEntry `json:"models"`
}

// CompareForecastModels evaluates baseline and feature-based models using
// identical chronological walk-forward settings.
func CompareForecastModels(
	dataset Dataset,
	config WalkForwardConfig,
	baselineModels []BaselineModel,
	featureModels []FeatureForecastModel,
) (ModelComparisonResult, error) {
	if len(baselineModels)+len(featureModels) == 0 {
		return ModelComparisonResult{}, errors.New("at least one forecast model is required")
	}

	result := ModelComparisonResult{
		HorizonSessions: dataset.HorizonSessions,
		Models:          make([]ModelComparisonEntry, 0, len(baselineModels)+len(featureModels)),
	}

	seenNames := make(map[string]struct{})

	for i, model := range baselineModels {
		if isNilForecastModel(model) {
			return ModelComparisonResult{}, fmt.Errorf("baseline model %d is nil", i)
		}
		name := model.Name()
		if name == "" {
			return ModelComparisonResult{}, fmt.Errorf("baseline model %d has an empty name", i)
		}
		if _, exists := seenNames[name]; exists {
			return ModelComparisonResult{}, fmt.Errorf("duplicate forecast model name %q", name)
		}
		seenNames[name] = struct{}{}

		evaluation, err := EvaluateBaselineWalkForward(dataset, config, model)
		if err != nil {
			return ModelComparisonResult{}, fmt.Errorf("evaluate baseline %q: %w", name, err)
		}

		result.Models = append(result.Models, ModelComparisonEntry{
			ModelName:       evaluation.ModelName,
			ModelType:       "baseline",
			HorizonSessions: evaluation.HorizonSessions,
			FoldCount:       evaluation.FoldCount,
			Folds:           evaluation.Folds,
			Observations:    evaluation.Observations,
			Metrics:         evaluation.Metrics,
		})
	}

	for i, model := range featureModels {
		if isNilForecastModel(model) {
			return ModelComparisonResult{}, fmt.Errorf("feature model %d is nil", i)
		}
		name := model.Name()
		if name == "" {
			return ModelComparisonResult{}, fmt.Errorf("feature model %d has an empty name", i)
		}
		if _, exists := seenNames[name]; exists {
			return ModelComparisonResult{}, fmt.Errorf("duplicate forecast model name %q", name)
		}
		seenNames[name] = struct{}{}

		evaluation, err := EvaluateFeatureWalkForward(dataset, config, model)
		if err != nil {
			return ModelComparisonResult{}, fmt.Errorf("evaluate feature model %q: %w", name, err)
		}

		result.Models = append(result.Models, ModelComparisonEntry{
			ModelName:       evaluation.ModelName,
			ModelType:       "feature",
			ModelMetadata:   evaluation.ModelMetadata,
			HorizonSessions: evaluation.HorizonSessions,
			FoldCount:       evaluation.FoldCount,
			Observations:    evaluation.Observations,
			Metrics:         evaluation.Metrics,
		})
	}

	if err := validateComparisonAlignment(result.Models); err != nil {
		return ModelComparisonResult{}, err
	}

	result.ModelCount = len(result.Models)
	return result, nil
}

func isNilForecastModel(model any) bool {
	if model == nil {
		return true
	}

	value := reflect.ValueOf(model)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func validateComparisonAlignment(models []ModelComparisonEntry) error {
	if len(models) < 2 {
		return nil
	}

	reference := models[0]
	for modelIndex := 1; modelIndex < len(models); modelIndex++ {
		current := models[modelIndex]

		if current.HorizonSessions != reference.HorizonSessions {
			return fmt.Errorf("model %q has a different evaluation horizon", current.ModelName)
		}
		if len(current.Observations) != len(reference.Observations) {
			return fmt.Errorf("model %q has a different observation count", current.ModelName)
		}

		for i, observation := range current.Observations {
			expected := reference.Observations[i]
			if observation.Date != expected.Date ||
				observation.HorizonSessions != expected.HorizonSessions ||
				observation.ActualReturn != expected.ActualReturn {
				return fmt.Errorf(
					"model %q is not aligned with comparison reference at observation %d",
					current.ModelName,
					i,
				)
			}
		}
	}

	return nil
}
