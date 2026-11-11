package config

import "testing"

func clearConfigEnv(t *testing.T) {
	t.Helper()

	keys := []string{
		"HOST", "PORT", "APP_ENV", "DB_HOST", "DB_PORT",
		"DB_USER", "DB_NAME", "PREFIX", "USER",
		"AI_BASE_URL", "AI_MODEL", "AI_API_KEY",
		"AI_TIMEOUT_SECONDS",
	}

	for _, key := range keys {
		t.Setenv(key, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearConfigEnv(t)

	t.Setenv("PREFIX", "/test-prefix")
	t.Setenv("USER", "test-user")

	cfg := Load()

	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected localhost host, got %s", cfg.Host)
	}

	if cfg.Port != "8080" {
		t.Errorf("expected port 8080, got %s", cfg.Port)
	}

	if cfg.Environment != "development" {
		t.Errorf("unexpected environment: %s", cfg.Environment)
	}

	if cfg.DBHost != "/test-prefix/tmp" {
		t.Errorf("unexpected DB host: %s", cfg.DBHost)
	}

	if cfg.DBUser != "test-user" {
		t.Errorf("unexpected DB user: %s", cfg.DBUser)
	}

	if cfg.DBName != "marketmind" {
		t.Errorf("unexpected DB name: %s", cfg.DBName)
	}

	if cfg.AIBaseURL != "http://127.0.0.1:8081/v1" {
		t.Errorf("unexpected AI URL: %s", cfg.AIBaseURL)
	}

	if cfg.AIModel != "gemini-3.5-flash" {
		t.Errorf("unexpected AI model: %s", cfg.AIModel)
	}

	if cfg.AITimeout != 45 {
		t.Errorf("expected timeout 45, got %d", cfg.AITimeout)
	}
}

func TestLoadCustomValues(t *testing.T) {
	clearConfigEnv(t)

	t.Setenv("HOST", "0.0.0.0")
	t.Setenv("PORT", "9090")
	t.Setenv("APP_ENV", "test")
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "tester")
	t.Setenv("DB_NAME", "testdb")
	t.Setenv("AI_BASE_URL", "http://localhost:8081/v1")
	t.Setenv("AI_MODEL", "test-model")
	t.Setenv("AI_API_KEY", "test-key")
	t.Setenv("AI_TIMEOUT_SECONDS", "60")

	cfg := Load()

	if cfg.Host != "0.0.0.0" || cfg.Port != "9090" || cfg.Environment != "test" {
		t.Error("custom server configuration not loaded")
	}

	if cfg.DBHost != "localhost" || cfg.DBPort != "5433" ||
		cfg.DBUser != "tester" || cfg.DBName != "testdb" {
		t.Error("custom database configuration not loaded")
	}

	if cfg.AIBaseURL != "http://localhost:8081/v1" ||
		cfg.AIModel != "test-model" || cfg.AIAPIKey != "test-key" {
		t.Error("custom AI configuration not loaded")
	}

	if cfg.AITimeout != 60 {
		t.Errorf("expected timeout 60, got %d", cfg.AITimeout)
	}
}

func TestLoadInvalidTimeoutFallback(t *testing.T) {
	tests := []string{"invalid", "0", "-5"}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("AI_TIMEOUT_SECONDS", value)

			cfg := Load()

			if cfg.AITimeout != 45 {
				t.Errorf("expected fallback timeout 45, got %d", cfg.AITimeout)
			}
		})
	}
}
