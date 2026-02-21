package bunny

import (
	_ "github.com/google/uuid"
	_ "go.uber.org/zap"
)
type Client struct {
	apiKey string
}

// NewClient creates a new Bunny.net API client
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
	}
}
