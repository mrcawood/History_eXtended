# Golden Dataset (PRD §14)

25 sample artifacts for Phase 1 acceptance validation.

| Dir        | Count | Purpose                        |
|------------|-------|--------------------------------|
| build/     | 5     | make, ninja, cmake, link, go   |
| ci/        | 5     | GitHub, GitLab, go test, cargo, maven |
| slurm/     | 5     | OOM, timeout, node fail, sbatch, salloc |
| traceback/ | 5     | KeyError, ModuleNotFound, Attribute, pytest, TypeError |
| compiler/  | 5     | gcc, clang, rust, java, syntax |

**Variants** (A4 skeleton_hash stability):
- `traceback/04_pytest_fail_variant.txt` — same content, different hex address
- `ci/01_github_actions_variant.log` — same content, different timestamps

Run: `./scripts/validate.sh` for A1–A7 validation.
