# GenericAgent Admin Go v0.1.0-alpha Release Notes

Status: v0.1.0-alpha publication approved by release owner for GitHub tag/release. These notes describe the alpha release scope; service restarts and existing-asset overwrites remain separately controlled.

## 中文用户摘要

v0.1.0-alpha 面向 GenericAgent Admin Go 的正式发布前审核与用户交付准备，聚焦 Admin 管理端的安全边界、配置体验、非聊天管理能力和发布可审计性：

- 文件管理相关接口与界面交互经过安全边界收敛，降低误操作和危险路径暴露风险。
- 模型配置写回体验完成整理，便于在管理端查看、调整与保存本地 GA 配置；私有密钥和本地配置不进入源码或发布包。
- 服务、进程、计划任务、BBS 协作、健康/风险/库存和版本更新说明已补齐，用户可在执行危险动作前获得更明确的操作提示。
- 版本信息、发布包命名和更新校验说明已整理，便于按平台资产与 `.sha256` 校验文件完成升级验证。

当前发布目标为 `v0.1.0-alpha`；正式 `v0.1.0` 仍需后续 release owner 单独批准。

## Version target

- Target tag: `v0.1.0-alpha`.
- Release baseline: `v0.0.30-fix1`.
- Candidate range: `v0.0.30-fix1..HEAD`.
- Local status: alpha release approved for commit, tag, push, and GitHub Release asset build.

## Highlights

- Hardened local file-management flows: root confinement, traversal protection, guarded delete/open/download behavior, clearer empty-state guidance, save-target review, and safer error handling.
- Improved model-configuration delivery: users can preview and write local GA `mykey.py` configuration while source and packages continue to exclude private keys; validation blocks malformed profile names, missing model data, and invalid API Base values before writeback.
- Added user-facing non-chat operations guidance for services/processes, scheduled tasks, BBS worker protocol status, inventory/risk/health observability, and version/update safety.
- Clarified release/update documentation: alpha publication, formal-release approval, asset verification, and post-release checks are kept separate.

## User-facing quickstart

A dedicated quickstart is provided at `docs/USER_QUICKSTART.md`. It covers:

1. First-run root selection and local-only private file boundaries.
2. Model profile validation, preview, and safe `mykey.py` writeback.
3. File browse/search/tail/save/delete behavior and delete warnings.
4. Service/process controls, dangerous confirmation, and read-before-stop guidance.
5. Schedule task review, create/update/delete boundaries, and artifact review.
6. BBS connection status, readme/protocol validation, worker service controls, and board-key caution.
7. Inventory/risk/health interpretation before write operations.
8. Version/update expectations, `.sha256` verification, and publication approval boundaries.

## User delivery checklist

Before telling users that v0.1.0 is published, complete all of the following with explicit approval:

1. Re-run or accept the final gate evidence for `git diff --check`, Go tests/build, frontend tests/build, and packaging checks.
2. Confirm release artifacts do not contain `config.local.json`, `model_profiles.json`, `mykey.py`, `.env`, `*.key`, tokens, cookies, or other local/private material.
3. Exclude local review notes, temporary backups, and troubleshooting artifacts from the release commit unless explicitly approved otherwise.
4. Approve the exact commit, tag, push, and Release commands for branch `main` and tag `v0.1.0`.
5. After publication, verify from the Admin UI that the update source can discover the `ga-admin-<tag>-<goos>-<goarch>.zip` asset and matching `.sha256` sidecar, download it, and pass checksum validation.
6. Confirm user documentation links resolve locally, referenced repo paths exist, and release-facing markdown contains only final user-facing copy.

## Safe-operation warnings

- File save/delete, model writeback, service start/stop, worker autostart, BBS write operations, schedule edits, and update application are privileged operations; they should remain guarded by explicit UI confirmation and backend dangerous-operation checks.
- Users should inspect path, PID, service name, task command, and risk summary before executing destructive or process-affecting actions.
- Board keys, API keys, tokens, cookies, and local config files are not release assets.
- If a live service or worker is already handling user traffic, do not restart or stop it as part of documentation or release-note validation.

## Release manifest

The release include/exclude manifest for v0.1.0-alpha is maintained in `docs/RELEASE_MANIFEST_v0.1.0.md`. It mirrors the current `.github/workflows/release-assets.yml` asset naming and packaging flow, and lists the runtime files expected in each zip, local/private files that must stay out of archives, and product-specific offline/error limitations for non-chat areas.

## Known limitations

- The quickstart is product guidance, not a guarantee that a specific external model provider, BBS server, or update host is reachable.
- The Admin UI helps manage local GA state; it does not replace code review, release-owner approval, or environment-specific backup policies.
- Files, Models, Schedule, BBS, Process/Services, Observability, Version/Update, and TMWebDriver can show partial, stale, disabled, unauthorized, or network-failed states depending on local OS permissions, configured roots, credentials, running services, and approved remote assets.
- Update verification depends on published alpha assets and matching checksum sidecars; formal `v0.1.0` promotion requires a separate verification pass.
- Existing GitHub/GitCode assets must not be overwritten without separate explicit approval.

## Non-goals / explicit boundaries

- No private key, token, cookie, or local-only config file is part of the official source or release package.
- Remote alpha publication is explicitly requested for tag `v0.1.0-alpha`; formal `v0.1.0` remains out of scope.
- Restarting or stopping live services/chat workers remains out of scope unless separately approved.
- If GitHub/GitCode assets already exist, replacing them requires a separate explicit overwrite approval.

## Publication and verification

Alpha publication proceeds after final local gates. If any release-blocking issue is found during publication or asset verification, block promotion to formal `v0.1.0` until it is resolved.

Documentation acceptance matrix: docs/USER_QUICKSTART.md covers Files, Models, Schedule, BBS, Process/Observability, and Release/Version with first-run expectations and evidence commands.
