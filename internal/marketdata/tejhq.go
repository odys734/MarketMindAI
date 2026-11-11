package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const baseURL = "https://api.tejhq.dev/v1"

type Client struct {
	httpClient *http.Client
	baseURL    string
}

type Status struct {
	TradingDate        string `json:"trading_date"`
	Stage              string `json:"stage"`
	PricesPublishedAt  string `json:"prices_published_at"`
	DerivedPublishedAt string `json:"derived_published_at"`
	GeneratedAt        string `json:"generated_at"`
}

type Price struct {
	Date      string      `json:"date"`
	Open      json.Number `json:"open"`
	High      json.Number `json:"high"`
	Low       json.Number `json:"low"`
	Close     json.Number `json:"close"`
	Last      json.Number `json:"last"`
	PrevClose json.Number `json:"prev_close"`
	Volume    json.Number `json:"volume"`
	Turnover  json.Number `json:"turnover"`
	Trades    json.Number `json:"trades"`
}

type priceResponse struct {
	Data []Price `json:"data"`
	Meta struct {
		Count    int    `json:"count"`
		Exchange string `json:"exchange"`
		Symbol   string `json:"symbol"`
		From     string `json:"from"`
		To       string `json:"to"`
	} `json:"meta"`
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 25 * time.Second,
		},
	}
}

func (c *Client) getJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MarketMindAI/0.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("TejHQ request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("TejHQ returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	decoder := json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024))
	decoder.UseNumber()

	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid TejHQ JSON response: %w", err)
	}

	return nil
}

func (c *Client) GetStatus(ctx context.Context) (Status, error) {
	var status Status

	err := c.getJSON(ctx, c.baseURL+"/status", &status)
	if err != nil {
		return status, err
	}

	if _, err := time.Parse("2006-01-02", status.TradingDate); err != nil {
		return status, fmt.Errorf("TejHQ returned invalid trading date")
	}

	return status, nil
}

func (c *Client) FetchPrices(
	ctx context.Context,
	exchange string,
	symbol string,
	from string,
	to string,
) ([]Price, error) {
	exchange = strings.ToLower(strings.TrimSpace(exchange))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	if exchange != "nse" && exchange != "bse" {
		return nil, fmt.Errorf("unsupported exchange: %s", exchange)
	}

	if symbol == "" || strings.ContainsAny(symbol, "/?#") {
		return nil, fmt.Errorf("invalid symbol")
	}

	fromDate, err := time.Parse("2006-01-02", from)
	if err != nil {
		return nil, fmt.Errorf("invalid from date: use YYYY-MM-DD")
	}

	toDate, err := time.Parse("2006-01-02", to)
	if err != nil {
		return nil, fmt.Errorf("invalid to date: use YYYY-MM-DD")
	}

	if fromDate.After(toDate) {
		return nil, fmt.Errorf("from date cannot be after to date")
	}

	endpoint := fmt.Sprintf(
		"%s/ohlcv/%s/%s?from=%s&to=%s",
		c.baseURL,
		exchange,
		url.PathEscape(symbol),
		url.QueryEscape(from),
		url.QueryEscape(to),
	)

	var result priceResponse
	if err := c.getJSON(ctx, endpoint, &result); err != nil {
		return nil, err
	}

	if !strings.EqualFold(result.Meta.Exchange, exchange) ||
		!strings.EqualFold(result.Meta.Symbol, symbol) {
		return nil, fmt.Errorf("TejHQ response metadata does not match the requested instrument")
	}

	for i, p := range result.Data {
		if _, err := time.Parse("2006-01-02", p.Date); err != nil {
			return nil, fmt.Errorf("invalid trading date in record %d", i)
		}

		open, e1 := p.Open.Float64()
		high, e2 := p.High.Float64()
		low, e3 := p.Low.Float64()
		closePrice, e4 := p.Close.Float64()

		if e1 != nil || e2 != nil || e3 != nil || e4 != nil ||
			open <= 0 || high <= 0 || low <= 0 || closePrice <= 0 ||
			high < low || open < low || open > high ||
			closePrice < low || closePrice > high {
			return nil, fmt.Errorf("invalid OHLC values in record %d", i)
		}

		volume, e5 := p.Volume.Int64()
		trades, e6 := p.Trades.Int64()
		turnover, e7 := p.Turnover.Float64()

		if e5 != nil || e6 != nil || e7 != nil ||
			volume < 0 || trades < 0 || turnover < 0 {
			return nil, fmt.Errorf("invalid volume, turnover or trades in record %d", i)
		}
	}

	return result.Data, nil
}
