# HX Sync Storage Contract v0
**Date:** 2026-02-26  
**Status:** Draft (for implementation)  
**Purpose:** Define the replicated object model, cryptographic envelope, on-disk layout, store interface, and deterministic import/merge rules for HX multi-device sync.

---

## 1) Contract principles (non-negotiable)

1. **SQLite is not replicated.** SQLite is a derived local index built from replicated objects.  
2. Objects are **immutable once published** (except via tombstones).  
3. Import is **idempotent** and **order-independent**.  
4. Merge semantics are deterministic: **union + tombstones**.  
5. Store backends must support **atomic publish** for object finalization.  
6. Default is **E2EE** for all object payloads.

---

## 2) Terminology

- **Vault:** the logical sync domain; devices enrolled into the same vault share keys.  
- **Node:** a device instance (laptop/desktop/host) with a stable `node_id`.  
- **Store:** the transport substrate (folder, S3, etc.) that holds objects.  
- **Object:** an immutable unit stored in the store (segment/blob/tombstone/manifest).  
- **Import:** reading objects from store and applying them into local SQLite and blob cache.

---

## 3) Object types

### 3.1 Segment object (`.hxseg`)
Purpose: publish a batch of events (command events, sessions, artifact metadata, pins) as an immutable unit.

**Required header fields (unencrypted):**
- `magic`: "HXOBJ"
- `version`: integer (v0 = 0)
- `object_type`: "segment"
- `vault_id`: uuid
- `node_id`: uuid
- `segment_id`: uuid
- `created_at`: timestamptz (publisher clock)
- `crypto`: envelope metadata (see §5)

**Encrypted payload (AEAD):**
- `events[]`: list of event records
- `sessions[]`: optional session records
- `artifacts[]`: optional artifact metadata records
- `pins[]`: optional pin/unpin records

**Event key requirements (idempotency):**
Each event must have a stable unique key:
- v0 uniqueness key: `(node_id, session_id, seq)`

Segments must be importable multiple times without duplicating events.

---

### 3.2 Blob object (`.hxblob`)
Purpose: content-addressed payload for artifacts and transcript chunks.

**Identity:**
- `blob_hash = sha256(plaintext_blob_bytes)` (v0)

**Header (unencrypted):**
- `magic`, `version`, `object_type`="blob"
- `vault_id`
- `blob_hash`
- `byte_len_plain`
- `compression` (e.g., "zstd")
- `crypto` envelope metadata

**Encrypted payload:**
- compressed blob bytes

---

### 3.3 Tombstone object (`.hxtomb`)
Purpose: replicate deletion intent; must win and prevent resurrection.

**Header (unencrypted):**
- `magic`, `version`, `object_type`="tombstone"
- `vault_id`
- `tombstone_id` (uuid)
- `created_at`
- `node_id` (issuer)

**Encrypted payload scope (v0):**
- **Time-window** (primary): `{node_id?, start_ts, end_ts}` — matches `hx forget --since`.
- Event-key and blob tombstone formats optional later.

**Semantics:**
- Tombstones always win.
- Importer must record applied tombstones and check them before inserting.

---

### 3.4 Manifest object (`.hxman`) (optional in v0)
Purpose: efficient incremental sync; required for S3 scale later.

---

## 4) Canonical store keyspace (layout)

```
HXSync/
  vaults/<vault_id>/
    objects/
      segments/<node_id>/<segment_id>.hxseg
      blobs/<aa>/<bb>/<blob_hash>.hxblob
      tombstones/<tombstone_id>.hxtomb
      manifests/<node_id>.hxman
    tmp/
      <key>.partial
```

Rules:
- Writers publish to `tmp/` then atomically rename into `objects/...`.
- Readers ignore `tmp/` and only import under `objects/`.

---

## 5) Crypto envelope (E2EE)

### Vault key
- `K_master` (256-bit) stored locally (keychain preferred; passphrase fallback).

### Envelope encryption
- Per object: generate `K_obj`, AEAD-encrypt payload with `K_obj`.
- Wrap `K_obj` with `K_master` (AEAD wrap).
- Header is AEAD associated data (integrity protected).

### AEAD
- XChaCha20-Poly1305 preferred; AES-256-GCM acceptable with strict nonce discipline.

---

## 6) Store interface (backend contract)

Backends implement:
- `list(prefix) -> [key]`
- `get(key) -> bytes`
- `put_atomic(key, bytes)`

`put_atomic` guarantees readers never observe partial content at final key.

---

## 7) Import contract (deterministic merge)

- Verify header + AEAD integrity.
- Segment import:
  - skip if already imported (segment hash recorded)
  - insert events with uniqueness key `(node_id, session_id, seq)` using INSERT OR IGNORE
- Blob import:
  - verify hash; store in local blob cache
- Tombstone import:
  - record applied tombstone
  - apply deletes locally
  - prevent resurrection by checking tombstones before inserts

Import order must not affect final state.

---

## 8) Conflict policies (v0 defaults)

- Pins: pinned if any device pins (monotonic).
- Forget: tombstones always win; irreversible in v0.

---

## 9) Required local DB tables (sync metadata)

Local SQLite tracks:
- vault config
- node config
- imported segments
- applied tombstones
- counters/metrics for `hx sync status`

---

## 10) Required automated tests

T1 Two-node convergence  
T2 Tombstone propagation  
T3 Partial/corrupt object rejection  
T4 Idempotent re-import  
T5 Tamper detection (crypto integrity)
