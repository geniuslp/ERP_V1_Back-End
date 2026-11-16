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

	// Web Push (phone / desktop notifications). Empty keys = push disabled; the bell + WebSocket keep working.
	// Generate a pair once with `go run ./cmd/vapidkeys`; never change them later (every subscription would break).
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string // "mailto:..." or an https URL identifying the sender to the push service
	// PushAlways = send a push even when the user has the page open (normally skipped: the toast already shows it).
	// For testing push with one account only; keep it off in real use or the user gets every alert twice.
	PushAlways bool
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

		VAPIDPublicKey:  getEnv("VAPID_PUBLIC_KEY", ""),
		VAPIDPrivateKey: getEnv("VAPID_PRIVATE_KEY", ""),
		VAPIDSubject:    getEnv("VAPID_SUBJECT", "mailto:admin@example.com"),
		PushAlways:      getEnv("PUSH_ALWAYS", "") == "true",
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
