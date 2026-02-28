# HX Manifest v0 Spec (Encrypted)
**Date:** 2026-02-28  
**Status:** Draft (implementation-ready)  
**Purpose:** Efficient incremental sync on remote stores by reducing full-prefix listings.

---

## 1) Goals
- Enable pull to discover new segments/tombstones efficiently on S3-like stores.
- Preserve E2EE (manifest payload encrypted).
- Support monotonic progression and safe rollback handling.

---

## 2) Location and naming
Canonical key:
- `vaults/<vault_id>/objects/manifests/<node_id>.hxman`

Notes:
- One manifest per node.
- Payload is encrypted AEAD with vault key.

---

## 3) Data model (payload)
All fields below are inside the encrypted payload unless explicitly stated.

Required:
- `vault_id` (uuid)
- `node_id` (uuid)
- `manifest_seq` (u64) monotonic, starts at 1
- `created_at` (timestamptz)
- `segments`: list of `{segment_id, segment_hash, created_at}` (or omit hash if redundant)
- `tombstones`: list of `{tombstone_id, created_at}`
- `capabilities`: `{format_version: 0, supports: ["segments","tombstones"]}`

Optional (defer in v0):
- cursors/ranges instead of explicit lists
- bloom filters / hash summaries

---

## 4) Publish rules
- Manifest is published on `hx sync push` after any new objects are written.
- `manifest_seq` increments each publish.
- The latest manifest overwrites the previous manifest key (single object).

Rollback protection (v0 minimal):
- On pull, accept the manifest with highest observed `manifest_seq` for that node.
- If a lower seq is observed later, treat as suspicious and ignore (report in status).

---

## 5) Pull usage
- Pull lists `manifests/` keys and downloads each manifest.
- For each node, compute missing segments/tombstones by comparing:
  - manifest declared items
  - local `imported_segments` and `applied_tombstones`
- Download/import missing objects.

---

## 6) Security
- Manifest payload is encrypted with vault key; any tamper fails AEAD.
- Must validate:
  - decrypted `vault_id` == local vault
  - `node_id` matches the manifest key
- Do not include sensitive plaintext in headers beyond minimal routing.

---

## 7) Failure behavior
- Missing manifests: fall back to listing segments/tombstones for that node (debug mode) OR treat as “no updates.”
- Corrupt manifest: skip and report.

