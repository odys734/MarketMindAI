package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"marketmind-ai/internal/marketdata"
)

type IndexDataHandler struct {
	DB     *sql.DB
	Client *marketdata.YahooClient
}

func (h *IndexDataHandler) SyncPrices(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid index ID"})
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

	var name, symbol, providerSymbol, exchange, currency string

	err = h.DB.QueryRowContext(
		c.Request.Context(),
		`SELECT name, symbol, provider_symbol, exchange, currency
		 FROM market_indices
		 WHERE id = $1 AND active = TRUE`,
		id,
	).Scan(&name, &symbol, &providerSymbol, &exchange, &currency)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Active market index not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load market index"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	prices, err := h.Client.FetchIndexPrices(
		ctx,
		providerSymbol,
		from,
		to,
	)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":  "Could not fetch historical index prices",
			"detail": err.Error(),
		})
		return
	}

	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not start database transaction"})
		return
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO index_prices (
			index_id, trading_date,
			open, high, low, close,
			volume, provider
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (index_id, trading_date, provider)
		DO UPDATE SET
			open = EXCLUDED.open,
			high = EXCLUDED.high,
			low = EXCLUDED.low,
			close = EXCLUDED.close,
			volume = EXCLUDED.volume,
			fetched_at = NOW()
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not prepare index price statement"})
		return
	}
	defer stmt.Close()

	saved := 0

	for _, p := range prices {
		var volume any
		if p.Volume != nil {
			volume = *p.Volume
		}

		_, err := stmt.ExecContext(
			ctx,
			id,
			p.Date,
			p.Open,
			p.High,
			p.Low,
			p.Close,
			volume,
			"yahoo_chart",
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Could not save index price for %s", p.Date),
			})
			return
		}

		saved++
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not commit index prices"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Historical index data sync completed",
		"index":           name,
		"symbol":          symbol,
		"provider_symbol": providerSymbol,
		"exchange":        exchange,
		"currency":        currency,
		"provider":        "Yahoo Finance chart endpoint",
		"data_type":       "historical_daily",
		"requested_from":  from,
		"requested_to":    to,
		"records_saved":   saved,
		"notice":          "Historical data only. Yahoo is an unofficial data source; index volume may be unavailable or not comparable to stock volume. Not live market data or investment advice.",
	})
}

type HistoricalIndexPrice struct {
	TradingDate    string    `json:"trading_date"`
	Open           string    `json:"open"`
	High           string    `json:"high"`
	Low            string    `json:"low"`
	Close          string    `json:"close"`
	DailyChangePct string    `json:"daily_change_pct"`
	Volume         *int64    `json:"volume"`
	Provider       string    `json:"provider"`
	FetchedAt      time.Time `json:"fetched_at"`
}

func (h *IndexDataHandler) GetPrices(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid index ID"})
		return
	}

	from := c.Query("from")
	to := c.Query("to")

	var fromDate, toDate any

	if from != "" {
		parsed, parseErr := time.Parse("2006-01-02", from)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid from date; use YYYY-MM-DD"})
			return
		}
		fromDate = parsed
	}

	if to != "" {
		parsed, parseErr := time.Parse("2006-01-02", to)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid to date; use YYYY-MM-DD"})
			return
		}
		toDate = parsed
	}

	if from != "" && to != "" && from > to {
		c.JSON(http.StatusBadRequest, gin.H{"error": "From date cannot be after to date"})
		return
	}

	limit := 100
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 1000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Limit must be between 1 and 1000"})
			return
		}
	}

	var name, symbol, exchange, currency string

	err = h.DB.QueryRowContext(
		c.Request.Context(),
		`SELECT name, symbol, exchange, currency
		 FROM market_indices
		 WHERE id = $1 AND active = TRUE`,
		id,
	).Scan(&name, &symbol, &exchange, &currency)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Active market index not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load market index"})
		return
	}

	rows, err := h.DB.QueryContext(c.Request.Context(), `
		WITH price_history AS (
			SELECT
				trading_date,
				open,
				high,
				low,
				close,
				volume,
				provider,
				fetched_at,
				LAG(close) OVER (ORDER BY trading_date) AS previous_close
			FROM index_prices
			WHERE index_id = $1
		)
		SELECT
			trading_date::text,
			open::text,
			high::text,
			low::text,
			close::text,
			COALESCE(
				ROUND(
					((close - previous_close) / NULLIF(previous_close, 0)) * 100,
					2
				)::text,
				''
			),
			volume,
			provider,
			fetched_at
		FROM price_history
		WHERE ($2::date IS NULL OR trading_date >= $2)
		  AND ($3::date IS NULL OR trading_date <= $3)
		ORDER BY trading_date DESC
		LIMIT $4
	`, id, fromDate, toDate, limit)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not query index prices"})
		return
	}
	defer rows.Close()

	prices := make([]HistoricalIndexPrice, 0)

	for rows.Next() {
		var p HistoricalIndexPrice
		var volume sql.NullInt64

		if err := rows.Scan(
			&p.TradingDate,
			&p.Open,
			&p.High,
			&p.Low,
			&p.Close,
			&p.DailyChangePct,
			&volume,
			&p.Provider,
			&p.FetchedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not read index prices"})
			return
		}

		if volume.Valid {
			v := volume.Int64
			p.Volume = &v
		}

		prices = append(prices, p)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Index price iteration failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"index":     name,
		"symbol":    symbol,
		"exchange":  exchange,
		"currency":  currency,
		"count":     len(prices),
		"from":      from,
		"to":        to,
		"limit":     limit,
		"data":      prices,
		"data_type": "historical_daily",
		"notice":    "Historical data only. Daily change compares consecutive available sessions; it does not establish causation or predict future performance.",
	})
}

