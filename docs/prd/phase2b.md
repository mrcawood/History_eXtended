# HX Phase 2B PRD — S3 Sync Store Backend + Manifest v0
**Date:** 2026-02-28  
**Status:** Draft (implementation-ready)  
**Phase:** 2B  
**Wedge:** Cloud-capable sync via **S3-compatible object storage** without changing the replication model  
**Depends on:** Phase 2A (FolderStore + importer validation + convergence tests)  
**Artifacts:** Sync Storage Contract v0 (unchanged) + Manifest v0 Spec + S3Store Spec

---

## 1) Objective

Add a new sync store backend: **S3Store**, enabling HX multi-device sync through S3-compatible object storage (AWS S3, MinIO, Wasabi, Backblaze B2 S3, etc.) while preserving Phase 2A guarantees:

- SQLite remains local-only (derived index)
- Objects are immutable (segments/blobs/tombstones)
- Import is idempotent, order-independent
- Tombstones always win (no resurrection)
- Default E2EE for all objects
- Corruption/partial handling is enforced in the production importer

---

## 2) Scope

### In scope (Phase 2B)
B1. Implement `S3Store` backend for the existing `SyncStore` interface.  
B2. Implement **Manifest v0** to avoid expensive bucket-wide listing on every sync.  
B3. Implement pull algorithm based on manifests: manifests → missing objects → import.  
B4. Implement robust network behavior: retries/backoff, timeouts, clear error categories.  
B5. Add S3 integration tests using a local S3-compatible service (MinIO) in CI/dev.

### Out of scope (defer)
D1. “HX Cloud” hosted product (accounts, billing, UI)  
D2. GUI management  
D3. Opaque-key mode (hiding object keys from provider)  
D4. Compaction / segment GC / tombstone GC (document future plan only)  
D5. Push notifications (S3 events), real-time sync daemon

---

## 3) Design decisions (locked)

### 3.1 Replication format is unchanged
Segments/blobs/tombstones remain exactly as in Phase 2A and Sync Storage Contract v0.

### 3.2 Atomic publish on S3
S3 PUT/multipart completion is atomic for readers. We **do not rely on rename** semantics.

### 3.3 Minimal metadata leakage accepted (Phase 2B)
We accept that object keys reveal:
- object type
- vault_id
- node_id for segments/manifests
- blob hash prefix structure (aa/bb)

All object payloads remain encrypted by default.

---

## 4) User experience

### Store URI forms
- `hx sync init --store s3://bucket/prefix --region us-east-1`
- `hx sync init --store s3+endpoint://bucket/prefix --endpoint http://localhost:9000 --path-style`

### Commands (existing)
- `hx sync init/status/push/pull`

### Expected behavior
- Push publishes new segments/blobs/tombstones + updates the local node manifest.
- Pull fetches manifests, computes missing objects, downloads/imports them.
- Blobs are fetched on demand (only when referenced by imported metadata).

---

## 5) Implementation requirements

### 5.1 S3Store must support
- `list(prefix)` with pagination/continuation tokens
- `get(key)` streaming download with size bounds + retries
- `put_atomic(key, bytes/stream)`:
  - for small objects: single PUT
  - for large objects: multipart upload (complete is atomic)

### 5.2 Manifest v0
- One encrypted manifest per node: `manifests/<node_id>.hxman`
- Must include:
  - vault_id, node_id, manifest_seq (monotonic)
  - published segment IDs (or a compact range/cursor)
  - published tombstone IDs (or cursor)
- Must be tamper-evident via AEAD and bound to vault_id/node_id.

### 5.3 Pull algorithm
1) List manifests (one per node) OR list manifest keys by prefix  
2) Download/decrypt manifests  
3) Compute missing segments/tombstones relative to local imported set  
4) Download missing objects and import (idempotent)  
5) Fetch referenced blobs if missing (on demand or in a second pass)

### 5.4 Error handling and observability
- Categorize errors:
  - auth/permission
  - not found
  - throttling
  - timeout
  - corrupt/tampered object
  - hash mismatch
- `hx sync status` should show:
  - last push/pull time
  - objects imported/skipped by category
  - last error category + message

---

## 6) Acceptance criteria (Phase 2B)

AB1. Two devices sync through MinIO S3Store and converge (union/no-dup/tombstones).  
AB2. Pull uses manifests and does not require listing all segments/blobs every time (measurable: fewer list calls).  
AB3. Corrupt objects do not import and do not block valid imports.  
AB4. Wrong-vault objects are rejected.  
AB5. Retries/backoff handle injected transient failures (simulated 5xx/timeouts).  
AB6. Large blob upload via multipart succeeds and remains atomic to readers.  
AB7. All tests pass under race detection where applicable.

---

## 7) Milestones

B0. Finalize Manifest v0 fields + publish rules  
B1. Implement S3Store list/get/put_atomic + config parsing  
B2. Implement manifest publish on push  
B3. Implement manifest-driven pull (fallback to listing allowed for debugging only)  
B4. Add MinIO integration tests + CI harness  
B5. Harden retries/backoff + status reporting

---

## 8) Open choices (bounded)
- Manifest “segments published” encoding:
  - list of segment IDs vs (start,end) cursor per node (recommend list of IDs first, then optimize)
- Manifest publish frequency:
  - on every push (recommend) + optional periodic later
- Blob prefetch:
  - on-demand only (recommend) vs optional eager prefetch for small blobs
