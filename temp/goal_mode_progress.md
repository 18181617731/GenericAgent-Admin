# Goal Mode Progress

## 2026-05-20 02:11:49 Goal output observability
- Backend: `/api/goals/output` now returns `output`, `goal`, `truncated`, `bytes_returned`, `total_bytes` via `ga.GoalOutputResult`.
- `tailFile` reports total file bytes and truncation state, while missing log returns empty output with goal metadata.
- Tests updated in `internal/ga/goals_test.go` and `internal/api/goals_test.go` for truncation metadata and missing-log route behavior.
- Frontend: Goal output panel prepends localized truncation marker when API reports truncated output.
- Dist assets synced with current Vite hash (`index-BLVkF4Sp.js`, `index-CLBW0Xpr.css`).
- Verification: `go test ./...` and `npm --prefix web run build` passed.

## 2026-05-20 02:14:40 Goal output max_bytes cap observability
- Backend: GoalOutputResult now includes requested_bytes, max_bytes, max_bytes_capped so callers can tell when requested max_bytes was reduced to the 1MiB safety cap.
- Tests: internal/ga GoalOutput tests now assert tail/full/capped metadata including cap application on oversized requests.
- Frontend: Goal output panel prepends localized cap marker before truncation marker when max_bytes_capped is true.
- Verification: gofmt, go test ./..., and npm.cmd --prefix web run build passed. Dist synced to current Vite hash index-BpPKV2TX.js.

## 2026-05-20 02:15:41 Goal output default max_bytes observability
- Backend: GoalOutputResult now includes default_bytes_used to distinguish omitted/non-positive max_bytes from an explicit byte count.
- Tests: internal/ga GoalOutput metadata checks cover explicit tail, full read, default 64KiB behavior, cap behavior, and missing-log behavior.
- Verification: gofmt, go test ./..., and npm.cmd --prefix web run build passed after the metadata extension.

## 2026-05-20 02:16:34 Goal output API metadata regression coverage
- API route test now asserts requested_bytes/max_bytes/default_bytes_used/max_bytes_capped in /api/goals/output responses, preventing silent schema regressions between internal/ga and internal/api.
- Validation: gofmt internal/api/goals_test.go internal/ga/goals.go internal/ga/goals_test.go; go test ./...; npm.cmd --prefix web run build all passed.

## 2026-05-20 02:17:19 Goal output metadata README note
- README Goal Mode feature line now documents output tail metadata fields: truncated/bytes_returned/total_bytes/requested_bytes/max_bytes/default_bytes_used/max_bytes_capped.
- Verification: go test ./... and npm --prefix web run build passed after the doc update; web/dist was refreshed and added with -f.

- 2026-05-20T02:20:47 Added Goal output default/cap byte metadata markers to the React Goal output pane; verified go test ./... and npm --prefix web run build.

- 2026-05-20T02:22:41 Extended API route coverage for Goal output default max_bytes and 1MiB cap metadata; re-verified go test ./... and frontend build.

- 2026-05-20T02:25:47 Improved Goal page auto-refresh: entering the Goals tab now refreshes list/output immediately before the 3s poll interval; verified npm --prefix web run build.

- 2026-05-20T02:27:53 Localized Goal Mode start/stop toast messages and added missing EN capped-output hint; rebuilt web assets successfully.

- 2026-05-20T02:28:49 Verified Goal Mode route coverage boundaries: successful start route launches a real process, so avoided unsafe integration test; go test ./... passed.

- 2026-05-20T02:29:44 Added a selected-output refresh control to the Goal output panel and rebuilt web assets successfully.

- 2026-05-20T02:31:04 Added configurable Goal output max_bytes input in the output panel; npm web build passed.

- 2026-05-20T02:31:39 Updated README Goal Mode feature list to mention configurable output byte reads; go test ./... already passed after UI change.

- 2026-05-20T02:32:40 Fixed Goal output auto-refresh to react immediately when the configurable max_bytes value changes; npm web build passed.

