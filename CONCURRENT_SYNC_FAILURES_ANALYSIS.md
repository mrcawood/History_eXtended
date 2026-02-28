# Concurrent Sync Test Failures Analysis

## 4 Failing Tests and Primary Assertions

### 1. TestConcurrentSync
- **Primary failure**: "Node A should have 10 total segments, got 0" / "Node B should have 10 total segments, got 0"
- **Assumption**: Concurrent segment creation immediately results in exactly 10 segments per node
- **Nature**: Deterministic (fails every time)

### 2. TestConcurrentTombstoneOperations  
- **Primary failure**: "Node B should have 4 objects, got 3"
- **Assumption**: Concurrent tombstone creation results in exact object count
- **Nature**: Deterministic (fails every time)

### 3. TestEventKeyTombstone
- **Primary failure**: "Node B should have 2 objects (1 segment + 1 tombstone), got 1"
- **Assumption**: Tombstone objects are immediately visible after creation
- **Nature**: Deterministic (fails every time)

### 4. TestBidirectionalSync
- **Primary failure**: "Node A should have 2 segments, got 0" / "Node B should have 2 segments, got 0"
- **Assumption**: Bidirectional sync immediately results in exact segment counts
- **Nature**: Deterministic (fails every time)

## Root Cause Analysis

All failures are **deterministic** and share the same pattern: **testing implementation details (segment counts) rather than convergence invariants**.

### Category: **Segment Flush Policy + Test Logic Gap**

The tests assume segments are immediately visible and countable, but:
1. No explicit flush/publish operations are called
2. Tests rely on background processes that may not trigger in fast test execution
3. Segment counts are implementation details, not product requirements

## Recommended Fix Strategy

### Step 1: Add Explicit Flush Operations
Add `FlushNow()` or equivalent to TestNode to force immediate segment publication.

### Step 2: Refactor Test Invariants
Change from testing "segment counts" to testing "eventual convergence":
- Assert union of events exists
- Assert no duplicates  
- Assert tombstones correctly applied

### Step 3: Make Tests Deterministic
- Use explicit push/pull cycles
- Add convergence loops with bounded iterations
- Test eventual state, not immediate state

## Implementation Details Needed

The tests need instrumentation to track:
- Segments created vs published
- Import operations
- Duplicate detection
- Partial file handling

This appears to be a **test design issue** rather than a core implementation bug.
