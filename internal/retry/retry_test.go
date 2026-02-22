package retry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries 5, got %d", cfg.MaxRetries)
	}

	if cfg.InitialBackoff != 1*time.Second {
		t.Errorf("Expected InitialBackoff 1s, got %v", cfg.InitialBackoff)
	}

	if cfg.MaxBackoff != 60*time.Second {
		t.Errorf("Expected MaxBackoff 60s, got %v", cfg.MaxBackoff)
	}

	if cfg.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier 2.0, got %f", cfg.Multiplier)
	}

	expectedCodes := []int{http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout}
	if len(cfg.RetryableErrors) != len(expectedCodes) {
		t.Errorf("Expected %d retryable error codes, got %d", len(expectedCodes), len(cfg.RetryableErrors))
	}
}

func TestConfig_IsRetryable(t *testing.T) {
	t.Run("returns true for retryable HTTP errors", func(t *testing.T) {
		cfg := DefaultConfig()

		for _, code := range cfg.RetryableErrors {
			err := NewHTTPError(code, fmt.Errorf("http error"))
			if !cfg.IsRetryable(err) {
				t.Errorf("Expected HTTP %d to be retryable", code)
			}
		}
	})

	t.Run("returns false for non-retryable HTTP errors", func(t *testing.T) {
		cfg := DefaultConfig()

		nonRetryable := []int{
			http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusForbidden,
			http.StatusNotFound,
		}

		for _, code := range nonRetryable {
			err := NewHTTPError(code, fmt.Errorf("http error"))
			if cfg.IsRetryable(err) {
				t.Errorf("Expected HTTP %d to not be retryable", code)
			}
		}
	})

	t.Run("returns true for non-HTTP errors", func(t *testing.T) {
		cfg := DefaultConfig()

		err := errors.New("generic error")
		if !cfg.IsRetryable(err) {
			t.Error("Expected non-HTTP errors to be retryable by default")
		}
	})

	t.Run("returns false for nil error", func(t *testing.T) {
		cfg := DefaultConfig()

		if cfg.IsRetryable(nil) {
			t.Error("Expected nil error to not be retryable")
		}
	})
}

