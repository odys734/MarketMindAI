package handlers_test

import (
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

func TestForecastEvaluationRejectsInvalidConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		body string
	}{
		{"unsupported horizon", `{"horizons":[2]}`},
		{"duplicate horizon", `{"horizons":[1,1]}`},
		{"minimum training exceeds initial", `{"initial_train_size":40,"minimum_train_size":50}`},
		{"test step overlaps", `{"test_size":20,"step_size":10}`},
		{"invalid ridge lambda", `{"ridge_lambda":-1}`},
		{"zero ridge lambda", `{"ridge_lambda":0}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/companies/:id/forecast/evaluate", (&handlers.ForecastEvaluationHandler{}).Run)
			req := httptest.NewRequest(http.MethodPost, "/companies/1/forecast/evaluate", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected HTTP 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestForecastEvaluationRunsWalkForwardModelComparison(t *testing.T) {
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
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 240; i++ {
		closePrice := 100 + float64(i)*0.08 + 2.5*math.Sin(float64(i)*0.17)
		priceRows.AddRow(
			start.AddDate(0, 0, i).Format("2006-01-02"),
			fmt.Sprintf("%.4f", closePrice),
			fmt.Sprintf("%.4f", closePrice+1),
			fmt.Sprintf("%.4f", closePrice-1),
			fmt.Sprintf("%.4f", closePrice),
			int64(10000+i*10),
			"tejhq",
		)
	}
	mock.ExpectQuery("(?s)SELECT trading_date::text.*FROM market_prices.*LIMIT 5001").
		WithArgs(int64(1), "tejhq").
		WillReturnRows(priceRows)

	router := gin.New()
	router.POST("/companies/:id/forecast/evaluate", (&handlers.ForecastEvaluationHandler{DB: db}).Run)
	body := `{
		"provider":"tejhq",
		"horizons":[5,20],
		"initial_train_size":80,
		"minimum_train_size":30,
		"test_size":20,
		"step_size":20,
		"ridge_lambda":1
	}`
	req := httptest.NewRequest(http.MethodPost, "/companies/1/forecast/evaluate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, expected := range []string{
		`"data_type":"historical_eod_walk_forward_evaluation"`,
		`"validation":"expanding_window_walk_forward"`,
		`"model_name":"zero_return"`,
		`"model_name":"historical_mean"`,
		`"model_name":"ridge_regression"`,
		`"mae"`,
		`"directional_accuracy_pct"`,
		`"purged_observations"`,
	} {
		if !strings.Contains(rec.Body.String(), expected) {
			t.Errorf("response missing %s", expected)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

