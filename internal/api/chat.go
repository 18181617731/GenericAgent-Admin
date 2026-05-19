package api

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type chatMessage struct {
	ID        string                   `json:"id"`
	Role      string                   `json:"role"`
	Content   string                   `json:"content"`
	Files     []map[string]interface{} `json:"files,omitempty"`
	CreatedAt int64                    `json:"created_at"`
	Error     bool                     `json:"error,omitempty"`
}

type chatSettings struct {
	LLMNo int `json:"llm_no"`
}
type chatSession struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	UpdatedAt int64         `json:"updated_at"`
	Messages  []chatMessage `json:"messages"`
	Settings  chatSettings  `json:"settings"`
}

type chatUpload struct{ Name, Type, DataURL string }

type chatRun struct {
	SID         string
	Events      [][]byte
	Done        bool
	Canceled    bool
	Cmd         *exec.Cmd
	Subscribers map[chan []byte]bool
}

type chatWorker struct {
	SID    string
	Cmd    *exec.Cmd
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
	Dead   bool
}

func (s *Server) chatSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	items := []map[string]interface{}{}
	_ = os.MkdirAll(chatSessionDir(s.CfgStore.Cfg.GARoot), 0755)
	entries, _ := os.ReadDir(chatSessionDir(s.CfgStore.Cfg.GARoot))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		cs, err := loadChatSession(s.CfgStore.Cfg.GARoot, strings.TrimSuffix(e.Name(), ".json"))
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
	cs := chatSession{ID: newChatID(), Title: "新会话", UpdatedAt: time.Now().Unix(), Messages: []chatMessage{}, Settings: chatSettings{}}
	if err := saveChatSession(s.CfgStore.Cfg.GARoot, cs); err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, cs)
}
func (s *Server) chatGetSession(w http.ResponseWriter, r *http.Request, sid string) {
	cs, err := loadChatSession(s.CfgStore.Cfg.GARoot, safeChatID(sid))
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
	cs, err := loadChatSession(s.CfgStore.Cfg.GARoot, safeChatID(sid))
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	cs.Title = title
	cs.UpdatedAt = time.Now().Unix()
	if err := saveChatSession(s.CfgStore.Cfg.GARoot, cs); err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, cs)
}
func (s *Server) chatDeleteSession(w http.ResponseWriter, r *http.Request, sid string) {
	_ = os.Remove(chatSessionPath(s.CfgStore.Cfg.GARoot, safeChatID(sid)))
	writeJSON(w, map[string]bool{"ok": true})
}
func (s *Server) chatSaveSettings(w http.ResponseWriter, r *http.Request, sid string) {
	var st chatSettings
	_ = decode(r, &st)
	cs, _ := loadChatSession(s.CfgStore.Cfg.GARoot, safeChatID(sid))
	cs.Settings = st
	_ = saveChatSession(s.CfgStore.Cfg.GARoot, cs)
	writeJSON(w, map[string]interface{}{"ok": true, "settings": st})
}
func (s *Server) chatState(w http.ResponseWriter, r *http.Request, sid string) {
	cs, _ := loadChatSession(s.CfgStore.Cfg.GARoot, safeChatID(sid))
	llms, err := listGARuntimeLLMs(s.CfgStore.Cfg.GARoot)
	backend := map[string]string{"class": "GenericAgent worker", "source": "agentmain.GenericAgent.list_llms"}
	if err != nil {
		backend["warning"] = err.Error()
	}
	running := s.chatRunActive(sid)
	writeJSON(w, map[string]interface{}{"settings": cs.Settings, "llm_no": cs.Settings.LLMNo, "llms": llms, "backend": backend, "running": running})
}

