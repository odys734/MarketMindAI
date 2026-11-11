package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type NewsPriceCorrelationHandler struct {
	DB *sql.DB
}

type NewsPriceObservation struct {
	NewsID          int64  `json:"news_id"`
	Headline        string `json:"headline"`
	ObservedDate    string `json:"observed_date"`
	Sentiment       string `json:"sentiment"`
	ImpactDirection string `json:"impact_direction"`
	Confidence      int    `json:"confidence"`
	Relevance       string `json:"relevance_category"`
	RelevanceScore  int    `json:"relevance_score"`
	PriorSession    string `json:"prior_session"`
	PriorClose      string `json:"prior_close"`
	EventSession    string `json:"event_session"`
	EventClose      string `json:"event_close"`
	ChangePct       string `json:"event_window_change_pct"`
}

func (h *NewsPriceCorrelationHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	limit := 50
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Limit must be between 1 and 100",
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
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not load company",
		})
		return
	}

	rows, err := h.DB.QueryContext(c.Request.Context(), `
		SELECT
			na.id,
			na.title,
			na.published_at::date::text,
			aa.sentiment,
			aa.impact_direction,
			aa.confidence,
			cn.relevance_category,
			cn.relevance_score,

			COALESCE(prior.trading_date::text, ''),
			COALESCE(prior.close::text, ''),

			COALESCE(event_price.trading_date::text, ''),
			COALESCE(event_price.close::text, ''),

			CASE
				WHEN prior.close > 0 AND event_price.close IS NOT NULL
				THEN ROUND(
					((event_price.close - prior.close)
					/ prior.close) * 100,
					2
				)::text
				ELSE ''
			END

		FROM news_articles na

		JOIN company_news cn
			ON cn.news_id = na.id

		JOIN ai_news_analysis aa
			ON aa.company_id = cn.company_id
		   AND aa.news_id = cn.news_id

		LEFT JOIN LATERAL (
			SELECT trading_date, close
			FROM market_prices
			WHERE company_id = cn.company_id
			  AND trading_date < na.published_at::date
			ORDER BY trading_date DESC
			LIMIT 1
		) prior ON TRUE

		LEFT JOIN LATERAL (
			SELECT trading_date, close
			FROM market_prices
			WHERE company_id = cn.company_id
			  AND trading_date >= na.published_at::date
			ORDER BY trading_date ASC
			LIMIT 1
		) event_price ON TRUE

		WHERE cn.company_id = $1
		  AND na.published_at IS NOT NULL

		ORDER BY na.published_at DESC
		LIMIT $2
	`, id, limit)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not calculate news-price observations",
		})
		return
	}
	defer rows.Close()

	observations := make([]NewsPriceObservation, 0)

	for rows.Next() {
		var item NewsPriceObservation

		err := rows.Scan(
			&item.NewsID,
			&item.Headline,
			&item.ObservedDate,
			&item.Sentiment,
			&item.ImpactDirection,
			&item.Confidence,
			&item.Relevance,
			&item.RelevanceScore,
			&item.PriorSession,
			&item.PriorClose,
			&item.EventSession,
			&item.EventClose,
			&item.ChangePct,
		)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Could not read correlation records",
			})
			return
		}

		observations = append(observations, item)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Correlation query failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"company":      company,
		"symbol":       symbol,
		"exchange":     exchange,
		"count":        len(observations),
		"observations": observations,
		"method":       "Compares the previous available trading-session close with the first available close on or after the news observation date.",
		"data_limitations": []string{
			"News dates currently represent GDELT observation dates, not verified publication timestamps.",
			"Observed price movement does not establish that a news article caused the movement.",
			"Missing event-session prices are returned as empty values.",
			"Small samples cannot support reliable statistical correlation.",
		},
	})
}
