# HX Phase 2B Test Plan (MinIO-backed)
**Date:** 2026-02-28

## Goals
Validate S3Store correctness, manifest-driven efficiency, and resilience to real network/storage behaviors.

## Setup
- Run MinIO in CI (docker) with known creds.
- Create bucket + prefix per test run.

## Test cases
T2B-1 Basic converge
- Node A publish, Node B pull → B sees A history; then reverse; convergence holds.

T2B-2 Tombstone propagation
- A forget → publish tombstone → B pull → B removes events; no resurrection after reimport.

T2B-3 Corrupt does not block valid
- Upload a corrupted segment object; upload a valid one; ensure valid imports proceed.

T2B-4 Pagination
- Generate > 1000 small objects under a prefix; ensure list pagination works.

T2B-5 Retry/backoff
- Inject transient errors (proxy or wrapper returning 503/timeouts); ensure retries and status categories.

T2B-6 Multipart upload
- Upload a blob above multipart threshold; verify import works and object is readable only after completion.

T2B-7 Manifest efficacy
- Measure list calls per pull:
  - first pull: list manifests + fetch new objects
  - subsequent pull no changes: minimal calls (manifest only), not full segments listing

## Pass criteria
- Convergence invariants hold (union/no dup/tombstones).
- Importer rejects tampered/corrupt content and continues scanning.
- Manifest-driven pull avoids full listing in steady state.

