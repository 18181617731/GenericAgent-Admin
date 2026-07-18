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
		items = append(items, map[string]interface{}{"id": cs.ID, "title": cs.Title, "updated_at": cs.UpdatedAt, "count": len(cs.Messages), "running": s.chatRunActive(cs.ID), "workspace": cs.Workspace, "project_mode": cs.ProjectMode})
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
	case "fork":
		if len(parts) == 2 && r.Method == http.MethodPost {
			s.chatForkSession(w, r, parts[1])
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
	case "btw":
		if len(parts) == 2 && r.Method == http.MethodPost {
			s.chatBTW(w, r, parts[1])
			return
		}
	case "worldline":
		if len(parts) == 2 && r.Method == http.MethodGet {
			s.chatWorldlineState(w, r, parts[1])
			return
		}
		if len(parts) == 3 && parts[2] == "switch" && r.Method == http.MethodPost {
			s.chatWorldlineSwitch(w, r, parts[1])
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
	writeJSON(w, chatSessionForClient(cs))
}

func visibleRawUserText(item map[string]interface{}) (string, bool) {
	content, ok := item["content"]
	if !ok {
		return "", false
	}
	text := rawChatText(content)
	return text, text != ""
}

func rawHistoryBeforeMessage(cs chatSession, messageIndex int) ([]map[string]interface{}, error) {
	if len(cs.RawHistory) == 0 {
		return []map[string]interface{}{}, nil
	}
	target := cs.Messages[messageIndex]
	occurrence := 0
	for i := 0; i <= messageIndex; i++ {
		if cs.Messages[i].Role == "user" && cs.Messages[i].Content == target.Content {
			occurrence++
		}
	}
	seen := 0
	for i, item := range cs.RawHistory {
		role, _ := item["role"].(string)
		text, ok := visibleRawUserText(item)
		if role != "user" || !ok || text != target.Content {
			continue
		}
		seen++
		if seen == occurrence {
			return append([]map[string]interface{}(nil), cs.RawHistory[:i]...), nil
		}
	}
	return nil, fmt.Errorf("raw history does not contain the selected user message")
}

func (s *Server) chatForkSession(w http.ResponseWriter, r *http.Request, sid string) {
	sid = safeChatID(sid)
	if sid == "" {
		bad(w, http.StatusBadRequest, "invalid session id")
		return
	}
	if s.chatRunActive(sid) {
		bad(w, http.StatusConflict, "chat is already running")
		return
	}
	var req struct {
		MessageID string `json:"message_id"`
	}
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	messageID := safeChatID(req.MessageID)
	if messageID == "" {
		bad(w, http.StatusBadRequest, "message_id is required")
		return
	}

	s.SessionMu.Lock()
	defer s.SessionMu.Unlock()
	cs, err := loadChatSession(s.CfgStore.Cfg, sid)
	if err != nil {
		bad(w, http.StatusNotFound, "session not found")
		return
	}
	targetIndex := -1
	for i := range cs.Messages {
		if cs.Messages[i].ID == messageID && cs.Messages[i].Role == "user" {
			targetIndex = i
			break
		}
	}
	if targetIndex < 0 {
		bad(w, http.StatusNotFound, "user message not found")
		return
	}
	raw, err := rawHistoryBeforeMessage(cs, targetIndex)
	if err != nil {
		bad(w, http.StatusConflict, err.Error())
		return
	}
	title := strings.TrimSpace(cs.Title)
	if title == "" || title == "新会话" {
		title = "编辑分支"
	} else {
		title += " · 分支"
	}
	fork := chatSession{
		ID:          newChatID(),
		Title:       title,
		Messages:    append([]chatMessage(nil), cs.Messages[:targetIndex]...),
		Settings:    cs.Settings,
		RawHistory:  raw,
		HistoryInfo: []interface{}{},
		Working:     nil,
		Workspace:   cs.Workspace,
		ProjectMode: cs.ProjectMode,
	}
	if err := saveChatSessionLocked(s.CfgStore.Cfg, fork); err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, chatSessionForClient(fork))
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
	if !validChatWorldlineID(sid) {
		bad(w, http.StatusBadRequest, "invalid session id")
		return
	}
	if s.chatRunActive(sid) {
		bad(w, http.StatusConflict, "chat is already running")
		return
	}
	s.ChatMu.Lock()
	worker := s.ChatWorkers[sid]
	s.ChatMu.Unlock()
	if worker != nil {
		s.dropChatWorker(sid, worker)
	}
	_ = os.Remove(chatSessionPath(s.CfgStore.Cfg, sid))
	sidecar := filepath.Join(s.CfgStore.Cfg.GARoot, "temp", "rewind_data", "ga-admin", "admin_sidecars", sid+".json")
	_ = os.Remove(sidecar)
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
	writeJSON(w, map[string]interface{}{"settings": cs.Settings, "llm_no": cs.Settings.LLMNo, "llms": llms, "backend": backend, "running": running, "workspace": cs.Workspace, "project_mode": cs.ProjectMode})
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

func validProjectModeName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" || name == "." || name == ".." || len([]byte(name)) > 128 || filepath.IsAbs(name) || filepath.Clean(name) != name || strings.ContainsAny(name, `/\\:`) || strings.HasSuffix(name, ".") {
		return "", false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", false
		}
	}
	return name, true
}

