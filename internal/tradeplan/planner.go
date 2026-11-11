package tradeplan

import (
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

type Bar struct {
	Date time.Time
	Open, High, Low, Close float64
	Volume int64
}

type Request struct {
	Budget float64
	Style string
	RiskPercent float64
	MaxCandidates int
}

type Plan struct {
	AsOfDate string `json:"as_of_date"`
	Style string `json:"style"`
	LastClose float64 `json:"last_close"`
	EntryLow float64 `json:"entry_low"`
	EntryHigh float64 `json:"entry_high"`
	StopLoss float64 `json:"stop_loss"`
	Target1 float64 `json:"target_1"`
	Target2 float64 `json:"target_2"`
	Quantity int64 `json:"simulated_quantity"`
	CapitalRequired float64 `json:"capital_required"`
	EstimatedRisk float64 `json:"estimated_price_risk"`
	RiskBudget float64 `json:"risk_budget"`
	RewardRiskRatio float64 `json:"target_2_reward_risk_ratio"`
	RSI14 float64 `json:"rsi_14"`
	EMA5 float64 `json:"ema_5"`
	EMA20 float64 `json:"ema_20"`
	ATR14 float64 `json:"atr_14"`
	AverageVolume20 float64 `json:"average_volume_20"`
	Score float64 `json:"research_score"`
	Conditions []string `json:"entry_conditions"`
	Invalidation string `json:"invalidation_condition"`
	ExitRules []string `json:"exit_rules"`
	Warnings []string `json:"warnings"`
}

func BuildPlan(bars []Bar, req Request) (Plan, error) {
	var out Plan
	if !finitePositive(req.Budget) || req.Budget > 1e12 { return out, errors.New("budget must be finite, positive and <= 1e12") }
	style := strings.ToLower(strings.TrimSpace(req.Style))
	if style != "swing" { return out, errors.New("style must be swing; intraday requires intraday data and long_term requires longer-horizon fundamental research") }
	if !finitePositive(req.RiskPercent) || req.RiskPercent > 2 { return out, errors.New("risk_percent must be > 0 and <= 2") }
	if req.MaxCandidates < 1 || req.MaxCandidates > 20 { return out, errors.New("max_candidates must be between 1 and 20") }
	if len(bars) < 30 { return out, errors.New("at least 30 historical sessions are required") }
	for i,b := range bars {
		if b.Date.IsZero() || !finitePositive(b.Close) || !finitePositive(b.High) || !finitePositive(b.Low) || b.High < b.Low || b.Close < b.Low || b.Close > b.High || b.Volume < 0 { return out, errors.New("invalid OHLCV bar") }
		if i > 0 && !b.Date.After(bars[i-1].Date) { return out, errors.New("bars must be strictly chronological") }
	}
	closes := make([]float64,len(bars)); for i,b := range bars { closes[i]=b.Close }
	ema5 := ema(closes,5); ema20 := ema(closes,20); rsi := rsi14(closes)
	atr := 0.0; start:=len(bars)-14
	for i:=start;i<len(bars);i++ {
		prev:=bars[i-1].Close
		tr:=math.Max(bars[i].High-bars[i].Low,math.Max(math.Abs(bars[i].High-prev),math.Abs(bars[i].Low-prev)))
		atr+=tr
	}
	atr/=14
	avgVol:=0.0; vstart:=len(bars)-20
	for _,b:=range bars[vstart:] { avgVol+=float64(b.Volume) }; avgVol/=20
	last:=bars[len(bars)-1]
	if !finitePositive(atr) || !finitePositive(ema20) || !finitePositive(avgVol) { return out, errors.New("insufficient valid indicator data") }
	// This is a historical EOD research screen, not an executable trade signal.
	// Keep each rejection reason distinct so API clients can explain skipped symbols.
	if last.Close <= ema20 {
		return out, errors.New("latest close is not above EMA20")
	}
	if ema5 <= ema20 {
		return out, errors.New("EMA5 is not above EMA20")
	}
	if rsi < 45 {
		return out, errors.New("RSI14 is below 45")
	}
	if rsi > 70 {
		return out, errors.New("RSI14 is above 70")
	}
	if last.Volume <= 0 {
		return out, errors.New("latest volume is zero")
	}
	entryLow:=last.Close-0.25*atr; entryHigh:=last.Close+0.25*atr
	stop:=last.Close-1.5*atr; riskPerShare:=entryHigh-stop
	if !finitePositive(riskPerShare) { return out, errors.New("could not calculate positive price risk") }
	riskBudget:=req.Budget*req.RiskPercent/100
	qtyRisk:=int64(math.Floor(riskBudget/riskPerShare))
	qtyBudget:=int64(math.Floor(req.Budget*0.98/entryHigh))
	qty:=qtyRisk; if qtyBudget<qty { qty=qtyBudget }
	if qty<1 { return out, errors.New("budget is too small for one simulated share within the risk limit") }
	capital:=float64(qty)*entryHigh
	estimatedRisk:=float64(qty)*riskPerShare
	target1:=entryHigh+riskPerShare
	target2:=entryHigh+2*riskPerShare
	score:=50.0
	score+=math.Min(20,math.Max(0,(ema5/ema20-1)*1000))
	score+=math.Min(15,math.Max(0,(rsi-45)*1.5))
	if float64(last.Volume)>=avgVol { score+=10 }
	out=Plan{
		AsOfDate:last.Date.Format("2006-01-02"),Style:style,LastClose:last.Close,
		EntryLow:entryLow,EntryHigh:entryHigh,StopLoss:stop,Target1:target1,Target2:target2,
		Quantity:qty,CapitalRequired:capital,EstimatedRisk:estimatedRisk,RiskBudget:riskBudget,
		RewardRiskRatio:2,RSI14:rsi,EMA5:ema5,EMA20:ema20,ATR14:atr,AverageVolume20:avgVol,Score:score,
		Conditions:[]string{"Use only after a fresh market-data update; this plan is based on the last stored EOD close.","For a simulated entry, price must be inside the entry zone and remain above the stop-loss level.","Re-check liquidity, corporate actions and market-wide conditions before recording a paper trade."},
		Invalidation:"Do not consider the setup valid if the next observed price opens below the stop-loss or the trend filter fails on a refreshed dataset.",
		ExitRules:[]string{"Exit the simulation at the stop-loss if reached; do not widen the stop to avoid recording a loss.","At target 1, record a partial-exit scenario only in paper trading; recompute remaining risk.","Close the remaining simulated position at target 2, stop-loss, or the configured time exit; record actual simulated slippage and charges."},
		Warnings:[]string{"Historical end-of-day data only; not intraday or live prices.","Research candidate, not a buy/sell recommendation or guarantee.","Long-term mode is technical-only; fundamental valuation and business-quality data are not included.","Taxes, brokerage and slippage are not included in this first version; actual net P&L will differ."},
	}
	return out,nil
}

func ema(values []float64, period int) float64 {
	if len(values)<period { return 0 }
	k:=2/float64(period+1); value:=values[0]
	for _,v:=range values[1:] { value=v*k+value*(1-k) }
	return value
}

func rsi14(values []float64) float64 {
	if len(values)<15 { return 0 }
	gain,loss:=0.0,0.0
	for i:=len(values)-14;i<len(values);i++ { d:=values[i]-values[i-1]; if d>0 {gain+=d} else {loss-=d} }
	if loss==0 { if gain==0{return 50}; return 100 }
	rs:=(gain/14)/(loss/14); return 100-100/(1+rs)
}

func finitePositive(v float64) bool { return v>0 && !math.IsNaN(v) && !math.IsInf(v,0) }
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v,0) }

type RankedPlan struct { CompanyID int64 `json:"company_id"`; CompanyName string `json:"company_name"`; Symbol string `json:"symbol"`; Exchange string `json:"exchange"`; Currency string `json:"currency"`; Plan Plan `json:"plan"` }

func Rank(plans []RankedPlan, limit int) []RankedPlan {
	sort.SliceStable(plans,func(i,j int)bool{return plans[i].Plan.Score>plans[j].Plan.Score})
	if len(plans)>limit { plans=plans[:limit] }
	return plans
}
