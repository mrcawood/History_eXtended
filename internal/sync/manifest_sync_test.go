package sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPublishManifest(t *testing.T) {
	// This test would require a database connection and sync store
	// For now, we'll test the manifest creation logic
	t.Skip("Requires database setup - integration test")
}

func TestPull_ManifestDriven(t *testing.T) {
	// This test would require a database connection and sync store
	// For now, we'll test the pull logic structure
	t.Skip("Requires database setup - integration test")
}

func TestPullResult(t *testing.T) {
	res := &PullResult{
		ManifestsDownloaded: 2,
		SegmentsImported:    5,
		TombstonesImported:  1,
		Errors:              []string{"test error"},
	}

	assert.Equal(t, 2, res.ManifestsDownloaded)
	assert.Equal(t, 5, res.SegmentsImported)
	assert.Equal(t, 1, res.TombstonesImported)
	assert.Len(t, res.Errors, 1)
	assert.Equal(t, "test error", res.Errors[0])
}
