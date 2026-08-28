# Changelog

This file records manually curated release changes for GenericAgent Admin Go.

## v1.0.99 - 2026-08-28

### Scheduled task workbench
- Refactor scheduled tasks into a Codex-inspired two-pane workbench with compact task selection, search and state filters, and a first-viewport layout focused on task operations.
- Show each selected task's recent execution history with task-scoped empty states, in-place artifact previews, and responsive mobile list/detail navigation while preserving reports navigation.
- Clear stale execution artifacts when a new task is created, keep the creation disclosure open on cancellation or failure, and prevent duplicate submissions during async creation.
- Add component, App smoke, and schedule regression coverage for filtering, history switching, empty states, preview cleanup, creation outcomes, and responsive workbench behavior.

## v1.0.98 - 2026-08-28

### Scheduled task execution history
- Show the selected scheduled task's recent execution history directly below the editor, with task-scoped empty states and responsive desktop/mobile stacking.
- Preview a selected execution artifact in place on the scheduled-task page while preserving the full reports subpage navigation from the task card.
- Invalidate stale artifact requests during task and route changes, release busy state safely, and cover task switching, empty history, in-place preview, navigation, and recovery regressions.

## v1.0.97 - 2026-08-28

### Upstream integration and local compatibility
- Sync upstream image input, chat sidebar, Loop, worldline, and app-dialog capabilities while preserving the local compact/privacy view, remote CMD, and update-restart flows.
- Make chat queue state authoritative through the backend REST/SSE API, add cross-platform directory-picker coverage, and keep the updater's direct GitHub download with configured and built-in mirror fallbacks.
- Fix repeated session snapshot polling for ISO `updated_at` values and extend mobile model/reasoning, attachment, queue, stop, and dialog regression coverage.

## v1.0.96 - 2026-08-25

### Remote CMD / Codex TUI
- Replace text-only remote terminal rendering with an xterm.js terminal that preserves raw ANSI/VT bytes, cursor movement, full-screen TUI state, and interactive Codex/Vim sessions.
- Forward terminal input one key event at a time with UTF-8 encoding, retain mobile shortcuts, and automatically fit and deduplicate ConPTY resize updates.
- Preserve replay/reconnect behavior and session ownership when clearing or disposing the browser terminal.
- Initialize Windows ConPTY `cmd.exe` sessions with `TERM=xterm-256color`, `COLORTERM=truecolor`, and `chcp 65001` so Codex can start its full-screen terminal UI.
- Add frontend, controller, terminal, Windows process, and ConPTY regression coverage, including exact Windows path placeholder rendering.

### Keychain management
- Add the upstream local keychain management API and settings page for storing and removing local credentials through the Admin interface.

## v1.0.95 - 2026-08-25

### Browser remote CMD
- Run a real Windows ConPTY CMD session on the Admin host from the browser, with server-side working directories and no local desktop window.
- Add a server directory browser, NDJSON output replay/reconnect, localStorage session recovery, mobile command shortcuts, and terminal ANSI/UTF-8 handling.
- Require Basic Auth for remote Local CMD routes even when anonymous remote access is enabled, and protect session creation, input, resize, and deletion with dangerous-action confirmation.
- Bound sessions, input, output replay buffers, idle lifetime, detached grace periods, and cleanup to prevent orphaned processes and unbounded resource use.
- Add backend, Windows ConPTY integration, authentication, protocol, cleanup, directory, and frontend regression coverage.

## v1.0.94 - 2026-08-24

### Local CMD workspace
- Add a lazy-loaded Local CMD menu and page where users can enter or browse to any local directory before opening a real independent Windows command prompt.
- Require an explicit dangerous-action confirmation for CMD process launch, with clear platform and security guidance in the UI.
- Validate trimmed absolute directories against the local filesystem, preserving support for spaces, Unicode, UNC paths, and drive paths without restricting access to the configured GA root.
- Launch `cmd.exe` with Windows `CreateProcessW`, a visible normal new console, interactive standard handles, a trusted absolute system path, and safe command-line quoting.
- Add backend and frontend regression coverage for confirmation, path validation, directory browsing, launch parameters, visible-window flags, handle cleanup, loading states, cancellation, and errors.

