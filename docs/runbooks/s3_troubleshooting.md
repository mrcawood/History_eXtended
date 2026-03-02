# S3 Sync Troubleshooting

**Phase 2B** - Comprehensive troubleshooting guide for S3-compatible sync  
**Last Updated**: 2026-03-01

---

## 🔍 Quick Diagnosis

### Health Check Commands

```bash
# Overall sync status
hx sync status --verbose

# Test store connectivity
hx sync test-connection

# Diagnose sync issues
hx sync diagnose

# Check configuration
hx config show
```

### Common Error Categories

| Error Type | Common Causes | Quick Fix |
|------------|---------------|-----------|
| **Connection** | Network, credentials, endpoint | Check credentials, test endpoint |
| **Permission** | IAM policies, bucket permissions | Verify bucket access, update policies |
| **Performance** | Large history, network latency | Increase concurrency, optimize settings |
| **Conflict** | Concurrent sync, sequence issues | Force refresh, reset sync state |
| **Corruption** | Network interruption, storage issues | Reset sync, re-initialize |

---

## 🔌 Connection Issues

### "Unable to locate credentials"

#### Symptoms
```
Error: Unable to locate credentials
hx sync init failed: no credential providers
```

#### Causes
- Missing AWS credentials
- Incorrect profile configuration
- Environment variables not set

#### Solutions

**1. Set Environment Variables**
```bash
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_DEFAULT_REGION="us-west-2"

hx sync init --store "s3://bucket/prefix"
```

**2. Configure AWS Profile**
```bash
aws configure --profile hx
# Enter access key, secret key, region

export AWS_PROFILE=hx
hx sync init --store "s3://bucket/prefix"
```

**3. Use IAM Role**
```bash
export AWS_ROLE_ARN="arn:aws:iam::account:role/HxSyncRole"
export AWS_WEB_IDENTITY_TOKEN_FILE="/path/to/token"

hx sync init --store "s3://bucket/prefix"
```

**4. Explicit Credentials in URL**
```bash
hx sync init --store "s3://bucket?access_key=key&secret_key=secret&region=us-west-2"
```

### "Connection refused" / "Timeout"

#### Symptoms
```
Error: connection refused
Error: dial tcp: connection refused
Error: timeout waiting for response
```

#### Causes
- Endpoint incorrect or unreachable
- Firewall blocking connection
- Service not running

#### Solutions

**1. Verify Endpoint**
```bash
# Test with curl
curl -I https://your-bucket.s3.us-west-2.amazonaws.com

# For MinIO
curl -I http://localhost:9000/minio/health/live

# Test with AWS CLI
aws s3 ls s3://your-bucket
```

**2. Check Network Connectivity**
```bash
# Test basic connectivity
ping s3.amazonaws.com
ping your-endpoint.com

# Test port connectivity
telnet s3.amazonaws.com 443
telnet localhost 9000
```

**3. Verify Service Status**
```bash
# For MinIO Docker
docker ps | grep minio
docker logs hx-minio

# Restart if needed
docker restart hx-minio
```

**4. Adjust Timeouts**
```bash
export HX_SYNC_CONNECT_TIMEOUT=30s
export HX_SYNC_READ_TIMEOUT=60s
export HX_SYNC_TOTAL_TIMEOUT=300s

hx sync push
```

### "SSL/TLS Handshake Failed"

#### Symptoms
```
Error: TLS handshake failed
Error: certificate signed by unknown authority
```

#### Causes
- Custom CA certificates
- Proxy SSL inspection
- Outdated certificate bundle

#### Solutions

**1. Update CA Bundle**
```bash
export AWS_CA_BUNDLE="/path/to/ca-bundle.crt"
export SSL_CERT_FILE="/path/to/ca-bundle.crt"
```

**2. Configure Proxy**
```bash
export HTTPS_PROXY="http://proxy.company.com:8080"
export HTTP_PROXY="http://proxy.company.com:8080"
export NO_PROXY="localhost,127.0.0.1,s3.amazonaws.com"
```

**3. Disable SSL Verification (Development Only)**
```bash
export AWS_DISABLE_SSL=true
```

---

## 🔐 Permission Issues

### "Access Denied"

#### Symptoms
```
Error: Access Denied
Error: 403 Forbidden
hx sync push failed: access denied
```

#### Causes
- Insufficient IAM permissions
- Bucket policy restrictions
- Incorrect bucket name

#### Solutions

**1. Verify Bucket Access**
```bash
aws s3 ls s3://your-bucket
aws s3 ls s3://your-bucket/prefix/
```

**2. Check IAM Permissions**
```bash
# Get current user
aws sts get-caller-identity

# Check user policies
aws iam list-attached-user-policies --user-name your-user

# Check role policies (if using role)
aws iam list-attached-role-policies --role-name your-role
```

