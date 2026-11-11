package forecast

import (
	"errors"
	"fmt"
	"time"
)

// DatasetRow keeps observation-time features separate from its future label.
type DatasetRow struct {
	Date     time.Time `json:"date"`
	Features Features  `json:"features"`
	Target   Target    `json:"target"`
}

// Dataset contains complete training/evaluation rows for one horizon.
type Dataset struct {
	HorizonSessions int          `json:"horizon_sessions"`
	Rows            []DatasetRow `json:"rows"`
	IncompleteCount int          `json:"incomplete_count"`
}

// BuildDataset joins same-date features with forward-return labels.
//
// Incomplete future labels are excluded from Rows and counted separately.
// Features remain strictly observation-time data; Target is kept separate.
func BuildDataset(features []Features, targets []TargetSet, horizon int) (Dataset, error) {
	if !isSupportedHorizon(horizon) {
		return Dataset{}, fmt.Errorf("unsupported forecast horizon: %d", horizon)
	}
	if len(features) == 0 {
		return Dataset{}, errors.New("features are required")
	}
	if len(features) != len(targets) {
		return Dataset{}, errors.New("features and target sets must have equal lengths")
	}

	dataset := Dataset{
		HorizonSessions: horizon,
		Rows:            make([]DatasetRow, 0, len(features)),
	}

	for i, feature := range features {
		if feature.Date.IsZero() {
			return Dataset{}, fmt.Errorf("feature %d has an invalid date", i)
		}
		if i > 0 && !feature.Date.After(features[i-1].Date) {
			return Dataset{}, fmt.Errorf("feature dates are not strictly increasing at index %d", i)
		}
		if !targets[i].Date.Equal(feature.Date) {
			return Dataset{}, fmt.Errorf("feature and target dates do not match at index %d", i)
		}

		target, exists := targets[i].Targets[horizon]
		if !exists {
			return Dataset{}, fmt.Errorf("target horizon %d is missing at index %d", horizon, i)
		}
		if target == nil {
			dataset.IncompleteCount++
			continue
		}

		if target.HorizonSessions != horizon {
			return Dataset{}, fmt.Errorf("wrong target horizon at index %d", i)
		}
		if !target.Date.Equal(feature.Date) {
			return Dataset{}, fmt.Errorf("target observation date mismatch at index %d", i)
		}
		if err := ValidateTarget(*target); err != nil {
			return Dataset{}, fmt.Errorf("invalid target at index %d: %w", i, err)
		}

		dataset.Rows = append(dataset.Rows, DatasetRow{
			Date:     feature.Date,
			Features: feature,
			Target:   *target,
		})
	}

	return dataset, nil
}

func isSupportedHorizon(horizon int) bool {
	for _, supported := range SupportedHorizons() {
		if horizon == supported {
			return true
		}
	}
	return false
}
