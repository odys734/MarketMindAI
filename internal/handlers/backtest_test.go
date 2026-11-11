package handlers_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"marketmind-ai/internal/handlers"
)

func TestBacktestRejectsInvalidRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "invalid company ID",
			path: "/companies/0/backtest",
			body: `{}`,
		},
		{
			name: "invalid JSON",
			path: "/companies/1/backtest",
			body: `{invalid}`,
		},
		{
			name: "invalid from date",
			path: "/companies/1/backtest",
			body: `{"from":"bad","to":"2026-01-01","fast_period":2,"slow_period":3,"initial_capital":1000}`,
		},
		{
			name: "invalid date range",
			path: "/companies/1/backtest",
			body: `{"from":"2026-02-01","to":"2026-01-01","fast_period":2,"slow_period":3,"initial_capital":1000}`,
		},
		{
			name: "invalid SMA periods",
			path: "/companies/1/backtest",
			body: `{"from":"2026-01-01","to":"2026-02-01","fast_period":5,"slow_period":3,"initial_capital":1000}`,
		},
		{
			name: "invalid capital",
			path: "/companies/1/backtest",
			body: `{"from":"2026-01-01","to":"2026-02-01","fast_period":2,"slow_period":3,"initial_capital":0}`,
		},
		{
			name: "invalid risk free rate",
			path: "/companies/1/backtest",
			body: `{"from":"2026-01-01","to":"2026-02-01","fast_period":2,"slow_period":3,"initial_capital":1000,"annual_risk_free_rate":2}`,
		},
		{
			name: "provider too long",
			path: "/companies/1/backtest",
			body: `{"from":"2026-01-01","to":"2026-02-01","fast_period":2,"slow_period":3,"initial_capital":1000,"provider":"` + strings.Repeat("x", 81) + `"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			handler := &handlers.BacktestHandler{}

			router.POST("/companies/:id/backtest", handler.Run)

			req := httptest.NewRequest(
				http.MethodPost,
				tt.path,
				strings.NewReader(tt.body),
			)
			req.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf(
					"expected HTTP 400, got %d: %s",
					rec.Code,
					rec.Body.String(),
				)
			}
		})
	}
}

func TestBacktestSuccessfulSimulation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT name, symbol, exchange FROM companies WHERE id = \\$1").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{
			"name", "symbol", "exchange",
		}).AddRow("Test Company", "TEST", "NSE"))

	priceRows := sqlmock.NewRows([]string{
		"trading_date", "open", "high", "low", "close", "provider",
	})

	closes := []float64{2, 2, 2, 4, 2, 1, 1}

	for i, closePrice := range closes {
		date := time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC)
		priceRows.AddRow(
			date.Format("2006-01-02"),
			fmt.Sprint(closePrice),
			fmt.Sprint(closePrice+1),
			fmt.Sprint(closePrice-0.5),
			fmt.Sprint(closePrice),
			"tejhq",
		)
	}

	mock.ExpectQuery("(?s)SELECT\\s+trading_date::text,\\s+open::text,\\s+high::text,\\s+low::text,\\s+close::text,\\s+provider\\s+FROM market_prices").
		WithArgs(int64(1), "2026-01-01", "2026-01-07", "tejhq").
		WillReturnRows(priceRows)

	router := gin.New()
	handler := &handlers.BacktestHandler{DB: db}
	router.POST("/companies/:id/backtest", handler.Run)

	body := `{
		"from":"2026-01-01",
		"to":"2026-01-07",
		"fast_period":2,
		"slow_period":3,
		"initial_capital":100000,
		"commission_bps":10,
		"slippage_bps":5,
		"annual_risk_free_rate":0.05,
		"provider":"tejhq"
	}`

	req := httptest.NewRequest(
		http.MethodPost,
		"/companies/1/backtest",
		strings.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}

	for _, expected := range []string{
		`"data_type":"historical_eod_simulation"`,
		`"provider":"tejhq"`,
		`"signals_count":2`,
		`"closed_trades":1`,
		`"equity_curve"`,
		`"notice"`,
	} {
		if !strings.Contains(rec.Body.String(), expected) {
			t.Errorf("response missing %s", expected)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations not met: %v", err)
	}
}
