package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"marketmind-ai/internal/ai"

	"github.com/gin-gonic/gin"
)

type AINewsHandler struct {
	DB *sql.DB
	AI *ai.Client
}

type AIAnalysisResult struct {
	Summary         string   `json:"summary"`
	EventType       string   `json:"event_type"`
	Sentiment       string   `json:"sentiment"`
	ImpactDirection string   `json:"impact_direction"`
	ImpactSummary   string   `json:"impact_summary"`
	Confidence      int      `json:"confidence"`
	Evidence        []string `json:"evidence"`
	Uncertainty     string   `json:"uncertainty"`
}

func extractJSON(output string) string {
	output = strings.TrimSpace(output)
	output = strings.TrimPrefix(output, "```json")
	output = strings.TrimPrefix(output, "```")
	output = strings.TrimSuffix(output, "```")
	output = strings.TrimSpace(output)

	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")

	if start < 0 || end < start {
		return output
	}

	return output[start : end+1]
}

func validAnalysis(r AIAnalysisResult) bool {
	allowed := map[string]bool{
		"positive": true,
		"negative": true,
		"mixed":    true,
		"neutral":  true,
		"unknown":  true,
	}

	return strings.TrimSpace(r.Summary) != "" &&
		strings.TrimSpace(r.EventType) != "" &&
		allowed[r.Sentiment] &&
		allowed[r.ImpactDirection] &&
		strings.TrimSpace(r.ImpactSummary) != "" &&
		r.Confidence >= 0 &&
		r.Confidence <= 100 &&
		strings.TrimSpace(r.Uncertainty) != ""
}

func (h *AINewsHandler) Analyze(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || companyID < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	newsID, err := strconv.ParseInt(c.Param("news_id"), 10, 64)
	if err != nil || newsID < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid news ID"})
		return
	}

	var companyName, title, domain string
	var sector, industry sql.NullString
	var relevanceCategory, relevanceReason string
	var relevanceScore int

	err = h.DB.QueryRow(`
		SELECT
			co.name,
			co.sector,
			co.industry,
			n.title,
			COALESCE(n.domain, ''),
			cn.relevance_category,
			cn.relevance_score,
			COALESCE(cn.relevance_reason, '')
		FROM companies co
		JOIN company_news cn ON cn.company_id = co.id
		JOIN news_articles n ON n.id = cn.news_id
		WHERE co.id = $1 AND n.id = $2
	`,
		companyID,
		newsID,
	).Scan(
		&companyName,
		&sector,
		&industry,
		&title,
		&domain,
		&relevanceCategory,
		&relevanceScore,
		&relevanceReason,
	)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Company news article not found",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not load article metadata",
		})
		return
	}

	input := map[string]interface{}{
		"company": map[string]string{
			"name":     companyName,
			"sector":   sector.String,
			"industry": industry.String,
		},
		"article": map[string]interface{}{
			"title":              title,
			"domain":             domain,
			"relevance_category": relevanceCategory,
			"relevance_score":    relevanceScore,
			"relevance_reason":   relevanceReason,
		},
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not prepare AI input",
		})
		return
	}

	systemPrompt := `
You are MarketMind AI, an evidence-conscious business news research analyst.

Analyze only the metadata provided by the user.

IMPORTANT:
- You have NOT read the full article.
- Never invent facts, events, financial figures or company announcements.
- Do not treat a sector-level headline as proof of direct company impact.
- Distinguish article sentiment from possible business impact.
- If the headline does not provide enough information, use unknown or neutral.
- Confidence means confidence in your interpretation, not probability of stock movement.
- Evidence must quote or identify exact wording from the supplied headline.
- Clearly state missing information and uncertainty.
- Never provide buy, sell or guaranteed price predictions.

Return ONLY valid JSON with these exact fields:
{
  "summary": "short summary",
  "event_type": "event category",
  "sentiment": "positive, negative, mixed, neutral or unknown",
  "impact_direction": "positive, negative, mixed, neutral or unknown",
  "impact_summary": "possible business implications",
  "confidence": 0,
  "evidence": ["headline evidence"],
  "uncertainty": "what is not established"
}
`

	ctx, cancel := context.WithTimeout(
		c.Request.Context(),
		60*time.Second,
	)
	defer cancel()

	output, err := h.AI.Chat(ctx, systemPrompt, string(inputJSON))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "AI analysis failed",
		})
		return
	}

	output = strings.TrimSpace(output)

	output = strings.TrimPrefix(output, "```json")
	output = strings.TrimPrefix(output, "```")
	output = strings.TrimSuffix(output, "```")
	output = strings.TrimSpace(output)

	var result AIAnalysisResult

	if err := json.Unmarshal([]byte(output), &result); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "AI returned invalid JSON",
		})
		return
	}

	if !validAnalysis(result) {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "AI returned invalid analysis fields",
		})
		return
	}

	evidenceJSON, err := json.Marshal(result.Evidence)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not encode evidence",
		})
		return
	}

	_, err = h.DB.Exec(`
		INSERT INTO ai_news_analysis (
			company_id,
			news_id,
			summary,
			event_type,
			sentiment,
			impact_direction,
			impact_summary,
			confidence,
			evidence,
			uncertainty,
			model,
			analyzed_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,NOW())
		ON CONFLICT (company_id, news_id)
		DO UPDATE SET
			summary = EXCLUDED.summary,
			event_type = EXCLUDED.event_type,
			sentiment = EXCLUDED.sentiment,
			impact_direction = EXCLUDED.impact_direction,
			impact_summary = EXCLUDED.impact_summary,
			confidence = EXCLUDED.confidence,
			evidence = EXCLUDED.evidence,
			uncertainty = EXCLUDED.uncertainty,
			model = EXCLUDED.model,
			analyzed_at = NOW()
	`,
		companyID,
		newsID,
		result.Summary,
		result.EventType,
		result.Sentiment,
		result.ImpactDirection,
		result.ImpactSummary,
		result.Confidence,
		string(evidenceJSON),
		result.Uncertainty,
		h.AI.Model,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not save AI analysis",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "completed",
		"company":  companyName,
		"news_id":  newsID,
		"model":    h.AI.Model,
		"analysis": result,
		"notice":   "Research analysis based on headline metadata, not a verified full-article assessment.",
	})
}

