# SyncStore API Documentation

**Phase 2B** - Developer guide for implementing custom sync backends  
**Last Updated**: 2026-03-01

---

## 📋 Overview

The `SyncStore` interface defines the contract for hx sync backends. This guide explains how to implement custom backends and integrate them with hx.

---

## 🔌 Interface Definition

### Core Interface

```go
// SyncStore is the backend contract for sync object storage.
type SyncStore interface {
    // List returns keys matching the given prefix.
    List(prefix string) ([]string, error)
    
    // Get retrieves the object at the given key.
    Get(key string) ([]byte, error)
    
    // PutAtomic atomically stores data at the given key.
    PutAtomic(key string, data []byte) error
}
```

### Key Format Contract

All backends must follow the key format defined in the Sync Storage Contract v0:

```
vaults/<vault_id>/objects/segments/<node_id>/<segment_id>.hxseg
vaults/<vault_id>/objects/blobs/<aa>/<bb>/<blob_hash>.hxblob
vaults/<vault_id>/objects/tombstones/<tombstone_id>.hxtomb
vaults/<vault_id>/objects/manifests/<node_id>.hxman
```

---

## 🏗️ Implementation Guide

### Basic Implementation Template

```go
package sync

import (
    "context"
    "errors"
    "time"
)

// CustomStore implements SyncStore interface
type CustomStore struct {
    // Backend-specific configuration
    endpoint string
    apiKey   string
    timeout  time.Duration
    
    // Internal state
    client *CustomClient
}

// NewCustomStore creates a new custom sync store
func NewCustomStore(config CustomConfig) (*CustomStore, error) {
    // Initialize backend client
    client, err := NewCustomClient(config.Endpoint, config.APIKey)
    if err != nil {
        return nil, err
    }
    
    return &CustomStore{
        endpoint: config.Endpoint,
        apiKey:   config.APIKey,
        timeout:  config.Timeout,
        client:   client,
    }, nil
}

// List implements SyncStore.List
func (s *CustomStore) List(prefix string) ([]string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
    defer cancel()
    
    // Call backend list API
    keys, err := s.client.ListObjects(ctx, prefix)
    if err != nil {
        return nil, fmt.Errorf("list objects: %w", err)
    }
    
    return keys, nil
}

// Get implements SyncStore.Get
func (s *CustomStore) Get(key string) ([]byte, error) {
    ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
    defer cancel()
    
    // Call backend get API
    data, err := s.client.GetObject(ctx, key)
    if err != nil {
        return nil, fmt.Errorf("get object: %w", err)
    }
    
    // Validate size limits
    if len(data) > MaxObjectSize {
        return nil, ErrObjectTooLarge
    }
    
    return data, nil
}

// PutAtomic implements SyncStore.PutAtomic
func (s *CustomStore) PutAtomic(key string, data []byte) error {
    ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
    defer cancel()
    
    // Validate input
    if err := ValidateKey(key); err != nil {
        return fmt.Errorf("invalid key: %w", err)
    }
    
    if len(data) > MaxObjectSize {
        return ErrObjectTooLarge
    }
    
    // Call backend put API
    err := s.client.PutObject(ctx, key, data)
    if err != nil {
        return fmt.Errorf("put object: %w", err)
    }
    
    return nil
}
```

### Configuration Structure

```go
// CustomConfig defines configuration for custom store
type CustomConfig struct {
    // Required fields
    Endpoint string `yaml:"endpoint" json:"endpoint"`
    APIKey   string `yaml:"api_key" json:"api_key"`
    
    // Optional fields
    Timeout    time.Duration `yaml:"timeout" json:"timeout"`
    MaxRetries int           `yaml:"max_retries" json:"max_retries"`
    
    // Backend-specific options
    Region      string            `yaml:"region" json:"region"`
    Compression bool              `yaml:"compression" json:"compression"`
    Metadata    map[string]string `yaml:"metadata" json:"metadata"`
}

// Validate configuration
func (c *CustomConfig) Validate() error {
    if c.Endpoint == "" {
        return errors.New("endpoint is required")
    }
    if c.APIKey == "" {
        return errors.New("api_key is required")
    }
    if c.Timeout == 0 {
        c.Timeout = 30 * time.Second
    }
    if c.MaxRetries == 0 {
        c.MaxRetries = 3
    }
    return nil
}
```

