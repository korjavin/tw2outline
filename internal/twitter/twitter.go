package twitter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/korjavin/tw2outline/internal/auth"
	"github.com/korjavin/tw2outline/internal/config"
	"github.com/korjavin/tw2outline/internal/logger"
	"github.com/korjavin/tw2outline/internal/storage"

	twitterv2 "github.com/g8rswimmer/go-twitter/v2"
	"golang.org/x/oauth2"
)

// ErrRateLimit signals that Twitter rejected a request with 429.
// Callers should stop further calls to the same endpoint for the
// current cycle rather than burning more quota.
var ErrRateLimit = errors.New("twitter API rate limit exceeded")

// Tweet represents a simplified tweet structure.
type Tweet struct {
	ID   string
	Text string
	URL  string
}

// Client defines the interface for interacting with the Twitter API.
type Client interface {
	GetBookmarks() ([]Tweet, error)
	RemoveBookmark(tweetID string) error
	CleanupProcessedBookmarks(storage storage.Storage) error
}

// APIClient implements the Client interface for the Twitter API.
type APIClient struct {
	client       *twitterv2.Client
	userID       string
	logger       *logger.Logger
	config       *config.Config
	oauth2Config *oauth2.Config
	token        *oauth2.Token
}

// NewClient creates a new Twitter API client.
func NewClient(cfg *config.Config, logger *logger.Logger, mux *http.ServeMux) (*APIClient, error) {
	logger.Debug("Creating OAuth2 configuration")

	oauth2Config := &oauth2.Config{
		ClientID:     cfg.TwitterClientID,
		ClientSecret: cfg.TwitterClientSecret,
		RedirectURL:  cfg.TwitterRedirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://twitter.com/i/oauth2/authorize",
			TokenURL: "https://api.twitter.com/2/oauth2/token",
		},
		Scopes: []string{"tweet.read", "users.read", "bookmark.read", "bookmark.write", "offline.access"},
	}

	logger.Debug("OAuth2 redirect URL: %s", cfg.TwitterRedirectURL)

	var token *oauth2.Token
	var userID string
	var err error

	if _, statErr := os.Stat(cfg.TokenFilePath); os.IsNotExist(statErr) {
		logger.Info("No token file found at %s", cfg.TokenFilePath)

		codeVerifier, err := auth.GenerateCodeVerifier()
		if err != nil {
			return nil, fmt.Errorf("failed to generate code verifier: %v", err)
		}
		codeChallenge := auth.GenerateCodeChallenge(codeVerifier)

		codeChan := make(chan string, 1)
		stateChan := make(chan string, 1)
		errorChan := make(chan error, 1)

		setupCallbackHandler(mux, codeChan, stateChan, errorChan, logger)

		authURL := auth.GetAuthURL(oauth2Config, "state", codeChallenge)
		logger.Info("Please visit the following URL to authorize this application:")
		logger.Info("%s", authURL)

		var code string
		select {
		case code = <-codeChan:
			logger.Debug("Received authorization code from callback")
		case err := <-errorChan:
			return nil, fmt.Errorf("OAuth callback error: %v", err)
		case <-time.After(30 * time.Minute):
			return nil, fmt.Errorf("OAuth authorization timeout after 30 minutes")
		}

		token, err = auth.ExchangeToken(oauth2Config, code, codeVerifier)
		if err != nil {
			return nil, fmt.Errorf("failed to exchange token: %v", err)
		}

		if err := auth.SaveToken(cfg.TokenFilePath, token, ""); err != nil {
			logger.Warn("Failed to save token: %v", err)
		}
	} else {
		token, userID, err = auth.LoadToken(cfg.TokenFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load token: %v", err)
		}
		logger.Debug("Loaded token with userID: %s", userID)
	}

	authorizer := auth.NewAuthorizer(token)

	client := &twitterv2.Client{
		Authorizer: authorizer,
		Client:     &http.Client{Timeout: 10 * time.Second},
		Host:       "https://api.twitter.com",
	}

	if userID == "" || userID == "me" {
		logger.Info("Need to lookup actual user ID for bookmarks API")
		userOpts := twitterv2.UserLookupOpts{
			UserFields: []twitterv2.UserField{twitterv2.UserFieldID},
		}
		cleanUsername := strings.TrimPrefix(cfg.TwitterUsername, "@")
		userResponse, err := client.UserNameLookup(context.Background(), []string{cleanUsername}, userOpts)
		if err != nil {
			return nil, fmt.Errorf("cannot get user ID for bookmarks API: %v", err)
		}
		if userResponse.Raw == nil || len(userResponse.Raw.Users) == 0 || userResponse.Raw.Users[0] == nil {
			return nil, fmt.Errorf("failed to get user information for bookmarks API")
		}
		userID = userResponse.Raw.Users[0].ID
		logger.Info("Successfully retrieved user ID: %s", userID)
		if err := auth.SaveToken(cfg.TokenFilePath, token, userID); err != nil {
			logger.Warn("Failed to save token with user info: %v", err)
		}
	} else {
		logger.Info("Using cached user ID: %s", userID)
	}

	apiClient := &APIClient{
		client:       client,
		userID:       userID,
		logger:       logger,
		config:       cfg,
		oauth2Config: oauth2Config,
		token:        token,
	}
	return apiClient, nil
}