## v1.0.91 - 2026-08-18

### Compact chat footer
- Remove the detailed rounds, latency, cache, and token statistics footer from compact chat view.
- Keep the existing task status, generic metrics, and temporary latest-result reveal available while compact view is enabled.
- Preserve the full conversation statistics footer when compact view is disabled.

## v1.0.90 - 2026-08-18

### Temporary latest-result reveal
- Let compact chat view reveal only the latest completed assistant result while the status area is hovered or keyboard-focused.
- Add an explicit, time-limited reveal control for touch devices, with immediate hide and automatic cleanup of the sensitive DOM content.
- Keep execution logs, user prompts, titles, streaming output, and failure details concealed while preserving responsive desktop and mobile layouts.

## v1.0.89 - 2026-08-18

### Discreet compact chat display
- Present the local chat protection switch as the neutral “Compact view” option instead of exposing privacy-specific wording in the sidebar.
- Use ordinary numbered session and project labels, and keep the protected transcript focused on generic task status and usage metrics.
- Remove privacy-specific wording from visible copy, tooltips, notifications, and accessibility labels while preserving the same persistent redaction behavior.

## v1.0.88 - 2026-08-18

### Global chat privacy mode
- Add a persistent, device-local privacy switch after the current-instance selector for desktop and mobile chat sidebars.
- Replace chat content, project and session names, paths, tools, diagnostics, queues, and Loop details with anonymous labels and generic execution metrics while privacy mode is enabled.
- Redact existing and new chat notifications in the in-app inbox, toasts, notification page, and browser notifications without changing stored notification or conversation data.
- Restore the original chat display only after privacy mode is explicitly disabled, with regression coverage for persistence, DOM redaction, accessibility, and responsive layouts.

## v1.0.87 - 2026-08-17

### Update restart continuity
- Preserve the effective app root, fixed port, headless/browser mode, and Windows argument quoting across successful and rollback restarts.
- Reclaim the previous listener port when an older updater launches the new binary without its original arguments, allowing the existing upgrade page to advance from 95% to completion.
- Validate restart handoffs against the previous process ID so stale runtime metadata cannot redirect a normal launch.

## v1.0.86 - 2026-08-17

### Multi-turn worldline history
- Import every completed conversation turn as a distinct, ordered worldline node instead of collapsing an existing multi-turn session into one checkpoint.
- Repair persisted aggregate nodes created by earlier releases while preserving node identity, descendants, backend history, and message mappings.
- Keep incomplete turns out of the worldline and add regression coverage for fresh imports, legacy repair, idempotency, and continuous branches.

## v1.0.85 - 2026-08-17

### Windows background process windows
- Hide PowerShell service discovery, portable bootstrap, and manual scheduled-task processes so startup and page navigation no longer flash CMD windows.
- Keep intentional directory picker, Explorer, browser, and installer UI behavior unchanged.
- Add Windows regression coverage for `HideWindow` and `CREATE_NO_WINDOW` process attributes.

## v1.0.84 - 2026-08-17

### Responsive chat workspace
- Expand the desktop transcript and composer measure so wide screens devote more space to the active conversation.
- Keep Loop collapsed when chat first opens while preserving its launcher, live status, and round counter.
- Remove the desktop mobile-tools trigger that rendered duplicate controls below the viewport, and consolidate phone tools into one overflow menu.
- Use a single-row mobile header with denser messages, cards, sidebars, and composer chrome while retaining touch targets and iOS-safe input sizing.

## v1.0.83 - 2026-08-17

### Reliable self-update compatibility
- Restore the `--version-json` candidate metadata contract required by installed v1.0.78 updaters.
- Include and validate `release-manifest.json` in local and CI release packages so older installations can upgrade safely.
- Bound each download attempt, switch failed sources promptly, and report byte-based progress instead of remaining at 25% until completion.
- Validate the release manifest and candidate executable before replacing the running application.
- Keep the release secret-export gate strict while allowing the tracked MiSans license metadata required by the web build.
- Create Windows release ZIPs with portable forward-slash entries and verify their upgrade-critical paths before upload.

