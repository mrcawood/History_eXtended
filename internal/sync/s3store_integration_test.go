package sync

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minIOAvailable checks if MinIO is reachable at localhost:9000.
// If HX_REQUIRE_S3_ENDPOINT=1, fails when unavailable; otherwise skips.
func minIOAvailable(t *testing.T) {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:9000", 2*time.Second)
	if err == nil {
		conn.Close()
		return
	}
	if os.Getenv("HX_REQUIRE_S3_ENDPOINT") == "1" {
		t.Fatalf("MinIO required but not reachable: %v", err)
	}
	t.Skipf("MinIO not available: %v", err)
}

// TestS3Store_MinIOIntegration tests S3Store against a real MinIO instance.
// Requires MinIO on localhost:9000. Skips if unavailable unless HX_REQUIRE_S3_ENDPOINT=1.
func TestS3Store_MinIOIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	minIOAvailable(t)

	ctx := context.Background()
	cfg := S3Config{
		Bucket:    "test-bucket",
		Prefix:    fmt.Sprintf("test-%d", time.Now().Unix()),
		Region:    "us-east-1",
		Endpoint:  "http://localhost:9000",
		PathStyle: true,
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
	}

	store, err := NewS3Store(ctx, cfg)
	if err != nil {
		if os.Getenv("HX_REQUIRE_S3_ENDPOINT") == "1" {
			t.Fatalf("NewS3Store failed: %v", err)
		}
		t.Skipf("MinIO not available: %v", err)
	}

	// Test List pagination
	t.Run("ListPagination", func(t *testing.T) {
		// Create test objects
		for i := 0; i < 15; i++ {
			key := fmt.Sprintf("test/pagination/obj%02d", i)
			data := []byte(fmt.Sprintf("test data %d", i))
			err := store.PutAtomicWithContext(ctx, key, data)
			require.NoError(t, err)
		}

		// List with pagination
		keys, err := store.ListWithContext(ctx, "test/pagination/")
		require.NoError(t, err)
		assert.Len(t, keys, 15)

		// Verify keys
		for i := 0; i < 15; i++ {
			expectedKey := fmt.Sprintf("test/pagination/obj%02d", i)
			assert.Contains(t, keys, expectedKey)
		}

		// Cleanup
		for _, key := range keys {
			fullKey := store.key(key)
			_, _ = store.client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(store.bucket),
				Key:    aws.String(fullKey),
			})
		}
	})

	// Test multipart upload (large object)
	t.Run("MultipartUpload", func(t *testing.T) {
		// Create a large object (>5MB to trigger multipart)
		largeData := make([]byte, 6*1024*1024) // 6MB
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}

		key := "test/multipart/large.bin"
		err := store.PutAtomicWithContext(ctx, key, largeData)
		require.NoError(t, err)

		// Get and verify
		retrieved, err := store.GetWithContext(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, len(largeData), len(retrieved))
		assert.Equal(t, largeData[:100], retrieved[:100]) // Check first 100 bytes

		// Cleanup
		fullKey := store.key(key)
		_, _ = store.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(store.bucket),
			Key:    aws.String(fullKey),
		})
	})

	// Test basic operations
	t.Run("BasicOperations", func(t *testing.T) {
		key := "test/basic/object.txt"
		data := []byte("hello world")

		// Put
		err := store.PutAtomicWithContext(ctx, key, data)
		require.NoError(t, err)

		// Get
		retrieved, err := store.GetWithContext(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, data, retrieved)

		// Get non-existent
		_, err = store.GetWithContext(ctx, "test/basic/nonexistent.txt")
		assert.Equal(t, ErrNotFound, err)

		// Cleanup
		fullKey := store.key(key)
		_, _ = store.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(store.bucket),
			Key:    aws.String(fullKey),
		})
	})
}
