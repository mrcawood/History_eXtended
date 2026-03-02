# S3 Sync User Guide

**Phase 2B**: S3-compatible object storage sync for History eXtended  
**Last Updated**: 2026-03-01

---

## 🚀 Quick Setup

### Prerequisites
- Go 1.21+ (for building)
- AWS CLI or S3-compatible storage account
- Existing hx installation (Phase 1 complete)

### AWS S3 Setup

```bash
# 1. Create S3 bucket
aws s3 mb s3://your-hx-sync-bucket --region us-west-2

# 2. Initialize hx sync with S3 store
hx sync init --store s3://your-hx-sync-bucket/hx-sync?region=us-west-2

# 3. Test connection
hx sync status

# 4. Push existing history
hx sync push

# 5. Pull on other devices
hx sync pull
```

### MinIO Setup (Local/Self-Hosted)

```bash
# 1. Start MinIO (Docker)
docker run -d --name hx-minio \
  -p 9000:9000 -p 9001:9001 \
  -e "MINIO_ROOT_USER=minioadmin" \
  -e "MINIO_ROOT_PASSWORD=minioadmin" \
  minio/minio server /data --console-address ":9001"

# 2. Create bucket
mc mb local/hx-sync

# 3. Initialize hx sync
hx sync init --store s3://hx-sync?endpoint=localhost:9000&path_style=true&access_key=minioadmin&secret_key=minioadmin

# 4. Test and sync
hx sync status
hx sync push
```

---

## ⚙️ Configuration Options

### S3 Store Configuration

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `bucket` | S3 bucket name | `my-hx-sync` | ✅ |
| `prefix` | Path prefix in bucket | `hx-sync` | ❌ (default: none) |
| `region` | AWS region | `us-west-2` | ✅ (AWS) |
| `endpoint` | Custom endpoint | `localhost:9000` | ❌ (MinIO) |
| `path_style` | Use path-style URLs | `true` | ❌ (MinIO) |
| `access_key` | AWS access key | `AKIA...` | ❌ (env/profile) |
| `secret_key` | AWS secret key | `secret` | ❌ (env/profile) |

### Configuration Methods

#### 1. URL Configuration (Recommended)
```bash
# AWS S3
hx sync init --store "s3://my-bucket/hx-sync?region=us-west-2"

# MinIO
hx sync init --store "s3://hx-sync?endpoint=localhost:9000&path_style=true&access_key=minioadmin&secret_key=minioadmin"

# Wasabi
hx sync init --store "s3://my-wasabi-bucket/hx-sync?endpoint=s3.wasabisys.com&region=us-east-1"
```

#### 2. Environment Variables
```bash
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_DEFAULT_REGION="us-west-2"

hx sync init --store "s3://my-bucket/hx-sync"
```

#### 3. AWS Profile
```bash
# Configure AWS profile
aws configure --profile hx

# Use profile
export AWS_PROFILE=hx
hx sync init --store "s3://my-bucket/hx-sync"
```

---

## 🔄 Migration from Folder Store

### Backup Current State
```bash
# Export current history
hx export --file hx-backup.json

# Note current sync location
hx sync status
```

### Migration Process
```bash
# 1. Initialize new S3 store
hx sync init --store "s3://new-bucket/hx-sync?region=us-west-2"

# 2. Push all history to S3
hx sync push

# 3. Verify migration
hx sync status
hx find "test query"  # Verify search works

# 4. Clean up old folder store (optional)
rm -rf ~/.local/share/hx/sync
```

### Multi-Device Migration
```bash
# On each device:
hx sync init --store "s3://shared-bucket/hx-sync?region=us-west-2"
hx sync pull
```

---

## 🔧 Advanced Configuration

### Performance Tuning

```bash
# Environment variables for performance
export HX_SYNC_TIMEOUT=30s          # Request timeout
export HX_SYNC_MAX_RETRIES=5        # Retry attempts
export HX_SYNC_CONCURRENT_UPLOADS=3 # Concurrent uploads

# Apply configuration
hx sync push
```

### Security Settings

```bash
# Use IAM roles (recommended for production)
export AWS_ROLE_ARN="arn:aws:iam::account:role/HxSyncRole"
export AWS_WEB_IDENTITY_TOKEN_FILE="/path/to/token"

# Use temporary credentials
aws sts assume-role --role-arn $AWS_ROLE_ARN --role-session-name hx-sync
```

