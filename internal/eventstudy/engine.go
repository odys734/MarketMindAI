package eventstudy

import (
	"errors"
	"fmt"
	"math"
	"time"
)

type PriceBar struct {
	Date  time.Time
	Close float64
}

type Config struct {
	PreSessions  int
	PostSessions int
}

type DailyObservation struct {
	Date                  time.Time `json:"date"`
	SessionOffset         int       `json:"session_offset"`
	StockReturnPct        float64   `json:"stock_return_pct"`
	BenchmarkReturnPct    float64   `json:"benchmark_return_pct"`
	AbnormalReturnPct     float64   `json:"abnormal_return_pct"`
	CumulativeAbnormalPct float64   `json:"cumulative_abnormal_return_pct"`
}

type Result struct {
	EventDate       time.Time          `json:"event_date"`
	EventSession    time.Time          `json:"event_session"`
	WindowStart     time.Time          `json:"window_start"`
	WindowEnd       time.Time          `json:"window_end"`
	PreSessions     int                `json:"pre_sessions"`
	PostSessions    int                `json:"post_sessions"`
	Observations    []DailyObservation `json:"observations"`
	CAR             float64            `json:"car_pct"`
	AverageAbnormal float64            `json:"average_abnormal_return_pct"`
	Method          string             `json:"method"`
}

func Calculate(
	stock []PriceBar,
	benchmark []PriceBar,
	eventDate time.Time,
	cfg Config,
) (Result, error) {
	var result Result

	if eventDate.IsZero() {
		return result, errors.New("event date is required")
	}
	if cfg.PreSessions < 0 || cfg.PostSessions < 0 ||
		cfg.PreSessions+cfg.PostSessions > 500 {
		return result, errors.New("event window must contain between 0 and 500 sessions")
	}
	if len(stock) == 0 || len(benchmark) == 0 {
		return result, errors.New("stock and benchmark prices are required")
	}

	if err := validateBars(stock, "stock"); err != nil {
		return result, err
	}
	if err := validateBars(benchmark, "benchmark"); err != nil {
		return result, err
	}

	eventIndex := -1
	for i := range stock {
		if !dateOnly(stock[i].Date).Before(dateOnly(eventDate)) {
			eventIndex = i
			break
		}
	}
	if eventIndex < 0 {
		return result, errors.New("no stock trading session exists on or after the event date")
	}

	start := eventIndex - cfg.PreSessions
	end := eventIndex + cfg.PostSessions

	if start <= 0 || end >= len(stock) {
		return result, errors.New("insufficient stock history for the requested event window and return baseline")
	}

	benchmarkByDate := make(map[string]int, len(benchmark))
	for i, bar := range benchmark {
		benchmarkByDate[dateKey(bar.Date)] = i
	}

	observations := make([]DailyObservation, 0, end-start+1)
	cumulative := 0.0

	for i := start; i <= end; i++ {
		currentStock := stock[i]
		previousStock := stock[i-1]

		benchmarkIndex, ok := benchmarkByDate[dateKey(currentStock.Date)]
		if !ok {
			return result, fmt.Errorf(
				"benchmark session missing for stock date %s",
				dateKey(currentStock.Date),
			)
		}

		previousBenchmarkIndex, ok := benchmarkByDate[dateKey(previousStock.Date)]
		if !ok {
			return result, fmt.Errorf(
				"benchmark baseline session missing for stock date %s",
				dateKey(previousStock.Date),
			)
		}

		if previousBenchmarkIndex >= benchmarkIndex {
			return result, errors.New("benchmark dates are not aligned with stock dates")
		}

		currentBenchmark := benchmark[benchmarkIndex]
		previousBenchmark := benchmark[previousBenchmarkIndex]

		stockReturn := (currentStock.Close/previousStock.Close - 1) * 100
		benchmarkReturn := (currentBenchmark.Close/previousBenchmark.Close - 1) * 100
		abnormalReturn := stockReturn - benchmarkReturn

		if !finite(stockReturn) || !finite(benchmarkReturn) ||
			!finite(abnormalReturn) {
			return result, errors.New("daily return calculation overflow")
		}

		cumulative += abnormalReturn
		if !finite(cumulative) {
			return result, errors.New("cumulative abnormal return overflow")
		}

		observations = append(observations, DailyObservation{
			Date:                  currentStock.Date,
			SessionOffset:         i - eventIndex,
			StockReturnPct:        stockReturn,
			BenchmarkReturnPct:    benchmarkReturn,
			AbnormalReturnPct:     abnormalReturn,
			CumulativeAbnormalPct: cumulative,
		})
	}

	result = Result{
		EventDate:       dateOnly(eventDate),
		EventSession:    stock[eventIndex].Date,
		WindowStart:     stock[start].Date,
		WindowEnd:       stock[end].Date,
		PreSessions:     cfg.PreSessions,
		PostSessions:    cfg.PostSessions,
		Observations:    observations,
		CAR:             cumulative,
		AverageAbnormal: cumulative / float64(len(observations)),
		Method:          "Market-adjusted model: stock simple daily return minus benchmark simple daily return; CAR is the arithmetic sum of abnormal returns.",
	}

	return result, nil
}

func validateBars(bars []PriceBar, label string) error {
	for i, bar := range bars {
		if bar.Date.IsZero() || !finitePositive(bar.Close) {
			return fmt.Errorf("%s contains invalid price data at index %d", label, i)
		}
		if i > 0 && !dateOnly(bar.Date).After(dateOnly(bars[i-1].Date)) {
			return fmt.Errorf("%s dates must be strictly increasing by trading day", label)
		}
	}
	return nil
}

func dateOnly(value time.Time) time.Time {
	y, m, d := value.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func dateKey(value time.Time) string {
	return dateOnly(value).Format("2006-01-02")
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func finitePositive(value float64) bool {
	return finite(value) && value > 0
}