---

## 🔧 Advanced Features

### Retry Logic

```go
// RetryableStore wraps a SyncStore with retry logic
type RetryableStore struct {
    store     SyncStore
    config    RetryConfig
    backoff   BackoffStrategy
}

type RetryConfig struct {
    MaxAttempts     int
    InitialDelay    time.Duration
    MaxDelay        time.Duration
    BackoffFactor   float64
    RetryableErrors []error
}

func (r *RetryableStore) PutAtomic(key string, data []byte) error {
    var lastErr error
    
    for attempt := 0; attempt < r.config.MaxAttempts; attempt++ {
        if attempt > 0 {
            delay := r.backoff.Delay(attempt)
            time.Sleep(delay)
        }
        
        err := r.store.PutAtomic(key, data)
        if err == nil {
            return nil
        }
        
        // Check if error is retryable
        if !isRetryableError(err, r.config.RetryableErrors) {
            return err
        }
        
        lastErr = err
    }
    
    return fmt.Errorf("operation failed after %d attempts: %w", r.config.MaxAttempts, lastErr)
}
```

### Metrics and Monitoring

```go
// InstrumentedStore adds metrics to SyncStore operations
type InstrumentedStore struct {
    store   SyncStore
    metrics MetricsCollector
}

func (i *InstrumentedStore) List(prefix string) ([]string, error) {
    start := time.Now()
    defer func() {
        i.metrics.RecordDuration("list", time.Since(start))
    }()
    
    keys, err := i.store.List(prefix)
    if err != nil {
        i.metrics.RecordError("list", err)
    } else {
        i.metrics.RecordCount("list_keys", len(keys))
    }
    
    return keys, err
}
```

### Validation Layer

```go
// ValidatingStore adds input validation
type ValidatingStore struct {
    store SyncStore
    rules []ValidationRule
}

type ValidationRule interface {
    ValidateKey(key string) error
    ValidateData(data []byte) error
}

func (v *ValidatingStore) PutAtomic(key string, data []byte) error {
    // Apply validation rules
    for _, rule := range v.rules {
        if err := rule.ValidateKey(key); err != nil {
            return fmt.Errorf("key validation failed: %w", err)
        }
        if err := rule.ValidateData(data); err != nil {
            return fmt.Errorf("data validation failed: %w", err)
        }
    }
    
    return v.store.PutAtomic(key, data)
}
```

---

## 🧪 Testing Patterns

### Unit Testing

```go
// MockSyncStore for testing
type MockSyncStore struct {
    objects map[string][]byte
    errors  map[string]error
    mu      sync.RWMutex
}

func NewMockSyncStore() *MockSyncStore {
    return &MockSyncStore{
        objects: make(map[string][]byte),
        errors:  make(map[string]error),
    }
}

func (m *MockSyncStore) PutAtomic(key string, data []byte) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    if err, exists := m.errors["put"]; exists {
        return err
    }
    
    m.objects[key] = data
    return nil
}

func (m *MockSyncStore) Get(key string) ([]byte, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    if err, exists := m.errors["get"]; exists {
        return nil, err
    }
    
    data, exists := m.objects[key]
    if !exists {
        return nil, ErrObjectNotFound
    }
    
    return data, nil
}

func (m *MockSyncStore) List(prefix string) ([]string, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    if err, exists := m.errors["list"]; exists {
        return nil, err
    }
    
    var keys []string
    for key := range m.objects {
        if strings.HasPrefix(key, prefix) {
            keys = append(keys, key)
        }
    }
    
    return keys, nil
}

// SetError for testing error conditions
func (m *MockSyncStore) SetError(operation string, err error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.errors[operation] = err
}
```

### Integration Testing

