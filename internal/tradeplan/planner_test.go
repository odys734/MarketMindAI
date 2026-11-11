package tradeplan

import (
	"testing"
	"time"
)

func sampleBars() []Bar {
	bars:=make([]Bar,60)
	price:=100.0
	for i:=range bars {
		price*=1.004
		bars[i]=Bar{Date:time.Date(2026,1,1+i,0,0,0,0,time.UTC),Open:price-0.2,High:price+1,Low:price-1,Close:price,Volume:10000+int64(i*20)}
	}
	return bars
}

func TestBuildPlanCalculatesBudgetAndRiskBoundedQuantity(t *testing.T) {
	p,err:=BuildPlan(sampleBars(),Request{Budget:10000,Style:"swing",RiskPercent:1,MaxCandidates:5})
	if err!=nil { t.Fatal(err) }
	if p.Quantity<1 || p.CapitalRequired>10000 || p.EstimatedRisk>p.RiskBudget+0.01 { t.Fatalf("budget/risk constraint failed: %+v",p) }
	if p.Target2<=p.EntryHigh || p.StopLoss>=p.EntryLow { t.Fatalf("invalid levels: %+v",p) }
}

func TestBuildPlanRejectsIntradayWithoutIntradayData(t *testing.T) {
	_,err:=BuildPlan(sampleBars(),Request{Budget:10000,Style:"intraday",RiskPercent:1,MaxCandidates:5})
	if err==nil { t.Fatal("expected intraday to be rejected") }
}

func TestBuildPlanRejectsInsufficientHistory(t *testing.T) {
	_,err:=BuildPlan(sampleBars()[:20],Request{Budget:10000,Style:"swing",RiskPercent:1,MaxCandidates:5})
	if err==nil { t.Fatal("expected insufficient history error") }
}
