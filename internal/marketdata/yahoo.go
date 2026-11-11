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

const yahooChartURL = "https://query1.finance.yahoo.com/v8/finance/chart/"

type YahooClient struct {
	httpClient *http.Client
	chartURL   string
}

type IndexPrice struct {
	Date   string
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume *int64
}

type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Symbol           string `json:"symbol"`
				ExchangeName     string `json:"fullExchangeName"`
				Currency         string `json:"currency"`
				ExchangeTimezone string `json:"exchangeTimezoneName"`
			} `json:"meta"`

			Timestamp []int64 `json:"timestamp"`

			Indicators struct {
				Quote []struct {
					Open   []*float64 `json:"open"`
					High   []*float64 `json:"high"`
					Low    []*float64 `json:"low"`
					Close  []*float64 `json:"close"`
					Volume []*int64   `json:"volume"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`

		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func NewYahooClient() *YahooClient {
	return &YahooClient{
		httpClient: &http.Client{
			Timeout: 25 * time.Second,
		},
	}
}

func (c *YahooClient) FetchIndexPrices(
	ctx context.Context,
	symbol string,
	from string,
	to string,
) ([]IndexPrice, error) {
	symbol = strings.TrimSpace(symbol)

	if symbol == "" || strings.ContainsAny(symbol, "/?#") {
		return nil, fmt.Errorf("invalid Yahoo index symbol")
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

	period1 := fromDate.UTC().Unix()
	period2 := toDate.AddDate(0, 0, 1).UTC().Unix()

	endpoint := strings.TrimRight(c.chartURL, "/") + "/" + url.PathEscape(symbol)
	query := url.Values{}
	query.Set("period1", fmt.Sprintf("%d", period1))
	query.Set("period2", fmt.Sprintf("%d", period2))
	query.Set("interval", "1d")
	query.Set("events", "history")

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint+"?"+query.Encode(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("could not create Yahoo request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 MarketMindAI/0.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Yahoo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf(
			"Yahoo returned HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	var result yahooChartResponse
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 5*1024*1024))

	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("invalid Yahoo JSON response: %w", err)
	}

	if result.Chart.Error != nil {
		return nil, fmt.Errorf(
			"Yahoo chart error: %s",
			result.Chart.Error.Description,
		)
	}

	if len(result.Chart.Result) == 0 {
		return nil, fmt.Errorf("Yahoo returned no index data")
	}

	data := result.Chart.Result[0]

	if !strings.EqualFold(data.Meta.Symbol, symbol) {
		return nil, fmt.Errorf("Yahoo response symbol does not match request")
	}

	if len(data.Indicators.Quote) == 0 {
		return nil, fmt.Errorf("Yahoo response has no quote data")
	}

	quote := data.Indicators.Quote[0]
	count := len(data.Timestamp)

	if len(quote.Open) != count ||
		len(quote.High) != count ||
		len(quote.Low) != count ||
		len(quote.Close) != count {
		return nil, fmt.Errorf("Yahoo returned inconsistent OHLC arrays")
	}

	location := time.UTC
	if data.Meta.ExchangeTimezone != "" {
		if loc, err := time.LoadLocation(data.Meta.ExchangeTimezone); err == nil {
			location = loc
		}
	}

	prices := make([]IndexPrice, 0, count)

	for i, timestamp := range data.Timestamp {
		o := quote.Open[i]
		h := quote.High[i]
		l := quote.Low[i]
		cl := quote.Close[i]

		// Ignore incomplete sessions.
		if o == nil || h == nil || l == nil || cl == nil {
			continue
		}

		if *o <= 0 || *h <= 0 || *l <= 0 || *cl <= 0 ||
			*h < *l || *o < *l || *o > *h ||
			*cl < *l || *cl > *h {
			return nil, fmt.Errorf("invalid Yahoo OHLC values at record %d", i)
		}

		tradingDate := time.Unix(timestamp, 0).In(location).Format("2006-01-02")

		if tradingDate < from || tradingDate > to {
			continue
		}

		var volume *int64
		if i < len(quote.Volume) && quote.Volume[i] != nil {
			if *quote.Volume[i] < 0 {
				return nil, fmt.Errorf("negative index volume at record %d", i)
			}
			v := *quote.Volume[i]
			volume = &v
		}

		prices = append(prices, IndexPrice{
			Date:   tradingDate,
			Open:   *o,
			High:   *h,
			Low:    *l,
			Close:  *cl,
			Volume: volume,
		})
	}

	return prices, nil
}
