// Package risk contains deterministic calculations for hypothetical paper-trading
// scenarios. It does not provide investment advice or place real orders.
package risk

import (
	"errors"
	"math"
)

// SizingInput describes a simulated long-only position. All percentages are
// expressed as percentage points (for example, 0.5 means 0.5%).
type SizingInput struct {
	VirtualBudget       float64
	EntryPrice          float64
	InvalidationPrice   float64
	MaxRiskPercent      float64
	EstimatedCostPct    float64 // estimated round-trip costs as a percentage of notional
	SlippagePct         float64 // estimated adverse slippage per side, in percentage points
	ReserveCashPercent  float64 // budget kept unused
}

// SizingResult is an estimate for a hypothetical paper position.
type SizingResult struct {
	Quantity             int     `json:"quantity"`
	EntryNotional        float64 `json:"entry_notional"`
	EstimatedCosts       float64 `json:"estimated_round_trip_costs"`
	EstimatedRisk        float64 `json:"estimated_risk_to_invalidation"`
	RiskBudget           float64 `json:"risk_budget"`
	AvailableBudget      float64 `json:"available_budget"`
	UnusedBudget         float64 `json:"unused_budget"`
	Feasible             bool    `json:"feasible"`
	Reason               string  `json:"reason"`
}

// CalculateLongOnlyPositionSize calculates a whole-share quantity constrained
// by virtual budget, a maximum loss budget, estimated costs and a cash reserve.
// It assumes a hypothetical long position and a stop/invalidation execution at
// the supplied price; real fills can differ, especially across price gaps.
func CalculateLongOnlyPositionSize(in SizingInput) (SizingResult, error) {
	values := []float64{
		in.VirtualBudget, in.EntryPrice, in.InvalidationPrice,
		in.MaxRiskPercent, in.EstimatedCostPct, in.SlippagePct,
		in.ReserveCashPercent,
	}
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return SizingResult{}, errors.New("all sizing inputs must be finite")
		}
	}
	if in.VirtualBudget <= 0 || in.EntryPrice <= 0 {
		return SizingResult{}, errors.New("virtual budget and entry price must be positive")
	}
	if in.InvalidationPrice <= 0 || in.InvalidationPrice >= in.EntryPrice {
		return SizingResult{}, errors.New("long-only invalidation price must be positive and below entry price")
	}
	if in.MaxRiskPercent <= 0 || in.MaxRiskPercent > 100 {
		return SizingResult{}, errors.New("max risk percent must be greater than 0 and at most 100")
	}
	if in.EstimatedCostPct < 0 || in.EstimatedCostPct > 100 ||
		in.SlippagePct < 0 || in.SlippagePct > 100 {
		return SizingResult{}, errors.New("cost and slippage percentages must be between 0 and 100")
	}
	if in.ReserveCashPercent < 0 || in.ReserveCashPercent >= 100 {
		return SizingResult{}, errors.New("reserve cash percent must be at least 0 and below 100")
	}

	available := in.VirtualBudget * (1 - in.ReserveCashPercent/100)
	riskBudget := in.VirtualBudget * in.MaxRiskPercent / 100
	// Include estimated round-trip costs and adverse entry/exit slippage in
	// the per-share loss estimate rather than sizing from price distance alone.
	costPerShare := in.EntryPrice * in.EstimatedCostPct / 100
	slippagePerShare := in.EntryPrice * in.SlippagePct * 2 / 100
	riskPerShare := in.EntryPrice - in.InvalidationPrice + costPerShare + slippagePerShare
	cashPerShare := in.EntryPrice * (1 + in.EstimatedCostPct/100 + in.SlippagePct/100)
	if !finite(riskPerShare) || !finite(cashPerShare) || !finite(available) || !finite(riskBudget) {
		return SizingResult{}, errors.New("sizing calculation overflowed")
	}

	byRisk := int(math.Floor(riskBudget / riskPerShare))
	byBudget := int(math.Floor(available / cashPerShare))
	quantity := byRisk
	if byBudget < quantity {
		quantity = byBudget
	}
	if quantity < 0 {
		quantity = 0
	}
	notional := float64(quantity) * in.EntryPrice
	costs := float64(quantity) * costPerShare
	estimatedRisk := float64(quantity) * riskPerShare
	unused := in.VirtualBudget - notional - costs
	if unused < 0 && unused > -1e-8 {
		unused = 0
	}
	result := SizingResult{
		Quantity: quantity, EntryNotional: notional, EstimatedCosts: costs,
		EstimatedRisk: estimatedRisk, RiskBudget: riskBudget,
		AvailableBudget: available, UnusedBudget: unused,
		Feasible: quantity > 0,
	}
	if quantity == 0 {
		result.Reason = "no whole-share position fits both the virtual risk limit and available budget"
	} else if byBudget < byRisk {
		result.Reason = "quantity limited by available virtual budget"
	} else {
		result.Reason = "quantity limited by configured virtual risk budget"
	}
	return result, nil
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
