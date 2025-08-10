package config

import (
	"os"

	"github.com/nats-io/nats.go"
)

type Config struct {
	NATSURL    string
	LogLevel   string
	InstanceID string
}

func New() *Config {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "nats-cron"
	}

	return &Config{
		NATSURL:    getEnv("NATS_URL", nats.DefaultURL),
		LogLevel:   getEnv("LOG_LEVEL", "info"),
		InstanceID: getEnv("INSTANCE_ID", hostname),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}