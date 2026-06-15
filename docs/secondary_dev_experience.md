# 基于 GitNexus 的代码二次开发经验沉淀

> 以 GenericAgent-Admin 项目为样本，借助 GitNexus 索引工具，系统性学习陌生项目并仿照风格进行二次开发的方法论。

---

## 一、总览：四步法

任何代码二次开发，核心是**先读懂项目基因，再模仿基因编码**。四步走：

```
Step 1: 用 GitNexus 建立高层鸟瞰 → 理解"项目是什么"
Step 2: 用 GitNexus context/query 追踪关键链路 → 理解"核心流程怎么走"
Step 3: 用 GitNexus impact + 源码阅读 提取编码规范 → 理解"怎么写是对的"
Step 4: 仿照模式编码 → 写出的代码像原项目作者写的
```

---

## 二、Step 1: 建立高层鸟瞰 (宏观)

### 2.1 索引项目
```bash
gitnexus analyze --index-only .
```
首次索引全量扫描，后续自动增量。得到 `6023 symbols | 14280 edges | 166 clusters | 300 flows`。

### 2.2 识别项目骨架
不看单个文件，先看全局结构。关键命令：

```bash
# 看项目统计
gitnexus status

# 看模块布局
ls internal/   # Go: 每个子目录 = 一个包 = 一个职责域

# 看入口
gitnexus context main  # 找到 main.go:main → 追踪启动链路
```

**实战发现** (GenericAgent-Admin):
- `internal/api/` 最大，是项目核心 => 所有功能通过 HTTP API 暴露
- `internal/ga/` 操作外部 GA 工作空间 => 项目本质是"GA 的外壳/管理器"
- `internal/version/` 有自更新 => 项目是独立的可发行二进制
- `go.mod` 只有一个外部依赖 => 偏好零外部依赖、用标准库
- `web/` 是 Vite+React SPA => 前后端耦合的整体应用

### 2.3 经验法则
1. **先看 import 层级**: 谁 import 谁 → 依赖方向 → 架构分层
2. **先看 go.mod / package.json**: 依赖量 = 封装倾向 (少=自造轮子，多=组装)
3. **先看顶层目录名**: 直接反映作者的心智划分
4. **先看配置结构**: config 文件是业务模型的浓缩

---

## 三、Step 2: 追踪关键链路 (中观)

### 3.1 从符号名出发
```bash
# 追踪一个 handler 的完整调用链
gitnexus context gaControl --repo GenericAgent-Admin
```
输出：
```
gaControl (api.go:271)
  incoming: Routes → Server
  outgoing calls: writeJSON, bad, BuildControlPlane
  outgoing accesses: CfgStore
```

### 3.2 反向追踪影响面
```bash
# 如果要改一个函数，会影响谁
gitnexus impact <symbol> --repo GenericAgent-Admin
```

### 3.3 搜索语义相关代码
```bash
# 自然语言搜索代码
gitnexus query "route handler pattern" --repo GenericAgent-Admin
```

### 3.4 经验法则
1. **先追一条完整请求链路**: HTTP 请求 → handler → 业务逻辑 → 数据存储 → 响应
2. **看完 Router 注册表**: `api.go:Routes()` 的 80+ 行路由注册 = 完整功能清单
3. **看中间件**: GA-Admin 只有 `recoverPanics + cors + requireDangerousConfirm` → 极其轻量
4. **看测试中的常量/约定**: `dangerous_confirm_test.go` 里有项目所有危险路由的"白名单" → 比文档更准确

---

## 四、Step 3: 提取编码规范 (微观)

### 4.1 命名规范
从代码中提取，不猜：

| 维度 | GA-Admin 规范 | 提取方式 |
|------|-------------|---------|
| 包名 | `api`, `ga`, `version` (全小写简短) | `ls internal/` |
| 导出类型 | `Server`, `ControlPlane`, `ServiceInfo` | `grep "type [A-Z]" internal/` |
| 未导出函数 | `buildWorkspaceSummary`, `readStatusLocked` | `grep "func [a-z]" internal/` |
| JSON 标签 | `json:"ga_root"`, `json:"log_tail_lines"` | 扫 struct 定义 |
| 测试函数 | `TestDangerousConfirmWrapperRejectsMissingHeader` | `grep "func Test" internal/` |