```go
// TestCustomStoreIntegration tests against real backend
func TestCustomStoreIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    // Setup test configuration
    config := CustomConfig{
        Endpoint: os.Getenv("CUSTOM_ENDPOINT"),
        APIKey:   os.Getenv("CUSTOM_API_KEY"),
        Timeout:  10 * time.Second,
    }
    
    if err := config.Validate(); err != nil {
        t.Skipf("Invalid test configuration: %v", err)
    }
    
    // Create store
    store, err := NewCustomStore(config)
    require.NoError(t, err)
    
    // Test basic operations
    testKey := "test/vault/objects/segments/node/test.hxseg"
    testData := []byte("test data")
    
    // Test PutAtomic
    err = store.PutAtomic(testKey, testData)
    require.NoError(t, err)
    
    // Test Get
    result, err := store.Get(testKey)
    require.NoError(t, err)
    assert.Equal(t, testData, result)
    
    // Test List
    keys, err := store.List("test/vault/objects/segments/")
    require.NoError(t, err)
    assert.Contains(t, keys, testKey)
    
    // Cleanup
    err = store.Delete(testKey) // if supported
    if err != nil {
        t.Logf("Cleanup failed: %v", err)
    }
}
```

### Property-Based Testing

```go
// Property-based tests for SyncStore interface
func TestSyncStoreProperties(t *testing.T) {
    store := NewMockSyncStore()
    
    // Property: Put followed by Get returns original data
    quick.Check(func(key string, data []byte) bool {
        err := store.PutAtomic(key, data)
        if err != nil {
            return false
        }
        
        result, err := store.Get(key)
        if err != nil {
            return false
        }
        
        return bytes.Equal(data, result)
    }, nil)
    
    // Property: List returns keys with matching prefix
    quick.Check(func(prefix string, keys []string) bool {
        // Put test data
        for i, key := range keys {
            data := []byte(fmt.Sprintf("data-%d", i))
            store.PutAtomic(key, data)
        }
        
        // List with prefix
        results, err := store.List(prefix)
        if err != nil {
            return false
        }
        
        // Verify all results have prefix
        for _, result := range results {
            if !strings.HasPrefix(result, prefix) {
                return false
            }
        }
        
        return true
    }, nil)
}
```

---

## 🔌 Integration with hx

### Store Registration

```go
// StoreFactory creates SyncStore instances
type StoreFactory func(config map[string]interface{}) (SyncStore, error)

var storeFactories = make(map[string]StoreFactory)

// RegisterStore registers a new store type
func RegisterStore(name string, factory StoreFactory) {
    storeFactories[name] = factory
}

// CreateStore creates a store from configuration
func CreateStore(storeConfig string) (SyncStore, error) {
    // Parse store configuration
    parts := strings.SplitN(storeConfig, ":", 2)
    if len(parts) != 2 {
        return nil, errors.New("invalid store format")
    }
    
    storeType := parts[0]
    storeURL := parts[1]
    
    factory, exists := storeFactories[storeType]
    if !exists {
        return nil, fmt.Errorf("unknown store type: %s", storeType)
    }
    
    // Parse URL into config
    config, err := parseStoreURL(storeURL, storeType)
    if err != nil {
        return nil, err
    }
    
    return factory(config)
}

// Initialize built-in stores
func init() {
    RegisterStore("s3", NewS3Store)
    RegisterStore("folder", NewFolderStore)
    RegisterStore("custom", NewCustomStore)
}
```

### Configuration Parsing

```go
// parseStoreURL parses store URL into configuration
func parseStoreURL(storeURL, storeType string) (map[string]interface{}, error) {
    u, err := url.Parse(storeURL)
    if err != nil {
        return nil, fmt.Errorf("invalid store URL: %w", err)
    }
    
    config := make(map[string]interface{})
    
    // Common fields
    config["bucket"] = u.Host
    config["prefix"] = strings.TrimPrefix(u.Path, "/")
    
    // Parse query parameters
    for key, values := range u.Query() {
        if len(values) > 0 {
            config[key] = values[0]
        }
    }
    
    // Store-specific parsing
    switch storeType {
    case "s3":
        config["region"] = config["region"]
        config["endpoint"] = config["endpoint"]
        config["path_style"] = config["path_style"]
    case "custom":
        config["endpoint"] = config["endpoint"]
        config["api_key"] = config["api_key"]
    }
    
    return config, nil
}
```

---

## 📚 Examples

### Simple File Backend

