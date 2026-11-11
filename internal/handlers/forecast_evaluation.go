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
	defaultEvaluationInitialTrainSize = 80
	defaultEvaluationMinimumTrainSize = 30
	defaultEvaluationTestSize         = 20
	maxEvaluationHistorySessions      = 5000
)

// ForecastEvaluationRequest configures chronological out-of-sample evaluation.
type ForecastEvaluationRequest struct {
	Provider           string  `json:"provider"`
	Horizons            []int   `json:"horizons"`
	InitialTrainSize    int     `json:"initial_train_size"`
	MinimumTrainSize    int     `json:"minimum_train_size"`
	TestSize            int     `json:"test_size"`
	StepSize            int     `json:"step_size"`
	RidgeLambda         *float64 `json:"ridge_lambda,omitempty"`
}

// ForecastEvaluationHandler runs leakage-aware walk-forward model comparisons.
type ForecastEvaluationHandler struct {
	DB *sql.DB
}

func (h *ForecastEvaluationHandler) Run(c *gin.Context) {
	companyID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || companyID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	var request ForecastEvaluationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON request"})
		return
	}

	request.Provider = strings.TrimSpace(request.Provider)
	if len(request.Provider) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Provider name must not exceed 80 characters"})
		return
	}

	if len(request.Horizons) == 0 {
		request.Horizons = forecast.SupportedHorizons()
	}
	if len(request.Horizons) > len(forecast.SupportedHorizons()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At most four supported horizons may be requested"})
		return
	}
	seenHorizons := make(map[int]struct{}, len(request.Horizons))
	for _, horizon := range request.Horizons {
		if !isSupportedForecastHorizon(horizon) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "horizons must contain only 1, 5, 10 or 20"})
			return
		}
		if _, exists := seenHorizons[horizon]; exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "horizons must not contain duplicates"})
			return
		}
		seenHorizons[horizon] = struct{}{}
	}

	if request.InitialTrainSize == 0 {
		request.InitialTrainSize = defaultEvaluationInitialTrainSize
	}
	if request.MinimumTrainSize == 0 {
		request.MinimumTrainSize = defaultEvaluationMinimumTrainSize
	}
	if request.TestSize == 0 {
		request.TestSize = defaultEvaluationTestSize
	}
	if request.StepSize == 0 {
		request.StepSize = request.TestSize
	}
	ridgeLambda := defaultForecastRidgeLambda
	if request.RidgeLambda != nil {
		ridgeLambda = *request.RidgeLambda
	}

	if request.InitialTrainSize < 20 || request.InitialTrainSize > 2000 ||
		request.MinimumTrainSize < 10 || request.MinimumTrainSize > request.InitialTrainSize ||
		request.TestSize < 1 || request.TestSize > 500 ||
		request.StepSize < request.TestSize || request.StepSize > 500 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "walk-forward sizes must satisfy initial_train_size 20..2000, minimum_train_size 10..initial_train_size, and 1 <= test_size <= step_size <= 500",
		})
		return
	}
	if math.IsNaN(ridgeLambda) || math.IsInf(ridgeLambda, 0) ||
		ridgeLambda <= 0 || ridgeLambda > 1e6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ridge_lambda must be finite, greater than zero and no greater than 1000000"})
		return
	}

	ctx := c.Request.Context()
	var companyName, symbol, exchange string
	var currency sql.NullString
	err = h.DB.QueryRowContext(ctx,
		"SELECT name, symbol, exchange, currency FROM companies WHERE id = $1",
		companyID,
	).Scan(&companyName, &symbol, &exchange, &currency)
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
		if len(bars) >= maxEvaluationHistorySessions {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Evaluation history exceeds the 5000-session processing limit; select a provider with a smaller history"})
			return
		}

		var dateText, openText, highText, lowText, closeText, provider string
		var volume int64
		if err := rows.Scan(&dateText, &openText, &highText, &lowText, &closeText, &volume, &provider); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not read historical price records"})
			return
		}
		date, err := time.Parse("2006-01-02", dateText)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid trading date in stored market data"})
			return
		}
		if len(bars) > 0 && !date.After(bars[len(bars)-1].Date) {
			c.JSON(http.StatusConflict, gin.H{"error": "Multiple provider records or duplicate dates exist; specify one provider"})
			return
		}
		if selectedProvider == "" {
			selectedProvider = provider
		} else if selectedProvider != provider {
			c.JSON(http.StatusConflict, gin.H{"error": "Historical range contains multiple providers; specify one provider"})
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
	if len(bars) < 100 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "At least 100 historical sessions are required for walk-forward evaluation"})
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

	datasets := make(map[int]forecast.Dataset, len(request.Horizons))
	for _, horizon := range request.Horizons {
		dataset, buildErr := forecast.BuildDataset(features, targets, horizon)
		if buildErr != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Could not build evaluation dataset", "detail": buildErr.Error()})
			return
		}
		datasets[horizon] = dataset
	}

	config := forecast.WalkForwardConfig{
		InitialTrainSize: request.InitialTrainSize,
		MinimumTrainSize: request.MinimumTrainSize,
		TestSize:         request.TestSize,
		StepSize:         request.StepSize,
	}
	lambda := ridgeLambda
	factories := []forecast.ForecastModelFactory{
		{NewBaseline: func() forecast.BaselineModel { return &forecast.ZeroReturnBaseline{} }},
		{NewBaseline: func() forecast.BaselineModel { return &forecast.HistoricalMeanBaseline{} }},
		{NewFeature: func() forecast.FeatureForecastModel {
			model, _ := forecast.NewRidgeRegression(lambda)
			return model
		}},
	}
	report, err := forecast.RunForecastExperiment(datasets, config, factories)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Walk-forward evaluation could not be completed", "detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"company": gin.H{
			"id": companyID, "name": companyName, "symbol": symbol,
			"exchange": exchange, "currency": nullableString(currency),
		},
		"provider": selectedProvider,
		"data_type": "historical_eod_walk_forward_evaluation",
		"methodology": gin.H{
			"validation": "expanding_window_walk_forward",
			"training_labels": "purged_when_target_date_reaches_or_overlaps_test_period",
			"metrics": "out_of_sample_only",
			"models": []string{"zero_return", "historical_mean", "ridge_regression"},
		},
		"experiment": report,
		"notice": "Historical research evaluation only. Out-of-sample metrics describe this sample and do not guarantee future performance or establish an investment recommendation.",
	})
}
