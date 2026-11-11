package handlers

import (
	"database/sql"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"marketmind-ai/internal/forecast"
)

const (
	defaultForecastRidgeLambda = 1.0
	maxForecastHistorySessions = 5000
)

type ForecastHandler struct {
	DB *sql.DB
}

type ForecastRequest struct {
	HorizonSessions int     "json:\"horizon_sessions\""
	Provider         string  "json:\"provider\""
	RidgeLambda      float64 "json:\"ridge_lambda,omitempty\""
}

type ForecastModelPrediction struct {
	ModelName          string   "json:\"model_name\""
	PredictedReturnPct float64  "json:\"predicted_return_pct\""
	IndicativePrice    *float64 "json:\"indicative_price,omitempty\""
}

func (h *ForecastHandler) Run(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || companyID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	var request ForecastRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON request"})
		return
	}

	if !isSupportedForecastHorizon(request.HorizonSessions) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "horizon_sessions must be one of 1, 5, 10 or 20",
		})
		return
	}

	request.Provider = strings.TrimSpace(request.Provider)
	if len(request.Provider) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Provider name must not exceed 80 characters"})
		return
	}

	if request.RidgeLambda == 0 {
		request.RidgeLambda = defaultForecastRidgeLambda
	}
	if math.IsNaN(request.RidgeLambda) ||
		math.IsInf(request.RidgeLambda, 0) ||
		request.RidgeLambda <= 0 ||
		request.RidgeLambda > 1e6 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "ridge_lambda must be finite, greater than zero and no greater than 1000000",
		})
		return
	}

	ctx := c.Request.Context()

	var companyName, symbol, exchange string
	err = h.DB.QueryRowContext(ctx,
		"SELECT name, symbol, exchange FROM companies WHERE id = $1",
		companyID,
	).Scan(&companyName, &symbol, &exchange)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load company"})
		return
	}

	rows, err := h.DB.QueryContext(ctx,
		"SELECT trading_date::text, open::text, high::text, low::text, close::text, volume, provider "+
			"FROM market_prices WHERE company_id = $1 AND ($2 = '' OR provider = $2) "+
			"ORDER BY trading_date ASC, provider ASC LIMIT 5001",
		companyID, request.Provider,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load historical prices"})
		return
	}
	defer rows.Close()

	bars := make([]forecast.Bar, 0)
	selectedProvider := ""

	for rows.Next() {
		if len(bars) >= maxForecastHistorySessions {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error": "Forecast history exceeds the 5000-session processing limit; select a provider with a smaller history",
			})
			return
		}

		var dateText, openText, highText, lowText, closeText, provider string
		var volume int64
		if err := rows.Scan(
			&dateText, &openText, &highText, &lowText, &closeText, &volume, &provider,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not read historical price records"})
			return
		}

		date, err := time.Parse("2006-01-02", dateText)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid trading date in stored market data"})
			return
		}

		if len(bars) > 0 && !date.After(bars[len(bars)-1].Date) {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Multiple provider records or duplicate dates exist; specify one provider",
			})
			return
		}

		if selectedProvider == "" {
			selectedProvider = provider
		} else if selectedProvider != provider {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Historical range contains multiple providers; specify one provider",
			})
			return
		}

		values := make([]float64, 4)
		for i, raw := range []string{openText, highText, lowText, closeText} {
			values[i], err = strconv.ParseFloat(raw, 64)
			if err != nil || math.IsNaN(values[i]) || math.IsInf(values[i], 0) {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid OHLC value in stored market data"})
				return
			}
		}

		bars = append(bars, forecast.Bar{
			Date: date, Open: values[0], High: values[1], Low: values[2],
			Close: values[3], Volume: volume,
		})
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Historical price query failed"})
		return
	}
	if len(bars) < 2 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "At least two historical sessions are required",
		})
		return
	}

	features, err := forecast.CalculateFeatures(bars)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Could not calculate technical features", "detail": err.Error()})
		return
	}

	targets, err := forecast.CalculateTargets(bars)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Could not calculate forward-return targets", "detail": err.Error()})
		return
	}

	dataset, err := forecast.BuildDataset(features, targets, request.HorizonSessions)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Could not build forecast training dataset", "detail": err.Error()})
		return
	}

	latestFeatures := features[len(features)-1]

	ridge, err := forecast.NewRidgeRegression(request.RidgeLambda)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Ridge Regression configuration"})
		return
	}
	if err := ridge.Fit(dataset.Rows, request.HorizonSessions); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "Insufficient or invalid historical observations for Ridge Regression",
			"detail": err.Error(),
		})
		return
	}

	ridgeReturn, err := ridge.Predict(latestFeatures)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "Latest session does not contain the features required for prediction",
			"detail": err.Error(),
		})
		return
	}

	zero := &forecast.ZeroReturnBaseline{}
	if err := zero.Fit(dataset.Rows, request.HorizonSessions); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Could not fit zero-return baseline"})
		return
	}
	zeroReturn, err := zero.Predict()
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Could not generate zero-return baseline"})
		return
	}

	mean := &forecast.HistoricalMeanBaseline{}
	if err := mean.Fit(dataset.Rows, request.HorizonSessions); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Could not fit historical-mean baseline"})
		return
	}
	meanReturn, err := mean.Predict()
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Could not generate historical-mean baseline"})
		return
	}

	lastBar := bars[len(bars)-1]
	predictions := []ForecastModelPrediction{
		forecastPrediction("ridge_regression", ridgeReturn, lastBar.Close),
		forecastPrediction("zero_return", zeroReturn, lastBar.Close),
		forecastPrediction("historical_mean", meanReturn, lastBar.Close),
	}

	c.JSON(http.StatusOK, gin.H{
		"company": gin.H{
			"id": companyID, "name": companyName, "symbol": symbol, "exchange": exchange,
		},
		"data_type": "historical_eod_model_forecast",
		"forecast": gin.H{
			"generated_at":              time.Now().UTC(),
			"as_of_date":                lastBar.Date.Format("2006-01-02"),
			"horizon_sessions":          request.HorizonSessions,
			"provider":                  selectedProvider,
			"last_close":                lastBar.Close,
			"training_observations":     len(dataset.Rows),
			"incomplete_target_count":   dataset.IncompleteCount,
			"ridge_model_metadata":      ridge.Metadata(),
			"models":                    predictions,
			"price_estimate_definition": "Indicative price = latest historical close multiplied by (1 + predicted simple return / 100).",
		},
		"limitations": []string{
			"Uses stored historical end-of-day data and technical features available at the latest stored session close.",
			"Predicted returns and indicative prices are experimental model outputs, not guaranteed outcomes or investment advice.",
			"Ridge Regression is not a calibrated probability model; no confidence level is implied.",
			"Historical baselines are included for comparison and do not establish future predictive skill.",
			"Corporate actions, survivorship bias, data revisions and provider coverage may affect results.",
		},
	})
}

func isSupportedForecastHorizon(horizon int) bool {
	for _, supported := range forecast.SupportedHorizons() {
		if horizon == supported {
			return true
		}
	}
	return false
}

func forecastPrediction(name string, predictedReturnPct, lastClose float64) ForecastModelPrediction {
	prediction := ForecastModelPrediction{
		ModelName:          name,
		PredictedReturnPct: predictedReturnPct,
	}

	// A simple-return price projection is undefined for a return at or below -100%.
	if predictedReturnPct <= -100 {
		return prediction
	}

	price := lastClose * (1 + predictedReturnPct/100)
	if price > 0 && !math.IsNaN(price) && !math.IsInf(price, 0) {
		prediction.IndicativePrice = &price
	}
	return prediction
}
