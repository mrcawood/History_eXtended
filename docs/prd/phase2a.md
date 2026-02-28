# HX Phase 2 PRD — Multi-Device Sync (Local-First, E2EE, Boring & Reliable)
**Date:** 2026-02-26  
**Status:** Draft (for iteration)  
**Owner:** Matt  
**Single wedge:** Multi-device sync + multi-device search  
**Primary deliverables:** Sync Storage Contract v0 + folder-store backend + deterministic import/merge

---

## 1) Problem statement

HX Phase 1 delivers a high-value local history index (SQLite + blobs). Real workflows span multiple machines (desktop/laptop, home workstation, SSH hosts). Without sync, history fragments and HX loses its “time machine” utility.

We want multi-device sync that is:
- secure by default (E2EE for non-trusted storage)
- resilient (offline-first, eventual convergence)
- low operational burden (no required server)
- safe (never sync a live SQLite DB file)

---

## 2) Goals and non-goals

### Goals (must-haves)
G1. **Multi-device convergence**: two devices independently capture history offline and converge after sync.  
G2. **SQLite is a derived index**: we do not sync SQLite DB files; we sync replication objects and import.  
G3. **E2EE** for the sync store by default (configurable for trusted local-only stores).  
G4. **Deterministic merges**: idempotent import; order-independent; no duplicates.  
G5. **Tombstones replicate**: `hx forget` propagates and prevents resurrection.  
G6. **Cross-device query**: once synced/imported, `hx find/query` searches across all imported history.  
G7. **Low friction**: first backend works with a “folder store” (NAS / Syncthing / removable media).  
G8. **Observable**: `hx sync status` reports what is pending, imported, skipped, and why.

### Non-goals (explicitly deferred)
N1. Team/multi-user tenancy and ACLs  
N2. Turnkey “HX Cloud” accounts, billing, hosted server  
N3. Real-time streaming sync (near-real-time); Phase 2 is periodic/pull-push  
N4. Syncing derived indexes (FTS rebuild state, embeddings)  
N5. GUI sync management  
N6. Agent mode (auto-run commands)  
N7. Integrations with Dropbox/Drive/OneDrive APIs (use folder backend + user tool)

---

## 3) “Widest path” architectural decisions (locked)

### D1. Replicate immutable objects, not the SQLite DB
- SQLite remains local and rebuildable.
- Sync store contains immutable **segments** and **blobs**, plus **tombstones** and optional **manifests**.

### D2. Object model
- **Segment**: append-only batch of events and metadata; immutable once published.
- **Blob**: content-addressed (sha256) compressed artifact payload.
- **Tombstone**: deletion intent that must win; prevents reintroduction.
- **Manifest** (optional in v0): per-node summary pointer for efficient incremental sync.

### D3. Merge semantics
- Merge = **set union of imported segments** + apply tombstones.
- Import is **idempotent** and **order-independent**.
- Tombstones always win.

### D4. Security posture
- Default to **E2EE** for objects in sync store (segments/blobs/tombstones/manifests).
- Key model: **vault master key** → per-object data key (envelope encryption).
- Device enrollment required to add a device to the vault.

### D5. Store interface abstraction
- Implement a generic `SyncStore` interface (list/get/put_atomic).
- Phase 2A implements `FolderStore`.
- Phase 2B can implement `S3Store` without changing replication format.

---

## 4) User experience (Phase 2A)

### Commands
- `hx sync init --store folder:/path/to/HXSync [--vault-name NAME]`
- `hx sync status`
- `hx sync push`
- `hx sync pull`
- `hx sync repair` (optional: re-import / reconcile)
- Flags:
  - `--no-encrypt` (allowed only for trusted local stores; off by default)
  - `--json` output
  - `--verbose` for import logs

### Expected UX
- User chooses a sync directory (NAS share, Syncthing folder, external drive).
- HX writes objects into the directory and imports objects from it.
- Once imported, HX queries automatically include synced history (no special flags).

---

## 5) Data and crypto requirements

### Vault and identity
- Each device has:
  - `node_id` (uuid)
  - device keypair (for enrollment metadata; optional in v0)
- Vault has:
  - vault id
  - vault master key (protected by OS keychain if available; fallback passphrase)

