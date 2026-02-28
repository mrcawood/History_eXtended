# Integration Test Encryption Issues - Context for Supervisor

## Current Status
We have implemented a comprehensive integration test suite for Phase 2A sync functionality with the following test files:
- `test_utils/testnode.go` - Test framework with isolated node simulation
- `two_node_converge_test.go` - Two-node synchronization tests
- `tombstone_propagation_test.go` - Tombstone propagation tests
- `concurrent_sync_test.go` - Concurrent sync operations
- `encryption_roundtrip_test.go` - Encryption/decryption tests

## Encryption Implementation Details

### Current Test Setup
- **TestNode**: Creates isolated test nodes with FolderStore, SQLite DB, and encryption keys
- **Key Generation**: Currently using deterministic keys based on node name:
  ```go
  key := make([]byte, 32)
  for i := range key {
      key[i] = byte(i + len(name))
  }
  ```
- **Encryption API**: Using `sync.EncodeSegment()`, `sync.DecodeObject()`, `sync.DecryptObject()`

### Encryption Test Behavior
The `TestEncryptionRoundtrip` test expects:
1. Node A encrypts data with its key
2. Node B should NOT be able to decrypt with its own different key
3. Node B should be able to decrypt after receiving Node A's key (key exchange simulation)

**Actual Behavior**: Node B CAN decrypt data encrypted with Node A's key, suggesting both nodes are using the same key.

### Test Results Summary
```
=== RUN   TestEncryptionRoundtrip
    encryption_roundtrip_test.go:75: Node B should not be able to decrypt data encrypted with node A's key
--- FAIL: TestEncryptionRoundtrip (0.00s)

=== RUN   TestTamperDetection
    encryption_roundtrip_test.go:146: Tampered data correctly failed to decrypt
--- PASS: TestTamperDetection (0.00s)

=== RUN   TestDifferentObjectTypes  
--- PASS: TestDifferentObjectTypes (0.00s)

=== RUN   TestCrossNodeKeyExchange
    encryption_roundtrip_test.go:305: Node B's original key should not be able to decrypt
--- FAIL: TestCrossNodeKeyExchange (0.00s)
```

## Key Issues Identified

### 1. Key Generation Problem
- **Expected**: Different nodes should have different encryption keys
- **Actual**: Evidence suggests nodes may be using identical keys
- **Root Cause**: The deterministic key generation may not be creating truly unique keys per node

### 2. Test Logic Questions
- Should the integration tests use different keys per node?
- Or should they simulate a real-world scenario where nodes share keys?
- Is the current key exchange simulation realistic?

### 3. API Usage Uncertainty
- Are we using `EncodeSegment/DecodeObject/DecryptObject` correctly?
- Should we be using a higher-level API for encryption/decryption?
- Is there a key management system we should be using instead of manual key handling?

## Code Context

### TestNode Key Generation
```go
// Create test encryption key (deterministic for testing)
// Different nodes get different keys for testing
key := make([]byte, 32)
for i := range key {
    key[i] = byte(i + len(name))
}
```

### Encryption Test Logic
```go
// Node B should NOT be able to decrypt with its own key
headerB, bodyB, err := hs.DecodeObject(retrievedData)
if err != nil {
    t.Fatalf("Failed to decode object: %v", err)
}
_, err = hs.DecryptObject(headerB, bodyB, nodeB.Key)
if err == nil {
    t.Error("Node B should not be able to decrypt data encrypted with node A's key")
}
```

## Questions for Supervisor

1. **Key Management Strategy**: Should integration tests use different keys per node, or simulate a shared key scenario?

2. **Test Design**: Is our current approach to testing encryption/decryption appropriate for the sync system's design?

3. **API Usage**: Are we using the encryption APIs correctly, or should we be using a different approach?

4. **Key Generation**: What's the recommended way to generate test encryption keys for integration tests?

5. **Real-world Simulation**: How should we model real-world key exchange scenarios in tests?

## Additional Context
- Other tests (tamper detection, object types) are passing, indicating basic encryption works
- The issue seems specific to multi-node key differentiation
- We're using XChaCha20-Poly1305 AEAD encryption as per the sync package design
- All tests use the same `TestNode` infrastructure, so fixing key generation will affect multiple tests

## Supervisor Direction Needed
Please provide guidance on:
1. Proper key management strategy for integration tests
2. Whether current test approach aligns with system design
3. Recommended fixes for encryption behavior
4. Any architectural considerations we may have missed
