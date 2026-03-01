package sync

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRetryableStore_BasicSuccess tests successful operations without retries
func TestRetryableStore_BasicSuccess(t *testing.T) {
	mockStore := &MockSyncStore{
		listResponses: []mockListResponse{{keys: []string{"key1", "key2"}, err: nil}},
		getResponses:  []mockGetResponse{{data: []byte("data1"), err: nil}},
		putResponses:  []mockPutResponse{{err: nil}},
	}

	retryable := NewRetryableStore(mockStore, DefaultRetryConfig())

	// Test List
	keys, err := retryable.List("prefix")
	require.NoError(t, err)
	assert.Equal(t, []string{"key1", "key2"}, keys)
	assert.Equal(t, 1, mockStore.listCalls)

	// Test Get
	data, err := retryable.Get("key1")
	require.NoError(t, err)
	assert.Equal(t, []byte("data1"), data)
	assert.Equal(t, 1, mockStore.getCalls)

	// Test PutAtomic
	err = retryable.PutAtomic("key1", []byte("data1"))
	require.NoError(t, err)
	assert.Equal(t, 1, mockStore.putCalls)
}

// TestRetryableStore_RetryOnTransientError tests retry behavior on transient errors
func TestRetryableStore_RetryOnTransientError(t *testing.T) {
	mockStore := &MockSyncStore{
		listResponses: []mockListResponse{
			{keys: nil, err: fmt.Errorf("connection refused")},
			{keys: nil, err: fmt.Errorf("connection refused")},
			{keys: []string{"key1"}, err: nil},
		},
	}

	config := RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		Multiplier:  2.0,
	}

	retryable := NewRetryableStore(mockStore, config)

	start := time.Now()
	keys, err := retryable.List("prefix")
	duration := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, []string{"key1"}, keys)
	assert.Equal(t, 3, mockStore.listCalls)
	assert.True(t, duration >= 20*time.Millisecond) // At least 2 delays
}

// TestRetryableStore_NoRetryOnNonRetryableError tests that non-retryable errors fail immediately
func TestRetryableStore_NoRetryOnNonRetryableError(t *testing.T) {
	mockStore := &MockSyncStore{
		getResponses: []mockGetResponse{
			{data: nil, err: fmt.Errorf("access denied")},
		},
	}

	retryable := NewRetryableStore(mockStore, DefaultRetryConfig())

	_, err := retryable.Get("key1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
	assert.Equal(t, 1, mockStore.getCalls) // Only one attempt
}

// TestRetryableStore_ExhaustRetries tests behavior when all retries are exhausted
func TestRetryableStore_ExhaustRetries(t *testing.T) {
	mockStore := &MockSyncStore{
		putResponses: []mockPutResponse{
			{err: fmt.Errorf("connection refused")},
			{err: fmt.Errorf("connection refused")},
			{err: fmt.Errorf("connection refused")},
		},
	}

	config := RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		Multiplier:  2.0,
	}

	retryable := NewRetryableStore(mockStore, config)

	start := time.Now()
	err := retryable.PutAtomic("key1", []byte("data1"))
	duration := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "put_atomic failed after 3 attempts")
	assert.Equal(t, 3, mockStore.putCalls)
	assert.True(t, duration >= 20*time.Millisecond) // At least 2 delays
}

// TestRetryableStore_ExponentialBackoff tests exponential backoff with jitter
func TestRetryableStore_ExponentialBackoff(t *testing.T) {
	config := RetryConfig{
		MaxAttempts: 4,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		Multiplier:  2.0,
	}

	retryable := NewRetryableStore(nil, config)

	// Test delay calculation
	delay1 := retryable.calculateDelay(1) // First retry
	delay2 := retryable.calculateDelay(2) // Second retry
	delay3 := retryable.calculateDelay(3) // Third retry

	// Should be exponential (with jitter tolerance)
	assert.True(t, delay1 >= 7*time.Millisecond && delay1 <= 13*time.Millisecond)
	assert.True(t, delay2 >= 15*time.Millisecond && delay2 <= 25*time.Millisecond)
	assert.True(t, delay3 >= 30*time.Millisecond && delay3 <= 50*time.Millisecond)
}

// Mock implementations for testing

type MockSyncStore struct {
	listCalls int
	getCalls  int
	putCalls  int

	listResponses []mockListResponse
	getResponses  []mockGetResponse
	putResponses  []mockPutResponse
}

type mockListResponse struct {
	keys []string
	err  error
}

type mockGetResponse struct {
	data []byte
	err  error
}

type mockPutResponse struct {
	err error
}

func (m *MockSyncStore) List(prefix string) ([]string, error) {
	m.listCalls++
	if len(m.listResponses) > 0 {
		resp := m.listResponses[0]
		m.listResponses = m.listResponses[1:]
		return resp.keys, resp.err
	}
	return nil, fmt.Errorf("unexpected List call")
}

func (m *MockSyncStore) Get(key string) ([]byte, error) {
	m.getCalls++
	if len(m.getResponses) > 0 {
		resp := m.getResponses[0]
		m.getResponses = m.getResponses[1:]
		return resp.data, resp.err
	}
	return nil, fmt.Errorf("unexpected Get call")
}

func (m *MockSyncStore) PutAtomic(key string, data []byte) error {
	m.putCalls++
	if len(m.putResponses) > 0 {
		resp := m.putResponses[0]
		m.putResponses = m.putResponses[1:]
		return resp.err
	}
	return fmt.Errorf("unexpected PutAtomic call")
}
