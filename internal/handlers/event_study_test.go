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

func TestEventStudyRejectsInvalidRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		path string
	}{
		{"invalid company ID", "/companies/0/event-study?index_id=1&from=2026-01-01&to=2026-01-10&provider=tejhq&index_provider=yahoo_chart"},
		{"missing index ID", "/companies/1/event-study?from=2026-01-01&to=2026-01-10&provider=tejhq&index_provider=yahoo_chart"},
		{"invalid index ID", "/companies/1/event-study?index_id=bad&from=2026-01-01&to=2026-01-10&provider=tejhq&index_provider=yahoo_chart"},
		{"invalid from date", "/companies/1/event-study?index_id=1&from=bad&to=2026-01-10&provider=tejhq&index_provider=yahoo_chart"},
		{"invalid to date", "/companies/1/event-study?index_id=1&from=2026-01-01&to=bad&provider=tejhq&index_provider=yahoo_chart"},
		{"reversed dates", "/companies/1/event-study?index_id=1&from=2026-02-01&to=2026-01-01&provider=tejhq&index_provider=yahoo_chart"},
		{"negative pre window", "/companies/1/event-study?index_id=1&from=2026-01-01&to=2026-01-10&pre=-1&provider=tejhq&index_provider=yahoo_chart"},
		{"oversized post window", "/companies/1/event-study?index_id=1&from=2026-01-01&to=2026-01-10&post=31&provider=tejhq&index_provider=yahoo_chart"},
		{"missing stock provider", "/companies/1/event-study?index_id=1&from=2026-01-01&to=2026-01-10&index_provider=yahoo_chart"},
		{"missing index provider", "/companies/1/event-study?index_id=1&from=2026-01-01&to=2026-01-10&provider=tejhq"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/companies/:id/event-study", (&handlers.EventStudyHandler{}).Get)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected HTTP 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestEventStudySuccessfulCalculation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	from := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT name, symbol, exchange").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "symbol", "exchange"}).
			AddRow("Test Company", "TEST", "NSE"))

	mock.ExpectQuery("SELECT name, symbol, exchange").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "symbol", "exchange"}).
			AddRow("NIFTY 50", "NIFTY50", "NSE"))

	publishedAt := time.Date(2026, 1, 3, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery("(?s)SELECT\\s+na.id,.*FROM news_articles na").
		WithArgs(int64(1), from, to).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "title", "published_at", "sentiment",
			"impact_direction", "event_type", "confidence",
		}).AddRow(
			int64(10), "Historical company event", publishedAt,
			"positive", "positive", "earnings", 85,
		))
	stockRows := sqlmock.NewRows([]string{"trading_date", "close"})
	benchmarkRows := sqlmock.NewRows([]string{"trading_date", "close"})

	for i, closePrice := range []float64{100, 101, 102, 104, 105} {
		date := time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC)
		stockRows.AddRow(date, closePrice)
		benchmarkRows.AddRow(date, 100.0)
	}

	mock.ExpectQuery("(?s)WITH before_window.*FROM market_prices").
		WithArgs(int64(1), "tejhq", from, to).
		WillReturnRows(stockRows)

	mock.ExpectQuery("(?s)WITH before_window.*FROM index_prices").
		WithArgs(int64(1), "yahoo_chart", from, to).
		WillReturnRows(benchmarkRows)

	router := gin.New()
	router.GET("/companies/:id/event-study", (&handlers.EventStudyHandler{DB: db}).Get)

	req := httptest.NewRequest(
		http.MethodGet,
		"/companies/1/event-study?index_id=1&from=2026-01-03&to=2026-01-03&pre=0&post=0&provider=tejhq&index_provider=yahoo_chart",
		nil,
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}

	for _, expected := range []string{
		`"completed":1`,
		`"insufficient_data":0`,
		`"car_pct":`,
		`"abnormal_return_pct":`,
		`"headline":"Historical company event"`,
		`"yahoo_chart"`,
		`"Market-adjusted event study`,
	} {
		if !strings.Contains(rec.Body.String(), expected) {
			t.Errorf("response missing %s", expected)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database expectations not met: %v", err)
	}
}

func TestEventStudyMissingCompany(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT name, symbol, exchange").
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "symbol", "exchange"}))

	router := gin.New()
	router.GET("/companies/:id/event-study", (&handlers.EventStudyHandler{DB: db}).Get)

	req := httptest.NewRequest(
		http.MethodGet,
		"/companies/99/event-study?index_id=1&from=2026-01-01&to=2026-01-10&provider=tejhq&index_provider=yahoo_chart",
		nil,
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected HTTP 404, got %d: %s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(fmt.Errorf("database expectations: %w", err))
	}
}
