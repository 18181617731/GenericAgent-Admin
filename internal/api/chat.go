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
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"genericagent-admin-go/internal/config"
)

type chatMessage struct {
	ID        string                   `json:"id"`
	Role      string                   `json:"role"`
	Content   string                   `json:"content"`
	Files     []map[string]interface{} `json:"files,omitempty"`
	CreatedAt int64                    `json:"created_at"`
	Error     bool                     `json:"error,omitempty"`
}

const (
	chatToolsModeOfficial = "official"
	chatToolsModeFixed    = "fixed"
)

type chatSettings struct {
	LLMNo     int    `json:"llm_no"`
	ToolsMode string `json:"tools_mode,omitempty"`
}

func normalizeChatSettings(st chatSettings) chatSettings {
	switch st.ToolsMode {
	case chatToolsModeFixed:
		// keep
	default:
		st.ToolsMode = chatToolsModeOfficial
	}
	return st
}

type chatSession struct {
	ID          string                   `json:"id"`
	Title       string                   `json:"title"`
	UpdatedAt   int64                    `json:"updated_at"`
	Messages    []chatMessage            `json:"messages"`
	Settings    chatSettings             `json:"settings"`
	RawHistory  []map[string]interface{} `json:"raw_history,omitempty"`
	HistoryInfo []interface{}            `json:"history_info,omitempty"`
	Working     map[string]interface{}   `json:"working,omitempty"`
}

const (
	maxChatUploadFiles        = 8
	maxChatUploadBytesPerFile = 20 << 20
	maxChatUploadBytesTotal   = 40 << 20
	// maxChatPostBodyBytes must accommodate base64-encoded uploads (which inflate
	// raw bytes by ~4/3) plus prompt text and per-file metadata, so it is set well
	// above maxChatUploadBytesTotal. The decoded raw size is still capped by
	// saveChatUploads, so this only governs the transport payload size.
	maxChatPostBodyBytes = 64 << 20
	// Worker stdout is NDJSON, but a single final/error event can contain a large
	// assistant answer. bufio.Scanner hard-limits tokens unless configured and
	// drops data above that limit, so runChatWorker uses readChatWorkerLine instead.
	maxChatWorkerLineBytes = 128 << 20
)

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
	Mu     sync.Mutex
}