- 2026-05-20T02:35:27 Added API route smoke test that launches a fake Goal runner through /api/goals/start, verifies GOAL_STATE/--reflect/--llm_no wiring, then stops by exact PID; go test ./... passed.

- 2026-05-20T02:36:37 Extended Goal API bad-input coverage for budget_minutes zero/negative fallback paths; go test ./... passed after gofmt.

- 2026-05-20T02:38:03 Strengthened /api/goals/start smoke test to assert runner cwd, GOAL_STATE env propagation, and stdout/stderr log capture; go test ./... passed.

- 2026-05-20T02:38:54 Documented Goal runner observability in README: GOAL_STATE state file path and stdout/stderr log append location; go test ./... passed.

- 2026-05-20T02:44:00 Moved Goal output truncation/cap/default byte notices out of the log text into structured React state badges; added badge styling; verified gofmt, go test ./..., and npm.cmd --prefix web run build.
- 2026-05-20T02:46:00 Added structured Goal output error badge path and re-ran npm.cmd --prefix web run build successfully.
- 2026-05-20T02:49:00 Added frontend validation/i18n for Goal output max_bytes (integer, non-negative, <=1048576) before API calls; npm.cmd --prefix web run build passed.
- 2026-05-20T02:52:00 Added Goal output byte preset buttons (64K/256K/1M/All) with compact styling so users can quickly switch tail size without typing; npm.cmd --prefix web run build passed.
- 2026-05-20T03:05:00 Added Goal row quick-open buttons for state/log files, wired to Files panel reader; log button is disabled when log is missing. npm.cmd --prefix web run build passed.
- 2026-05-20T03:12:00 Added README Goal Mode control-console section documenting reuse of reflect/goal_mode.py, GOAL_STATE temp/goal_admin_<id>.json, exact-PID stop constraint, and start/list/stop/output API behavior.
- 2026-05-20T03:18:00 Fixed Goal output preset wording: max_bytes=0 is backend default tail size, not full log; UI now says 默认64K / Default 64K. npm build passed after change; go test ./... re-run.
- 2026-05-20T03:25:00 Fixed finished Goal run elapsed/remaining drift: metaFromState now uses end_time when present; added TestMetaFromStateUsesEndTimeForFinishedRuns.
- 2026-05-20T03:30:00 Verified after end_time drift fix: gofmt + go test ./... passed; npm.cmd --prefix web run build passed. Working checkpoint updated; continuing UX/test gap scan.
- 2026-05-20T03:40:00 Added Goal run progress bars in web UI: turn usage percent and elapsed budget percent with tooltip labels; CSS added for compact dual bars. Frontend build passed after JSX/CSS change.
- 2026-05-20T03:50:00 Fixed finished Goal runs with stale live PID: metaFromState now reports running=false when end_time exists, preventing recycled/current PID from making completed runs look active; added regression test using os.Getpid().
- 2026-05-20T04:25:00 Hardened StopGoal against stale ended states: it now refuses to kill when end_time exists or status is not running, even if the request supplies the stored PID; added regression test ensuring no kill callback is invoked and state remains unchanged.
- 2026-05-20T04:32:00 Added API route regression for stopping an already-ended Goal state carrying a stale PID: /api/goals/stop returns 400, includes 'goal is not running', and leaves the state file byte-for-byte unchanged.
- 2026-05-20T04:40:00 Added frontend Goal stop confirmation in zh/en: warns that only the exact PID recorded for the Goal will be terminated before calling /api/goals/stop.
- 2026-05-20T04:48:00 Updated README Goal stop documentation: backend only stops records still marked running with exact PID; frontend asks for confirmation before stop.
- 2026-05-20T04:58:00 Enhanced Goal stop confirmation to include concrete Goal id and PID; frontend build passed.
- 2026-05-20T05:06:00 Added frontend ended-time display for completed Goal rows using end_time; frontend build passed.
- 2026-05-20T05:12:00 Added API test assertion that /api/goals/output response carries the requested goal id; internal/api test passed.

