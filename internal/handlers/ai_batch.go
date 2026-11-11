package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type BatchNewsItem struct {
	ID              int64
	Title           string
	Domain          string
	CompanyName     string
	Sector          string
	Industry        string
	Relevance       string
	RelevanceScore  int
	RelevanceReason string
}

func (h *AINewsHandler) AnalyzeBatch(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || companyID < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	limit := 5
	if value := c.Query("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 5 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Batch limit must be between 1 and 5",
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
			n.id,
			n.title,
			COALESCE(n.domain, ''),
			co.name,
			COALESCE(co.sector, ''),
			COALESCE(co.industry, ''),
			cn.relevance_category,
			cn.relevance_score,
			COALESCE(cn.relevance_reason, '')
		FROM company_news cn
		JOIN companies co ON co.id = cn.company_id
		JOIN news_articles n ON n.id = cn.news_id
		WHERE cn.company_id = $1
		  AND cn.relevance_category IN ('direct', 'sector', 'macro')
		  AND NOT EXISTS (
			SELECT 1
			FROM ai_news_analysis a
			WHERE a.company_id = cn.company_id
			  AND a.news_id = cn.news_id
		  )
		ORDER BY cn.relevance_score DESC, cn.matched_at DESC
		LIMIT $2
	`, companyID, limit)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load pending news"})
		return
	}

	pending := make([]BatchNewsItem, 0, limit)

	for rows.Next() {
		var item BatchNewsItem
		if err := rows.Scan(
			&item.ID,
			&item.Title,
			&item.Domain,
			&item.CompanyName,
			&item.Sector,
			&item.Industry,
			&item.Relevance,
			&item.RelevanceScore,
			&item.RelevanceReason,
		); err != nil {
			rows.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not read pending news"})
			return
		}
		pending = append(pending, item)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Pending news query failed"})
		return
	}
	rows.Close()

	succeeded := make([]gin.H, 0)
	failed := make([]gin.H, 0)

	for _, item := range pending {
		input := map[string]interface{}{
			"company": map[string]string{
				"name":     item.CompanyName,
				"sector":   item.Sector,
				"industry": item.Industry,
			},
			"article": map[string]interface{}{
				"title":              item.Title,
				"domain":             item.Domain,
				"relevance_category": item.Relevance,
				"relevance_score":    item.RelevanceScore,
				"relevance_reason":   item.RelevanceReason,
			},
		}

		inputJSON, err := json.Marshal(input)
		if err != nil {
			failed = append(failed, gin.H{
				"news_id": item.ID,
				"error":   "Could not prepare AI input",
			})
			continue
		}

		systemPrompt := `
You are MarketMind AI, an evidence-conscious business news research analyst.

Analyze only the metadata provided.

IMPORTANT:
- You have NOT read the full article.
- Never invent facts, financial figures or announcements.
- Sector-level news does not prove direct company impact.
- Distinguish article sentiment from possible business impact.
- Use unknown or neutral when evidence is insufficient.
- Confidence measures interpretation confidence, not price probability.
- Evidence must identify exact headline wording.
- Clearly explain uncertainty.
- Never provide buy, sell or guaranteed price predictions.

Return ONLY valid JSON:
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

		var result AIAnalysisResult
		var analysisErr string
		valid := false

		for attempt := 1; attempt <= 2; attempt++ {
			ctx, cancel := context.WithTimeout(
				c.Request.Context(),
				60*time.Second,
			)

			prompt := systemPrompt

			if attempt == 2 {
				prompt += `
Your previous response was invalid.
Return a corrected response using ONLY the exact JSON schema.
Do not add Markdown fences, explanations, or text outside JSON.
Do not invent missing facts.
`
			}

			output, chatErr := h.AI.Chat(ctx, prompt, string(inputJSON))
			cancel()

			if chatErr != nil {
				analysisErr = "AI analysis request failed"
				continue
			}

			output = extractJSON(output)

			if err := json.Unmarshal([]byte(output), &result); err != nil {
				log.Printf("BATCH AI INVALID JSON: news_id=%d attempt=%d response=%q",
					item.ID, attempt, output)
				analysisErr = "AI returned invalid JSON after retry"
				continue
			}

			if !validAnalysis(result) {
				analysisErr = "AI returned invalid analysis fields after retry"
				continue
			}

			valid = true
			break
		}

		if !valid {
			failed = append(failed, gin.H{
				"news_id": item.ID,
				"error":   analysisErr,
			})
			continue
		}

		evidenceJSON, err := json.Marshal(result.Evidence)
		if err != nil {
			failed = append(failed, gin.H{
				"news_id": item.ID,
				"error":   "Could not encode evidence",
			})
			continue
		}

		_, err = h.DB.Exec(`
			INSERT INTO ai_news_analysis (
				company_id, news_id, summary, event_type,
				sentiment, impact_direction, impact_summary,
				confidence, evidence, uncertainty, model, analyzed_at
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,NOW())
			ON CONFLICT (company_id, news_id) DO NOTHING
		`,
			companyID,
			item.ID,
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
			failed = append(failed, gin.H{
				"news_id": item.ID,
				"error":   "Could not save analysis",
			})
			continue
		}

		succeeded = append(succeeded, gin.H{
			"news_id":    item.ID,
			"title":      item.Title,
			"sentiment":  result.Sentiment,
			"confidence": result.Confidence,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"status":       "completed",
		"company":      companyName,
		"requested":    limit,
		"found":        len(pending),
		"succeeded":    len(succeeded),
		"failed_count": len(failed),
		"skipped":      limit - len(pending),
		"results":      succeeded,
		"errors":       failed,
		"notice":       "Analysis uses headline metadata, not verified full article contents. Not investment advice.",
	})
}
