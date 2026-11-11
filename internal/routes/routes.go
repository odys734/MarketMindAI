package routes

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"marketmind-ai/internal/ai"
	"marketmind-ai/internal/handlers"
	"marketmind-ai/internal/marketdata"
)

func Register(app *gin.Engine, db *sql.DB, aiClient *ai.Client) {
	app.GET("/health", handlers.Health)

	app.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":    "MarketMind AI",
			"version": "0.1.0",
			"status":  "running",
		})
	})

	company := &handlers.CompanyHandler{DB: db}
	news := &handlers.NewsHandler{DB: db}
	aiNews := &handlers.AINewsHandler{DB: db, AI: aiClient}
	intelligence := &handlers.IntelligenceHandler{DB: db}
	sentimentHistory := &handlers.SentimentHistoryHandler{DB: db}
	marketData := &handlers.MarketDataHandler{DB: db, Client: marketdata.NewClient()}
	analytics := &handlers.MarketAnalyticsHandler{DB: db}
	newsPriceCorrelation := &handlers.NewsPriceCorrelationHandler{DB: db}
	indexData := &handlers.IndexDataHandler{DB: db, Client: marketdata.NewYahooClient()}
	overview := &handlers.MarketOverviewHandler{DB: db}
	dataQuality := &handlers.DataQualityHandler{DB: db}
	backtestHandler := &handlers.BacktestHandler{DB: db}
	eventStudyHandler := &handlers.EventStudyHandler{DB: db}

	v1 := app.Group("/api/v1")
	{
		v1.GET("/health", handlers.Health)

		v1.POST("/companies", company.Create)
		v1.GET("/companies", company.List)
		v1.GET("/companies/:id", company.Get)
		v1.PUT("/companies/:id", company.Update)
		v1.DELETE("/companies/:id", company.Delete)

		v1.POST("/news/collect", news.Collect)
		v1.GET("/companies/:id/news", news.ListByCompany)
		v1.POST("/companies/:id/news/analyze", news.AnalyzeRelevance)
		v1.POST("/companies/:id/news/:news_id/analyze-ai", aiNews.Analyze)
		v1.POST("/companies/:id/analyze-ai-batch", aiNews.AnalyzeBatch)
		v1.GET("/companies/:id/news/:news_id/analysis-ai", aiNews.GetAnalysis)
		v1.GET("/companies/:id/news/analyses-ai", aiNews.ListAnalyses)
		v1.GET("/companies/:id/intelligence", intelligence.Get)
		v1.GET("/companies/:id/sentiment-history", sentimentHistory.Get)
		v1.POST("/companies/:id/prices/sync", marketData.SyncPrices)
		v1.GET("/companies/:id/prices", marketData.GetPrices)
		v1.GET("/companies/:id/market-analytics", analytics.Get)
		v1.GET("/companies/:id/news-price-correlation", newsPriceCorrelation.Get)
		v1.POST("/indices/:id/prices/sync", indexData.SyncPrices)
		v1.GET("/indices/:id/prices", indexData.GetPrices)
		v1.GET("/market/overview", overview.Get)
		v1.GET("/market/data-quality", dataQuality.Get)
		v1.POST("/companies/:id/backtest", backtestHandler.Run)
		v1.GET("/companies/:id/event-study", eventStudyHandler.Get)
		v1.GET("/indices/:id/analytics", indexData.GetAnalytics)
		v1.GET("/indices/compare", indexData.CompareIndices)
		v1.GET("/companies/:id/compare/indices/:index_id", indexData.CompareCompanyIndex)
	}
}