func (s *Server) chatPost(w http.ResponseWriter, r *http.Request, sid string) {
	var req struct {
		Prompt       string       `json:"prompt"`
		Files        []chatUpload `json:"files"`
		Settings     chatSettings `json:"settings"`
		ClientUserID string       `json:"client_user_id"`
	}
	if err := decode(r, &req); err != nil {
		bad(w, 400, "bad request")
		return
	}
	sid = safeChatID(sid)
	if !s.beginChatRun(sid) {
		bad(w, 409, "chat is already running")
		return
	}
	cs, _ := loadChatSession(s.CfgStore.Cfg.GARoot, sid)
	if cs.ID == "" {
		cs.ID = sid
		cs.Title = "新会话"
	}
	if req.Settings.LLMNo != 0 {
		cs.Settings = req.Settings
	}
	saved, refs, err := saveChatUploads(s.CfgStore.Cfg.GARoot, req.Files)
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
	_ = saveChatSession(s.CfgStore.Cfg.GARoot, cs)
	s.publishChatRun(sid, map[string]interface{}{"type": "user", "message": userMsg})
	workerPrompt := buildPromptWithHistory(display, cs.Messages)
	cmdReq := map[string]interface{}{"prompt": workerPrompt, "llm_no": cs.Settings.LLMNo, "ga_root": s.CfgStore.Cfg.GARoot}
	go s.runChatWorker(sid, cs, cmdReq)
	s.streamChatRun(w, r, sid, 0)
}

func (s *Server) runChatWorker(sid string, cs chatSession, cmdReq map[string]interface{}) {
	worker, err := s.getChatWorker(sid)
	if err != nil {
		msg := chatMessage{ID: newChatID(), Role: "assistant", Content: fmt.Sprintf("提交失败：%v", err), CreatedAt: time.Now().Unix(), Error: true}
		cs.Messages = append(cs.Messages, msg)
		_ = saveChatSession(s.CfgStore.Cfg.GARoot, cs)
		s.publishChatRun(sid, map[string]interface{}{"type": "error", "message": msg})
		s.endChatRun(sid)
		return
	}
	s.setChatRunCmd(sid, worker.Cmd)
	if err := json.NewEncoder(worker.Stdin).Encode(cmdReq); err != nil {
		s.dropChatWorker(sid, worker)
		msg := chatMessage{ID: newChatID(), Role: "assistant", Content: fmt.Sprintf("提交失败：%v", err), CreatedAt: time.Now().Unix(), Error: true}
		cs.Messages = append(cs.Messages, msg)
		_ = saveChatSession(s.CfgStore.Cfg.GARoot, cs)
		s.publishChatRun(sid, map[string]interface{}{"type": "error", "message": msg})
		s.endChatRun(sid)
		return
	}
	scanner := bufio.NewScanner(worker.Stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var final chatMessage
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev map[string]interface{}
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		if msg, ok := ev["message"].(map[string]interface{}); ok && (ev["type"] == "done" || ev["type"] == "error") {
			b, _ := json.Marshal(msg)
			_ = json.Unmarshal(b, &final)
			s.publishChatLine(sid, line)
			break
		}
		s.publishChatLine(sid, line)
	}
	if final.ID == "" {
		if s.chatRunCanceled(sid) {
			final = chatMessage{ID: newChatID(), Role: "assistant", Content: "已停止生成", CreatedAt: time.Now().Unix(), Error: true}
			s.publishChatRun(sid, map[string]interface{}{"type": "error", "message": final})
		} else {
			err := scanner.Err()
			if err == nil {
				err = fmt.Errorf("worker exited before done")
			}
			s.dropChatWorker(sid, worker)
			final = chatMessage{ID: newChatID(), Role: "assistant", Content: fmt.Sprintf("生成失败：%v", err), CreatedAt: time.Now().Unix(), Error: true}
			s.publishChatRun(sid, map[string]interface{}{"type": "error", "message": final})
		}
	}
	cs.Messages = append(cs.Messages, final)
	cs.UpdatedAt = time.Now().Unix()
	_ = saveChatSession(s.CfgStore.Cfg.GARoot, cs)
	s.endChatRun(sid)
}

func (s *Server) chatStream(w http.ResponseWriter, r *http.Request, sid string) {
	from := 0
	if v := strings.TrimSpace(r.URL.Query().Get("from")); v != "" {
		_, _ = fmt.Sscanf(v, "%d", &from)
	}
	s.streamChatRun(w, r, safeChatID(sid), from)
}

func (s *Server) chatRunActive(sid string) bool {
	s.ChatMu.Lock()
	defer s.ChatMu.Unlock()
	r := s.ChatRuns[safeChatID(sid)]
	return r != nil && !r.Done
}

func (s *Server) beginChatRun(sid string) bool {
	s.ChatMu.Lock()
	defer s.ChatMu.Unlock()
	if s.ChatRuns == nil {
		s.ChatRuns = map[string]*chatRun{}
	}
	if r := s.ChatRuns[sid]; r != nil && !r.Done {
		return false
	}
	s.ChatRuns[sid] = &chatRun{SID: sid, Subscribers: map[chan []byte]bool{}}
	return true
}

