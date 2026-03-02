# Configuration Reference

**History eXtended** - Complete configuration options reference  
**Last Updated**: 2026-03-01

---

## 📋 Table of Contents

- [Sync Store Configuration](#sync-store-configuration)
- [Security Settings](#security-settings)
- [Performance Tuning](#performance-tuning)
- [Environment Variables](#environment-variables)
- [Configuration Files](#configuration-files)
- [Examples by Use Case](#examples-by-use-case)

---

## 🔄 Sync Store Configuration

### Folder Store (Phase 2A)

```bash
# Basic folder store
hx sync init --store folder:/path/to/sync/folder

# With custom permissions
hx sync init --store folder:/path/to/sync/folder?mode=0755

# Network share (NAS/Syncthing)
hx sync init --store folder:/mnt/syncthing/hx-sync
```

**Parameters**:
- `path` - Local filesystem path (required)
- `mode` - Directory permissions (optional, default: 0755)

### S3 Store (Phase 2B)

```bash
# AWS S3
hx sync init --store "s3://bucket-name/prefix?region=us-west-2"

# MinIO
hx sync init --store "s3://bucket?endpoint=localhost:9000&path_style=true&access_key=minioadmin&secret_key=minioadmin"

# Wasabi
hx sync init --store "s3://bucket/prefix?endpoint=s3.wasabisys.com&region=us-east-1"

# Backblaze B2
hx sync init --store "s3://bucket/prefix?endpoint=s3.us-west-000.backblazeb2.com"
```

**Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `bucket` | string | - | S3 bucket name (required) |
| `prefix` | string | - | Path prefix in bucket (optional) |
| `region` | string | `us-east-1` | AWS region |
| `endpoint` | string | `s3.amazonaws.com` | Custom endpoint URL |
| `path_style` | bool | `false` | Use path-style URLs (MinIO) |
| `access_key` | string | - | AWS access key ID |
| `secret_key` | string | - | AWS secret access key |
| `session_token` | string | - | AWS session token |
| `profile` | string | - | AWS profile name |

### Store Comparison

| Feature | Folder Store | S3 Store |
|---------|--------------|----------|
| **Setup Complexity** | Low | Medium |
| **Network Access** | Local/Network share | Global S3-compatible |
| **Scalability** | Limited by disk | Unlimited |
| **Cost** | Free | Pay-per-use |
| **Latency** | Low | Variable |
| **Durability** | Single-device | 99.9999999% (S3) |
| **Encryption** | Filesystem | AES-256 + E2EE |
| **Versioning** | Manual | Built-in (S3) |

---

## 🔒 Security Settings

### Encryption Configuration

```bash
# Default E2EE (recommended)
hx sync init --store "s3://bucket/prefix?region=us-west-2"

# Custom encryption settings
export HX_ENCRYPTION_ALGORITHM="XChaCha20-Poly1305"
export HX_KEY_DERIVATION="scrypt"
export HX_SCRYPT_COST=32768
export HX_SCRYPT_BLOCKSIZE=8
export HX_SCRYPT_PARALLELISM=1
```

**Encryption Parameters**:
| Variable | Default | Description |
|----------|---------|-------------|
| `HX_ENCRYPTION_ALGORITHM` | `XChaCha20-Poly1305` | AEAD encryption algorithm |
| `HX_KEY_DERIVATION` | `scrypt` | Key derivation function |
| `HX_SCRYPT_COST` | `32768` | scrypt CPU/memory cost |
| `HX_SCRYPT_BLOCKSIZE` | `8` | scrypt block size |
| `HX_SCRYPT_PARALLELISM` | `1` | scrypt parallelism |

### Access Control

```bash
# AWS IAM role (recommended)
export AWS_ROLE_ARN="arn:aws:iam::account:role/HxSyncRole"
export AWS_WEB_IDENTITY_TOKEN_FILE="/path/to/web-identity-token"

# Temporary credentials
export AWS_SESSION_TOKEN="temporary-session-token"

# Assume role manually
aws sts assume-role --role-arn $AWS_ROLE_ARN --role-session-name hx-sync
```

### Network Security

```bash
# Force HTTPS (default)
export HX_FORCE_HTTPS=true

# Custom CA bundle
export AWS_CA_BUNDLE="/path/to/ca-bundle.crt"

# Proxy configuration
export HTTPS_PROXY="http://proxy.company.com:8080"
export HTTP_PROXY="http://proxy.company.com:8080"
export NO_PROXY="localhost,127.0.0.1"
```

---

## ⚡ Performance Tuning

### Concurrency Settings

```bash
# Concurrent uploads/downloads
export HX_SYNC_CONCURRENT_UPLOADS=3
export HX_SYNC_CONCURRENT_DOWNLOADS=5

# Connection pool
export HX_SYNC_MAX_CONNECTIONS=10
export HX_SYNC_MAX_IDLE_CONNECTIONS=2
```

### Timeout and Retry Configuration

```bash
# Timeouts
export HX_SYNC_CONNECT_TIMEOUT=10s
export HX_SYNC_READ_TIMEOUT=30s
export HX_SYNC_TOTAL_TIMEOUT=300s

# Retry policy
export HX_SYNC_MAX_RETRIES=3
export HX_SYNC_RETRY_DELAY=100ms
export HX_SYNC_MAX_RETRY_DELAY=30s
export HX_SYNC_RETRY_BACKOFF=exponential
```

### Memory and Storage

```bash
# Memory limits
export HX_SYNC_MAX_MEMORY_USAGE=1GB
export HX_SYNC_CHUNK_SIZE=64MB

# Local cache
export HX_SYNC_CACHE_SIZE=100MB
export HX_SYNC_CACHE_TTL=1h
```

### Compression

```bash
# Enable compression
export HX_SYNC_COMPRESSION=true
export HX_SYNC_COMPRESSION_LEVEL=6
export HX_SYNC_COMPRESSION_ALGORITHM=gzip
```

---

## 🌍 Environment Variables

### Core Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `HX_CONFIG_PATH` | `~/.config/hx/config.yaml` | Configuration file path |
| `HX_DATA_PATH` | `~/.local/share/hx` | Data directory |
| `HX_SPOOL_PATH` | `/tmp/hx-*` | Spool directory pattern |
| `HX_LOG_LEVEL` | `info` | Logging level (debug, info, warn, error) |

### Sync Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `HX_SYNC_STORE` | - | Sync store configuration (overrides config file) |
| `HX_SYNC_TIMEOUT` | `30s` | Default operation timeout |
| `HX_SYNC_BATCH_SIZE` | `1000` | Objects per batch |
| `HX_SYNC_MANIFEST_CACHE_TTL` | `5m` | Manifest cache duration |

### AWS/S3 Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `AWS_ACCESS_KEY_ID` | - | AWS access key ID |
| `AWS_SECRET_ACCESS_KEY` | - | AWS secret access key |
| `AWS_SESSION_TOKEN` | - | AWS session token |
| `AWS_DEFAULT_REGION` | `us-east-1` | Default AWS region |
| `AWS_PROFILE` | - | AWS profile name |
| `AWS_ROLE_ARN` | - | IAM role ARN |
| `AWS_WEB_IDENTITY_TOKEN_FILE` | - | Web identity token file |

### Performance Tuning

| Variable | Default | Description |
|----------|---------|-------------|
| `HX_SYNC_CONCURRENT_UPLOADS` | `3` | Concurrent upload operations |
| `HX_SYNC_CONCURRENT_DOWNLOADS` | `5` | Concurrent download operations |
| `HX_SYNC_MAX_RETRIES` | `3` | Maximum retry attempts |
| `HX_SYNC_RETRY_DELAY` | `100ms` | Initial retry delay |
| `HX_SYNC_MAX_RETRY_DELAY` | `30s` | Maximum retry delay |

---

## 📄 Configuration Files

### Primary Configuration File

**Location**: `~/.config/hx/config.yaml`

```yaml
# HX Configuration File
version: 1

# Core settings
data_path: "~/.local/share/hx"
spool_path: "/tmp/hx-{uid}"
log_level: "info"

# Sync configuration
sync:
  # Store configuration
  store: "s3://my-hx-bucket/hx-sync?region=us-west-2"
  
  # Performance settings
  timeout: "30s"
  concurrent_uploads: 3
  concurrent_downloads: 5
  max_retries: 3
  retry_delay: "100ms"
  max_retry_delay: "30s"
  
  # Cache settings
  cache_size: "100MB"
  cache_ttl: "1h"
  manifest_cache_ttl: "5m"

# Security settings
security:
  encryption:
    algorithm: "XChaCha20-Poly1305"
    key_derivation: "scrypt"
    scrypt_cost: 32768
    scrypt_blocksize: 8
    scrypt_parallelism: 1

# Retention settings
retention:
  events_retention: "12months"
  blobs_retention: "90days"
  disk_cap_gb: 10

# Import settings
import:
  batch_size: 1000
  max_lines: 100000
  truncate_warning: true
```

### Environment-Specific Configuration

**Development**: `~/.config/hx/config.dev.yaml`
```yaml
sync:
  store: "folder:/tmp/hx-dev-sync"
log_level: "debug"
security:
  encryption:
    algorithm: "XChaCha20-Poly1305"
```

**Production**: `~/.config/hx/config.prod.yaml`
```yaml
sync:
  store: "s3://company-hx-prod/hx-sync?region=us-west-2"
  timeout: "60s"
  max_retries: 5
log_level: "warn"
```

### Configuration Precedence

1. Environment variables (highest priority)
2. Command-line arguments
3. Configuration file
4. Default values (lowest priority)

---

## 💼 Examples by Use Case

### Personal Use (Single Device)

```yaml
# ~/.config/hx/config.yaml
sync:
  store: "folder:/home/user/Dropbox/hx-sync"
retention:
  events_retention: "6months"
  blobs_retention: "30days"
  disk_cap_gb: 5
```

### Team Environment (Multiple Devices)

```yaml
# ~/.config/hx/config.yaml
sync:
  store: "s3://team-hx-sync/hx-sync?region=us-west-2"
  timeout: "60s"
  max_retries: 5
  concurrent_uploads: 5
security:
  encryption:
    algorithm: "XChaCha20-Poly1305"
    scrypt_cost: 65536  # Higher security for team data
```

### Enterprise Deployment

```bash
# Environment variables for enterprise
export AWS_PROFILE=hx-enterprise
export HX_SYNC_STORE="s3://enterprise-hx/hx-sync?region=us-east-1"
export HX_SYNC_TIMEOUT="120s"
export HX_SYNC_MAX_RETRIES=10
export HX_SYNC_CONCURRENT_UPLOADS=10
export HX_LOG_LEVEL="warn"
export HX_ENCRYPTION_ALGORITHM="XChaCha20-Poly1305"
export HX_SCRYPT_COST=131072
```

### Development Environment

```yaml
# ~/.config/hx/config.dev.yaml
sync:
  store: "folder:/tmp/hx-dev-sync"
log_level: "debug"
security:
  encryption:
    algorithm: "XChaCha20-Poly1305"
    scrypt_cost: 16384  # Faster for development
```

### High-Performance Setup

```bash
# Performance optimization
export HX_SYNC_CONCURRENT_UPLOADS=10
export HX_SYNC_CONCURRENT_DOWNLOADS=15
export HX_SYNC_MAX_CONNECTIONS=20
export HX_SYNC_CHUNK_SIZE=128MB
export HX_SYNC_MAX_MEMORY_USAGE=2GB
export HX_SYNC_COMPRESSION=true
export HX_SYNC_COMPRESSION_LEVEL=9
```

### Cost-Optimized Setup

```bash
# Cost optimization for S3
export HX_SYNC_STORE="s3://hx-budget/hx-sync?region=us-east-1"
export HX_SYNC_CONCURRENT_UPLOADS=1
export HX_SYNC_CHUNK_SIZE=32MB
export HX_SYNC_COMPRESSION=true
export HX_SYNC_COMPRESSION_LEVEL=6
```

### Security-Hardened Setup

```bash
# Maximum security configuration
export HX_ENCRYPTION_ALGORITHM="XChaCha20-Poly1305"
export HX_SCRYPT_COST=262144
export HX_SCRYPT_BLOCKSIZE=16
export HX_SCRYPT_PARALLELISM=2
export HX_FORCE_HTTPS=true
export AWS_PROFILE=hx-high-security
```

---

## 🔧 Configuration Validation

### Check Current Configuration

```bash
# Show effective configuration
hx config show

# Validate configuration
hx config validate

# Test sync store connectivity
hx sync test-connection
```

### Configuration Diagnostics

```bash
# Diagnose sync issues
hx sync diagnose

# Show sync status
hx sync status --verbose

# Test encryption
hx crypto test
```

---

## 📚 Additional Resources

- [S3 Sync User Guide](../user_guide/s3_sync.md)
- [Security Best Practices](../architecture/threat_model_phase2.md)
- [Performance Tuning Guide](../runbooks/performance_optimization.md)
- [Troubleshooting Runbooks](../runbooks/)

---

*Last updated: 2026-03-01*
