package handlers

import (
	"database/sql"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type MarketSnapshot struct {
	Name      string    `json:"name"`
	Symbol    string    `json:"symbol"`
	Exchange  string    `json:"exchange"`
	Currency  string    `json:"currency"`
	Date      string    `json:"trading_date"`
	Close     float64   `json:"close"`
	Previous  *float64  `json:"previous_close"`
	ChangePct *float64  `json:"daily_change_pct"`
	Provider  string    `json:"provider"`
	FetchedAt time.Time `json:"fetched_at"`
}

type MarketOverviewHandler struct {
	DB *sql.DB
}

func (h *MarketOverviewHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()

	indices, err := h.DB.QueryContext(ctx, `
		WITH ranked AS (
			SELECT
				m.name,
				m.symbol,
				m.exchange,
				m.currency,
				p.trading_date,
				p.close,
				p.provider,
				p.fetched_at,
				ROW_NUMBER() OVER (
					PARTITION BY p.index_id
					ORDER BY p.trading_date DESC, p.fetched_at DESC
				) AS rn
			FROM market_indices m
			LEFT JOIN index_prices p ON p.index_id = m.id
			WHERE m.active = TRUE
		)
		SELECT
			name,
			symbol,
			exchange,
			currency,
			trading_date::text,
			close::float8,
			provider,
			fetched_at,
			rn
		FROM ranked
		WHERE rn <= 2
		ORDER BY symbol, rn
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load index overview"})
		return
	}

	indexSnapshots := make([]MarketSnapshot, 0)
	indexPrevious := make(map[string]float64)

	for indices.Next() {
		var item MarketSnapshot
		var date, provider sql.NullString
		var close sql.NullFloat64
		var fetched sql.NullTime
		var rank int

		if err := indices.Scan(
			&item.Name,
			&item.Symbol,
			&item.Exchange,
			&item.Currency,
			&date,
			&close,
			&provider,
			&fetched,
			&rank,
		); err != nil {
			indices.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not read index overview"})
			return
		}

		if !date.Valid || !close.Valid {
			continue
		}

		if rank == 1 {
			item.Date = date.String
			item.Close = close.Float64
			item.Provider = provider.String
			if fetched.Valid {
				item.FetchedAt = fetched.Time
			}
			indexSnapshots = append(indexSnapshots, item)
		} else if rank == 2 {
			indexPrevious[item.Symbol] = close.Float64
		}
	}

	if err := indices.Err(); err != nil {
		indices.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Index overview iteration failed"})
		return
	}
	indices.Close()

	for i := range indexSnapshots {
		if previous, ok := indexPrevious[indexSnapshots[i].Symbol]; ok && previous != 0 {
			indexSnapshots[i].Previous = &previous
			change := math.Round(((indexSnapshots[i].Close-previous)/previous)*10000) / 100
			indexSnapshots[i].ChangePct = &change
		}
	}

	companies, err := h.DB.QueryContext(ctx, `
		WITH ranked AS (
			SELECT
				c.name,
				c.symbol,
				c.exchange,
				c.currency,
				p.trading_date,
				p.close,
				p.provider,
				p.fetched_at,
				ROW_NUMBER() OVER (
					PARTITION BY p.company_id
					ORDER BY p.trading_date DESC, p.fetched_at DESC
				) AS rn
			FROM companies c
			LEFT JOIN market_prices p ON p.company_id = c.id
		)
		SELECT
			name,
			symbol,
			exchange,
			currency,
			trading_date::text,
			close::float8,
			provider,
			fetched_at,
			rn
		FROM ranked
		WHERE rn <= 2
		ORDER BY symbol, rn
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load company overview"})
		return
	}

	companySnapshots := make([]MarketSnapshot, 0)
	companyPrevious := make(map[string]float64)

	for companies.Next() {
		var item MarketSnapshot
		var date, provider sql.NullString
		var close sql.NullFloat64
		var fetched sql.NullTime
		var rank int

		if err := companies.Scan(
			&item.Name,
			&item.Symbol,
			&item.Exchange,
			&item.Currency,
			&date,
			&close,
			&provider,
			&fetched,
			&rank,
		); err != nil {
			companies.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not read company overview"})
			return
		}

		if !date.Valid || !close.Valid {
			continue
		}

		if rank == 1 {
			item.Date = date.String
			item.Close = close.Float64
			item.Provider = provider.String
			if fetched.Valid {
				item.FetchedAt = fetched.Time
			}
			companySnapshots = append(companySnapshots, item)
		} else if rank == 2 {
			companyPrevious[item.Symbol] = close.Float64
		}
	}

	if err := companies.Err(); err != nil {
		companies.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Company overview iteration failed"})
		return
	}
	companies.Close()

	for i := range companySnapshots {
		if previous, ok := companyPrevious[companySnapshots[i].Symbol]; ok && previous != 0 {
			companySnapshots[i].Previous = &previous
			change := math.Round(((companySnapshots[i].Close-previous)/previous)*10000) / 100
			companySnapshots[i].ChangePct = &change
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"market_indices": indexSnapshots,
		"companies":      companySnapshots,
		"data_type":      "historical_latest_available",
		"notice":         "Latest stored historical records, not live quotes. Daily changes compare the previous available stored session. Educational research only; not investment advice.",
	})
}
