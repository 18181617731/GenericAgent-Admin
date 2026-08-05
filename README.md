<div align="center">

# GenericAgent Admin Go

**Desktop Management Panel for GenericAgent**

*Go backend + React frontend · Single executable · Cross-platform*

<p>
  <a href="https://github.com/Fwind43/GenericAgent-Admin"><img src="https://img.shields.io/badge/Repository-GenericAgent--Admin-181717?style=flat-square&logo=github" alt="Repository"/></a>
  <a href="https://github.com/Fwind43/GenericAgent-Admin/releases/latest"><img src="https://img.shields.io/badge/Download-Latest_Release-00A67E?style=flat-square" alt="Latest Release"/></a>
  <a href="https://github.com/Lsdefine/GenericAgent"><img src="https://img.shields.io/badge/Upstream-GenericAgent-EA4335?style=flat-square" alt="GenericAgent"/></a>
</p>

**[English](#-english) · [中文](#-中文)**

</div>

> 📌 **Requires:** This admin panel manages local **[GenericAgent](https://github.com/Lsdefine/GenericAgent)** instances. Install GenericAgent first.

---

<a id="-english"></a>

## 🌟 Overview

**GenericAgent Admin Go** is a desktop management panel for GenericAgent. A Go backend handles processes, files, configuration, updates, and system integration, while a React/Vite frontend provides a control console. It packages into a single `ga-admin` / `ga-admin.exe` executable.

The goal is not to replace GenericAgent, but to consolidate local GA runtime state, task entry points, model configuration, team collaboration, and desktop assistance into a maintainable UI.

### 📑 Table of Contents

- [Key Features](#-key-features)
- [Quick Start](#-quick-start)
- [Core Capabilities](#-core-capabilities)
- [Development](#-development)
- [Release](#-release)
- [Documentation](#-documentation)
- [Upstream](#-upstream)
- [License](#-license)

```text
GenericAgent-Admin-Go/
├─ main.go                         # 程序入口，启动 API、静态前端与系统托盘
├─ internal/                       # Go 后端模块：api/config/ga/service/version 等
├─ cmd/chat_worker.py              # Chat worker，发布包必须保留 cmd/ 路径
├─ cmd/frontends/worldline.py      # Admin Chat 分支/回退运行时
├─ web/                            # React/Vite 前端源码
│  ├─ src/
│  └─ dist/                         # Vite 构建产物，会被 Go embed
├─ build.bat                       # 本地 Windows 构建入口
├─ run.bat                         # Windows 一键构建并启动
├─ config.example.json             # 配置示例
└─ dist/                           # 本地构建输出，不提交
```

### For Human Users

### Windows 一键启动

首次拉取仓库后，安装 Node.js 22，然后双击根目录的 `run.bat`。也可以在终端执行：

```bat
cd /d C:\path\to\GenericAgent-Admin-Go
run.bat
```

`run.bat` 首次运行时会使用 `npm ci` 安装锁定的前端依赖，然后构建并嵌入前端、编译 `dist\ga-admin.exe`、复制 Chat worker 并自动启动管理端。脚本会记录 `web/package-lock.json` 的 SHA-256；后续运行只有在 `node_modules` 缺失或锁文件变化时才重新执行 `npm ci`。Go 未加入 `PATH` 且常见安装位置也不存在时，脚本会从 Go 官方站点下载与当前 Windows 架构匹配的稳定版，校验 SHA-256 后解压到仓库的 `.tools\go`，无需管理员权限或修改系统 `PATH`。如果用户没有设置 `GOPROXY`，且 Go 仍使用国内网络经常无法访问的默认模块代理，构建过程会仅在当前脚本内改用 `https://goproxy.cn`；已有的环境变量或 `go env` 自定义配置不会被覆盖。首次构建需要网络访问 npm、Go 官方站点和 Go 依赖源；失败时窗口会保留错误信息。

程序启动后默认打开 `http://127.0.0.1:8787`。如果当前电脑已连接 Tailscale，程序还会自动发现活动网卡上属于 `100.64.0.0/10` 的 Tailscale IPv4，并同时监听 `http://<Tailscale-IP>:8787`；不需要硬编码某台电脑的地址，也不会因此监听普通 WLAN/LAN 地址。首次使用在页面中选择已有 GenericAgent 根目录即可，不需要预先创建 `config.local.json`。

```
ga-admin-windows-amd64.zip
ga-admin-linux-amd64.tar.gz
ga-admin-darwin-amd64.tar.gz  (macOS Intel)
ga-admin-darwin-arm64.tar.gz  (macOS Apple Silicon)
```

Extract and create `config.local.json` in the same directory:

```json
{
  "ga_root": "E:/Work/GenericAgent",
  "host": "127.0.0.1",
  "port": 8787
}
```

**Windows:** Double-click `ga-admin.exe` or run `ga-admin.exe --no-browser` for headless mode.  
**Linux/macOS:** Run `./ga-admin` or `./ga-admin --no-browser`.

Open `http://127.0.0.1:8787` in your browser.

#### Method 2 — Local Development Build

```bash
cd GenericAgent-Admin-Go
npm --prefix web install
npm --prefix web run build
go run .
```

Open `http://127.0.0.1:8787`.

---

## 🎯 Core Capabilities

### For Daily Users

- **Service Management:** Start/stop worker, monitor logs, check process status
- **Chat Interface:** `/chat` entrypoint with streaming, usage tracking, model switching

### For Administrators

- **Goal Mode:** Persistent goals (JSON), BBS team board, sync UI
- **File Operations:** Browse GA root, edit skills/SOPs/configs, create/rename/delete
- **Models:** Add/test/remove model profiles, wizard UI
- **Updates:** Check GitHub Releases, download & apply platform-specific packages

### For Developers

- **Frontend:** React 18 + Vite 6, code-split routes, theme toggle, accessibility
- **Backend:** Go 1.22+, embedded web assets (`//go:embed web/dist`), subprocess lifecycle
- **Build:** Single-executable distribution, GitHub Actions CI/CD for 6 platforms
- **Test:** `npm run verify` (lint + test:lib + build), `go test ./...`

---

## 🛠️ Development

### CLI Flags

- `--headless` / `--server-only` / `--no-browser`: Run without opening browser
- `--app-root <path>`: Override GA root directory (default: from `config.local.json`)
- `--port <port>`: Override HTTP port (default: 8787)

### Environment Variables

- Authentication is disabled by default, so localhost, LAN, and Tailscale access do not require a login.
- `GA_ADMIN_AUTH_ENABLED=1`: Enable HTTP Basic Auth for non-localhost access.
- `GA_ADMIN_AUTH_USER` / `GA_ADMIN_AUTH_PASSWORD`: Optional environment-managed credentials; providing both also enables authentication.

### Configuration

Place `config.local.json` in the executable directory:

```json
{
  "ga_root": "/path/to/GenericAgent",
  "host": "127.0.0.1",
  "port": 8787,
  "service_autostart": ["worker"],
  "slash_commands": [
    {"cmd": "/plan", "desc": "Call plan_worker.py for multi-step planning"}
  ]
}
```

同一 tailnet 中的其他设备可使用启动日志显示的 Tailscale URL 访问，例如 `http://100.x.y.z:8787`。如果本机或 tailnet ACL 阻止入站连接，需要为该端口放行 Tailscale 私有网络流量；不要将管理端口直接暴露到公网。

前端开发时也可以在 `web/` 下运行 Vite dev server；正式 Go 程序使用 `web/dist` 的 embed 产物。

The repository ignores:

```

如果只是有桌面但不想自动打开浏览器：

```bash
./ga-admin --no-browser
# 或
GA_ADMIN_NO_BROWSER=1 ./ga-admin
```

无桌面服务器需要远程访问时，请把 `config.local.json` 中的 `host` 设为可信网络可访问的地址，例如 `0.0.0.0`。默认不开启 HTTP Basic Auth，因此本机、局域网和 Tailscale 地址都可以直接打开管理页面，不需要输入账号密码。已有的 `auth.local.json` 只有在显式启用认证后才会读取。

设置后的凭据以加盐 PBKDF2 哈希保存到应用数据目录的 `auth.local.json`，不会保存明文密码。不要把这个本地状态文件提交到版本库；备份或迁移应用数据时应将它视为敏感文件。设置或改密会立即使旧凭据失效；从其他设备通过 HTTP Basic Auth 访问时，需要使用当前密码重新认证。

如果需要重新启用认证，可以设置 `GA_ADMIN_AUTH_ENABLED=1`；如果希望由部署环境托管凭据，也可以同时设置以下两个变量，提供凭据会自动启用认证。两个凭据变量必须成对提供；只设置其中一个时程序会拒绝启动。环境托管模式不会写入本地密码文件，也不显示首次改密页面：

```bash
GA_ADMIN_AUTH_USER=admin \
GA_ADMIN_AUTH_PASSWORD='replace-with-a-long-random-password' \
./ga-admin --headless
```

PowerShell 示例：

```powershell
$env:GA_ADMIN_AUTH_USER = 'admin'
$env:GA_ADMIN_AUTH_PASSWORD = 'replace-with-a-long-random-password'
.\ga-admin.exe --headless
```

启用认证后，所有来源地址不是 IPv4 `127.0.0.0/8` 的请求都会被整站 HTTP Basic Auth 保护，覆盖页面、静态资源和全部 `/api/*` 路由。本机回环访问不要求 Basic Auth。程序只按实际 TCP 来源地址判断是否为 `127.*`，不会信任客户端提供的 `X-Forwarded-For`。Basic Auth 本身不加密凭据；跨不可信网络访问时，必须在 GA Admin 前配置 HTTPS/TLS 反向代理，并限制防火墙访问来源。反向代理连接 GA Admin 时也必须携带有效的 Basic Auth 凭据。

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
dist\cmd\frontends\worldline.py
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

总览页的版本管理默认从 `18181617731/GenericAgent-Admin` 的 GitHub Releases 检查和下载更新。系统会读取 Release 列表并选择最高的正式语义版本，不依赖 GitHub `/releases/latest` 的发布时间排序，因此历史 tag 后补发布不会覆盖真正的新版本。`config.local.json` 中的 `update_repo_url` 可覆盖默认值；可填写 GitHub 仓库地址，也可填写返回单个 Release 或 Release 数组的完整 API URL。“检查更新”需要目标仓库至少有一个有效语义版本 Release；“一键升级”还要求该 Release 包含当前平台的 ZIP，以及 GitHub Release API 提供的 SHA256 digest 或配套 `.sha256` 资产。

当 GitHub Release 直连下载失败时，更新器会自动尝试 `gh-proxy.com` 和 `ghfast.top`，但仍使用 GitHub Release API 返回的官方 SHA256 digest 校验文件，镜像内容无法绕过完整性检查。可用 `GA_ADMIN_UPDATE_MIRRORS`（逗号或分号分隔）在默认镜像前添加自定义镜像前缀；设置 `GA_ADMIN_UPDATE_DISABLE_MIRRORS=true` 可完全关闭镜像回退。没有官方 digest 的旧 Release 不会通过第三方镜像下载。

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
cmd/frontends/worldline.py # Admin Chat 世界线运行时
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
*.local.json
model_profiles.json
dist/
*.exe
web/node_modules/
/temp/
/release/
*.pid
*.log
```

### Validation Before Commit

Run at least:

```bash
npm --prefix web run verify    # lint + test:lib + build
go test ./...
go build ./...
git diff --check
```

**Notes:**
- `npm run verify` runs `lint + test:lib + build` (skips `test:ui`)
- `web/src/lib/*.test.mjs` are auto-discovered by `npm run test:lib`
- Test files do not need `package.json` registration

---

## 📦 Release

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

```
ga-admin-windows-amd64.zip
ga-admin-windows-arm64.zip
ga-admin-linux-amd64.tar.gz
ga-admin-linux-arm64.tar.gz
ga-admin-darwin-amd64.tar.gz
ga-admin-darwin-arm64.tar.gz
```

- 页面打开但功能为空：先检查 `config.local.json` 的 `ga_root` 是否指向真实 GenericAgent 根目录。
- Chat 无响应：确认 `cmd/chat_worker.py` 与 `cmd/frontends/worldline.py` 在发布包中存在，且 GA Python 环境可 import 所需依赖。
- Goal 无日志：检查 `temp/goal_admin_<id>.log` 路径、Python 解释器和 `reflect/goal_mode.py` 是否存在。
- 自更新找不到资产：确认 Release 资产名严格匹配 `ga-admin-<tag>-<goos>-<goarch>.zip`，并带有同名 `.sha256`。
- TMWebDriver 不就绪：先使用页面“安装依赖”补齐 `requests`、`bottle`、`simple-websocket-server`，再使用“修复/启动”按钮检查 `18766` 端口和扩展目录。

---

## 📄 Documentation

- User-focused quickstart: [`docs/USER_QUICKSTART.md`](docs/USER_QUICKSTART.md) (Chinese)
- Knowledge base: [`docs/knowledge_base.md`](docs/knowledge_base.md)
- Developer experience: [`docs/secondary_dev_experience.md`](docs/secondary_dev_experience.md)

---

- Run the normal validation gates first; `build.bat` only installs/builds the web app and compiles `dist\ga-admin.exe`, it is not a substitute for tests.
- Keep `cmd\chat_worker.py` and `cmd\frontends\worldline.py` beside the executable in packaged builds; the worker also embeds a compressed compatibility copy for upgrades from legacy installers.
- Confirm local-only secrets such as `config.local.json`, `mykey.py`, `.env`, and `*.key` files are not present in the working tree, `dist`, or release assets.
- After an approved publication, verify the update flow from the Admin UI so the asset name and sha256 sidecar are both discoverable.

This project manages local **[GenericAgent](https://github.com/Lsdefine/GenericAgent)** instances. GA Admin requires a GenericAgent installation to function.

---

## 📄 License

This project is used internally within the GenericAgent ecosystem. For external distribution, confirm upstream GenericAgent and dependency project license requirements first.

---

<a id="-中文"></a>

## 🌟 项目简介

**GenericAgent Admin Go** 是 GenericAgent 的桌面管理面板。Go 后端负责进程、文件、配置、更新和系统集成，React/Vite 前端提供控制台界面。打包为单个 `ga-admin` / `ga-admin.exe` 可执行文件。

目标不是替代 GenericAgent，而是将本地 GA 运行状态、任务入口、模型配置、团队协作和桌面辅助整合到一个可维护的 UI 中。

### 📑 目录

- [核心特性](#-核心特性)
- [快速开始](#-快速开始)
- [核心功能](#-核心功能)
- [开发](#-开发)
- [发布](#-发布)
- [文档](#-文档)
- [上游项目](#-上游项目)
- [许可](#-许可)

---

## 📋 核心特性

| 特性 | 说明 |
| :--- | :--- |
| 📊 **仪表盘** | 服务状态概览、最近活动、系统统计 |
| 💬 **原生聊天** | `/chat` 入口的聊天界面，流式响应，用量跟踪，模型切换 |
| 📝 **文件编辑器** | 浏览 GA 根目录，编辑技能/SOP/配置，语法高亮，文件树 |
| 📋 **任务管理** | 服务（启动/停止 worker）、计划任务、Goal 运行、自主报告 |
| 🧠 **记忆浏览器** | 查看/搜索全局和项目记忆 |
| 📡 **通道监控** | 活动通道、消息日志 |
| 🤖 **自主模式** | 后台任务执行 |
| 📈 **用量跟踪** | 每个模型的 Token/成本统计 |
| 🎯 **Goal 模式** | 启动/停止长时间运行的目标，查看日志/状态 |
| ⚙️ **模型配置** | 添加模型的向导，测试端点，管理配置文件 |
| 🔧 **设置** | 应用配置、认证、ga_root 路径 |
| 📄 **日志查看器** | 尾随 worker 日志，搜索历史 |

---

## 📈 Star 历史

[![Star History Chart](https://api.star-history.com/svg?repos=Fwind43/GenericAgent-Admin&type=Date)](https://star-history.com/#Fwind43/GenericAgent-Admin&Date)

---

## 🚀 快速开始

> ⚠️ **前置要求：** Python 3.11+（用于 GenericAgent），Node.js 18+ / Go 1.22+（用于开发构建）

### 给 LLM Agent 看的

获取安装指南并照做：

```bash
curl -fsSL https://raw.githubusercontent.com/Fwind43/GenericAgent-Admin/main/README.md
```

### 给人类用户看的

#### 方法一 — 下载发布包 *（推荐）*

从 [GitHub Releases](https://github.com/Fwind43/GenericAgent-Admin/releases/latest) 下载平台特定包：

```
ga-admin-windows-amd64.zip
ga-admin-linux-amd64.tar.gz
ga-admin-darwin-amd64.tar.gz  (macOS Intel)
ga-admin-darwin-arm64.tar.gz  (macOS Apple Silicon)
```

解压后在同目录创建 `config.local.json`：

```json
{
  "ga_root": "E:/Work/GenericAgent",
  "host": "127.0.0.1",
  "port": 8787
}
```

**Windows：** 双击 `ga-admin.exe` 或运行 `ga-admin.exe --no-browser`（无头模式）。  
**Linux/macOS：** 运行 `./ga-admin` 或 `./ga-admin --no-browser`。

浏览器打开 `http://127.0.0.1:8787`。

#### 方法二 — 本地开发构建

```bash
cd GenericAgent-Admin-Go
npm --prefix web install
npm --prefix web run build
go run .
```

浏览器打开 `http://127.0.0.1:8787`。

---

## 🎯 核心功能

### 面向日常用户

- **服务管理：** 启动/停止 worker，监控日志，检查进程状态
- **聊天界面：** `/chat` 入口，流式响应，用量跟踪，模型切换

### 面向管理员

- **Goal 模式：** 持久化目标（JSON），BBS 团队看板，同步 UI
- **文件操作：** 浏览 GA 根目录，编辑技能/SOP/配置，创建/重命名/删除
- **模型管理：** 添加/测试/移除模型配置文件，向导 UI
- **更新：** 检查 GitHub Releases，下载并应用平台特定包

### 面向开发者

- **前端：** React 18 + Vite 6，路由代码分割，主题切换，无障碍
- **后端：** Go 1.22+，嵌入 web 资源（`//go:embed web/dist`），子进程生命周期
- **构建：** 单可执行文件分发，GitHub Actions CI/CD 支持 6 平台
- **测试：** `npm run verify`（lint + test:lib + build），`go test ./...`

---

## 🛠️ 开发

### CLI 参数

- `--headless` / `--server-only` / `--no-browser`：无浏览器模式运行
- `--app-root <路径>`：覆盖 GA 根目录（默认从 `config.local.json` 读取）
- `--port <端口>`：覆盖 HTTP 端口（默认 8787）

### 环境变量

- 默认关闭认证，本机、局域网和 Tailscale 访问无需登录。
- `GA_ADMIN_AUTH_ENABLED=1`：启用非 localhost 访问的 HTTP Basic Auth。
- `GA_ADMIN_AUTH_USER` / `GA_ADMIN_AUTH_PASSWORD`：可选的环境托管凭据；同时提供两个变量也会自动启用认证。

### 配置

在可执行文件目录放置 `config.local.json`：

```json
{
  "ga_root": "/path/to/GenericAgent",
  "host": "127.0.0.1",
  "port": 8787,
  "service_autostart": ["worker"],
  "slash_commands": [
    {"cmd": "/plan", "desc": "调用 plan_worker.py 进行多步规划"}
  ]
}
```

参见 `config.example.json` 获取所有可用选项。

仓库忽略：

```
config.local.json
*.local.json
model_profiles.json
dist/
*.exe
web/node_modules/
/temp/
/release/
*.pid
*.log
```

### 提交前验证

至少运行：

```bash
npm --prefix web run verify    # lint + test:lib + build
go test ./...
go build ./...
git diff --check
```

**注意：**
- `npm run verify` 运行 `lint + test:lib + build`（跳过 `test:ui`）
- `web/src/lib/*.test.mjs` 由 `npm run test:lib` 自动发现
- 测试文件无需 `package.json` 注册

---

## 📦 发布

1. **验证清洁状态：** 无未提交更改，所有测试通过
2. **运行验证：** `npm --prefix web run verify && go test ./...`
3. **提交并打标签：** `git commit -am "release: v0.x.x"` → `git tag v0.x.x`
4. **推送：** `git push origin main --tags`

GitHub Actions 将构建 6 个平台包并附加到发布：

```
ga-admin-windows-amd64.zip
ga-admin-windows-arm64.zip
ga-admin-linux-amd64.tar.gz
ga-admin-linux-arm64.tar.gz
ga-admin-darwin-amd64.tar.gz
ga-admin-darwin-arm64.tar.gz
```

每个包包含：
- 平台特定可执行文件（`ga-admin` / `ga-admin.exe`）
- `config.example.json` 模板
- 版本元数据（构建时通过 `-ldflags` 注入）

---

## 📄 文档

- 用户快速开始：[`docs/USER_QUICKSTART.md`](docs/USER_QUICKSTART.md)（中文）
- 知识库：[`docs/knowledge_base.md`](docs/knowledge_base.md)
- 二次开发体验：[`docs/secondary_dev_experience.md`](docs/secondary_dev_experience.md)

---

## 🔗 上游项目

本项目管理本地 **[GenericAgent](https://github.com/Lsdefine/GenericAgent)** 实例。GA Admin 需要 GenericAgent 安装才能运行。

---

## 📄 许可

本项目在 GenericAgent 生态系统内部使用。如需外部分发，请先确认上游 GenericAgent 及依赖项目的许可要求。
