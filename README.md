# GenericAgent Admin Go

GenericAgent Admin Go 是 GenericAgent 的桌面级管理面板：Go 后端负责进程、文件、配置、更新和系统集成，React/Vite 前端提供控制台界面，最终可打包为单个 `ga-admin`/`ga-admin.exe` 分发。它的目标不是替代 GenericAgent，而是把本机 GA 的运行状态、任务入口、模型配置、团队协作和桌面辅助能力收拢到一个可维护的控制面。

## 面向普通用户的阅读路径

- Ordinary-user non-chat quickstart and acceptance matrix: `docs/USER_QUICKSTART.md`.

如果你只想安装或使用管理面板，建议先读：**能做什么**、**快速开始**、**Linux / 无桌面环境启动**、**配置与私密文件**、**常用验证**、**故障排查**。Goal/Hive、TMWebDriver、发布流程和本地打包属于管理员/维护者能力；它们不影响日常启动、聊天、文件浏览和模型配置。

## 能做什么

### 日常用户能力

- **一键管理 GA 服务**：发现并启动 `reflect/*.py`、`frontends/*app.py`、任务型服务与 Streamlit 前端，查看 stdout/stderr，支持停止单个服务或停止全部服务。
- **原生 Chat 面板**：复用 GenericAgent 的模型配置与 `cmd/chat_worker.py`，支持流式回复、会话导航、文件/图片上传、复制与多轮对话。

### 管理员 / 维护者能力

- **Goal 模式**：通过 `reflect/goal_mode.py` 启动长期目标，设置预算、最大轮次、LLM 编号，查看状态文件、日志路径、运行时长、剩余预算和截断后的输出尾部。
- **Hive 模式**：管理端使用 GA 官方 Goal/Hive 逻辑；协作看板由 GA 根目录的 `assets/agent_bbs.py` 按需外置启动，Admin 不内置 BBS 服务或 worker 协议。
- **文件与任务编辑**：在 GA 根目录内安全浏览、搜索、tail、编辑文本文件，并为保存/删除操作生成备份。
- **模型配置向导**：首次安装无需随包提供私有 `mykey.py`；可在页面填写 API Base、Model、API Key，预览后写回生成 `mykey.py`，草稿保存在 `model_profiles.json`。
- **TMWebDriver 监控**：检查浏览器自动化环境、`18766` master 端口、Python 依赖和 `tmwd_cdp_bridge` 扩展路径，并提供依赖安装、启动/修复入口。
- **更新辅助**：支持一键配置/关闭 GitHub `insteadOf` 镜像，网络受限时可配合自更新、源码拉取和发布流程使用。
- **Windows 桌面集成**：托盘菜单、开机自启、桌面宠物、窗口显示/隐藏和常用动作触发。
- **自更新能力**：展示构建版本、提交与时间；按 GitHub Release 资产约定下载 `ga-admin-<tag>-<goos>-<goarch>.zip` 与 `.sha256` 进行更新。

- **界面体验**：支持浅色/深色主题切换（顶栏开关，偏好持久化到 `localStorage`，并跟随系统 `prefers-color-scheme`）；按路由/页面做代码分割并提供骨架屏加载反馈；侧边导航与头部补充 `aria-current`/`aria-pressed`/`role=status` 等无障碍语义。

## 界面模块

| 模块 | 说明 |
| --- | --- |
| 总览 / Control | 运行前检查、能力地图、风险摘要、最近报告和 TMWebDriver 状态。 |
| Chat | GA 原生聊天界面，支持流式输出、上传、复制、会话导航与 worker 调用。 |
| 文件 | 限定在 GA 根目录内的文本文件浏览、搜索、tail、编辑和备份保存。 |
| 任务 | 普通任务、批处理入口、任务型服务、`sche_tasks` 定时任务与服务生命周期管理。 |
| Hive 模式 | 复用 GA 官方 Goal/Hive 流程；如需多 worker 协作，请在 GA 根目录外置启动 `assets/agent_bbs.py`，Admin 不提供内置 BBS。 |
| 记忆 / 通道 / 自主进化 / 定时 | 面向 GenericAgent 现有目录与脚本的管理入口。 |
| Goal 模式 | 长目标启动、停止、日志查看、状态追踪和输出读取上限控制。 |
| 模型 | `mykey.py` 首次生成、模型草稿、API Key 保存与导出。 |
| 配置 / 日志 | GA 根目录、端口、缓冲行数、Python 路径、服务日志等基础配置。 |

## 目录约定

