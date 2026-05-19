package ntfy

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/korjavin/tw2outline/internal/logger"
)

// Client defines the interface for sending notifications.
type Client interface {
	Send(message string, title string) error
}

type client struct {
	serverURL string
	topic     string
	username  string
	password  string
	logger    *logger.Logger
}

// NewClient creates a new ntfy notification client.
func NewClient(serverURL, topic, username, password string, logger *logger.Logger) Client {
	return &client{
		serverURL: serverURL,
		topic:     topic,
		username:  username,
		password:  password,
		logger:    logger,
	}
}

func (c *client) Send(message string, title string) error {
	if c.serverURL == "" {
		c.logger.Debug("ntfy server URL is not set, skipping notification")
		return nil
	}

	url := fmt.Sprintf("%s/%s", c.serverURL, c.topic)
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(message))
	if err != nil {
		return fmt.Errorf("failed to create ntfy request: %w", err)
	}

	req.Header.Set("Title", title)
	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send ntfy notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send ntfy notification, status code: %d", resp.StatusCode)
	}

	c.logger.Info("Sent ntfy notification to topic %s", c.topic)
	return nil
}
