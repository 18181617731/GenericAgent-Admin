# Changelog

This file records manually curated release changes for GenericAgent Admin Go.

## v0.4.3

- **弱网列表增量同步**：会话列表首轮传输精简摘要，后续仅同步新增、变更、删除项及变化的项目元数据；无变化返回空正文，保留旧客户端兼容与实时运行状态判断。
- **会话首屏按需加载**：大型已完成助手消息先展示最终答复与执行过程计数，展开执行过程或复制消息时再获取完整原文；无法可靠识别的内容仍完整返回，不修改持久化原文。
- **加载隔离与恢复**：完整原文缓存按内容修订复用，切换会话或实例隔离旧请求；加载失败可重试，列表同步基线不匹配时重新获取全量，保留故障后恢复能力。
- **验证边界**：前端 lint、520 项库测试与构建，11 项 hook/UI 定向回归，Go 全量测试与构建通过；浅色/深色/暖色桌面与 390px 窄屏的隔离真实 React 交互通过。100 KiB/s、80ms 延迟的单样本模拟请求证明首屏传输减量，不代表真实外网或完整开页耗时保证；截图已保存但视觉服务超时，未完成完整视觉复核或手机真机验收。发布不自动部署或重启现有实例。

## v0.4.2

- **更新日志恢复**：执行日志按更新操作持久化，显示阶段变化、错误与下载重试；旧状态没有日志时回退显示已有消息或错误，纯进度刷新不重复刷日志。
- **真实下载进度**：独立显示文件下载百分比、已下载/总字节数和重试次数；总大小未知时只显示已下载量，重试重置计数，完成前不提前显示 100%。准备更新与确认重启仍分开执行。
- **侧栏偏好后端持久化**：侧栏布局偏好保存到 Admin 后端，恢复时以对应实例的持久化数据为准，不再仅依赖当前浏览器的本地存储。
- **回放资源回收**：完成运行的回放清理采用可取消、幂等的计时器，替换运行时释放旧回放引用，保留当前运行的游标回放和取消落盘握手。
- **空闲 worker 回收**：Python worker 在最后一个请求释放后空闲 10 分钟自动回收；重开会话从持久化历史恢复，并加强并发启动、退出与关闭的生命周期隔离。
- **验证边界**：17 项更新专项回归、前端 lint/库测试/构建、Go 全量测试与构建通过；浅色/深色/暖色桌面与 390px 窄屏的更新弹窗交互检查通过。未宣称完整视觉验收、手机真机、生产更新端到端或全应用无内存泄漏；发布不自动部署或重启现有实例。

## v0.4.1

- **版本更新红点**：Chat 左下角版本号后以 6px 静态红点提醒可用的新版本；不再展示额外提醒条或新增提醒开关，保留原版本更新入口。
- **更新前持续提醒**：打开或关闭更新窗口、刷新页面及旧的已读记录均不会清除红点；临时检查失败或同一运行版本下后续检查无更新时仍保留，实际运行新版本后重新判断。静默检查不自动下载、安装或重启。
- **会话传输优化**：会话列表与详情支持 gzip 和 ETag/304，合并后台重复列表请求并复用有界缓存；写后刷新、运行状态变化及实例切换仍按实际快照判断。
- **发布校验修复**：样式契约检查兼容 LF/CRLF 检出，避免 Windows 换行差异导致误报。
- **验证边界**：前端 lint、库测试及构建，更新提醒 28 项定向回归，以及浅色/深色/暖色桌面与 390px 窄屏模拟 API 交互通过。发布前 Go 全量测试仅真实 GitHub 网络检查因匿名 API 限流（403）失败，排除该外网依赖后的其余测试与 Go 构建均通过；未宣称完整 UI 全测、生产外网提速、手机真机或生产部署验收。

## v0.4.0

- **侧栏自定义与选项卡**：支持原地调整版块显示、排序、显示数量及选项卡归属；同一版块可出现在多个选项卡，提供选项卡管理、重命名与持久化图标选择。再次点击已选中的选项卡可打开菜单，名称显隐统一作用于已选中的选项卡。
- **侧栏体验优化**：采用图标优先导航，收紧工具、搜索与版块间距，弱化装饰和静态箭头；管理操作改为浮动菜单，恢复版本更新入口，隐藏选项卡滚动条但保留横向滚动；浅色主题使用中性控件与独立选中底色。
- **更新镜像设置**：系统更新弹窗新增 GitHub 镜像地址与独立保存确认；留空直连，保存前读取最新配置，失败保留草稿，更新忙碌期间禁用编辑。
- **问题修复**：修复侧栏子菜单裁切、空置顶版块不显示、选项卡图标初始化异常；复制会话 ID 的反馈支持约 3 秒自动消失及重复复制重新计时；修复 Markdown 表格列被压缩。
- **依赖维护**：将 PostCSS 使用的 nanoid 固定至已修补版本 3.3.18。
- **验证边界**：前端 lint、502 项库测试与构建通过，镜像设置 25 项专项回归及浅色/深色/暖色桌面与 390px 窄屏交互、截图复核通过；发布前 Go 串行全量测试与构建通过。未宣称全量 UI 测试或手机真机验收；镜像实页验收拦截写请求，不代表生产配置保存或实际镜像下载已经验证。