func (h *IndexDataHandler) GetAnalytics(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid index ID"})
		return
	}

	days := 30
	if raw := c.Query("days"); raw != "" {
		days, err = strconv.Atoi(raw)
		if err != nil || days < 2 || days > 3650 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Days must be between 2 and 3650",
			})
			return
		}
	}

	var name, symbol, exchange, currency string

	err = h.DB.QueryRowContext(c.Request.Context(), `
		SELECT name, symbol, exchange, currency
		FROM market_indices
		WHERE id = $1 AND active = TRUE
	`, id).Scan(&name, &symbol, &exchange, &currency)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Active market index not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not load market index",
		})
		return
	}

	var (
		count        int64
		firstDate    sql.NullString
		latestDate   sql.NullString
		firstClose   sql.NullString
		latestClose  sql.NullString
		periodReturn sql.NullString
		highestClose sql.NullString
		lowestClose  sql.NullString
		averageClose sql.NullString
		volatility   sql.NullString
		positiveDays int64
		negativeDays int64
		flatDays     int64
	)

	err = h.DB.QueryRowContext(c.Request.Context(), `
		WITH price_window AS (
			SELECT trading_date, close
			FROM index_prices
			WHERE index_id = $1
			ORDER BY trading_date DESC
			LIMIT $2
		),
		returns AS (
			SELECT
				trading_date,
				close,
				LAG(close) OVER (ORDER BY trading_date) AS previous_close
			FROM price_window
		),
		summary AS (
			SELECT
				COUNT(*) AS total,
				MIN(trading_date) AS first_date,
				MAX(trading_date) AS latest_date,
				(array_agg(close ORDER BY trading_date))[1] AS first_close,
				(array_agg(close ORDER BY trading_date DESC))[1] AS latest_close,
				MIN(close) AS lowest_close,
				MAX(close) AS highest_close,
				AVG(close) AS average_close,
				COUNT(*) FILTER (
					WHERE close > previous_close
				) AS positive_days,
				COUNT(*) FILTER (
					WHERE close < previous_close
				) AS negative_days,
				COUNT(*) FILTER (
					WHERE close = previous_close
				) AS flat_days,
				STDDEV_SAMP(
					((close - previous_close) / NULLIF(previous_close, 0)) * 100
				) AS volatility
			FROM returns
		)
		SELECT
			total,
			first_date::text,
			latest_date::text,
			first_close::text,
			latest_close::text,
			ROUND(
				((latest_close - first_close) / NULLIF(first_close, 0)) * 100,
				2
			)::text,
			highest_close::text,
			lowest_close::text,
			ROUND(average_close, 4)::text,
			ROUND(volatility, 4)::text,
			positive_days,
			negative_days,
			flat_days
		FROM summary
	`, id, days).Scan(
		&count,
		&firstDate,
		&latestDate,
		&firstClose,
		&latestClose,
		&periodReturn,
		&highestClose,
		&lowestClose,
		&averageClose,
		&volatility,
		&positiveDays,
		&negativeDays,
		&flatDays,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not calculate index analytics",
		})
		return
	}

	if count < 2 {
		c.JSON(http.StatusOK, gin.H{
			"index":       name,
			"symbol":      symbol,
			"data_points": count,
			"message":     "At least two historical sessions are required for analytics",
			"data_type":   "historical_daily",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"index":                name,
		"symbol":               symbol,
		"exchange":             exchange,
		"currency":             currency,
		"period_days":          days,
		"data_points":          count,
		"first_date":           firstDate.String,
		"latest_date":          latestDate.String,
		"first_close":          firstClose.String,
		"latest_close":         latestClose.String,
		"period_return_pct":    periodReturn.String,
		"highest_close":        highestClose.String,
		"lowest_close":         lowestClose.String,
		"average_close":        averageClose.String,
		"daily_volatility_pct": volatility.String,
		"positive_days":        positiveDays,
		"negative_days":        negativeDays,
		"flat_days":            flatDays,
		"data_type":            "historical_daily",
		"notice":               "Historical descriptive analytics only. Volatility is sample standard deviation of daily close-to-close percentage returns; it is not a forecast or investment recommendation.",
	})
}

