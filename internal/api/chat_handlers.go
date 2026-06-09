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
		items = append(items, map[string]interface{}{"id": cs.ID, "title": cs.Title, "updated_at": cs.UpdatedAt, "count": len(cs.Messages), "running": s.chatRunActive(cs.ID)})
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
	cs := chatSession{ID: newChatID(), Title: "新会话", UpdatedAt: time.Now().Unix(), Messages: []chatMessage{}, Settings: normalizeChatSettings(chatSettings{})}
	if err := saveChatSession(s.CfgStore.Cfg, cs); err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, cs)
}

func (s *Server) chatGetSession(w http.ResponseWriter, r *http.Request, sid string) {
	cs, err := loadChatSession(s.CfgStore.Cfg, safeChatID(sid))
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, cs)
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
	writeJSON(w, cs)
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
	writeJSON(w, map[string]interface{}{"settings": cs.Settings, "llm_no": cs.Settings.LLMNo, "llms": llms, "backend": backend, "running": running})
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
		"prompt":     display,
		"history":    workerHistory,
		"llm_no":     cs.Settings.LLMNo,
		"tools_mode": cs.Settings.ToolsMode,
		"ga_root":    s.CfgStore.Cfg.GARoot,
	}
	s.NotifyPetEvent("chat:start")
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
	s.NotifyPetEvent("chat:cancel")
	writeJSON(w, map[string]interface{}{"ok": true, "running": false})
}

func (s *Server) chatFile(w http.ResponseWriter, r *http.Request, name string) {
	http.ServeFile(w, r, filepath.Join(chatUploadDir(s.CfgStore.Cfg), filepath.Base(name)))
}