**3. Required IAM Policy**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket",
        "s3:GetBucketLocation"
      ],
      "Resource": [
        "arn:aws:s3:::your-bucket",
        "arn:aws:s3:::your-bucket/*"
      ]
    }
  ]
}
```

**4. Bucket Policy Check**
```bash
aws s3api get-bucket-policy --bucket your-bucket
```

### "Forbidden" / "403"

#### Symptoms
```
Error: 403 Forbidden
Error: operation not permitted
```

#### Causes
- Cross-region access restrictions
- VPC endpoint restrictions
- Bucket ownership changes

#### Solutions

**1. Check Bucket Region**
```bash
aws s3api get-bucket-location --bucket your-bucket
```

**2. Use Correct Region**
```bash
export AWS_DEFAULT_REGION="us-west-2"
hx sync init --store "s3://bucket/prefix?region=us-west-2"
```

**3. Verify Bucket Ownership**
```bash
aws s3api list-buckets | grep your-bucket
```

---

## ⚡ Performance Issues

### Slow Sync Speed

#### Symptoms
- Sync operations taking minutes/hours
- High CPU/memory usage
- Network timeouts

#### Solutions

**1. Increase Concurrency**
```bash
export HX_SYNC_CONCURRENT_UPLOADS=5
export HX_SYNC_CONCURRENT_DOWNLOADS=10
export HX_SYNC_MAX_CONNECTIONS=15

hx sync push
```

**2. Optimize Chunk Size**
```bash
export HX_SYNC_CHUNK_SIZE=128MB
export HX_SYNC_MAX_MEMORY_USAGE=2GB

hx sync push
```

**3. Enable Compression**
```bash
export HX_SYNC_COMPRESSION=true
export HX_SYNC_COMPRESSION_LEVEL=6

hx sync push
```

**4. Monitor Progress**
```bash
hx sync status --verbose --progress
```

### High Memory Usage

#### Symptoms
- Out of memory errors
- System becomes unresponsive
- Swap usage increases

#### Solutions

**1. Reduce Memory Limits**
```bash
export HX_SYNC_MAX_MEMORY_USAGE=512MB
export HX_SYNC_CHUNK_SIZE=32MB

hx sync push
```

**2. Reduce Concurrency**
```bash
export HX_SYNC_CONCURRENT_UPLOADS=1
export HX_SYNC_CONCURRENT_DOWNLOADS=2

hx sync push
```

**3. Use Streaming Mode**
```bash
export HX_SYNC_STREAMING=true
hx sync push
```

### High S3 Costs

#### Symptoms
- Unexpected AWS bill
- High data transfer costs
- Many API requests

#### Solutions

**1. Monitor Usage**
```bash
# Check S3 usage
aws s3 ls --human-readable --summarize --recursive s3://your-bucket

