package handlers

import (
	"database/sql"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"marketmind-ai/internal/tradeplan"
)

type TradePlannerHandler struct { DB *sql.DB }

type TradePlannerRequest struct {
	Budget float64 `json:"budget"`
	Style string `json:"style"`
	RiskPercent float64 `json:"risk_percent"`
	MaxCandidates int `json:"max_candidates"`
	Provider string `json:"provider"`
	Exchange string `json:"exchange"`
}

type plannerCompany struct {
	ID int64
	Name,Symbol,Exchange,Currency string
}

func (h *TradePlannerHandler) Scan(c *gin.Context) {
	var req TradePlannerRequest
	if err:=c.ShouldBindJSON(&req); err!=nil {
		c.JSON(http.StatusBadRequest,gin.H{"error":"Invalid JSON request"}); return
	}
	if math.IsNaN(req.Budget)||math.IsInf(req.Budget,0)||req.Budget<=0||req.Budget>1e12 {
		c.JSON(http.StatusBadRequest,gin.H{"error":"budget must be finite, positive and <= 1e12"}); return
	}
	req.Style=strings.ToLower(strings.TrimSpace(req.Style))
	if req.Style!="swing" {
		c.JSON(http.StatusUnprocessableEntity,gin.H{"error":"only swing research is supported in this first release; intraday needs intraday data and long_term needs longer-horizon fundamental research"}); return
	}
	if req.RiskPercent==0 { req.RiskPercent=1 }
	if req.MaxCandidates==0 { req.MaxCandidates=10 }
	if req.Provider=="" { req.Provider="tejhq" }
	if req.Exchange!="" { req.Exchange=strings.ToUpper(strings.TrimSpace(req.Exchange)) }
	if len(req.Provider)>80 || (req.Exchange!="" && req.Exchange!="NSE" && req.Exchange!="BSE") {
		c.JSON(http.StatusBadRequest,gin.H{"error":"invalid provider or exchange"}); return
	}
	// Validate planner settings before scanning the database.
	if math.IsNaN(req.RiskPercent)||math.IsInf(req.RiskPercent,0)||req.RiskPercent<=0||req.RiskPercent>2||req.MaxCandidates<1||req.MaxCandidates>20 {
		c.JSON(http.StatusBadRequest,gin.H{"error":"risk_percent must be > 0 and <= 2; max_candidates must be 1..20"}); return
	}
	rows,err:=h.DB.QueryContext(c.Request.Context(),`
		SELECT id,name,symbol,exchange,COALESCE(currency,'INR')
		FROM companies
		WHERE UPPER(exchange) IN ('NSE','BSE')
		  AND UPPER(country) IN ('INDIA','IN')
		  AND ($1='' OR UPPER(exchange)=$1)
		ORDER BY symbol
		LIMIT 100
	`,req.Exchange)
	if err!=nil { c.JSON(http.StatusInternalServerError,gin.H{"error":"Could not load eligible company universe"}); return }
	companies:=make([]plannerCompany,0,100)
	for rows.Next() {
		var item plannerCompany
		if err:=rows.Scan(&item.ID,&item.Name,&item.Symbol,&item.Exchange,&item.Currency); err!=nil {
			rows.Close(); c.JSON(http.StatusInternalServerError,gin.H{"error":"Could not read company universe"}); return
		}
		companies=append(companies,item)
	}
	if err:=rows.Err(); err!=nil { rows.Close(); c.JSON(http.StatusInternalServerError,gin.H{"error":"Company universe query failed"}); return }
	rows.Close()

	candidates:=make([]tradeplan.RankedPlan,0)
	skipped:=0
	skipReasons:=make(map[string]int)
	for _,company:=range companies {
		priceRows,queryErr:=h.DB.QueryContext(c.Request.Context(),`
			SELECT trading_date::text,open::float8,high::float8,low::float8,close::float8,volume
			FROM market_prices
			WHERE company_id=$1 AND provider=$2
			ORDER BY trading_date DESC
			LIMIT 60
		`,company.ID,req.Provider)
		if queryErr!=nil { c.JSON(http.StatusInternalServerError,gin.H{"error":"Could not load historical prices"}); return }
		bars:=make([]tradeplan.Bar,0,60)
		for priceRows.Next() {
			var dateText string
			var bar tradeplan.Bar
			if scanErr:=priceRows.Scan(&dateText,&bar.Open,&bar.High,&bar.Low,&bar.Close,&bar.Volume); scanErr!=nil {
				priceRows.Close(); c.JSON(http.StatusInternalServerError,gin.H{"error":"Could not read historical prices"}); return
			}
			parsedDate,dateErr:=time.Parse("2006-01-02",dateText)
			if dateErr!=nil { priceRows.Close(); c.JSON(http.StatusInternalServerError,gin.H{"error":"Invalid stored trading date"}); return }
			bar.Date=parsedDate
			bars=append(bars,bar)
		}
		if err:=priceRows.Err(); err!=nil { priceRows.Close(); c.JSON(http.StatusInternalServerError,gin.H{"error":"Historical price iteration failed"}); return }
		priceRows.Close()
		// SQL returns newest-first; the planner requires chronological bars.
		for left,right:=0,len(bars)-1; left<right; left,right=left+1,right-1 { bars[left],bars[right]=bars[right],bars[left] }
		plan,planErr:=tradeplan.BuildPlan(bars,tradeplan.Request{Budget:req.Budget,Style:req.Style,RiskPercent:req.RiskPercent,MaxCandidates:req.MaxCandidates})
		if planErr!=nil { skipped++; skipReasons[planErr.Error()]++; continue }
		candidates=append(candidates,tradeplan.RankedPlan{CompanyID:company.ID,CompanyName:company.Name,Symbol:company.Symbol,Exchange:company.Exchange,Currency:company.Currency,Plan:plan})
	}
	candidates=tradeplan.Rank(candidates,req.MaxCandidates)
	c.JSON(http.StatusOK,gin.H{
		"mode":"historical_eod_paper_trade_research",
		"configuration":gin.H{"budget":req.Budget,"style":req.Style,"risk_percent":req.RiskPercent,"max_candidates":req.MaxCandidates,"provider":req.Provider,"exchange":req.Exchange},
		"universe":gin.H{"companies_scanned":len(companies),"eligible_candidates":len(candidates),"skipped_or_insufficient_data":skipped,"skip_reasons":skipReasons,"universe_limit":100},
		"candidates":candidates,
		"notice":"Educational paper-trading research only. Prices are the latest stored end-of-day observations, not live quotes. Entry zones, stops, targets and quantities are hypothetical calculations, not buy/sell recommendations. Brokerage, taxes, slippage, corporate actions and fundamentals are not included. No real orders are placed.",
	})
}