```text
GenericAgent-Admin-Go/
├─ main.go                         # 程序入口，启动 API、静态前端与系统托盘
├─ internal/                       # Go 后端模块：api/config/ga/service/version 等
├─ cmd/chat_worker.py              # Chat worker，发布包必须保留 cmd/ 路径
├─ web/                            # React/Vite 前端源码
│  ├─ src/
│  └─ dist/                         # Vite 构建产物，会被 Go embed
├─ build.bat                       # 本地 Windows 构建入口
├─ run.bat                         # Windows 一键构建并启动
├─ config.example.json             # 配置示例
└─ dist/                           # 本地构建输出，不提交
```

## 快速开始

### Windows 一键启动

首次拉取仓库后，安装 Node.js 22，然后双击根目录的 `run.bat`。也可以在终端执行：

```bat
cd /d C:\path\to\GenericAgent-Admin-Go
run.bat
```

`run.bat` 首次运行时会使用 `npm ci` 安装锁定的前端依赖，然后构建并嵌入前端、编译 `dist\ga-admin.exe`、复制 Chat worker 并自动启动管理端。脚本会记录 `web/package-lock.json` 的 SHA-256；后续运行只有在 `node_modules` 缺失或锁文件变化时才重新执行 `npm ci`。Go 未加入 `PATH` 且常见安装位置也不存在时，脚本会从 Go 官方站点下载与当前 Windows 架构匹配的稳定版，校验 SHA-256 后解压到仓库的 `.tools\go`，无需管理员权限或修改系统 `PATH`。如果用户没有设置 `GOPROXY`，且 Go 仍使用国内网络经常无法访问的默认模块代理，构建过程会仅在当前脚本内改用 `https://goproxy.cn`；已有的环境变量或 `go env` 自定义配置不会被覆盖。首次构建需要网络访问 npm、Go 官方站点和 Go 依赖源；失败时窗口会保留错误信息。

程序启动后默认打开 `http://127.0.0.1:8787`。首次使用在页面中选择已有 GenericAgent 根目录即可，不需要预先创建 `config.local.json`。

### 1. 准备 GenericAgent

管理端默认控制本机已有 GenericAgent 目录。第一次启动后可以在页面配置 GA 根目录；也可以先创建 `config.local.json`：

```json
{
  "ga_root": "C:/path/to/GenericAgent",
  "host": "127.0.0.1",
  "port": 8787,
  "log_tail_lines": 200,
  "buffer_lines": 1000,
  "python_path": "",
  "service_autostart": []
}
```

`python_path` 留空时会自动查找 GA 根目录下的虚拟环境解释器（Windows: `.venv/Scripts/python.exe`、`venv/Scripts/python.exe`；Linux/macOS: `.venv/bin/python`、`venv/bin/python`）。未找到虚拟环境时，Windows 回退到 `python`，Linux/macOS 回退到 `python3`；如果 GA 有固定虚拟环境，建议填写该解释器路径。

### 2. 安装前端依赖

```bat
cd /d C:\path\to\GenericAgent-Admin-Go
npm.cmd --prefix web install
```

### 3. 开发运行

```bat
npm.cmd --prefix web run build
go run .
```

打开：

```text
http://127.0.0.1:8787
```

前端开发时也可以在 `web/` 下运行 Vite dev server；正式 Go 程序使用 `web/dist` 的 embed 产物。

## Linux / 无桌面环境启动

GA Admin 支持 Linux server-only/headless 运行。Linux 下如果没有检测到 `DISPLAY`、`WAYLAND_DISPLAY` 或 `MIR_SOCKET`，程序会自动进入 headless 模式：只启动 Go HTTP 服务，不打开浏览器、不启动系统托盘。

也可以显式指定：

```bash
./ga-admin --headless
# 或
GA_ADMIN_HEADLESS=1 ./ga-admin
```

如果只是有桌面但不想自动打开浏览器：

```bash
./ga-admin --no-browser
# 或
GA_ADMIN_NO_BROWSER=1 ./ga-admin
```

无桌面服务器需要远程访问时，请把 `config.local.json` 中的 `host` 设为可信网络可访问的地址，例如 `0.0.0.0`，并通过浏览器访问 `http://<server-ip>:<port>`。管理端适合本机或可信局域网使用；公网暴露前应额外配置认证、TLS 和访问控制。

## 本地构建

Windows 推荐直接运行：

```bat
cd /d C:\path\to\GenericAgent-Admin-Go
build.bat
```

产物：

```text
dist\ga-admin.exe
dist\cmd\chat_worker.py
```

`build.bat` 会执行前端构建，然后用 `go build` 生成 exe，并通过 `-ldflags -X` 写入版本元数据：

