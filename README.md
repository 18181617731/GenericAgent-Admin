# GenericAgent Admin Go

Go 后端 + Vite React 前端的 GenericAgent 管理端，目标是方便打包成单个 `ga-admin.exe` 分发。

## 功能

- Web 页面管理 GA 服务。
- 自动发现 `reflect/*.py`，按 `python agentmain.py --reflect reflect/<file>.py` 启动。
- Goal 模式：通过 `reflect/goal_mode.py` 启动带预算的持续目标，支持列表、查看输出、可配置输出读取字节数、运行时长/剩余预算/状态文件/日志路径展示、日志缺失容错和精确 PID 停止；启动时会把 `GOAL_STATE` 指向 `temp/goal_admin_<id>.json`，并将 stdout/stderr 追加到 `temp/goal_admin_<id>.log`；输出尾部接口会返回 `truncated`、`bytes_returned`、`total_bytes`、`requested_bytes`、`max_bytes`、`default_bytes_used`、`max_bytes_capped`，用于区分默认读取、截断和 1MiB 安全上限。
- TMWebDriver 监控：检测浏览器进程、`18766` master 端口和 `tmwd_cdp_bridge` 扩展路径，方便确认 GA 浏览器自动化环境是否就绪。
- 自动发现 `frontends/*app.py`（文件名 stem 必须以 `app` 结尾），其中 `stapp.py` 使用 `python -m streamlit run`。
- 查看服务 stdout/stderr 日志。
- 页面配置 GA 根目录。
- 页面配置模型，保存独立草稿 `model_profiles.json`。
- 导出 `mykey_admin.generated.py`，也可显式写回生成首次安装所需的 `mykey.py`。
- 安全策略：`mykey.py` 是私有可选文件，官方源码不随包提供；没有它时管理端仍可启动，并在模型页用默认草稿引导创建。
- Vite 构建产物通过 Go `embed` 打进 exe。

## 构建

```bat
cd /d C:\Users\Fwind43\Desktop\code\GenericAgent-Admin-Go
build.bat
```

产物：

```text
dist\ga-admin.exe
```

## 开发运行

```bat
npm.cmd --prefix web install
npm.cmd --prefix web run build
go run .
```

打开：

```text
http://127.0.0.1:8787
```

## Goal Mode 控制台

Admin-Go 只负责调用和监管 GA 现有 Goal Mode 执行器，不重造执行器。通过页面启动时复用 `reflect/goal_mode.py`，并把 `GOAL_STATE` 指向 `temp/goal_admin_<id>.json`；AI/其它入口自行开启 Goal Mode 时，Admin-Go 也会扫描标准状态文件并纳入只读/软控制视图。

状态与日志发现规则：

- Admin 托管运行：`temp/goal_admin_<id>.json` + `temp/goal_admin_<id>.log`。
- 外部自启运行：`temp/goal_state.json`、`temp/goal_*.json` 等标准 Goal 状态文件。
- 外部运行通常没有 Admin 专属日志；输出接口会优先读取状态文件声明的 log 路径，缺失时回退到 `temp/model_responses` 最近输出。

控制边界：

- `origin=admin` / `managed=true`：PID 来自 Admin 启动记录，属于可信 PID；停止时仍要求 `id` + 精确 `pid` 匹配，只终止该记录对应进程，避免误杀无关 Python。
- `origin=external` / `managed=false`：状态由 AI 或其它入口自启产生，PID 不视为可信；停止只把状态文件写为 `stopped_by_admin`，不杀进程，让 Goal 循环自行观察状态后退出。
- 列表会展示来源、控制级别和 PID 可信度；前端停止前会根据控制级别显示不同二次确认文案。

主要接口：

| Method | Path | 说明 |
| --- | --- | --- |
| `POST` | `/api/goals/start` | 启动 Admin 托管 Goal Mode；支持 `objective`、`budget_seconds` 或 `budget_minutes`、`max_turns`、`llm_no`。 |
| `GET` | `/api/goals/list` | 列出 Admin 托管和外部自启 Goal，展示预算、剩余时间、状态/日志路径、PID、来源、控制级别与运行状态。 |
| `POST` | `/api/goals/stop` | Admin 托管运行执行精确 PID 停止；外部自启运行执行状态文件软停止，成功后把状态改为 `stopped_by_admin`。 |
| `GET` | `/api/goals/output?id=<id>&max_bytes=<n>` | 读取日志/输出尾部；`max_bytes=0` 使用默认值，超大值会被限制到 1MiB，并在响应元数据中标记。 |

前端输出区提供 64K / 256K / 1M / 默认 64K 快捷按钮，会根据接口元数据显示“已截断 / 使用默认值 / 后端上限裁剪”等提示；复制按钮支持 Clipboard API，也会在非安全上下文中回退到临时 `textarea` 复制。

## TMWebDriver 监控

首页会调用 `/api/tmwebdriver/status` 展示 GA 浏览器自动化运行环境的基础状态：

- 浏览器/调试端口：检查本机 `18766` master 端口是否可访问。
- 扩展：扫描常见位置中的 `tmwd_cdp_bridge` 扩展路径。
- 建议：当环境不完整时，接口会返回 `recommendation` 供页面直接展示。
- 一键修复：当 master 端口未监听时，可通过 `/api/tmwebdriver/repair` 或页面“修复/启动”按钮启动 TMWebDriver master；接口只负责启动 master，不会停止浏览器进程。

## 验证

常规改动建议至少执行：

```bat
npm.cmd --prefix web run build
go test ./...
go build ./...
git diff --check
```

其中 `npm.cmd --prefix web run build` 会刷新 `web/dist`，Go 后端通过 `embed` 打包该目录。

## 配置

### 首次安装与 `mykey.py`

官方源码不会包含私有密钥文件 `mykey.py`。GA Admin 不再把它视为启动必需项：首次安装时可以先进入页面，打开“模型”页填写 API Base / Model / API Key，预览后写回生成 `mykey.py`。

首次运行会读取默认配置，也可以创建 `config.local.json`：

```json
{
  "ga_root": "E:/Work/GenericAgent",
  "host": "127.0.0.1",
  "port": 8787,
  "log_tail_lines": 200,
  "buffer_lines": 1000,
  "python_path": ""
}
```

Go 管理端本身可以单 exe 分发；GA 服务仍依赖 GA 目录中的 Python/venv 环境。