func (s *Server) runChatWorker(sid string, cs chatSession, cmdReq map[string]interface{}) {
	worker, err := s.getChatWorker(sid)
	if err != nil {
		msg := chatMessage{ID: newChatID(), Role: "assistant", Content: fmt.Sprintf("提交失败：%v", err), CreatedAt: time.Now().Unix(), Error: true}
		cs.Messages = append(cs.Messages, msg)
		_ = saveChatSession(s.CfgStore.Cfg, cs)
		s.publishChatRun(sid, map[string]interface{}{"type": "error", "message": msg})
		s.NotifyPetEvent("chat:error")
		s.endChatRun(sid)
		return
	}
	s.setChatRunCmd(sid, worker.Cmd)
	worker.Mu.Lock()
	defer worker.Mu.Unlock()
	if err := json.NewEncoder(worker.Stdin).Encode(cmdReq); err != nil {
		s.dropChatWorker(sid, worker)
		msg := chatMessage{ID: newChatID(), Role: "assistant", Content: fmt.Sprintf("提交失败：%v", err), CreatedAt: time.Now().Unix(), Error: true}
		cs.Messages = append(cs.Messages, msg)
		_ = saveChatSession(s.CfgStore.Cfg, cs)
		s.publishChatRun(sid, map[string]interface{}{"type": "error", "message": msg})
		s.NotifyPetEvent("chat:error")
		s.endChatRun(sid)
		return
	}
	reader := bufio.NewReaderSize(worker.Stdout, 64*1024)
	var final chatMessage
	var finalRawHistory []map[string]interface{}
	var finalHistoryInfo []interface{}
	var finalWorking map[string]interface{}
	var readErr error
	for {
		line, err := readChatWorkerLine(reader)
		if len(bytes.TrimSpace(line)) == 0 {
			if err != nil {
				readErr = err
				break
			}
			continue
		}
		line = bytes.TrimSpace(line)
		var ev map[string]interface{}
		if json.Unmarshal(line, &ev) != nil {
			if err != nil {
				readErr = err
				break
			}
			continue
		}
		if msg, ok := ev["message"].(map[string]interface{}); ok && (ev["type"] == "done" || ev["type"] == "error") {
			b, _ := json.Marshal(msg)
			_ = json.Unmarshal(b, &final)
			finalRawHistory = chatRawHistoryFromEvent(ev)
			finalHistoryInfo = chatHistoryInfoFromEvent(ev)
			finalWorking = chatWorkingFromEvent(ev)
			delete(ev, "raw_history")
			delete(ev, "history_info")
			delete(ev, "working")
			if cleanLine, err := json.Marshal(ev); err == nil {
				s.publishChatLine(sid, cleanLine)
			} else {
				s.publishChatLine(sid, line)
			}
			break
		}
		s.publishChatLine(sid, line)
		if err != nil {
			readErr = err
			break
		}
	}
	if final.ID == "" {
		partial := s.chatRunPartialContent(sid)
		if s.chatRunCanceled(sid) {
			content := strings.TrimSpace(partial)
			if content != "" {
				content += "\n\n[已中止生成]"
			} else {
				content = "已停止生成"
			}
			final = chatMessage{ID: newChatID(), Role: "assistant", Content: content, CreatedAt: time.Now().Unix(), Error: true}
			s.publishChatRun(sid, map[string]interface{}{"type": "error", "message": final})
		} else {
			err := readErr
			if err == nil || err == io.EOF {
				err = fmt.Errorf("worker exited before done")
			}
			s.dropChatWorker(sid, worker)
			content := strings.TrimSpace(partial)
			if content != "" {
				content += fmt.Sprintf("\n\n[生成中断：%v]", err)
			} else {
				content = fmt.Sprintf("生成失败：%v", err)
			}
			final = chatMessage{ID: newChatID(), Role: "assistant", Content: content, CreatedAt: time.Now().Unix(), Error: true}
			s.publishChatRun(sid, map[string]interface{}{"type": "error", "message": final})
		}
	}
	var fallbackMessages []chatMessage
	if len(cs.Messages) > 0 {
		fallbackMessages = append(fallbackMessages, cs.Messages[len(cs.Messages)-1])
	}
	fallbackMessages = append(fallbackMessages, final)
	cs.Messages = append(cs.Messages, final)
	if len(finalRawHistory) > 0 {
		cs.RawHistory = finalRawHistory
	} else {
		cs.RawHistory = appendChatRawHistoryFallback(cs.RawHistory, fallbackMessages...)
	}
	if finalHistoryInfo != nil {
		cs.HistoryInfo = finalHistoryInfo
	}
	if finalWorking != nil {
		cs.Working = finalWorking
	}
	cs.UpdatedAt = time.Now().Unix()
	_ = saveChatSession(s.CfgStore.Cfg, cs)
	if final.Error {
		s.NotifyPetEvent("chat:error")
	} else {
		s.NotifyPetEvent("chat:done")
	}
	s.endChatRun(sid)
}