- `internal/version.Version`：来自最近的正式语义版本 tag（`git describe --tags --abbrev=0 --match=v[0-9]*`），失败时为 `dev`；提交号和 UTC 构建时间单独展示，不再把 `-8-g<commit>-dirty` 等开发信息混入正式版本号。
- `internal/version.Commit`：来自 `git rev-parse --short HEAD`，失败时为 `unknown`。
- `internal/version.Date`：UTC 构建时间，如 `2026-06-01T12:00:00Z`。

### v1.0.0 初版

- `v1.0.0` 是 `18181617731/GenericAgent-Admin` 的首个正式发布版本；仓库中更早的 tags 来自原项目历史，不代表本仓库已发布过对应 Release。
- 后端默认版本仍为 `dev`；本地 `build.bat` 通过 Go `-ldflags` 写入 Git tag、commit 和 UTC 构建时间，Release workflow 使用触发构建的 tag 作为版本号。
- 总览页从后端版本接口读取并展示版本信息和当前更新源，不在前端代码中硬编码版本号。

## 发布包约定

总览页的版本管理默认从 `18181617731/GenericAgent-Admin` 的 GitHub Releases 检查和下载更新。系统会读取 Release 列表并选择最高的正式语义版本，不依赖 GitHub `/releases/latest` 的发布时间排序，因此历史 tag 后补发布不会覆盖真正的新版本。`config.local.json` 中的 `update_repo_url` 可覆盖默认值；可填写 GitHub 仓库地址，也可填写返回单个 Release 或 Release 数组的完整 API URL。“检查更新”需要目标仓库至少有一个有效语义版本 Release；“一键升级”还要求该 Release 包含当前平台的 ZIP 和 `.sha256` 资产。

自更新模块会在 Release 中查找与当前平台匹配的资产：

```text
ga-admin-<tag>-<goos>-<goarch>.zip
ga-admin-<tag>-<goos>-<goarch>.zip.sha256
```

zip 内必须包含：

```text
ga-admin.exe          # Windows
ga-admin              # macOS/Linux
cmd/chat_worker.py    # Chat worker 固定相对路径
README.txt            # 简短运行说明
```

仓库的 `.github/workflows/release-assets.yml` 会在推送 `v*` tag 后构建 Windows/macOS/Linux 资产并上传到 GitHub Release。也可以从 Actions 手动输入尚不存在的新版本（例如 `v1.0.1`）：工作流会从触发时选定分支的提交构建全部平台，所有构建成功后再由单一发布任务创建 tag 和 Release；如果输入的 tag 已存在，则严格从该 tag 对应提交重建，避免资产与源码错位。

## 配置与私密文件

官方源码和发布包不携带私有密钥文件。首次安装时，如果 GA 根目录没有 `mykey.py`，管理端仍可启动；进入“模型”页后可以：

1. 填写 API Base / Model / API Key。
2. 预览将生成的 Python 配置。
3. 显式写回 GA 根目录的 `mykey.py`。

本仓库忽略以下本机文件：

```text
config.local.json
model_profiles.json
dist/
*.exe
web/node_modules/
```

请不要把真实 API Key、Token 或本机绝对私密路径提交到仓库。

## Goal 模式细节

Goal 模式由后端调用：

```text
python agentmain.py --reflect reflect/goal_mode.py
```

启动时会注入：

- `GOAL_STATE=temp/goal_admin_<id>.json`
- stdout/stderr 追加到 `temp/goal_admin_<id>.log`

输出读取接口会返回 `truncated`、`bytes_returned`、`total_bytes`、`requested_bytes`、`max_bytes`、`default_bytes_used`、`max_bytes_capped` 等字段，用于区分默认读取、用户指定读取、截断和 1MiB 安全上限。

## Hive / 外置协作

GA Admin 不内置 BBS 服务，也不暴露 `/api/bbs/*` 或根路径 worker 兼容接口。多 worker 协作请使用 GA 官方 Hive 流程：在 GA 根目录按需启动 `assets/agent_bbs.py --cwd temp/hive_<目标短名> --port <PORT> --key <BOARD_KEY>`，通过 `/readme?key=<BOARD_KEY>` 获取协议，并用 `agentmain.py --reflect reflect/agent_team_worker.py --base_url http://127.0.0.1:<PORT> --board_key <BOARD_KEY>` 启动 worker。

## TMWebDriver 监控

首页会调用 `/api/tmwebdriver/status` 检查：

- 浏览器自动化 master 端口 `18766` 是否监听。
- `requests`、`bottle`、`simple-websocket-server` 等 Python 依赖是否可 import，并展示可复制的安装命令。
- `tmwd_cdp_bridge` 扩展路径是否存在。
- 当前环境缺口和建议修复动作。

