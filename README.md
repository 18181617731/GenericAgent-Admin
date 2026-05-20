# GenericAgent Admin Go

Go 后端 + Vite React 前端的 GenericAgent 管理端，目标是方便打包成单个 `ga-admin.exe` 分发。

## 功能

- Web 页面管理 GA 服务。
- 自动发现 `reflect/*.py`，按 `python agentmain.py --reflect reflect/<file>.py` 启动。
- Goal 模式：通过 `reflect/goal_mode.py` 启动带预算的持续目标，支持列表、查看输出、可配置输出读取字节数、运行时长/剩余预算/状态文件/日志路径展示、日志缺失容错和精确 PID 停止；启动时会把 `GOAL_STATE` 指向 `temp/goal_admin_<id>.json`，并将 stdout/stderr 追加到 `temp/goal_admin_<id>.log`；输出尾部接口会返回 `truncated`、`bytes_returned`、`total_bytes`、`requested_bytes`、`max_bytes`、`default_bytes_used`、`max_bytes_capped`，用于区分默认读取、截断和 1MiB 安全上限。
- 自动发现 `frontends/*app.py`（文件名 stem 必须以 `app` 结尾），其中 `stapp.py` 使用 `python -m streamlit run`。
- 查看服务 stdout/stderr 日志。
- 页面配置 GA 根目录。
- 页面配置模型，保存独立草稿 `model_profiles.json`。
- 导出 `mykey_admin.generated.py`。
- 安全策略：不读取、不覆盖现有 `mykey.py`。
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

Admin-Go 只负责调用 GA 现有 Goal Mode 执行器，不重造执行器：启动时复用 `reflect/goal_mode.py`，并通过 `GOAL_STATE=temp/goal_admin_<id>.json` 指定状态文件。每个运行对应：

- 状态文件：`temp/goal_admin_<id>.json`
- 日志文件：`temp/goal_admin_<id>.log`
- 停止策略：必须由前端/接口提交该运行记录中的精确 PID；后端仅在状态仍为 `running` 且 PID 完全匹配时停止，避免误杀已结束记录中的陈旧 PID 或无关 Python 进程；前端停止前会二次确认。

主要接口：

| Method | Path | 说明 |
| --- | --- | --- |
| `POST` | `/api/goals/start` | 启动 Goal Mode；支持 `objective`、`budget_seconds` 或 `budget_minutes`、`max_turns`、`llm_no`。 |
| `GET` | `/api/goals/list` | 列出 `temp/goal_admin_*.json`，展示预算、剩余时间、状态/日志路径、PID 与运行状态。 |
| `POST` | `/api/goals/stop` | 以 `id` + 精确 `pid` 停止指定运行；仅允许停止仍处于 `running` 的记录，成功后把状态改为 `stopped_by_admin`。 |
| `GET` | `/api/goals/output?id=<id>&max_bytes=<n>` | 读取日志尾部；`max_bytes=0` 使用默认值，超大值会被限制到 1MiB，并在响应元数据中标记。 |

前端输出区提供 64K / 256K / 1M / 默认 64K 快捷按钮，会根据接口元数据显示“已截断 / 使用默认值 / 后端上限裁剪”等提示；复制按钮支持 Clipboard API，也会在非安全上下文中回退到临时 `textarea` 复制。

## 配置

首次运行会读取默认配置，也可以创建 `config.local.json`：

```json
{
  "ga_root": "E:/Work/GenericAgent",
  "host": "127.0.0.1",
  "port": 8787,
  "log_tail_lines": 200,
  "buffer_lines": 1000
}
```

Go 管理端本身可以单 exe 分发；GA 服务仍依赖 GA 目录中的 Python/venv 环境。
