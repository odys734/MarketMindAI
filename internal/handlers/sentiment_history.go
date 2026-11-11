package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type SentimentHistoryHandler struct {
	DB *sql.DB
}

func (h *SentimentHistoryHandler) Get(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || companyID < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	days := 30
	if value := c.Query("days"); value != "" {
		days, err = strconv.Atoi(value)
		if err != nil || days < 1 || days > 365 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Days must be between 1 and 365",
			})
			return
		}
	}

	var companyName string
	err = h.DB.QueryRow(
		`SELECT name FROM companies WHERE id = $1`,
		companyID,
	).Scan(&companyName)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load company"})
		return
	}

	rows, err := h.DB.Query(`
		SELECT
			DATE(analyzed_at) AS analysis_date,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE sentiment = 'positive') AS positive,
			COUNT(*) FILTER (WHERE sentiment = 'negative') AS negative,
			COUNT(*) FILTER (WHERE sentiment = 'mixed') AS mixed,
			COUNT(*) FILTER (WHERE sentiment = 'neutral') AS neutral,
			COUNT(*) FILTER (WHERE sentiment = 'unknown') AS unknown,
			AVG(confidence)::float8 AS average_confidence
		FROM ai_news_analysis
		WHERE company_id = $1
		  AND analyzed_at >= NOW() - ($2 * INTERVAL '1 day')
		GROUP BY DATE(analyzed_at)
		ORDER BY analysis_date ASC
	`, companyID, days)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not calculate sentiment history",
		})
		return
	}
	defer rows.Close()

	history := make([]gin.H, 0)

	for rows.Next() {
		var date string
		var total, positive, negative, mixed, neutral, unknown int
		var averageConfidence float64

		if err := rows.Scan(
			&date,
			&total,
			&positive,
			&negative,
			&mixed,
			&neutral,
			&unknown,
			&averageConfidence,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Could not read sentiment history",
			})
			return
		}

		history = append(history, gin.H{
			"date":  date,
			"total": total,
			"sentiment": gin.H{
				"positive": positive,
				"negative": negative,
				"mixed":    mixed,
				"neutral":  neutral,
				"unknown":  unknown,
			},
			"average_confidence": averageConfidence,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Sentiment history query failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"company":     companyName,
		"period_days": days,
		"history":     history,
		"notice":      "Historical counts describe analyzed news sentiment, not historical stock-price performance or future price predictions.",
	})
}