func chatRawHistoryFromEvent(ev map[string]interface{}) []map[string]interface{} {
	items, ok := ev["raw_history"].([]interface{})
	if !ok || len(items) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func chatHistoryInfoFromEvent(ev map[string]interface{}) []interface{} {
	items, ok := ev["history_info"].([]interface{})
	if !ok {
		return nil
	}
	return append([]interface{}(nil), items...)
}

func chatWorkingFromEvent(ev map[string]interface{}) map[string]interface{} {
	m, ok := ev["working"].(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func appendChatRawHistoryFallback(raw []map[string]interface{}, messages ...chatMessage) []map[string]interface{} {
	out := append([]map[string]interface{}(nil), raw...)
	for _, msg := range messages {
		text := strings.TrimSpace(msg.Content)
		if text == "" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "assistant" && role != "system" {
			role = "user"
		}
		out = append(out, map[string]interface{}{
			"role": role,
			"content": []map[string]interface{}{{
				"type": "text",
				"text": text,
			}},
		})
	}
	return out
}

func chatSessionForClient(cs chatSession) chatSession {
	return cs
}

func (s *Server) chatRunActive(sid string) bool {
	s.ChatMu.Lock()
	defer s.ChatMu.Unlock()
	r := s.ChatRuns[safeChatID(sid)]
	return r != nil && !r.Done
}

func (s *Server) reinjectChatWorkerTools(sid string) (map[string]interface{}, error) {
	sid = safeChatID(sid)
	worker, err := s.getChatWorker(sid)
	if err != nil {
		return nil, err
	}
	worker.Mu.Lock()
	defer worker.Mu.Unlock()
	if err := json.NewEncoder(worker.Stdin).Encode(map[string]interface{}{"op": "reinject_tools", "ga_root": s.CfgStore.Cfg.GARoot}); err != nil {
		s.dropChatWorker(sid, worker)
		return nil, err
	}
	reader := bufio.NewReaderSize(worker.Stdout, 64*1024)
	for {
		line, err := readChatWorkerLine(reader)
		if len(bytes.TrimSpace(line)) == 0 {
			if err != nil {
				s.dropChatWorker(sid, worker)
				return nil, err
			}
			continue
		}
		var ev map[string]interface{}
		if json.Unmarshal(bytes.TrimSpace(line), &ev) == nil {
			if ev["type"] == "reinject_tools" {
				return ev, nil
			}
			if ev["type"] == "error" {
				return ev, nil
			}
		}
		if err != nil {
			s.dropChatWorker(sid, worker)
			return nil, err
		}
	}
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

func (s *Server) chatRunPartialContent(sid string) string {
	s.ChatMu.Lock()
	r := s.ChatRuns[safeChatID(sid)]
	var events [][]byte
	if r != nil {
		events = append(events, r.Events...)
	}
	s.ChatMu.Unlock()
	var b strings.Builder
	for _, line := range events {
		var ev map[string]interface{}
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		if delta, ok := ev["delta"].(string); ok && delta != "" {
			b.WriteString(delta)
		}
	}
	return b.String()
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
	_ = saveChatSession(s.CfgStore.Cfg, *cs)
	_ = enc.Encode(map[string]interface{}{"type": "error", "message": msg})
	if flusher != nil {
		flusher.Flush()
	}
}

func chatPythonForConfig(cfg config.AppConfig) string {
	// Chat must honor the Python selected during setup. Falling back to a bare
	// launcher can miss GA dependencies (for example requests) and hide models.
	return resolvePythonForRoot(cfg.GARoot, cfg.PythonPath)
}

func (s *Server) listGARuntimeLLMs(cfg config.AppConfig) ([]map[string]interface{}, error) {
	root := cfg.GARoot
	py := chatPythonForConfig(cfg)
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
	cmd.Env = pythonEnvWithAdminProxy(cfg, "PYTHONUNBUFFERED=1", "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return []map[string]interface{}{}, fmt.Errorf("list GA LLMs failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	clean := bytes.TrimSpace(out)
	llms, parseErr := parseLLMJSONArrayFromMixedOutput(clean)
	if parseErr != nil {
		return []map[string]interface{}{}, fmt.Errorf("parse GA LLMs failed: %v: %s", parseErr, strings.TrimSpace(string(out)))
	}
	return llms, nil
}

func parseLLMJSONArrayFromMixedOutput(out []byte) ([]map[string]interface{}, error) {
	var lastErr error
	for start := bytes.IndexByte(out, '['); start >= 0; {
		var llms []map[string]interface{}
		dec := json.NewDecoder(bytes.NewReader(out[start:]))
		if err := dec.Decode(&llms); err == nil {
			return llms, nil
		} else {
			lastErr = err
		}
		next := bytes.IndexByte(out[start+1:], '[')
		if next < 0 {
			break
		}
		start += next + 1
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no JSON array found")
}

func markChatLLMActive(llms []map[string]interface{}, llmNo int) {
	for _, item := range llms {
		idx, ok := chatLLMIndex(item["index"])
		item["active"] = ok && idx == llmNo
	}
}

func chatLLMIndex(v interface{}) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int8:
		return int(x), true
	case int16:
		return int(x), true
	case int32:
		return int(x), true
	case int64:
		return int(x), true
	case uint:
		return int(x), true
	case uint8:
		return int(x), true
	case uint16:
		return int(x), true
	case uint32:
		return int(x), true
	case uint64:
		return int(x), true
	case float32:
		return int(x), true
	case float64:
		return int(x), true
	case json.Number:
		n, err := x.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(x))
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
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
	worker, err := startChatWorkerFunc(s.CfgStore.Cfg, sid)
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

var startChatWorkerFunc = startChatWorker

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

func startChatWorker(cfg config.AppConfig, sid string) (*chatWorker, error) {
	root := cfg.GARoot
	py := chatPythonForConfig(cfg)
	script, err := resolveChatWorkerScript()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(py, script)
	cmd.Dir = root
	hideChildWindow(cmd)
	cmd.Env = pythonEnvWithAdminProxy(cfg, "PYTHONUNBUFFERED=1", "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8", "GA_ROOT="+root)
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
	go logChatWorkerStderr(sid, stderr)
	return worker, nil
}

func logChatWorkerStderr(sid string, stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			fmt.Fprintf(os.Stderr, "[chat_worker:%s] %s\n", sid, line)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "[chat_worker:%s] stderr read error: %v\n", sid, err)
	}
}

func pythonEnvWithAdminProxy(cfg config.AppConfig, extra ...string) []string {
	proxyKeys := map[string]bool{
		"HTTP_PROXY": true, "HTTPS_PROXY": true, "ALL_PROXY": true, "NO_PROXY": true,
		"http_proxy": true, "https_proxy": true, "all_proxy": true, "no_proxy": true,
	}
	env := []string{}
	for _, kv := range os.Environ() {
		key := kv
		if i := strings.Index(kv, "="); i >= 0 {
			key = kv[:i]
		}
		if proxyKeys[key] && cfg.ProxyMode != "system" {
			continue
		}
		env = append(env, kv)
	}
	if cfg.ProxyMode != "system" {
		env = append(env, "HTTP_PROXY=", "HTTPS_PROXY=", "ALL_PROXY=", "NO_PROXY=", "http_proxy=", "https_proxy=", "all_proxy=", "no_proxy=")
	}
	if cfg.ProxyMode == "custom" {
		if cfg.HTTPProxy != "" {
			env = append(env, "HTTP_PROXY="+cfg.HTTPProxy, "http_proxy="+cfg.HTTPProxy)
		}
		if cfg.HTTPSProxy != "" {
			env = append(env, "HTTPS_PROXY="+cfg.HTTPSProxy, "https_proxy="+cfg.HTTPSProxy)
		}
		if cfg.AllProxy != "" {
			env = append(env, "ALL_PROXY="+cfg.AllProxy, "all_proxy="+cfg.AllProxy)
		}
		if cfg.NoProxy != "" {
			env = append(env, "NO_PROXY="+cfg.NoProxy, "no_proxy="+cfg.NoProxy)
		}
	}
	env = append(env, extra...)
	return env
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
	if _, file, _, ok := runtime.Caller(0); ok {
		// In `go run`, os.Executable() points to a temporary build directory and
		// main changes cwd to that directory. runtime.Caller keeps the source path,
		// so this finds <repo>/cmd/chat_worker.py for development runs.
		candidates = append(candidates, filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(file))), "cmd", "chat_worker.py"))
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

var chatDataMigrationMu sync.Mutex
var chatDataMigrated = map[string]bool{}

func chatDataDir(cfg config.AppConfig) string {
	dir := strings.TrimSpace(cfg.ChatDataDir)
	if dir == "" {
		dir = config.DefaultChatDataDir()
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}
func chatSessionDir(cfg config.AppConfig) string {
	return filepath.Join(chatDataDir(cfg), "chat_sessions")
}
func chatUploadDir(cfg config.AppConfig) string {
	return filepath.Join(chatDataDir(cfg), "chat_uploads")
}
func legacyChatSessionDir(root string) string {
	return filepath.Join(root, "temp", "react_frontend_sessions")
}
func legacyChatUploadDir(root string) string {
	return filepath.Join(root, "temp", "react_frontend_uploads")
}
func chatSessionPath(cfg config.AppConfig, sid string) string {
	return filepath.Join(chatSessionDir(cfg), safeChatID(sid)+".json")
}
func ensureChatDataMigrated(cfg config.AppConfig) error {
	key := cfg.GARoot + "|" + chatDataDir(cfg)
	chatDataMigrationMu.Lock()
	if chatDataMigrated[key] {
		chatDataMigrationMu.Unlock()
		return nil
	}
	chatDataMigrationMu.Unlock()
	if err := copyDirIfTargetEmpty(legacyChatSessionDir(cfg.GARoot), chatSessionDir(cfg)); err != nil {
		return err
	}
	if err := copyDirIfTargetEmpty(legacyChatUploadDir(cfg.GARoot), chatUploadDir(cfg)); err != nil {
		return err
	}
	chatDataMigrationMu.Lock()
	chatDataMigrated[key] = true
	chatDataMigrationMu.Unlock()
	return nil
}
func copyDirIfTargetEmpty(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil || len(entries) == 0 {
		return nil
	}
	if existing, err := os.ReadDir(dst); err == nil && len(existing) > 0 {
		return nil
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		in := filepath.Join(src, e.Name())
		out := filepath.Join(dst, e.Name())
		if _, err := os.Stat(out); err == nil {
			continue
		}
		b, err := os.ReadFile(in)
		if err != nil {
			return err
		}
		if err := writeChatFileAtomic(out, b, 0644); err != nil {
			return err
		}
	}
	return nil
}
func loadChatSession(cfg config.AppConfig, sid string) (chatSession, error) {
	if err := ensureChatDataMigrated(cfg); err != nil {
		return chatSession{}, err
	}
	sid = safeChatID(sid)
	cs := chatSession{ID: sid, Title: "新会话", Messages: []chatMessage{}, Settings: normalizeChatSettings(chatSettings{})}
	b, err := os.ReadFile(chatSessionPath(cfg, sid))
	if err != nil {
		if os.IsNotExist(err) {
			return cs, nil
		}
		return cs, err
	}
	if err := json.Unmarshal(b, &cs); err != nil {
		return cs, err
	}
	if cs.ID == "" {
		cs.ID = sid
	}
	if cs.Messages == nil {
		cs.Messages = []chatMessage{}
	}
	if cs.RawHistory == nil {
		cs.RawHistory = []map[string]interface{}{}
	}
	cs.Settings = normalizeChatSettings(cs.Settings)
	return cs, nil
}
func saveChatSession(cfg config.AppConfig, cs chatSession) error {
	if err := ensureChatDataMigrated(cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(chatSessionDir(cfg), 0755); err != nil {
		return err
	}
	cs.Settings = normalizeChatSettings(cs.Settings)
	cs.UpdatedAt = time.Now().Unix()
	b, _ := json.MarshalIndent(cs, "", "  ")
	return writeChatFileAtomic(chatSessionPath(cfg, cs.ID), b, 0644)
}

func writeChatFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
func readChatWorkerLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > maxChatWorkerLineBytes {
			return line, fmt.Errorf("chat worker line too large: %d > %d bytes", len(line), maxChatWorkerLineBytes)
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		return line, err
	}
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

func saveChatUploads(cfg config.AppConfig, files []chatUpload) (saved []map[string]interface{}, refs []string, err error) {
	if len(files) == 0 {
		return nil, nil, nil
	}
	if len(files) > maxChatUploadFiles {
		return nil, nil, fmt.Errorf("too many upload files: %d > %d", len(files), maxChatUploadFiles)
	}
	if err := ensureChatDataMigrated(cfg); err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(chatUploadDir(cfg), 0755); err != nil {
		return nil, nil, err
	}
	created := []string{}
	defer func() {
		if err == nil {
			return
		}
		for _, path := range created {
			_ = os.Remove(path)
		}
	}()
	totalBytes := 0
	for _, f := range files {
		name := sanitizeChatUploadName(f.Name)
		data := f.DataURL
		if i := strings.Index(data, ","); i >= 0 {
			data = data[i+1:]
		}
		raw, decodeErr := base64.StdEncoding.DecodeString(data)
		if decodeErr != nil {
			return nil, nil, fmt.Errorf("decode %s: %w", name, decodeErr)
		}
		if len(raw) > maxChatUploadBytesPerFile {
			return nil, nil, fmt.Errorf("upload %s too large: %d > %d bytes", name, len(raw), maxChatUploadBytesPerFile)
		}
		totalBytes += len(raw)
		if totalBytes > maxChatUploadBytesTotal {
			return nil, nil, fmt.Errorf("chat uploads too large: %d > %d bytes", totalBytes, maxChatUploadBytesTotal)
		}
		name = fmt.Sprintf("%d_%s", time.Now().UnixNano(), name)
		target := filepath.Join(chatUploadDir(cfg), name)
		if writeErr := writeChatFileAtomic(target, raw, 0644); writeErr != nil {
			return nil, nil, writeErr
		}
		created = append(created, target)
		mime := strings.TrimSpace(f.Type)
		meta := map[string]interface{}{"path": target, "name": name, "mime": mime, "url": "/api/chat/file/" + name}
		saved = append(saved, meta)
		refs = append(refs, chatUploadPromptRef(target, name, mime))
	}
	return saved, refs, nil
}

func chatUploadPromptRef(path, name, mime string) string {
	lowerMime := strings.ToLower(strings.TrimSpace(mime))
	lowerName := strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(lowerMime, "image/") || strings.HasSuffix(lowerName, ".png") || strings.HasSuffix(lowerName, ".jpg") || strings.HasSuffix(lowerName, ".jpeg") || strings.HasSuffix(lowerName, ".gif") || strings.HasSuffix(lowerName, ".webp") || strings.HasSuffix(lowerName, ".bmp") {
		return "[image:" + path + "]"
	}
	return "[FILE:" + path + "]"
}

func sanitizeChatUploadName(name string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "upload.bin"
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|':
			return '_'
		case r < 32:
			return '_'
		default:
			return r
		}
	}, name)
	name = strings.Trim(name, " .")
	if name == "" {
		return "upload.bin"
	}
	return name
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
