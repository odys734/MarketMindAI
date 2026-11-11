package backtest

import (
	"errors"
	"math"
)

type PerformanceMetrics struct {
	TotalReturnPct        float64 `json:"total_return_pct"`
	CAGR                  float64 `json:"cagr"`
	CAGRAvailable         bool    `json:"cagr_available"`
	MaxDrawdownPct        float64 `json:"max_drawdown_pct"`
	SharpeRatio           float64 `json:"sharpe_ratio"`
	SharpeAvailable       bool    `json:"sharpe_available"`
	WinRatePct            float64 `json:"win_rate_pct"`
	ProfitFactor          float64 `json:"profit_factor"`
	ProfitFactorAvailable bool    `json:"profit_factor_available"`
	ClosedTrades          int     `json:"closed_trades"`
	WinningTrades         int     `json:"winning_trades"`
	LosingTrades          int     `json:"losing_trades"`
}

func CalculateMetrics(
	result PortfolioResult,
	annualRiskFreeRate float64,
) (PerformanceMetrics, error) {
	var metrics PerformanceMetrics

	if !finitePositive(result.InitialCapital) ||
		!finitePositive(result.FinalEquity) {
		return metrics, errors.New("initial capital and final equity must be finite and positive")
	}

	if math.IsNaN(annualRiskFreeRate) ||
		math.IsInf(annualRiskFreeRate, 0) ||
		annualRiskFreeRate <= -1 {
		return metrics, errors.New("annual risk-free rate must be finite and greater than -1")
	}

	if len(result.EquityCurve) == 0 {
		return metrics, errors.New("equity curve is required")
	}

	lastPoint := result.EquityCurve[len(result.EquityCurve)-1]
	tolerance := math.Max(1, math.Abs(result.FinalEquity)) * 1e-9
	if math.Abs(lastPoint.Equity-result.FinalEquity) > tolerance {
		return metrics, errors.New("final equity does not match the equity curve")
	}

	peak := result.InitialCapital
	maxDrawdown := 0.0
	dailyReturns := make([]float64, 0, len(result.EquityCurve))
	previousEquity := result.InitialCapital

	for i, point := range result.EquityCurve {
		if point.Date.IsZero() || !finitePositive(point.Equity) {
			return metrics, errors.New("equity curve contains invalid data")
		}
		if i > 0 && !point.Date.After(result.EquityCurve[i-1].Date) {
			return metrics, errors.New("equity curve dates must be strictly increasing")
		}

		dailyReturn := point.Equity/previousEquity - 1
		if math.IsNaN(dailyReturn) || math.IsInf(dailyReturn, 0) {
			return metrics, errors.New("equity curve contains an invalid return")
		}
		dailyReturns = append(dailyReturns, dailyReturn)
		previousEquity = point.Equity

		if point.Equity > peak {
			peak = point.Equity
		}

		drawdown := (peak - point.Equity) / peak * 100
		if drawdown > maxDrawdown {
			maxDrawdown = drawdown
		}
	}

	returnRatio := result.FinalEquity / result.InitialCapital
	if !finitePositive(returnRatio) {
		return metrics, errors.New("portfolio return ratio overflow")
	}
	metrics.TotalReturnPct = (returnRatio - 1) * 100
	if !finiteNumber(metrics.TotalReturnPct) {
		return metrics, errors.New("total return calculation overflow")
	}
	metrics.MaxDrawdownPct = maxDrawdown

	firstDate := result.EquityCurve[0].Date
	lastDate := result.EquityCurve[len(result.EquityCurve)-1].Date
	elapsedDays := lastDate.Sub(firstDate).Hours() / 24

	if elapsedDays > 0 {
		years := elapsedDays / 365.25
		cagrFactor := math.Pow(returnRatio, 1/years)
		cagr := (cagrFactor - 1) * 100
		if finitePositive(cagrFactor) && finiteNumber(cagr) {
			metrics.CAGR = cagr
			metrics.CAGRAvailable = true
		}
	}

	// Sharpe ratio uses daily returns, a zero-default risk-free rate,
	// sample standard deviation and 252 trading sessions per year.
	if len(dailyReturns) >= 2 {
		dailyRiskFree := math.Pow(1+annualRiskFreeRate, 1.0/252) - 1
		if !finiteNumber(dailyRiskFree) {
			return metrics, errors.New("daily risk-free rate calculation overflow")
		}

		meanExcess := 0.0
		for _, dailyReturn := range dailyReturns {
			meanExcess += dailyReturn - dailyRiskFree
		}
		meanExcess /= float64(len(dailyReturns))
		if !finiteNumber(meanExcess) {
			return metrics, errors.New("Sharpe mean calculation overflow")
		}

		variance := 0.0
		for _, dailyReturn := range dailyReturns {
			diff := (dailyReturn - dailyRiskFree) - meanExcess
			variance += diff * diff
		}
		variance /= float64(len(dailyReturns) - 1)
		if !finiteNumber(variance) || variance < 0 {
			return metrics, errors.New("Sharpe variance calculation overflow")
		}

		stdDev := math.Sqrt(variance)
		if stdDev > 0 {
			sharpe := meanExcess / stdDev * math.Sqrt(252)
			if !finiteNumber(sharpe) {
				return metrics, errors.New("Sharpe ratio calculation overflow")
			}
			metrics.SharpeRatio = sharpe
			metrics.SharpeAvailable = true
		}
	}

	metrics.ClosedTrades = len(result.Trades)

	grossProfits := 0.0
	grossLosses := 0.0

	for _, trade := range result.Trades {
		if math.IsNaN(trade.NetPnL) || math.IsInf(trade.NetPnL, 0) {
			return metrics, errors.New("trade contains invalid net PnL")
		}

		if trade.NetPnL > 0 {
			metrics.WinningTrades++
			grossProfits += trade.NetPnL
		} else if trade.NetPnL < 0 {
			metrics.LosingTrades++
			grossLosses += math.Abs(trade.NetPnL)
		}
	}

	if !finiteNumber(grossProfits) || !finiteNumber(grossLosses) {
		return metrics, errors.New("gross profit/loss accumulation overflow")
	}

	if metrics.ClosedTrades > 0 {
		metrics.WinRatePct =
			float64(metrics.WinningTrades) /
				float64(metrics.ClosedTrades) * 100
	}

	if grossLosses > 0 {
		factor := grossProfits / grossLosses
		if !finiteNumber(factor) {
			return metrics, errors.New("profit factor calculation overflow")
		}
		metrics.ProfitFactor = factor
		metrics.ProfitFactorAvailable = true
	}

	return metrics, nil
}
