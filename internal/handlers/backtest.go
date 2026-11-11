package handlers

import (
	"database/sql"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"marketmind-ai/internal/backtest"
)

type BacktestHandler struct {
	DB *sql.DB
}

type BacktestRequest struct {
	From               string  `json:"from"`
	To                 string  `json:"to"`
	FastPeriod         int     `json:"fast_period"`
	SlowPeriod         int     `json:"slow_period"`
	InitialCapital     float64 `json:"initial_capital"`
	CommissionBPS      float64 `json:"commission_bps"`
	SlippageBPS        float64 `json:"slippage_bps"`
	AnnualRiskFreeRate float64 `json:"annual_risk_free_rate"`
	Provider           string  `json:"provider"`
}

type BacktestTradeResponse struct {
	EntryDate  time.Time `json:"entry_date"`
	ExitDate   time.Time `json:"exit_date"`
	Quantity   int64     `json:"quantity"`
	EntryPrice float64   `json:"entry_price"`
	ExitPrice  float64   `json:"exit_price"`
	EntryCost  float64   `json:"entry_cost"`
	ExitCost   float64   `json:"exit_cost"`
	GrossPnL   float64   `json:"gross_pnl"`
	NetPnL     float64   `json:"net_pnl"`
	ReturnPct  float64   `json:"return_pct"`
}

type BacktestEquityResponse struct {
	Date   time.Time `json:"date"`
	Cash   float64   `json:"cash"`
	Shares int64     `json:"shares"`
	Price  float64   `json:"price"`
	Equity float64   `json:"equity"`
}

