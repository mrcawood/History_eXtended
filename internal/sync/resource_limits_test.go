package sync

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResourceLimits tests the resource limit constants and validation
func TestResourceLimits(t *testing.T) {
	// Test that limits are reasonable
	assert.Greater(t, MaxManifestSize, 1024, "MaxManifestSize should be at least 1KB")
	assert.Less(t, MaxManifestSize, 100*1024*1024, "MaxManifestSize should be less than 100MB")

	assert.Greater(t, MaxSegmentSize, 1024*1024, "MaxSegmentSize should be at least 1MB")
	assert.Less(t, MaxSegmentSize, 1024*1024*1024, "MaxSegmentSize should be less than 1GB")

	assert.Greater(t, MaxTombstoneSize, 64, "MaxTombstoneSize should be at least 64 bytes")
	assert.Less(t, MaxTombstoneSize, 1024*1024, "MaxTombstoneSize should be less than 1MB")

	assert.Greater(t, MaxObjectsPerPull, 100, "MaxObjectsPerPull should be at least 100")
	assert.Less(t, MaxObjectsPerPull, 1000000, "MaxObjectsPerPull should be less than 1M")

	assert.Greater(t, MaxPullDuration, 30, "MaxPullDuration should be at least 30 minutes")
	assert.Less(t, MaxPullDuration, 24*60*60, "MaxPullDuration should be less than 24 hours")
}

// TestCheckManifestSize tests manifest size validation
func TestCheckManifestSize(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr bool
		errType error
	}{
		{"Valid small manifest", 1024, false, nil},
		{"Valid medium manifest", 1024 * 1024, false, nil}, // 1MB
		{"Valid large manifest", MaxManifestSize, false, nil},
		{"Invalid too large manifest", MaxManifestSize + 1, true, ErrManifestTooLarge},
		{"Invalid huge manifest", 100 * 1024 * 1024, true, ErrManifestTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckManifestSize(tt.size)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.errType))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCheckSegmentSize tests segment size validation
func TestCheckSegmentSize(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr bool
		errType error
	}{
		{"Valid small segment", 1024, false, nil},
		{"Valid medium segment", 10 * 1024 * 1024, false, nil}, // 10MB
		{"Valid large segment", MaxSegmentSize, false, nil},
		{"Invalid too large segment", MaxSegmentSize + 1, true, ErrSegmentTooLarge},
		{"Invalid huge segment", 200 * 1024 * 1024, true, ErrSegmentTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckSegmentSize(tt.size)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.errType))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCheckTombstoneSize tests tombstone size validation
func TestCheckTombstoneSize(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr bool
		errType error
	}{
		{"Valid small tombstone", 64, false, nil},
		{"Valid medium tombstone", 512, false, nil},
		{"Valid large tombstone", MaxTombstoneSize, false, nil},
		{"Invalid too large tombstone", MaxTombstoneSize + 1, true, ErrTombstoneTooLarge},
		{"Invalid huge tombstone", 2048, true, ErrTombstoneTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckTombstoneSize(tt.size)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.errType))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestResourceLimiter tests the resource limiter functionality
func TestResourceLimiter(t *testing.T) {
	limiter := NewResourceLimiter(5)

	// Test initial state
	assert.Equal(t, 0, limiter.GetProcessedCount())
	assert.NoError(t, limiter.CheckObjectCount())

	// Test incrementing within limits
	for i := 0; i < 5; i++ {
		err := limiter.IncrementObjectCount()
		require.NoError(t, err)
		assert.Equal(t, i+1, limiter.GetProcessedCount())
	}

	// Test exceeding limit
	err := limiter.IncrementObjectCount()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTooManyObjects))
	assert.Equal(t, 5, limiter.GetProcessedCount()) // Should not increment

	// Test reset
	limiter.Reset()
	assert.Equal(t, 0, limiter.GetProcessedCount())
	assert.NoError(t, limiter.CheckObjectCount())
}

// TestResourceLimiterDefault tests resource limiter with default limits
func TestResourceLimiterDefault(t *testing.T) {
	limiter := NewResourceLimiter(0) // Should use default

	assert.Equal(t, 0, limiter.GetProcessedCount())
	assert.Equal(t, MaxObjectsPerPull, limiter.maxObjects)
}

// TestValidateManifest tests comprehensive manifest validation
func TestValidateManifest(t *testing.T) {
	tests := []struct {
		name     string
		manifest *Manifest
		wantErr  bool
		errType  error
	}{
		{
			name: "Valid small manifest",
			manifest: &Manifest{
				VaultID:     "test-vault",
				NodeID:      "test-node",
				ManifestSeq: 1,
				Segments:    make([]ManifestSegment, 10),
				Tombstones:  make([]ManifestTombstone, 5),
			},
			wantErr: false,
		},
		{
			name: "Valid large manifest",
			manifest: &Manifest{
				VaultID:     "test-vault",
				NodeID:      "test-node",
				ManifestSeq: 1,
				Segments:    make([]ManifestSegment, MaxObjectsPerPull/3),
				Tombstones:  make([]ManifestTombstone, MaxObjectsPerPull/3),
			},
			wantErr: false,
		},
		{
			name: "Invalid too many total objects",
			manifest: &Manifest{
				VaultID:     "test-vault",
				NodeID:      "test-node",
				ManifestSeq: 1,
				Segments:    make([]ManifestSegment, MaxObjectsPerPull/2+1),
				Tombstones:  make([]ManifestTombstone, MaxObjectsPerPull/2+1),
			},
			wantErr: true,
			errType: ErrTooManyObjects,
		},
		{
			name: "Invalid too many segments",
			manifest: &Manifest{
				VaultID:     "test-vault",
				NodeID:      "test-node",
				ManifestSeq: 1,
				Segments:    make([]ManifestSegment, MaxObjectsPerPull/2+1),
				Tombstones:  make([]ManifestTombstone, 1),
			},
			wantErr: true,
			errType: ErrTooManyObjects,
		},
		{
			name: "Invalid too many tombstones",
			manifest: &Manifest{
				VaultID:     "test-vault",
				NodeID:      "test-node",
				ManifestSeq: 1,
				Segments:    make([]ManifestSegment, 1),
				Tombstones:  make([]ManifestTombstone, MaxObjectsPerPull/2+1),
			},
			wantErr: true,
			errType: ErrTooManyObjects,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateManifest(tt.manifest)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.errType))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestResourceExhaustionPrevention tests resource exhaustion scenarios
func TestResourceExhaustionPrevention(t *testing.T) {
	// Test that large manifests are rejected
	largeManifest := &Manifest{
		VaultID:     "test-vault",
		NodeID:      "test-node",
		ManifestSeq: 1,
		Segments:    make([]ManifestSegment, MaxObjectsPerPull+1),
		Tombstones:  make([]ManifestTombstone, 0),
	}

	err := ValidateManifest(largeManifest)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTooManyObjects))

	// Test that resource limiter prevents unlimited processing
	limiter := NewResourceLimiter(100)

	for i := 0; i < 100; i++ {
		err := limiter.IncrementObjectCount()
		require.NoError(t, err)
	}

	// Next increment should fail
	err = limiter.IncrementObjectCount()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTooManyObjects))
}
