# Integration Test Assertion Library

## Core Convergence Invariants

### AssertConverged(nodeA, nodeB)
**Checks**: Both nodes have identical object sets
- Same number of objects
- Identical object keys
- No missing objects on either side

### AssertEventAbsent(node, eventCmd)
**Checks**: Specific event is not present on a node
- Scans all segments for the event command
- Fails if event is found anywhere
- Logs success when event is correctly absent

### AssertNoResurrection(node, segmentKey)
**Checks**: Deleted events cannot be resurrected
- Attempts to retrieve the segment
- Verifies segment is absent or properly tombstoned
- Ensures events cannot reappear after deletion

## Test Primitives

### FlushNow()
**Purpose**: Forces immediate publication of buffered segments/tombstones
- Finalizes any buffered data
- Writes complete objects to store
- Updates indexes/metadata

### SyncRound(peer)
**Purpose**: Performs complete sync cycle with peer node
- Flushes pending data
- Pushes all objects to peer
- Peer processes incoming data
- Peer flushes to ensure completion

## Bounded Convergence Pattern

```go
for i := 0; i < maxRounds; i++ {
    if err := nodeA.SyncRound(nodeB); err != nil {
        t.Fatalf("Sync round %d failed: %v", i, err)
    }
    if err := nodeB.SyncRound(nodeA); err != nil {
        t.Fatalf("Sync round %d failed: %v", i, err)
    }
    
    // Early convergence detection
    keysA, _ := nodeA.ListSegments()
    keysB, _ := nodeB.ListSegments()
    if len(keysA) == len(keysB) {
        break
    }
}

// Verify final convergence invariants
AssertConverged(t, nodeA, nodeB)
```

## Invariants Being Asserted

### Core Sync Invariants
- **Union**: Each node contains union of all events (minus tombstoned)
- **No Duplicates**: Uniqueness constraints maintained across sync
- **Order Independence**: Sync order doesn't affect final state
- **Eventual Consistency**: Convergence achieved through bounded rounds

### Tombstone Invariants
- **Propagation**: Tombstones created on one node appear on others
- **Wins**: Target events are removed and non-retrievable
- **No Resurrection**: Re-importing doesn't reintroduce deleted events

### Encryption Invariants
- **Same Vault**: Nodes in same vault can decrypt each other's data
- **Different Vault**: Nodes in different vaults cannot decrypt
- **Tamper Detection**: Corrupted data is rejected
- **Key Exchange**: Enrollment enables decryption

### Store Robustness Invariants
- **Junk Rejection**: Non-object files are ignored
- **Partial Rejection**: Truncated/corrupted objects are rejected
- **Disorder Tolerance**: Convergence works regardless of file order
- **Atomic Publish**: Only complete objects are imported

## Missing Invariant Check

**Import Idempotency Under Concurrent Imports**
- Not currently explicitly tested
- Should verify that concurrent import of the same object doesn't cause corruption
- Could be added with concurrent goroutine import test
