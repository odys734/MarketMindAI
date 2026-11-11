package forecast

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ForecastModelFactory creates a fresh model for each horizon.
type ForecastModelFactory struct {
	NewBaseline func() BaselineModel
	NewFeature  func() FeatureForecastModel
}

// ExperimentHorizonResult records one independently evaluated horizon.
type ExperimentHorizonResult struct {
	HorizonSessions int                   `json:"horizon_sessions"`
	FirstDate       time.Time             `json:"first_date"`
	LastDate        time.Time             `json:"last_date"`
	RowCount        int                   `json:"row_count"`
	IncompleteCount int                   `json:"incomplete_count"`
	Comparison      ModelComparisonResult `json:"comparison"`
}

// ForecastExperimentReport is a reproducible multi-horizon evaluation report.
type ForecastExperimentReport struct {
	SchemaVersion string                    `json:"schema_version"`
	GeneratedAt   time.Time                 `json:"generated_at"`
	Configuration WalkForwardConfig         `json:"configuration"`
	Horizons      []ExperimentHorizonResult `json:"horizons"`
}

// RunForecastExperiment evaluates fresh model instances on each supplied
// horizon dataset using the same walk-forward configuration.
func RunForecastExperiment(
	datasets map[int]Dataset,
	config WalkForwardConfig,
	factories []ForecastModelFactory,
) (ForecastExperimentReport, error) {
	if len(datasets) == 0 {
		return ForecastExperimentReport{}, errors.New("at least one horizon dataset is required")
	}
	if len(factories) == 0 {
		return ForecastExperimentReport{}, errors.New("at least one model factory is required")
	}

	for horizon, dataset := range datasets {
		if !isSupportedHorizon(horizon) {
			return ForecastExperimentReport{}, fmt.Errorf("unsupported dataset horizon: %d", horizon)
		}
		if dataset.HorizonSessions != horizon {
			return ForecastExperimentReport{}, fmt.Errorf(
				"dataset map horizon %d does not match dataset horizon %d",
				horizon, dataset.HorizonSessions,
			)
		}
		if len(dataset.Rows) == 0 {
			return ForecastExperimentReport{}, fmt.Errorf("horizon %d has no complete dataset rows", horizon)
		}
		if dataset.IncompleteCount < 0 {
			return ForecastExperimentReport{}, fmt.Errorf("horizon %d has a negative incomplete count", horizon)
		}
	}

	for i, factory := range factories {
		hasBaseline := factory.NewBaseline != nil
		hasFeature := factory.NewFeature != nil
		if hasBaseline == hasFeature {
			return ForecastExperimentReport{}, fmt.Errorf(
				"model factory %d must define exactly one constructor", i,
			)
		}
	}

	report := ForecastExperimentReport{
		SchemaVersion: "1.0",
		GeneratedAt:   time.Now().UTC(),
		Configuration: config,
		Horizons:      make([]ExperimentHorizonResult, 0, len(datasets)),
	}

	// Use canonical order so serialized reports are deterministic by horizon.
	for _, horizon := range SupportedHorizons() {
		dataset, exists := datasets[horizon]
		if !exists {
			continue
		}

		baselineModels := make([]BaselineModel, 0, len(factories))
		featureModels := make([]FeatureForecastModel, 0, len(factories))

		for i, factory := range factories {
			if factory.NewBaseline != nil {
				model := factory.NewBaseline()
				if isNilForecastModel(model) {
					return ForecastExperimentReport{}, fmt.Errorf(
						"baseline factory %d returned nil for horizon %d", i, horizon,
					)
				}
				baselineModels = append(baselineModels, model)
			} else {
				model := factory.NewFeature()
				if isNilForecastModel(model) {
					return ForecastExperimentReport{}, fmt.Errorf(
						"feature factory %d returned nil for horizon %d", i, horizon,
					)
				}
				featureModels = append(featureModels, model)
			}
		}

		comparison, err := CompareForecastModels(
			dataset, config, baselineModels, featureModels,
		)
		if err != nil {
			return ForecastExperimentReport{}, fmt.Errorf(
				"compare models for horizon %d: %w", horizon, err,
			)
		}

		report.Horizons = append(report.Horizons, ExperimentHorizonResult{
			HorizonSessions: horizon,
			FirstDate:       dataset.Rows[0].Date,
			LastDate:        dataset.Rows[len(dataset.Rows)-1].Date,
			RowCount:        len(dataset.Rows),
			IncompleteCount: dataset.IncompleteCount,
			Comparison:      comparison,
		})
	}

	if len(report.Horizons) == 0 {
		return ForecastExperimentReport{}, errors.New("no supported horizon datasets were supplied")
	}

	return report, nil
}

// MarshalForecastExperimentReport serializes a report as indented JSON.
func MarshalForecastExperimentReport(report ForecastExperimentReport) ([]byte, error) {
	if report.SchemaVersion == "" {
		return nil, errors.New("experiment report schema version is required")
	}
	if report.GeneratedAt.IsZero() {
		return nil, errors.New("experiment report generation time is required")
	}
	if len(report.Horizons) == 0 {
		return nil, errors.New("experiment report must contain at least one horizon")
	}

	return json.MarshalIndent(report, "", "  ")
}
