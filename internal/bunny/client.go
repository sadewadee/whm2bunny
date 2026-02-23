package bunny

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	goRetry "github.com/sethvargo/go-retry"
	"go.uber.org/zap"

	"github.com/mordenhost/whm2bunny/internal/retry"
)

const (
	// DefaultBaseURL is the default Bunny.net API base URL
	DefaultBaseURL = "https://api.bunny.net"
	// AccessKeyHeader is the header name for the API key
	AccessKeyHeader = "AccessKey"
	// DefaultTimeout is the default HTTP timeout
	DefaultTimeout = 30 * time.Second
)

// APIError represents an error response from the Bunny.net API
type APIError struct {
	StatusCode int
	Message    string
	Errors     []string `json:"Errors"`
}

// Error returns the error message
func (e *APIError) Error() string {
	if len(e.Errors) > 0 {
		return fmt.Sprintf("API error (status %d): %s - %v", e.StatusCode, e.Message, e.Errors)
	}
	return fmt.Sprintf("API error (status %d): %s", e.StatusCode, e.Message)
}

// IsNotFound returns true if the error is a 404 Not Found
func (e *APIError) IsNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}

// IsConflict returns true if the error is a 409 Conflict
func (e *APIError) IsConflict() bool {
	return e.StatusCode == http.StatusConflict
}

// IsBadRequest returns true if the error is a 400 Bad Request
func (e *APIError) IsBadRequest() bool {
	return e.StatusCode == http.StatusBadRequest
}

// Client is a Bunny.net API client
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
	retryCfg   *retry.Config
	backoff    goRetry.Backoff
}

// ClientOption is a function that configures a Client
type ClientOption func(*Client)

// WithBaseURL sets the base URL for the client
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// WithHTTPClient sets the HTTP client for the Client
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithLogger sets the logger for the client
func WithLogger(logger *zap.Logger) ClientOption {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithRetryConfig sets the retry configuration for the client
func WithRetryConfig(retryCfg *retry.Config) ClientOption {
	return func(c *Client) {
		c.retryCfg = retryCfg
		c.backoff = retry.WithBackoff(retryCfg)
	}
}

// NewClient creates a new Bunny.net API client
func NewClient(apiKey string, opts ...ClientOption) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: DefaultBaseURL,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		retryCfg: retry.DefaultConfig(),
		logger:   zap.NewNop(), // No-op logger by default
	}

	// Initialize backoff with default config
	c.backoff = retry.WithBackoff(c.retryCfg)

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// doRequest performs an HTTP request with retry logic
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	// Create a retry function that captures bodyReader properly
	// We need to recreate the reader on each retry
	retryFunc := func(ctx context.Context) error {
		// Recreate body reader if needed
		var currentBodyReader io.Reader = bodyReader
		if body != nil {
			jsonData, err := json.Marshal(body)
			if err != nil {
				return fmt.Errorf("failed to marshal request body: %w", err)
			}
			currentBodyReader = bytes.NewReader(jsonData)
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, currentBodyReader)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set(AccessKeyHeader, c.apiKey)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		c.logger.Debug("making API request",
			zap.String("method", method),
			zap.String("path", path),
		)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			c.logger.Warn("API request failed, will retry",
				zap.String("method", method),
				zap.String("path", path),
				zap.Error(err),
			)
			return err // Retry on network errors
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		// Log response for debugging
		c.logger.Debug("API response received",
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status", resp.StatusCode),
			zap.String("response", string(respBody)),
		)

		// Handle error responses
		if resp.StatusCode >= 400 {
			apiErr := &APIError{
				StatusCode: resp.StatusCode,
				Message:    http.StatusText(resp.StatusCode),
			}
			_ = json.Unmarshal(respBody, apiErr)

			// Don't retry on client errors (4xx) except 429 (rate limit)
			if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
				// Mark as non-retryable by wrapping with RetryableError
				// RetryableError actually marks an error as retryable, so for non-retryable
				// we return the error directly which will stop retries
				return apiErr
			}

			c.logger.Warn("API request returned error, will retry",
				zap.String("method", method),
				zap.String("path", path),
				zap.Int("status", resp.StatusCode),
				zap.Error(apiErr),
			)
			return goRetry.RetryableError(apiErr) // Mark as retryable
		}

		// Parse success response
		if result != nil {
			if err := json.Unmarshal(respBody, result); err != nil {
				return fmt.Errorf("failed to unmarshal response: %w", err)
			}
		}

		return nil // Success, no more retries
	}

	return goRetry.Do(ctx, c.backoff, retryFunc)
}

// get performs a GET request
func (c *Client) get(ctx context.Context, path string, result interface{}) error {
	return c.doRequest(ctx, http.MethodGet, path, nil, result)
}

// post performs a POST request
func (c *Client) post(ctx context.Context, path string, body, result interface{}) error {
	return c.doRequest(ctx, http.MethodPost, path, body, result)
}

// delete performs a DELETE request
func (c *Client) delete(ctx context.Context, path string) error {
	return c.doRequest(ctx, http.MethodDelete, path, nil, nil)
}
