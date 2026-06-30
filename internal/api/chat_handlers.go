package api

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s *Server) chatSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	items := []map[string]interface{}{}
	if err := ensureChatDataMigrated(s.CfgStore.Cfg); err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.MkdirAll(chatSessionDir(s.CfgStore.Cfg), 0755); err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	entries, err := os.ReadDir(chatSessionDir(s.CfgStore.Cfg))
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		cs, err := loadChatSession(s.CfgStore.Cfg, strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			continue
		}
		items = append(items, map[string]interface{}{"id": cs.ID, "title": cs.Title, "updated_at": cs.UpdatedAt, "count": len(cs.Messages), "running": s.chatRunActive(cs.ID), "workspace": cs.Workspace})
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["updated_at"].(int64) > items[j]["updated_at"].(int64) })
	if len(items) > 80 {
		items = items[:80]
	}
	writeJSON(w, map[string]interface{}{"sessions": items})
}

func (s *Server) chatHandler(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/api/chat/")
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		bad(w, 404, "not found")
		return
	}
	switch parts[0] {
	case "session":
		if len(parts) == 2 && parts[1] == "new" && r.Method == http.MethodPost {
			s.chatNewSession(w, r)
			return
		}
		if len(parts) == 2 && r.Method == http.MethodGet {
			s.chatGetSession(w, r, parts[1])
			return
		}
		if len(parts) == 2 && r.Method == http.MethodPatch {
			s.chatRenameSession(w, r, parts[1])
			return
		}
		if len(parts) == 2 && r.Method == http.MethodDelete {
			s.chatDeleteSession(w, r, parts[1])
			return
		}
	case "settings":
		if len(parts) == 2 && r.Method == http.MethodPost {
			s.chatSaveSettings(w, r, parts[1])
			return
		}
	case "state":
		if len(parts) == 1 && r.Method == http.MethodGet {
			s.chatState(w, r, "")
			return
		}
		if len(parts) == 2 && r.Method == http.MethodGet {
			s.chatState(w, r, parts[1])
			return
		}
	case "stream":
		if len(parts) == 2 && r.Method == http.MethodGet {
			s.chatStream(w, r, parts[1])
			return
		}
	case "cancel":
		if len(parts) == 2 && r.Method == http.MethodPost {
			s.chatCancel(w, r, parts[1])
			return
		}
	case "reinject-tools":
		if len(parts) == 2 && r.Method == http.MethodPost {
			s.chatReinjectTools(w, r, parts[1])
			return
		}
	case "file":
		if len(parts) >= 2 && r.Method == http.MethodGet {
			s.chatFile(w, r, strings.Join(parts[1:], "/"))
			return
		}
	default:
		if len(parts) == 1 && r.Method == http.MethodPost {
			s.chatPost(w, r, parts[0])
			return
		}
	}
	bad(w, 404, "not found")
}

func (s *Server) chatNewSession(w http.ResponseWriter, r *http.Request) {
	cs := chatSession{ID: newChatID(), Title: "新会话", UpdatedAt: time.Now().Unix(), Messages: []chatMessage{}, Settings: normalizeChatSettings(chatSettings{}), RawHistory: []map[string]interface{}{}}
	if err := saveChatSession(s.CfgStore.Cfg, cs); err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, chatSessionForClient(cs))
}

func (s *Server) chatGetSession(w http.ResponseWriter, r *http.Request, sid string) {
	cs, err := loadChatSession(s.CfgStore.Cfg, safeChatID(sid))
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, chatSessionForClient(cs))
}

func (s *Server) chatRenameSession(w http.ResponseWriter, r *http.Request, sid string) {
	var req struct {
		Title string `json:"title"`
	}
	if err := decode(r, &req); err != nil {
		bad(w, 400, err.Error())
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		bad(w, 400, "title required")
		return
	}
	if len([]rune(title)) > 80 {
		title = string([]rune(title)[:80])
	}
	cs, err := loadChatSession(s.CfgStore.Cfg, safeChatID(sid))
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	cs.Title = title
	cs.UpdatedAt = time.Now().Unix()
	if err := saveChatSession(s.CfgStore.Cfg, cs); err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, chatSessionForClient(cs))
}

