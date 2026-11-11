package eventstudy

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

type SessionAnalytics struct {
	SessionOffset int `json:"session_offset"`
	SampleSize    int `json:"sample_size"`

	AAR           float64  `json:"aar_pct"`
	AARStdDev     float64  `json:"aar_std_dev_pct"`
	AARStdError   float64  `json:"aar_std_error_pct"`
	AARTStatistic *float64 `json:"aar_t_statistic,omitempty"`
	AARPValue     *float64 `json:"aar_p_value,omitempty"`
	AARLowerCI    *float64 `json:"aar_ci_lower_pct,omitempty"`
	AARUpperCI    *float64 `json:"aar_ci_upper_pct,omitempty"`

	CAAR           float64  `json:"caar_pct"`
	CAARStdDev     float64  `json:"caar_std_dev_pct"`
	CAARStdError   float64  `json:"caar_std_error_pct"`
	CAARTStatistic *float64 `json:"caar_t_statistic,omitempty"`
	CAARPValue     *float64 `json:"caar_p_value,omitempty"`
	CAARLowerCI    *float64 `json:"caar_ci_lower_pct,omitempty"`
	CAARUpperCI    *float64 `json:"caar_ci_upper_pct,omitempty"`
}

type AnalyticsResult struct {
	EventCount      int                `json:"event_count"`
	ConfidenceLevel float64            `json:"confidence_level"`
	DegreesFreedom  int                `json:"degrees_of_freedom"`
	Sessions        []SessionAnalytics `json:"sessions"`
	Method          string             `json:"method"`
	Warnings        []string           `json:"warnings"`
}

// CalculateAnalytics aggregates event-level abnormal returns by session offset.
// All events must have the same complete event window.
func CalculateAnalytics(results []Result, confidenceLevel float64) (AnalyticsResult, error) {
	var output AnalyticsResult

	if len(results) < 2 {
		return output, errors.New("at least two completed events are required")
	}
	if !finite(confidenceLevel) || confidenceLevel <= 0.5 || confidenceLevel >= 1 {
		return output, errors.New("confidence level must be greater than 0.5 and less than 1")
	}

	offsets := make([]int, 0)
	eventReturns := make([]map[int]float64, len(results))

	for eventIndex, result := range results {
		if len(result.Observations) == 0 {
			return output, fmt.Errorf("event %d has no observations", eventIndex)
		}

		values := make(map[int]float64, len(result.Observations))
		for _, observation := range result.Observations {
			offset := observation.SessionOffset
			value := observation.AbnormalReturnPct

			if !finite(value) {
				return output, fmt.Errorf("event %d has a non-finite abnormal return", eventIndex)
			}
			if _, exists := values[offset]; exists {
				return output, fmt.Errorf("event %d has duplicate session offset %d", eventIndex, offset)
			}
			values[offset] = value
		}

		if eventIndex == 0 {
			for offset := range values {
				offsets = append(offsets, offset)
			}
			sort.Ints(offsets)
		} else if len(values) != len(offsets) {
			return output, errors.New("events must have identical complete event windows")
		}

		eventReturns[eventIndex] = values
	}

	if len(offsets) == 0 {
		return output, errors.New("no event-window sessions available")
	}

	for eventIndex, values := range eventReturns {
		for _, offset := range offsets {
			if _, exists := values[offset]; !exists {
				return output, fmt.Errorf(
					"event %d is missing session offset %d",
					eventIndex, offset,
				)
			}
		}
	}

	alpha := 1 - confidenceLevel
	degreesFreedom := len(results) - 1
	critical, err := studentTCritical(1-alpha/2, degreesFreedom)
	if err != nil {
		return output, err
	}

	cumulativeByEvent := make([]float64, len(results))
	sessions := make([]SessionAnalytics, 0, len(offsets))

	for _, offset := range offsets {
		dailyValues := make([]float64, len(results))

		for i, values := range eventReturns {
			dailyValues[i] = values[offset]
			cumulativeByEvent[i] += values[offset]

			if !finite(cumulativeByEvent[i]) {
				return output, errors.New("event-level cumulative return overflow")
			}
		}

		aar, aarSD, aarSE, err := sampleStatistics(dailyValues)
		if err != nil {
			return output, fmt.Errorf("AAR at offset %d: %w", offset, err)
		}

		caar, caarSD, caarSE, err := sampleStatistics(cumulativeByEvent)
		if err != nil {
			return output, fmt.Errorf("CAAR at offset %d: %w", offset, err)
		}

		session := SessionAnalytics{
			SessionOffset: offset,
			SampleSize:    len(results),
			AAR:           aar,
			AARStdDev:     aarSD,
			AARStdError:   aarSE,
			CAAR:          caar,
			CAARStdDev:    caarSD,
			CAARStdError:  caarSE,
		}

		if len(results) >= 2 {
			if aarSE > 0 {
				aarT := aar / aarSE
				if !finite(aarT) {
					return output, fmt.Errorf("AAR test statistic overflow at offset %d", offset)
				}
				aarP := twoSidedTPValue(aarT, degreesFreedom)
				aarLower := aar - critical*aarSE
				aarUpper := aar + critical*aarSE

				session.AARTStatistic = &aarT
				session.AARPValue = &aarP
				session.AARLowerCI = &aarLower
				session.AARUpperCI = &aarUpper
			}

			if caarSE > 0 {
				caarT := caar / caarSE
				if !finite(caarT) {
					return output, fmt.Errorf("CAAR test statistic overflow at offset %d", offset)
				}
				caarP := twoSidedTPValue(caarT, degreesFreedom)
				caarLower := caar - critical*caarSE
				caarUpper := caar + critical*caarSE

				session.CAARTStatistic = &caarT
				session.CAARPValue = &caarP
				session.CAARLowerCI = &caarLower
				session.CAARUpperCI = &caarUpper
			}
		}

		sessions = append(sessions, session)
	}

	output = AnalyticsResult{
		EventCount:      len(results),
		ConfidenceLevel: confidenceLevel,
		DegreesFreedom:  degreesFreedom,
		Sessions:        sessions,
		Method: "AAR is the cross-sectional mean of daily abnormal returns. " +
			"CAAR is the cross-sectional mean of event-level cumulative abnormal returns. " +
			"Confidence intervals and p-values use two-sided Student's t inference.",
		Warnings: []string{
			"Sessions with zero cross-sectional standard error omit t-statistics, p-values, and confidence intervals because the test is undefined.",
			"Events are treated as independent cross-sectional observations; overlapping event windows may violate this assumption.",
			"Statistical significance does not establish causality or predictive value.",
			"Results depend on the selected benchmark, event dates, and historical data quality.",
			"Multiple session-level tests are not adjusted for multiple comparisons.",
			"These statistics describe historical research and are not investment advice.",
		},
	}

	return output, nil
}

