package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds application configuration
type Config struct {
	// Database
	DatabaseURL string

	// API
	APIPort string

	// Worker
	MaxWorkers           int
	RateLimitPerSec      float64
	DomainRateLimitPerSec float64
	PollInterval         time.Duration
	BatchSize            int

	// HTTP Client
	RequestTimeout  time.Duration
	MaxRedirects    int
	MaxResponseSize int64
	UserAgent       string

	// Retry
	MaxRetryAttempts int
	RetryBaseDelay   time.Duration
	RetryMaxDelay    time.Duration
}

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		// Database
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/urlcollector?sslmode=disable"),

		// API
		APIPort: getEnv("API_PORT", "8080"),

		// Worker
		MaxWorkers:            getEnvInt("MAX_WORKERS", 10),
		RateLimitPerSec:       getEnvFloat("RATE_LIMIT_PER_SEC", 5.0),
		DomainRateLimitPerSec: getEnvFloat("DOMAIN_RATE_LIMIT_PER_SEC", 2.0),
		PollInterval:          getEnvDuration("POLL_INTERVAL", 5*time.Second),
		BatchSize:             getEnvInt("BATCH_SIZE", 100),

		// HTTP Client
		RequestTimeout:  getEnvDuration("REQUEST_TIMEOUT", 8*time.Second),
		MaxRedirects:    getEnvInt("MAX_REDIRECTS", 5),
		MaxResponseSize: getEnvInt64("MAX_RESPONSE_SIZE", 1*1024*1024),
		UserAgent:       getEnv("USER_AGENT", "InternInc-MetadataCollector/0.1"),

		// Retry
		MaxRetryAttempts: getEnvInt("MAX_RETRY_ATTEMPTS", 3),
		RetryBaseDelay:   getEnvDuration("RETRY_BASE_DELAY", 10*time.Second),
		RetryMaxDelay:    getEnvDuration("RETRY_MAX_DELAY", 2*time.Minute),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
