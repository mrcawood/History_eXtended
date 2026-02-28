# HX S3Store Spec (S3-compatible SyncStore Backend)
**Date:** 2026-02-28  
**Status:** Draft (implementation-ready)

---

## 1) Purpose
Implement `SyncStore` against S3-compatible object storage. Must preserve Phase 2A invariants and importer validation.

---

## 2) Configuration
Support:
- `bucket`
- `prefix` (root path under bucket, e.g., `hx/HXSync`)
- `region` (AWS) (optional for compatible endpoints)
- `endpoint` (optional for MinIO/B2/Wasabi/etc.)
- `path_style` (bool, needed for some endpoints)
- credentials:
  - AWS default credential chain
  - explicit env/config
- timeouts:
  - connect/read
- retry policy:
  - max attempts, backoff

---

## 3) Key operations

### 3.1 list(prefix)
- Must page with continuation tokens.
- Must return keys only (and optionally size/etag for diagnostics).

### 3.2 get(key)
- Stream download to memory or temp file.
- Enforce max size (guard against hostile objects).
- Retry on transient errors (5xx, throttling, timeouts).

### 3.3 put_atomic(key, payload)
- Small payload: single PUT.
- Large payload: multipart upload; only finalize on CompleteMultipartUpload.
- No rename/copy required.

---

## 4) Semantics and edge cases
- S3 is atomic per object PUT; readers never see partial bytes.
- Listing is eventually consistent in rare cases; manifests reduce reliance on listing.
- Handle duplicate uploads of same key (idempotent overwrites).
- Do not assume key ordering.

---

## 5) Testing requirements
- Use MinIO in integration tests.
- Tests must cover:
  - pagination listing
  - transient failure retry
  - wrong credentials / denied access
  - large object multipart upload
  - manifest-driven pull correctness

