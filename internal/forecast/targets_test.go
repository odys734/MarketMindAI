package forecast

import (
	"testing"
)

func TestCalculateTargetsForwardReturns(t *testing.T) {
	bars := makeBars(increasingCloses(30))

	targets, err := CalculateTargets(bars)
	if err != nil {
		t.Fatal(err)
	}

	for _, horizon := range SupportedHorizons() {
		target := targets[0].Targets[horizon]
		if target == nil {
			t.Fatalf("expected target for horizon %d", horizon)
		}

		expected := (bars[horizon].Close/bars[0].Close - 1) * 100
		assertNear(t, "forward return", target.ForwardReturnPct, expected, 1e-9)

		if target.HorizonSessions != horizon {
			t.Errorf("wrong horizon: got %d, want %d", target.HorizonSessions, horizon)
		}
		if !target.TargetDate.Equal(bars[horizon].Date) {
			t.Errorf("wrong target date for horizon %d", horizon)
		}
		if err := ValidateTarget(*target); err != nil {
			t.Errorf("valid target rejected: %v", err)
		}
	}
}

func TestCalculateTargetsIncompleteHorizons(t *testing.T) {
	bars := makeBars(increasingCloses(10))

	targets, err := CalculateTargets(bars)
	if err != nil {
		t.Fatal(err)
	}

	last := targets[len(targets)-1]
	for _, horizon := range SupportedHorizons() {
		if last.Targets[horizon] != nil {
			t.Errorf("expected incomplete horizon %d to be nil", horizon)
		}
	}

	first := targets[0]
	if first.Targets[1] == nil || first.Targets[5] == nil {
		t.Fatal("expected complete short-horizon targets")
	}
	if first.Targets[10] != nil || first.Targets[20] != nil {
		t.Fatal("long horizons should be incomplete")
	}
}

func TestCalculateTargetsRejectsInvalidBars(t *testing.T) {
	tests := []struct {
		name string
		bars []Bar
	}{
		{"too few bars", makeBars([]float64{10})},
		{"unordered dates", func() []Bar {
			b := makeBars([]float64{10, 11, 12})
			b[2].Date = b[0].Date
			return b
		}()},
		{"invalid close", func() []Bar {
			b := makeBars([]float64{10, 11, 12})
			b[1].Close = 0
			return b
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := CalculateTargets(tt.bars); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateTargetRejectsInconsistentReturn(t *testing.T) {
	bars := makeBars(increasingCloses(3))
	target := Target{
		Date:             bars[0].Date,
		HorizonSessions:  1,
		TargetDate:       bars[1].Date,
		StartClose:       bars[0].Close,
		TargetClose:      bars[1].Close,
		ForwardReturnPct: 999,
	}

	if err := ValidateTarget(target); err == nil {
		t.Fatal("expected inconsistent forward return to be rejected")
	}
}