```go
// FileStore implements SyncStore using local filesystem
type FileStore struct {
    baseDir string
    mu      sync.RWMutex
}

func NewFileStore(baseDir string) (*FileStore, error) {
    if err := os.MkdirAll(baseDir, 0755); err != nil {
        return nil, err
    }
    return &FileStore{baseDir: baseDir}, nil
}

func (f *FileStore) PutAtomic(key string, data []byte) error {
    f.mu.Lock()
    defer f.mu.Unlock()
    
    // Write to temporary file first
    tempPath := filepath.Join(f.baseDir, key+".tmp")
    finalPath := filepath.Join(f.baseDir, key)
    
    if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
        return err
    }
    
    if err := os.WriteFile(tempPath, data, 0644); err != nil {
        return err
    }
    
    // Atomic rename
    return os.Rename(tempPath, finalPath)
}
```

### Memory Backend (Testing)

```go
// MemoryStore implements SyncStore in memory
type MemoryStore struct {
    objects map[string][]byte
    mu      sync.RWMutex
}

func NewMemoryStore() *MemoryStore {
    return &MemoryStore{
        objects: make(map[string][]byte),
    }
}

func (m *MemoryStore) PutAtomic(key string, data []byte) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    // Simulate network latency
    time.Sleep(10 * time.Millisecond)
    
    m.objects[key] = data
    return nil
}
```

### HTTP Backend

```go
// HTTPStore implements SyncStore over HTTP
type HTTPStore struct {
    baseURL string
    client  *http.Client
    headers map[string]string
}

func NewHTTPStore(baseURL string) *HTTPStore {
    return &HTTPStore{
        baseURL: baseURL,
        client:  &http.Client{Timeout: 30 * time.Second},
        headers: map[string]string{
            "User-Agent": "hx-sync/1.0",
        },
    }
}

func (h *HTTPStore) PutAtomic(key string, data []byte) error {
    url := fmt.Sprintf("%s/%s", h.baseURL, key)
    
    req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
    if err != nil {
        return err
    }
    
    for k, v := range h.headers {
        req.Header.Set(k, v)
    }
    
    resp, err := h.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
    }
    
    return nil
}
```

---

## 🔍 Best Practices

### Error Handling

```go
// Use wrapped errors with context
return fmt.Errorf("put object %s: %w", key, err)

// Define specific error types
var (
    ErrObjectNotFound = errors.New("object not found")
    ErrObjectTooLarge = errors.New("object too large")
    ErrInvalidKey    = errors.New("invalid key format")
)

// Use error types for handling
if errors.Is(err, ErrObjectNotFound) {
    // Handle not found
}
```

### Performance

```go
// Use connection pooling
client := &http.Client{
    Transport: &http.Transport{
        MaxIdleConns:        10,
        MaxIdleConnsPerHost: 5,
        IdleConnTimeout:     90 * time.Second,
    },
}

// Use streaming for large objects
func (s *Store) Get(key string) (io.ReadCloser, error) {
    // Return stream instead of []byte for large objects
}
```

### Security

```go
// Validate inputs
func validateKey(key string) error {
    if strings.Contains(key, "..") {
        return ErrInvalidKey
    }
    if len(key) > MaxKeyLength {
        return ErrInvalidKey
    }
    return nil
}

// Use secure defaults
config := DefaultConfig()
config.Timeout = 30 * time.Second
config.MaxRetries = 3
```

---

## 📋 Testing Checklist

### Unit Tests
- [ ] All SyncStore methods tested
- [ ] Error conditions covered
- [ ] Edge cases handled
- [ ] Input validation tested

### Integration Tests
- [ ] Real backend connectivity
- [ ] Performance benchmarks
- [ ] Concurrent access patterns
- [ ] Large object handling

### Property Tests
- [ ] Put/Get roundtrip property
- [ ] List prefix matching property
- [ ] Atomic operations property
- [ ] Error handling properties

---

## 📞 Support

- [HX Sync Storage Contract](../architecture/sync_storage_contract_v0.md)
- [S3Store Implementation](../architecture/s3store.md)
- [Testing Guidelines](../validation/)

---

*Last updated: 2026-03-01*