// GetAnalysis retrieves a previously saved AI news analysis.
func (h *AINewsHandler) GetAnalysis(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || companyID < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	newsID, err := strconv.ParseInt(c.Param("news_id"), 10, 64)
	if err != nil || newsID < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid news ID"})
		return
	}

	var companyName, title string
	var result AIAnalysisResult
	var evidenceJSON []byte
	var model string
	var analyzedAt time.Time

	err = h.DB.QueryRow(`
		SELECT
			co.name,
			n.title,
			a.summary,
			a.event_type,
			a.sentiment,
			a.impact_direction,
			a.impact_summary,
			a.confidence,
			a.evidence,
			a.uncertainty,
			a.model,
			a.analyzed_at
		FROM ai_news_analysis a
		JOIN companies co ON co.id = a.company_id
		JOIN news_articles n ON n.id = a.news_id
		WHERE a.company_id = $1 AND a.news_id = $2
	`,
		companyID,
		newsID,
	).Scan(
		&companyName,
		&title,
		&result.Summary,
		&result.EventType,
		&result.Sentiment,
		&result.ImpactDirection,
		&result.ImpactSummary,
		&result.Confidence,
		&evidenceJSON,
		&result.Uncertainty,
		&model,
		&analyzedAt,
	)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "AI analysis not found. Analyze this article first.",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not retrieve AI analysis",
		})
		return
	}

	if err := json.Unmarshal(evidenceJSON, &result.Evidence); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not decode analysis evidence",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "completed",
		"company":     companyName,
		"news_id":     newsID,
		"article":     title,
		"model":       model,
		"analyzed_at": analyzedAt,
		"analysis":    result,
		"notice":      "Research analysis based on headline metadata, not a verified full-article assessment.",
	})
}

// ListAnalyses returns saved AI analyses for all news linked to a company.
func (h *AINewsHandler) ListAnalyses(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || companyID < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	limit := 20
	offset := 0

	if value := c.Query("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Limit must be between 1 and 100"})
			return
		}
	}

	if value := c.Query("offset"); value != "" {
		offset, err = strconv.Atoi(value)
		if err != nil || offset < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid offset"})
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
			a.news_id,
			n.title,
			a.summary,
			a.event_type,
			a.sentiment,
			a.impact_direction,
			a.impact_summary,
			a.confidence,
			a.evidence,
			a.uncertainty,
			a.model,
			a.analyzed_at
		FROM ai_news_analysis a
		JOIN news_articles n ON n.id = a.news_id
		WHERE a.company_id = $1
		ORDER BY a.analyzed_at DESC
		LIMIT $2 OFFSET $3
	`, companyID, limit, offset)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load analyses"})
		return
	}
	defer rows.Close()

	type AnalysisItem struct {
		NewsID     int64            `json:"news_id"`
		Title      string           `json:"title"`
		Analysis   AIAnalysisResult `json:"analysis"`
		Model      string           `json:"model"`
		AnalyzedAt time.Time        `json:"analyzed_at"`
	}

	items := make([]AnalysisItem, 0)

	for rows.Next() {
		var item AnalysisItem
		var evidenceJSON []byte

		err := rows.Scan(
			&item.NewsID,
			&item.Title,
			&item.Analysis.Summary,
			&item.Analysis.EventType,
			&item.Analysis.Sentiment,
			&item.Analysis.ImpactDirection,
			&item.Analysis.ImpactSummary,
			&item.Analysis.Confidence,
			&evidenceJSON,
			&item.Analysis.Uncertainty,
			&item.Model,
			&item.AnalyzedAt,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not read analysis"})
			return
		}

		if err := json.Unmarshal(evidenceJSON, &item.Analysis.Evidence); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not decode evidence"})
			return
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Analysis query failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"company": companyName,
		"count":   len(items),
		"limit":   limit,
		"offset":  offset,
		"results": items,
	})
}
