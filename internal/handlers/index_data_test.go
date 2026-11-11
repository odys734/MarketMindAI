package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"marketmind-ai/internal/config"
	"marketmind-ai/internal/database"
	"marketmind-ai/internal/handlers"
)

func TestIndexAnalyticsEndpoint(t *testing.T) {
	db, err := database.Connect(config.Load())
	if err != nil {
		t.Skipf("PostgreSQL unavailable: %v", err)
	}
	defer db.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()

	handler := &handlers.IndexDataHandler{DB: db}
	router.GET("/api/v1/indices/:id/analytics", handler.GetAnalytics)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/indices/1/analytics?days=30", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result["symbol"] != "NIFTY50" {
		t.Errorf("expected NIFTY50, got %v", result["symbol"])
	}

	if result["data_type"] != "historical_daily" {
		t.Errorf("unexpected data_type: %v", result["data_type"])
	}

	if result["notice"] == nil {
		t.Error("expected historical analytics notice")
	}

	if points, ok := result["data_points"].(float64); !ok || points < 2 {
		t.Errorf("expected at least 2 data points, got %v", result["data_points"])
	}
}

func TestCompareIndicesEndpoint(t *testing.T) {
	db, err := database.Connect(config.Load())
	if err != nil {
		t.Skipf("PostgreSQL unavailable: %v", err)
	}
	defer db.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()

	handler := &handlers.IndexDataHandler{DB: db}
	router.GET("/api/v1/indices/compare", handler.CompareIndices)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/indices/compare?left_id=1&right_id=2&days=30",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		ComparisonType string `json:"comparison_type"`
		DataPoints     int    `json:"data_points"`
		Data           []struct {
			TradingDate string  `json:"trading_date"`
			LeftValue   float64 `json:"left_base_100"`
			RightValue  float64 `json:"right_base_100"`
		} `json:"data"`
		Notice string `json:"notice"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result.ComparisonType != "base_100_normalized" {
		t.Errorf("unexpected comparison type: %s", result.ComparisonType)
	}

	if result.DataPoints != len(result.Data) {
		t.Errorf("data_points mismatch: %d vs %d", result.DataPoints, len(result.Data))
	}

	if result.DataPoints < 2 {
		t.Fatalf("expected at least 2 common dates, got %d", result.DataPoints)
	}

	if result.Data[0].LeftValue != 100 || result.Data[0].RightValue != 100 {
		t.Errorf("first common date should be normalized to 100; got %.4f and %.4f",
			result.Data[0].LeftValue, result.Data[0].RightValue)
	}

	if result.Notice == "" {
		t.Error("expected comparison notice")
	}
}

func TestCompareIndicesRejectsSameID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	handler := &handlers.IndexDataHandler{}
	router.GET("/api/v1/indices/compare", handler.CompareIndices)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/indices/compare?left_id=1&right_id=1",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400, got %d", rec.Code)
	}
}

func TestIndexAnalyticsRejectsInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	handler := &handlers.IndexDataHandler{}
	router.GET("/api/v1/indices/:id/analytics", handler.GetAnalytics)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/indices/0/analytics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400, got %d", rec.Code)
	}
}

func TestIndexAnalyticsRejectsInvalidDays(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	handler := &handlers.IndexDataHandler{}
	router.GET("/api/v1/indices/:id/analytics", handler.GetAnalytics)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/indices/1/analytics?days=1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400, got %d", rec.Code)
	}
}