func (s *Server) setChatRunCmd(sid string, cmd *exec.Cmd) {
	s.ChatMu.Lock()
	if r := s.ChatRuns[safeChatID(sid)]; r != nil {
		r.Cmd = cmd
	}
	s.ChatMu.Unlock()
}

func (s *Server) chatRunCanceled(sid string) bool {
	s.ChatMu.Lock()
	defer s.ChatMu.Unlock()
	r := s.ChatRuns[safeChatID(sid)]
	return r != nil && r.Canceled
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

func (s *Server) publishChatRun(sid string, ev map[string]interface{}) {
	b, _ := json.Marshal(ev)
	s.publishChatLine(sid, b)
}

func (s *Server) publishChatLine(sid string, line []byte) {
	s.ChatMu.Lock()
	defer s.ChatMu.Unlock()
	r := s.ChatRuns[sid]
	if r == nil {
		return
	}
	b := append([]byte(nil), line...)
	r.Events = append(r.Events, b)
	for ch := range r.Subscribers {
		select {
		case ch <- b:
		default:
		}
	}
}

func (s *Server) endChatRun(sid string) {
	s.ChatMu.Lock()
	r := s.ChatRuns[sid]
	if r != nil && !r.Done {
		r.Done = true
		for ch := range r.Subscribers {
			close(ch)
		}
		r.Subscribers = map[chan []byte]bool{}
	}
	s.ChatMu.Unlock()
	go func() {
		time.Sleep(5 * time.Minute)
		s.ChatMu.Lock()
		if rr := s.ChatRuns[sid]; rr != nil && rr.Done {
			delete(s.ChatRuns, sid)
		}
		s.ChatMu.Unlock()
	}()
}

func (s *Server) streamChatRun(w http.ResponseWriter, r *http.Request, sid string, from int) {
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	flusher, _ := w.(http.Flusher)
	s.ChatMu.Lock()
	run := s.ChatRuns[sid]
	if run == nil {
		s.ChatMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if from < 0 {
		from = 0
	}
	if from > len(run.Events) {
		from = len(run.Events)
	}
	initial := append([][]byte(nil), run.Events[from:]...)
	ch := make(chan []byte, 128)
	if !run.Done {
		run.Subscribers[ch] = true
	}
	done := run.Done
	s.ChatMu.Unlock()
	for _, line := range initial {
		_, _ = w.Write(append(append([]byte(nil), line...), '\n'))
		if flusher != nil {
			flusher.Flush()
		}
	}
	if done {
		return
	}
	defer func() {
		s.ChatMu.Lock()
		if rr := s.ChatRuns[sid]; rr != nil && rr.Subscribers != nil {
			delete(rr.Subscribers, ch)
		}
		s.ChatMu.Unlock()
	}()
	for {
		select {
		case line, ok := <-ch:
			if !ok {
				return
			}
			_, _ = w.Write(append(append([]byte(nil), line...), '\n'))
			if flusher != nil {
				flusher.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) finishChatError(w http.ResponseWriter, enc *json.Encoder, flusher http.Flusher, cs *chatSession, err error) {
	msg := chatMessage{ID: newChatID(), Role: "assistant", Content: fmt.Sprintf("提交失败：%v", err), CreatedAt: time.Now().Unix(), Error: true}
	cs.Messages = append(cs.Messages, msg)
	_ = saveChatSession(s.CfgStore.Cfg.GARoot, *cs)
	_ = enc.Encode(map[string]interface{}{"type": "error", "message": msg})
	if flusher != nil {
		flusher.Flush()
	}
}

func listGARuntimeLLMs(root string) ([]map[string]interface{}, error) {
	py := pythonForRoot(root)
	code := `import json, os, sys
root = sys.argv[1]
if root not in sys.path:
    sys.path.insert(0, root)
os.chdir(root)
from agentmain import GenericAgent
agent = GenericAgent()
items = []
for idx, label, active in agent.list_llms():
    text = str(label)
    name = text.split('/', 1)[1] if '/' in text else text
    model = text.rsplit('/', 1)[1] if '/' in text else ''
    items.append({'index': int(idx), 'label': text, 'name': name, 'model': model, 'active': bool(active)})
print(json.dumps(items, ensure_ascii=False))`
	cmd := exec.Command(py, "-c", code, root)
	cmd.Dir = root
	hideChildWindow(cmd)
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1", "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return []map[string]interface{}{}, fmt.Errorf("list GA LLMs failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	clean := bytes.TrimSpace(out)
	if i := bytes.LastIndex(clean, []byte("[")); i >= 0 {
		clean = clean[i:]
	}
	var llms []map[string]interface{}
	if err := json.Unmarshal(clean, &llms); err != nil {
		return []map[string]interface{}{}, fmt.Errorf("parse GA LLMs failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return llms, nil
}

func (s *Server) getChatWorker(sid string) (*chatWorker, error) {
	sid = safeChatID(sid)
	s.ChatMu.Lock()
	if s.ChatWorkers == nil {
		s.ChatWorkers = map[string]*chatWorker{}
	}
	if w := s.ChatWorkers[sid]; w != nil && !w.Dead && w.Cmd != nil && w.Cmd.Process != nil {
		s.ChatMu.Unlock()
		return w, nil
	}
	s.ChatMu.Unlock()
	worker, err := startChatWorker(s.CfgStore.Cfg.GARoot, sid)
	if err != nil {
		return nil, err
	}
	s.ChatMu.Lock()
	if s.ChatWorkers == nil {
		s.ChatWorkers = map[string]*chatWorker{}
	}
	s.ChatWorkers[sid] = worker
	s.ChatMu.Unlock()
	return worker, nil
}

func (s *Server) dropChatWorker(sid string, worker *chatWorker) {
	sid = safeChatID(sid)
	s.ChatMu.Lock()
	if s.ChatWorkers[sid] == worker {
		delete(s.ChatWorkers, sid)
	}
	if worker != nil {
		worker.Dead = true
	}
	s.ChatMu.Unlock()
	if worker != nil && worker.Cmd != nil && worker.Cmd.Process != nil {
		_ = worker.Cmd.Process.Kill()
		_, _ = worker.Cmd.Process.Wait()
	}
}

func startChatWorker(root string, sid string) (*chatWorker, error) {
	py := pythonForRoot(root)
	script, err := resolveChatWorkerScript()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(py, script)
	cmd.Dir = root
	hideChildWindow(cmd)
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1", "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8", "GA_ROOT="+root)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	worker := &chatWorker{SID: sid, Cmd: cmd, Stdin: stdin, Stdout: stdout, Stderr: stderr}
	go io.Copy(io.Discard, stderr)
	return worker, nil
}

func resolveChatWorkerScript() (string, error) {
	candidates := []string{}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "cmd", "chat_worker.py"))
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "cmd", "chat_worker.py"))
		candidates = append(candidates, filepath.Join(filepath.Dir(filepath.Dir(exe)), "cmd", "chat_worker.py"))
	}
	for _, script := range candidates {
		if st, err := os.Stat(script); err == nil && !st.IsDir() {
			return script, nil
		}
	}
	return "", fmt.Errorf("chat_worker.py not found; checked: %s", strings.Join(candidates, "; "))
}

func mustGetwd() string { wd, _ := os.Getwd(); return wd }
func safeChatID(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	for _, c := range v {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return newChatID()
		}
	}
	return v
}
func chatSessionDir(root string) string {
	return filepath.Join(root, "temp", "react_frontend_sessions")
}
func chatUploadDir(root string) string { return filepath.Join(root, "temp", "react_frontend_uploads") }
func chatSessionPath(root, sid string) string {
	return filepath.Join(chatSessionDir(root), safeChatID(sid)+".json")
}
func loadChatSession(root, sid string) (chatSession, error) {
	sid = safeChatID(sid)
	cs := chatSession{ID: sid, Title: "新会话", Messages: []chatMessage{}, Settings: chatSettings{}}
	b, err := os.ReadFile(chatSessionPath(root, sid))
	if err != nil {
		return cs, nil
	}
	_ = json.Unmarshal(b, &cs)
	if cs.ID == "" {
		cs.ID = sid
	}
	if cs.Messages == nil {
		cs.Messages = []chatMessage{}
	}
	return cs, nil
}
func saveChatSession(root string, cs chatSession) error {
	_ = os.MkdirAll(chatSessionDir(root), 0755)
	cs.UpdatedAt = time.Now().Unix()
	b, _ := json.MarshalIndent(cs, "", "  ")
	return os.WriteFile(chatSessionPath(root, cs.ID), b, 0644)
}
func updateChatTitle(cs *chatSession) {
	if cs.Title != "" && cs.Title != "新会话" {
		return
	}
	for _, m := range cs.Messages {
		if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			t := strings.Split(strings.TrimSpace(m.Content), "\n")[0]
			if len([]rune(t)) > 64 {
				t = string([]rune(t)[:64])
			}
			cs.Title = t
			return
		}
	}
}

func saveChatUploads(root string, files []chatUpload) ([]map[string]interface{}, []string, error) {
	if len(files) == 0 {
		return nil, nil, nil
	}
	_ = os.MkdirAll(chatUploadDir(root), 0755)
	var saved []map[string]interface{}
	var refs []string
	for _, f := range files {
		name := filepath.Base(f.Name)
		if name == "." || name == string(filepath.Separator) || name == "" {
			name = "upload.bin"
		}
		data := f.DataURL
		if i := strings.Index(data, ","); i >= 0 {
			data = data[i+1:]
		}
		raw, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, nil, fmt.Errorf("decode %s: %w", name, err)
		}
		name = fmt.Sprintf("%d_%s", time.Now().UnixNano(), name)
		target := filepath.Join(chatUploadDir(root), name)
		if err := os.WriteFile(target, raw, 0644); err != nil {
			return nil, nil, err
		}
		meta := map[string]interface{}{"path": target, "name": name, "mime": f.Type, "url": "/api/chat/file/" + name}
		saved = append(saved, meta)
		refs = append(refs, "[FILE:"+target+"]")
	}
	return saved, refs, nil
}