func TestDo(t *testing.T) {
	t.Run("succeeds on first attempt", func(t *testing.T) {
		ctx := context.Background()
		cfg := DefaultConfig()
		attempts := 0

		err := Do(ctx, cfg, func() error {
			attempts++
			return nil
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if attempts != 1 {
			t.Errorf("Expected 1 attempt, got %d", attempts)
		}
	})

	t.Run("retries on retryable errors", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			MaxRetries:      3,
			InitialBackoff:  10 * time.Millisecond,
			MaxBackoff:      100 * time.Millisecond,
			Multiplier:      2.0,
			RetryableErrors: []int{http.StatusInternalServerError},
		}

		attempts := 0

		err := Do(ctx, cfg, func() error {
			attempts++
			if attempts < 3 {
				return NewHTTPError(http.StatusInternalServerError, errors.New("server error"))
			}
			return nil
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("returns error after max retries", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			MaxRetries:      3,
			InitialBackoff:  10 * time.Millisecond,
			MaxBackoff:      100 * time.Millisecond,
			Multiplier:      2.0,
			RetryableErrors: []int{http.StatusInternalServerError},
		}

		attempts := 0

		err := Do(ctx, cfg, func() error {
			attempts++
			return NewHTTPError(http.StatusInternalServerError, errors.New("server error"))
		})

		if err == nil {
			t.Fatal("Expected error after max retries")
		}

		// Should be called 1 initial + 3 retries = 4 total
		expectedAttempts := cfg.MaxRetries + 1
		if attempts != expectedAttempts {
			t.Errorf("Expected %d attempts, got %d", expectedAttempts, attempts)
		}
	})

	t.Run("does not retry non-retryable errors", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			MaxRetries:      5,
			InitialBackoff:  10 * time.Millisecond,
			MaxBackoff:      100 * time.Millisecond,
			Multiplier:      2.0,
			RetryableErrors: []int{http.StatusInternalServerError},
		}

		attempts := 0

		err := Do(ctx, cfg, func() error {
			attempts++
			return NewHTTPError(http.StatusNotFound, errors.New("not found"))
		})

		if err == nil {
			t.Fatal("Expected error")
		}

		if attempts != 1 {
			t.Errorf("Expected 1 attempt (no retries), got %d", attempts)
		}
	})

	t.Run("uses default config when nil is passed", func(t *testing.T) {
		ctx := context.Background()
		attempts := 0

		err := Do(ctx, nil, func() error {
			attempts++
			if attempts < 2 {
				return errors.New("temporary error")
			}
			return nil
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if attempts != 2 {
			t.Errorf("Expected 2 attempts, got %d", attempts)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cfg := &Config{
			MaxRetries:      100,
			InitialBackoff:  1 * time.Second,
			MaxBackoff:      1 * time.Second,
			Multiplier:      1.0,
			RetryableErrors: []int{http.StatusInternalServerError},
		}

		attempts := 0

		// Cancel after first attempt
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		err := Do(ctx, cfg, func() error {
			attempts++
			return NewHTTPError(http.StatusInternalServerError, errors.New("server error"))
		})

		if err == nil {
			t.Fatal("Expected context cancellation error")
		}

		if attempts != 1 {
			t.Errorf("Expected 1 attempt before cancellation, got %d", attempts)
		}
	})
}

func TestDoWithRetry(t *testing.T) {
	t.Run("uses custom retry check function", func(t *testing.T) {
		ctx := context.Background()
		cfg := DefaultConfig()

		attempts := 0

		// Custom retry logic: only retry errors with message "retry-me"
		isRetryable := func(err error) bool {
			return err.Error() == "retry-me"
		}

		err := DoWithRetry(ctx, cfg, func() error {
			attempts++
			if attempts == 1 {
				return errors.New("retry-me")
			}
			if attempts == 2 {
				return errors.New("dont-retry-me")
			}
			return nil
		}, isRetryable)

		// Should stop at "dont-retry-me"
		if err == nil {
			t.Fatal("Expected error")
		}

		if err.Error() != "dont-retry-me" {
			t.Errorf("Expected 'dont-retry-me' error, got %v", err)
		}

		if attempts != 2 {
			t.Errorf("Expected 2 attempts, got %d", attempts)
		}
	})
}

func TestWithBackoff(t *testing.T) {
	t.Run("creates valid backoff strategy", func(t *testing.T) {
		cfg := DefaultConfig()
		backoff := WithBackoff(cfg)

		if backoff == nil {
			t.Fatal("Expected non-nil backoff")
		}
	})

	t.Run("uses default config when nil is passed", func(t *testing.T) {
		backoff := WithBackoff(nil)

		if backoff == nil {
			t.Fatal("Expected non-nil backoff with default config")
		}
	})
}

func TestDoHTTP(t *testing.T) {
	t.Run("retries on retryable HTTP status codes", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			MaxRetries:      3,
			InitialBackoff:  10 * time.Millisecond,
			MaxBackoff:      100 * time.Millisecond,
			RetryableErrors: []int{http.StatusServiceUnavailable},
		}

		attempts := 0

		resp, err := DoHTTP(ctx, cfg, func() (*http.Response, error) {
			attempts++
			if attempts < 3 {
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
			}, nil
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %d", resp.StatusCode)
		}

		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("returns error on non-retryable status", func(t *testing.T) {
		ctx := context.Background()
		cfg := DefaultConfig()

		attempts := 0

		resp, err := DoHTTP(ctx, cfg, func() (*http.Response, error) {
			attempts++
			return &http.Response{
				StatusCode: http.StatusNotFound,
			}, nil
		})

		// Should return error but also the response
		if err == nil {
			t.Fatal("Expected error")
		}

		if resp == nil {
			t.Fatal("Expected response to be returned even with error")
		}

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", resp.StatusCode)
		}

		if attempts != 1 {
			t.Errorf("Expected 1 attempt (no retry), got %d", attempts)
		}
	})

	t.Run("propagates network errors", func(t *testing.T) {
		ctx := context.Background()
		cfg := &Config{
			MaxRetries:      2,
			InitialBackoff:  10 * time.Millisecond,
			MaxBackoff:      100 * time.Millisecond,
			RetryableErrors: []int{http.StatusServiceUnavailable},
		}

		attempts := 0
		networkErr := errors.New("connection refused")

		_, err := DoHTTP(ctx, cfg, func() (*http.Response, error) {
			attempts++
			return nil, networkErr
		})

		if err == nil {
			t.Fatal("Expected error")
		}

		// Should retry network errors
		expectedAttempts := cfg.MaxRetries + 1
		if attempts != expectedAttempts {
			t.Errorf("Expected %d attempts, got %d", expectedAttempts, attempts)
		}
	})

	t.Run("succeeds on 200 OK", func(t *testing.T) {
		ctx := context.Background()
		cfg := DefaultConfig()

		attempts := 0

		resp, err := DoHTTP(ctx, cfg, func() (*http.Response, error) {
			attempts++
			return &http.Response{
				StatusCode: http.StatusOK,
			}, nil
		})

		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %d", resp.StatusCode)
		}

		if attempts != 1 {
			t.Errorf("Expected 1 attempt, got %d", attempts)
		}
	})
}

func TestNewHTTPError(t *testing.T) {
	t.Run("creates HTTP error with correct fields", func(t *testing.T) {
		originalErr := errors.New("original error")
		httpErr := NewHTTPError(http.StatusBadRequest, originalErr)

		if httpErr.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, httpErr.StatusCode)
		}

		if httpErr.Err != originalErr {
			t.Error("Expected original error to be wrapped")
		}

		if httpErr.Error() == "" {
			t.Error("Expected non-empty error message")
		}
	})
}