当依赖缺失时，可通过页面“安装依赖”按钮或 `/api/tmwebdriver/install-deps` 使用清华 PyPI 镜像安装 TMWebDriver 运行依赖；当 master 端口未监听时，可通过页面按钮或 `/api/tmwebdriver/repair` 启动 master。上述写入/执行类接口均需要危险确认，修复接口只负责启动 master，不会停止浏览器进程。

## GitHub 镜像 / 更新辅助

总览页提供 GitHub 镜像开关，可通过 `/api/ga/git-mirror` 写入或移除全局 git `url.<mirror>.insteadOf https://github.com/` 配置。默认镜像前缀为 `https://gh-proxy.com/https://github.com/`，用于网络受限时辅助源码同步、依赖拉取和 Release 更新。


## 常用验证

提交前建议至少执行：

```bat
npm.cmd --prefix web test -- --run
npm.cmd --prefix web run build
gofmt -w main.go tray.go tray_linux.go internal
go test ./...
go build ./...
git diff --check
```

说明：

- `npm.cmd --prefix web run build` 会刷新 `web/dist`，Go 后端通过 `embed` 打包该目录。
- `go test ./...` 覆盖 API、配置、进程管理、Goal、文件安全、版本更新等后端测试。
- `git diff --check` 用于拦截尾随空格和补丁格式问题。

## 发布流程

1. 确认工作区只包含本次要发布的改动。
2. 执行前端测试、前端构建、Go 测试、Go 构建和 `git diff --check`。
3. 提交代码。
4. 打新 tag，例如 `v1.0.0`。
5. 推送 `main` 和 tag。
6. GitHub Actions 根据 tag 构建并上传 Release assets。
7. 在管理端“版本/更新”能力中验证新版本可发现、可下载且 sha256 校验通过。

### v1.0.0 发布门禁

`v1.0.0` 发布前需要确认：

- `git diff --check`、`go test -count=1 ./internal/api`、`go test ./...`、`go build ./...`、`npm.cmd --prefix web test`、`npm.cmd --prefix web run build` 的最终门禁证据。
- `build.bat` 或等效 workflow 输出不包含 `config.local.json`、`model_profiles.json`、`mykey.py`、`.env`、`*.key` 等本地/私密文件。
- 发布提交只包含应用代码、构建脚本、文档、测试和必要资源，不包含本地验证产物、临时备份或排查文件。
- 发布目标分支为 `main`，目标 tag 为 `v1.0.0`。

## 故障排查

- 页面打开但功能为空：先检查 `config.local.json` 的 `ga_root` 是否指向真实 GenericAgent 根目录。
- Chat 无响应：确认 `cmd/chat_worker.py` 在发布包中存在，且 GA Python 环境可 import 所需依赖。
- Goal 无日志：检查 `temp/goal_admin_<id>.log` 路径、Python 解释器和 `reflect/goal_mode.py` 是否存在。
- 自更新找不到资产：确认 Release 资产名严格匹配 `ga-admin-<tag>-<goos>-<goarch>.zip`，并带有同名 `.sha256`。
- TMWebDriver 不就绪：先使用页面“安装依赖”补齐 `requests`、`bottle`、`simple-websocket-server`，再使用“修复/启动”按钮检查 `18766` 端口和扩展目录。

## 安全边界

- 写入、删除、停止进程、保存模型密钥等危险操作要求前端显式确认，后端接口使用 `X-GA-Confirm: dangerous` 保护。
- 文件操作限制在配置的 GA 根目录内，避免越权访问本机其他目录。
- `mykey.py`、真实 API Key、Token、Cookie 等私密文件不应提交、不应随发布包分发。
- 管理端适合在本机或可信局域网使用；如需公网暴露，应额外增加认证、TLS 和访问控制。

## Local packaging checklist

Before creating a local Windows package or a release asset:

- Run the normal validation gates first; `build.bat` only installs/builds the web app and compiles `dist\ga-admin.exe`, it is not a substitute for tests.
- Keep `cmd\chat_worker.py` beside the executable in packaged builds; the batch script copies it to `dist\cmd\chat_worker.py`.
- Confirm local-only secrets such as `config.local.json`, `mykey.py`, `.env`, and `*.key` files are not present in the working tree, `dist`, or release assets.
- After an approved publication, verify the update flow from the Admin UI so the asset name and sha256 sidecar are both discoverable.

## License

本项目随 GenericAgent 生态内部使用；如需对外分发，请先确认上游 GenericAgent 与依赖项目的许可要求。