func (s *Server) maybeHandleProjectCommand(w http.ResponseWriter, r *http.Request, sid string, cs *chatSession, prompt string) bool {
	cmd := strings.TrimSpace(prompt)
	if cmd != "/project" && !strings.HasPrefix(cmd, "/project ") && !strings.HasPrefix(cmd, "/project\t") {
		return false
	}

	arg := strings.TrimSpace(strings.TrimPrefix(cmd, "/project"))
	reply := ""
	switch {
	case arg == "" || strings.EqualFold(arg, "status"):
		if strings.TrimSpace(cs.ProjectMode) == "" {
			reply = "Project Mode 未启用。用法：`/project <项目名>`，关闭：`/project off`。"
		} else {
			reply = fmt.Sprintf("当前 Project Mode：`%s`\n\n项目记忆：`%s`\n\n关闭：`/project off`。", cs.ProjectMode, filepath.Join(s.CfgStore.Cfg.GARoot, "temp", "projects", cs.ProjectMode, "project_memory.md"))
		}
	case strings.EqualFold(arg, "off") || strings.EqualFold(arg, "disable") || strings.EqualFold(arg, "none"):
		cs.ProjectMode = ""
		reply = "已关闭当前会话的 Project Mode。项目文件和记忆均已保留。"
	default:
		name, ok := validProjectModeName(arg)
		if !ok {
			reply = "进入 Project Mode 失败：项目名必须是 1 个安全的目录名称（不能包含路径分隔符、冒号、控制字符或 `.` / `..`）。"
			break
		}
		gaRoot := strings.TrimSpace(s.CfgStore.Cfg.GARoot)
		if gaRoot == "" {
			reply = "进入 Project Mode 失败：GA Root 未配置。"
			break
		}
		projectDir := projectModeWorkspace(s.CfgStore.Cfg, name)
		if st, err := os.Lstat(projectDir); err == nil {
			if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
				reply = fmt.Sprintf("进入 Project Mode 失败：项目路径不是安全目录：`%s`", projectDir)
				break
			}
		} else if !os.IsNotExist(err) {
			reply = fmt.Sprintf("进入 Project Mode 失败：无法检查项目目录：%v", err)
			break
		} else if err := os.MkdirAll(projectDir, 0755); err != nil {
			reply = fmt.Sprintf("进入 Project Mode 失败：无法创建项目目录：%v", err)
			break
		}
		memoryPath := filepath.Join(projectDir, "project_memory.md")
		if f, err := os.OpenFile(memoryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644); err == nil {
			if closeErr := f.Close(); closeErr != nil {
				reply = fmt.Sprintf("进入 Project Mode 失败：无法初始化项目记忆：%v", closeErr)
				break
			}
		} else if !os.IsExist(err) {
			reply = fmt.Sprintf("进入 Project Mode 失败：无法初始化项目记忆：%v", err)
			break
		}
		cs.ProjectMode = name
		reply = fmt.Sprintf("已进入 Project Mode：`%s`\n\n项目目录：`%s`\n项目记忆：`%s`", name, projectDir, memoryPath)
	}

	msg := chatMessage{ID: newChatID(), Role: "assistant", Content: reply, CreatedAt: time.Now().Unix()}
	cs.Messages = append(cs.Messages, msg)
	cs.UpdatedAt = time.Now().Unix()
	if err := saveChatSession(s.CfgStore.Cfg, *cs); err != nil {
		s.endChatRun(sid)
		bad(w, http.StatusInternalServerError, err.Error())
		return true
	}
	s.publishChatRun(sid, map[string]interface{}{"type": "message", "message": msg, "workspace": cs.Workspace, "project_mode": cs.ProjectMode})
	s.endChatRun(sid)
	s.streamChatRun(w, r, sid, 0)
	return true
}

