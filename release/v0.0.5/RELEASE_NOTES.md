# v0.0.5

## 更新内容

- 托盘：替换为新的自定义托盘图标，并提供 Windows 多尺寸 ICO 资源。
- 托盘：新增“打开 Chat”菜单，一键进入 `/chat` 对话界面。
- Chat：使用会话级持久 worker，尽量保留 GenericAgent 上下文。
- 打包：Windows amd64 发布包，内置最新 React/Vite 前端构建产物。

## 校验

- `cmd /c build.bat`
- `go test ./...`
