package sync

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ParseS3URI parses an S3 URI into S3Config.
func ParseS3URI(uri string) (S3Config, error) {
	var cfg S3Config

	// Parse URI
	parsed, err := url.Parse(uri)
	if err != nil {
		return cfg, err
	}

	cfg.Bucket = parsed.Host
	cfg.Prefix = parsed.Path
	if cfg.Prefix != "" && cfg.Prefix[0] == '/' {
		cfg.Prefix = cfg.Prefix[1:]
	}

	// Default region
	cfg.Region = "us-east-1"

	// Parse query params
	query := parsed.Query()
	if region := query.Get("region"); region != "" {
		cfg.Region = region
	}
	if endpoint := query.Get("endpoint"); endpoint != "" {
		cfg.Endpoint = endpoint
	}
	if pathStyle := query.Get("path-style"); pathStyle == "true" {
		cfg.PathStyle = true
	}

	return cfg, nil
}

func TestS3Store_ConfigParsing(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		want    S3Config
		wantErr bool
	}{
		{
			name: "AWS S3 simple",
			uri:  "s3://my-bucket/hx-sync",
			want: S3Config{
				Bucket: "my-bucket",
				Prefix: "hx-sync",
				Region: "us-east-1", // default
			},
		},
		{
			name: "AWS S3 with region",
			uri:  "s3://my-bucket/hx-sync?region=us-west-2",
			want: S3Config{
				Bucket: "my-bucket",
				Prefix: "hx-sync",
				Region: "us-west-2",
			},
		},
		{
			name: "MinIO with endpoint",
			uri:  "s3+endpoint://test-bucket/hx?endpoint=http://localhost:9000&path-style=true",
			want: S3Config{
				Bucket:    "test-bucket",
				Prefix:    "hx",
				Region:    "us-east-1", // default
				Endpoint:  "http://localhost:9000",
				PathStyle: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseS3URI(tt.uri)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg)
		})
	}
}

func TestS3Store_KeyGeneration(t *testing.T) {
	store := &S3Store{bucket: "test", prefix: "hx-sync"}

	tests := []struct {
		input    string
		expected string
	}{
		{"vaults/123/objects/segments/456/seg.hxseg", "hx-sync/vaults/123/objects/segments/456/seg.hxseg"},
		{"objects/blobs/ab/cd/hash.hxblob", "hx-sync/objects/blobs/ab/cd/hash.hxblob"},
		{"manifests/node.hxman", "hx-sync/manifests/node.hxman"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := store.key(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}
