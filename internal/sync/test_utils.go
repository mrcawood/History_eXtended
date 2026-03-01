package sync

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// CreateTestBucket creates a test bucket in MinIO for integration tests
func CreateTestBucket(ctx context.Context, cfg S3Config) error {
	// Create S3 client with the same config as NewS3Store but without bucket validation
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(aws.NewCredentialsCache(
			aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
				return aws.Credentials{
					AccessKeyID:     cfg.AccessKey,
					SecretAccessKey: cfg.SecretKey,
				}, nil
			}),
		)),
	)
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}

	// Set endpoint for MinIO
	if cfg.Endpoint != "" {
		awsCfg.EndpointResolverWithOptions = aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:           cfg.Endpoint,
				SigningRegion: cfg.Region,
			}, nil
		})
	}

	client := s3.NewFromConfig(awsCfg)

	// Check if bucket exists
	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(cfg.Bucket),
	})
	if err == nil {
		// Bucket already exists
		return nil
	}

	// Create bucket
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(cfg.Bucket),
	})
	if err != nil {
		// For MinIO with path-style and us-east-1, try without location constraint
		if cfg.Region == "us-east-1" {
			_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
				Bucket: aws.String(cfg.Bucket),
			})
		}
		if err != nil {
			return fmt.Errorf("create bucket: %w", err)
		}
	}

	log.Printf("Created test bucket: %s", cfg.Bucket)
	return nil
}
