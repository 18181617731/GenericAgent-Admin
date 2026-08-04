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

---

## 📋 Key Features

| Feature | Description |
| :--- | :--- |
| 📊 **Dashboard** | Overview of service status, recent activity, system stats |
| 💬 **Native Chat** | Chat interface with `/chat` entrypoint, streaming response, usage tracking, model switching |
| 📝 **File Editor** | Browse GA root, edit skills/SOPs/configs, syntax highlighting, file tree |
| 📋 **Task Management** | Services (start/stop worker), scheduled tasks, Goal runs, autonomous reports |
| 🧠 **Memory Browser** | View/search global and project memory |
| 📡 **Channel Monitor** | Active channels, message logs |
| 🤖 **Autonomous Mode** | Background task execution |
| 📈 **Usage Tracking** | Token/cost statistics per model |
| 🎯 **Goal Mode** | Start/stop long-running goals, view logs/status |
| ⚙️ **Model Config** | Wizard to add models, test endpoints, manage profiles |
| 🔧 **Settings** | App config, auth, ga_root path |
| 📄 **Log Viewer** | Tail worker logs, search history |

---

## 📈 Star History

[![Star History Chart](https://api.star-history.com/svg?repos=Fwind43/GenericAgent-Admin&type=Date)](https://star-history.com/#Fwind43/GenericAgent-Admin&Date)

---

## 🚀 Quick Start

> ⚠️ **Prerequisites:** Python 3.11+ (for GenericAgent), Node.js 18+ / Go 1.22+ (for development builds)

### For LLM Agents

Fetch installation guide and follow:

```bash
curl -fsSL https://raw.githubusercontent.com/Fwind43/GenericAgent-Admin/main/README.md
```

### For Human Users

#### Method 1 — Download Release Package *(Recommended)*

Download the platform-specific package from [GitHub Releases](https://github.com/Fwind43/GenericAgent-Admin/releases/latest):

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

- `GA_ADMIN_AUTH_USER` / `GA_ADMIN_AUTH_PASSWORD`: HTTP Basic Auth for non-localhost access

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

See `config.example.json` for all available options.

The repository ignores:

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

1. **Verify clean state:** No uncommitted changes, all tests pass
2. **Run validation:** `npm --prefix web run verify && go test ./...`
3. **Commit & tag:** `git commit -am "release: v0.x.x"` → `git tag v0.x.x`
4. **Push:** `git push origin main --tags`

GitHub Actions will build 6 platform packages and attach to the release:

```
ga-admin-windows-amd64.zip
ga-admin-windows-arm64.zip
ga-admin-linux-amd64.tar.gz
ga-admin-linux-arm64.tar.gz
ga-admin-darwin-amd64.tar.gz
ga-admin-darwin-arm64.tar.gz
```

Each package includes:
- Platform-specific executable (`ga-admin` / `ga-admin.exe`)
- `config.example.json` template
- Version metadata (injected via `-ldflags` during build)

---

## 📄 Documentation

- User-focused quickstart: [`docs/USER_QUICKSTART.md`](docs/USER_QUICKSTART.md) (Chinese)
- Knowledge base: [`docs/knowledge_base.md`](docs/knowledge_base.md)
- Developer experience: [`docs/secondary_dev_experience.md`](docs/secondary_dev_experience.md)

---

## 🔗 Upstream

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

- `GA_ADMIN_AUTH_USER` / `GA_ADMIN_AUTH_PASSWORD`：非 localhost 访问的 HTTP Basic Auth

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
