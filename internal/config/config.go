package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds all configuration for the application.
type Config struct {
	// Outline settings
	OutlineURL          string
	OutlineToken        string
	OutlineCollectionID string

	// Twitter OAuth2 settings
	TwitterClientID     string
	TwitterClientSecret string
	TwitterRedirectURL  string
	TwitterUsername     string

	// Persistence
	CacheFilePath string
	TokenFilePath string

	// Behaviour
	CheckInterval             time.Duration
	RemoveBookmarks           bool
	CleanupProcessedBookmarks bool

	// HTTP server
	CallbackPort string

	// Logging
	LogLevel string

	// ntfy notifications
	NtfyServer   string
	NtfyTopic    string
	NtfyUsername string
	NtfyPassword string
}

// Load reads configuration from environment variables and returns a Config struct.
func Load() (*Config, error) {
	outlineURL := os.Getenv("OUTLINE_URL")
	if outlineURL == "" {
		return nil, fmt.Errorf("OUTLINE_URL environment variable is required")
	}

	outlineToken := os.Getenv("OUTLINE_API_TOKEN")
	if outlineToken == "" {
		return nil, fmt.Errorf("OUTLINE_API_TOKEN environment variable is required")
	}

	outlineCollectionID := os.Getenv("OUTLINE_COLLECTION_ID")
	if outlineCollectionID == "" {
		return nil, fmt.Errorf("OUTLINE_COLLECTION_ID environment variable is required")
	}

	twitterClientID := os.Getenv("TWITTER_CLIENT_ID")
	if twitterClientID == "" {
		return nil, fmt.Errorf("TWITTER_CLIENT_ID environment variable is required")
	}

	twitterClientSecret := os.Getenv("TWITTER_CLIENT_SECRET")
	if twitterClientSecret == "" {
		return nil, fmt.Errorf("TWITTER_CLIENT_SECRET environment variable is required")
	}

	twitterRedirectURL := os.Getenv("TWITTER_REDIRECT_URL")
	if twitterRedirectURL == "" {
		twitterRedirectURL = "http://localhost:8080/callback"
	}

	twitterUsername := os.Getenv("TW_USER")
	if twitterUsername == "" {
		return nil, fmt.Errorf("TW_USER environment variable is required")
	}

	cacheFilePath := os.Getenv("CACHE_FILE_PATH")
	if cacheFilePath == "" {
		cacheFilePath = "cache.json"
	}

	tokenFilePath := os.Getenv("TOKEN_FILE_PATH")
	if tokenFilePath == "" {
		tokenFilePath = "token.json"
	}

	checkIntervalStr := os.Getenv("CHECK_INTERVAL")
	var checkInterval time.Duration
	if checkIntervalStr == "" {
		checkInterval = 1 * time.Hour
	} else {
		var err error
		checkInterval, err = time.ParseDuration(checkIntervalStr)
		if err != nil {
			return nil, fmt.Errorf("invalid CHECK_INTERVAL format: %v", err)
		}
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "INFO"
	}

	removeBookmarks := os.Getenv("REMOVE_BOOKMARKS") == "true"
	cleanupProcessedBookmarks := os.Getenv("CLEANUP_PROCESSED_BOOKMARKS") == "true"

	callbackPort := os.Getenv("CALLBACK_PORT")
	if callbackPort == "" {
		callbackPort = "8080"
	}

	ntfyServer := os.Getenv("NTFY_SERVER")
	if ntfyServer == "" {
		ntfyServer = "http://ntfy:80"
	}
	ntfyTopic := os.Getenv("NTFY_TOPIC")
	if ntfyTopic == "" {
		ntfyTopic = "tw2outline"
	}

	ntfyUsername := os.Getenv("NTFY_USERNAME")
	ntfyPassword := os.Getenv("NTFY_PASSWORD")

	return &Config{
		OutlineURL:                outlineURL,
		OutlineToken:              outlineToken,
		OutlineCollectionID:       outlineCollectionID,
		TwitterClientID:           twitterClientID,
		TwitterClientSecret:       twitterClientSecret,
		TwitterRedirectURL:        twitterRedirectURL,
		TwitterUsername:           twitterUsername,
		CacheFilePath:             cacheFilePath,
		TokenFilePath:             tokenFilePath,
		CheckInterval:             checkInterval,
		LogLevel:                  logLevel,
		RemoveBookmarks:           removeBookmarks,
		CleanupProcessedBookmarks: cleanupProcessedBookmarks,
		CallbackPort:              callbackPort,
		NtfyServer:                ntfyServer,
		NtfyTopic:                 ntfyTopic,
		NtfyUsername:              ntfyUsername,
		NtfyPassword:              ntfyPassword,
	}, nil
}