type IndexComparisonPoint struct {
	TradingDate string  `json:"trading_date"`
	LeftValue   float64 `json:"left_base_100"`
	RightValue  float64 `json:"right_base_100"`
}

func (h *IndexDataHandler) CompareIndices(c *gin.Context) {
	leftID, err := strconv.ParseInt(c.Query("left_id"), 10, 64)
	if err != nil || leftID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid left_id is required"})
		return
	}

	rightID, err := strconv.ParseInt(c.Query("right_id"), 10, 64)
	if err != nil || rightID <= 0 || rightID == leftID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid, different right_id is required"})
		return
	}

	days := 30
	if raw := c.Query("days"); raw != "" {
		days, err = strconv.Atoi(raw)
		if err != nil || days < 2 || days > 3650 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Days must be between 2 and 3650"})
			return
		}
	}

	var leftName, leftSymbol, rightName, rightSymbol string

	err = h.DB.QueryRowContext(c.Request.Context(), `
		SELECT name, symbol
		FROM market_indices
		WHERE id = $1 AND active = TRUE
	`, leftID).Scan(&leftName, &leftSymbol)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Left index not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load left index"})
		return
	}

	err = h.DB.QueryRowContext(c.Request.Context(), `
		SELECT name, symbol
		FROM market_indices
		WHERE id = $1 AND active = TRUE
	`, rightID).Scan(&rightName, &rightSymbol)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Right index not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load right index"})
		return
	}

	rows, err := h.DB.QueryContext(c.Request.Context(), `
		WITH left_prices AS (
			SELECT DISTINCT ON (trading_date)
				trading_date, close
			FROM index_prices
			WHERE index_id = $1
			ORDER BY trading_date, fetched_at DESC
		),
		right_prices AS (
			SELECT DISTINCT ON (trading_date)
				trading_date, close
			FROM index_prices
			WHERE index_id = $2
			ORDER BY trading_date, fetched_at DESC
		),
		matched AS (
			SELECT
				l.trading_date,
				l.close AS left_close,
				r.close AS right_close
			FROM left_prices l
			INNER JOIN right_prices r
				ON r.trading_date = l.trading_date
		),
		recent AS (
			SELECT *
			FROM matched
			ORDER BY trading_date DESC
			LIMIT $3
		),
		baseline AS (
			SELECT
				(array_agg(left_close ORDER BY trading_date))[1] AS left_base,
				(array_agg(right_close ORDER BY trading_date))[1] AS right_base
			FROM recent
		)
		SELECT
			r.trading_date::text,
			ROUND((r.left_close / NULLIF(b.left_base, 0)) * 100, 4)::text,
			ROUND((r.right_close / NULLIF(b.right_base, 0)) * 100, 4)::text
		FROM recent r
		CROSS JOIN baseline b
		ORDER BY r.trading_date
	`, leftID, rightID, days)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not calculate index comparison"})
		return
	}
	defer rows.Close()

	points := make([]IndexComparisonPoint, 0)

	for rows.Next() {
		var (
			point IndexComparisonPoint
			left  string
			right string
		)

		if err := rows.Scan(&point.TradingDate, &left, &right); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not read comparison data"})
			return
		}

		point.LeftValue, err = strconv.ParseFloat(left, 64)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid left index value"})
			return
		}

		point.RightValue, err = strconv.ParseFloat(right, 64)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid right index value"})
			return
		}

		points = append(points, point)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Comparison iteration failed"})
		return
	}

	if len(points) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message": "No common historical trading dates found",
			"count":   0,
			"data":    points,
		})
		return
	}

	first := points[0]
	last := points[len(points)-1]

	c.JSON(http.StatusOK, gin.H{
		"comparison_type": "base_100_normalized",
		"left_index": gin.H{
			"name":   leftName,
			"symbol": leftSymbol,
		},
		"right_index": gin.H{
			"name":   rightName,
			"symbol": rightSymbol,
		},
		"baseline_date":    first.TradingDate,
		"latest_date":      last.TradingDate,
		"left_return_pct":  roundComparisonReturn(last.LeftValue),
		"right_return_pct": roundComparisonReturn(last.RightValue),
		"data_points":      len(points),
		"data":             points,
		"notice":           "Historical normalized performance comparison on common trading dates only. Base 100 is a relative scale, not an index price or investment recommendation.",
	})
}

