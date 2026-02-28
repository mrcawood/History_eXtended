# Test and Documentation Review
**Date:** 2026-02-26  
**Scope:** Phase 2A sync; consistency with codebase

---

## 1. PROGRESS.md

**Status:** Updated.

- Phase 2A.4 (segment writer) and 2A.5 (CLI) marked complete.
- Current State reflects codec, FolderStore, importer, writer, CLI done.
- Commands list includes `hx sync init/status/push/pull`.
- Recent Changes updated.
- Proposed Next Step: integration tests.

---

## 2. README.md

**Status:** Updated.

- Features: multi-device sync added.
- Commands table: sync init, status, push, pull added.
- Aligned with `cmd/hx/main.go` usage string.

---

## 3. INSTALL.md

**Status:** Updated.

- New section: "Multi-device sync (Phase 2, hx sync)".
- Usage: init, push, pull, status with examples.
- Notes: store type, v0 plaintext, init required before push/pull.
- Matches current behavior (encrypt=false by default).

---

## 4. Sync Package Tests

### 4.1 Coverage Summary

| Component | Test | Status | Notes |
|-----------|------|--------|-------|
| Codec | TestEncodeDecodeSegment_Plaintext | ✓ | Wire format, DecodeObject |
| Codec | TestEncodeDecodeSegment_Encrypted | ✓ | AEAD roundtrip |
| Codec | TestTamperDetection | ✓ | Contract T5 |
| FolderStore | TestFolderStore_PutGet | ✓ | Get after PutAtomic |
| FolderStore | TestFolderStore_List | ✓ | List, ignores tmp/ |
| FolderStore | TestFolderStore_AtomicPublish | ✓ | tmp/ empty after rename |
| Importer | TestImport_Segment | Skip* | Needs FTS5 |
| Importer | TestImport_Idempotent | Skip* | Needs FTS5; Contract T4 |
| Writer | TestPush_PublishesSegment | Skip* | Needs FTS5 |

\* Skipped when SQLite built without FTS5. Run with `-tags sqlite_fts5` or on system with FTS5 for full coverage.

### 4.2 Gaps

| Gap | Priority | Notes |
|-----|----------|-------|
| Blob import test | Medium | Import path for .hxblob; hash verify, blob.Store, DB insert |
| Tombstone import test | Medium | Time-window delete, applied_tombstones, resurrection check |
| EncodeBlob / EncodeTombstone | Low | Codec coverage; tombstone used in import |
| End-to-end push→pull | High | Phase 2A.6 integration tests |
| DecodeObject + invalid header | Low | Rejects bad magic/version |

### 4.3 Consistency with Codebase

- **openDBWithTimeout:** Used in importer and writer tests; avoids hangs.
- **SegmentKey, BlobKey, TombstoneKey:** Used in tests; match store layout.
- **Store interface:** FolderStore implements List/Get/PutAtomic per contract.
- **Import/Export flow:** Import uses `vaults/<id>/objects/` prefix; writer uses SegmentKey.

---

## 5. scripts/validate.sh

**Status:** Unchanged; Phase 1 only.

- Validates A1–A7 (golden dataset, retention, etc.).
- Phase 2 sync not in scope.
- **Recommendation:** Add optional Phase 2 sync validation block when integration tests exist.

---

## 6. Contract Tests (docs/hx_sync_storage_contract_v0.md §10)

| Contract Test | Status |
|---------------|--------|
| T1 Two-node convergence | Pending (integration) |
| T2 Tombstone propagation | Pending (integration) |
| T3 Partial/corrupt object rejection | Partial; DecodeObject length checks; no explicit corrupt-file test |
| T4 Idempotent re-import | Covered by TestImport_Idempotent (when FTS5 available) |
| T5 Tamper detection | Covered by TestTamperDetection |

---

## 7. Recommendations

1. Run sync tests with FTS5: `make test-sync` or `go test -tags sqlite_fts5 ./internal/sync/...`
2. Add blob and tombstone import unit tests when FTS5 is available.
3. Implement Phase 2A.6 integration tests for T1 and T2.
4. Document FTS5 requirement for sync tests in INSTALL or README if needed.
