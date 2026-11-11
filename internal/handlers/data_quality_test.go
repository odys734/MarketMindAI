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

func TestDataQualityEndpoint(t *testing.T) {
	db, err := database.Connect(config.Load())
	if err != nil {
		t.Skipf("PostgreSQL unavailable; skipping integration test: %v", err)
	}
	defer db.Close()

	gin.SetMode(gin.TestMode)

	router := gin.New()
	handler := &handlers.DataQualityHandler{DB: db}
	router.GET("/api/v1/market/data-quality", handler.Get)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/market/data-quality",
		nil,
	)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Count int `json:"count"`
		Data  []struct {
			Type                  string `json:"type"`
			Symbol                string `json:"symbol"`
			Records               int64  `json:"records"`
			MultipleProviderDates *int64 `json:"multiple_provider_dates"`
		} `json:"data"`
		Notice string `json:"notice"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	if response.Count != len(response.Data) {
		t.Fatalf("count mismatch: count=%d, data length=%d",
			response.Count, len(response.Data))
	}

	if response.Notice == "" {
		t.Error("expected a data quality notice")
	}

	for _, item := range response.Data {
		if item.Symbol == "" {
			t.Error("instrument has an empty symbol")
		}

		if item.MultipleProviderDates == nil {
			t.Errorf("%s: multiple_provider_dates field is missing", item.Symbol)
		}
	}
}
