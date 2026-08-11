package ga

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildProjectTodosParsesStatusAndModuleContext(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	content := `# TODO

## 当前周期

### [x] R10 P0: 修复 scheduler 调度错误
- **问题**：任务没有按时运行

[ ] 用户已批准 | R11 | deep_search 模型链路修复 | 按草案执行 <!-- ga-admin-approval:draft-r11 -->
[ ] 归档 temp 历史文件
[ ] 用户已批准 | R12 已完成闭环 | 等待同步 <!-- ga-admin-approval:draft-r12 -->
`
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(projectTodoPath)), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	overview, err := BuildProjectTodos(root)
	if err != nil {
		t.Fatal(err)
	}
	if !overview.SourceExists || overview.SourcePath != projectTodoPath || overview.Total != 4 || overview.Open != 3 || overview.Completed != 1 {
		t.Fatalf("overview = %+v", overview)
	}
	assertProjectTodo(t, overview.Items[0], "completed", "tasks", "R10", "P0")
	if overview.Items[0].Summary != "任务没有按时运行" || overview.Items[0].Section != "当前周期" {
		t.Fatalf("heading task context = %+v", overview.Items[0])
	}
	assertProjectTodo(t, overview.Items[1], "queued", "models", "R11", "")
	if overview.Items[1].Title != "R11 · deep_search 模型链路修复" || !overview.Items[1].Approved {
		t.Fatalf("approved task title = %+v", overview.Items[1])
	}
	assertProjectTodo(t, overview.Items[2], "pending", "files", "", "")
	assertProjectTodo(t, overview.Items[3], "needs_sync", "other", "R12", "")

	modules := map[string]ProjectTodoModule{}
	for _, module := range overview.Modules {
		modules[module.Module] = module
	}
	if modules["tasks"].Completed != 1 || modules["models"].Open != 1 || modules["files"].Open != 1 || modules["other"].Open != 1 {
		t.Fatalf("module summaries = %#v", modules)
	}
}

func TestBuildProjectTodosReturnsEmptyOverviewWhenSourceIsMissing(t *testing.T) {
	overview, err := BuildProjectTodos(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if overview.SourceExists || overview.Total != 0 || len(overview.Items) != 0 || overview.SourcePath != projectTodoPath {
		t.Fatalf("missing overview = %+v", overview)
	}
}

func TestProjectTodoModuleUsesSpecificProductDomains(t *testing.T) {
	tests := map[string]string{
		"scheduler 定时调度失败":   "tasks",
		"mykey 模型服务商配置":      "models",
		"memory 用户画像去重":      "memory",
		"PDF 文件归档与压缩":        "files",
		"Goal Mode 持续目标":     "goals",
		"watchdog 服务健康检查":    "overview",
		"浏览器通知提示音":           "notifications",
		"Token 用量统计":         "usage",
		"执行日志 trace 可观测":     "logs",
		"TMWebDriver Web 通道": "channels",
		"Python 依赖与 PATH 配置": "settings",
		"开发一个新的冷门能力工具":       "other",
	}
	for title, expected := range tests {
		if actual := projectTodoModule(title); actual != expected {
			t.Errorf("projectTodoModule(%q) = %q, want %q", title, actual, expected)
		}
	}
}

func assertProjectTodo(t *testing.T, item ProjectTodoItem, status, module, round, priority string) {
	t.Helper()
	if item.Status != status || item.Module != module || item.Round != round || item.Priority != priority || item.ID == "" || item.Line <= 0 {
		t.Fatalf("todo item = %+v", item)
	}
}