[2026-05-20 03:24:19] Added internal/ga test TestStartGoalFailurePersistsFailedStateAndLog to lock start_failed state/end_time/log marker when interpreter launch fails; internal/ga tests passed.

[2026-05-20 03:25:51] Added Goal output copy button with localized label; web build passed.

[2026-05-20 03:26:48] Hardened Goal output copy action with textarea fallback for non-secure/non-clipboard contexts; npm build and go test passed.

[2026-05-20 03:27:51] Added localized Goal output copy success/error feedback and passed setMsg into GoalsPage; npm build passed and go test ./... passed.

[2026-05-20 03:28:19] Documented Goal output presets/truncation badges and clipboard textarea fallback in README; npm build and go test ./... passed.

[2026-05-20 03:29:50] Hardened Goal output tailing to avoid splitting UTF-8 runes at max_bytes boundary; added regression test; go test ./internal/ga focused and go test ./... passed; npm build passed.

## 2026-05-20 03:35:57
- Added Goal output auto-refresh toggle in React state/GoalsPage wiring and i18n labels.
- Re-ran gofmt, go test ./..., npm.cmd --prefix web run build: all passed.
- Next: inspect diff for accidental dist churn and find next UX/API hardening point.

## 2026-05-20 03:37:15
- Added CSS for Goal output auto-refresh toggle and reran gofmt, go test ./..., npm.cmd --prefix web run build: all passed.
- Observed web/dist hash churn after build; tracked dist is intentionally rebuilt with Vite.
- Next: inspect Goal auto-refresh effect and backend output metadata for race/error UX hardening.

## 2026-05-20 03:38:00
- Hardened Goal auto-refresh against overlapping polls and stale output responses using refs/sequence guard.
- Verified npm.cmd --prefix web run build and go test ./... passed.
- Next: inspect backend Goal stop/list observability and frontend edge cases for another concrete improvement.

## 2026-05-20 03:39:50
- Hardened stale Goal metadata: when a state remains `running` but PID is not alive and no `end_time` exists, Admin now reports status `exited` and freezes elapsed/remaining at the state file mtime instead of counting forever.
- Added regression test `TestMetaFromStateFreezesExitedRunningStateAtModTime`.
- Verified `gofmt`, `go test ./...`, and `npm.cmd --prefix web run build` passed.
- Next: inspect API/frontend presentation of missing logs and output errors for another concrete UX improvement.

## 2026-05-20 03:40:30
- Fixed mojibake in `internal/ga/goals_test.go` UTF-8 sanitize fixture back to `中文`.
- Re-ran gofmt, `go test ./...`, and `npm.cmd --prefix web run build`: all passed.
- Previous PowerShell heredoc attempt failed before running tests; corrected by splitting commands.

## 2026-05-20 03:41:30
- Verified after strict JSON decode hardening attempt: `go test ./...` passed and `npm.cmd --prefix web run build` passed; only the progress-recording Python one-liner failed due quoting.
- Repository status still shows expected Goal/API/frontend/dist working changes.
- Next: inspect current API decode implementation and route tests to confirm strict trailing-payload coverage, then add another concrete hardening point.

## 2026-05-20 frontend output observability
- Improved Goal Mode output panel in `web/src/App.jsx`:
  - added byte formatter and output statistics derived from API metadata/current text;
  - shows displayed bytes, line count, configured read limit, and backend metadata badges (`bytes`, `lines`, `truncated`, `tail`, etc. when present);
  - added localized `outputShown`/`outputLines`/`outputLimit` labels in zh/en.
- Validation:
  - `cd web && npm.cmd run build` passed (Vite production build).
  - `go test ./...` passed.

- 2026-05-20T05:20:00 Added compact Goal output statistics cards (returned/total bytes, rendered line count, active max_bytes limit) above metadata badges; restored missing .goal-output-stats styling; npm run build and go test ./... passed.
## 2026-05-20 frontend output error-state polish
- Patched `web/src/App.jsx` Goal output fetch failure path: failed `/api/goals/output` now replaces stale output with the error text and supplies fallback byte/count metadata so the output panel summary remains honest instead of showing previous content with an error banner.
- Verified `npm.cmd --prefix web run build` succeeds after the patch.
- Verified `go test ./...` succeeds after the patch.


