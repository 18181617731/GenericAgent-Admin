package ga

import (
	"crypto/sha256"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

const projectTodoPath = autonomousTodoPath

type ProjectTodoItem struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Summary    string `json:"summary,omitempty"`
	Section    string `json:"section,omitempty"`
	Status     string `json:"status"`
	Module     string `json:"module"`
	Round      string `json:"round,omitempty"`
	Priority   string `json:"priority,omitempty"`
	Approved   bool   `json:"approved,omitempty"`
	SourcePath string `json:"source_path"`
	Line       int    `json:"line"`
}

type ProjectTodoModule struct {
	Module    string `json:"module"`
	Total     int    `json:"total"`
	Open      int    `json:"open"`
	Completed int    `json:"completed"`
	NeedsSync int    `json:"needs_sync,omitempty"`
}

type ProjectTodoOverview struct {
	SourcePath      string              `json:"source_path"`
	SourceExists    bool                `json:"source_exists"`
	SourceTruncated bool                `json:"source_truncated,omitempty"`
	UpdatedAt       time.Time           `json:"updated_at,omitempty"`
	GeneratedAt     time.Time           `json:"generated_at"`
	Total           int                 `json:"total"`
	Open            int                 `json:"open"`
	Completed       int                 `json:"completed"`
	Items           []ProjectTodoItem   `json:"items"`
	Modules         []ProjectTodoModule `json:"modules"`
}

var projectTodoRoundPattern = regexp.MustCompile(`(?i)(?:^|[^A-Z0-9])(R[0-9]+(?:-[0-9]+)?)`)
var projectTodoPriorityPattern = regexp.MustCompile(`(?i)(?:^|[^A-Z0-9])(P[0-9]+)`)
var projectTodoCommentPattern = regexp.MustCompile(`\s*<!--.*?-->\s*`)
var projectTodoNumberPrefix = regexp.MustCompile(`^[0-9]+[.)、]\s*`)

var projectTodoModuleOrder = []string{
	"overview", "notifications", "tasks", "autonomous", "goals", "models",
	"files", "memory", "channels", "usage", "settings", "logs",
}

type projectTodoModuleRule struct {
	module   string
	keywords []string
}

var projectTodoModuleRules = []projectTodoModuleRule{
	{"notifications", []string{"消息通知", "浏览器通知", "notification", "通知", "提示音", "铃声", "告警"}},
	{"usage", []string{"token", "用量", "usage", "计费", "消耗统计"}},
	{"tasks", []string{"scheduler", "sche_tasks", "定时任务", "定时调度", "调度器", "cron", "daily_git", "autosync"}},
	{"models", []string{"mykey", "deep_search", "grok", "llm", "模型", "服务商", "api key", "apikey", "codex", "opencode", "claude", "gemini", "ai编码"}},
	{"memory", []string{"mem_search", "memory", "记忆", "用户画像", "知识库", "知识沉淀", "sop"}},
	{"goals", []string{"goal mode", "ultraplan", "持续目标", "目标控制", "goal"}},
	{"logs", []string{"allure", "trace", "可观测", "执行日志", "日志", ".log"}},
	{"channels", []string{"tmwebdriver", "im bot", "web通道", "web 通道", "桌面通道", "tui", "channel", "通道"}},
	{"files", []string{"filesystem", "文件系统", "文件", "磁盘", "存储", "temp", "归档", "压缩", "解压", "pdf", "docx", "xlsx", "xml", "截图", "图像", "image", "ocr", "扫描"}},
	{"settings", []string{"runtime_backends", "python", "依赖", "环境参数", "环境配置", "path", "node", "npm", "choco", "配置"}},
	{"overview", []string{"watchdog", "服务健康", "系统状态", "运行状态", "进程", "开机自启", "启动项", "health"}},
}

func BuildProjectTodos(root string) (ProjectTodoOverview, error) {
	overview := ProjectTodoOverview{
		SourcePath:  projectTodoPath,
		GeneratedAt: time.Now(),
		Items:       make([]ProjectTodoItem, 0),
		Modules:     make([]ProjectTodoModule, 0),
	}
	detail, err := ReadSafe(root, projectTodoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return overview, nil
		}
		return overview, err
	}
	overview.SourceExists = true
	overview.SourceTruncated = detail.Truncated
	overview.UpdatedAt = detail.ModTime
	overview.Items = parseProjectTodos(detail.Content)
	summarizeProjectTodos(&overview)
	return overview, nil
}

func parseProjectTodos(content string) []ProjectTodoItem {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	items := make([]ProjectTodoItem, 0)
	section := ""
	active := -1
	for index, line := range lines {
		marker, body, ok := projectTodoChecklist(line)
		if ok {
			title, summary := projectTodoTitle(body)
			if title == "" {
				active = -1
				continue
			}
			items = append(items, newProjectTodoItem(marker, body, title, summary, section, index+1))
			active = len(items) - 1
			continue
		}
		if heading := projectTodoHeading(line); heading != "" {
			section = heading
			active = -1
			continue
		}
		if active >= 0 && items[active].Summary == "" {
			items[active].Summary = projectTodoDetail(line)
		}
	}
	return items
}

