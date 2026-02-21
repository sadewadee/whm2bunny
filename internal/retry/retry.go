package retry

import (
	"context"

	"github.com/sethvargo/go-retry"
)

// RetryConfig contains retry configuration
type RetryConfig struct {
	MaxAttempts uint
	Backoff     retry.Backoff
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts: 5,
		Backoff:     retry.NewExponential(1000),
	}
}

// Do executes the given function with retry logic
func Do(ctx context.Context, config *RetryConfig, fn retry.RetryFunc) error {
	if config == nil {
		config = DefaultRetryConfig()
	}
	policy := retry.WithMaxAttempts(config.Backoff, config.MaxAttempts)
	return retry.Do(ctx, policy, fn)
}