### 4.2 错误处理模式
```go
// GA-Admin 的统一模式: 所有 handler 返回前统一 check
if err != nil {
    bad(w, http.StatusInternalServerError, "描述性消息: "+err.Error())
    return
}
// 成功时:
ok(w, result)
```
- 不用 panic（除了 recover middleware）
- 错误消息中文/英文混用（按功能域）
- 使用 `writeJSON` / `bad` / `ok` 辅助函数统一响应格式

### 4.3 依赖注入模式
```go
// Server 作为"上帝结构体"，聚合所有依赖
type Server struct {
    CfgStore    *config.Store
    Svc         *service.Manager
    Models      *modelconfig.Store
    // ...
}
// 构造器 New() 完成注入
// handler 作为 Server 的方法 (receiver)
```

### 4.4 并发模式
```go
sync.Mutex  // 保护共享状态
sync.Once   // 懒初始化
// 进程管理使用 os.StartProcess + PID 轮询
```

### 4.5 经验法则
1. **别猜，直接 grep**: `grep "func Test"` → 测试命名模式，`grep "json:"` → 序列化约定
2. **找重复模式**: `writeFileAtomic` 出现两次 → 原子写入是项目偏好
3. **看注释语言**: 中文注释 + 英文标识符 = 中文团队 + 国际化输出
4. **看 import 分组**: 空行分隔标准库/内部包 → 代码格式化工具 (`goimports`)

---

## 五、Step 4: 仿照模式编码

### 5.1 新增 API 路由的"填空模板"

假设要给 GA-Admin 加一个 `/api/ga/memory/stats` 路由：

```go
// Step 1: 在 api.go Routes() 中注册
mux.HandleFunc("/api/ga/memory/stats", s.gaMemoryStats)
// 如果是危险操作:
mux.HandleFunc("/api/ga/memory/clean", s.requireDangerousConfirm(s.gaMemoryClean))

// Step 2: 在 ga_memory_handlers.go 实现 handler
func (s *Server) gaMemoryStats(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        bad(w, http.StatusMethodNotAllowed, "仅支持 GET")
        return
    }
    stats, err := ga.BuildMemoryStats(s.CfgStore.Cfg.GARoot)
    if err != nil {
        bad(w, http.StatusInternalServerError, "获取内存统计失败: "+err.Error())
        return
    }
    writeJSON(w, http.StatusOK, stats)
}

// Step 3: 在 api.go riskCatalog 添加条目 (如果是危险操作)
{Path: "/api/ga/memory/clean", Level: "dangerous",
 Action: "clean_memory", Reason: "删除内存文件"}

// Step 4: 编写测试
func TestGAMemoryStatsRejectsNonGET(t *testing.T) { /* ... */ }
func TestGAMemoryStatsReturnsValidJSON(t *testing.T) { /* ... */ }

// Step 5: 前端调用 (web/src/lib/)
export const memoryStats = () => api("/api/ga/memory/stats")
export const memoryClean = () => api("/api/ga/memory/clean", { dangerous: true })
```

### 5.2 关键原则
1. **复制最近的同类文件**: 新建 handler → 看 `ga_handlers.go`，新建测试 → 看 `non_chat_contract_test.go`
2. **先写测试再写实现**: 测试即规格，也即"预期行为"的文档
3. **保守修改**: 不改已有的共享结构 (Server/CfgStore)，只新增方法和字段
4. **前后端同步**: 危险操作前后端都要标记 (`dangerous: true` + `requireDangerousConfirm` + `riskCatalogItems`)

---

## 六、GitNexus 作为"代码考古层"

### 6.1 什么时候用 GitNexus
| 场景 | GitNexus 命令 | 代替 |
|------|-------------|------|
| 不知道项目结构 | `analyze` + 看模块 | `ls -R` + 人肉理解 |
| 想找某功能在哪 | `query "自然语言描述"` | `grep -r` 猜关键词 |
| 想理解调用链 | `context <符号名>` | IDE "Go to Definition" 反复跳 |
| 想评估改动影响 | `impact <符号>` | 全局搜索 + 人肉分析 |
| 想了解架构聚类 | `query "cluster"` → clusters | 手动分析 |

