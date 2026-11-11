package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestYahooFetchIndexPrices(t *testing.T) {
	timestamp := time.Date(2026, 11, 9, 6, 0, 0, 0, time.UTC).Unix()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"chart": {
				"result": [{
					"meta": {
						"symbol": "^NSEI",
						"exchangeTimezoneName": "Asia/Kolkata"
					},
					"timestamp": [%d],
					"indicators": {
						"quote": [{
							"open": [100.0],
							"high": [110.0],
							"low": [90.0],
							"close": [105.0],
							"volume": [123]
						}]
					}
				}],
				"error": null
			}
		}`, timestamp)
	}))
	defer server.Close()

	client := &YahooClient{
		httpClient: server.Client(),
		chartURL:   server.URL,
	}

	prices, err := client.FetchIndexPrices(
		context.Background(),
		"^NSEI",
		"2026-11-09",
		"2026-11-09",
	)
	if err != nil {
		t.Fatalf("unexpected Yahoo error: %v", err)
	}

	if len(prices) != 1 {
		t.Fatalf("expected 1 price, got %d", len(prices))
	}

	if prices[0].Close != 105 {
		t.Errorf("expected close 105, got %f", prices[0].Close)
	}

	if prices[0].Date != "2026-11-09" {
		t.Errorf("unexpected trading date: %s", prices[0].Date)
	}

	if prices[0].Volume == nil || *prices[0].Volume != 123 {
		t.Errorf("unexpected volume: %v", prices[0].Volume)
	}
}

func TestYahooRejectsMismatchedSymbol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			"chart": {
				"result": [{
					"meta": {"symbol": "^BSESN"},
					"timestamp": [],
					"indicators": {"quote": []}
				}],
				"error": null
			}
		}`)
	}))
	defer server.Close()

	client := &YahooClient{
		httpClient: server.Client(),
		chartURL:   server.URL,
	}

	_, err := client.FetchIndexPrices(
		context.Background(), "^NSEI", "2026-11-09", "2026-11-09",
	)

	if err == nil {
		t.Fatal("expected symbol mismatch error")
	}
}

func TestTejHQFetchPrices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"meta": {
				"exchange": "nse",
				"symbol": "RELIANCE"
			},
			"data": [{
				"date": "2026-11-09",
				"open": 1300,
				"high": 1330,
				"low": 1290,
				"close": 1324.4,
				"last": 1324.4,
				"prev_close": 1320,
				"volume": 1000,
				"turnover": 1324400,
				"trades": 500
			}]
		}`)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	prices, err := client.FetchPrices(
		context.Background(),
		"nse",
		"RELIANCE",
		"2026-11-09",
		"2026-11-09",
	)
	if err != nil {
		t.Fatalf("unexpected TejHQ error: %v", err)
	}

	if len(prices) != 1 {
		t.Fatalf("expected 1 price, got %d", len(prices))
	}

	closePrice, err := prices[0].Close.Float64()
	if err != nil {
		t.Fatal(err)
	}

	if closePrice != 1324.4 {
		t.Errorf("expected close 1324.4, got %f", closePrice)
	}
}

func TestTejHQRejectsMismatchedMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			"meta": {
				"exchange": "nse",
				"symbol": "TCS"
			},
			"data": []
		}`)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	_, err := client.FetchPrices(
		context.Background(), "nse", "RELIANCE",
		"2026-11-09", "2026-11-09",
	)

	if err == nil {
		t.Fatal("expected metadata mismatch error")
	}
}

func TestTejHQRejectsInvalidOHLC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{
			"meta": {"exchange":"nse","symbol":"RELIANCE"},
			"data":[{
				"date":"2026-11-09",
				"open":1300,
				"high":1200,
				"low":1290,
				"close":1310,
				"volume":1000,
				"turnover":1000,
				"trades":100
			}]
		}`)
	}))
	defer server.Close()

	client := &Client{httpClient: server.Client(), baseURL: server.URL}

	_, err := client.FetchPrices(
		context.Background(), "nse", "RELIANCE",
		"2026-11-09", "2026-11-09",
	)

	if err == nil {
		t.Fatal("expected invalid OHLC error")
	}
}

func TestYahooRejectsInvalidDateRange(t *testing.T) {
	client := NewYahooClient()

	_, err := client.FetchIndexPrices(
		context.Background(), "^NSEI",
		"2026-11-10", "2026-11-09",
	)

	if err == nil {
		t.Fatal("expected invalid date range error")
	}
}
