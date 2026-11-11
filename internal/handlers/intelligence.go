package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type IntelligenceHandler struct {
	DB *sql.DB
}

func (h *IntelligenceHandler) Get(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || companyID < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
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

	var totalNews, direct, sector, macro, unrelated, unreviewed int
	var analyzed, positive, negative, mixed, neutral, unknown int
	var averageConfidence sql.NullFloat64

	err = h.DB.QueryRow(`
		SELECT
			COUNT(cn.news_id),
			COUNT(*) FILTER (WHERE cn.relevance_category = 'direct'),
			COUNT(*) FILTER (WHERE cn.relevance_category = 'sector'),
			COUNT(*) FILTER (WHERE cn.relevance_category = 'macro'),
			COUNT(*) FILTER (WHERE cn.relevance_category = 'unrelated'),
			COUNT(*) FILTER (WHERE cn.relevance_category = 'unreviewed'),

			COUNT(a.news_id),
			COUNT(*) FILTER (WHERE a.sentiment = 'positive'),
			COUNT(*) FILTER (WHERE a.sentiment = 'negative'),
			COUNT(*) FILTER (WHERE a.sentiment = 'mixed'),
			COUNT(*) FILTER (WHERE a.sentiment = 'neutral'),
			COUNT(*) FILTER (WHERE a.sentiment = 'unknown'),
			AVG(a.confidence)::float8

		FROM company_news cn
		LEFT JOIN ai_news_analysis a
			ON a.company_id = cn.company_id
			AND a.news_id = cn.news_id
		WHERE cn.company_id = $1
	`, companyID).Scan(
		&totalNews,
		&direct,
		&sector,
		&macro,
		&unrelated,
		&unreviewed,
		&analyzed,
		&positive,
		&negative,
		&mixed,
		&neutral,
		&unknown,
		&averageConfidence,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not calculate intelligence metrics"})
		return
	}

	rows, err := h.DB.Query(`
		SELECT
			a.news_id,
			n.title,
			a.summary,
			a.sentiment,
			a.impact_direction,
			a.confidence,
			a.analyzed_at
		FROM ai_news_analysis a
		JOIN news_articles n ON n.id = a.news_id
		WHERE a.company_id = $1
		ORDER BY a.analyzed_at DESC
		LIMIT 5
	`, companyID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load recent analyses"})
		return
	}
	defer rows.Close()

	recent := make([]gin.H, 0)

	for rows.Next() {
		var newsID int64
		var title, summary, sentiment, impactDirection string
		var confidence int
		var analyzedAt interface{}

		if err := rows.Scan(
			&newsID,
			&title,
			&summary,
			&sentiment,
			&impactDirection,
			&confidence,
			&analyzedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not read recent analysis"})
			return
		}

		recent = append(recent, gin.H{
			"news_id":          newsID,
			"title":            title,
			"summary":          summary,
			"sentiment":        sentiment,
			"impact_direction": impactDirection,
			"confidence":       confidence,
			"analyzed_at":      analyzedAt,
		})
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Recent analysis query failed"})
		return
	}

	var confidenceValue interface{}
	if averageConfidence.Valid {
		confidenceValue = averageConfidence.Float64
	}

	c.JSON(http.StatusOK, gin.H{
		"company": companyName,
		"news": gin.H{
			"total": totalNews,
			"relevance": gin.H{
				"direct":     direct,
				"sector":     sector,
				"macro":      macro,
				"unrelated":  unrelated,
				"unreviewed": unreviewed,
			},
		},
		"ai": gin.H{
			"analyzed":  analyzed,
			"remaining": totalNews - analyzed,
			"sentiment": gin.H{
				"positive": positive,
				"negative": negative,
				"mixed":    mixed,
				"neutral":  neutral,
				"unknown":  unknown,
			},
			"average_confidence": confidenceValue,
			"confidence_note":    "Confidence measures AI interpretation confidence, not probability of market movement.",
		},
		"recent_analyses": recent,
		"notice":          "Research intelligence based on collected news metadata and AI analysis. Not investment advice.",
	})
}
