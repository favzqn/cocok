package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL   string
	RedisAddr     string
	ServerAddr    string
	SessionCookie bool // secure
	SessionTTL    time.Duration
	LogLevel      string
	WebOrigin     string
}

func Load() Config {
	return Config{
		DatabaseURL:   env("DATABASE_URL", "postgres://cocok:cocok_local@localhost:5432/cocok?sslmode=disable"),
		RedisAddr:     env("REDIS_ADDR", "localhost:6379"),
		ServerAddr:    env("SERVER_ADDR", ":8080"),
		SessionCookie: envBool("SESSION_COOKIE_SECURE", false),
		SessionTTL:    time.Duration(envInt("SESSION_TTL_HOURS", 720)) * time.Hour,
		LogLevel:      env("LOG_LEVEL", "info"),
		WebOrigin:     env("WEB_ORIGIN", "http://localhost:3000"),
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return def
}
