package forecast

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Target represents an observed forward return label.
// It is intended for model training and historical evaluation,
// never as an input feature for the same observation date.
type Target struct {
	Date             time.Time `json:"date"`
	HorizonSessions  int       `json:"horizon_sessions"`
	TargetDate       time.Time `json:"target_date"`
	StartClose       float64   `json:"start_close"`
	TargetClose      float64   `json:"target_close"`
	ForwardReturnPct float64   `json:"forward_return_pct"`
}

// TargetSet contains labels for supported forecast horizons.
type TargetSet struct {
	Date    time.Time       `json:"date"`
	Targets map[int]*Target `json:"targets"`
}

// SupportedHorizons returns the supported session horizons.
func SupportedHorizons() []int {
	return []int{1, 5, 10, 20}
}

// CalculateTargets creates forward-return labels from chronological bars.
//
// A target at index i uses the close at i and the close at i+horizon.
// Incomplete future horizons remain nil.
func CalculateTargets(bars []Bar) ([]TargetSet, error) {
	if len(bars) < 2 {
		return nil, errors.New("at least two historical bars are required")
	}

	for i, bar := range bars {
		if bar.Date.IsZero() {
			return nil, fmt.Errorf("bar %d has an invalid date", i)
		}
		if i > 0 && !bar.Date.After(bars[i-1].Date) {
			return nil, fmt.Errorf("bar dates must be strictly increasing at index %d", i)
		}
		if !finitePositive(bar.Close) {
			return nil, fmt.Errorf("bar %d has an invalid close", i)
		}
	}

	horizons := SupportedHorizons()
	result := make([]TargetSet, len(bars))

	for i, bar := range bars {
		set := TargetSet{
			Date:    bar.Date,
			Targets: make(map[int]*Target, len(horizons)),
		}

		for _, horizon := range horizons {
			targetIndex := i + horizon
			if targetIndex >= len(bars) {
				set.Targets[horizon] = nil
				continue
			}

			targetBar := bars[targetIndex]
			forwardReturn := (targetBar.Close/bar.Close - 1) * 100

			if !finite(forwardReturn) {
				return nil, fmt.Errorf(
					"forward return is not finite at index %d horizon %d",
					i,
					horizon,
				)
			}

			set.Targets[horizon] = &Target{
				Date:             bar.Date,
				HorizonSessions:  horizon,
				TargetDate:       targetBar.Date,
				StartClose:       bar.Close,
				TargetClose:      targetBar.Close,
				ForwardReturnPct: forwardReturn,
			}
		}

		result[i] = set
	}

	return result, nil
}

// ValidateTarget checks the consistency of a forward-return label.
func ValidateTarget(target Target) error {
	if target.Date.IsZero() || target.TargetDate.IsZero() {
		return errors.New("target dates are required")
	}
	if !target.TargetDate.After(target.Date) {
		return errors.New("target date must be after observation date")
	}
	if target.HorizonSessions <= 0 {
		return errors.New("horizon must be positive")
	}
	if !finitePositive(target.StartClose) || !finitePositive(target.TargetClose) {
		return errors.New("target prices must be finite and positive")
	}
	if !finite(target.ForwardReturnPct) {
		return errors.New("forward return must be finite")
	}

	expected := (target.TargetClose/target.StartClose - 1) * 100
	if !finite(expected) || math.Abs(expected-target.ForwardReturnPct) > 1e-9 {
		return errors.New("forward return does not match target prices")
	}

	return nil
}