func (c *APIClient) refreshToken() error {
	c.logger.Info("Attempting to refresh token")
	if c.token.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	newToken, err := c.oauth2Config.TokenSource(context.Background(), c.token).Token()
	if err != nil {
		return fmt.Errorf("failed to refresh token: %v", err)
	}

	c.logger.Info("Token refreshed successfully")
	if err := auth.SaveToken(c.config.TokenFilePath, newToken, c.userID); err != nil {
		c.logger.Warn("Failed to save refreshed token: %v", err)
	}

	c.token = newToken
	c.client.Authorizer = auth.NewAuthorizer(c.token)
	return nil
}

// GetBookmarks retrieves bookmarked tweets for the authenticated user.
func (c *APIClient) GetBookmarks() ([]Tweet, error) {
	c.logger.Info("Fetching bookmarks for user ID: %s", c.userID)
	opts := twitterv2.TweetBookmarksLookupOpts{
		MaxResults: 100,
		TweetFields: []twitterv2.TweetField{
			twitterv2.TweetFieldID,
			twitterv2.TweetFieldText,
			twitterv2.TweetFieldAuthorID,
			twitterv2.TweetFieldCreatedAt,
		},
		UserFields: []twitterv2.UserField{
			twitterv2.UserFieldID,
			twitterv2.UserFieldName,
			twitterv2.UserFieldUserName,
		},
		Expansions: []twitterv2.Expansion{
			twitterv2.ExpansionAuthorID,
		},
	}

	bookmarksResponse, err := c.client.TweetBookmarksLookup(context.Background(), c.userID, opts)
	if err != nil {
		if strings.Contains(err.Error(), "401") {
			c.logger.Warn("Received 401 Unauthorized, attempting to refresh token")
			if err := c.refreshToken(); err != nil {
				c.logger.Error("Failed to refresh token: %v", err)
				if _, statErr := os.Stat(c.config.TokenFilePath); !os.IsNotExist(statErr) {
					if removeErr := os.Remove(c.config.TokenFilePath); removeErr != nil {
						c.logger.Error("Failed to remove token file: %v", removeErr)
					} else {
						c.logger.Info("Removed token file, please re-authenticate")
					}
				}
				return nil, fmt.Errorf("failed to refresh token, re-authentication required: %v", err)
			}
			c.logger.Info("Retrying to get bookmarks after token refresh")
			bookmarksResponse, err = c.client.TweetBookmarksLookup(context.Background(), c.userID, opts)
		}
		if err != nil {
			var twitterErr *twitterv2.ErrorResponse
			if errors.As(err, &twitterErr) {
				if twitterErr.StatusCode == 429 {
					c.logger.Warn("Twitter API rate limit hit on bookmarks endpoint. Original message: %s", twitterErr.Detail)
					return []Tweet{}, nil
				}
			}
			return nil, fmt.Errorf("failed to get bookmarks: %v", err)
		}
	}

	if bookmarksResponse.Raw == nil || len(bookmarksResponse.Raw.Tweets) == 0 {
		c.logger.Info("Found 0 bookmarked tweets")
		return []Tweet{}, nil
	}

	c.logger.Info("Found %d bookmarks", len(bookmarksResponse.Raw.Tweets))

	authorMap := make(map[string]string)
	if bookmarksResponse.Raw.Includes != nil {
		for _, user := range bookmarksResponse.Raw.Includes.Users {
			authorMap[user.ID] = user.UserName
		}
	}

	var tweets []Tweet
	for _, tweet := range bookmarksResponse.Raw.Tweets {
		username := "user"
		if authorUsername, ok := authorMap[tweet.AuthorID]; ok {
			username = authorUsername
		}
		tweetURL := fmt.Sprintf("https://twitter.com/%s/status/%s", username, tweet.ID)
		tweets = append(tweets, Tweet{
			ID:   tweet.ID,
			Text: tweet.Text,
			URL:  tweetURL,
		})
	}
	return tweets, nil
}

