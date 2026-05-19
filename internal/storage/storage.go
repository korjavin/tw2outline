package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/korjavin/tw2outline/internal/logger"
)

// Storage defines the interface for cache operations.
type Storage interface {
	MarkProcessed(tweetID string)
	IsProcessed(tweetID string) bool
	Save() error
}

// FileStorage implements the Storage interface using a local file.
type FileStorage struct {
	filePath        string
	logger          *logger.Logger
	processedTweets map[string]bool
	mu              sync.Mutex
}

// NewFileStorage initializes a new file-based storage or loads an existing one.
func NewFileStorage(filePath string, logger *logger.Logger) (*FileStorage, error) {
	storage := &FileStorage{
		filePath:        filePath,
		logger:          logger,
		processedTweets: make(map[string]bool),
	}

	// Create directory if it doesn't exist.
	dir := filepath.Dir(filePath)
	if dir != "." {
		logger.Debug("Creating cache directory: %s", dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create cache directory: %v", err)
		}
	}

	// Try to load existing cache.
	logger.Debug("Attempting to load cache from: %s", filePath)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Cache file doesn't exist, return empty cache.
			logger.Info("Cache file doesn't exist, creating new cache")
			return storage, nil
		}
		return nil, fmt.Errorf("failed to read cache file: %v", err)
	}

	// Parse cache data.
	logger.Debug("Parsing cache data")
	if err := json.Unmarshal(data, &storage.processedTweets); err != nil {
		// For backward compatibility with the old format, try to unmarshal into a struct
		var oldCache struct {
			ProcessedTweets map[string]bool `json:"processed_tweets"`
		}
		if err2 := json.Unmarshal(data, &oldCache); err2 == nil {
			storage.processedTweets = oldCache.ProcessedTweets
			logger.Info("Successfully converted old cache format")
		} else {
			return nil, fmt.Errorf("failed to parse cache file: %v", err)
		}
	}

	logger.Info("Cache loaded successfully with %d processed tweets", len(storage.processedTweets))
	return storage, nil
}

// Save persists the cache to disk.
func (s *FileStorage) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Debug("Marshaling cache data")
	data, err := json.MarshalIndent(s.processedTweets, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %v", err)
	}

	s.logger.Debug("Writing cache to file: %s", s.filePath)
	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %v", err)
	}

	s.logger.Info("Cache saved successfully with %d processed tweets", len(s.processedTweets))
	return nil
}

// MarkProcessed marks a tweet as processed.
func (s *FileStorage) MarkProcessed(tweetID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processedTweets[tweetID] = true
}

// IsProcessed checks if a tweet has been processed.
func (s *FileStorage) IsProcessed(tweetID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.processedTweets[tweetID]
}
