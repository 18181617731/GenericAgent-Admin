# ga_admin_pets_sop

目标：帮助用户为 GenericAgent Admin Go 创建自己的专属桌宠（ga-admin-pets runtime），产出可被 Web 端展示、并可嵌入 Windows 桌面宠物的 `pet.json` + `spritesheet.webp/png` + QA 文件。

## 0. 适用范围与硬边界
- 适用：用户要把角色、品牌 mascot、参考图、文字概念制作成 GA Admin 专用桌宠。
- GA Admin runtime 名称：`ga-admin-pets`。
- GA Admin 桌宠资产不依赖 Codex pet 目录；需要放进 GA Admin 仓库资产路径。
- 当前已验证代码根目录：`C:\Users\Fwind43\Desktop\code\GenericAgent-Admin-Go`。
- 不要把密钥写进 SOP、脚本或仓库；如需图像生成，按 `gpt_image_2_sop` 运行时读取配置。

## 1. 标准 atlas 规格
GA Admin 当前桌面宠物代码按固定 atlas 读取：

| 项 | 值 |
|---|---|
| cell | `192 x 208` |
| columns | `8` |
| rows | `9` |
| atlas size | `1536 x 1872` |
| Web spritesheet | `spritesheet.webp` |
| Windows embedded spritesheet | `spritesheet.png` |

9 行状态必须按以下顺序排列：

| row | state | frames | loop | 语义 |
|---:|---|---:|---|---|
| 0 | `idle` | 6 | yes | 待机、呼吸、眨眼 |
| 1 | `running-right` | 8 | yes | 向右拖动/移动 |
| 2 | `running-left` | 8 | yes | 向左拖动/移动 |
| 3 | `waving` | 4 | no | 打招呼/吸引注意 |
| 4 | `jumping` | 5 | no | 跳跃/活泼反馈 |
| 5 | `failed` | 8 | no | 失败/阻塞/取消 |
| 6 | `waiting` | 6 | yes | 等待用户输入/批准 |
| 7 | `running` | 6 | yes | 任务处理中 |
| 8 | `review` | 6 | yes | 结果待查看/复盘 |

未使用格必须透明，尤其每行 `frames` 后面的剩余列。

## 2. 仓库内落点
设宠物 ID 为 `<pet_id>`，建议只用小写字母、数字、短横线，例如 `orange-cat`。

### Web 端资产
放在：

```text
web/public/ga-admin-pets/<pet_id>/
  pet.json
  spritesheet.webp
  contact-sheet.png
  validation.json
```

并更新索引：

```text
web/public/ga-admin-pets/pets.json
```

索引项示例：

```json
{
  "id": "orange-cat",
  "name": "Orange Cat",
  "manifest": "/ga-admin-pets/orange-cat/pet.json",
  "spritesheet": "/ga-admin-pets/orange-cat/spritesheet.webp"
}
```

### Windows 桌面宠物内嵌资产
当前 Windows 桌面宠物代码已验证通过 `pet_assets_windows.go` 嵌入：

```go
//go:embed assets/ga-admin-pets/upload-pet/spritesheet.png
var gaAdminPetSpritesheetPNG []byte
```

因此桌面宠物实际读取的是：

```text
assets/ga-admin-pets/<active-pet>/spritesheet.png
```

若要把新的 `<pet_id>` 作为默认桌面宠物，需要：
1. 复制 PNG 到 `assets/ga-admin-pets/<pet_id>/spritesheet.png`。
2. 复制 manifest 到 `assets/ga-admin-pets/<pet_id>/pet.json`。
3. 修改 `pet_assets_windows.go` 的 `//go:embed` 路径指向新 PNG。
4. `gofmt`、`go test ./...`、`go build ./...` 闭环验证。

## 3. 推荐制作流程
优先复用 `hatch_pet_sop` 的确定性流程制作 atlas，因为它能产出 QA、contact sheet、preview 和 invariant 校验。