func projectTodoChecklist(line string) (rune, string, bool) {
	clean := strings.TrimSpace(line)
	if strings.HasPrefix(clean, "#") {
		prefix, rest, found := strings.Cut(clean, " ")
		if !found || strings.Trim(prefix, "#") != "" {
			return 0, "", false
		}
		clean = strings.TrimSpace(rest)
	}
	if len(clean) > 2 && (strings.HasPrefix(clean, "- ") || strings.HasPrefix(clean, "* ") || strings.HasPrefix(clean, "+ ")) {
		clean = strings.TrimSpace(clean[2:])
	}
	if len(clean) < 4 || clean[0] != '[' || clean[2] != ']' {
		return 0, "", false
	}
	marker := rune(clean[1])
	if marker != ' ' && marker != 'x' && marker != 'X' {
		return 0, "", false
	}
	return marker, strings.TrimSpace(clean[3:]), true
}

func projectTodoHeading(line string) string {
	clean := strings.TrimSpace(line)
	if !strings.HasPrefix(clean, "#") {
		return ""
	}
	prefix, rest, found := strings.Cut(clean, " ")
	if !found || strings.Trim(prefix, "#") != "" {
		return ""
	}
	return strings.TrimSpace(rest)
}

func projectTodoTitle(body string) (string, string) {
	clean := strings.TrimSpace(projectTodoCommentPattern.ReplaceAllString(body, ""))
	parts := strings.FieldsFunc(clean, func(r rune) bool { return r == '|' || r == '｜' })
	parts = slicesWithoutEmptyStrings(parts)
	if len(parts) < 2 || !projectTodoDecisionPrefix(parts[0]) {
		return clean, ""
	}
	parts = parts[1:]
	summary := ""
	if len(parts) > 1 && projectTodoNextStep(parts[len(parts)-1]) {
		summary = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, " · "), summary
}

func projectTodoDecisionPrefix(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return containsAny(lower, "用户已批准", "已批准", "用户已拒绝", "已拒绝", "待审批", "待批准")
}

func projectTodoNextStep(value string) bool {
	clean := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(clean, "按") || strings.HasPrefix(clean, "执行") || strings.HasPrefix(clean, "等待") || strings.Contains(clean, "下一步") || strings.Contains(clean, "生成报告")
}

func newProjectTodoItem(marker rune, body, title, summary, section string, line int) ProjectTodoItem {
	approved := containsAny(strings.ToLower(body), "用户已批准", "已批准", "已审批")
	item := ProjectTodoItem{
		ID: projectTodoID(title, line), Title: title, Summary: summary, Section: section,
		Status: projectTodoStatus(marker, title, approved), Module: projectTodoModule(title),
		Round: projectTodoToken(projectTodoRoundPattern, title), Priority: projectTodoToken(projectTodoPriorityPattern, title),
		Approved: approved, SourcePath: projectTodoPath, Line: line,
	}
	return item
}

func projectTodoStatus(marker rune, title string, approved bool) string {
	if marker == 'x' || marker == 'X' {
		return "completed"
	}
	if containsAny(strings.ToLower(title), "✅", "已完成", "闭环完成", "验证通过") {
		return "needs_sync"
	}
	if approved {
		return "queued"
	}
	return "pending"
}

func projectTodoToken(pattern *regexp.Regexp, title string) string {
	match := pattern.FindStringSubmatch(title)
	if len(match) < 2 {
		return ""
	}
	return strings.ToUpper(match[1])
}

func projectTodoDetail(line string) string {
	clean := strings.TrimSpace(line)
	if clean == "" || clean == "---" || strings.HasPrefix(clean, "#") {
		return ""
	}
	clean = strings.TrimSpace(strings.TrimLeft(clean, "-*+"))
	clean = projectTodoNumberPrefix.ReplaceAllString(clean, "")
	clean = strings.ReplaceAll(clean, "**", "")
	for _, separator := range []string{"：", ":"} {
		if label, value, found := strings.Cut(clean, separator); found && containsAny(strings.ToLower(label), "问题", "目标", "价值", "下一步", "范围") {
			return projectTodoLimit(value, 240)
		}
	}
	return ""
}

func projectTodoModule(title string) string {
	lower := strings.ToLower(title)
	bestModule, bestScore := "autonomous", 0
	for _, rule := range projectTodoModuleRules {
		score := 0
		for _, keyword := range rule.keywords {
			if strings.Contains(lower, keyword) {
				score++
			}
		}
		if score > bestScore {
			bestModule, bestScore = rule.module, score
		}
	}
	return bestModule
}

func summarizeProjectTodos(overview *ProjectTodoOverview) {
	modules := make(map[string]*ProjectTodoModule, len(projectTodoModuleOrder))
	for _, name := range projectTodoModuleOrder {
		modules[name] = &ProjectTodoModule{Module: name}
	}
	for _, item := range overview.Items {
		overview.Total++
		module := modules[item.Module]
		module.Total++
		if item.Status == "completed" {
			overview.Completed++
			module.Completed++
		} else {
			overview.Open++
			module.Open++
		}
		if item.Status == "needs_sync" {
			module.NeedsSync++
		}
	}
	for _, name := range projectTodoModuleOrder {
		if modules[name].Total > 0 {
			overview.Modules = append(overview.Modules, *modules[name])
		}
	}
}

func projectTodoID(title string, line int) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(title))))
	return fmt.Sprintf("todo-%x-%d", sum[:6], line)
}

func projectTodoLimit(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}
