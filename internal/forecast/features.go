package forecast

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Bar represents one historical end-of-day OHLCV session.
type Bar struct {
	Date   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
}

// Features contains indicators calculated using data available
// at the close of the corresponding session.
type Features struct {
	Date time.Time `json:"date"`

	Close float64 `json:"close"`

	DailyReturnPct *float64 `json:"daily_return_pct,omitempty"`
	LogReturn      *float64 `json:"log_return,omitempty"`

	SMA5  *float64 `json:"sma_5,omitempty"`
	SMA10 *float64 `json:"sma_10,omitempty"`
	SMA20 *float64 `json:"sma_20,omitempty"`
	SMA50 *float64 `json:"sma_50,omitempty"`

	EMA12 *float64 `json:"ema_12,omitempty"`
	EMA26 *float64 `json:"ema_26,omitempty"`

	RSI14         *float64 `json:"rsi_14,omitempty"`
	MACD          *float64 `json:"macd,omitempty"`
	MACDSignal    *float64 `json:"macd_signal,omitempty"`
	MACDHistogram *float64 `json:"macd_histogram,omitempty"`

	Volatility20AnnualizedPct *float64 `json:"volatility_20_annualized_pct,omitempty"`

	VolumeAverage20 *float64 `json:"volume_average_20,omitempty"`
	VolumeRatio20   *float64 `json:"volume_ratio_20,omitempty"`
}

// CalculateFeatures validates historical bars and calculates
// technical features in chronological order.
//
// No future observations are used. Indicators remain nil until
// sufficient historical observations are available.
func CalculateFeatures(bars []Bar) ([]Features, error) {
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
		if !finitePositive(bar.Open) ||
			!finitePositive(bar.High) ||
			!finitePositive(bar.Low) ||
			!finitePositive(bar.Close) {
			return nil, fmt.Errorf("bar %d contains invalid OHLC values", i)
		}
		if bar.High < bar.Low ||
			bar.Open < bar.Low || bar.Open > bar.High ||
			bar.Close < bar.Low || bar.Close > bar.High {
			return nil, fmt.Errorf("bar %d contains inconsistent OHLC values", i)
		}
		if bar.Volume < 0 {
			return nil, fmt.Errorf("bar %d has negative volume", i)
		}
	}

	n := len(bars)
	closes := make([]float64, n)
	returns := make([]float64, n)

	for i := range bars {
		closes[i] = bars[i].Close
		if i > 0 {
			returns[i] = closes[i]/closes[i-1] - 1
			if !finite(returns[i]) {
				return nil, fmt.Errorf("daily return overflow at index %d", i)
			}
		}
	}

	ema12, ema12Valid := calculateEMA(closes, 12)
	ema26, ema26Valid := calculateEMA(closes, 26)
	rsi14 := calculateRSI(closes, 14)

	macd := make([]float64, n)
	macdValid := make([]bool, n)

	for i := range bars {
		if ema12Valid[i] && ema26Valid[i] {
			macd[i] = ema12[i] - ema26[i]
			if !finite(macd[i]) {
				return nil, fmt.Errorf("MACD calculation is not finite at index %d", i)
			}
			macdValid[i] = true
		}
	}

	macdSignal, macdSignalValid := calculateSignalEMA(macd, macdValid, 9)

	features := make([]Features, n)

	for i, bar := range bars {
		f := Features{
			Date:  bar.Date,
			Close: bar.Close,
		}

		if i > 0 {
			dailyReturn := returns[i] * 100
			logReturn := math.Log(bar.Close / bars[i-1].Close)

			f.DailyReturnPct = floatPtr(dailyReturn)
			f.LogReturn = floatPtr(logReturn)
		}

		setSMA := func(period int, target **float64) error {
			if i+1 < period {
				return nil
			}
			value := simpleAverage(closes[i-period+1 : i+1])
			if !finite(value) {
				return fmt.Errorf("SMA%d calculation is not finite at index %d", period, i)
			}
			*target = floatPtr(value)
			return nil
		}

		if err := setSMA(5, &f.SMA5); err != nil {
			return nil, err
		}
		if err := setSMA(10, &f.SMA10); err != nil {
			return nil, err
		}
		if err := setSMA(20, &f.SMA20); err != nil {
			return nil, err
		}
		if err := setSMA(50, &f.SMA50); err != nil {
			return nil, err
		}

		if ema12Valid[i] {
			f.EMA12 = floatPtr(ema12[i])
		}
		if ema26Valid[i] {
			f.EMA26 = floatPtr(ema26[i])
		}
		if i < len(rsi14) && !math.IsNaN(rsi14[i]) {
			f.RSI14 = floatPtr(rsi14[i])
		}
		if macdValid[i] {
			f.MACD = floatPtr(macd[i])
		}
		if macdSignalValid[i] {
			f.MACDSignal = floatPtr(macdSignal[i])
			histogram := macd[i] - macdSignal[i]
			f.MACDHistogram = floatPtr(histogram)
		}

		// Twenty daily returns require twenty prior-to-current
		// return observations, so the first value appears at index 20.
		if i >= 20 {
			window := returns[i-19 : i+1]
			volatility, err := annualizedVolatility(window)
			if err != nil {
				return nil, fmt.Errorf("volatility at index %d: %w", i, err)
			}
			f.Volatility20AnnualizedPct = floatPtr(volatility)
		}

		if i >= 19 {
			average := 0.0
			for j := i - 19; j <= i; j++ {
				average += float64(bars[j].Volume) / 20
			}
			if !finite(average) {
				return nil, fmt.Errorf("volume average overflow at index %d", i)
			}
			f.VolumeAverage20 = floatPtr(average)

			if average > 0 {
				ratio := float64(bar.Volume) / average
				if !finite(ratio) {
					return nil, fmt.Errorf("volume ratio overflow at index %d", i)
				}
				f.VolumeRatio20 = floatPtr(ratio)
			}
		}

		features[i] = f
	}

	return features, nil
}

