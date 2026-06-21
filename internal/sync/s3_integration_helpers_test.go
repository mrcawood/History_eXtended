package sync

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"testing"
	"time"
)

const defaultMinIOEndpoint = "http://127.0.0.1:9000"

// minIOTestEndpoint returns the MinIO base URL for integration tests.
// CI sets HX_S3_ENDPOINT (e.g. http://minio:9000 on Forgejo DinD).
func minIOTestEndpoint() string {
	if ep := os.Getenv("HX_S3_ENDPOINT"); ep != "" {
		return ep
	}
	return defaultMinIOEndpoint
}

// minIOTestDialAddr returns host:port for a TCP reachability probe.
func minIOTestDialAddr() string {
	u, err := url.Parse(minIOTestEndpoint())
	if err != nil || u.Host == "" {
		return "127.0.0.1:9000"
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "9000"
		}
	}
	return net.JoinHostPort(host, port)
}

func minIOTestS3Config(bucket, prefix string) S3Config {
	return S3Config{
		Bucket:    bucket,
		Prefix:    prefix,
		Region:    "us-east-1",
		Endpoint:  minIOTestEndpoint(),
		PathStyle: true,
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
	}
}

// minIOAvailable checks MinIO reachability at HX_S3_ENDPOINT (or 127.0.0.1:9000).
// If HX_REQUIRE_S3_ENDPOINT=1, fails when unavailable; otherwise skips.
func minIOAvailable(t *testing.T) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", minIOTestDialAddr(), 2*time.Second)
	if err == nil {
		conn.Close()
		return
	}
	if os.Getenv("HX_REQUIRE_S3_ENDPOINT") == "1" {
		t.Fatalf("MinIO required but not reachable at %s: %v", minIOTestDialAddr(), err)
	}
	t.Skipf("MinIO not available at %s: %v", minIOTestDialAddr(), err)
}

func minIOTestPrefix() string {
	return fmt.Sprintf("test-%d", time.Now().Unix())
}
