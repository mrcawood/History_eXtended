# HX Phase 2 — Implementation Agent Context
**Date:** 2026-02-26

## Objective
Implement Phase 2A: multi-device sync using Sync Storage Contract v0, starting with FolderStore.

## Constraints (do not violate)
- Do NOT sync SQLite DB files.
- Default to E2EE (no plaintext payloads).
- Import is idempotent and order-independent.
- Tombstones must win and prevent resurrection.
- Keep scope to multi-device sync only (no team tenancy, no GUI, no agent mode).

## Deliverables (Phase 2A)
1) `SyncStore` interface + `FolderStore`
2) Object codec + crypto envelope + atomic publish
3) Segment writer (daemon flush policy)
4) Importer (segments/blobs/tombstones)
5) CLI: `hx sync init/status/push/pull`
6) Integration tests (2-node converge + tombstone)

## Recommended build order
1) Object codec + crypto + put_atomic
2) FolderStore list/get/put_atomic + directory layout
3) Importer + sync metadata tables
4) Segment writer + flush triggers
5) CLI wiring + status counters
6) Integration tests + hardening

## Definition of done
- Two devices converge with union of events and blobs after push/pull.
- Forget propagates and blocks resurrection.
- Store contains only encrypted objects by default.