func calculateEMA(values []float64, period int) ([]float64, []bool) {
	result := make([]float64, len(values))
	valid := make([]bool, len(values))

	if period <= 0 || len(values) < period {
		return result, valid
	}

	seed := simpleAverage(values[:period])
	result[period-1] = seed
	valid[period-1] = true

	alpha := 2.0 / float64(period+1)

	for i := period; i < len(values); i++ {
		result[i] = alpha*values[i] + (1-alpha)*result[i-1]
		valid[i] = true
	}

	return result, valid
}

func calculateSignalEMA(values []float64, valid []bool, period int) ([]float64, []bool) {
	result := make([]float64, len(values))
	resultValid := make([]bool, len(values))

	if period <= 0 {
		return result, resultValid
	}

	seedValues := make([]float64, 0, period)
	seedEnd := -1

	for i := range values {
		if !valid[i] {
			continue
		}
		seedValues = append(seedValues, values[i])
		if len(seedValues) == period {
			seedEnd = i
			break
		}
	}

	if seedEnd < 0 {
		return result, resultValid
	}

	result[seedEnd] = simpleAverage(seedValues)
	resultValid[seedEnd] = true

	alpha := 2.0 / float64(period+1)

	for i := seedEnd + 1; i < len(values); i++ {
		if !valid[i] {
			continue
		}
		result[i] = alpha*values[i] + (1-alpha)*result[i-1]
		resultValid[i] = true
	}

	return result, resultValid
}

func calculateRSI(closes []float64, period int) []float64 {
	result := make([]float64, len(closes))
	for i := range result {
		result[i] = math.NaN()
	}

	if period <= 0 || len(closes) <= period {
		return result
	}

	divisor := float64(period)
	averageGain, averageLoss := 0.0, 0.0

	// Scale each observation before accumulation to avoid overflow.
	for i := 1; i <= period; i++ {
		change := closes[i] - closes[i-1]
		if change > 0 {
			averageGain += change / divisor
		} else {
			averageLoss += (-change) / divisor
		}
	}

	result[period] = rsiValue(averageGain, averageLoss)

	for i := period + 1; i < len(closes); i++ {
		change := closes[i] - closes[i-1]
		gain, loss := 0.0, 0.0

		if change > 0 {
			gain = change
		} else {
			loss = -change
		}

		weight := float64(period-1) / divisor
		averageGain = averageGain*weight + gain/divisor
		averageLoss = averageLoss*weight + loss/divisor
		result[i] = rsiValue(averageGain, averageLoss)
	}

	return result
}

func rsiValue(averageGain, averageLoss float64) float64 {
	if averageGain == 0 && averageLoss == 0 {
		return 50
	}
	if averageLoss == 0 {
		return 100
	}
	if averageGain == 0 {
		return 0
	}

	scale := math.Max(averageGain, averageLoss)
	normalizedGain := averageGain / scale
	normalizedLoss := averageLoss / scale

	return 100 * normalizedGain / (normalizedGain + normalizedLoss)
}

func annualizedVolatility(returns []float64) (float64, error) {
	if len(returns) < 2 {
		return 0, errors.New("at least two returns are required")
	}

	mean := 0.0
	for _, value := range returns {
		mean += value / float64(len(returns))
	}
	if !finite(mean) {
		return 0, errors.New("mean is not finite")
	}

	variance := 0.0
	for _, value := range returns {
		diff := value - mean
		variance += diff * diff
	}
	variance /= float64(len(returns) - 1)

	if !finite(variance) || variance < 0 {
		return 0, errors.New("variance is invalid")
	}

	result := math.Sqrt(variance*252) * 100
	if !finite(result) {
		return 0, errors.New("annualized volatility is not finite")
	}

	return result, nil
}

func simpleAverage(values []float64) float64 {
	sum := 0.0
	for _, value := range values {
		sum += value / float64(len(values))
	}
	return sum
}

func finitePositive(value float64) bool {
	return value > 0 && finite(value)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func floatPtr(value float64) *float64 {
	return &value
}
