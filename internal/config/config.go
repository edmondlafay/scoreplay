package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL     string
	Port            string
	UploadDir       string
	BaseURL         string
	DBMaxOpenConns  int
	DBMaxIdleConns  int
	TraceSampleRate float64
}

func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		host    := getEnvOrDefault("DB_HOST",     "localhost")
		port    := getEnvOrDefault("DB_PORT",     "5432")
		user    := getEnvOrDefault("DB_USER",     "scoreplay")
		pass    := getEnvOrDefault("DB_PASSWORD", "scoreplay")
		name    := getEnvOrDefault("DB_NAME",     "scoreplay")
		sslMode := getEnvOrDefault("DB_SSL_MODE", "disable") // use "require" in production
		dbURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, name, sslMode)
	}

	maxOpen, _ := strconv.Atoi(getEnvOrDefault("DB_MAX_OPEN_CONNS", "25"))
	maxIdle, _  := strconv.Atoi(getEnvOrDefault("DB_MAX_IDLE_CONNS", "5"))
	sampleRate, _ := strconv.ParseFloat(getEnvOrDefault("TRACE_SAMPLE_RATE", "1.0"), 64)

	return &Config{
		DatabaseURL:     dbURL,
		Port:            getEnvOrDefault("PORT",       "8080"),
		UploadDir:       getEnvOrDefault("UPLOAD_DIR", "./uploads"),
		BaseURL:         getEnvOrDefault("BASE_URL",   "http://localhost:8080"),
		DBMaxOpenConns:  maxOpen,
		DBMaxIdleConns:  maxIdle,
		TraceSampleRate: sampleRate,
	}, nil
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