func TestHTTPError_Unwrap(t *testing.T) {
	t.Run("unwraps to original error", func(t *testing.T) {
		originalErr := errors.New("original error")
		httpErr := NewHTTPError(http.StatusInternalServerError, originalErr)

		unwrapped := errors.Unwrap(httpErr)

		if unwrapped != originalErr {
			t.Errorf("Expected unwrapped error to be original error, got %v", unwrapped)
		}
	})

	t.Run("works with errors.As", func(t *testing.T) {
		originalErr := errors.New("original error")
		httpErr := NewHTTPError(http.StatusInternalServerError, originalErr)

		var target *HTTPError
		if !errors.As(httpErr, &target) {
			t.Error("Expected errors.As to find HTTPError")
		}

		if target.StatusCode != http.StatusInternalServerError {
			t.Errorf("Expected status 500, got %d", target.StatusCode)
		}
	})
}

func TestIsRetryableStatusCode(t *testing.T) {
	t.Run("returns true for retryable status codes", func(t *testing.T) {
		retryableCodes := []int{
			http.StatusRequestTimeout,
			http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
		}

		for _, code := range retryableCodes {
			if !IsRetryableStatusCode(code) {
				t.Errorf("Expected HTTP %d to be retryable", code)
			}
		}
	})

	t.Run("returns false for non-retryable status codes", func(t *testing.T) {
		nonRetryableCodes := []int{
			http.StatusOK,
			http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusForbidden,
			http.StatusNotFound,
		}

		for _, code := range nonRetryableCodes {
			if IsRetryableStatusCode(code) {
				t.Errorf("Expected HTTP %d to not be retryable", code)
			}
		}
	})
}

func TestConstantBackoff(t *testing.T) {
	t.Run("creates constant backoff strategy", func(t *testing.T) {
		interval := 100 * time.Millisecond
		maxRetries := 3

		backoff := ConstantBackoff(interval, maxRetries)

		if backoff == nil {
			t.Fatal("Expected non-nil backoff")
		}
	})
}

func TestFibonacciBackoff(t *testing.T) {
	t.Run("creates fibonacci backoff strategy", func(t *testing.T) {
		initial := 50 * time.Millisecond
		maxRetries := 4

		backoff := FibonacciBackoff(initial, maxRetries)

		if backoff == nil {
			t.Fatal("Expected non-nil backoff")
		}
	})
}

func TestConfigWithCustomRetryableErrors(t *testing.T) {
	t.Run("respects custom retryable error codes", func(t *testing.T) {
		cfg := &Config{
			MaxRetries:      2,
			InitialBackoff:  10 * time.Millisecond,
			MaxBackoff:      100 * time.Millisecond,
			Multiplier:      2.0,
			RetryableErrors: []int{http.StatusTeapot}, // 418
		}

		attempts := 0

		ctx := context.Background()
		err := Do(ctx, cfg, func() error {
			attempts++
			return NewHTTPError(http.StatusTeapot, errors.New("I'm a teapot"))
		})

		// Should retry custom retryable error
		expectedAttempts := cfg.MaxRetries + 1
		if attempts != expectedAttempts {
			t.Errorf("Expected %d attempts, got %d", expectedAttempts, attempts)
		}

		if err == nil {
			t.Fatal("Expected error")
		}
	})
}

func BenchmarkDo_Success(b *testing.B) {
	ctx := context.Background()
	cfg := DefaultConfig()

	for i := 0; i < b.N; i++ {
		Do(ctx, cfg, func() error {
			return nil
		})
	}
}

func BenchmarkDo_RetryOnce(b *testing.B) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:      1,
		InitialBackoff:  time.Microsecond,
		MaxBackoff:      time.Microsecond,
		Multiplier:      1.0,
		RetryableErrors: []int{http.StatusInternalServerError},
	}

	for i := 0; i < b.N; i++ {
		attempts := 0
		Do(ctx, cfg, func() error {
			attempts++
			if attempts == 1 {
				return NewHTTPError(http.StatusInternalServerError, errors.New("error"))
			}
			return nil
		})
	}
}
