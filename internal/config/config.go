package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	DiscordToken string
	DBHost       string
	DBPort       string
	DBUser       string
	DBPassword   string
	DBName       string
}

// Load reads configuration from .env file or environment variables
func Load() (*Config, error) {
	// First try to load .env file
	err := godotenv.Load()
	if err != nil {
		// It's ok if .env doesn't exist, we'll use environment variables
		fmt.Println("Info: .env file not found, using environment variables")
	}

	// Check required environment variables
	if os.Getenv("DISCORD_TOKEN") == "" {
		return nil, fmt.Errorf("DISCORD_TOKEN environment variable is required")
	}

	// Use environment variables with fallbacks
	config := &Config{
		DiscordToken: os.Getenv("DISCORD_TOKEN"),
		DBHost:       getEnvWithDefault("DB_HOST", "localhost"),
		DBPort:       getEnvWithDefault("DB_PORT", "5432"),
		DBUser:       getEnvWithDefault("DB_USER", "postgres"),
		DBPassword:   os.Getenv("DB_PASSWORD"),
		DBName:       getEnvWithDefault("DB_NAME", "lockinbot"),
	}

	// Additional validation
	if config.DBPassword == "" {
		return nil, fmt.Errorf("DB_PASSWORD environment variable is required")
	}

	return config, nil
}

// getEnvWithDefault returns environment variable or default if not set
func getEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
