package backtest

import (
	"errors"
	"fmt"
	"math"
	"time"
)

type PortfolioConfig struct {
	InitialCapital float64
	CommissionBPS  float64
	SlippageBPS    float64
}

type Trade struct {
	EntryDate  time.Time
	ExitDate   time.Time
	Quantity   int64
	EntryPrice float64
	ExitPrice  float64
	EntryCost  float64
	ExitCost   float64
	GrossPnL   float64
	NetPnL     float64
	ReturnPct  float64
}

type EquityPoint struct {
	Date   time.Time
	Cash   float64
	Shares int64
	Price  float64
	Equity float64
}

type PortfolioResult struct {
	InitialCapital float64
	FinalCash      float64
	FinalEquity    float64
	OpenShares     int64
	OpenPosition   bool
	Trades         []Trade
	EquityCurve    []EquityPoint
	TotalReturnPct float64
}

type position struct {
	entryDate  time.Time
	quantity   int64
	entryPrice float64
	entryCost  float64
}

func SimulatePortfolio(
	bars []Bar,
	signals []Signal,
	cfg PortfolioConfig,
) (PortfolioResult, error) {
	result := PortfolioResult{
		InitialCapital: cfg.InitialCapital,
		Trades:         make([]Trade, 0),
		EquityCurve:    make([]EquityPoint, 0, len(bars)),
	}

	if cfg.InitialCapital <= 0 ||
		math.IsNaN(cfg.InitialCapital) ||
		math.IsInf(cfg.InitialCapital, 0) {
		return result, errors.New("initial capital must be finite and positive")
	}

	if !validBPS(cfg.CommissionBPS) || !validBPS(cfg.SlippageBPS) {
		return result, errors.New("commission and slippage must be between 0 and 10000 basis points")
	}

	if len(bars) == 0 {
		return result, errors.New("historical bars are required")
	}

	if cfg.CommissionBPS+cfg.SlippageBPS >= 10000 {
		return result, errors.New("combined commission and slippage must be below 10000 basis points")
	}

	for i, bar := range bars {
		if bar.Date.IsZero() ||
			!finitePositive(bar.Open) ||
			!finitePositive(bar.High) ||
			!finitePositive(bar.Low) ||
			!finitePositive(bar.Close) ||
			bar.High < bar.Low ||
			bar.Open < bar.Low || bar.Open > bar.High ||
			bar.Close < bar.Low || bar.Close > bar.High {
			return result, fmt.Errorf("invalid OHLC bar at index %d", i)
		}
		if i > 0 && !bar.Date.After(bars[i-1].Date) {
			return result, errors.New("bar dates must be strictly increasing")
		}
	}

	for i, signal := range signals {
		if signal.Action != "ENTER_LONG" && signal.Action != "EXIT_LONG" {
			return result, fmt.Errorf("invalid signal action at index %d", i)
		}
		if signal.Date.IsZero() || signal.ExecutionDate.IsZero() ||
			!signal.ExecutionDate.After(signal.Date) {
			return result, fmt.Errorf("invalid signal dates at index %d", i)
		}
		if i > 0 && !signal.ExecutionDate.After(signals[i-1].ExecutionDate) {
			return result, errors.New("signal execution dates must be strictly increasing")
		}
	}

	cash := cfg.InitialCapital
	var shares int64
	var openPosition *position
	signalIndex := 0

	commissionRate := cfg.CommissionBPS / 10000
	slippageRate := cfg.SlippageBPS / 10000

	for _, bar := range bars {
		for signalIndex < len(signals) &&
			signals[signalIndex].ExecutionDate.Before(bar.Date) {
			return result, errors.New("signal execution date does not match a supplied bar")
		}

		if signalIndex < len(signals) &&
			signals[signalIndex].ExecutionDate.Equal(bar.Date) {

			signal := signals[signalIndex]
			signalIndex++

			switch signal.Action {
			case "ENTER_LONG":
				if openPosition != nil {
					return result, errors.New("cannot enter while a position is already open")
				}

				executionPrice := bar.Open * (1 + slippageRate)
				unitCost := executionPrice * (1 + commissionRate)

				if !finitePositive(executionPrice) || !finitePositive(unitCost) {
					return result, errors.New("invalid entry execution price")
				}

				rawQuantity := math.Floor(cash / unitCost)
				if math.IsNaN(rawQuantity) || math.IsInf(rawQuantity, 0) ||
					rawQuantity >= 9223372036854775808.0 {
					return result, errors.New("share quantity exceeds supported integer range")
				}

				quantity := int64(rawQuantity)
				if quantity <= 0 {
					break
				}

				notional := float64(quantity) * executionPrice
				commission := notional * commissionRate
				totalCost := notional + commission

				if !finitePositive(notional) ||
					!finiteNonNegative(commission) ||
					!finitePositive(totalCost) {
					return result, errors.New("entry transaction calculation overflow")
				}

				if totalCost > cash {
					return result, errors.New("entry cost exceeds available cash")
				}

				cash -= totalCost
				shares = quantity
				openPosition = &position{
					entryDate:  bar.Date,
					quantity:   quantity,
					entryPrice: executionPrice,
					entryCost:  totalCost,
				}

			case "EXIT_LONG":
				if openPosition == nil {
					break
				}

				executionPrice := bar.Open * (1 - slippageRate)
				if !finitePositive(executionPrice) {
					return result, errors.New("invalid exit execution price")
				}

				notional := float64(shares) * executionPrice
				commission := notional * commissionRate
				proceeds := notional - commission

				grossPnL := (executionPrice - openPosition.entryPrice) * float64(shares)
				netPnL := proceeds - openPosition.entryCost
				returnPct := netPnL / openPosition.entryCost * 100

				if !finitePositive(notional) ||
					!finiteNonNegative(commission) ||
					!finitePositive(proceeds) ||
					!finiteNumber(grossPnL) ||
					!finiteNumber(netPnL) ||
					!finiteNumber(returnPct) {
					return result, errors.New("exit transaction calculation overflow")
				}

				trade := Trade{
					EntryDate:  openPosition.entryDate,
					ExitDate:   bar.Date,
					Quantity:   shares,
					EntryPrice: openPosition.entryPrice,
					ExitPrice:  executionPrice,
					EntryCost:  openPosition.entryCost,
					ExitCost:   commission,
					GrossPnL:   grossPnL,
					NetPnL:     netPnL,
					ReturnPct:  returnPct,
				}

				result.Trades = append(result.Trades, trade)
				cash += proceeds
				shares = 0
				openPosition = nil
			}
		}

		equity := cash + float64(shares)*bar.Close
		if !finitePositive(equity) {
			return result, errors.New("portfolio equity is invalid")
		}

		result.EquityCurve = append(result.EquityCurve, EquityPoint{
			Date:   bar.Date,
			Cash:   cash,
			Shares: shares,
			Price:  bar.Close,
			Equity: equity,
		})
	}

	if signalIndex < len(signals) {
		return result, errors.New("one or more signals have no matching execution bar")
	}

	result.FinalCash = cash
	result.OpenShares = shares
	result.OpenPosition = openPosition != nil

	result.FinalEquity = cash
	if openPosition != nil {
		lastClose := bars[len(bars)-1].Close
		result.FinalEquity += float64(shares) * lastClose
	}

	result.TotalReturnPct =
		(result.FinalEquity/cfg.InitialCapital - 1) * 100

	if !finitePositive(result.FinalEquity) || !finiteNumber(result.TotalReturnPct) {
		return result, errors.New("final portfolio calculation overflow")
	}

	return result, nil
}

func validBPS(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) &&
		value >= 0 && value < 10000
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func finiteNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func finiteNonNegative(value float64) bool {
	return value >= 0 && finiteNumber(value)
}
