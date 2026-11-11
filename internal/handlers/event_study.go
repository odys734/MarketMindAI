package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"marketmind-ai/internal/eventstudy"
)

type EventStudyHandler struct {
	DB *sql.DB
}

type eventStudyArticle struct {
	NewsID          int64     `json:"news_id"`
	Headline        string    `json:"headline"`
	PublishedAt     time.Time `json:"published_at"`
	Sentiment       string    `json:"sentiment"`
	ImpactDirection string    `json:"impact_direction"`
	EventType       string    `json:"event_type"`
	Confidence      int       `json:"confidence"`
}

type eventStudyItem struct {
	Article eventStudyArticle  `json:"article"`
	Status  string             `json:"status"`
	Error   string             `json:"error,omitempty"`
	Result  *eventstudy.Result `json:"result,omitempty"`
}

func (h *EventStudyHandler) Get(c *gin.Context) {
	companyID, ok := parseID(c)
	if !ok {
		return
	}

	indexID, err := strconv.ParseInt(c.Query("index_id"), 10, 64)
	if err != nil || indexID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A valid index_id is required"})
		return
	}

	from, err := time.Parse("2006-01-02", c.Query("from"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid from date is required (YYYY-MM-DD)"})
		return
	}

	to, err := time.Parse("2006-01-02", c.Query("to"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid to date is required (YYYY-MM-DD)"})
		return
	}

	if from.After(to) || to.Sub(from) > 5*365*24*time.Hour {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event date range; maximum range is five years"})
		return
	}

	pre, err := parseEventWindow(c.Query("pre"), 5)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pre must be between 0 and 30"})
		return
	}

	post, err := parseEventWindow(c.Query("post"), 5)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "post must be between 0 and 30"})
		return
	}

	stockProvider := c.Query("provider")
	indexProvider := c.Query("index_provider")

	if len(stockProvider) == 0 || len(stockProvider) > 80 ||
		len(indexProvider) == 0 || len(indexProvider) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "provider and index_provider are required and must not exceed 80 characters",
		})
		return
	}

	ctx := c.Request.Context()

	var company, symbol, exchange string
	err = h.DB.QueryRowContext(ctx, `
		SELECT name, symbol, exchange
		FROM companies
		WHERE id = $1
	`, companyID).Scan(&company, &symbol, &exchange)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load company"})
		return
	}

	var indexName, indexSymbol, indexExchange string
	err = h.DB.QueryRowContext(ctx, `
		SELECT name, symbol, exchange
		FROM market_indices
		WHERE id = $1 AND active = TRUE
	`, indexID).Scan(&indexName, &indexSymbol, &indexExchange)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Active benchmark index not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load benchmark index"})
		return
	}

	events, err := h.loadEvents(ctx, companyID, from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load historical news events"})
		return
	}

	if len(events) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"company": company,
			"symbol":  symbol,
			"benchmark": gin.H{
				"name": indexName, "symbol": indexSymbol, "exchange": indexExchange,
			},
			"count":   0,
			"events":  []eventStudyItem{},
			"message": "No AI-analyzed news with verified publication timestamps was found in this date range.",
		})
		return
	}

	// Load up to 31 trading sessions before and after the requested range.
	// Keep all available observations inside the requested range.
	stockBars, err := h.loadStockBars(ctx, companyID, stockProvider, from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load stock history for the selected provider"})
		return
	}

	benchmarkBars, err := h.loadBenchmarkBars(ctx, indexID, indexProvider, from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load benchmark history for the selected provider"})
		return
	}

	if len(stockBars) == 0 || len(benchmarkBars) == 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "Historical stock or benchmark prices are unavailable for the selected providers and date range",
		})
		return
	}

	results := make([]eventStudyItem, 0, len(events))
	successful := 0

	for _, article := range events {
		item := eventStudyItem{
			Article: article,
			Status:  "insufficient_data",
		}

		calculated, calcErr := eventstudy.Calculate(
			stockBars,
			benchmarkBars,
			article.PublishedAt,
			eventstudy.Config{
				PreSessions:  pre,
				PostSessions: post,
			},
		)

		if calcErr != nil {
			item.Error = calcErr.Error()
		} else {
			item.Status = "completed"
			item.Result = &calculated
			successful++
		}

		results = append(results, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"company": gin.H{
			"name": company, "symbol": symbol, "exchange": exchange,
		},
		"benchmark": gin.H{
			"name": indexName, "symbol": indexSymbol, "exchange": indexExchange,
			"provider": indexProvider,
		},
		"configuration": gin.H{
			"from":           from.Format("2006-01-02"),
			"to":             to.Format("2006-01-02"),
			"pre_sessions":   pre,
			"post_sessions":  post,
			"stock_provider": stockProvider,
		},
		"count":             len(results),
		"completed":         successful,
		"insufficient_data": len(results) - successful,
		"events":            results,
		"method":            "Market-adjusted event study: daily stock simple returns minus daily benchmark simple returns. CAR is the arithmetic sum of abnormal returns.",
		"data_limitations": []string{
			"Only AI-analyzed news with non-null published_at timestamps is included.",
			"Publication dates are mapped to the first available stock trading session on or after the publication date.",
			"Daily data cannot determine intraday event timing.",
			"Benchmark and stock trading calendars may differ.",
			"Abnormal returns do not establish that an event caused a price movement.",
			"Overlapping event windows are not statistically independent.",
			"This is historical educational research, not investment advice or a prediction.",
		},
	})
}

