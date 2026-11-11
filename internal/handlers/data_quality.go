package handlers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

type DataQualityHandler struct {
	DB *sql.DB
}

type QualityRecord struct {
	Type                  string  `json:"type"`
	Name                  string  `json:"name"`
	Symbol                string  `json:"symbol"`
	Records               int64   `json:"records"`
	FirstDate             *string `json:"first_date"`
	LatestDate            *string `json:"latest_date"`
	Provider              *string `json:"provider"`
	LastFetchedAt         *string `json:"last_fetched_at"`
	LargeMoveFlags        int64   `json:"large_move_flags"`
	LongCalendarGaps      int64   `json:"long_calendar_gaps"`
	MultipleProviderDates int64   `json:"multiple_provider_dates"`
}

func (h *DataQualityHandler) Get(c *gin.Context) {
	rows, err := h.DB.QueryContext(c.Request.Context(), `
		WITH instruments AS (
			SELECT
				'index'::text AS instrument_type,
				m.id AS instrument_id,
				m.name,
				m.symbol
			FROM market_indices m
			WHERE m.active = TRUE

			UNION ALL

			SELECT
				'stock'::text,
				co.id,
				co.name,
				co.symbol
			FROM companies co
		),
		all_prices AS (
			SELECT
				'index'::text AS instrument_type,
				index_id AS instrument_id,
				trading_date,
				close,
				provider,
				fetched_at
			FROM index_prices

			UNION ALL

			SELECT
				'stock'::text,
				company_id,
				trading_date,
				close,
				provider,
				fetched_at
			FROM market_prices
		),
		ranked AS (
			SELECT
				*,
				LAG(close) OVER (
					PARTITION BY instrument_type, instrument_id
					ORDER BY trading_date
				) AS previous_close,
				LAG(trading_date) OVER (
					PARTITION BY instrument_type, instrument_id
					ORDER BY trading_date
				) AS previous_date
			FROM all_prices
		),
		quality AS (
			SELECT
				instrument_type,
				instrument_id,
				COUNT(*) AS records,
				MIN(trading_date)::text AS first_date,
				MAX(trading_date)::text AS latest_date,
				(array_agg(provider ORDER BY fetched_at DESC))[1] AS provider,
				MAX(fetched_at)::text AS last_fetched_at,
				COUNT(*) FILTER (
					WHERE previous_close IS NOT NULL
					AND ABS((close - previous_close) / NULLIF(previous_close, 0)) > 0.10
				) AS large_moves,
				COUNT(*) FILTER (
					WHERE previous_date IS NOT NULL
					AND trading_date - previous_date > 4
				) AS long_gaps,
			(
				SELECT COUNT(*)
				FROM (
					SELECT ap2.trading_date
					FROM all_prices ap2
					WHERE ap2.instrument_type = ranked.instrument_type
					  AND ap2.instrument_id = ranked.instrument_id
					GROUP BY ap2.trading_date
					HAVING COUNT(DISTINCT ap2.provider) > 1
				) multi_provider
			) AS multiple_provider_dates
			FROM ranked
			GROUP BY instrument_type, instrument_id
		)
		SELECT
			i.instrument_type,
			i.name,
			i.symbol,
			COALESCE(q.records, 0),
			q.first_date,
			q.latest_date,
			q.provider,
			q.last_fetched_at,
			COALESCE(q.large_moves, 0),
			COALESCE(q.long_gaps, 0),
			COALESCE(q.multiple_provider_dates, 0)
		FROM instruments i
		LEFT JOIN quality q
			ON q.instrument_type = i.instrument_type
			AND q.instrument_id = i.instrument_id
		ORDER BY i.instrument_type, i.symbol
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not generate data quality report",
		})
		return
	}
	defer rows.Close()

	records := make([]QualityRecord, 0)

	for rows.Next() {
		var item QualityRecord
		var firstDate, latestDate, provider, fetched sql.NullString

		err := rows.Scan(
			&item.Type,
			&item.Name,
			&item.Symbol,
			&item.Records,
			&firstDate,
			&latestDate,
			&provider,
			&fetched,
			&item.LargeMoveFlags,
			&item.LongCalendarGaps,
			&item.MultipleProviderDates,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Could not read data quality report",
			})
			return
		}

		if firstDate.Valid {
			item.FirstDate = &firstDate.String
		}
		if latestDate.Valid {
			item.LatestDate = &latestDate.String
		}
		if provider.Valid {
			item.Provider = &provider.String
		}
		if fetched.Valid {
			item.LastFetchedAt = &fetched.String
		}

		records = append(records, item)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Data quality iteration failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"count":  len(records),
		"data":   records,
		"notice": "Large daily moves require manual review; they are not automatically errors. Calendar gaps are informational and may include weekends or exchange holidays. This report does not certify provider data as official.",
	})
}
