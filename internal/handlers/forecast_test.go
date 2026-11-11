package handlers_test

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"marketmind-ai/internal/handlers"
)

func TestForecastRejectsInvalidRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "invalid company ID", path: "/companies/0/forecast", body: "{\"horizon_sessions\":5}"},
		{name: "invalid JSON", path: "/companies/1/forecast", body: "{invalid}"},
		{name: "unsupported horizon", path: "/companies/1/forecast", body: "{\"horizon_sessions\":2}"},
		{name: "negative lambda", path: "/companies/1/forecast", body: "{\"horizon_sessions\":5,\"ridge_lambda\":-1}"},
		{name: "provider too long", path: "/companies/1/forecast", body: "{\"horizon_sessions\":5,\"provider\":\"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\"}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/companies/:id/forecast", (&handlers.ForecastHandler{}).Run)

			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected HTTP 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestForecastSuccessfulPrediction(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT name, symbol, exchange, currency FROM companies WHERE id = \\$1").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "symbol", "exchange", "currency"}).
			AddRow("Test Company", "TEST", "NSE", "INR"))

	priceRows := sqlmock.NewRows([]string{
		"trading_date", "open", "high", "low", "close", "volume", "provider",
	})

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 120; i++ {
		closePrice := 100 + float64(i)*0.2 + 2*math.Sin(float64(i)*0.3)
		priceRows.AddRow(
			start.AddDate(0, 0, i).Format("2006-01-02"),
			fmt.Sprintf("%.4f", closePrice*0.999),
			fmt.Sprintf("%.4f", closePrice+1),
			fmt.Sprintf("%.4f", closePrice-1),
			fmt.Sprintf("%.4f", closePrice),
			int64(10000+i*25),
			"tejhq",
		)
	}

	mock.ExpectQuery("(?s)SELECT trading_date::text.*FROM market_prices.*LIMIT 5001").
		WithArgs(int64(1), "tejhq").
		WillReturnRows(priceRows)

	router := gin.New()
	router.POST("/companies/:id/forecast", (&handlers.ForecastHandler{DB: db}).Run)

	body := "{\"horizon_sessions\":5,\"provider\":\"tejhq\",\"ridge_lambda\":1}"
	req := httptest.NewRequest(http.MethodPost, "/companies/1/forecast", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response struct {
		DataType string "json:\"data_type\""
		Forecast struct {
			HorizonSessions      int "json:\"horizon_sessions\""
			Provider             string "json:\"provider\""
			DatasetTrainingRows int "json:\"dataset_training_rows\""
			Models []struct {
				ModelName string "json:\"model_name\""
				PredictedReturnPct float64 "json:\"predicted_return_pct\""
				IndicativePrice *float64 "json:\"indicative_price\""
			} "json:\"models\""
		} "json:\"forecast\""
		Limitations []string "json:\"limitations\""
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if response.DataType != "historical_eod_model_forecast" {
		t.Fatalf("unexpected data type: %q", response.DataType)
	}
	if response.Forecast.HorizonSessions != 5 || response.Forecast.Provider != "tejhq" {
		t.Fatalf("unexpected forecast metadata: %+v", response.Forecast)
	}
	if response.Forecast.DatasetTrainingRows < 10 {
		t.Fatalf("expected at least 10 training observations, got %d", response.Forecast.DatasetTrainingRows)
	}
	if len(response.Forecast.Models) != 3 {
		t.Fatalf("expected three model predictions, got %d", len(response.Forecast.Models))
	}
	for i, expected := range []string{"ridge_regression", "zero_return", "historical_mean"} {
		model := response.Forecast.Models[i]
		if model.ModelName != expected {
			t.Errorf("model %d: got %q, want %q", i, model.ModelName, expected)
		}
		if math.IsNaN(model.PredictedReturnPct) || math.IsInf(model.PredictedReturnPct, 0) {
			t.Errorf("model %q returned a non-finite prediction", model.ModelName)
		}
		if model.IndicativePrice != nil && (*model.IndicativePrice <= 0 ||
			math.IsNaN(*model.IndicativePrice) || math.IsInf(*model.IndicativePrice, 0)) {
			t.Errorf("model %q returned an invalid indicative price", model.ModelName)
		}
	}
	if len(response.Limitations) == 0 {
		t.Error("expected forecast limitations in response")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations not met: %v", err)
	}
}

func TestForecastRejectsMixedProviders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT name, symbol, exchange, currency FROM companies WHERE id = \\$1").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "symbol", "exchange", "currency"}).
			AddRow("Test Company", "TEST", "NSE", "INR"))

	priceRows := sqlmock.NewRows([]string{
		"trading_date", "open", "high", "low", "close", "volume", "provider",
	}).AddRow("2026-01-01", "99", "101", "98", "100", int64(1000), "tejhq").
		AddRow("2026-01-02", "100", "102", "99", "101", int64(1100), "other")

	mock.ExpectQuery("(?s)SELECT trading_date::text.*FROM market_prices.*LIMIT 5001").
		WithArgs(int64(1), "").
		WillReturnRows(priceRows)

	router := gin.New()
	router.POST("/companies/:id/forecast", (&handlers.ForecastHandler{DB: db}).Run)

	req := httptest.NewRequest(
		http.MethodPost,
		"/companies/1/forecast",
		strings.NewReader("{\"horizon_sessions\":1}"),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected HTTP 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