## 2026-05-20 05:24:39 - Frontend output line count accuracy
- Fixed `outputLineCount` in `web/src/App.jsx` to ignore one trailing newline, so Goal output metadata reports visible log lines instead of counting a terminal newline as an extra blank line.
- Validation: `npm run build` in `web/` passed; `go test ./...` passed.

## 2026-05-20 05:29 输出行数元数据闭环
- 后端 GoalOutput 增加 lines_returned / total_lines，读取输出时基于 tail 结果与原文件流式统计暴露总行数。
- API/ga 测试补充行数元数据断言，覆盖截断输出仍能显示返回行数与总行数。
- 前端输出卡片改用后端行数元数据，显示 `返回 / 总计`，避免仅按本地截断文本误报。
- 验证：`go test ./...` 通过；`web` 目录下 `npm.cmd run build` 通过。

- 2026-05-20 05:33:49 Goal list meta now exposes budget_percent/turn_percent from backend; UI prefers server percentages with local fallback. Verified `go test ./...` and `npm.cmd --prefix web run build`.

## 2026-05-20 05:35:35 frontend output line metadata mapping
- Fixed Goals output meta mapping in web/src/App.jsx: API snake_case lines_returned/total_lines now mapped to camelCase linesReturned/totalLines used by UI.
- Validation: go test ./... passed; npm.cmd --prefix web run build passed.

## 2026-05-20 07:13:56 - output metadata UX validation
- Frontend Goal output panel now displays shown/total bytes, shown/total lines, active output limit, and warning/default/cap badges from API metadata.
- Added responsive styles for output stats, metadata badges, and output limit presets in web/src/style.css.
- Validation: npm run build passed in web/; go test ./... passed at repo root.
- Notes: working tree contains prior release/dist artifacts; no global memory updated.

- 2026-05-20T05:20:00 Added internal/ga regression coverage for zero-byte existing Goal logs: GoalOutput returns empty output with log_exists=true, missing_log=false, default byte metadata, and zero returned/total bytes. Verified go test ./internal/ga and go test ./... passed.

## 2026-05-20 07:18:17 - goal empty-log API coverage
- Added API route coverage for a zero-byte existing goal log alongside missing-log coverage.
- Verified list flags distinguish missing log (`missing_log=true`, `log_exists=false`) from empty existing log (`missing_log=false`, `log_exists=true`).
- Verified `/api/goals/output?id=route_empty_log&max_bytes=5` returns HTTP 200 with empty output and zero byte/line metadata, without truncation/default/cap flags.
- Validation: `gofmt -w internal/api/goals_test.go`; `go test ./internal/api`; `go test ./...` all pass.

Git status:
```text
MM README.md
MM internal/api/api.go
AM internal/api/goals_test.go
AM internal/ga/goals.go
AM internal/ga/goals_test.go
M  internal/service/service.go
A  internal/service/service_test.go
AM temp/goal_mode_progress.md
AD web/dist/assets/index-CLBW0Xpr.css
AD web/dist/assets/index-CUr04SVj.js
D  web/dist/assets/index-O4ivZUq9.js
D  web/dist/assets/index-eQk42TvX.css
MM web/dist/index.html
MM web/src/App.jsx
MM web/src/style.css
?? release/GenericAgent-Admin-Go-v0.0.4-windows-amd64.zip
?? release/GenericAgent-Admin-Go-v0.0.5-windows-amd64.zip
?? release/v0.0.1/
?? release/v0.0.2/
?? release/v0.0.3/
?? release/v0.0.5/README.txt
?? temp/ga-admin-tray-check.exe~
```
Diff stat:
```text
README.md                          |  19 ++++
 internal/api/api.go                |  14 ++-
 internal/api/goals_test.go         | 108 ++++++++++++++++++++++-
 internal/ga/goals.go               | 125 +++++++++++++++++++++++++--
 internal/ga/goals_test.go          | 173 ++++++++++++++++++++++++++++++++++++-
 temp/goal_mode_progress.md         | 102 ++++++++++++++++++++++
 web/dist/assets/index-CLBW0Xpr.css |   1 -
 web/dist/assets/index-CUr04SVj.js  |  35 --------
 web/dist/index.html                |   4 +-
 web/src/App.jsx                    | 143 ++++++++++++++++++++++++------
 web/src/style.css                  |  12 +++
 11 files changed, 659 insertions(+), 77 deletions(-)
```

