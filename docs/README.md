# Documentation

**Start at [README.md](../README.md) and [INSTALL.md](../INSTALL.md) for user setup.** This folder is engineering and design documentation, not the user entrypoint.

---

## Directory Structure

### [`prd/`](prd/)
Product Requirements Documents by phase
- [`phase2a.md`](prd/phase2a.md) - Phase 2A requirements (multi-device sync)
- [`phase2b.md`](prd/phase2b.md) - Phase 2B requirements (cloud store, manifest-driven sync)

### [`architecture/`](architecture/)
Technical specifications and architecture contracts
- [`sync_storage_contract_v0.md`](architecture/sync_storage_contract_v0.md) - Sync storage interface contract
- [`manifest_v0.md`](architecture/manifest_v0.md) - Manifest v0 spec
- [`s3store.md`](architecture/s3store.md) - S3 store spec (implementation in `internal/sync`; not wired in CLI)
- [`threat_model_phase2.md`](architecture/threat_model_phase2.md) - Phase 2 security analysis
- [`phase2a_agent_context.md`](architecture/phase2a_agent_context.md) - Phase 2A implementation guidance
- [`phase2b_agent_context.md`](architecture/phase2b_agent_context.md) - Phase 2B implementation guidance
- [`history_import.md`](architecture/history_import.md) - History import design
- [`hx_bash5_support_spec.md`](architecture/hx_bash5_support_spec.md) - Bash 5+ support spec

### [`roadmap/`](roadmap/)
Design and planned features (not yet in CLI)
- [`s3_sync.md`](roadmap/s3_sync.md) - S3 sync user guide (Design/Roadmap — S3Store exists but CLI supports `folder:` only)

### [`validation/`](validation/)
Test results, validation evidence, and test gates
- [`status_report.md`](validation/status_report.md) - Overall project status
- [`validation_appendix.md`](validation/validation_appendix.md) - Detailed validation results
- [`test_gate_phase2a.md`](validation/test_gate_phase2a.md) - Phase 2A test gate status
- [`phase2b_acceptance_checklist.md`](validation/phase2b_acceptance_checklist.md) - Phase 2B acceptance criteria
- [`phase2b_security_verification.md`](validation/phase2b_security_verification.md) - Phase 2B security verification

### [`configuration/`](configuration/)
- [`reference.md`](configuration/reference.md) - Configuration options reference

### [`runbooks/`](runbooks/)
- [`s3_troubleshooting.md`](runbooks/s3_troubleshooting.md) - S3 sync troubleshooting

### [`developer/`](developer/)
- [`syncstore_api.md`](developer/syncstore_api.md) - SyncStore interface documentation

### [`release/`](release/)
- [`bash5_release_checklist.md`](release/bash5_release_checklist.md) - Bash 5+ release checklist

### [`archive/`](archive/)
- [`2026-02_phase2a_reports/`](archive/2026-02_phase2a_reports/) - Phase 2A development reports

---

## Quick Navigation

### Start here (user docs)
1. [README.md](../README.md) - Project overview and quick start
2. [INSTALL.md](../INSTALL.md) - Installation and setup

### Development
1. [PROGRESS.md](../PROGRESS.md) - Current status and phase
2. [prd.md](../prd.md) - Product requirements (root)
3. [configuration/reference.md](configuration/reference.md) - Config options

### Phase 2
- Phase 2A: [test_gate_phase2a.md](validation/test_gate_phase2a.md)
- Phase 2B: [prd/phase2b.md](prd/phase2b.md), [architecture/s3store.md](architecture/s3store.md)
- Architecture: [sync_storage_contract_v0.md](architecture/sync_storage_contract_v0.md)

---

## Related Files

- [../README.md](../README.md) - Project overview
- [../INSTALL.md](../INSTALL.md) - Installation
- [../PROGRESS.md](../PROGRESS.md) - Development status

---

*Last updated: 2026-03-04*
