package sync

import (
	"fmt"
	"math"
	"time"
)

// RetryConfig defines retry behavior for S3 operations
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Multiplier  float64
}

// DefaultRetryConfig returns sensible defaults for S3 operations
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		Multiplier:  2.0,
	}
}

// RetryableStore wraps a SyncStore with retry logic
type RetryableStore struct {
	store  SyncStore
	config RetryConfig
}

// NewRetryableStore creates a new retryable store wrapper
func NewRetryableStore(store SyncStore, config RetryConfig) *RetryableStore {
	return &RetryableStore{
		store:  store,
		config: config,
	}
}

// List implements SyncStore with retry logic
func (r *RetryableStore) List(prefix string) ([]string, error) {
	var lastErr error

	for attempt := 0; attempt < r.config.MaxAttempts; attempt++ {
		if attempt > 0 {
			delay := r.calculateDelay(attempt)
			time.Sleep(delay)
		}

		result, err := r.store.List(prefix)
		if err == nil {
			return result, nil
		}

		lastErr = err
		if !isRetryableError(err) {
			break // Don't retry non-retryable errors
		}
	}

	return nil, fmt.Errorf("list failed after %d attempts: %w", r.config.MaxAttempts, lastErr)
}

// Get implements SyncStore with retry logic
func (r *RetryableStore) Get(key string) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt < r.config.MaxAttempts; attempt++ {
		if attempt > 0 {
			delay := r.calculateDelay(attempt)
			time.Sleep(delay)
		}

		result, err := r.store.Get(key)
		if err == nil {
			return result, nil
		}

		lastErr = err
		if !isRetryableError(err) {
			break // Don't retry non-retryable errors
		}
	}

	return nil, fmt.Errorf("get failed after %d attempts: %w", r.config.MaxAttempts, lastErr)
}

// PutAtomic implements SyncStore with retry logic
func (r *RetryableStore) PutAtomic(key string, data []byte) error {
	var lastErr error

	for attempt := 0; attempt < r.config.MaxAttempts; attempt++ {
		if attempt > 0 {
			delay := r.calculateDelay(attempt)
			time.Sleep(delay)
		}

		err := r.store.PutAtomic(key, data)
		if err == nil {
			return nil
		}

		lastErr = err
		if !isRetryableError(err) {
			break // Don't retry non-retryable errors
		}
	}

	return fmt.Errorf("put_atomic failed after %d attempts: %w", r.config.MaxAttempts, lastErr)
}

// calculateDelay implements exponential backoff with jitter
func (r *RetryableStore) calculateDelay(attempt int) time.Duration {
	delay := float64(r.config.BaseDelay) * math.Pow(r.config.Multiplier, float64(attempt-1))
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}

	// Add jitter (±25%)
	jitter := delay * 0.25 * (2*float64(time.Now().UnixNano()%1000)/1000 - 1)
	return time.Duration(delay + jitter)
}

// isRetryableError determines if an error should be retried
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// AWS SDK v2 retryable errors
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"temporary failure",
		"service unavailable",
		"server error",
		"throttling",
		"SlowDown",
		"RequestTimeout",
		"RequestTimeoutException",
	}

	for _, pattern := range retryablePatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