## 2026-05-20 07:19:47 - frontend missing-log output badge
- Patched `web/src/App.jsx` Goal output UI to surface backend `goal.missing_log` as a localized warning badge in the output metadata area.
- Added zh/en hint text so a selected Goal whose log file has not been created yet clearly says there is no readable output, instead of only showing an empty log panel.
- Validation: `npm.cmd run build` in `web/` passed; `go test ./...` passed.
- Notes: existing working tree still contains prior Goal Mode changes, generated `web/dist` asset churn, release artifacts, and temp executable backup; no global memory updated.

- 2026-05-20T05:18:00 Hardened Goal start API budget_minutes handling: /api/goals/start now rejects budget_minutes >43200 before multiplying to seconds/launching; added route regression ensuring 400 response and no goal state file is created. Also kept JSON single-value decode validation and GA positive-budget/start-failure/stale-ended-stop regressions green. Verification: gofmt and go test ./... passed.

## 2026-05-20 07:57:54
- Added frontend Goal output metadata retention (`goal: d.goal || null`) so missing-log/log-ready badges can reflect `/api/goals/output` refreshed goal state instead of stale list data.
- Verified: gofmt, `go test ./internal/ga ./internal/api`, `web: npm.cmd run build` (previous step) all pass.


## 2026-05-20 08:02 - Goal 输出面板清空 UX
- 前端 `web/src/App.jsx`：在 Goal 输出面板新增 `清空/Clear` 操作，清空当前 output 与 outputMeta，并通过既有 toast 文案提示。
- 补齐中英文文案：`clear`、`goalOutputCleared`；沿用 `XCircle` 图标与现有 actions 样式，避免新增样式风险。
- 验证：`npm --prefix web run build` 通过；`go test ./internal/ga ./internal/api` 通过。
- 注意：构建会刷新 `web/dist` 产物，后续提交前按 release 策略确认是否纳入。
- 2026-05-20T08:06:23 Added regression coverage for Goal meta negative progress inputs: future start_time and negative turns now assert elapsed/remaining/percents clamp to zero; verified gofmt, go test ./internal/ga, go test ./..., and npm.cmd --prefix web run build pass.
- 2026-05-20T08:08:08 Hardened Goal meta percent calculation for corrupted/negative turns_used by clamping turns before deriving turn_percent; verified gofmt, go test ./internal/ga, go test ./..., and npm.cmd --prefix web run build pass.

- 2026-05-20T05:18:00 Extended Goal API bad-input regression coverage: /api/goals/start now rejects budget_seconds above 30 days and trailing concatenated JSON before launch, budget_minutes overflow/over-cap remains rejected, and /api/goals/stop rejects trailing concatenated JSON; verified gofmt, go test ./internal/api, go test ./..., and go build . passed.

- 2026-05-20T05:32:00 Hardened Goal metadata reporting for corrupted/edge state: negative turns_used is now reported as 0, turns_used above max_turns is capped in API metadata, and percent fields stay 0..100; added internal/ga regression tests; verified gofmt, go test ./internal/ga, go test ./..., and go build . passed.


## 2026-05-20 08:17:13 - output default max_bytes route coverage
- Added API route regression coverage in `internal/api/goals_test.go` for `/api/goals/output?max_bytes=0`.
- Verified route returns full output with `default_bytes_used=true`, `requested_bytes=0`, `max_bytes=64*1024`, and no truncation/capping.
- Ran `gofmt -w internal/api/goals_test.go`.
- Verification: `go test ./internal/api` passed; `go test ./...` passed.

