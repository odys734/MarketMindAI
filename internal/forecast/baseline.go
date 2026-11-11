package forecast

import (
	"errors"
	"fmt"
	"time"
)

// BaselineModel is a simple benchmark forecast model.
type BaselineModel interface {
	Name() string
	Fit(training []DatasetRow, horizon int) error
	Predict() (float64, error)
}

// ZeroReturnBaseline predicts zero return for every observation.
type ZeroReturnBaseline struct {
	horizon int
	fitted  bool
}

func (m *ZeroReturnBaseline) Name() string {
	return "zero_return"
}

func (m *ZeroReturnBaseline) Fit(training []DatasetRow, horizon int) error {
	if err := validateBaselineTraining(training, horizon); err != nil {
		return err
	}

	m.horizon = horizon
	m.fitted = true
	return nil
}

func (m *ZeroReturnBaseline) Predict() (float64, error) {
	if !m.fitted {
		return 0, errors.New("zero-return model has not been fitted")
	}
	return 0, nil
}

// HistoricalMeanBaseline predicts the mean forward return
// observed in the training window.
type HistoricalMeanBaseline struct {
	horizon int
	mean    float64
	count   int
	fitted  bool
}

func (m *HistoricalMeanBaseline) Name() string {
	return "historical_mean"
}

func (m *HistoricalMeanBaseline) Fit(training []DatasetRow, horizon int) error {
	if err := validateBaselineTraining(training, horizon); err != nil {
		return err
	}

	mean := 0.0

	for i, row := range training {
		value := row.Target.ForwardReturnPct
		n := float64(i + 1)

		// Weighted update avoids directly summing potentially large values.
		mean = mean*(1-1/n) + value/n

		if !finite(mean) {
			return fmt.Errorf("historical mean became non-finite at row %d", i)
		}
	}

	m.horizon = horizon
	m.mean = mean
	m.count = len(training)
	m.fitted = true
	return nil
}

func (m *HistoricalMeanBaseline) Predict() (float64, error) {
	if !m.fitted || m.count == 0 {
		return 0, errors.New("historical-mean model has not been fitted")
	}
	if !finite(m.mean) {
		return 0, errors.New("historical mean is not finite")
	}
	return m.mean, nil
}

func validateBaselineTraining(training []DatasetRow, horizon int) error {
	if !isSupportedHorizon(horizon) {
		return fmt.Errorf("unsupported baseline horizon: %d", horizon)
	}
	if len(training) == 0 {
		return errors.New("baseline training data is required")
	}

	var previousDate time.Time

	for i, row := range training {
		if row.Date.IsZero() {
			return fmt.Errorf("training row %d has invalid date", i)
		}
		if i > 0 && !row.Date.After(previousDate) {
			return fmt.Errorf("training dates must be strictly increasing at row %d", i)
		}
		if !row.Target.Date.Equal(row.Date) {
			return fmt.Errorf("training target date mismatch at row %d", i)
		}
		if row.Target.HorizonSessions != horizon {
			return fmt.Errorf("training target horizon mismatch at row %d", i)
		}
		if err := ValidateTarget(row.Target); err != nil {
			return fmt.Errorf("invalid training target at row %d: %w", i, err)
		}

		previousDate = row.Date
	}

	return nil
}
