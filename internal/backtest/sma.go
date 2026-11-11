package backtest

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Bar is one historical end-of-day OHLC record.
type Bar struct {
	Date  time.Time
	Open  float64
	High  float64
	Low   float64
	Close float64
}

// Config defines a long/flat SMA crossover simulation.
type Config struct {
	FastPeriod int
	SlowPeriod int
}

// Signal is generated using information available at a session close.
// ExecutionDate is the next available session.
type Signal struct {
	Date          time.Time
	ExecutionDate time.Time
	Action        string
	FastSMA       float64
	SlowSMA       float64
}

// Result contains the generated historical signals.
type Result struct {
	Signals []Signal
}

// RunSMACrossover calculates SMA crossover signals without look-ahead bias.
// It does not place real orders or provide investment recommendations.
func RunSMACrossover(bars []Bar, cfg Config) (Result, error) {
	result := Result{
		Signals: make([]Signal, 0),
	}

	if cfg.FastPeriod <= 0 || cfg.SlowPeriod <= 0 {
		return result, errors.New("SMA periods must be positive")
	}
	if cfg.FastPeriod >= cfg.SlowPeriod {
		return result, errors.New("fast SMA period must be smaller than slow SMA period")
	}
	if len(bars) < cfg.SlowPeriod+1 {
		return result, fmt.Errorf(
			"insufficient historical bars: need at least %d, got %d",
			cfg.SlowPeriod+1,
			len(bars),
		)
	}

	for i, bar := range bars {
		if bar.Date.IsZero() {
			return result, fmt.Errorf("bar %d has an invalid date", i)
		}
		if !finiteOHLC(bar.Open) || !finiteOHLC(bar.High) ||
			!finiteOHLC(bar.Low) || !finiteOHLC(bar.Close) {
			return result, fmt.Errorf("bar %d contains a non-positive OHLC value", i)
		}
		if bar.High < bar.Low ||
			bar.Open < bar.Low || bar.Open > bar.High ||
			bar.Close < bar.Low || bar.Close > bar.High {
			return result, fmt.Errorf("bar %d contains invalid OHLC values", i)
		}
		if i > 0 && !bar.Date.After(bars[i-1].Date) {
			return result, fmt.Errorf("bar dates must be strictly increasing: index %d", i)
		}
	}

	previousFast, previousSlow := 0.0, 0.0

	for i := cfg.SlowPeriod - 1; i < len(bars); i++ {
		fast, err := simpleMovingAverage(bars, i, cfg.FastPeriod)
		if err != nil {
			return result, err
		}

		slow, err := simpleMovingAverage(bars, i, cfg.SlowPeriod)
		if err != nil {
			return result, err
		}

		if i > cfg.SlowPeriod-1 {
			action := ""

			if previousFast <= previousSlow && fast > slow {
				action = "ENTER_LONG"
			} else if previousFast >= previousSlow && fast < slow {
				action = "EXIT_LONG"
			}

			// A signal at today's close can only execute at the next
			// available historical session's open.
			if action != "" && i+1 < len(bars) {
				result.Signals = append(result.Signals, Signal{
					Date:          bars[i].Date,
					ExecutionDate: bars[i+1].Date,
					Action:        action,
					FastSMA:       fast,
					SlowSMA:       slow,
				})
			}
		}

		previousFast = fast
		previousSlow = slow
	}

	return result, nil
}

func simpleMovingAverage(bars []Bar, end, period int) (float64, error) {
	if period <= 0 || end < 0 || end >= len(bars) || end-period+1 < 0 {
		return 0, errors.New("invalid SMA calculation range")
	}

	sum := 0.0
	for i := end - period + 1; i <= end; i++ {
		sum += bars[i].Close
	}

	if math.IsNaN(sum) || math.IsInf(sum, 0) {
		return 0, errors.New("SMA calculation overflow")
	}

	average := sum / float64(period)
	if math.IsNaN(average) || math.IsInf(average, 0) {
		return 0, errors.New("SMA result is not finite")
	}

	return average, nil
}

func finiteOHLC(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