// RemoveBookmark removes a tweet from bookmarks.
func (c *APIClient) RemoveBookmark(tweetID string) error {
	c.logger.Debug("Attempting to remove bookmark for tweet ID: %s", tweetID)
	url := fmt.Sprintf("%s/2/users/%s/bookmarks/%s", c.client.Host, c.userID, tweetID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create remove bookmark request: %v", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token.AccessToken))

	resp, err := c.client.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send remove bookmark request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		c.logger.Warn("Received 401 Unauthorized, attempting to refresh token")
		if err := c.refreshToken(); err != nil {
			return fmt.Errorf("failed to refresh token during bookmark removal: %v", err)
		}

		c.logger.Info("Retrying to remove bookmark after token refresh")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token.AccessToken))
		resp, err = c.client.Client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send remove bookmark request on retry: %v", err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		c.logger.Debug("Successfully removed bookmark for tweet %s", tweetID)
		return nil
	}
	if resp.StatusCode == 404 {
		c.logger.Debug("Bookmark for tweet %s was not found", tweetID)
		return nil
	}
	if resp.StatusCode == 403 {
		return fmt.Errorf("insufficient permissions for bookmark removal - re-authorization required")
	}
	if resp.StatusCode == 429 {
		return ErrRateLimit
	}

	return fmt.Errorf("failed to remove bookmark (HTTP %d)", resp.StatusCode)
}

// CleanupProcessedBookmarks removes all bookmarks that have already been processed.
func (c *APIClient) CleanupProcessedBookmarks(storage storage.Storage) error {
	c.logger.Info("Starting cleanup of processed bookmarks")
	tweets, err := c.GetBookmarks()
	if err != nil {
		return fmt.Errorf("failed to get bookmarks for cleanup: %v", err)
	}

	if len(tweets) == 0 {
		c.logger.Info("No bookmarks found for cleanup")
		return nil
	}

	var removed, failed int
	for _, tweet := range tweets {
		if storage.IsProcessed(tweet.ID) {
			if err := c.RemoveBookmark(tweet.ID); err != nil {
				if errors.Is(err, ErrRateLimit) {
					c.logger.Warn("Rate limit hit during cleanup after %d removals; will resume next cycle", removed)
					break
				}
				c.logger.Warn("Failed to remove processed bookmark %s: %v", tweet.ID, err)
				failed++
				continue
			}
			removed++
			time.Sleep(500 * time.Millisecond)
		}
	}
	c.logger.Info("Cleanup complete. Removed %d processed bookmarks, failed: %d", removed, failed)
	if failed > 0 {
		return fmt.Errorf("cleanup completed with %d failures", failed)
	}
	return nil
}

// setupCallbackHandler sets up the OAuth callback endpoint.
func setupCallbackHandler(mux *http.ServeMux, codeChan chan string, stateChan chan string, errorChan chan error, logger *logger.Logger) {
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		code := query.Get("code")
		state := query.Get("state")
		errorParam := query.Get("error")

		if errorParam != "" {
			errorMsg := fmt.Sprintf("OAuth Error: %s", errorParam)
			http.Error(w, errorMsg, http.StatusBadRequest)
			errorChan <- fmt.Errorf(errorMsg)
			return
		}

		if code == "" {
			http.Error(w, "Authorization code not found", http.StatusBadRequest)
			errorChan <- fmt.Errorf("authorization code not found")
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Authorization Successful! You can close this window."))
		codeChan <- code
		stateChan <- state
	})
}