## v1.0.80 - 2026-08-17

### Security and release hygiene
- Reject cross-origin browser requests at the Admin API boundary instead of advertising wildcard CORS for file, process, model, and service controls.
- Add regression coverage for hostile origins, cross-site fetch metadata, and same-origin compatibility.
- Add a push/PR verification workflow, reject tracked local secret exports during release builds, and require `npm ci` for reproducible frontend dependencies.
- Align the README with the current authentication behavior, React/Vite versions, test commands, and Windows release matrix.

## v1.0.79 - 2026-08-16

### Manual scheduled-task run
- Add confirmed `POST /api/schedule/run` and `GET /api/schedule/run/status` so operators can launch a scheduled task immediately without waiting for the cron tick.
- Require `X-GA-Confirm: dangerous` on run, keep run IDs alphanumeric, and persist status/logs under `temp/ga-admin-schedule-runs`.
- Surface run progress in the schedule UI with i18n strings and regression coverage for confirm, prompt, and safe run IDs.

## v1.0.69 - 2026-08-12

### Latest merged build
- 发布合并后的最新代码，包含 Loop 首轮执行修复、移动端聊天布局优化，以及聊天结果图片预览改进。
- 保留实例管理帮助入口、控制模型状态同步和 Python 解释器失效回退等稳定性修复。
- 通过 Go、前端 lint、库测试、UI 回归和生产构建验证后发布 Windows 安装包。

## v1.0.68 - 2026-08-12

### Inline result previews in chat
- Recognize labeled screenshot and evidence image paths in tool output and render them as inline previews.
- Support local PNG, JPEG, WebP, BMP, and SVG result previews while filtering out fenced tool output, device paths, upload history, remote URLs, and unlabeled arbitrary paths.
- Add regression coverage for labeled image extraction and chat attachment rendering.
- Make build metadata choose the highest release tag reachable through a merge commit, avoiding an apparent version rollback after synchronization.

## v1.0.67 - 2026-08-11

### Loop 首轮执行与可操作示例
- 修复新会话启动 Loop 后长期停留在“进展 0/最大轮次”的问题，启动时会立即排队执行首轮目标。
- 补充 Loop 的真实可执行 Demo、轮次预期和结果检查说明，支持一键填入配置并继续调整。
- 完善 Loop 失败状态与执行结果回传，避免错误被显示为无限等待，并增加首轮执行、失败收敛和前端交互回归覆盖。

## v1.0.65 - 2026-08-10

### Simplified project TODO overview
- 将总览中的多模块卡片收敛为待处理、待执行、待同步、已完成四个互斥状态分组，点击即可筛选对应清单。
- 保留产品模块作为条目辅助标签，避免把不稳定的关键词归类误认为总览主分类。
- 未识别模块改为“其他”，不再错误归入“自主进化”，并补充状态筛选与未知模块回归测试。

## v1.0.64 - 2026-08-10

### Image-first chat previews
- Changed local image attachments and generated image cards to show the image preview without visible file paths, names, or extensions.
- Kept file details available on hover and keyboard focus, with download, open, folder, and copy-path actions grouped into the preview card.
- Added click-to-open full-size image previews and responsive image-only card coverage for chat attachments.

## v1.0.62 - 2026-08-08

### Mobile zoom navigation layout
- Fixed the overview observability grid staying in two columns below 420px, which compressed or clipped runtime details on iPhone-sized screens.
- Kept the selected interface scale usable across SPA route changes, including the chat sidebar height, width, and footer actions.
- Added responsive viewport regression coverage for 320px and 375px mobile layouts at enlarged and reduced scale.

## v1.0.61 - 2026-08-08

### Worldline runtime compatibility
- Fixed worldline requests loading GenericAgent's optional TUI implementation instead of the dependency-free Admin sidecar.
- Prevented `ModuleNotFoundError: No module named 'rich'` when the GA core virtual environment does not install the optional UI extras.
- Added a regression check covering packaged worldline import precedence.