func roundComparisonReturn(value float64) float64 {
	return math.Round((value-100)*100) / 100
}

type StockIndexComparisonPoint struct {
	TradingDate  string  `json:"trading_date"`
	StockBase100 float64 `json:"stock_base_100"`
	IndexBase100 float64 `json:"index_base_100"`
}

func (h *IndexDataHandler) CompareCompanyIndex(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || companyID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	indexID, err := strconv.ParseInt(c.Param("index_id"), 10, 64)
	if err != nil || indexID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid index ID"})
		return
	}

	days := 30
	if raw := c.Query("days"); raw != "" {
		days, err = strconv.Atoi(raw)
		if err != nil || days < 2 || days > 3650 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Days must be between 2 and 3650"})
			return
		}
	}

	var companyName, companySymbol string
	err = h.DB.QueryRowContext(c.Request.Context(), `
		SELECT name, symbol
		FROM companies
		WHERE id = $1
	`, companyID).Scan(&companyName, &companySymbol)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load company"})
		return
	}

	var indexName, indexSymbol string
	err = h.DB.QueryRowContext(c.Request.Context(), `
		SELECT name, symbol
		FROM market_indices
		WHERE id = $1 AND active = TRUE
	`, indexID).Scan(&indexName, &indexSymbol)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Active index not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load index"})
		return
	}

	rows, err := h.DB.QueryContext(c.Request.Context(), `
		WITH stock_prices AS (
			SELECT DISTINCT ON (trading_date)
				trading_date, close
			FROM market_prices
			WHERE company_id = $1
			ORDER BY trading_date, fetched_at DESC
		),
		index_history AS (
			SELECT DISTINCT ON (trading_date)
				trading_date, close
			FROM index_prices
			WHERE index_id = $2
			ORDER BY trading_date, fetched_at DESC
		),
		matched AS (
			SELECT
				s.trading_date,
				s.close AS stock_close,
				i.close AS index_close
			FROM stock_prices s
			INNER JOIN index_history i
				ON i.trading_date = s.trading_date
		),
		recent AS (
			SELECT *
			FROM matched
			ORDER BY trading_date DESC
			LIMIT $3
		),
		baseline AS (
			SELECT
				(array_agg(stock_close ORDER BY trading_date))[1] AS stock_base,
				(array_agg(index_close ORDER BY trading_date))[1] AS index_base
			FROM recent
		)
		SELECT
			r.trading_date::text,
			ROUND((r.stock_close / NULLIF(b.stock_base, 0)) * 100, 4)::text,
			ROUND((r.index_close / NULLIF(b.index_base, 0)) * 100, 4)::text
		FROM recent r
		CROSS JOIN baseline b
		ORDER BY r.trading_date
	`, companyID, indexID, days)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not calculate stock-index comparison"})
		return
	}
	defer rows.Close()

	points := make([]StockIndexComparisonPoint, 0)

	for rows.Next() {
		var point StockIndexComparisonPoint
		var stockValue, indexValue string

		if err := rows.Scan(&point.TradingDate, &stockValue, &indexValue); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not read comparison data"})
			return
		}

		point.StockBase100, err = strconv.ParseFloat(stockValue, 64)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid stock comparison value"})
			return
		}

		point.IndexBase100, err = strconv.ParseFloat(indexValue, 64)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid index comparison value"})
			return
		}

		points = append(points, point)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Comparison iteration failed"})
		return
	}

	if len(points) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"count":   0,
			"message": "No common historical trading dates found",
			"data":    points,
		})
		return
	}

	first := points[0]
	last := points[len(points)-1]
	stockReturn := roundComparisonReturn(last.StockBase100)
	indexReturn := roundComparisonReturn(last.IndexBase100)

	c.JSON(http.StatusOK, gin.H{
		"comparison_type": "stock_vs_index_base_100",
		"company": gin.H{
			"name":   companyName,
			"symbol": companySymbol,
		},
		"index": gin.H{
			"name":   indexName,
			"symbol": indexSymbol,
		},
		"baseline_date":           first.TradingDate,
		"latest_date":             last.TradingDate,
		"stock_return_pct":        stockReturn,
		"index_return_pct":        indexReturn,
		"relative_difference_pct": math.Round((stockReturn-indexReturn)*100) / 100,
		"data_points":             len(points),
		"data":                    points,
		"notice":                  "Historical descriptive comparison on common trading dates only. Relative difference is not evidence that the index or company caused the other's performance. Not investment advice.",
	})
}
