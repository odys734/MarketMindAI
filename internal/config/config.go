package config

import (
	"os"
	"strconv"
)

type Config struct {
	Host        string
	Port        string
	Environment string
	DBHost      string
	DBPort      string
	DBUser      string
	DBName      string

	AIBaseURL string
	AIModel   string
	AIAPIKey  string
	AITimeout int
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func Load() Config {
	timeout, err := strconv.Atoi(env("AI_TIMEOUT_SECONDS", "45"))
	if err != nil || timeout < 1 {
		timeout = 45
	}

	return Config{
		Host:        env("HOST", "127.0.0.1"),
		Port:        env("PORT", "8080"),
		Environment: env("APP_ENV", "development"),
		DBHost:      env("DB_HOST", os.Getenv("PREFIX")+"/tmp"),
		DBPort:      env("DB_PORT", "5432"),
		DBUser:      env("DB_USER", os.Getenv("USER")),
		DBName:      env("DB_NAME", "marketmind"),

		AIBaseURL: env("AI_BASE_URL", "http://127.0.0.1:8081/v1"),
		AIModel:   env("AI_MODEL", "gemini-3.5-flash"),
		AIAPIKey:  os.Getenv("AI_API_KEY"),
		AITimeout: timeout,
	}
}
