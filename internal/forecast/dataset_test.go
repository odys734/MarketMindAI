package forecast

import (
	"testing"
)

func TestBuildDatasetSeparatesHorizons(t *testing.T) {
	bars := makeBars(increasingCloses(30))

	features, err := CalculateFeatures(bars)
	if err != nil {
		t.Fatal(err)
	}

	targets, err := CalculateTargets(bars)
	if err != nil {
		t.Fatal(err)
	}

	shortDataset, err := BuildDataset(features, targets, 1)
	if err != nil {
		t.Fatal(err)
	}
	longDataset, err := BuildDataset(features, targets, 20)
	if err != nil {
		t.Fatal(err)
	}

	if shortDataset.HorizonSessions != 1 || longDataset.HorizonSessions != 20 {
		t.Fatal("dataset horizons were mixed")
	}

	if len(shortDataset.Rows) != len(bars)-1 {
		t.Fatalf("short dataset rows: got %d, want %d", len(shortDataset.Rows), len(bars)-1)
	}
	if len(longDataset.Rows) != len(bars)-20 {
		t.Fatalf("long dataset rows: got %d, want %d", len(longDataset.Rows), len(bars)-20)
	}

	if shortDataset.Rows[0].Target.HorizonSessions != 1 {
		t.Fatal("short dataset contains a wrong horizon")
	}
	if longDataset.Rows[0].Target.HorizonSessions != 20 {
		t.Fatal("long dataset contains a wrong horizon")
	}
}

func TestBuildDatasetCountsIncompleteTargets(t *testing.T) {
	bars := makeBars(increasingCloses(10))

	features, err := CalculateFeatures(bars)
	if err != nil {
		t.Fatal(err)
	}

	targets, err := CalculateTargets(bars)
	if err != nil {
		t.Fatal(err)
	}

	dataset, err := BuildDataset(features, targets, 5)
	if err != nil {
		t.Fatal(err)
	}

	if len(dataset.Rows) != 5 {
		t.Fatalf("rows: got %d, want 5", len(dataset.Rows))
	}
	if dataset.IncompleteCount != 5 {
		t.Fatalf("incomplete count: got %d, want 5", dataset.IncompleteCount)
	}
}

func TestBuildDatasetRejectsMismatchedDates(t *testing.T) {
	bars := makeBars(increasingCloses(10))

	features, err := CalculateFeatures(bars)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := CalculateTargets(bars)
	if err != nil {
		t.Fatal(err)
	}

	targets[2].Date = targets[2].Date.AddDate(0, 0, 1)

	if _, err := BuildDataset(features, targets, 1); err == nil {
		t.Fatal("expected date mismatch to be rejected")
	}
}

func TestBuildDatasetRejectsUnsupportedHorizon(t *testing.T) {
	bars := makeBars(increasingCloses(10))

	features, err := CalculateFeatures(bars)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := CalculateTargets(bars)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := BuildDataset(features, targets, 3); err == nil {
		t.Fatal("expected unsupported horizon to be rejected")
	}
}
