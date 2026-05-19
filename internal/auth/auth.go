package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
)

// Token represents an OAuth2 token with user info.
type Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
	UserID       string    `json:"user_id,omitempty"`
}

// TokenProvider is an interface for types that can provide an access token.
type TokenProvider interface {
	Token() *oauth2.Token
}

// Authorizer implements the Authorizer interface for OAuth2.
type Authorizer struct {
	token *oauth2.Token
}

// NewAuthorizer creates a new Authorizer.
func NewAuthorizer(token *oauth2.Token) *Authorizer {
	return &Authorizer{token: token}
}

// Add adds the OAuth2 authorization to the request.
func (a *Authorizer) Add(req *http.Request) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.token.AccessToken))
}

// Token returns the authorizer's token.
func (a *Authorizer) Token() *oauth2.Token {
	return a.token
}

// SaveToken saves the OAuth2 token with user ID to a file.
func SaveToken(filePath string, token *oauth2.Token, userID string) error {
	tokenData := Token{
		AccessToken:  token.AccessToken,
		TokenType:    token.TokenType,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
		UserID:       userID,
	}

	data, err := json.MarshalIndent(tokenData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal token: %v", err)
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write token file: %v", err)
	}

	return nil
}

// LoadToken loads the OAuth2 token and user ID from a file.
func LoadToken(filePath string) (*oauth2.Token, string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read token file: %v", err)
	}

	var tokenData Token
	if err := json.Unmarshal(data, &tokenData); err != nil {
		return nil, "", fmt.Errorf("failed to parse token file: %v", err)
	}

	token := &oauth2.Token{
		AccessToken:  tokenData.AccessToken,
		TokenType:    tokenData.TokenType,
		RefreshToken: tokenData.RefreshToken,
		Expiry:       tokenData.Expiry,
	}

	return token, tokenData.UserID, nil
}

// GenerateCodeVerifier creates a code verifier for PKCE.
func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// GenerateCodeChallenge creates a code challenge from a code verifier.
func GenerateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// GetAuthURL returns the URL to redirect the user to for OAuth2 authentication with PKCE.
func GetAuthURL(config *oauth2.Config, state string, codeChallenge string) string {
	opts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	}
	return config.AuthCodeURL(state, opts...)
}

// ExchangeToken exchanges an authorization code for an OAuth2 token with PKCE.
func ExchangeToken(config *oauth2.Config, code string, codeVerifier string) (*oauth2.Token, error) {
	ctx := context.Background()
	opts := []oauth2.AuthCodeOption{
		oauth2.SetAuthURLParam("code_verifier", codeVerifier),
	}
	return config.Exchange(ctx, code, opts...)
}
