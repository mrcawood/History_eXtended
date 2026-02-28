# HX Phase 2B — Implementation Agent Context (S3Store + Manifest v0)
**Date:** 2026-02-28

## Objective
Implement Phase 2B: S3-compatible sync backend + manifest-based incremental pull, without changing replication format.

## Constraints
- No SQLite replication.
- Keep E2EE default.
- Preserve importer validation and convergence invariants from Phase 2A.
- Do not introduce “HX Cloud” accounts/billing.

## Deliverables
1) `S3Store` implementing SyncStore
2) Manifest v0 object codec + publish/pull logic
3) Integration tests using MinIO:
   - two-node converge over S3Store
   - tombstone propagation over S3Store
   - corrupt object does not block valid
   - pagination and transient retries
4) `hx sync status` improvements for remote errors

## Recommended build order
1) Implement S3Store list/get/put_atomic with config parsing
2) Implement manifest v0 codec (encrypt/decrypt using existing object envelope)
3) Implement manifest publish on push
4) Implement manifest-driven pull
5) Add MinIO integration tests and CI harness
6) Harden retries/backoff/status reporting

## Definition of done
- Two nodes converge over MinIO-backed S3Store with manifests enabled.
- Pull does not require listing all segments each time (manifest is used).
- All tests pass reliably; corruption handling is enforced in importer.