Current diff stat:
```text
README.md                          |  19 ++++
 internal/api/api.go                |  18 +++-
 internal/api/goals_test.go         | 154 ++++++++++++++++++++++++++++-
 internal/ga/goals.go               | 132 ++++++++++++++++++++++++-
 internal/ga/goals_test.go          | 198 ++++++++++++++++++++++++++++++++++++-
 temp/goal_mode_progress.md         | 174 ++++++++++++++++++++++++++++++++
 web/dist/assets/index-CLBW0Xpr.css |   1 -
 web/dist/assets/index-CUr04SVj.js  |  35 -------
 web/dist/index.html                |   4 +-
 web/src/App.jsx                    | 145 +++++++++++++++++++++------
 web/src/style.css                  |  12 +++
 11 files changed, 815 insertions(+), 77 deletions(-)
```


## 2026-05-20 08:17:56 - frontend production build verification
- Windows subprocess could not launch bare `npm` even though PATH resolves it as `npm.CMD`; reran with explicit `npm.cmd` per Windows convention.
- Verification: `cd web && npm.cmd run build` passed.
- Build regenerated committed `web/dist` assets: `index-BqNFstNZ.css` and `index-nzOfjkyY.js` referenced from `web/dist/index.html`.

Current diff stat:
```text
README.md                          |  19 ++++
 internal/api/api.go                |  18 +++-
 internal/api/goals_test.go         | 154 ++++++++++++++++++++++++++++-
 internal/ga/goals.go               | 132 ++++++++++++++++++++++++-
 internal/ga/goals_test.go          | 198 ++++++++++++++++++++++++++++++++++++-
 temp/goal_mode_progress.md         | 197 ++++++++++++++++++++++++++++++++++++
 web/dist/assets/index-CLBW0Xpr.css |   1 -
 web/dist/assets/index-CUr04SVj.js  |  35 -------
 web/dist/index.html                |   4 +-
 web/src/App.jsx                    | 145 +++++++++++++++++++++------
 web/src/style.css                  |  12 +++
 11 files changed, 838 insertions(+), 77 deletions(-)
```


## 2026-05-20 08:19:10 - verification after dist index sync
- Re-ran backend full suite: `go test ./...` passed.
- Re-ran frontend production build with explicit Windows command: `cd web && npm.cmd run build` passed.
- `web/dist` is ignored by `.gitignore`, so the generated production bundle was synchronized with `git add -f web/dist` to avoid stale staged deletes/adds.
- Cached `web/dist` changes now reflect the current Vite build artifacts:
```text
A	web/dist/assets/index-BqNFstNZ.css
D	web/dist/assets/index-O4ivZUq9.js
D	web/dist/assets/index-eQk42TvX.css
A	web/dist/assets/index-nzOfjkyY.js
M	web/dist/index.html
```

Current working tree summary:
```text
MM README.md
MM internal/api/api.go
AM internal/api/goals_test.go
AM internal/ga/goals.go
AM internal/ga/goals_test.go
M  internal/service/service.go
A  internal/service/service_test.go
AM temp/goal_mode_progress.md
A  web/dist/assets/index-BqNFstNZ.css
D  web/dist/assets/index-O4ivZUq9.js
D  web/dist/assets/index-eQk42TvX.css
A  web/dist/assets/index-nzOfjkyY.js
M  web/dist/index.html
MM web/src/App.jsx
MM web/src/style.css
?? release/GenericAgent-Admin-Go-v0.0.4-windows-amd64.zip
?? release/GenericAgent-Admin-Go-v0.0.5-windows-amd64.zip
?? release/v0.0.1/
?? release/v0.0.2/
?? release/v0.0.3/
?? release/v0.0.5/README.txt
?? temp/ga-admin-tray-check.exe~
```

## 2026-05-20 Start time surfaced in Goal list
- Added `start_time` to Goal metadata/API view so list responses preserve the SOP runner start timestamp instead of only derived elapsed/remaining values.
- Surfaced start time in Goals UI metadata row with zh/en `started` labels, alongside elapsed/remaining/end/update times.
- Verified with `go test ./...` and `npm.cmd --prefix web run build`.

