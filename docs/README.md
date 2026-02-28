# Documentation

This directory contains the canonical documentation for the History eXtended project.

---

## 📁 Directory Structure

### [`prd/`](prd/)
Product Requirements Documents by phase
- [`phase1.md`](prd/phase1.md) - Phase 1 requirements (local history capture)
- [`phase2a.md`](prd/phase2a.md) - Phase 2A requirements (multi-device sync)
- [`phase2b.md`](prd/phase2b.md) - Phase 2B requirements (cloud store, performance)

### [`architecture/`](architecture/)
Technical specifications and architecture contracts
- [`sync_storage_contract_v0.md`](architecture/sync_storage_contract_v0.md) - Sync storage interface contract
- [`threat_model_phase2.md`](architecture/threat_model_phase2.md) - Phase 2 security analysis
- [`phase2a_agent_context.md`](architecture/phase2a_agent_context.md) - Phase 2A implementation guidance

### [`validation/`](validation/)
Test results, validation evidence, and test gates
- [`status_report.md`](validation/status_report.md) - Overall project status and validation
- [`validation_appendix.md`](validation/validation_appendix.md) - Detailed validation results
- [`test_gate_phase2a.md`](validation/test_gate_phase2a.md) - Phase 2A test gate status
- [`test_and_doc_review.md`](validation/test_and_doc_review.md) - Documentation and testing review

### [`runbooks/`](runbooks/)
Operational guides and troubleshooting
- *Coming soon for Phase 2B*

### [`archive/`](archive/)
Historical documentation and reports
- [`2026-02_phase2a_reports/`](archive/2026-02_phase2a_reports/) - Phase 2A development reports

---

## 🚀 Quick Navigation

### For New Contributors
1. Start with [`../README.md`](../README.md) - Project overview and quick start
2. Review [`../INSTALL.md`](../INSTALL.md) - Installation instructions
3. Check [`../PROGRESS.md`](../PROGRESS.md) - Current development status

### For Phase 2 Development
1. **Phase 2A**: See [`validation/test_gate_phase2a.md`](validation/test_gate_phase2a.md) for completion status
2. **Phase 2B**: See [`prd/phase2b.md`](prd/phase2b.md) for current requirements
3. **Architecture**: Review [`architecture/sync_storage_contract_v0.md`](architecture/sync_storage_contract_v0.md) for storage contracts

### For Operations
1. **Status**: [`validation/status_report.md`](validation/status_report.md)
2. **Validation**: [`validation/validation_appendix.md`](validation/validation_appendix.md)
3. **Troubleshooting**: See [`runbooks/`](runbooks/) directory

---

## 📋 Document Standards

- **Naming**: `snake_case` for all filenames
- **Location**: All project docs live under `docs/`
- **Canonical**: One source of truth per topic
- **Archive**: Historical reports moved to `archive/` by date

---

## 🔗 Related Files

Root-level files (project-level documentation):
- [`../README.md`](../README.md) - Project overview and features
- [`../INSTALL.md`](../INSTALL.md) - Installation and setup
- [`../PROGRESS.md`](../PROGRESS.md) - Development progress and status
- [`../prd.md`](../prd.md) - Legacy PRD (may be deprecated)

---

## 📝 Contributing

When adding new documentation:

1. Choose the appropriate directory based on content type
2. Use `snake_case` naming convention
3. Update this README for new canonical documents
4. Archive outdated documents rather than deleting
5. Keep links and references up to date

---

*Last updated: 2026-02-28*
