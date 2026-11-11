package forecast

import (
	"errors"
	"fmt"
	"time"
)

// WalkForwardConfig controls chronological expanding-window validation.
type WalkForwardConfig struct {
	// InitialTrainSize is the number of dataset observations
	// before the first test window.
	InitialTrainSize int

	// MinimumTrainSize is the minimum number of samples remaining
	// after purging overlapping target windows.
	MinimumTrainSize int

	// TestSize is the number of observations in each test window.
	TestSize int

	// StepSize controls how far the next test window advances.
	// Zero defaults to TestSize.
	StepSize int
}

// WalkForwardFold contains one strictly chronological train/test split.
type WalkForwardFold struct {
	Number        int          `json:"number"`
	Train         []DatasetRow `json:"train"`
	Test          []DatasetRow `json:"test"`
	TrainEndDate  time.Time    `json:"train_end_date"`
	TestStartDate time.Time    `json:"test_start_date"`
	TestEndDate   time.Time    `json:"test_end_date"`
	PurgedCount   int          `json:"purged_count"`
}

// BuildWalkForwardFolds creates expanding training windows and
// forward-only test windows. Training rows whose target dates overlap
// or reach the test period are purged.
func BuildWalkForwardFolds(dataset Dataset, config WalkForwardConfig) ([]WalkForwardFold, error) {
	if !isSupportedHorizon(dataset.HorizonSessions) {
		return nil, fmt.Errorf("unsupported dataset horizon: %d", dataset.HorizonSessions)
	}
	if config.InitialTrainSize <= 0 {
		return nil, errors.New("initial train size must be positive")
	}
	if config.MinimumTrainSize <= 0 {
		return nil, errors.New("minimum train size must be positive")
	}
	if config.MinimumTrainSize > config.InitialTrainSize {
		return nil, errors.New("minimum train size cannot exceed initial train size")
	}
	if config.TestSize <= 0 {
		return nil, errors.New("test size must be positive")
	}
	if config.StepSize < 0 {
		return nil, errors.New("step size cannot be negative")
	}

	step := config.StepSize
	if step == 0 {
		step = config.TestSize
	}

	rows := dataset.Rows
	if len(rows) <= config.InitialTrainSize {
		return nil, errors.New("dataset does not contain observations after initial training window")
	}

	for i, row := range rows {
		if row.Date.IsZero() {
			return nil, fmt.Errorf("dataset row %d has invalid date", i)
		}
		if i > 0 && !row.Date.After(rows[i-1].Date) {
			return nil, fmt.Errorf("dataset dates must be strictly increasing at index %d", i)
		}
		if !row.Target.Date.Equal(row.Date) {
			return nil, fmt.Errorf("dataset target date mismatch at index %d", i)
		}
		if row.Target.HorizonSessions != dataset.HorizonSessions {
			return nil, fmt.Errorf("dataset target horizon mismatch at index %d", i)
		}
		if err := ValidateTarget(row.Target); err != nil {
			return nil, fmt.Errorf("invalid dataset target at index %d: %w", i, err)
		}
	}

	folds := make([]WalkForwardFold, 0)
	foldNumber := 1

	for testStart := config.InitialTrainSize; testStart < len(rows); testStart += step {
		testEnd := testStart + config.TestSize
		if testEnd > len(rows) {
			testEnd = len(rows)
		}

		testRows := rows[testStart:testEnd]
		testStartDate := testRows[0].Date

		trainRows := make([]DatasetRow, 0, testStart)
		purged := 0

		for _, candidate := range rows[:testStart] {
			// Strictly purge labels ending on or after the first test date.
			if !candidate.Target.TargetDate.Before(testStartDate) {
				purged++
				continue
			}
			trainRows = append(trainRows, candidate)
		}

		if len(trainRows) < config.MinimumTrainSize {
			return nil, fmt.Errorf(
				"fold %d has %d training rows after purging; minimum is %d",
				foldNumber,
				len(trainRows),
				config.MinimumTrainSize,
			)
		}

		folds = append(folds, WalkForwardFold{
			Number:        foldNumber,
			Train:         trainRows,
			Test:          append([]DatasetRow(nil), testRows...),
			TrainEndDate:  trainRows[len(trainRows)-1].Date,
			TestStartDate: testRows[0].Date,
			TestEndDate:   testRows[len(testRows)-1].Date,
			PurgedCount:   purged,
		})

		foldNumber++
	}

	if len(folds) == 0 {
		return nil, errors.New("no walk-forward folds could be created")
	}

	return folds, nil
}
