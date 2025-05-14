package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	DiscordToken     string
	DBHost           string
	DBPort           string
	DBUser           string
	DBPassword       string
	DBName           string
	LoggingChannelID string
	TestGuildID      string
	// New fields for allowed voice channels
	AllowedVoiceChannelIDsRaw string
	AllowedVoiceChannelIDsMap map[string]struct{}
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
		DiscordToken:              os.Getenv("DISCORD_TOKEN"),
		DBHost:                    getEnvWithDefault("DB_HOST", "localhost"),
		DBPort:                    getEnvWithDefault("DB_PORT", "5432"),
		DBUser:                    getEnvWithDefault("DB_USER", "postgres"),
		DBPassword:                os.Getenv("DB_PASSWORD"),
		DBName:                    getEnvWithDefault("DB_NAME", "lockinbot"),
		LoggingChannelID:          os.Getenv("LOGGING_CHANNEL_ID"),
		TestGuildID:               os.Getenv("TEST_GUILD_ID"),
		AllowedVoiceChannelIDsRaw: os.Getenv("ALLOWED_VOICE_CHANNEL_IDS"),
	}

	config.AllowedVoiceChannelIDsMap = parseChannelIDs(config.AllowedVoiceChannelIDsRaw)

	// Additional validation
	if config.DBPassword == "" {
		return nil, fmt.Errorf("DB_PASSWORD environment variable is required")
	}

	if config.LoggingChannelID == "" {
		fmt.Println("Info: LOGGING_CHANNEL_ID environment variable is not set. Study time announcements will be disabled.")
	}

	if config.AllowedVoiceChannelIDsRaw != "" && len(config.AllowedVoiceChannelIDsMap) == 0 {
		fmt.Printf("Info: ALLOWED_VOICE_CHANNEL_IDS was set to '%s' but resulted in no valid channel IDs. No voice channels will be specifically tracked for study time.\n", config.AllowedVoiceChannelIDsRaw)
	} else if len(config.AllowedVoiceChannelIDsMap) > 0 {
		fmt.Printf("Info: Bot will only track study time in the following voice channels: %v\n", getKeysFromMap(config.AllowedVoiceChannelIDsMap))
	}

	return config, nil
}

func parseChannelIDs(rawIDs string) map[string]struct{} {
	idsMap := make(map[string]struct{})
	if rawIDs == "" {
		return idsMap
	}
	ids := strings.Split(rawIDs, ",")
	for _, id := range ids {
		trimmedID := strings.TrimSpace(id)
		if trimmedID != "" {
			idsMap[trimmedID] = struct{}{}
		}
	}
	return idsMap
}

// Helper function to get keys from map for logging (optional, can be inlined if preferred)
func getKeysFromMap(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// getEnvWithDefault returns environment variable or default if not set
func getEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
