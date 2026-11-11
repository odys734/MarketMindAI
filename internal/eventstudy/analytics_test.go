package eventstudy

import (
	"math"
	"testing"
	"time"
)

func analyticsTestResult(values ...float64) Result {
	observations := make([]DailyObservation, len(values))
	cumulative := 0.0

	for i, value := range values {
		cumulative += value
		observations[i] = DailyObservation{
			Date:                  time.Date(2026, 1, i+1, 0, 0, 0, 0, time.UTC),
			SessionOffset:         i - 1,
			AbnormalReturnPct:     value,
			CumulativeAbnormalPct: cumulative,
		}
	}

	return Result{Observations: observations}
}

func closeEnough(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

func TestCalculateAnalyticsAARAndCAAR(t *testing.T) {
	results := []Result{
		analyticsTestResult(1, 2, 3),
		analyticsTestResult(2, 4, 6),
		analyticsTestResult(3, 6, 9),
	}

	got, err := CalculateAnalytics(results, 0.95)
	if err != nil {
		t.Fatal(err)
	}

	if got.EventCount != 3 || got.DegreesFreedom != 2 || len(got.Sessions) != 3 {
		t.Fatalf("unexpected analytics dimensions: %+v", got)
	}

	middle := got.Sessions[1]
	if middle.SessionOffset != 0 || middle.SampleSize != 3 {
		t.Fatalf("unexpected middle session: %+v", middle)
	}
	if !closeEnough(middle.AAR, 4, 1e-10) {
		t.Fatalf("AAR got %v, want 4", middle.AAR)
	}
	if !closeEnough(middle.AARStdDev, 2, 1e-10) {
		t.Fatalf("AAR standard deviation got %v, want 2", middle.AARStdDev)
	}
	if !closeEnough(middle.CAAR, 6, 1e-10) {
		t.Fatalf("CAAR got %v, want 8", middle.CAAR)
	}
	if middle.AARPValue == nil || middle.AARLowerCI == nil || middle.CAARUpperCI == nil {
		t.Fatal("expected inferential statistics")
	}
	if *middle.AARPValue < 0 || *middle.AARPValue > 1 {
		t.Fatalf("invalid p-value: %v", *middle.AARPValue)
	}
}

func TestStudentTCriticalKnownValue(t *testing.T) {
	got, err := studentTCritical(0.975, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !closeEnough(got, 12.706204736, 1e-7) {
		t.Fatalf("t critical got %.10f, want approximately 12.706204736", got)
	}

	p := twoSidedTPValue(12.706204736, 1)
	if !closeEnough(p, 0.05, 1e-7) {
		t.Fatalf("two-sided p-value got %.10f, want 0.05", p)
	}
}

func TestCalculateAnalyticsZeroVarianceOmitsInference(t *testing.T) {
	results := []Result{
		analyticsTestResult(2, 2, 2),
		analyticsTestResult(2, 2, 2),
	}

	got, err := CalculateAnalytics(results, 0.95)
	if err != nil {
		t.Fatal(err)
	}

	for _, session := range got.Sessions {
		if session.AARStdError != 0 || session.CAARStdError != 0 {
			t.Fatalf("expected zero standard error: %+v", session)
		}
		if session.AARTStatistic != nil || session.AARPValue != nil ||
			session.AARLowerCI != nil || session.AARUpperCI != nil {
			t.Fatalf("AAR inference should be omitted for zero variance: %+v", session)
		}
		if session.CAARTStatistic != nil || session.CAARPValue != nil ||
			session.CAARLowerCI != nil || session.CAARUpperCI != nil {
			t.Fatalf("CAAR inference should be omitted for zero variance: %+v", session)
		}
	}
}

func TestCalculateAnalyticsRejectsInvalidInputs(t *testing.T) {
	valid := []Result{
		analyticsTestResult(1, 2, 3),
		analyticsTestResult(2, 3, 4),
	}

	tests := []struct {
		name       string
		results    []Result
		confidence float64
	}{
		{"too few events", valid[:1], 0.95},
		{"confidence too low", valid, 0.5},
		{"confidence too high", valid, 1},
		{"empty event", []Result{{}, valid[1]}, 0.95},
		{"mismatched windows", []Result{
			analyticsTestResult(1, 2, 3),
			analyticsTestResult(1, 2),
		}, 0.95},
		{"non-finite return", []Result{
			analyticsTestResult(1, math.NaN()),
			analyticsTestResult(1, 2),
		}, 0.95},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := CalculateAnalytics(tt.results, tt.confidence); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestCalculateAnalyticsRejectsDuplicateOffsets(t *testing.T) {
	a := Result{Observations: []DailyObservation{
		{SessionOffset: 0, AbnormalReturnPct: 1},
		{SessionOffset: 0, AbnormalReturnPct: 2},
	}}
	b := Result{Observations: []DailyObservation{
		{SessionOffset: 0, AbnormalReturnPct: 1},
	}}

	if _, err := CalculateAnalytics([]Result{a, b}, 0.95); err == nil {
		t.Fatal("expected duplicate offset error")
	}
}
