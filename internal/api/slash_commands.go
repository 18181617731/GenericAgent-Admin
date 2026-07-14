package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type slashCommandItem struct {
	Cmd     string `json:"cmd"`
	Key     string `json:"key,omitempty"`
	Insert  string `json:"insert,omitempty"`
	Desc    string `json:"desc"`
	BuiltIn bool   `json:"builtIn"`
	Source  string `json:"source,omitempty"`
}

var adminSlashCommands = []slashCommandItem{
	{Cmd: "/continue", Key: "/continue", Insert: "/continue", Desc: "列出可恢复的官方 GA 会话", BuiltIn: true, Source: "admin"},
	{Cmd: "/continue <编号>", Key: "/continue", Insert: "/continue ", Desc: "恢复第 N 个官方 GA 会话，可继续对话", BuiltIn: true, Source: "admin"},
	{Cmd: "/review <自然语言请求>", Key: "/review", Insert: "/review ", Desc: "审阅当前改动；可继续输入范围或关注点", BuiltIn: true, Source: "admin"},
	{Cmd: "/review help", Key: "/review help", Insert: "/review help", Desc: "显示 /review 帮助，不启动审阅", BuiltIn: true, Source: "admin"},
	{Cmd: "/improve", Key: "/improve", Insert: "/improve", Desc: "发送记忆提炼请求（L3 skill + L1 索引）", BuiltIn: true, Source: "admin"},
	{Cmd: "/effort", Key: "/effort", Insert: "/effort", Desc: "查看当前 reasoning effort", BuiltIn: true, Source: "admin"},
	{Cmd: "/effort low", Key: "/effort low", Insert: "/effort low", Desc: "设置 reasoning effort 为 low", BuiltIn: true, Source: "admin"},
	{Cmd: "/effort medium", Key: "/effort medium", Insert: "/effort medium", Desc: "设置 reasoning effort 为 medium", BuiltIn: true, Source: "admin"},
	{Cmd: "/effort high", Key: "/effort high", Insert: "/effort high", Desc: "设置 reasoning effort 为 high", BuiltIn: true, Source: "admin"},
	{Cmd: "/effort xhigh", Key: "/effort xhigh", Insert: "/effort xhigh", Desc: "设置 reasoning effort 为 xhigh（Claude 对应 max）", BuiltIn: true, Source: "admin"},
	{Cmd: "/effort off", Key: "/effort off", Insert: "/effort off", Desc: "清除 reasoning effort", BuiltIn: true, Source: "admin"},
	{Cmd: "/project", Key: "/project", Insert: "/project", Desc: "查看当前会话 Project Mode", BuiltIn: true, Source: "admin"},
	{Cmd: "/project <项目名>", Key: "/project", Insert: "/project ", Desc: "进入或切换当前会话的 Project Mode", BuiltIn: true, Source: "admin"},
	{Cmd: "/project off", Key: "/project off", Insert: "/project off", Desc: "关闭当前会话 Project Mode", BuiltIn: true, Source: "admin"},
	{Cmd: "/workspace <路径>", Key: "/workspace", Insert: "/workspace ", Desc: "为当前会话绑定项目目录", BuiltIn: true, Source: "admin"},
	{Cmd: "/workspace off", Key: "/workspace off", Insert: "/workspace off", Desc: "关闭当前会话 workspace", BuiltIn: true, Source: "admin"},
}

func (s *Server) slashCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	items := mergeSlashCommands(adminSlashCommands, discoverGASlashCommands(s.CfgStore.Cfg.GARoot, s.CfgStore.Cfg.EffectivePython))
	writeJSON(w, map[string]interface{}{"commands": items})
}

func mergeSlashCommands(groups ...[]slashCommandItem) []slashCommandItem {
	seen := map[string]bool{}
	out := []slashCommandItem{}
	for _, group := range groups {
		for _, item := range group {
			item.Cmd = strings.TrimSpace(item.Cmd)
			if item.Cmd == "" || !strings.HasPrefix(item.Cmd, "/") {
				continue
			}
			if item.Key == "" {
				item.Key = item.Cmd
			}
			if item.Insert == "" {
				item.Insert = slashInsertFor(item.Cmd)
			}
			key := strings.ToLower(strings.TrimSpace(item.Cmd))
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, item)
		}
	}
	return out
}

func slashInsertFor(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	if strings.Contains(cmd, " ") {
		return cmd
	}
	return cmd + " "
}

func discoverGASlashCommands(root, pythonPath string) []slashCommandItem {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	path := filepath.Join(root, "frontends", "slash_cmds.py")
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	return readGASlashCommandsFallback(path)
}

func readGASlashCommandsFallback(path string) []slashCommandItem {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	text := string(data)
	start := strings.Index(text, "PALETTE_ENTRIES")
	if start < 0 {
		return nil
	}
	text = text[start:]
	var rows []struct{ Cmd, Hint, Desc string }
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "(") {
			if len(rows) > 0 && strings.HasPrefix(line, "]") {
				break
			}
			continue
		}
		parts := splitPythonTupleStrings(line)
		if len(parts) >= 3 {
			rows = append(rows, struct{ Cmd, Hint, Desc string }{parts[0], parts[1], parts[2]})
		}
	}
	return gaPaletteRowsToItems(rows)
}

func gaPaletteRowsToItems(rows []struct{ Cmd, Hint, Desc string }) []slashCommandItem {
	items := make([]slashCommandItem, 0, len(rows))
	for _, row := range rows {
		cmd := strings.TrimSpace(row.Cmd)
		if cmd == "" || !strings.HasPrefix(cmd, "/") {
			continue
		}
		display := cmd
		if hint := strings.TrimSpace(row.Hint); hint != "" {
			display += " " + hint
		}
		if display == "/improve help" {
			continue
		}
		items = append(items, slashCommandItem{Cmd: display, Key: cmd, Insert: slashInsertFor(cmd), Desc: strings.TrimSpace(row.Desc), BuiltIn: true, Source: "ga"})
	}
	return items
}

func splitPythonTupleStrings(line string) []string {
	var out []string
	for i := 0; i < len(line); i++ {
		if line[i] != '\'' && line[i] != '"' {
			continue
		}
		quote := line[i]
		var b strings.Builder
		for i++; i < len(line); i++ {
			ch := line[i]
			if ch == '\\' && i+1 < len(line) {
				i++
				b.WriteByte(line[i])
				continue
			}
			if ch == quote {
				break
			}
			b.WriteByte(ch)
		}
		out = append(out, b.String())
	}
	return out
}