### Cost Optimization

```bash
# Enable lifecycle rules (AWS CLI)
aws s3api put-bucket-lifecycle-configuration \
  --bucket your-hx-sync-bucket \
  --lifecycle-configuration file://lifecycle.json

# lifecycle.json example:
{
  "Rules": [
    {
      "ID": "hx-history-lifecycle",
      "Status": "Enabled",
      "Transitions": [
        {
          "Days": 30,
          "StorageClass": "STANDARD_IA"
        },
        {
          "Days": 90,
          "StorageClass": "GLACIER"
        }
      ]
    }
  ]
}
```

---

## 🔍 Troubleshooting

### Connection Issues

#### "Unable to locate credentials"
```bash
# Check credentials
aws sts get-caller-identity

# Set credentials explicitly
export AWS_ACCESS_KEY_ID="your-key"
export AWS_SECRET_ACCESS_KEY="your-secret"

# Or use profile
export AWS_PROFILE=your-profile
```

#### "Connection refused" (MinIO)
```bash
# Check MinIO status
docker ps | grep minio

# Verify endpoint
curl http://localhost:9000/minio/health/live

# Check network connectivity
telnet localhost 9000
```

#### "Access Denied"
```bash
# Verify bucket permissions
aws s3 ls s3://your-bucket

# Check IAM policies
aws iam get-user-policy --user-name your-user --policy-name hx-policy

# Test with explicit credentials
hx sync init --store "s3://bucket?access_key=key&secret_key=secret&region=us-west-2"
```

### Performance Issues

#### Slow Sync Speed
```bash
# Check concurrent uploads
export HX_SYNC_CONCURRENT_UPLOADS=5

# Increase timeout
export HX_SYNC_TIMEOUT=60s

# Monitor progress
hx sync status --verbose
```

#### High S3 Costs
```bash
# Monitor usage
aws s3 ls --human-readable --summarize --recursive s3://your-bucket

# Enable compression
export HX_SYNC_COMPRESSION=true

# Review lifecycle rules
aws s3api get-bucket-lifecycle-configuration --bucket your-bucket
```

### Sync Conflicts

#### "Sequence number conflict"
```bash
# Force refresh from remote
hx sync pull --force

# Or push local changes
hx sync push --force

# Check status
hx sync status --detailed
```

#### "Manifest corruption detected"
```bash
# Reset sync state
hx sync reset

# Re-initialize
hx sync init --store "s3://bucket/hx-sync?region=us-west-2"

# Pull fresh state
hx sync pull
```

---

## 📊 Monitoring and Maintenance

### Sync Status
```bash
# Basic status
hx sync status

# Detailed status
hx sync status --verbose

# Check last sync
hx sync status --last-sync
```

### Storage Usage
```bash
# Local storage
hx status --storage

# S3 usage (AWS CLI)
aws s3 ls --human-readable --summarize --recursive s3://your-bucket

# Cost estimation (AWS CLI)
aws ce get-cost-and-usage --time-period Start=2023-01-01,End=2023-01-31 --filter file://cost-filter.json
```

### Backup and Recovery
```bash
# Export backup
hx export --file hx-backup-$(date +%Y%m%d).json

# Import backup
hx import --file hx-backup-20230301.json

# Verify backup integrity
hx find "test query" --after "2023-03-01"
```

---

## 🚀 Best Practices

### Security
- Use IAM roles instead of access keys when possible
- Enable S3 bucket encryption
- Use HTTPS endpoints only
- Rotate credentials regularly
- Enable S3 access logging

### Performance
- Use appropriate S3 storage classes
- Enable multipart uploads for large histories
- Configure appropriate timeouts and retries
- Monitor S3 costs and usage

### Reliability
- Test restore procedures regularly
- Keep local backups of critical history
- Use S3 versioning for important data
- Monitor sync status and errors

---

## 📚 Additional Resources

- [AWS S3 Documentation](https://docs.aws.amazon.com/s3/)
- [MinIO Documentation](https://docs.min.io/)
- [Wasabi S3 Documentation](https://wasabi-support.zendesk.com/hc/en-us)
- [HX Architecture Specs](../architecture/)
- [Troubleshooting Runbooks](../runbooks/)

---

*Last updated: 2026-03-01*
