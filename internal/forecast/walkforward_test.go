package forecast

import "testing"

func buildTestDataset(t *testing.T, count, horizon int) Dataset {
	t.Helper()

	bars := makeBars(increasingCloses(count))

	features, err := CalculateFeatures(bars)
	if err != nil {
		t.Fatal(err)
	}

	targets, err := CalculateTargets(bars)
	if err != nil {
		t.Fatal(err)
	}

	dataset, err := BuildDataset(features, targets, horizon)
	if err != nil {
		t.Fatal(err)
	}

	return dataset
}

func TestBuildWalkForwardFoldsChronologicalAndPurged(t *testing.T) {
	dataset := buildTestDataset(t, 100, 5)

	folds, err := BuildWalkForwardFolds(dataset, WalkForwardConfig{
		InitialTrainSize: 40,
		MinimumTrainSize: 30,
		TestSize:         10,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(folds) != 6 {
		t.Fatalf("fold count: got %d, want 6", len(folds))
	}

	for i, fold := range folds {
		if fold.Number != i+1 {
			t.Errorf("fold number: got %d, want %d", fold.Number, i+1)
		}
		if len(fold.Train) < 30 {
			t.Errorf("fold %d has insufficient training rows", fold.Number)
		}
		if len(fold.Test) == 0 || len(fold.Test) > 10 {
			t.Errorf("fold %d has invalid test size: %d", fold.Number, len(fold.Test))
		}
		if !fold.TrainEndDate.Before(fold.TestStartDate) {
			t.Errorf("fold %d training dates overlap test dates", fold.Number)
		}
		if !fold.TestStartDate.Equal(fold.Test[0].Date) {
			t.Errorf("fold %d has incorrect test start date", fold.Number)
		}
		if !fold.TestEndDate.Equal(fold.Test[len(fold.Test)-1].Date) {
			t.Errorf("fold %d has incorrect test end date", fold.Number)
		}

		for _, row := range fold.Train {
			if !row.Target.TargetDate.Before(fold.TestStartDate) {
				t.Errorf("fold %d contains a training target overlapping test period", fold.Number)
			}
		}

		for j := 1; j < len(fold.Test); j++ {
			if !fold.Test[j].Date.After(fold.Test[j-1].Date) {
				t.Errorf("fold %d test dates are not chronological", fold.Number)
			}
		}
	}

	if folds[0].PurgedCount != 5 {
		t.Errorf("first fold purged count: got %d, want 5", folds[0].PurgedCount)
	}
}

func TestBuildWalkForwardFoldsRejectsInvalidConfig(t *testing.T) {
	dataset := buildTestDataset(t, 50, 1)

	tests := []struct {
		name   string
		config WalkForwardConfig
	}{
		{"zero initial train", WalkForwardConfig{MinimumTrainSize: 1, TestSize: 5}},
		{"zero minimum train", WalkForwardConfig{InitialTrainSize: 10, TestSize: 5}},
		{"minimum exceeds initial", WalkForwardConfig{InitialTrainSize: 10, MinimumTrainSize: 11, TestSize: 5}},
		{"zero test size", WalkForwardConfig{InitialTrainSize: 10, MinimumTrainSize: 5}},
		{"negative step", WalkForwardConfig{InitialTrainSize: 10, MinimumTrainSize: 5, TestSize: 5, StepSize: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := BuildWalkForwardFolds(dataset, tt.config); err == nil {
				t.Fatal("expected invalid config error")
			}
		})
	}
}

func TestBuildWalkForwardFoldsRejectsInsufficientPurgedTraining(t *testing.T) {
	dataset := buildTestDataset(t, 30, 10)

	_, err := BuildWalkForwardFolds(dataset, WalkForwardConfig{
		InitialTrainSize: 15,
		MinimumTrainSize: 10,
		TestSize:         5,
	})
	if err == nil {
		t.Fatal("expected insufficient training error after purging")
	}
}

func TestBuildWalkForwardFoldsExpandingWindow(t *testing.T) {
	dataset := buildTestDataset(t, 100, 1)

	folds, err := BuildWalkForwardFolds(dataset, WalkForwardConfig{
		InitialTrainSize: 30,
		MinimumTrainSize: 20,
		TestSize:         10,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(folds) < 2 {
		t.Fatal("expected multiple folds")
	}
	if len(folds[1].Train) <= len(folds[0].Train) {
		t.Fatal("expected expanding training window")
	}
}
