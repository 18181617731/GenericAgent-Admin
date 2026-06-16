# Changelog

This file records manually curated release changes for GenericAgent Admin Go.

## v0.1.0-alpha2 - 2026-06-14

### Scope
- Follow-up alpha for `v0.1.0-alpha` focused on the Goal/Hive boundary and Windows background-process UX.
- Target branch: `main`; target tag: `v0.1.0-alpha2`.

### User-facing changes
- Removed the GA Admin built-in BBS collaboration page/API surface; Hive collaboration now follows the official GenericAgent external BBS/worker flow.
- Added Hive mode to Goal start so GA Admin can manage Goal/Hive lifecycle while delegating collaboration protocol details to GA official scripts.
- Goal state now reports Hive metadata such as readme URL, worker PID, BBS PID, and Hive working directory for operator visibility.
- Windows Goal/Hive background launches prefer `pythonw.exe` and keep no-window process flags to avoid popping terminal windows for users.

### Safety and validation
- Stop Goal now also cleans recorded Hive worker/BBS PIDs without broad process-tree termination.
- Release workflow explicitly accepts `v0.1.0-alpha2` in addition to prior approved release tags.
- Validation gates before publication: `go test ./...`, `go build ./...`, `npm.cmd test -- --run`, and `npm.cmd run build`.

## v0.1.0-alpha - 2026-06-12

### Scope
- Baseline: `v0.0.30-fix1`.
- Candidate commit anchor: `2202eb6 feat(admin): harden files and model configuration UX`.
- Current state: v0.1.0-alpha is approved for commit, push, tag creation, and GitHub release asset publication; live-service restart and existing-asset overwrite remain separate approvals.

### User-facing changes
- Files UI now protects configured roots, blocks traversal, supports download/open/delete guards, and keeps destructive operations behind explicit confirmation.
- Model configuration UX supports safe preview/write-back for local `mykey.py` generation without shipping private keys in source or release assets.
- Release/update documentation now separates three states: local RC evidence complete, user approval required, and post-publication verification.

### 中文用户摘要
- v0.1.0-alpha 聚焦 Admin 管理端的安全边界、配置体验和发布可审计性，属于正式 v0.1.0 前的 alpha 交付。
- 文件管理接口和界面交互收敛了危险路径与误操作风险；模型配置支持在管理端查看、调整和保存本地 GA 配置。
- 用户应按平台资产与 `.sha256` 校验文件完成升级验证；alpha 验证通过后再推进正式 v0.1.0。

### Approval boundary
- This alpha publication targets branch `main` and tag `v0.1.0-alpha`; formal `v0.1.0`, GitCode release, live-service restart, and existing-asset overwrite require separate approval.
- Packaging or workflow output must be checked for local/private files including `config.local.json`, `model_profiles.json`, `mykey.py`, `.env`, and `*.key`.
- Local review notes, temporary backups, and troubleshooting artifacts are excluded from the release commit by default unless the release owner explicitly chooses otherwise.

### Validation
- Release gates are rerun before tag publication: web tests, web build, Go tests/build, and diff hygiene.
- Alpha release gates are rerun before tag publication.
- Formal v0.1.0 readiness still depends on post-alpha verification and separate release-owner approval.