func (s *Server) chatDeleteSession(w http.ResponseWriter, r *http.Request, sid string) {
	_ = os.Remove(chatSessionPath(s.CfgStore.Cfg, safeChatID(sid)))
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) chatSaveSettings(w http.ResponseWriter, r *http.Request, sid string) {
	var st chatSettings
	if err := decode(r, &st); err != nil {
		bad(w, 400, err.Error())
		return
	}
	cs, err := loadChatSession(s.CfgStore.Cfg, safeChatID(sid))
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	cs.Settings = normalizeChatSettings(st)
	if err := saveChatSession(s.CfgStore.Cfg, cs); err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "settings": cs.Settings})
}

func (s *Server) chatState(w http.ResponseWriter, r *http.Request, sid string) {
	cs, err := loadChatSession(s.CfgStore.Cfg, safeChatID(sid))
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	cs.Settings = normalizeChatSettings(cs.Settings)
	llms, err := s.listGARuntimeLLMs(s.CfgStore.Cfg)
	markChatLLMActive(llms, cs.Settings.LLMNo)
	backend := map[string]string{"class": "GenericAgent worker", "source": "agentmain.GenericAgent.list_llms"}
	if err != nil {
		backend["warning"] = err.Error()
	}
	running := s.chatRunActive(sid)
	writeJSON(w, map[string]interface{}{"settings": cs.Settings, "llm_no": cs.Settings.LLMNo, "llms": llms, "backend": backend, "running": running, "workspace": cs.Workspace})
}

func (s *Server) maybeHandleWorkspaceCommand(w http.ResponseWriter, r *http.Request, sid string, cs *chatSession, prompt string) bool {
	cmd := strings.TrimSpace(prompt)
	if cmd != "/workspace" && !strings.HasPrefix(cmd, "/workspace ") && !strings.HasPrefix(cmd, "/workspace\t") {
		return false
	}
	reply := ""
	arg := strings.TrimSpace(strings.TrimPrefix(cmd, "/workspace"))
	switch {
	case arg == "":
		if strings.TrimSpace(cs.Workspace) == "" {
			reply = "Workspace 模式未启用。用法：`/workspace <绝对路径>`，关闭：`/workspace off`。"
		} else {
			reply = fmt.Sprintf("当前 workspace：`%s`\n\n关闭：`/workspace off`。", cs.Workspace)
		}
	case strings.EqualFold(arg, "off") || strings.EqualFold(arg, "disable") || strings.EqualFold(arg, "none"):
		cs.Workspace = ""
		reply = "已关闭当前会话的 workspace 模式。"
	default:
		abs, err := filepath.Abs(arg)
		if err != nil {
			reply = fmt.Sprintf("设置 workspace 失败：%v", err)
			break
		}
		st, err := os.Stat(abs)
		if err != nil {
			reply = fmt.Sprintf("设置 workspace 失败：目录不存在或不可访问：`%s`", abs)
			break
		}
		if !st.IsDir() {
			reply = fmt.Sprintf("设置 workspace 失败：不是目录：`%s`", abs)
			break
		}
		cs.Workspace = abs
		reply = fmt.Sprintf("已启用当前会话 workspace：`%s`\n\n之后本会话任务会优先在该目录执行。", abs)
	}
	msg := chatMessage{ID: newChatID(), Role: "assistant", Content: reply, CreatedAt: time.Now().Unix()}
	cs.Messages = append(cs.Messages, msg)
	cs.UpdatedAt = time.Now().Unix()
	if err := saveChatSession(s.CfgStore.Cfg, *cs); err != nil {
		s.endChatRun(sid)
		bad(w, http.StatusInternalServerError, err.Error())
		return true
	}
	s.publishChatRun(sid, map[string]interface{}{"type": "message", "message": msg, "workspace": cs.Workspace})
	s.endChatRun(sid)
	s.streamChatRun(w, r, sid, 0)
	return true
}

