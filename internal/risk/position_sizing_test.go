package risk

import (
	"math"
	"testing"
)

func TestCalculateLongOnlyPositionSizeUsesRiskAndBudgetCaps(t *testing.T) {
	got, err := CalculateLongOnlyPositionSize(SizingInput{
		VirtualBudget: 10000, EntryPrice: 100, InvalidationPrice: 95,
		MaxRiskPercent: 1, EstimatedCostPct: 0.2, SlippagePct: 0.1,
		ReserveCashPercent: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Feasible || got.Quantity <= 0 {
		t.Fatalf("expected feasible positive quantity, got %+v", got)
	}
	if got.EstimatedRisk > got.RiskBudget+1e-8 {
		t.Fatalf("risk %.4f exceeds budget %.4f", got.EstimatedRisk, got.RiskBudget)
	}
	if got.EntryNotional+got.EstimatedCosts > got.AvailableBudget+1e-8 {
		t.Fatalf("position exceeds available budget: %+v", got)
	}
}

func TestCalculateLongOnlyPositionSizeReturnsInfeasibleWhenNoShareFits(t *testing.T) {
	got, err := CalculateLongOnlyPositionSize(SizingInput{
		VirtualBudget: 100, EntryPrice: 1000, InvalidationPrice: 990,
		MaxRiskPercent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Feasible || got.Quantity != 0 {
		t.Fatalf("expected infeasible zero quantity, got %+v", got)
	}
}

func TestCalculateLongOnlyPositionSizeRejectsInvalidInputs(t *testing.T) {
	cases := []SizingInput{
		{VirtualBudget: 1000, EntryPrice: 100, InvalidationPrice: 100, MaxRiskPercent: 1},
		{VirtualBudget: 1000, EntryPrice: 100, InvalidationPrice: 90, MaxRiskPercent: 0},
		{VirtualBudget: 1000, EntryPrice: 100, InvalidationPrice: 90, MaxRiskPercent: 1, ReserveCashPercent: 100},
		{VirtualBudget: math.NaN(), EntryPrice: 100, InvalidationPrice: 90, MaxRiskPercent: 1},
	}
	for i, input := range cases {
		if _, err := CalculateLongOnlyPositionSize(input); err == nil {
			t.Errorf("case %d: expected validation error", i)
		}
	}
}
