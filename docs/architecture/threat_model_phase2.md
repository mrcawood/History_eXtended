# HX Phase 2 — Threat Model (Sync)
**Date:** 2026-02-26  
**Scope:** Sync Storage Contract v0, E2EE, folder store, multi-device convergence

---

## Design Lint Checklist (30–60 min)

Explicit confirmations before implementation:

| # | Item | Status |
|---|------|--------|
| 1 | **Atomic publish across NAS** | Write to tmp/, fsync, atomic rename. Readers ignore tmp/. NAS: rename must be atomic (POSIX; NFS/SMB may vary — document and test in integration suite). |
| 2 | **Key storage / passphrase fallback** | K_master in keychain (OS) preferred; passphrase-derived fallback. Key never written by HX at runtime. |
| 3 | **Tombstone non-resurrection** | Importer records applied tombstones; checks before inserts. Tombstones always win. Time-window primary (matches hx forget). |
| 4 | **Plaintext in headers** | Only: magic, version, object_type, vault_id, node_id, segment_id/blob_hash/tombstone_id, created_at, byte_len_plain, compression, crypto envelope metadata. No event content, no command text. |
| 5 | **Segment uniqueness key** | `(node_id, session_id, seq)` — stable across imports. INSERT OR IGNORE; idempotent. |

---

## 1) Data classification and boundaries

| Data | Classification | Location | Trust boundary |
|------|----------------|----------|----------------|
| Vault master key (K_master) | **Critical** | Local only (keychain/passphrase) | Never leaves device |
| Per-object keys (K_obj) | **Critical** | Wrapped in object envelope | Sync store (encrypted) |
| Segment payloads (events, sessions, pins) | **Sensitive** | Sync store (encrypted) | Untrusted when in folder/NAS/S3 |
| Blob payloads | **Sensitive** | Sync store (encrypted) | Same |
| Tombstone payload | **Sensitive** | Sync store (encrypted) | Same |
| Object headers (unencrypted) | **Low** | Sync store | Vault_id, node_id, timestamps, object type — no plaintext content |
| Local SQLite DB | **Sensitive** | Device only | Never synced |
| Blob cache | **Sensitive** | Device only | Local copy of synced blobs |

**Boundaries:**
- Device boundary: K_master, SQLite, blob cache stay local.
- Sync store: folder/NAS/S3 is assumed **untrusted** by default. Encrypted payloads only.
- Trusted local store: `--no-encrypt` allowed only when user explicitly trusts the path (e.g., local-only NAS).

---

## 2) STRIDE threat list and mitigations

### Spoofing
| Threat | Mitigation |
|--------|------------|
| Attacker publishes forged segment/blob into vault | Objects encrypted with vault K_master; attacker cannot forge without key. Import fails on AEAD verification. |
| Attacker reuses old segment with different content | Per-object K_obj; header is AEAD AAD — tampering fails verification. |
| Malicious node_id in header | Header integrity via AAD; cannot decrypt to verify node. Idempotency keys `(node_id, session_id, seq)` — spoofed node_id creates separate event space; no cross-node impersonation of events. |

### Tampering
| Threat | Mitigation |
|--------|------------|
| Modify object in transit/store | AEAD auth tag; any byte change causes decrypt failure. Import rejects (T5 test). |
| Modify header only | Header is AEAD associated data; integrity protected. |
| Reorder/delete objects | Idempotent import; order-independent. Deletion of segment = older state; tombstone wins for deletes. |
| Partial/corrupt object | Atomic publish; readers ignore tmp/. Integrity checks (size, hash) before import (T3, T4). |

### Repudiation
| Threat | Mitigation |
|--------|------------|
| "I never published that segment" | node_id, created_at in header; immutable once published. No audit log in v0. |
| Tombstone abuse (malicious forget) | Tombstones irreversible in v0. Single-user/multi-device: same user owns all nodes. Deferred: audit trail for tombstones. |

### Information disclosure
| Threat | Mitigation |
|--------|------------|
| Sync store exfiltrates plaintext | E2EE default; payloads encrypted. Header reveals only routing metadata. |
| Key extraction from device | K_master in keychain (OS-protected) or passphrase-derived. Out of scope: device compromise. |
| Blob hash in header | Content-addressable; hash does not reveal content. |
| Timing/metadata analysis | Not addressed in v0 (e.g., object count, size patterns). Low priority for folder store. |

### Denial of service
| Threat | Mitigation |
|--------|------------|
| Fill sync store with junk | No quota in v0. User controls folder; can prune. S3 Phase 2B: need store-side limits. |
| Maliciously large segment | Size check before decrypt; reject objects above configurable max. Contract should specify max segment size. |
| Tombstone flood | Each tombstone is small; applied once. Flood adds metadata only. |
| Corrupt object causes import crash | Verify integrity before decrypt; fail safe, skip corrupt. |

### Elevation of privilege
| Threat | Mitigation |
|--------|------------|
| Import runs arbitrary code | No code in objects; structured data only. Parsing must be strict; reject malformed. |
| SQL injection via event content | Parameterized queries; event content in bound params. |
| Path traversal in store keys | Store interface uses controlled key space; no user-supplied paths in final key. |

---

## 3) Logging policy

| Event | Log level | Contents |
|-------|-----------|----------|
| Sync init | Info | Vault id, store type, path (redact full path if sensitive) |
| Push success | Info | Segment count, blob count |
| Pull success | Info | Imported count, skipped count |
| Import skip (already imported) | Debug | Segment id, node id |
| Import skip (integrity failure) | Warn | Object key, reason (checksum/decrypt failed) |
| Tombstone applied | Info | Tombstone id, scope summary |
| Decrypt failure (tamper) | Warn | Object key; do not log payload/key material |
| Key access failure | Error | Keychain/passphrase error (no key material) |

**Do not log:** K_master, K_obj, passphrase, decrypted payloads.

---

## 4) Least-privilege tool plan

| Component | Access | Notes |
|-----------|--------|-------|
| hx sync push | Read: config, spool, SQLite (events), blob cache. Write: sync store (put_atomic). | Needs vault key for encrypt. |
| hx sync pull | Read: sync store (list, get). Write: SQLite, blob cache, sync metadata. | Needs vault key for decrypt. |
| FolderStore | Read/write: user-specified directory only. No network. | Least privilege: only the sync path. |
| Daemon (future auto-sync) | Same as push/pull. | Deferred. |
| Key storage | Read: keychain or user passphrase. Write: none at runtime. | Key never written by HX at runtime. |

**Recommendations:**
- Enforce max object size (e.g., 64MB) before decrypt to bound DoS.
- Document `--no-encrypt` as explicitly lowering security; require flag.
- Add contract clarification: blob_hash in header is over plaintext pre-compression.

---

## 5) Summary

- E2EE design is sound: envelope encryption, AEAD, header-as-AAD.
- Main residual risks: key extraction (device compromise), tombstone irreversibility, no store quota.
- Mitigations documented; T3–T5 cover integrity and tamper detection.
- No blocking issues for Phase 2A implementation.
