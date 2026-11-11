package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type RelevanceResult struct {
	Category string `json:"category"`
	Score    int    `json:"score"`
	Reason   string `json:"reason"`
}

func containsPhrase(text, phrase string) bool {
	text = " " + strings.ToLower(text) + " "
	phrase = " " + strings.ToLower(strings.TrimSpace(phrase)) + " "
	return strings.Contains(text, phrase)
}

func cleanCompanyName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))

	suffixes := []string{
		" limited", " ltd.", " ltd",
		" incorporated", " inc.",
		" inc", " corporation", " corp.",
		" corp",
	}

	for _, suffix := range suffixes {
		name = strings.TrimSuffix(name, suffix)
	}

	return strings.TrimSpace(name)
}

func classifyNews(title, companyName, symbol, sector, industry string) RelevanceResult {
	titleLower := strings.ToLower(strings.TrimSpace(title))
	if titleLower == "" {
		return RelevanceResult{
			Category: "unrelated",
			Score:    0,
			Reason:   "Article title is empty",
		}
	}

	coreName := cleanCompanyName(companyName)

	if coreName != "" && strings.Contains(titleLower, coreName) {
		return RelevanceResult{
			Category: "direct",
			Score:    95,
			Reason:   "Company name appears in the article title",
		}
	}

	symbol = strings.ToLower(strings.TrimSpace(symbol))
	if symbol != "" && containsPhrase(titleLower, symbol) {
		return RelevanceResult{
			Category: "direct",
			Score:    90,
			Reason:   "Company name or ticker identifier appears in the article title",
		}
	}

	sectorTerms := map[string][]string{
		"energy": {
			"crude oil", "petroleum", "oil prices", "oil refinery",
			"refining", "natural gas", "lng", "fuel",
			"oil and gas", "energy expenses", "energy costs",
			"energy consumption", "oil dominance", "oil imports",
			"oil exports", "oil buyers", "oil production",
			"oil supply", "oil demand", "oil shipping",
			"crude imports", "crude exports", "energy sector", "energy expenses",
			"energy costs", "energy consumption", "oil dominance",
			"oil imports", "oil exports", "oil buyers",
			"oil production", "oil supply", "oil demand",
			"oil shipping", "crude imports", "crude exports",
		},
		"oil and gas": {
			"crude oil", "petroleum", "oil prices", "oil refinery",
			"refining", "natural gas", "lng", "fuel",
			"oil and gas", "energy expenses", "energy costs",
			"energy consumption", "oil dominance", "oil imports",
			"oil exports", "oil buyers", "oil production",
			"oil supply", "oil demand", "oil shipping",
			"crude imports", "crude exports",
		},
		"technology": {
			"software", "semiconductor", "artificial intelligence",
			"cloud computing", "technology sector",
		},
		"banking": {
			"banking sector", "bank stocks", "lending",
			"non-performing loans",
		},
		"automobile": {
			"automobile industry", "car sales", "vehicle sales",
			"electric vehicles", "auto sector",
		},
		"pharmaceuticals": {
			"pharmaceutical industry", "drug maker",
			"clinical trial", "pharma sector",
		},
	}

	sectorKey := strings.ToLower(strings.TrimSpace(sector))
	industryKey := strings.ToLower(strings.TrimSpace(industry))

	terms := append(
		sectorTerms[sectorKey],
		sectorTerms[industryKey]...,
	)

	for _, term := range terms {
		if strings.Contains(titleLower, term) {
			return RelevanceResult{
				Category: "sector",
				Score:    65,
				Reason:   "Article title contains a keyword related to the company's sector or industry: " + term,
			}
		}
	}

	macroTerms := []string{
		"sensex", "nifty", "rbi", "reserve bank",
		"inflation", "interest rate", "gdp",
		"indian economy", "global economy",
		"rupee", "fii inflows", "fii outflows",
		"stock market", "market rally", "market selloff",
	}

	for _, term := range macroTerms {
		if strings.Contains(titleLower, term) {
			return RelevanceResult{
				Category: "macro",
				Score:    40,
				Reason:   "Article title contains a broad economic or market keyword: " + term,
			}
		}
	}

	return RelevanceResult{
		Category: "unrelated",
		Score:    0,
		Reason:   "No matching company, sector, or macro keyword found in the title",
	}
}

func (h *NewsHandler) AnalyzeRelevance(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || companyID < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	var name, symbol string
	var sector, industry sql.NullString

	err = h.DB.QueryRow(`
		SELECT name, symbol, sector, industry
		FROM companies
		WHERE id = $1
	`, companyID).Scan(&name, &symbol, &sector, &industry)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load company"})
		return
	}

	rows, err := h.DB.Query(`
		SELECT n.id, n.title
		FROM news_articles n
		JOIN company_news cn ON cn.news_id = n.id
		WHERE cn.company_id = $1
	`, companyID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load company news"})
		return
	}

	type article struct {
		ID    int64
		Title string
	}

	articles := make([]article, 0)

	for rows.Next() {
		var a article
		if err := rows.Scan(&a.ID, &a.Title); err != nil {
			rows.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not read article"})
			return
		}
		articles = append(articles, a)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "News iteration failed"})
		return
	}
	rows.Close()

	tx, err := h.DB.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not start analysis transaction"})
		return
	}
	defer tx.Rollback()

	counts := map[string]int{
		"direct":    0,
		"sector":    0,
		"macro":     0,
		"unrelated": 0,
	}

	for _, a := range articles {
		result := classifyNews(
			a.Title,
			name,
			symbol,
			sector.String,
			industry.String,
		)

		_, err := tx.Exec(`
			UPDATE company_news
			SET relevance_category = $1,
			    relevance_score = $2,
			    relevance_reason = $3,
			    analyzed_at = $4
			WHERE company_id = $5 AND news_id = $6
		`,
			result.Category,
			result.Score,
			result.Reason,
			time.Now(),
			companyID,
			a.ID,
		)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not save relevance analysis"})
			return
		}

		counts[result.Category]++
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not commit relevance analysis"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":            "completed",
		"company":           name,
		"articles_analyzed": len(articles),
		"categories":        counts,
		"method":            "rule_based_title_analysis",
		"notice":            "Scores are heuristic relevance indicators, not probabilities or investment recommendations.",
	})
}
