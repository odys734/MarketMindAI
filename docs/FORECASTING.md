# Forecasting Research

## Current capabilities

The `internal/forecast` package provides:

- Historical technical features and forward-return targets.
- Zero Return and Historical Mean baseline models.
- Ridge Regression using eight normalized technical features.
- Expanding-window walk-forward evaluation.
- Purging of training labels that overlap or reach the test period.
- Feature-based and baseline evaluation paths.
- Unified model comparison using identical dataset and walk-forward settings.

## Model comparison

`CompareForecastModels` accepts baseline models and feature-based models.

Each model is evaluated independently with the same:

- Dataset and forecast horizon.
- Initial training size.
- Minimum training size.
- Test window size and step size.

The comparison result contains each model's name, type, optional metadata, fold count, out-of-sample observations and evaluation metrics.

Before returning, the comparison engine verifies that models have matching forecast horizons, observation counts, test dates, horizons and actual returns. Duplicate model names and nil models are rejected.

## Metrics

The evaluation layer reports:

- Mean absolute error (MAE).
- Root mean squared error (RMSE).
- Mean forecast error.
- Directional accuracy.
- Brier score when probability forecasts are supplied.
- Prediction interval coverage and average width when intervals are supplied.

Metrics are calculated from out-of-sample observations only.

## Important limitations

A lower error metric in one historical sample does not prove that a model will perform better in future markets. Model comparisons depend on the selected period, data quality, horizon, features and validation configuration.

The current comparison engine does not automatically select or deploy a winning model. It does not establish calibrated probabilities or reliable prediction intervals unless a model explicitly supplies and validates them.

These models are experimental research tools, not personalized investment recommendations or guarantees of future performance.
