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

func TestMarketOverviewEndpoint(t *testing.T) {
	db, err := database.Connect(config.Load())
	if err != nil {
		t.Skipf("PostgreSQL unavailable: %v", err)
	}
	defer db.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()

	handler := &handlers.MarketOverviewHandler{DB: db}
	router.GET("/api/v1/market/overview", handler.Get)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/market/overview",
		nil,
	)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Indices []struct {
			Symbol   string  `json:"symbol"`
			Close    float64 `json:"close"`
			Provider string  `json:"provider"`
		} `json:"market_indices"`
		Companies []struct {
			Symbol string  `json:"symbol"`
			Close  float64 `json:"close"`
		} `json:"companies"`
		DataType string `json:"data_type"`
		Notice   string `json:"notice"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result.DataType != "historical_latest_available" {
		t.Errorf("unexpected data_type: %s", result.DataType)
	}

	if result.Notice == "" {
		t.Error("expected historical data notice")
	}

	found := make(map[string]bool)

	for _, item := range result.Indices {
		if item.Close <= 0 {
			t.Errorf("%s has invalid close: %f", item.Symbol, item.Close)
		}
		found[item.Symbol] = true
	}

	for _, symbol := range []string{"NIFTY50", "SENSEX"} {
		if !found[symbol] {
			t.Errorf("missing index: %s", symbol)
		}
	}

	relianceFound := false
	for _, item := range result.Companies {
		if item.Symbol == "RELIANCE" {
			relianceFound = true
			if item.Close <= 0 {
				t.Errorf("RELIANCE has invalid close: %f", item.Close)
			}
		}
	}

	if !relianceFound {
		t.Error("RELIANCE missing from market overview")
	}
}