### 3.1 准备输入
向用户确认：
- 宠物名字 / `<pet_id>`。
- 参考图或文字描述。
- 风格：像素、扁平插画、3D 卡通、品牌 mascot 等。
- 需要保留的关键特征：颜色、道具、表情、服装、logo。
- 是否要作为 Windows 桌宠默认宠物，还是只放 Web 端浏览。

### 3.2 生成 / 修图
- 有 Codex `$imagegen` / hatch-pet skill 时，按 `hatch_pet_sop`：prepare → generate strips → compose atlas → validate → contact sheet → previews → package。
- 在 GA 环境没有 `$imagegen` 时，可按 `gpt_image_2_sop` 调 `gpt-image-2` 生成角色参考图或行 strip 原图，再交给 hatch-pet 脚本做确定性拼版和校验。
- 不要只手工拼一张大图就结束；必须跑透明像素、尺寸、行列、帧数校验。

### 3.3 输出格式
最终至少保留：

```text
final/spritesheet.webp
final/spritesheet.png
final/pet.json
qa/contact-sheet.png
qa/review.json 或 validation.json
qa/previews/*.gif
```

复制到 GA 仓库时：
- `final/spritesheet.webp` → `web/public/ga-admin-pets/<pet_id>/spritesheet.webp`
- `final/spritesheet.png` → `assets/ga-admin-pets/<pet_id>/spritesheet.png`
- `final/pet.json` → 两处都可复制：
  - `web/public/ga-admin-pets/<pet_id>/pet.json`
  - `assets/ga-admin-pets/<pet_id>/pet.json`
- `qa/contact-sheet.png` → `web/public/ga-admin-pets/<pet_id>/contact-sheet.png`
- 校验报告 → `web/public/ga-admin-pets/<pet_id>/validation.json`

## 4. pet.json 最小要求
`pet.json` 至少应描述：
- `pet_id`
- `display_name`
- `description`
- `atlas.columns = 8`
- `atlas.rows = 9`
- `atlas.cell_width = 192`
- `atlas.cell_height = 208`
- `atlas.width = 1536`
- `atlas.height = 1872`
- `rows[]`：9 个 state 的 row、frames、purpose
- `runtime = "ga-admin-pets"`
- `asset_base = "/ga-admin-pets/<pet_id>/"`
- `spritesheet = "spritesheet.webp"`
- `source = "embedded-ga-admin-asset"` 或实际来源说明

可参考已验证样例：

```text
C:\Users\Fwind43\Desktop\code\GenericAgent-Admin-Go\assets\ga-admin-pets\upload-pet\pet.json
C:\Users\Fwind43\Desktop\code\GenericAgent-Admin-Go\web\public\ga-admin-pets\upload-pet\pet.json
```

## 5. 视觉 QA 标准
通过前不要交付：
- 9 行状态齐全，顺序正确。
- 每行角色身份、配色、材质、道具一致。
- `running-right` 朝右，`running-left` 朝左，步态方向不能反。
- 非循环动作有明确开始和结束，不应突然缩放、跳基线。
- 透明背景干净；未使用单元格完全透明。
- 小尺寸下轮廓清楚，脸和关键道具可读。
- contact sheet 肉眼检查无错帧、串行、裁切、残影。

## 6. 验证命令
复制资产和改 Go embed 后，在 GA Admin Go 根目录执行：

```bash
gofmt -w pet_assets_windows.go desktop_pet_windows.go desktop_pet_other.go tray.go main.go
go test ./...
go build ./...
```

如果只增加 Web 端资产，也至少检查：

```bash
python -m json.tool web/public/ga-admin-pets/pets.json > NUL
python -m json.tool web/public/ga-admin-pets/<pet_id>/pet.json > NUL
```

Windows PowerShell 下可把 `> NUL` 换成 `$null` 或直接省略输出重定向。

