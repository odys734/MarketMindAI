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

## Multi-horizon forecast experiments

`RunForecastExperiment` runs a unified model comparison across one or more
supported forecast horizons: 1, 5, 10 and 20 sessions.

The caller supplies:

- A `map[int]Dataset`, keyed by forecast horizon.
- One shared `WalkForwardConfig`.
- A list of `ForecastModelFactory` values.

Each factory must define exactly one constructor:

- `NewBaseline` for a `BaselineModel`.
- `NewFeature` for a `FeatureForecastModel`.

Factories create fresh model instances independently for each horizon. This
prevents fitted model state from leaking between horizon evaluations.

The runner:

1. Validates supported horizons and dataset metadata.
2. Rejects empty datasets, negative incomplete counts and invalid factories.
3. Evaluates supplied horizons in canonical order.
4. Calls `CompareForecastModels` separately for each horizon.
5. Records dataset diagnostics and comparison results in one report.

### Experiment report

`ForecastExperimentReport` contains:

- `schema_version`: report schema version (`1.0`).
- `generated_at`: UTC report generation timestamp.
- `configuration`: the walk-forward configuration used.
- `horizons`: one result per supplied horizon.

Each horizon result records its session horizon, first and last dataset dates,
row count, incomplete count and model comparison results.

Use `MarshalForecastExperimentReport` to serialize the report as indented
JSON. The function returns JSON bytes; persistence to a file or database is
the responsibility of the caller.

### HTTP evaluation endpoint

The experiment runner is exposed through `POST /api/v1/companies/:id/forecast/evaluate`.
The handler loads a provider-consistent EOD series from PostgreSQL, calculates
features and forward-return targets, builds the requested horizon datasets and
runs the shared model comparison.

The endpoint defaults to horizons 1, 5, 10 and 20; initial training size 80;
minimum post-purge training size 30; test size and step size 20; and Ridge
lambda 1.0. Training labels reaching or overlapping the test period are
purged, and Ridge preprocessing is fitted inside each training fold. The
response includes aligned out-of-sample observations, fold diagnostics, MAE,
RMSE, mean error and directional accuracy.

### Scope and limitations

The endpoint does not select a winning model, tune hyperparameters, persist
reports, schedule evaluations or deploy forecasts. Results depend on data
quality, date coverage, validation settings and market regime. The generation
timestamp differs between runs. Passing tests does not establish predictive
skill; review sample size, fold diagnostics and baseline comparisons before
interpreting metrics.