func parseEventWindow(raw string, defaultValue int) (int, error) {
	if raw == "" {
		return defaultValue, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 || value > 30 {
		return 0, strconv.ErrSyntax
	}
	return value, nil
}

func (h *EventStudyHandler) loadEvents(
	ctx context.Context,
	companyID int64,
	from, to time.Time,
) ([]eventStudyArticle, error) {
	rows, err := h.DB.QueryContext(ctx, `
		SELECT
			na.id,
			na.title,
			na.published_at,
			aa.sentiment,
			aa.impact_direction,
			aa.event_type,
			aa.confidence
		FROM news_articles na
		JOIN company_news cn ON cn.news_id = na.id
		JOIN ai_news_analysis aa
			ON aa.company_id = cn.company_id
			AND aa.news_id = cn.news_id
		WHERE cn.company_id = $1
		  AND na.published_at IS NOT NULL
		  AND na.published_at >= $2
		  AND na.published_at < $3 + INTERVAL '1 day'
		ORDER BY na.published_at ASC, na.id ASC
		LIMIT 500
	`, companyID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]eventStudyArticle, 0)

	for rows.Next() {
		var article eventStudyArticle
		if err := rows.Scan(
			&article.NewsID,
			&article.Headline,
			&article.PublishedAt,
			&article.Sentiment,
			&article.ImpactDirection,
			&article.EventType,
			&article.Confidence,
		); err != nil {
			return nil, err
		}
		events = append(events, article)
	}

	return events, rows.Err()
}

func (h *EventStudyHandler) loadStockBars(
	ctx context.Context,
	companyID int64,
	provider string,
	from, to time.Time,
) ([]eventstudy.PriceBar, error) {
	rows, err := h.DB.QueryContext(ctx, `

WITH before_window AS (
    SELECT trading_date, close::double precision AS close
    FROM market_prices
    WHERE company_id = $1 AND provider = $2 AND trading_date < $3
    ORDER BY trading_date DESC LIMIT 31
),
in_window AS (
    SELECT trading_date, close::double precision AS close
    FROM market_prices
    WHERE company_id = $1 AND provider = $2
      AND trading_date BETWEEN $3 AND $4
    ORDER BY trading_date ASC LIMIT 3000
),
after_window AS (
    SELECT trading_date, close::double precision AS close
    FROM market_prices
    WHERE company_id = $1 AND provider = $2 AND trading_date > $4
    ORDER BY trading_date ASC LIMIT 31
)
SELECT trading_date, close FROM (
    SELECT * FROM before_window
    UNION ALL SELECT * FROM in_window
    UNION ALL SELECT * FROM after_window
) selected_bars
ORDER BY trading_date ASC

        `, companyID, provider, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bars := make([]eventstudy.PriceBar, 0)
	seen := make(map[string]bool)

	for rows.Next() {
		var bar eventstudy.PriceBar
		if err := rows.Scan(&bar.Date, &bar.Close); err != nil {
			return nil, err
		}

		key := bar.Date.Format("2006-01-02")
		if seen[key] {
			return nil, sql.ErrNoRows
		}
		seen[key] = true
		bars = append(bars, bar)
	}

	return bars, rows.Err()
}

func (h *EventStudyHandler) loadBenchmarkBars(
	ctx context.Context,
	indexID int64,
	provider string,
	from, to time.Time,
) ([]eventstudy.PriceBar, error) {
	rows, err := h.DB.QueryContext(ctx, `

WITH before_window AS (
    SELECT trading_date, close::double precision AS close
    FROM index_prices
    WHERE index_id = $1 AND provider = $2 AND trading_date < $3
    ORDER BY trading_date DESC LIMIT 31
),
in_window AS (
    SELECT trading_date, close::double precision AS close
    FROM index_prices
    WHERE index_id = $1 AND provider = $2
      AND trading_date BETWEEN $3 AND $4
    ORDER BY trading_date ASC LIMIT 3000
),
after_window AS (
    SELECT trading_date, close::double precision AS close
    FROM index_prices
    WHERE index_id = $1 AND provider = $2 AND trading_date > $4
    ORDER BY trading_date ASC LIMIT 31
)
SELECT trading_date, close FROM (
    SELECT * FROM before_window
    UNION ALL SELECT * FROM in_window
    UNION ALL SELECT * FROM after_window
) selected_bars
ORDER BY trading_date ASC

        `, indexID, provider, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bars := make([]eventstudy.PriceBar, 0)
	seen := make(map[string]bool)

	for rows.Next() {
		var bar eventstudy.PriceBar
		if err := rows.Scan(&bar.Date, &bar.Close); err != nil {
			return nil, err
		}

		key := bar.Date.Format("2006-01-02")
		if seen[key] {
			return nil, sql.ErrNoRows
		}
		seen[key] = true
		bars = append(bars, bar)
	}

	return bars, rows.Err()
}