## 7. 常见坑
- **只放 web/public，不放 assets**：Web 可见，但 Windows 桌面宠物不会变。
- **只放 webp，不放 png**：当前 Go 桌宠解码的是内嵌 PNG。
- **改了 assets 但没改 `pet_assets_windows.go`**：仍会嵌入旧宠物。
- **frame 数不匹配**：Go 代码当前按固定 `petAnimations` 读取；必须保持 6/8/8/4/5/8/6/6/6。
- **把 running-left 直接复制 running-right 但不镜像**：拖动方向会错。
- **未用 QA 校验**：透明残留、尺寸偏差、未用格非透明会导致桌宠边缘脏或动画抖动。
- **修改 memory 时忘记同步 L1**：新增本 SOP 后需在 `global_mem_insight.txt` L3 索引加极简项。

## 8. 桌宠自动动作联动（已验证）
GA Admin 桌宠不应只靠托盘菜单手动切动作；已验证可用事件桥：
- `internal/api.Server` 增加 `PetEvent func(string)` 回调与 `NotifyPetEvent(event string)`，API handler 内只发字符串事件，避免 `internal/api` 依赖 `main`/Windows 桌宠实现。
- `main.go` 注册事件映射到 `setDesktopPetAction` / `setDesktopPetActionForTicks`。
- 已验证触发点：service start/stop/stopAll/autostart、React app start/stop、goals start/stop、chat post/cancel、tmwebdriver repair 的成功/失败分支。
- 持续运行态要在 `main.go` 侧用小状态机聚合 `chat/goal/react`，不要让某个完成事件直接把仍在运行的其它域切回 idle。
- 完成/失败/等待这类一次性反馈要先恢复 base（running 或 idle），再调用 `setDesktopPetActionForTicks(...)`；`setDesktopPetAction(...)` 会清空 oneshot，顺序反了会把 `review/failed/waiting` 立即覆盖掉。
- goals 列表轮询若无 running 且无 completed/success/done reviewable，应发 `goal:idle`，否则上一次 goal running base 可能残留。
- 若用户反馈“还要手动切动作”，不要只检查托盘菜单：确认 `desktop_pet_windows.go` 的 `onTimer` 空闲态会定期触发 `playNextIdleAction`/oneshot 轮播，并用临时 Go 测试模拟 32 个 timer tick 验证自动动作；同时重建 `dist/ga-admin.exe` 与根目录 `genericagent-admin-go.exe`，避免实际启动旧二进制。
- 改完必须在 GA Admin Go 根目录执行：`gofmt`、`go test ./...`、`go build ./...`。

## 8.1 内置 Codex `hatch-pet` skill（已验证）
当用户要求“GA Admin 自带/导出 hatch-pet 工具链”时，不是新增某个桌宠资产，而是把 Codex skill 作为仓库内置工具链携带：
- skill vendor 落点已验证可用：`internal/hatchpet/skill`，Go 侧用 `embed.FS`，运行时导出到用户选择目录或 `~/.codex/skills/hatch-pet`。
- 后端模式：独立 `internal/hatchpet` 包提供 manifest/status/export；`internal/api` 增加 status/export/open handlers。API JSON 解析使用项目既有 `decode(r, &req)`，不要新写/误用不存在的 `readJSON`。
- 前端 Settings/桌宠区域可放“内置 hatch-pet”卡片，调用 `/api/hatch-pet/status`、`/api/hatch-pet/export`、`/api/hatch-pet/open`。
- GA Admin 根目录没有 `package.json`；前端验证必须在 `web/` 下执行 `npm.cmd run build`。
- 收尾验证：`gofmt`、`go test ./...`、`cd web && npm.cmd run build`；导出逻辑建议加 `internal/hatchpet` 包级测试，确认 embedded manifest 非空、导出后 `StatusAt` 为已导出。

## 9. 收尾清单
- [ ] `<pet_id>` 命名稳定，无空格和中文路径依赖。
- [ ] `spritesheet.webp`、`spritesheet.png`、`pet.json` 均存在。
- [ ] `validation.json` / `review.json` 无 errors。
- [ ] `contact-sheet.png` 人眼检查通过。
- [ ] `web/public/ga-admin-pets/pets.json` 已追加索引。
- [ ] 如作为桌面默认宠物，`pet_assets_windows.go` embed 路径已切换。
- [ ] `go test ./...` 与 `go build ./...` 通过。