## v1.0.60 - 2026-08-08

### Adjustable interface scale
- Added an 80%-120% interface scale control in 5% steps for the admin workspace, settings page, and mobile navigation/tool menus.
- Persisted the selected scale in the current browser, with a one-click reset and Ctrl/Cmd `+`, `-`, and `0` shortcuts.
- Fixed narrow 320px layouts at enlarged scale so mobile pages remain fully visible without horizontal clipping.

## v1.0.59 - 2026-08-08

### Direct access for trusted networks
- 取消外网、局域网和 Tailscale 访问的 HTTP Basic Auth，访问服务地址后直接进入管理页面，不再要求用户名和密码。
- 旧版 `GA_ADMIN_AUTH_*` 环境变量与 `auth.local.json` 不再参与请求拦截；保留认证状态接口兼容前端和旧客户端。
- 远程免登录访问应配合 Tailscale ACL、操作系统防火墙和 HTTPS 使用，不要将管理端口直接暴露到公网。

## v1.0.58 - 2026-08-08

### Mobile verification and fix
- 修复桌面导航分组样式覆盖移动端折叠规则的问题，手机端只显示导航触发器，打开后使用可滚动分组弹层。
- 完成 320px、390px 手机视口的真实浏览器验收，确认主页面、通道、实例、模型、对话、任务及其他功能页没有页面级横向溢出。
- 增加样式回归断言，防止移动端导航再次被桌面导航规则撑开。

## v1.0.57 - 2026-08-08

### User-facing changes
- 重构主导航，按工作区、服务与自动化、配置与监控分组，并新增 GA 实例入口，降低功能查找成本。
- 通道服务页增加全部、运行中、已停止筛选，并为筛选为空的状态提供准确提示。
- 优化移动端侧栏和模型选择器的可访问交互；移动端选择器使用全屏弹层、明确的服务商/模型语义和可恢复焦点。
- 修复 worldline 运行时侧车文件打包过期、错误流使用量未收尾，以及多实例 Chat/API 路由合并后的兼容问题。

### Validation
- Passed `go test ./...`, frontend lint, library tests (`244/244`), UI smoke tests (`93/93`), Python protocol tests, frontend production build, Windows `build.bat`, and live packaged HTTP checks.

## v1.0.56 - 2026-08-07

### User-facing changes
- 新增项目待办模块，在概览、文件、任务、模型、日志、服务和通知等页面提供按模块查看、筛选、搜索和回源入口。
- 优化聊天顶部工具栏，将上下文、世界线、更多工具和通知统一收拢到右侧，改善收起侧栏后的操作路径。
- 优化世界线面板的按钮反馈、节点标题截断、状态标识和窄屏浮层布局，减少长文本溢出并提升切换可读性。
- 改进配置归一化、服务进程状态、会话搜索、用量记录及相关 API 的稳定性和测试覆盖。

## v1.0.52 - 2026-08-05

### User-facing changes
- 默认关闭 HTTP Basic Auth，本机、局域网和 Tailscale 地址可以直接访问，不再被历史密码阻塞。
- 保留可选认证能力，可通过 `GA_ADMIN_AUTH_ENABLED=1` 或同时配置 `GA_ADMIN_AUTH_USER` 与 `GA_ADMIN_AUTH_PASSWORD` 重新启用。
- 认证状态接口明确返回当前是否启用认证，关闭认证时改密接口给出明确的 `auth_disabled` 提示。
- 修复 Windows 发布工作流的构建任务依赖配置，确保新版本标签可以正常生成发布包。

### Validation
- Passed `go test ./...`, frontend lint, library tests (`234/234`), frontend production build, authentication UI tests (`6/6`), Windows `build.bat`, and live HTTP checks on localhost and the detected Tailscale interface.

## v1.0.51 - 2026-08-04