### 6.2 GitNexus 的局限与补充
- **不替换源码阅读**: context 告诉你"谁调了谁"，但"为什么这样写"必须看源码
- **需要先有索引**: 每次改代码后 `analyze --index-only` 增量更新
- **自然语言查询有噪音**: 需要尝试不同关键词组合
- **不能读业务逻辑**: 只能追踪符号间的关系

### 6.3 最佳搭配
```
GitNexus (导航) + 源码阅读 (理解) + grep (精确查找) + test 文件 (规格文档)
```

---

## 七、可迁移的二次开发通用框架

### 7.1 适用于任何项目的四步清单

```
□ 1. 建立索引 → gitnexus analyze
□ 2. 识别骨架:
   □ 入口文件在哪
   □ 模块/包如何划分
   □ 核心数据结构是什么 (看最大的 struct)
   □ 配置格式是什么样的
□ 3. 追踪一条完整链路:
   □ 请求入口 → 中间件 → 业务处理 → 数据落盘 → 响应
□ 4. 提取规范:
   □ 命名规则 (导出/未导出/JSON标签/测试)
   □ 错误处理统一模式
   □ 依赖注入方式
   □ 并发/安全约定
□ 5. 仿写:
   □ 找最近的相似文件作为模板
   □ 先写测试
   □ 前后端同步 (如有)
   □ 保持与现有代码的风格一致性
```

### 7.2 安全底线 (从不跳过)
1. **从不改核心结构体除非必要**: 优先新增字段/方法，不重构
2. **从不跳过危险确认机制**: 如果原项目有 safety gate，新增功能也要穿
3. **永远先读测试**: 测试是真实运行的规格文档，比注释可靠
4. **永远保持 import 约定**: 不引入新的外部依赖除非绝对必要

### 7.3 效率技巧
- **第一次索引后立刻读 `Routes()`** → 完整功能清单
- **`grep "func Test"` 输出** → 项目的"考试大纲"
- **RISK_CATALOG (如有)** → 哪些操作是敏感的
- **最大文件通常是核心**: `api.go 1025行` = 项目心脏
- **配置文件 = 数据模型**: `config.example.json 8行` = 极简设计哲学

---

## 八、GA-Admin 的具体编码基因 (速查)

当你需要为这个项目写代码时，直接从以下模板套：

### Go Handler 模板
```go
func (s *Server) myFeature(w http.ResponseWriter, r *http.Request) {
    // 1. 方法检查
    if r.Method != http.MethodGet {
        bad(w, http.StatusMethodNotAllowed, "仅支持 GET")
        return
    }
    // 2. 获取配置
    root := s.CfgStore.Cfg.GARoot
    // 3. 业务逻辑
    result, err := doSomething(root)
    if err != nil {
        bad(w, http.StatusInternalServerError, "操作失败: "+err.Error())
        return
    }
    // 4. 返回
    writeJSON(w, http.StatusOK, result)
}
```

### Go Test 模板
```go
func TestMyFeatureRejectsNonGET(t *testing.T) {
    srv := testServer(t, ".")
    w := httptest.NewRecorder()
    r := httptest.NewRequest("POST", "/api/my-feature", nil)
    srv.ServeHTTP(w, r)
    if w.Code != http.StatusMethodNotAllowed {
        t.Fatalf("expected 405, got %d", w.Code)
    }
}
```

### 前端 api.js 调用模板
```js
// 只读 GET
export const getData = () => api("/api/my-feature")

// 危险操作 POST (需要 confirm)
export const doDangerousThing = (data) =>
    api("/api/my-feature", {
        method: "POST",
        body: JSON.stringify(data),
        dangerous: true,
    })
```

---

## 九、实战案例：为反思服务启动增加模型选择弹窗

### 9.1 需求分析
**需求**：在"自主进化"页面的反思服务启动按钮上，增加弹窗支持用户在启动前选择模型。

**关键问题**：
1. 启动按钮在哪个文件？
2. 现有代码有没有类似的弹窗模式可以复用？
3. 改动影响面有多大？

### 9.2 高效定位流程（实战记录）

#### Step 1: 读项目文档（2秒）
```bash
ls /path/to/GenericAgent-Admin/docs/
# 发现 secondary_dev_experience.md，确认有 GitNexus 索引
```