### Encryption (default)
- Object payloads are encrypted with AEAD:
  - recommended: XChaCha20-Poly1305 or AES-256-GCM
- Each object includes:
  - header (unencrypted minimal routing fields) + encrypted payload + auth tag
- Header integrity is protected via AEAD associated data.

---

## 6) Segmenting and publishing policy (Phase 2A)

### Segmenting (concept)
- Events are buffered and periodically flushed into a new segment.
- Segment is immutable after finalization.

### Flush triggers (configurable; values TBD)
- time-based (e.g., every 5 minutes)
- size-based (e.g., N events)
- explicit `hx sync push` flushes immediately

### Atomic publish
- Write to temp file → fsync → atomic rename to final name
- Import only finalized objects (never read temp)

---

## 7) Import policy

### Import loop
- Enumerate new objects in store.
- Verify integrity (size/checksum), then decrypt if enabled.
- For segments:
  - parse and insert events into local SQLite with idempotency keys
- For blobs:
  - ensure present in local blob cache by hash
- For tombstones:
  - apply to local DB and local blob cache; record applied tombstones; prevent resurrection

### Idempotency
- Segment import tracked by `(node_id, segment_id, segment_hash)`.
- Event uniqueness ensured by stable keys (see contract).
- Blob uniqueness by sha256.

---

## 8) Acceptance criteria (Phase 2)

A2.1 Two devices converge after offline capture; no duplicate events.  
A2.2 `hx find` on device B returns results captured on device A after push/pull.  
A2.3 `hx forget` on A propagates to B and removes data; cannot reappear after subsequent syncs.  
A2.4 Partial/incomplete objects do not import (integrity checks + atomic publish).  
A2.5 Encrypted store reveals no plaintext content; tampering causes import failure.  
A2.6 Sync never writes to or syncs a live SQLite file; DB remains local-only.  
A2.7 Status is observable: pending/imported/skipped counts reported with reasons.

---

## 9) Rollout plan

### Phase 2A (folder store)
- Implement Sync Storage Contract v0
- Implement FolderStore
- Implement segment writer + importer + tombstones
- Implement CLI sync commands + status
- Add integration tests for 2-node convergence and tombstone propagation

### Phase 2B (cloud store via S3-compatible)
- Implement S3Store using same contract
- Add manifest/cursor optimization to avoid expensive listings
- Harden retries/backoff

---

## 10) Deferred items (explicit)
- Live daemon “auto sync” mode (can be added later)
- QR code / slick enrollment UX
- Team sharing
- GUI

---

## 11) Open choices to finalize (Phase 2A)
- Segment flush thresholds (time vs count defaults)
- Tombstone model: time-window vs event-id granularity (support both?)
- Pin merge semantics (“pinned if any” vs “latest wins”)
- Key storage: keychain-first with passphrase fallback

---

## 12) Implementation agent checklist
- ✅ Read Sync Storage Contract v0
- ✅ Implement `SyncStore` + `FolderStore`
- ✅ Implement segment writer (finalize + publish)
- ✅ Implement importer (idempotent + order-independent)
- ✅ Implement tombstones (win and prevent resurrection)
- ✅ Implement `hx sync init/status/push/pull`
- ✅ Implement 2-node integration tests
- ✅ Add comprehensive robustness tests (corruption handling, non-blocking scan)
- ✅ Add production validation with defense-in-depth guarantees

## 13) Phase 2A Test Gate Status

**Status:** ✅ **GREEN** - All acceptance criteria met

### Validation Results:
- **15/15 integration tests passing** with race detection
- **Production importer validation** with defense-in-depth guarantees
- **Vault-based encryption model** implemented and tested
- **Atomic publish operations** with strict validation
- **Non-blocking scan behavior** despite corrupt objects
- **Pre-insert tombstone enforcement** preventing resurrection

### Test Coverage:
- ✅ Multi-device convergence (2-node, bidirectional)
- ✅ Vault encryption with device enrollment
- ✅ Tombstone propagation and enforcement
- ✅ Concurrent sync operations
- ✅ Corruption rejection and robustness
- ✅ Store scan resilience and disorder handling

**Ready for Phase 2B planning.**
