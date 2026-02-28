package sync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Store implements SyncStore using S3-compatible object storage.
type S3Store struct {
	client     *s3.Client
	uploader   *manager.Uploader
	downloader *manager.Downloader
	bucket     string
	prefix     string
}

// S3Config contains S3Store configuration.
type S3Config struct {
	Bucket       string
	Prefix       string
	Region       string
	Endpoint     string
	PathStyle    bool
	AccessKey    string
	SecretKey    string
	SessionToken string // optional
}

// NewS3Store creates a new S3Store from config.
func NewS3Store(ctx context.Context, cfg S3Config) (*S3Store, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		func(opts *config.LoadOptions) error {
			if cfg.Endpoint != "" {
				opts.EndpointResolverWithOptions = aws.EndpointResolverWithOptionsFunc(
					func(service, region string, options ...interface{}) (aws.Endpoint, error) {
						return aws.Endpoint{
							URL:               cfg.Endpoint,
							SigningRegion:     cfg.Region,
							HostnameImmutable: cfg.PathStyle,
						}, nil
					},
				)
			}
			if cfg.AccessKey != "" && cfg.SecretKey != "" {
				opts.Credentials = credentials.NewStaticCredentialsProvider(
					cfg.AccessKey, cfg.SecretKey, cfg.SessionToken,
				)
			}
			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg)
	uploader := manager.NewUploader(client)
	downloader := manager.NewDownloader(client)

	return &S3Store{
		client:     client,
		uploader:   uploader,
		downloader: downloader,
		bucket:     cfg.Bucket,
		prefix:     cfg.Prefix,
	}, nil
}

// key returns the full S3 object key for a store key.
func (s *S3Store) key(key string) string {
	if s.prefix == "" {
		return key
	}
	return strings.TrimPrefix(s.prefix+"/", "/") + key
}

// List returns keys under prefix with pagination support.
func (s *S3Store) List(prefix string) ([]string, error) {
	return s.ListWithContext(context.Background(), prefix)
}

// ListWithContext returns keys under prefix with pagination support and context.
func (s *S3Store) ListWithContext(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	prefix = s.key(prefix)

	var continuationToken *string
	for {
		resp, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}

		for _, obj := range resp.Contents {
			// Return relative keys (remove prefix)
			key := strings.TrimPrefix(*obj.Key, s.prefix)
			if key != "" {
				keys = append(keys, key)
			}
		}

		if resp.IsTruncated == nil || !*resp.IsTruncated {
			break
		}
		continuationToken = resp.NextContinuationToken
	}

	return keys, nil
}

// Get downloads an object from S3.
func (s *S3Store) Get(key string) ([]byte, error) {
	return s.GetWithContext(context.Background(), key)
}

// GetWithContext downloads an object from S3 with context.
func (s *S3Store) GetWithContext(ctx context.Context, key string) ([]byte, error) {
	key = s.key(key)

	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// Check for not found
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get object: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read object body: %w", err)
	}

	return data, nil
}

// PutAtomic uploads an object atomically to S3.
// For small objects: single PUT. For large objects: multipart upload.
func (s *S3Store) PutAtomic(key string, data []byte) error {
	return s.PutAtomicWithContext(context.Background(), key, data)
}

// PutAtomicWithContext uploads an object atomically to S3 with context.
func (s *S3Store) PutAtomicWithContext(ctx context.Context, key string, data []byte) error {
	key = s.key(key)

	// Use uploader which handles multipart automatically for large objects
	_, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}

	return nil
}

// isNotFound checks if an error is a "not found" error from S3.
func isNotFound(err error) bool {
	var noSuchKey *types.NoSuchKey
	var notFound *types.NotFound
	return errors.As(err, &noSuchKey) || errors.As(err, &notFound)
}