# Check costs
aws ce get-cost-and-usage --time-period Start=2023-01-01,End=2023-01-31
```

**2. Optimize Storage Class**
```bash
# Lifecycle rule for cost optimization
aws s3api put-bucket-lifecycle-configuration \
  --bucket your-bucket \
  --lifecycle-configuration '{
    "Rules": [
      {
        "ID": "hx-cost-optimization",
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
  }'
```

**3. Reduce API Calls**
```bash
export HX_SYNC_BATCH_SIZE=5000
export HX_SYNC_MANIFEST_CACHE_TTL=30m

hx sync push
```

---

## 🔄 Sync Conflicts

### "Sequence Number Conflict"

#### Symptoms
```
Error: sequence number conflict
Error: manifest sequence regression detected
hx sync push failed: sequence conflict
```

#### Causes
- Concurrent sync operations
- Network interruptions
- Clock synchronization issues

#### Solutions

**1. Force Refresh**
```bash
hx sync pull --force
hx sync push
```

**2. Reset Sync State**
```bash
hx sync reset --local-only
hx sync pull
hx sync push
```

**3. Check Clock Sync**
```bash
# Verify system time
date
timedatectl status

# Sync time (Linux)
sudo ntpdate -s time.nist.gov
```

### "Manifest Corruption Detected"

#### Symptoms
```
Error: manifest corruption detected
Error: invalid manifest format
hx sync pull failed: manifest corrupted
```

#### Causes
- Network interruption during manifest transfer
- Storage corruption
- Version incompatibility

#### Solutions

**1. Reset Sync State**
```bash
hx sync reset
hx sync init --store "s3://bucket/prefix?region=us-west-2"
hx sync pull
```

**2. Clear Manifest Cache**
```bash
rm -rf ~/.local/share/hx/sync/cache/*
hx sync pull --force
```

**3. Verify Manifest Integrity**
```bash
hx sync verify --manifest
```

### "Object Not Found"

#### Symptoms
```
Error: object not found
Error: 404 Not Found
hx sync pull failed: missing object
```

#### Causes
- Object deleted externally
- Manifest references non-existent objects
- Replication lag

#### Solutions

**1. Re-sync from Scratch**
```bash
hx sync reset
hx sync pull --full
```

**2. Check Object Existence**
```bash
aws s3 ls s3://your-bucket/prefix/objects/
```

**3. Repair Manifest**
```bash
hx sync repair --manifest
```

---

## 🛠️ Recovery Procedures

### Complete Sync Reset

#### When to Use
- Persistent sync conflicts
- Corrupted sync state
- Migration between storage types

#### Procedure
```bash
# 1. Export current data (backup)
hx export --file hx-backup-$(date +%Y%m%d).json

# 2. Reset sync state
hx sync reset

# 3. Re-initialize sync
hx sync init --store "s3://new-bucket/prefix?region=us-west-2"

# 4. Pull fresh state
hx sync pull

# 5. Push local changes
hx sync push

# 6. Verify integrity
hx sync verify
```

### Emergency Recovery

#### When to Use
- Complete data loss
- Storage backend failure
- Major corruption

#### Procedure
```bash
# 1. Stop all hx processes
pkill -f hx
pkill -f hxd

# 2. Backup corrupted data
mv ~/.local/share/hx ~/.local/share/hx.backup.$(date +%Y%m%d)

# 3. Start fresh
hx sync init --store "s3://bucket/prefix?region=us-west-2"

# 4. Import from backup (if available)
hx import --file hx-backup-20230301.json

# 5. Verify recovery
hx status
hx find "test query"
```

### Storage Backend Migration

#### When to Use
- Moving from folder to S3 store
- Changing S3 providers
- Reorganizing bucket structure

#### Procedure
```bash
# 1. Export current data
hx export --file migration-backup.json

# 2. Note current sync status
hx sync status > sync-status-before.txt

# 3. Initialize new backend
hx sync init --store "s3://new-bucket/prefix?region=us-west-2"

# 4. Push all data
hx sync push --full

# 5. Verify migration
hx sync status
hx find "test query"

# 6. Clean up old backend (optional)
# rm -rf /path/to/old/sync/folder
```

---

## 🔧 Advanced Diagnostics

### Network Diagnostics

```bash
# Test S3 connectivity
aws s3 ls s3://your-bucket

# Test endpoint reachability
curl -v https://your-endpoint.com

# Trace network path
traceroute s3.amazonaws.com

# Check DNS resolution
nslookup s3.amazonaws.com
dig s3.amazonaws.com
```

### Performance Diagnostics

```bash
# Monitor system resources
htop
iotop
iftop

# Check sync performance
hx sync status --verbose --timing

# Profile sync operations
HX_PROFILE=cpu hx sync push
```

### Storage Diagnostics

```bash
# Check S3 bucket status
aws s3api head-bucket --bucket your-bucket

# List all objects
aws s3 ls --recursive s3://your-bucket

# Check bucket size
aws s3 ls --human-readable --summarize --recursive s3://your-bucket

# Verify object integrity
aws s3api head-object --bucket your-bucket --key path/to/object
```

### Configuration Diagnostics

```bash
# Show effective configuration
hx config show

# Validate configuration
hx config validate

# Test encryption
hx crypto test

# Check permissions
hx sync test-permissions
```

---

## 📞 Getting Help

### Debug Information Collection

```bash
# Collect debug information
hx sync diagnose > hx-diagnose.txt 2>&1
hx config show >> hx-diagnose.txt
hx status --verbose >> hx-diagnose.txt

# Include system information
uname -a >> hx-diagnose.txt
go version >> hx-diagnose.txt
aws --version >> hx-diagnose.txt
```

### Log Analysis

```bash
# Enable debug logging
export HX_LOG_LEVEL=debug

# Run with logging
hx sync push 2>&1 | tee hx-sync.log

# Analyze logs
grep -i error hx-sync.log
grep -i warning hx-sync.log
tail -f hx-sync.log
```

### Support Resources

- [HX Documentation](../README.md)
- [S3 Sync User Guide](../user_guide/s3_sync.md)
- [Configuration Reference](../configuration/reference.md)
- [GitHub Issues](https://github.com/mrcawood/History_eXtended/issues)

---

## 📋 Troubleshooting Checklist

### Before Reporting Issues

- [ ] Check network connectivity
- [ ] Verify credentials and permissions
- [ ] Update to latest version
- [ ] Review configuration
- [ ] Check system resources
- [ ] Collect debug information

### Information to Include

- HX version (`hx --version`)
- Operating system (`uname -a`)
- Configuration (`hx config show`)
- Error messages (full output)
- Steps to reproduce
- Debug logs (`HX_LOG_LEVEL=debug hx sync push`)

---

*Last updated: 2026-03-01*
