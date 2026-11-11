package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"marketmind-ai/internal/marketdata"
)

type MarketDataHandler struct {
	DB     *sql.DB
	Client *marketdata.Client
}

func (h *MarketDataHandler) SyncPrices(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	from := c.Query("from")
	to := c.Query("to")

	fromDate, err := time.Parse("2006-01-02", from)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid from date is required (YYYY-MM-DD)"})
		return
	}

	toDate, err := time.Parse("2006-01-02", to)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid to date is required (YYYY-MM-DD)"})
		return
	}

	if fromDate.After(toDate) || toDate.Sub(fromDate) > 365*24*time.Hour {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date range; maximum range is 365 days"})
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

	exchange = strings.ToLower(strings.TrimSpace(exchange))
	if exchange != "nse" && exchange != "bse" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Only NSE and BSE companies are currently supported",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	status, err := h.Client.GetStatus(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":  "Could not retrieve market data provider status",
			"detail": err.Error(),
		})
		return
	}

	prices, err := h.Client.FetchPrices(
		ctx,
		exchange,
		symbol,
		from,
		to,
	)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":  "Could not fetch historical prices",
			"detail": err.Error(),
		})
		return
	}

	var publishedAt sql.NullTime
	if status.PricesPublishedAt != "" {
		parsed, parseErr := time.Parse(time.RFC3339, status.PricesPublishedAt)
		if parseErr != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error": "Provider returned invalid publication timestamp",
			})
			return
		}
		publishedAt = sql.NullTime{Time: parsed, Valid: true}
	}

	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not start database transaction"})
		return
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO market_prices (
			company_id, symbol, exchange, trading_date,
			open, high, low, close, last, prev_close,
			volume, turnover, trades, provider, source_published_at
		)
		VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8,
			NULLIF($9, '')::numeric,
			NULLIF($10, '')::numeric,
			$11, $12, $13, $14, $15
		)
		ON CONFLICT (company_id, trading_date, provider)
		DO UPDATE SET
			symbol = EXCLUDED.symbol,
			exchange = EXCLUDED.exchange,
			open = EXCLUDED.open,
			high = EXCLUDED.high,
			low = EXCLUDED.low,
			close = EXCLUDED.close,
			last = EXCLUDED.last,
			prev_close = EXCLUDED.prev_close,
			volume = EXCLUDED.volume,
			turnover = EXCLUDED.turnover,
			trades = EXCLUDED.trades,
			source_published_at = EXCLUDED.source_published_at,
			fetched_at = NOW()
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not prepare price database statement"})
		return
	}
	defer stmt.Close()

	insertedOrUpdated := 0

	for _, p := range prices {
		_, err := stmt.ExecContext(
			ctx,
			id,
			strings.ToUpper(symbol),
			strings.ToUpper(exchange),
			p.Date,
			p.Open.String(),
			p.High.String(),
			p.Low.String(),
			p.Close.String(),
			p.Last.String(),
			p.PrevClose.String(),
			p.Volume.String(),
			p.Turnover.String(),
			p.Trades.String(),
			"tejhq",
			publishedAt,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Could not save price for %s", p.Date),
			})
			return
		}

		insertedOrUpdated++
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not commit market data"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":             "Historical market data sync completed",
		"company":             companyName,
		"symbol":              strings.ToUpper(symbol),
		"exchange":            strings.ToUpper(exchange),
		"provider":            "TejHQ",
		"data_type":           "historical_eod",
		"latest_trading_date": status.TradingDate,
		"prices_published_at": status.PricesPublishedAt,
		"requested_from":      from,
		"requested_to":        to,
		"records_saved":       insertedOrUpdated,
		"notice":              "Historical end-of-day data; not live market prices or investment advice.",
	})
}

type HistoricalPrice struct {
	TradingDate    string    `json:"trading_date"`
	Open           string    `json:"open"`
	High           string    `json:"high"`
	Low            string    `json:"low"`
	Close          string    `json:"close"`
	Last           string    `json:"last"`
	PrevClose      string    `json:"prev_close"`
	DailyChangePct string    `json:"daily_change_pct"`
	Volume         int64     `json:"volume"`
	Turnover       string    `json:"turnover"`
	Trades         int64     `json:"trades"`
	Provider       string    `json:"provider"`
	FetchedAt      time.Time `json:"fetched_at"`
}

func (h *MarketDataHandler) GetPrices(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	from := c.Query("from")
	to := c.Query("to")

	if from != "" {
		if _, err := time.Parse("2006-01-02", from); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid from date"})
			return
		}
	}

	if to != "" {
		if _, err := time.Parse("2006-01-02", to); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid to date"})
			return
		}
	}

	if from != "" && to != "" && from > to {
		c.JSON(http.StatusBadRequest, gin.H{"error": "From date cannot be after to date"})
		return
	}

	limit := 100
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 500 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Limit must be between 1 and 500"})
			return
		}
		limit = parsed
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
			COALESCE(last::text, ''),
			COALESCE(prev_close::text, ''),
			COALESCE(
				ROUND(
					((close - prev_close) / NULLIF(prev_close, 0)) * 100,
					2
				)::text,
				''
			),
			volume,
			turnover::text,
			trades,
			provider,
			fetched_at
		FROM market_prices
		WHERE company_id = $1
		  AND ($2 = '' OR trading_date >= $2::date)
		  AND ($3 = '' OR trading_date <= $3::date)
		ORDER BY trading_date DESC
		LIMIT $4
	`, id, from, to, limit)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not fetch historical prices"})
		return
	}
	defer rows.Close()

	prices := make([]HistoricalPrice, 0)

	for rows.Next() {
		var p HistoricalPrice

		err := rows.Scan(
			&p.TradingDate,
			&p.Open,
			&p.High,
			&p.Low,
			&p.Close,
			&p.Last,
			&p.PrevClose,
			&p.DailyChangePct,
			&p.Volume,
			&p.Turnover,
			&p.Trades,
			&p.Provider,
			&p.FetchedAt,
		)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not read price records"})
			return
		}

		prices = append(prices, p)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Price query failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"company":   companyName,
		"symbol":    symbol,
		"exchange":  exchange,
		"data_type": "historical_eod",
		"count":     len(prices),
		"from":      from,
		"to":        to,
		"limit":     limit,
		"prices":    prices,
		"notice":    "Historical end-of-day data, not live market prices or investment advice.",
	})
}