## 2026-05-20 08:29:26 - Goal output selected-summary UX
- Added a selected-goal summary card above the Goal output tail in `web/src/App.jsx`.
- Summary exposes status/id/objective, turn progress, elapsed/remaining budget, started/updated timestamps, pid, and compact progress bars.
- Added responsive styles in `web/src/style.css` (`goal-output-summary`, `goal-summary-grid`, `summary-progress`).
- Validation: `cd web && npm run build` passed; `go test ./...` passed.


## 2026-05-20 08:31:23 - Goal output summary file shortcuts
- Added direct state/log file shortcut buttons to the selected-goal summary card above the Goal output tail in `web/src/App.jsx`.
- Buttons reuse existing file viewer via `onFile(...)`; log shortcut is disabled when the log path is absent or marked missing.
- Added compact `goal-summary-files` button styling in `web/src/style.css`.
- Validation: `cd web && npm run build` passed; `go test ./...` passed.

## 2026-05-20 08:35:18 Goal output status observability
- Added backend output_status for /api/goals/output: full, tail_truncated, empty_log, missing_log.
- Surfaced localized output status badge in Goal output panel.
- Extended Go tests for missing/empty/full/tail-truncated log states.
- Verified: go test ./internal/ga; cd web && npm run build.

## 2026-05-20 output_status API test hardening
- Added explicit `output_status` JSON assertions to `internal/api/goals_test.go` for full, tail_truncated, empty_log, and missing_log output cases.
- Verified with `go test ./internal/api ./internal/ga` (pass).
- Next: search adjacent Goal Mode API/UX hardening beyond output_status.

## 2026-06-09T03:43:00 BBS dangerous header guard coverage
- Added backend regression coverage for `/api/bbs/config` and `/api/bbs/posts` explicit `X-GA-Confirm` guards.
- Verified backend/API/full build/frontend gates passed; no live 8787/chat-worker/process actions, no commits/releases/secrets.

## 2026-06-09T04:25:00 Files/Logs config load validation hardening
- Added config load validation so persisted `config.local.json` is checked with `Validate(cfg)` before replacing in-memory defaults.
- Added regression coverage for invalid persisted `port`, `log_tail_lines`, and `buffer_lines`, ensuring bad runtime bounds are rejected and do not mutate `Store.Cfg`.
- Verified `go test ./internal/config -count=1`, `go test ./internal/... -count=1`, `go test ./... -count=1`, `npm.cmd --prefix web test`, and `npm.cmd --prefix web run build` all passed.
- No live 8787/PID189940/chat_worker interaction; no commit/push/release/delete/secret reads.

## 2026-06-09T04:45:00 Schedule raw/task contract compatibility
- Fixed Schedule task API/frontend contract mismatch by accepting `raw` or `task` in PUT and returning both fields on GET/PUT.
- Added raw round-trip regression coverage plus existing nested-id dangerous-route coverage.
- Verified targeted Schedule tests, full internal Go tests, web tests, web build, and `git diff --check`; no live 8787/PID189940/chat_worker interaction; no commit/push/release/delete/secret reads.

## 2026-06-09T05:15:00 Models import-mykey save/reveal guard hardening
- Hardened `/api/models/import-mykey` conditional dangerous-header guard so both `reveal=true` and `save=true` require explicit `X-GA-Confirm`; masked preview (`reveal:false, save:false`) remains safe and header-free.
- Added regression coverage for reveal guard, confirmed reveal response, save-with-masked refusal after confirmation, and dangerous-header route coverage for the conditional models import path.
- Verified `gofmt`, `go test ./internal/api -run TestModelsImportMyKey|TestDangerous|TestRiskCatalog|TestMutatingRoutes|TestDocumentedSafe -count=1`, and `node --test web/src/lib/dangerous_api_contract.test.mjs` all passed.
- No live 8787/PID189940/chat_worker interaction; no commit/push/release/delete/secret reads.
