package sync

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRetryableStore_MinIOIntegration tests retry behavior with real MinIO
func TestRetryableStore_MinIOIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup MinIO connection
	ctx := context.Background()
	cfg := S3Config{
		Bucket:    "hx-test",
		Prefix:    fmt.Sprintf("test-%d", time.Now().Unix()),
		Region:    "us-east-1",
		Endpoint:  "http://localhost:9000",
		PathStyle: true,
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
	}

	store, err := NewS3Store(ctx, cfg)
	if err != nil {
		t.Skipf("MinIO not available: %v", err)
	}

	// Create bucket if needed
	if err := CreateTestBucket(ctx, cfg); err != nil {
		t.Skipf("Failed to create test bucket: %v", err)
	}

	// Wrap with retryable store
	retryConfig := RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   50 * time.Millisecond,
		MaxDelay:    200 * time.Millisecond,
		Multiplier:  2.0,
	}

	retryableStore := NewRetryableStore(store, retryConfig)

	// Test successful operations
	t.Run("SuccessfulOperations", func(t *testing.T) {
		// Put
		err := retryableStore.PutAtomic("test/retry/key1", []byte("test data"))
		require.NoError(t, err)

		// Get
		data, err := retryableStore.Get("test/retry/key1")
		require.NoError(t, err)
		assert.Equal(t, []byte("test data"), data)

		// List
		keys, err := retryableStore.List("test/retry/")
		require.NoError(t, err)
		assert.Contains(t, keys, "/test/retry/key1") // S3Store returns full paths
	})

	// Test that retryable store implements SyncStore interface
	t.Run("InterfaceCompliance", func(t *testing.T) {
		var _ SyncStore = retryableStore
	})
}

// TestRetryableStore_SimulatedTransientError tests retry with simulated transient errors
func TestRetryableStore_SimulatedTransientError(t *testing.T) {
	// Create a mock store that fails initially then succeeds
	mockStore := &MockSyncStore{
		listResponses: []mockListResponse{
			{keys: nil, err: fmt.Errorf("connection refused")},
			{keys: nil, err: fmt.Errorf("connection refused")},
			{keys: []string{"key1", "key2"}, err: nil},
		},
	}

	retryConfig := RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    50 * time.Millisecond,
		Multiplier:  2.0,
	}

	retryable := NewRetryableStore(mockStore, retryConfig)

	start := time.Now()
	keys, err := retryable.List("prefix")
	duration := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, []string{"key1", "key2"}, keys)
	assert.Equal(t, 3, mockStore.listCalls)

	// Should have taken at least some time for retries
	assert.True(t, duration >= 15*time.Millisecond)
}

// TestRetryableStore_NonRetryableErrorFailsFast tests that non-retryable errors fail immediately
func TestRetryableStore_NonRetryableErrorFailsFast(t *testing.T) {
	mockStore := &MockSyncStore{
		getResponses: []mockGetResponse{
			{data: nil, err: fmt.Errorf("access denied")},
		},
	}

	retryable := NewRetryableStore(mockStore, DefaultRetryConfig())

	start := time.Now()
	_, err := retryable.Get("key1")
	duration := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
	assert.Equal(t, 1, mockStore.getCalls)

	// Should fail immediately without delay
	assert.True(t, duration < 10*time.Millisecond)
}

// TestRetryableStore_ExhaustedRetriesReturnsError tests that exhausted retries return proper error
func TestRetryableStore_ExhaustedRetriesReturnsError(t *testing.T) {
	mockStore := &MockSyncStore{
		putResponses: []mockPutResponse{
			{err: fmt.Errorf("connection refused")},
			{err: fmt.Errorf("connection refused")},
			{err: fmt.Errorf("connection refused")},
		},
	}

	retryConfig := RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    50 * time.Millisecond,
		Multiplier:  2.0,
	}

	retryable := NewRetryableStore(mockStore, retryConfig)

	err := retryable.PutAtomic("key1", []byte("data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "put_atomic failed after 3 attempts")
	assert.Contains(t, err.Error(), "connection refused")
	assert.Equal(t, 3, mockStore.putCalls)
}
