package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var gdeltRequestMu sync.Mutex
var gdeltLastRequest time.Time

func doGDELTRequest(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	gdeltRequestMu.Lock()
	defer gdeltRequestMu.Unlock()

	if !gdeltLastRequest.IsZero() {
		wait := 5*time.Second - time.Since(gdeltLastRequest)
		if wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}

	gdeltLastRequest = time.Now()
	return client.Do(req)
}

type NewsHandler struct {
	DB *sql.DB
}

type GDELTResponse struct {
	Articles []GDELTArticle `json:"articles"`
}

type GDELTArticle struct {
	Title         string `json:"title"`
	URL           string `json:"url"`
	Domain        string `json:"domain"`
	Language      string `json:"language"`
	SourceCountry string `json:"sourcecountry"`
	SeenDate      string `json:"seendate"`
}

type NewsArticle struct {
	ID                int64      `json:"id"`
	Title             string     `json:"title"`
	URL               string     `json:"url"`
	Domain            *string    `json:"domain"`
	Language          *string    `json:"language"`
	SourceCountry     *string    `json:"source_country"`
	PublishedAt       *time.Time `json:"published_at"`
	FetchedAt         time.Time  `json:"fetched_at"`
	RelevanceCategory string     `json:"relevance_category"`
	RelevanceScore    int        `json:"relevance_score"`
	RelevanceReason   *string    `json:"relevance_reason"`
}

func parseGDELTDate(value string) *time.Time {
	if value == "" {
		return nil
	}

	t, err := time.Parse("20060102T150405Z", value)
	if err != nil {
		return nil
	}

	return &t
}

func (h *NewsHandler) Collect(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Query("company_id"), 10, 64)
	if err != nil || companyID < 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Valid company_id is required",
		})
		return
	}

	var companyName string

	err = h.DB.QueryRow(
		"SELECT name FROM companies WHERE id=$1",
		companyID,
	).Scan(&companyName)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database query failed",
		})
		return
	}

	query := strings.TrimSpace(c.Query("query"))
	if query == "" {
		query = companyName
	}

	if len(query) > 150 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Search query too long",
		})
		return
	}

	params := url.Values{}
	params.Set("query", query)
	params.Set("mode", "ArtList")
	params.Set("format", "json")
	params.Set("maxrecords", "20")
	params.Set("timespan", "7d")
	params.Set("sort", "DateDesc")

	apiURL := "https://api.gdeltproject.org/api/v2/doc/doc?" + params.Encode()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		apiURL,
		nil,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not create news request",
		})
		return
	}

	req.Header.Set("User-Agent", "MarketMindAI/0.1")

	client := &http.Client{
		Timeout: 25 * time.Second,
	}

	resp, err := doGDELTRequest(ctx, client, req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "News provider unavailable",
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		log.Printf("GDELT HTTP ERROR: status=%d body=%q", resp.StatusCode, string(body))

		if resp.StatusCode == http.StatusTooManyRequests {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "GDELT rate limit reached. Wait before trying again."})
			return
		}

		c.JSON(http.StatusBadGateway, gin.H{
			"error":           "News provider returned an error",
			"provider_status": resp.StatusCode,
		})
		return
	}

	var result GDELTResponse

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "Invalid news provider response",
		})
		return
	}

	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Database transaction failed",
		})
		return
	}
	defer tx.Rollback()

	inserted := 0
	linked := 0

	for _, article := range result.Articles {
		title := strings.TrimSpace(article.Title)
		articleURL := strings.TrimSpace(article.URL)

		parsedURL, parseErr := url.Parse(articleURL)

		if title == "" || parseErr != nil ||
			parsedURL == nil ||
			(parsedURL.Scheme != "https" && parsedURL.Scheme != "http") ||
			parsedURL.Host == "" {
			continue
		}

		publishedAt := parseGDELTDate(article.SeenDate)

		var newsID int64

		err := tx.QueryRow(`
			INSERT INTO news_articles
			(title, url, domain, language, source_country, published_at)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (url)
			DO UPDATE SET title=EXCLUDED.title
			RETURNING id
		`,
			title,
			articleURL,
			article.Domain,
			article.Language,
			article.SourceCountry,
			publishedAt,
		).Scan(&newsID)

		if err != nil {
			log.Printf("NEWS INSERT ERROR: %v", err)

			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Could not save news article",
			})
			return
		}

		inserted++

		tag, err := tx.Exec(`
			INSERT INTO company_news (company_id, news_id)
			VALUES ($1,$2)
			ON CONFLICT DO NOTHING
		`, companyID, newsID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Could not link news to company",
			})
			return
		}

		count, _ := tag.RowsAffected()
		linked += int(count)
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not commit news data",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":             "completed",
		"company":            companyName,
		"query":              query,
		"articles_processed": inserted,
		"new_company_links":  linked,
		"provider":           "GDELT DOC 2.0",
		"message":            fmt.Sprintf("Processed %d article records", inserted),
	})
}

func (h *NewsHandler) ListByCompany(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || companyID < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	rows, err := h.DB.Query(`
		SELECT n.id, n.title, n.url, n.domain,
		       n.language, n.source_country,
		       n.published_at, n.fetched_at,
                       cn.relevance_category, cn.relevance_score, cn.relevance_reason
		FROM news_articles n
		JOIN company_news cn ON cn.news_id=n.id
		WHERE cn.company_id=$1
		ORDER BY n.published_at DESC NULLS LAST, n.id DESC
		LIMIT $2
	`, companyID, limit)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Could not retrieve company news",
		})
		return
	}
	defer rows.Close()

	articles := make([]NewsArticle, 0)

	for rows.Next() {
		var a NewsArticle

		err := rows.Scan(
			&a.ID,
			&a.Title,
			&a.URL,
			&a.Domain,
			&a.Language,
			&a.SourceCountry,
			&a.PublishedAt,
			&a.FetchedAt,
			&a.RelevanceCategory,
			&a.RelevanceScore,
			&a.RelevanceReason,
		)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Could not read news article",
			})
			return
		}

		articles = append(articles, a)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "News iteration failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"company_id": companyID,
		"count":      len(articles),
		"data":       articles,
	})
}