func (s *Server) chatBTW(w http.ResponseWriter, r *http.Request, sid string) {
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "/btw" || (!strings.HasPrefix(prompt, "/btw ") && !strings.HasPrefix(prompt, "/btw\t")) {
		bad(w, http.StatusBadRequest, "a non-empty /btw question is required")
		return
	}
	sid = safeChatID(sid)
	s.SessionMu.Lock()
	cs, err := loadChatSession(s.CfgStore.Cfg, sid)
	s.SessionMu.Unlock()
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	cmdReq := map[string]interface{}{
		"op":               "btw",
		"prompt":           prompt,
		"history":          cs.Messages,
		"raw_history":      cs.RawHistory,
		"workspace":        cs.Workspace,
		"project_mode":     cs.ProjectMode,
		"llm_no":           cs.Settings.LLMNo,
		"tools_mode":       cs.Settings.ToolsMode,
		"reasoning_effort": cs.Settings.ReasoningEffort,
		"ga_root":          s.CfgStore.Cfg.GARoot,
	}
	msg, err := runOneShotBTWWorkerFunc(s.CfgStore.Cfg, sid, cmdReq)
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.SessionMu.Lock()
	latest, loadErr := loadChatSession(s.CfgStore.Cfg, sid)
	if loadErr == nil {
		latest.Messages = mergeChatMessageLists(latest.Messages, []chatMessage{msg})
		latest.UpdatedAt = time.Now().Unix()
		loadErr = saveChatSession(s.CfgStore.Cfg, latest)
	}
	s.SessionMu.Unlock()
	if loadErr != nil {
		bad(w, http.StatusInternalServerError, loadErr.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "message": msg})
}

func (s *Server) chatPost(w http.ResponseWriter, r *http.Request, sid string) {
	var req struct {
		Prompt              string        `json:"prompt"`
		Files               []chatUpload  `json:"files"`
		Settings            *chatSettings `json:"settings"`
		ClientUserID        string        `json:"client_user_id"`
		SourceUserMessageID string        `json:"source_user_message_id"`
	}
	if err := decodeLimited(r, &req, maxChatPostBodyBytes); err != nil {
		bad(w, 400, err.Error())
		return
	}
	sid = safeChatID(sid)
	cmd, immediate, parseErr := parseImmediateChatCommand(req.Prompt)
	if parseErr != nil {
		bad(w, http.StatusBadRequest, parseErr.Error())
		return
	}
	if immediate && commandNeedsDanger(cmd) && !requireDangerousHeader(w, r) {
		return
	}
	token := s.beginChatRun(sid)
	if token == nil {
		bad(w, 409, "chat is already running")
		return
	}
	if immediate {
		s.maybeHandleImmediateChatCommand(w, r, sid, token, cmd)
		return
	}
	cs, err := loadChatSession(s.CfgStore.Cfg, sid)
	if err != nil {
		s.endChatRunOwned(sid, token)
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
	if s.maybeHandleProjectCommand(w, r, sid, &cs, req.Prompt) {
		return
	}
	if sourceID := strings.TrimSpace(req.SourceUserMessageID); sourceID != "" {
		if len(req.Files) > 0 {
			s.endChatRunOwned(sid, token)
			bad(w, http.StatusBadRequest, "worldline edit/resend does not accept new attachments")
			return
		}
		if err := s.prepareChatWorldlineResend(sid, token, &cs, sourceID); err != nil {
			s.endChatRunOwned(sid, token)
			bad(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	saved, refs, err := saveChatUploads(s.CfgStore.Cfg, req.Files)
	if err != nil {
		s.endChatRunOwned(sid, token)
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
	var saveErr error
	if strings.TrimSpace(req.SourceUserMessageID) != "" {
		saveErr = s.saveChatSessionExact(cs)
	} else {
		saveErr = s.saveChatSessionMerged(cs)
	}
	if saveErr != nil {
		s.endChatRunOwned(sid, token)
		bad(w, http.StatusInternalServerError, saveErr.Error())
		return
	}
	s.publishChatRun(sid, map[string]interface{}{"type": "user", "message": userMsg})
	workerHistory := append([]chatMessage(nil), cs.Messages[:len(cs.Messages)-1]...)
	cmdReq := map[string]interface{}{
		"prompt":               display,
		"history":              workerHistory,
		"raw_history":          cs.RawHistory,
		"history_info":         cs.HistoryInfo,
		"working":              cs.Working,
		"workspace":            cs.Workspace,
		"project_mode":         cs.ProjectMode,
		"llm_no":               cs.Settings.LLMNo,
		"tools_mode":           cs.Settings.ToolsMode,
		"reasoning_effort":     cs.Settings.ReasoningEffort,
		"ga_root":              s.CfgStore.Cfg.GARoot,
		"_ga_worldline_resend": strings.TrimSpace(req.SourceUserMessageID) != "",
	}
	go s.runChatWorkerOwned(sid, token, cs, cmdReq)
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
	var token *chatRun
	s.ChatMu.Lock()
	run := s.ChatRuns[sid]
	if run == nil || run.Done {
		s.ChatMu.Unlock()
		writeJSON(w, map[string]interface{}{"ok": true, "running": false})
		return
	}
	run.Canceled = true
	token = run
	cmd = run.Cmd
	worker = s.ChatWorkers[sid]
	s.ChatMu.Unlock()
	// Immediate commands do not have a worker process to kill. End their run here
	// so cancellation always releases SSE subscribers and the session reservation.
	s.endChatRunOwned(sid, token)
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