func (s *Server) chatPost(w http.ResponseWriter, r *http.Request, sid string) {
	var req struct {
		Prompt       string        `json:"prompt"`
		Files        []chatUpload  `json:"files"`
		Settings     *chatSettings `json:"settings"`
		ClientUserID string        `json:"client_user_id"`
	}
	if err := decodeLimited(r, &req, maxChatPostBodyBytes); err != nil {
		bad(w, 400, err.Error())
		return
	}
	sid = safeChatID(sid)
	if !s.beginChatRun(sid) {
		bad(w, 409, "chat is already running")
		return
	}
	cs, err := loadChatSession(s.CfgStore.Cfg, sid)
	if err != nil {
		s.endChatRun(sid)
		bad(w, 500, err.Error())
		return
	}
	if cs.ID == "" {
		cs.ID = sid
		cs.Title = "新会话"
	}
	cs.Settings = normalizeChatSettings(cs.Settings)
	if req.Settings != nil {
		cs.Settings = normalizeChatSettings(*req.Settings)
	}
	if s.maybeHandleWorkspaceCommand(w, r, sid, &cs, req.Prompt) {
		return
	}
	saved, refs, err := saveChatUploads(s.CfgStore.Cfg, req.Files)
	if err != nil {
		s.endChatRun(sid)
		bad(w, 400, err.Error())
		return
	}
	display := req.Prompt
	if len(refs) > 0 {
		display += "\n\n[附件已保存]\n" + strings.Join(refs, "\n")
	}
	uid := safeChatID(req.ClientUserID)
	if uid == "" {
		uid = newChatID()
	}
	userMsg := chatMessage{ID: uid, Role: "user", Content: display, Files: saved, CreatedAt: time.Now().Unix()}
	cs.Messages = append(cs.Messages, userMsg)
	updateChatTitle(&cs)
	if err := saveChatSession(s.CfgStore.Cfg, cs); err != nil {
		s.endChatRun(sid)
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishChatRun(sid, map[string]interface{}{"type": "user", "message": userMsg})
	workerHistory := append([]chatMessage(nil), cs.Messages[:len(cs.Messages)-1]...)
	cmdReq := map[string]interface{}{
		"prompt":           display,
		"history":          workerHistory,
		"raw_history":      cs.RawHistory,
		"history_info":     cs.HistoryInfo,
		"working":          cs.Working,
		"workspace":        cs.Workspace,
		"llm_no":           cs.Settings.LLMNo,
		"tools_mode":       cs.Settings.ToolsMode,
		"reasoning_effort": cs.Settings.ReasoningEffort,
		"ga_root":          s.CfgStore.Cfg.GARoot,
	}
	go s.runChatWorker(sid, cs, cmdReq)
	s.streamChatRun(w, r, sid, 0)
}

func (s *Server) chatStream(w http.ResponseWriter, r *http.Request, sid string) {
	from := 0
	if v := strings.TrimSpace(r.URL.Query().Get("from")); v != "" {
		_, _ = fmt.Sscanf(v, "%d", &from)
	}
	s.streamChatRun(w, r, safeChatID(sid), from)
}

func (s *Server) chatReinjectTools(w http.ResponseWriter, r *http.Request, sid string) {
	sid = safeChatID(sid)
	if s.chatRunActive(sid) {
		bad(w, http.StatusConflict, "chat is already running")
		return
	}
	ev, err := s.reinjectChatWorkerTools(sid)
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ev == nil {
		bad(w, http.StatusInternalServerError, "worker returned empty response")
		return
	}
	if ok, _ := ev["ok"].(bool); !ok {
		msg, _ := ev["message"].(string)
		if msg == "" {
			msg = "tools reinjection failed"
		}
		bad(w, http.StatusInternalServerError, msg)
		return
	}
	writeJSON(w, ev)
}

func (s *Server) chatCancel(w http.ResponseWriter, r *http.Request, sid string) {
	sid = safeChatID(sid)
	var cmd *exec.Cmd
	var worker *chatWorker
	s.ChatMu.Lock()
	run := s.ChatRuns[sid]
	if run == nil || run.Done {
		s.ChatMu.Unlock()
		writeJSON(w, map[string]interface{}{"ok": true, "running": false})
		return
	}
	run.Canceled = true
	cmd = run.Cmd
	worker = s.ChatWorkers[sid]
	s.ChatMu.Unlock()
	if worker != nil {
		s.dropChatWorker(sid, worker)
	} else if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	writeJSON(w, map[string]interface{}{"ok": true, "running": false})
}

func (s *Server) chatFile(w http.ResponseWriter, r *http.Request, name string) {
	http.ServeFile(w, r, filepath.Join(chatUploadDir(s.CfgStore.Cfg), filepath.Base(name)))
}
