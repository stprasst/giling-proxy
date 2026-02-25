package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds application configuration
type Config struct {
	// Server
	Port string

	// Database
	DBPath string

	// Authentication
	SessionSecret string
	AdminPassword string

	// Proxy Checker
	CheckInterval time.Duration
	WorkerCount   int
	CheckTimeout  time.Duration
	TestURLs      []string
}

// Load reads configuration from .env file and environment variables
func Load() (*Config, error) {
	// Load .env file if exists
	if err := loadEnvFile(".env"); err != nil {
		// Non-fatal, just log and continue with env vars
		fmt.Printf("Note: %v (using environment variables)\n", err)
	}

	cfg := &Config{
		Port:          getEnv("PORT", "8080"),
		DBPath:        getEnv("DB_PATH", "data/proxy.db"),
		SessionSecret: getEnv("SESSION_SECRET", ""),
		AdminPassword: getEnv("ADMIN_PASSWORD", ""),
	}

	// Parse check interval
	checkIntervalStr := getEnv("CHECK_INTERVAL", "20m")
	checkInterval, err := time.ParseDuration(checkIntervalStr)
	if err != nil {
		return nil, fmt.Errorf("invalid CHECK_INTERVAL: %w", err)
	}
	cfg.CheckInterval = checkInterval

	// Parse worker count
	workerCount, err := strconv.Atoi(getEnv("WORKER_COUNT", "100"))
	if err != nil {
		return nil, fmt.Errorf("invalid WORKER_COUNT: %w", err)
	}
	cfg.WorkerCount = workerCount

	// Parse check timeout
	checkTimeoutStr := getEnv("CHECK_TIMEOUT", "10s")
	checkTimeout, err := time.ParseDuration(checkTimeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid CHECK_TIMEOUT: %w", err)
	}
	cfg.CheckTimeout = checkTimeout

	// Parse test URLs
	testURLsStr := getEnv("TEST_URLS", "http://httpbin.org/ip,https://api.ipify.org?format=json")
	cfg.TestURLs = strings.Split(testURLsStr, ",")

	// Validation
	if cfg.SessionSecret == "" {
		return nil, fmt.Errorf("SESSION_SECRET is required")
	}
	if len(cfg.SessionSecret) < 32 {
		return nil, fmt.Errorf("SESSION_SECRET must be at least 32 characters")
	}
	if cfg.AdminPassword == "" {
		return nil, fmt.Errorf("ADMIN_PASSWORD is required")
	}
	if len(cfg.AdminPassword) < 12 {
		return nil, fmt.Errorf("ADMIN_PASSWORD must be at least 12 characters")
	}

	return cfg, nil
}

// getEnv returns environment variable value or default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// loadEnvFile loads environment variables from a .env file
func loadEnvFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", filename, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
			(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
			value = value[1 : len(value)-1]
		}

		// Only set if not already in environment
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}

	return scanner.Err()
}