### User-facing changes
- 待审批列表按真实审批状态过滤，自动排除已完成、无需审批和已失效的项目，避免用户重复处理。
- 待审批卡片增加“要解决什么问题”、审批场景和文件/配置/验证提示；报告包含多个方案时展示候选方案及推荐项。
- 无法可靠细分审批场景的项目明确标记为“需要人工确认”，并保留审批上下文，减少用户对审核依据的疑惑。
- 定期清理历史执行完成文件，避免临时报告长期堆积影响查看和磁盘空间。

### Validation
- Passed `go test ./...`, frontend library tests (`234/234`), UI smoke tests (`94/94`), frontend lint, frontend production build, and Windows `build.bat` packaging.

## v1.0.50 - 2026-08-04

### User-facing changes
- 修复 Windows 服务看护器启动后无法稳定拉起 scheduler/autonomous 的问题，启动前会清理旧看护器及其子进程树，避免端口冲突和重复实例。
- 服务看护器及其托管服务优先使用 `pythonw.exe` 和无窗口进程标志启动，避免反复弹出终端窗口。
- 管理端识别看护器实际拉起的外部服务状态，页面状态与 `45762/45763` 端口和真实进程保持一致。
- 停止看护器时同步停止其托管服务，主动停止记录为正常结束，避免页面显示误导性的失败返回码。

### Validation
- Passed `go test ./...`, frontend lint, library tests (`232/232`), UI smoke tests (`93/93`), Windows `build.bat`, and live watchdog recovery, stop, duplicate-start, port, and hidden-window checks.

## v1.0.49 - 2026-08-04

### User-facing changes
- 总览页移除重复的“服务控制”和“调度提醒”卡片，只保留后台服务与定时任务，并支持直接跳转到对应页面。
- 总览统计卡片在桌面端与手机端保持两列布局，避免空白、重叠和横向溢出。
- 兼容历史模型配置中以浮点数保存的 `availability_latency_ms`，加载后自动规范化为整数。

## v1.0.47 - 2026-08-03

### User-facing changes
- 修复 Windows `run.bat` 因前端锁文件缺少 `@emnapi/core` 和 `@emnapi/runtime` 条目而无法执行 `npm ci` 的问题。
- 自主进化页面的已处理审批记录按状态分类并支持折叠查看，保持批量审批结果清晰可追踪。

### Validation
- Passed frontend lint, library tests (`229/229`), UI smoke tests (`92/92`), frontend production build, clean `npm ci`, Windows `run.bat`, and HTTP startup checks on `127.0.0.1:8787` and the Tailscale interface.

## v1.0.46 - 2026-08-03

### User-facing changes
- Batch approval now shows live progress, processed totals, success and failure counts, the current item, and scrollable per-item results instead of leaving users with an indefinite waiting message.
- Failed approval items remain visible with their failure reason and can be retried as a batch after the initial operation finishes.
- Batch progress controls are responsive on mobile and prevent duplicate refresh or batch actions while processing.

### Validation
- Passed frontend lint, library tests (`228/228`), UI smoke tests (`91/91`), frontend production build, and focused batch approval progress coverage.

## v1.0.45 - 2026-08-03

### User-facing changes
- Autonomous approval model reviews now reuse the retry, timeout, API mode, user-agent, and reasoning settings configured on the Models page.
- Network failures and transient provider responses retry with exponential backoff; failed reviews retain their attempt count and next retry time, while manual re-review can retry immediately.
- Approval cards clearly distinguish model-unavailable rule screening from model-reviewed results, show the model conclusion and plain-language reason, and keep execution status and report links visible after approval.

### Validation
- Passed Go tests, frontend library tests (`228/228`), frontend UI smoke tests (`90/90`), frontend lint, frontend production build, and staged diff checks.

## v1.0.44 - 2026-08-02

### User-facing changes
- Automatically repairs a stale Windows `GenericAgent Admin` startup entry when an older packaged executable was removed, so Windows no longer tries to launch a missing target after an update.
- Keeps the current executable and application root in the migrated startup command while preserving the no-browser startup behavior.

### Validation
- Passed Windows autostart, GA, and version tests, frontend lint/library/UI tests, frontend production build, Windows packaging, and live startup-entry/browser checks.

