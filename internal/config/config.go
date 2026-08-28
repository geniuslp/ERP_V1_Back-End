package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port            string
	DatabaseURL     string
	JWTSecret       string
	JWTExpiry       time.Duration
	RefreshExpiry   time.Duration
	CORSOrigins     string
	Environment     string
	BcryptCost      int
	PublicBaseURL   string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, using environment variables")
	}

	return &Config{
		Port:          getEnv("PORT", "8080"),
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://postgres:root@localhost:5432/postgres?sslmode=disable"),
		JWTSecret:     getEnv("JWT_SECRET", "change-me-in-production-use-32chars!!"),
		JWTExpiry:     getDuration("JWT_EXPIRY_HOURS", 8) * time.Hour,
		RefreshExpiry: getDuration("JWT_REFRESH_HOURS", 168) * time.Hour, // 7 days
		CORSOrigins:   getEnv("CORS_ORIGINS", "*"),
		Environment:   getEnv("APP_ENV", "development"),
		BcryptCost:    getInt("BCRYPT_COST", 12),
		PublicBaseURL: strings.TrimRight(getEnv("PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return time.Duration(i)
		}
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
