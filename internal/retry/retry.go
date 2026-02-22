package retry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sethvargo/go-retry"
)

// Config contains retry configuration
type Config struct {
	// MaxRetries is the maximum number of retry attempts (default: 5)
	MaxRetries int
	// InitialBackoff is the initial backoff duration (default: 1s)
	InitialBackoff time.Duration
	// MaxBackoff is the maximum backoff duration (default: 60s)
	MaxBackoff time.Duration
	// Multiplier is the backoff multiplier (default: 2.0)
	Multiplier float64
	// RetryableErrors is a list of HTTP status codes that should trigger a retry
	RetryableErrors []int
}

// DefaultConfig returns the default retry configuration
func DefaultConfig() *Config {
	return &Config{
		MaxRetries:      5,
		InitialBackoff:  1 * time.Second,
		MaxBackoff:      60 * time.Second,
		Multiplier:      2.0,
		RetryableErrors: []int{http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout},
	}
}

// HTTPError represents an HTTP error with status code
type HTTPError struct {
	StatusCode int
	Err        error
}

// Error returns the error message
func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %v", e.StatusCode, e.Err)
}

// Unwrap returns the underlying error
func (e *HTTPError) Unwrap() error {
	return e.Err
}

// IsRetryable returns true if the given error is retryable based on the configuration
func (c *Config) IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's an HTTPError (direct or wrapped)
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		for _, code := range c.RetryableErrors {
			if httpErr.StatusCode == code {
				return true
			}
		}
		return false
	}

	// For non-HTTP errors, assume retryable (network errors, timeouts, etc.)
	return true
}

// Do executes the given function with retry logic using exponential backoff
func Do(ctx context.Context, cfg *Config, fn func() error) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	backoff := retry.NewExponential(cfg.InitialBackoff)
	backoff = retry.WithMaxRetries(uint64(cfg.MaxRetries), backoff)
	backoff = retry.WithCappedDuration(cfg.MaxBackoff, backoff)

	retryFunc := func(ctx context.Context) error {
		err := fn()
		if err == nil {
			return nil
		}

		// Check if error is retryable - wrap in RetryableError if so
		if cfg.IsRetryable(err) {
			return retry.RetryableError(err)
		}

		// Return non-retryable errors as-is (will stop retries)
		return err
	}

	return retry.Do(ctx, backoff, retryFunc)
}

// DoWithRetry executes the given function with retry logic and a custom retry check
func DoWithRetry(ctx context.Context, cfg *Config, fn func() error, isRetryable func(error) bool) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	backoff := retry.NewExponential(cfg.InitialBackoff)
	backoff = retry.WithMaxRetries(uint64(cfg.MaxRetries), backoff)
	backoff = retry.WithCappedDuration(cfg.MaxBackoff, backoff)

	retryFunc := func(ctx context.Context) error {
		err := fn()
		if err == nil {
			return nil
		}

		if isRetryable(err) {
			return retry.RetryableError(err)
		}

		// Return non-retryable errors as-is (will stop retries)
		return err
	}

	return retry.Do(ctx, backoff, retryFunc)
}

// WithBackoff returns a go-retry Backoff based on the configuration
func WithBackoff(cfg *Config) retry.Backoff {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	backoff := retry.NewExponential(cfg.InitialBackoff)
	backoff = retry.WithMaxRetries(uint64(cfg.MaxRetries), backoff)
	backoff = retry.WithCappedDuration(cfg.MaxBackoff, backoff)

	return backoff
}

// DoHTTP executes an HTTP request function with retry logic
func DoHTTP(ctx context.Context, cfg *Config, fn func() (*http.Response, error)) (*http.Response, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	var lastResp *http.Response

	err := Do(ctx, cfg, func() error {
		resp, err := fn()
		if err != nil {
			return err
		}
		lastResp = resp

		// Check if status code is in the retryable list
		for _, code := range cfg.RetryableErrors {
			if resp.StatusCode == code {
				return NewHTTPError(resp.StatusCode, fmt.Errorf("HTTP %d", resp.StatusCode))
			}
		}

		// Non-retryable status code - return as error to stop
		if resp.StatusCode >= 400 {
			return NewHTTPError(resp.StatusCode, fmt.Errorf("HTTP %d", resp.StatusCode))
		}

		return nil
	})

	if err != nil {
		// If we have a response, return it even with error for caller to inspect
		if lastResp != nil {
			return lastResp, err
		}
		return nil, err
	}

	return lastResp, nil
}

// RetryFunc is a function that can be retried
type RetryFunc = retry.RetryFunc

// DoContext executes a context-aware function with retry logic
// Note: The function itself must handle marking errors as retryable using retry.RetryableError
func DoContext(ctx context.Context, cfg *Config, fn RetryFunc) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	backoff := WithBackoff(cfg)

	return retry.Do(ctx, backoff, fn)
}

// NewHTTPError creates a new HTTPError
func NewHTTPError(statusCode int, err error) *HTTPError {
	return &HTTPError{
		StatusCode: statusCode,
		Err:        err,
	}
}

// IsRetryableStatusCode returns true if the HTTP status code is retryable
func IsRetryableStatusCode(code int) bool {
	retryableCodes := []int{
		http.StatusRequestTimeout,      // 408
		http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
	}

	for _, c := range retryableCodes {
		if code == c {
			return true
		}
	}
	return false
}

// ConstantBackoff returns a constant backoff strategy
func ConstantBackoff(interval time.Duration, maxRetries int) retry.Backoff {
	backoff := retry.NewConstant(interval)
	backoff = retry.WithMaxRetries(uint64(maxRetries), backoff)
	return backoff
}

// FibonacciBackoff returns a fibonacci backoff strategy (alternative to linear)
func FibonacciBackoff(initial time.Duration, maxRetries int) retry.Backoff {
	backoff := retry.NewFibonacci(initial)
	backoff = retry.WithMaxRetries(uint64(maxRetries), backoff)
	return backoff
}
