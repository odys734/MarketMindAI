package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type MarketAnalyticsHandler struct {
	DB *sql.DB
}

func (h *MarketAnalyticsHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	days := 30
	if raw := c.Query("days"); raw != "" {
		days, err = strconv.Atoi(raw)
		if err != nil || days < 1 || days > 365 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Days must be between 1 and 365",
			})
			return
		}
	}

	var company, symbol, exchange string

	err = h.DB.QueryRowContext(
		c.Request.Context(),
		`SELECT name, symbol, exchange FROM companies WHERE id = $1`,
		id,
	).Scan(&company, &symbol, &exchange)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load company"})
		return
	}

	var (
		dataPoints      int
		firstDate       string
		latestDate      string
		firstClose      string
		latestClose     string
		totalChange     string
		periodHigh      string
		periodLow       string
		avgDailyVolume  string
		totalVolume     string
		totalTurnover   string
		positiveDays    int
		negativeDays    int
		flatDays        int
		dailyVolatility string
	)

	err = h.DB.QueryRowContext(c.Request.Context(), `
		WITH selected AS (
			SELECT
				trading_date,
				high,
				low,
				close,
				volume,
				turnover,
				COALESCE(
					prev_close,
					LAG(close) OVER (ORDER BY trading_date)
				) AS comparison_close
			FROM market_prices
			WHERE company_id = $1
			  AND trading_date >= CURRENT_DATE - ($2 * INTERVAL '1 day')
		),
		returns AS (
			SELECT *,
				CASE
					WHEN comparison_close > 0
					THEN ((close - comparison_close) / comparison_close) * 100
					ELSE NULL
				END AS daily_return
			FROM selected
		),
		aggregates AS (
			SELECT
				COUNT(*) AS data_points,
				MIN(trading_date) AS first_date,
				MAX(trading_date) AS latest_date,
				(ARRAY_AGG(close ORDER BY trading_date ASC))[1] AS first_close,
				(ARRAY_AGG(close ORDER BY trading_date DESC))[1] AS latest_close,
				MAX(high) AS period_high,
				MIN(low) AS period_low,
				AVG(volume) AS avg_daily_volume,
				SUM(volume) AS total_volume,
				SUM(turnover) AS total_turnover,
				COUNT(*) FILTER (WHERE daily_return > 0) AS positive_days,
				COUNT(*) FILTER (WHERE daily_return < 0) AS negative_days,
				COUNT(*) FILTER (WHERE daily_return = 0) AS flat_days,
				STDDEV_SAMP(daily_return) AS daily_volatility
			FROM returns
		)
		SELECT
			data_points,
			COALESCE(first_date::text, ''),
			COALESCE(latest_date::text, ''),
			COALESCE(first_close::text, ''),
			COALESCE(latest_close::text, ''),
			COALESCE(
				ROUND(
					((latest_close - first_close) / NULLIF(first_close, 0)) * 100,
					2
				)::text, ''
			),
			COALESCE(period_high::text, ''),
			COALESCE(period_low::text, ''),
			COALESCE(ROUND(avg_daily_volume, 2)::text, ''),
			COALESCE(total_volume::text, '0'),
			COALESCE(ROUND(total_turnover, 2)::text, '0'),
			positive_days,
			negative_days,
			flat_days,
			COALESCE(ROUND(daily_volatility, 4)::text, '')
		FROM aggregates
	`, id, days).Scan(
		&dataPoints,
		&firstDate,
		&latestDate,
		&firstClose,
		&latestClose,
		&totalChange,
		&periodHigh,
		&periodLow,
		&avgDailyVolume,
		&totalVolume,
		&totalTurnover,
		&positiveDays,
		&negativeDays,
		&flatDays,
		&dailyVolatility,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not calculate market analytics",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"company":                 company,
		"symbol":                  symbol,
		"exchange":                exchange,
		"period_days":             days,
		"data_points":             dataPoints,
		"first_trading_date":      firstDate,
		"latest_trading_date":     latestDate,
		"first_close":             firstClose,
		"latest_close":            latestClose,
		"total_change_pct":        totalChange,
		"period_high":             periodHigh,
		"period_low":              periodLow,
		"average_daily_volume":    avgDailyVolume,
		"total_volume":            totalVolume,
		"total_turnover":          totalTurnover,
		"positive_days":           positiveDays,
		"negative_days":           negativeDays,
		"flat_days":               flatDays,
		"daily_return_stddev_pct": dailyVolatility,
		"notice":                  "Historical statistical analysis only. Volatility is not a prediction of future prices or investment advice.",
		"generated_at":            time.Now().UTC(),
	})
}