## v1.0.42 - 2026-08-02

### User-facing changes
- Autonomous approvals now scan `temp/autonomous_reports` in reverse, so blocked or unverifiable approval reports such as R49/R50 cannot disappear when `pending_drafts.md` is absent or stale.
- Approval discovery scans the complete report set independently of the inventory display limit, while related reports are grouped into one review item instead of silently dropping older evidence.
- Approval candidates retain all related reports, expose the report-backed reason and confidence, and invalidate older unverified approval decisions when a newer audit proves the approval gate is still blocked.
- The approval page uses the selected `reflect/autonomous.py` model (falling back to the first ordered enabled model) for a bounded, read-only structured review; model failures keep the conservative human-review state and never auto-approve or modify files.
- Model review runs with a short response window, background completion/cache, retry backoff, and worker timeout so a slow or unavailable provider cannot block the approvals page.

### Validation
- Passed targeted Go approval/API tests, frontend lint, frontend library tests, frontend UI smoke tests, and Windows packaging validation.

## v1.0.41 - 2026-08-02

### User-facing changes
- Configured reflection and autonomous services now start before the Admin HTTP listener is exposed, so the first page load reflects their actual running state when “Start with GA Admin” is enabled.

### Validation
- Passed targeted service autostart tests, frontend lint/library/UI tests, Windows `build.bat`, and live browser restart checks.

## v1.0.40 - 2026-08-02

### User-facing changes
- Autonomous approval cards now show whether an approved task is queued, completed, failed, or finished without a report, and link directly to the matched execution report.
- Approval report matching now ignores audit documents that merely quote a TODO item, preventing false completed results.
- Approval decisions refresh the GA inventory so newly generated autonomous reports appear without a manual page reload.
- Fixed Windows autostart command quoting for GA roots ending in a backslash.

### Validation
- Passed `go test ./...`, frontend lint, frontend library tests (`223/223`), frontend UI smoke tests (`89/89`), Windows `build.bat`, and live browser/API checks on `http://127.0.0.1:8787/autonomous`.

## v1.0.39 - 2026-07-31

### User-facing changes
- Unified Chat, Goal Mode, autonomous services, and scheduled-task model pickers into one provider/model cascade with search, complete labels, keyboard navigation, and mobile bottom-sheet behavior.
- Scheduled tasks now expose the scheduler execution model, show the concrete GA default model when following the default, and let each task follow the scheduler or choose an explicit model.
- Prevented changing a running scheduler's model through a misleading control; stop the scheduler first so the next start uses the saved model consistently.
- Added responsive scheduler and task-form layout rules so provider names and model IDs do not overlap or get clipped on narrow screens.

### Validation
- Passed Go tests/build, frontend lint, frontend library tests (`209/209`), frontend UI smoke tests (`85/85`), frontend production build, and live `8787`/Tailscale API checks.

## v1.0.38 - 2026-07-31

### User-facing changes
- Fixed chat sessions falling back to the default reasoning strength instead of the selected model's saved `reasoning_effort` configuration.
- Chat model lists now expose the configured reasoning strength for each enabled provider model, including legacy provider-level settings.
- New sessions and model switches now display and send the selected model's configured reasoning strength, while preserving explicit per-session overrides.

### Validation
- Passed Go tests and build, frontend lint, frontend library/UI tests, frontend build, and live `8787` API model/settings checks.

## v1.0.37 - 2026-07-31

### User-facing changes
- Added a unified usage ledger for chat, title generation, side questions, autonomous evolution, scheduled tasks, Goal Mode, model probes, and other background model calls.
- Usage records now show the channel, source, provider/model, reasoning effort, token breakdown, and elapsed time, with filtering and CSV export.
- Added runtime telemetry installation and final usage flushing so automatic calls are recorded even when usage arrives in a terminal stream event.
- Fixed scheduled-task restarts after model dispatch changes so they retain usage telemetry context.

### Validation
- Passed Go tests, frontend lint/build, frontend library tests, and real GenericAgent runtime health checks.

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