func (s *Server) chatFile(w http.ResponseWriter, r *http.Request, name string) {
	http.ServeFile(w, r, filepath.Join(chatUploadDir(s.CfgStore.Cfg.GARoot), filepath.Base(name)))
}

func newChatID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:4]) + "-" + hex.EncodeToString(b[4:6]) + "-" + hex.EncodeToString(b[6:8]) + "-" + hex.EncodeToString(b[8:10]) + "-" + hex.EncodeToString(b[10:])
}

func chatMessageLabel(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant":
		return "ASSISTANT"
	case "system":
		return "SYSTEM"
	default:
		return "USER"
	}
}

func compactChatText(v string, limit int) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	v = strings.Join(strings.Fields(v), " ")
	r := []rune(v)
	if len(r) > limit {
		return string(r[:limit]) + "..."
	}
	return v
}

func buildPromptWithHistory(prompt string, messages []chatMessage) string {
	prompt = strings.TrimSpace(prompt)
	if len(messages) <= 1 {
		return prompt
	}
	previous := []string{}
	// chatPost appends the current user message before building the worker prompt.
	for _, msg := range messages[:len(messages)-1] {
		if msg.Error {
			continue
		}
		label := chatMessageLabel(msg.Role)
		limit := 3000
		if label == "ASSISTANT" {
			limit = 5000
		}
		content := compactChatText(msg.Content, limit)
		if content != "" {
			previous = append(previous, fmt.Sprintf("[%s]: %s", label, content))
		}
	}
	if len(previous) == 0 {
		return prompt
	}
	if len(previous) > 24 {
		previous = previous[len(previous)-24:]
	}
	text := strings.Join(previous, "\n\n")
	textRunes := []rune(text)
	if len(textRunes) > 28000 {
		text = "...[older history omitted]\n" + string(textRunes[len(textRunes)-28000:])
	}
	return "以下是当前会话的历史上下文，请在回答时延续这些上下文，不要把它当作用户的新问题。\n" +
		"<history>\n" + text + "\n</history>\n\n" +
		"### 用户当前消息\n" + prompt
}

// CloseChatWorkers terminates all persistent chat worker child processes.
func (s *Server) CloseChatWorkers() {
	if s == nil {
		return
	}
	var workers []*chatWorker
	s.ChatMu.Lock()
	for sid, w := range s.ChatWorkers {
		if w != nil {
			workers = append(workers, w)
		}
		delete(s.ChatWorkers, sid)
	}
	s.ChatMu.Unlock()
	for _, w := range workers {
		if w.Cmd != nil && w.Cmd.Process != nil {
			_ = w.Cmd.Process.Kill()
			_, _ = w.Cmd.Process.Wait()
		}
	}
}
