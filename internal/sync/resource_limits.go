package sync

import (
	"errors"
	"fmt"
)

// Resource limits for security and stability
const (
	// MaxManifestSize is the maximum allowed manifest size in bytes
	MaxManifestSize = 10 * 1024 * 1024 // 10MB

	// MaxSegmentSize is the maximum allowed segment size in bytes
	MaxSegmentSize = 100 * 1024 * 1024 // 100MB

	// MaxTombstoneSize is the maximum allowed tombstone size in bytes
	MaxTombstoneSize = 1024 // 1KB (tombstones should be small)

	// MaxObjectsPerPull is the maximum number of objects to process in one pull operation
	MaxObjectsPerPull = 10000

	// MaxPullDuration is the maximum time budget for a pull operation
	MaxPullDuration = 30 * 60 // 30 minutes
)

// Resource limit errors
var (
	ErrManifestTooLarge  = errors.New("manifest size exceeds maximum allowed")
	ErrSegmentTooLarge   = errors.New("segment size exceeds maximum allowed")
	ErrTombstoneTooLarge = errors.New("tombstone size exceeds maximum allowed")
	ErrTooManyObjects    = errors.New("too many objects to process in single operation")
	ErrOperationTimeout  = errors.New("operation exceeded maximum duration")
)

// ResourceLimiter enforces resource limits on sync operations
type ResourceLimiter struct {
	objectsProcessed int
	maxObjects       int
}

// NewResourceLimiter creates a new resource limiter
func NewResourceLimiter(maxObjects int) *ResourceLimiter {
	if maxObjects <= 0 {
		maxObjects = MaxObjectsPerPull
	}
	return &ResourceLimiter{
		maxObjects: maxObjects,
	}
}

// CheckManifestSize validates manifest size against limits
func CheckManifestSize(size int) error {
	if size > MaxManifestSize {
		return ErrManifestTooLarge
	}
	return nil
}

// CheckSegmentSize validates segment size against limits
func CheckSegmentSize(size int) error {
	if size > MaxSegmentSize {
		return ErrSegmentTooLarge
	}
	return nil
}

// CheckTombstoneSize validates tombstone size against limits
func CheckTombstoneSize(size int) error {
	if size > MaxTombstoneSize {
		return ErrTombstoneTooLarge
	}
	return nil
}

// CheckObjectCount validates that we haven't exceeded object processing limits
func (rl *ResourceLimiter) CheckObjectCount() error {
	if rl.objectsProcessed >= rl.maxObjects {
		return ErrTooManyObjects
	}
	return nil
}

// IncrementObjectCount increments the processed object counter
func (rl *ResourceLimiter) IncrementObjectCount() error {
	if err := rl.CheckObjectCount(); err != nil {
		return err
	}
	rl.objectsProcessed++
	return nil
}

// GetProcessedCount returns the number of objects processed
func (rl *ResourceLimiter) GetProcessedCount() int {
	return rl.objectsProcessed
}

// Reset resets the object counter
func (rl *ResourceLimiter) Reset() {
	rl.objectsProcessed = 0
}

// ValidateManifest validates a manifest against all resource limits
func ValidateManifest(manifest *Manifest) error {
	// Check total number of objects
	totalObjects := len(manifest.Segments) + len(manifest.Tombstones)
	if totalObjects > MaxObjectsPerPull {
		return fmt.Errorf("%w: manifest has %d objects, max %d",
			ErrTooManyObjects, totalObjects, MaxObjectsPerPull)
	}

	// Check individual object counts
	if len(manifest.Segments) > MaxObjectsPerPull/2 {
		return fmt.Errorf("%w: too many segments (%d), max %d",
			ErrTooManyObjects, len(manifest.Segments), MaxObjectsPerPull/2)
	}

	if len(manifest.Tombstones) > MaxObjectsPerPull/2 {
		return fmt.Errorf("%w: too many tombstones (%d), max %d",
			ErrTooManyObjects, len(manifest.Tombstones), MaxObjectsPerPull/2)
	}

	return nil
}