## v0.3.20

- **更新入口与弹窗**：新增独立、按更新状态呈现的弹窗，可从 Chat 左上角版本号打开；区分更新说明、日志、下载及已准备状态，安装与重启仍需明确确认。
- **安全更新说明**：发布说明支持 Markdown 展示，过滤不安全的 HTML 与链接。
- **吞吐统计修复**：输出 tok/s 使用完整模型请求耗时计算，避免与界面展示的模型耗时口径不一致。
- **指挥家中途指导**：新增 `conductor_instruct`，向本父会话正在运行或排队的 worker 投递增量指令；在下一次模型调用边界送入，失败可重试，不中止 worker 或更换模型。
- **指挥家进度查询**：新增 `conductor_inspect`，按需查看所属 worker 的公开阶段、步骤、摘要及活动工具耗时；进度不是交付或验收证据，终态结果仍通过 `conductor_collect` 获取。
- **验证边界**：前端 lint、490 项库测试与构建，35 项相关 UI 回归，Python Conductor 33 项、协议 45 项、中途指令与真实 GA 循环安装夹具，以及 Go 专项与构建通过。Go 全量测试有 3 项 Windows 文件占用/重命名失败，隔离单跑均通过，不宣称全测通过；UI 全量测试亦有非本次范围失败，不宣称全量 UI 通过。真实 GA 循环覆盖使用假模型，不代表生产模型端到端验收。

## Unreleased

### v0.3.6 iteration (not released)
- **Conductor delivery review:** Separate execution completion from pending/verified/needs-work review; persist parent review basis, dispatch-owned tool evidence references, and unverified boundaries. Worker completion prose is not verification evidence.
- **Usage:** Expose parent, finalized-child, and recorded-total input/output tokens, with explicit missing-snapshot and incomplete-running-usage notices. Normalize cache protocols per call and preserve dispatch snapshot isolation.
- **Bounded dispatch:** Share a cumulative 48-dispatch limit between enforcement and API metadata; count worker reuse and all terminal outcomes. Regression covers accepting dispatch 48 and rejecting dispatch 49 without mutating the parent.
- **Documentation:** Explain Conductor versus automatic continuation modes and the limits of parent review and usage accounting.
- **Validation boundary:** Backend handler/persistence tests and fixture-based UI tests are not a live browser-to-worker end-to-end acceptance. No production deployment or release is implied.

### Bug Fixes
- **Chat:** Fixed cache hit rate calculation to use correct formulas for different API types
  - **Claude API (Modern)**: `cache_read / (output + cache_read)` — portion of generated content from cache
  - **Non-Claude API (Legacy)**: `cached / output` — cache-to-output ratio
  - **Mixed sessions**: weighted average by denominator size (previously only used Modern formula)
  - Automatically detects API type and applies appropriate formula

## v0.2.16 - 2026-08-24

### New Features
- **Models:** Added optional Chinese-friendly provider display names without changing configuration variable identities.
- **Models:** Added stable model instance identities, allowing the same model ID to be configured more than once under one provider and referenced independently in failover groups.
- **Chat:** Moved guided-message queues from browser-local storage to durable per-session server state.

### Improvements
- **Chat:** Large file-change summaries now collapse automatically and use a compact, stable layout that keeps assistant output visible.
- **Update:** Staged updates now require an explicit restart authorization step before replacing the running application.

### Reliability
- **Update:** Hardened replacement failure recovery so interrupted updates restore and restart the original service safely.
- **Compatibility:** Legacy failover references without an instance ID remain supported when they resolve to exactly one model instance; ambiguous references now fail explicitly instead of selecting the wrong instance.

### Validation
- Go tests and build pass across the full repository.
- Web lint, tests, and production build pass.

## v0.2.15 - 2026-08-21

### Bug Fixes
- **Chat:** Fixed cache hit rate calculation for legacy API models (OpenAI, etc.)
  - Legacy APIs report `cached_tokens` as a subset of `input_tokens`, but the old algorithm added both to the denominator, causing double-counting
  - Modern APIs (Claude) report `input_tokens`, `cache_creation_tokens`, and `cache_read_tokens` as disjoint buckets
  - The algorithm now detects API type per-turn and correctly handles mixed legacy/modern usage in the same session
  - Example: Legacy `input=100, cached=80` now correctly calculates as `80/100 = 80%` instead of incorrectly `80/180 = 44%`

- **Chat:** Exposed live tool timing in real-time during execution
  - Tool timing now updates progressively as tools run, not only after completion
  - Improves transparency for long-running operations

- **Chat:** Preserved assistant prose and tool call ordering in message display
  - Fixed rendering issue where tool calls could appear out of sequence relative to explanatory text

### Validation
- All 309 lib tests pass
- Web build successful
- No breaking changes

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