#### Step 2: GitNexus 语义搜索（5秒）
```bash
/path/to/gitnexus-cli/bin/gitnexus query "反思服务 自主进化 启动" --repo GenericAgent-Admin
# 输出：
# - GoalsPage.jsx (startLine: 48)
# - App.jsx:confirmServiceStart (类似弹窗逻辑)
```

**关键发现**：
- 目标页面：`web/src/pages/GoalsPage.jsx` 第 48 行附近
- 已有模式：`App.jsx` 的 `confirmServiceStart()` 已经实现了服务启动弹窗选模型

#### Step 3: 精确读取确认（10秒）
```bash
file_read web/src/App.jsx --start 270 --count 50
# 确认：serviceStartDialog + serviceStartLLMNo + confirmServiceStart 完整链路
```

### 9.3 变更方案（最小改动原则）

**发现可复用模式**：`App.jsx:280-296` 的 Service 启动弹窗流程

| 改动位置 | 改动类型 | 行数 |
|----------|----------|------|
| `App.jsx` | 新增状态：`goalStartDialog`/`goalStartLLMNo` | +2 |
| `App.jsx` | 改名：`startGoal()` → `confirmGoalStart()` | ~5 |
| `App.jsx` | 新增组件：`<GoalStartDialog>` | +30 |
| `GoalsPage.jsx` | 修改按钮：`onClick={onStart}` → 打开弹窗 | 1 |

**关键设计决策**：
- ✅ **复用现有弹窗组件结构**（ServiceStartDialog），保持 UI 一致性
- ✅ **后端 API 无需改动**（`/api/goals/start` 已支持 `llm_no` 参数）
- ✅ **前后端同步**（弹窗内参数校验与表单一致）

### 9.4 效率对比

| 方法 | 定位时间 | 上下文污染 | 准确度 |
|------|----------|------------|--------|
| ❌ 传统：grep + 递归读文件 | ~5分钟 | 高（扫描几百行代码） | 低（可能漏掉相关逻辑） |
| ✅ 正确：docs + GitNexus + 精确读取 | ~20秒 | 极低（只读 3 个代码段） | 高（找到可复用模式） |

**关键收益**：
1. **避免重复造轮：通过 GitNexus 发现已有 `ServiceStartDialog`，直接复用**
2. **精准定位：只读需要改的文件和行号，不扫全项目**
3. **理解影响面：从 `query` 结果直接看出前后端改动范围**

### 9.5 教训总结

**错误做法**：
```bash
# ❌ 不看文档直接 rg
rg "启动|start" --type jsx  # 输出几百行，噪音大
rg "func.*Start" --type go  # 不知道哪个是目标函数

# ❌ 盲目读文件
file_read web/src/pages/GoalsPage.jsx  # 读全文 218 行，浪费上下文
file_read web/src/App.jsx  # 读全文 838 行，更浪费
```

**正确做法**：
```bash
# ✅ Step 1: 读文档确认工具链
ls docs/ && cat docs/secondary_dev_experience.md

# ✅ Step 2: GitNexus 语义搜索
gitnexus query "功能关键词" --repo <项目名>

# ✅ Step 3: 精确行号读取
file_read <文件> --start <行号> --count 50
```

---

## 十、总结

| 层次 | 工具 | 产出 |
|------|------|------|
| 宏观 | `gitnexus status` + 目录结构 | 项目定位 + 模块划分 |
| 中观 | `gitnexus context` / `query` | 调用链路 + 关键流程 |
| 微观 | 源码阅读 + `grep` 模式提取 | 编码规范 + 设计模式 |
| 执行 | 仿照模板编码 | 新代码像原作者写的 |

**核心理念**: 二次开发不是凭空设计，而是**考古 + 模仿**。GitNexus 是考古用的"洛阳铲"，帮你快速看清地层结构；真正的理解来自源码阅读和测试研读。写新代码时，找一个最近的同类文件当模板，填空式开发，保持风格一致。

**效率铁律**：
1. **文档优先**：有 `/docs` 先读文档，了解工具链和架构
2. **GitNexus 定位**：用语义搜索找到文件和行号，不要 grep 扫全项目
3. **精确读取**：只读需要改的代码段，避免上下文污染
4. **复用优先**：先找类似功能，复制粘贴改，不要从零开始写

---

> 生成于 2026-06-14 | 基于 GitNexus 对 GenericAgent-Admin 的索引分析