func sampleStatistics(values []float64) (mean, standardDeviation, standardError float64, err error) {
	if len(values) < 2 {
		return 0, 0, 0, errors.New("at least two observations are required")
	}

	for _, value := range values {
		if !finite(value) {
			return 0, 0, 0, errors.New("non-finite observation")
		}
		mean += value / float64(len(values))
	}

	sumSquares := 0.0
	for _, value := range values {
		difference := value - mean
		sumSquares += difference * difference
	}

	variance := sumSquares / float64(len(values)-1)
	if !finite(variance) || variance < 0 {
		return 0, 0, 0, errors.New("invalid sample variance")
	}

	standardDeviation = math.Sqrt(variance)
	standardError = standardDeviation / math.Sqrt(float64(len(values)))

	if !finite(mean) || !finite(standardDeviation) || !finite(standardError) {
		return 0, 0, 0, errors.New("statistics overflow")
	}

	return mean, standardDeviation, standardError, nil
}

func studentTCritical(probability float64, degreesFreedom int) (float64, error) {
	if degreesFreedom < 1 || probability <= 0 || probability >= 1 {
		return 0, errors.New("invalid Student's t parameters")
	}

	low, high := 0.0, 1.0
	for studentTCDF(high, degreesFreedom) < probability {
		high *= 2
		if high > 1e8 {
			return 0, errors.New("could not determine Student's t critical value")
		}
	}

	for i := 0; i < 120; i++ {
		mid := (low + high) / 2
		if studentTCDF(mid, degreesFreedom) < probability {
			low = mid
		} else {
			high = mid
		}
	}

	return (low + high) / 2, nil
}

func twoSidedTPValue(statistic float64, degreesFreedom int) float64 {
	probability := 2 * (1 - studentTCDF(math.Abs(statistic), degreesFreedom))
	return math.Max(0, math.Min(1, probability))
}

func studentTCDF(value float64, degreesFreedom int) float64 {
	if value == 0 {
		return 0.5
	}

	x := float64(degreesFreedom) /
		(float64(degreesFreedom) + value*value)

	beta := regularizedIncompleteBeta(x, float64(degreesFreedom)/2, 0.5)
	if value > 0 {
		return 1 - 0.5*beta
	}
	return 0.5 * beta
}

func regularizedIncompleteBeta(x, a, b float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}

	logBeta, _ := math.Lgamma(a)
	logBetaB, _ := math.Lgamma(b)
	logBetaAB, _ := math.Lgamma(a + b)

	front := math.Exp(
		a*math.Log(x) + b*math.Log1p(-x) +
			logBetaAB - logBeta - logBetaB,
	)

	if x < (a+1)/(a+b+2) {
		return front * betaContinuedFraction(x, a, b) / a
	}

	return 1 - front*betaContinuedFraction(1-x, b, a)/b
}

func betaContinuedFraction(x, a, b float64) float64 {
	const (
		maxIterations = 300
		epsilon       = 3e-14
		minimum       = 1e-300
	)

	qab := a + b
	qap := a + 1
	qam := a - 1

	c := 1.0
	d := 1 - qab*x/qap
	if math.Abs(d) < minimum {
		d = minimum
	}
	d = 1 / d
	h := d

	for m := 1; m <= maxIterations; m++ {
		mf := float64(m)
		m2 := 2 * mf

		aa := mf * (b - mf) * x /
			((qam + m2) * (a + m2))
		d = 1 + aa*d
		if math.Abs(d) < minimum {
			d = minimum
		}
		c = 1 + aa/c
		if math.Abs(c) < minimum {
			c = minimum
		}
		d = 1 / d
		h *= d * c

		aa = -(a + mf) * (qab + mf) * x /
			((a + m2) * (qap + m2))
		d = 1 + aa*d
		if math.Abs(d) < minimum {
			d = minimum
		}
		c = 1 + aa/c
		if math.Abs(c) < minimum {
			c = minimum
		}
		d = 1 / d

		delta := d * c
		h *= delta

		if math.Abs(delta-1) < epsilon {
			break
		}
	}

	return h
}