func (h *BacktestHandler) Run(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	var request BacktestRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON request"})
		return
	}

	fromDate, err := time.Parse("2006-01-02", request.From)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid from date is required (YYYY-MM-DD)"})
		return
	}

	toDate, err := time.Parse("2006-01-02", request.To)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid to date is required (YYYY-MM-DD)"})
		return
	}

	if fromDate.After(toDate) || toDate.Sub(fromDate) > 5*365*24*time.Hour {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date range; maximum range is approximately 5 years"})
		return
	}

	if request.FastPeriod < 1 ||
		request.SlowPeriod <= request.FastPeriod ||
		request.SlowPeriod > 500 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "SMA periods must satisfy 1 <= fast_period < slow_period <= 500",
		})
		return
	}

	if math.IsNaN(request.InitialCapital) ||
		math.IsInf(request.InitialCapital, 0) ||
		request.InitialCapital <= 0 ||
		request.InitialCapital > 1e12 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Initial capital must be finite, positive, and no greater than 1 trillion"})
		return
	}

	if math.IsNaN(request.AnnualRiskFreeRate) ||
		math.IsInf(request.AnnualRiskFreeRate, 0) ||
		request.AnnualRiskFreeRate < 0 ||
		request.AnnualRiskFreeRate > 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Annual risk-free rate must be between 0 and 1"})
		return
	}

	request.Provider = strings.TrimSpace(request.Provider)
	if len(request.Provider) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Provider name is too long"})
		return
	}

	var companyName, symbol, exchange string

	err = h.DB.QueryRowContext(
		c.Request.Context(),
		`SELECT name, symbol, exchange FROM companies WHERE id = $1`,
		id,
	).Scan(&companyName, &symbol, &exchange)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load company"})
		return
	}

	rows, err := h.DB.QueryContext(c.Request.Context(), `
		SELECT
			trading_date::text,
			open::text,
			high::text,
			low::text,
			close::text,
			provider
		FROM market_prices
		WHERE company_id = $1
		  AND trading_date >= $2::date
		  AND trading_date <= $3::date
		  AND ($4 = '' OR provider = $4)
		ORDER BY trading_date ASC, provider ASC
		LIMIT 5001
	`, id, request.From, request.To, request.Provider)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load historical prices"})
		return
	}
	defer rows.Close()

	bars := make([]backtest.Bar, 0)
	selectedProvider := ""

	for rows.Next() {
		var dateText, openText, highText, lowText, closeText, provider string

		if err := rows.Scan(
			&dateText,
			&openText,
			&highText,
			&lowText,
			&closeText,
			&provider,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not read historical price records"})
			return
		}

		if len(bars) >= 5000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Backtest is limited to 5000 historical sessions"})
			return
		}

		date, err := time.Parse("2006-01-02", dateText)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid trading date in stored data"})
			return
		}

		if len(bars) > 0 {
			lastDate := bars[len(bars)-1].Date

			if date.Equal(lastDate) {
				c.JSON(http.StatusConflict, gin.H{
					"error": "Multiple provider records exist for the same trading date; select a provider",
				})
				return
			}

			if !date.After(lastDate) {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Historical records are not chronological"})
				return
			}
		}

		if selectedProvider == "" {
			selectedProvider = provider
		} else if selectedProvider != provider {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Historical range contains multiple providers; select one provider for a consistent backtest",
			})
			return
		}

		values := make([]float64, 4)
		for i, raw := range []string{openText, highText, lowText, closeText} {
			values[i], err = strconv.ParseFloat(raw, 64)
			if err != nil || math.IsNaN(values[i]) || math.IsInf(values[i], 0) {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid OHLC value in stored market data"})
				return
			}
		}

		bars = append(bars, backtest.Bar{
			Date:  date,
			Open:  values[0],
			High:  values[1],
			Low:   values[2],
			Close: values[3],
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Historical price query failed"})
		return
	}

	if len(bars) == 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "No historical prices available for the selected date range and provider"})
		return
	}

	strategyResult, err := backtest.RunSMACrossover(bars, backtest.Config{
		FastPeriod: request.FastPeriod,
		SlowPeriod: request.SlowPeriod,
	})
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	portfolioResult, err := backtest.SimulatePortfolio(
		bars,
		strategyResult.Signals,
		backtest.PortfolioConfig{
			InitialCapital: request.InitialCapital,
			CommissionBPS:  request.CommissionBPS,
			SlippageBPS:    request.SlippageBPS,
		},
	)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	metrics, err := backtest.CalculateMetrics(
		portfolioResult,
		request.AnnualRiskFreeRate,
	)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	trades := make([]BacktestTradeResponse, 0, len(portfolioResult.Trades))
	for _, trade := range portfolioResult.Trades {
		trades = append(trades, BacktestTradeResponse{
			EntryDate:  trade.EntryDate,
			ExitDate:   trade.ExitDate,
			Quantity:   trade.Quantity,
			EntryPrice: trade.EntryPrice,
			ExitPrice:  trade.ExitPrice,
			EntryCost:  trade.EntryCost,
			ExitCost:   trade.ExitCost,
			GrossPnL:   trade.GrossPnL,
			NetPnL:     trade.NetPnL,
			ReturnPct:  trade.ReturnPct,
		})
	}

	equityCurve := make([]BacktestEquityResponse, 0, len(portfolioResult.EquityCurve))
	for _, point := range portfolioResult.EquityCurve {
		equityCurve = append(equityCurve, BacktestEquityResponse{
			Date:   point.Date,
			Cash:   point.Cash,
			Shares: point.Shares,
			Price:  point.Price,
			Equity: point.Equity,
		})
	}

	calendarGaps := 0
	for i := 1; i < len(bars); i++ {
		if bars[i].Date.Sub(bars[i-1].Date) > 4*24*time.Hour {
			calendarGaps++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"company":   companyName,
		"symbol":    symbol,
		"exchange":  exchange,
		"provider":  selectedProvider,
		"data_type": "historical_eod_simulation",
		"strategy": gin.H{
			"name":             "SMA crossover",
			"fast_period":      request.FastPeriod,
			"slow_period":      request.SlowPeriod,
			"signal_timing":    "session_close",
			"execution_timing": "next_available_session_open",
		},
		"configuration": gin.H{
			"initial_capital":       request.InitialCapital,
			"commission_bps":        request.CommissionBPS,
			"slippage_bps":          request.SlippageBPS,
			"annual_risk_free_rate": request.AnnualRiskFreeRate,
			"from":                  request.From,
			"to":                    request.To,
		},
		"data_quality": gin.H{
			"historical_sessions":       len(bars),
			"calendar_gaps_over_4_days": calendarGaps,
			"provider_consistent":       true,
		},
		"portfolio": gin.H{
			"final_cash":       portfolioResult.FinalCash,
			"final_equity":     portfolioResult.FinalEquity,
			"open_position":    portfolioResult.OpenPosition,
			"open_shares":      portfolioResult.OpenShares,
			"total_return_pct": portfolioResult.TotalReturnPct,
		},
		"metrics":       metrics,
		"signals_count": len(strategyResult.Signals),
		"trades":        trades,
		"equity_curve":  equityCurve,
		"notice":        "Educational historical simulation only. Hypothetical results do not predict future performance and are not investment advice. No real orders are placed.",
	})
}
