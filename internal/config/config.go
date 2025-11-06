package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds application configuration sourced from environment variables.
type Config struct {
	Server ServerConfig
	Redis  RedisConfig
	Log    LogConfig
}

// LogConfig contains logging settings
type LogConfig struct {
	Level  string // debug, info, warn, error
	Format string // json, console
}

// ServerConfig contains HTTP server settings
type ServerConfig struct {
	Host         string
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// RedisConfig contains Redis connection settings
type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
	PoolSize int
}

// Load reads environment variables into Config. It expects godotenv to have been
// executed by the caller when needed (e.g. in development).
func Load() Config {
	sever := ServerConfig{
		Host:         getEnv("APP_HOST", "0.0.0.0"),
		Port:         getEnvAsInt("APP_PORT", 3000),
		ReadTimeout:  getEnvAsDuration("APP_READ_TIMEOUT", 10*time.Second),
		WriteTimeout: getEnvAsDuration("APP_WRITE_TIMEOUT", 10*time.Second),
		IdleTimeout:  getEnvAsDuration("APP_IDLE_TIMEOUT", 10*time.Second),
	}

	redis := RedisConfig{
		Host:     getEnv("REDIS_HOST", "localhost"),
		Port:     getEnvAsInt("REDIS_PORT", 6379),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       getEnvAsInt("REDIS_DB", 0),
	}

	log := LogConfig{
		Level:  getEnv("LOG_LEVEL", "debug"),
		Format: getEnv("LOG_FORMAT", "console"),
	}

	cfg := Config{
		Server: sever,
		Redis:  redis,
		Log:    log,
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func getEnvAsDuration(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}

	dur, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return dur
}

func LoadDotEnv() {
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			log.Printf("warning: could not load .env: %v", err)
		}
	}
}

// RedisAddr returns the Redis address in host:port format
func (c *Config) RedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Redis.Host, c.Redis.Port)
}

// ServerAddr returns the server address in host:port format
func (c *Config) ServerAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}
