# GenericAgent Admin Go v0.1.0-alpha2 Release Notes

Status: alpha2 publication requested by release owner. This release is a follow-up alpha focused on Goal/Hive integration and Windows launch polish. Formal `v0.1.0` remains a separate approval boundary.

## 中文用户摘要

v0.1.0-alpha2 将 GA Admin 的协作能力收敛为官方 Hive/Goal 用法：Admin 不再内置 BBS 协作页面或 `/api/bbs/*` 管理接口，而是在 Goal 启动中提供 Hive 模式，负责上层生命周期管理；BBS 协议、帖子和 worker 协作继续由 GenericAgent 官方脚本承担。

同时，本版修复了 Windows 用户反馈的后台 Goal/Hive 启动时弹出终端窗口的问题：Windows 下优先使用 `pythonw.exe` 并保留隐藏窗口创建参数。

## Highlights

- Removed the built-in BBS collaboration UI/API from GA Admin.
- Added Hive mode to Goal start and state display.
- Starts official `assets/agent_bbs.py`, `reflect/goal_mode.py`, and `reflect/agent_team_worker.py` for Hive mode instead of reimplementing collaboration in Admin.
- Stop Goal now cleans the recorded Hive worker/BBS PIDs while still avoiding broad process-tree termination.
- Windows background launches prefer `pythonw.exe` to avoid terminal popups.
- Release workflow now permits `v0.1.0-alpha2` asset builds.

## Validation

Before tagging alpha2, the following gates were rerun successfully:

```text
go test ./...
go build ./...
npm.cmd test -- --run
npm.cmd run build
```

## Upgrade notes

- Users who relied on the old BBS page should switch to Goal Hive mode or GA official Hive commands.
- GA Admin displays Hive readme URL/PIDs/cwd for visibility, but it does not host posts, replies, or board-key storage.
- Download platform assets and verify matching `.sha256` files after the GitHub Release workflow finishes.
