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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/modelconfig"
)

type chatMessage struct {
	ID             string                   `json:"id"`
	Role           string                   `json:"role"`
	Content        string                   `json:"content"`
	ModelID        string                   `json:"model_id,omitempty"`
	Files          []map[string]interface{} `json:"files,omitempty"`
	CreatedAt      int64                    `json:"created_at"`
	Error          bool                     `json:"error,omitempty"`
	Usage          map[string]int           `json:"usage,omitempty"`
	Usages         []map[string]int         `json:"usages,omitempty"`
	ElapsedMS      int64                    `json:"elapsed_ms,omitempty"`
	UltraPlanState map[string]interface{}   `json:"ultraplan_state,omitempty"`
	TaskOutputs    map[string][]string      `json:"task_outputs,omitempty"`
}

const (
	chatReasoningEffortOff     = "off"
	chatReasoningEffortNone    = "none"
	chatReasoningEffortMinimal = "minimal"
	chatReasoningEffortLow     = "low"
	chatReasoningEffortMedium  = "medium"
	chatReasoningEffortHigh    = "high"
	chatReasoningEffortXHigh   = "xhigh"
	chatReasoningEffortMax     = "max"
)

type chatSettings struct {
	LLMNo           int    `json:"llm_no"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type chatSettingsPatch struct {
	LLMNo           int       `json:"llm_no"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	ExtraSysPrompts *[]string `json:"extra_sys_prompts"`
}

func normalizeChatExtraSysPrompts(prompts []string) []string {
	cleaned := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		prompt = strings.TrimSpace(prompt)
		if prompt != "" {
			cleaned = append(cleaned, prompt)
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}

func normalizeChatSettings(st chatSettings) chatSettings {
	switch strings.ToLower(strings.TrimSpace(st.ReasoningEffort)) {
	case "", "default", "model":
		st.ReasoningEffort = ""
	case chatReasoningEffortOff, "clear", "unset":
		st.ReasoningEffort = chatReasoningEffortOff
	case chatReasoningEffortNone:
		st.ReasoningEffort = chatReasoningEffortNone
	case chatReasoningEffortMinimal:
		st.ReasoningEffort = chatReasoningEffortMinimal
	case chatReasoningEffortLow:
		st.ReasoningEffort = chatReasoningEffortLow
	case chatReasoningEffortMedium:
		st.ReasoningEffort = chatReasoningEffortMedium
	case chatReasoningEffortHigh:
		st.ReasoningEffort = chatReasoningEffortHigh
	case chatReasoningEffortXHigh:
		st.ReasoningEffort = chatReasoningEffortXHigh
	case chatReasoningEffortMax:
		st.ReasoningEffort = chatReasoningEffortMax
	default:
		st.ReasoningEffort = chatReasoningEffortOff
	}
	return st
}

type chatSession struct {
	ID              string                   `json:"id"`
	Title           string                   `json:"title"`
	UpdatedAt       int64                    `json:"updated_at"`
	Messages        []chatMessage            `json:"messages"`
	Settings        chatSettings             `json:"settings"`
	RawHistory      []map[string]interface{} `json:"raw_history,omitempty"`
	HistoryInfo     []interface{}            `json:"history_info,omitempty"`
	Working         map[string]interface{}   `json:"working,omitempty"`
	WorldlineHead   string                   `json:"worldline_head,omitempty"`
	Workspace       string                   `json:"workspace,omitempty"`
	ProjectMode     string                   `json:"project_mode,omitempty"`
	ExtraSysPrompts []string                 `json:"extra_sys_prompts,omitempty"`
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

const chatRunSubscriberBuffer = 4096

type chatWorker struct {
	SID    string
	Cmd    *exec.Cmd
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
	Dead   bool
	Mu     sync.Mutex
}

func runOneShotBTWWorker(cfg config.AppConfig, sid string, req map[string]interface{}) (chatMessage, error) {
	worker, err := startChatWorker(cfg, sid+"-btw")
	if err != nil {
		return chatMessage{}, err
	}
	waited := false
	defer func() {
		_ = worker.Stdin.Close()
		if !waited && worker.Cmd != nil && worker.Cmd.Process != nil {
			_ = worker.Cmd.Process.Kill()
			_, _ = worker.Cmd.Process.Wait()
		}
	}()
	if err := json.NewEncoder(worker.Stdin).Encode(req); err != nil {
		return chatMessage{}, err
	}
	reader := bufio.NewReaderSize(worker.Stdout, 64*1024)
	for {
		line, readErr := readChatWorkerLine(reader)
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			var ev map[string]interface{}
			if err := json.Unmarshal(line, &ev); err == nil {
				typ, _ := ev["type"].(string)
				if typ == "btw_done" {
					data, err := json.Marshal(ev["message"])
					if err != nil {
						return chatMessage{}, err
					}
					var msg chatMessage
					if err := json.Unmarshal(data, &msg); err != nil {
						return chatMessage{}, err
					}
					if strings.TrimSpace(msg.Content) == "" {
						return chatMessage{}, fmt.Errorf("btw worker returned empty response")
					}
					_ = worker.Stdin.Close()
					waitErr := worker.Cmd.Wait()
					waited = true
					if waitErr != nil {
						return chatMessage{}, waitErr
					}
					return msg, nil
				}
				if typ == "error" {
					data, _ := json.Marshal(ev["message"])
					var msg chatMessage
					_ = json.Unmarshal(data, &msg)
					if strings.TrimSpace(msg.Content) != "" {
						return chatMessage{}, fmt.Errorf("%s", msg.Content)
					}
					return chatMessage{}, fmt.Errorf("btw worker failed")
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return chatMessage{}, fmt.Errorf("btw worker exited before response")
			}
			return chatMessage{}, readErr
		}
	}
}

var runOneShotBTWWorkerFunc = runOneShotBTWWorker

func (s *Server) runChatWorker(sid string, cs chatSession, cmdReq map[string]interface{}) {
	s.runChatWorkerOwned(sid, nil, cs, cmdReq)
}

func (s *Server) runChatWorkerOwned(sid string, token *chatRun, cs chatSession, cmdReq map[string]interface{}) {
	worldlineResend, _ := cmdReq["_ga_worldline_resend"].(bool)
	delete(cmdReq, "_ga_worldline_resend")
	saveTerminal := func(session chatSession) error {
		if worldlineResend {
			return s.saveChatSessionExact(session)
		}
		return s.saveChatSessionMerged(session)
	}
	startedAt := time.Now()
	elapsedMillis := func() int64 {
		ms := time.Since(startedAt).Milliseconds()
		if ms < 1 {
			return 1
		}
		return ms
	}
	worker, err := s.getChatWorker(sid)
	if err != nil {
		msg := chatMessage{ID: newChatID(), Role: "assistant", Content: fmt.Sprintf("提交失败：%v", err), CreatedAt: time.Now().Unix(), Error: true, ElapsedMS: elapsedMillis()}
		cs.Messages = append(cs.Messages, msg)
		_ = saveTerminal(cs)
		s.publishChatRun(sid, map[string]interface{}{"type": "error", "message": msg})
		s.endChatRunOwned(sid, token)
		return
	}
	s.setChatRunCmd(sid, worker.Cmd)
	worker.Mu.Lock()
	defer worker.Mu.Unlock()
	if err := json.NewEncoder(worker.Stdin).Encode(cmdReq); err != nil {
		s.dropChatWorker(sid, worker)
		msg := chatMessage{ID: newChatID(), Role: "assistant", Content: fmt.Sprintf("提交失败：%v", err), CreatedAt: time.Now().Unix(), Error: true, ElapsedMS: elapsedMillis()}
		cs.Messages = append(cs.Messages, msg)
		_ = saveTerminal(cs)
		s.publishChatRun(sid, map[string]interface{}{"type": "error", "message": msg})
		s.endChatRunOwned(sid, token)
		return
	}
	reader := bufio.NewReaderSize(worker.Stdout, 64*1024)
	// Publish start event so frontend can base its live timer on backend clock
	if startLine, err := json.Marshal(map[string]interface{}{
		"type":              "start",
		"run_started_at_ms": startedAt.UnixMilli(),
	}); err == nil {
		s.publishChatLine(sid, startLine)
	}
	var final chatMessage
	var finalRawHistory []map[string]interface{}
	var finalHistoryInfo []interface{}
	var finalWorking map[string]interface{}
	var finalUltraPlanState map[string]interface{}
	var finalReasoningEffort string
	var finalModelID string
	var taskOutputsAccumulator = make(map[string][]string)
	var terminalLine []byte
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
		if ev["type"] == "model" {
			if modelID, ok := ev["model_id"].(string); ok {
				finalModelID = strings.TrimSpace(modelID)
			}
		}
		if ev["type"] == "ultraplan_event" {
			if state := chatUltraPlanStateFromEvent(ev); state != nil {
				finalUltraPlanState = mergeChatMaps(finalUltraPlanState, state)
			}
		}
		if ev["type"] == "ultraplan_output" {
			if taskID, ok := ev["task_id"].(string); ok {
				if lines, ok := ev["lines"].([]interface{}); ok {
					for _, line := range lines {
						if lineStr, ok := line.(string); ok {
							taskOutputsAccumulator[taskID] = append(taskOutputsAccumulator[taskID], lineStr)
						}
					}
				}
			}
		}
		if msg, ok := ev["message"].(map[string]interface{}); ok && (ev["type"] == "done" || ev["type"] == "error") {
			b, _ := json.Marshal(msg)
			_ = json.Unmarshal(b, &final)
			final.ModelID = strings.TrimSpace(final.ModelID)
			if final.ModelID != "" {
				finalModelID = final.ModelID
			} else if finalModelID != "" {
				final.ModelID = finalModelID
				msg["model_id"] = finalModelID
			}
			if final.ElapsedMS <= 0 {
				final.ElapsedMS = elapsedMillis()
			}
			msg["elapsed_ms"] = final.ElapsedMS
			ev["message"] = msg
			// Extract usage from event if present
			if usage, ok := ev["usage"].(map[string]interface{}); ok && len(usage) > 0 {
				final.Usage = make(map[string]int)
				for k, v := range usage {
					if val, ok := v.(float64); ok {
						final.Usage[k] = int(val)
					}
				}
			}
			// Extract per-internal-turn usages array if present
			if usages, ok := ev["usages"].([]interface{}); ok && len(usages) > 0 {
				final.Usages = make([]map[string]int, 0, len(usages))
				for _, u := range usages {
					um, ok := u.(map[string]interface{})
					if !ok {
						continue
					}
					turn := make(map[string]int)
					for k, v := range um {
						if val, ok := v.(float64); ok {
							turn[k] = int(val)
						}
					}
					final.Usages = append(final.Usages, turn)
				}
			}
			if finalUltraPlanState != nil {
				final.UltraPlanState = mergeChatMaps(mergeChatMaps(nil, finalUltraPlanState), final.UltraPlanState)
			}
			if len(taskOutputsAccumulator) > 0 {
				if final.UltraPlanState == nil {
					final.UltraPlanState = make(map[string]interface{})
				}
				final.UltraPlanState["task_outputs"] = taskOutputsAccumulator
				final.TaskOutputs = taskOutputsAccumulator
				msg["task_outputs"] = taskOutputsAccumulator
			}
			if final.UltraPlanState != nil {
				msg["ultraplan_state"] = final.UltraPlanState
			}
			finalRawHistory = chatRawHistoryFromEvent(ev)
			finalHistoryInfo = chatHistoryInfoFromEvent(ev)
			finalWorking = chatWorkingFromEvent(ev)
			if v, ok := ev["reasoning_effort"].(string); ok {
				finalReasoningEffort = v
			}
			delete(ev, "raw_history")
			delete(ev, "history_info")
			delete(ev, "working")
			if cleanLine, err := json.Marshal(ev); err == nil {
				terminalLine = cleanLine
			} else {
				terminalLine = append([]byte(nil), line...)
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
			final = chatMessage{ID: newChatID(), Role: "assistant", Content: content, ModelID: finalModelID, CreatedAt: time.Now().Unix(), Error: true, ElapsedMS: elapsedMillis(), UltraPlanState: mergeChatMaps(nil, finalUltraPlanState)}
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
			final = chatMessage{ID: newChatID(), Role: "assistant", Content: content, ModelID: finalModelID, CreatedAt: time.Now().Unix(), Error: true, ElapsedMS: elapsedMillis(), UltraPlanState: mergeChatMaps(nil, finalUltraPlanState)}
			s.publishChatRun(sid, map[string]interface{}{"type": "error", "message": final})
		}
	}
	var fallbackMessages []chatMessage
	if len(cs.Messages) > 0 {
		fallbackMessages = append(fallbackMessages, cs.Messages[len(cs.Messages)-1])
	}
	fallbackMessages = append(fallbackMessages, final)
	cs.Messages = append(cs.Messages, final)
	if !final.Error && s.ownsChatRun(sid, token) && len(cs.Messages) >= 2 {
		userMsg := cs.Messages[len(cs.Messages)-2]
		if userMsg.Role == "user" {
			bound, bindErr := s.chatWorldlineRPCLocked(sid, worker, cs.Workspace, map[string]interface{}{
				"action":               "bind",
				"turn_status":          "completed",
				"has_final_answer":     true,
				"user_message_id":      userMsg.ID,
				"assistant_message_id": final.ID,
				"display_path":         cs.Messages,
			})
			if bindErr != nil {
				final.Error = true
				final.Content = strings.TrimSpace(final.Content) + fmt.Sprintf("\n\n[Worldline bind failed: %v]", bindErr)
				cs.Messages[len(cs.Messages)-1] = final
				terminalLine, _ = json.Marshal(map[string]interface{}{"type": "error", "message": final})
			} else if bound.Tree.Head != nil && s.ownsChatRun(sid, token) {
				cs.WorldlineHead = *bound.Tree.Head
			}
		}
	}
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
	if strings.TrimSpace(finalReasoningEffort) != "" {
		cs.Settings.ReasoningEffort = normalizeChatSettings(chatSettings{ReasoningEffort: finalReasoningEffort}).ReasoningEffort
	}
	cs.UpdatedAt = time.Now().Unix()
	if token != nil && !s.ownsChatRun(sid, token) {
		s.endChatRunOwned(sid, token)
		return
	}
	if saveErr := saveTerminal(cs); saveErr != nil {
		commitFailure := chatMessage{
			ID:        newChatID(),
			Role:      "assistant",
			Content:   fmt.Sprintf("Failed to persist terminal chat state: %v", saveErr),
			CreatedAt: time.Now().Unix(),
			Error:     true,
		}
		s.publishChatRun(sid, map[string]interface{}{"type": "error", "message": commitFailure})
		s.endChatRunOwned(sid, token)
		return
	}
	if len(terminalLine) > 0 {
		s.publishChatLine(sid, terminalLine)
	}
	s.endChatRunOwned(sid, token)
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

func chatUltraPlanStateFromEvent(ev map[string]interface{}) map[string]interface{} {
	m, ok := ev["state"].(map[string]interface{})
	if !ok {
		return nil
	}
	return mergeChatMaps(nil, m)
}

func mergeChatMaps(dst map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]interface{}, len(src))
	}
	for k, v := range src {
		if existing, ok := dst[k].(map[string]interface{}); ok {
			if incoming, ok := v.(map[string]interface{}); ok {
				dst[k] = mergeChatMaps(existing, incoming)
				continue
			}
		}
		dst[k] = v
	}
	return dst
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

func projectModeWorkspace(cfg config.AppConfig, name string) string {
	name, ok := validProjectModeName(name)
	if !ok {
		return ""
	}
	return filepath.Join(cfg.GARoot, "temp", "projects", name)
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

func (s *Server) beginChatRun(sid string) *chatRun {
	sid = safeChatID(sid)
	s.ChatMu.Lock()
	defer s.ChatMu.Unlock()
	if s.ChatRuns == nil {
		s.ChatRuns = map[string]*chatRun{}
	}
	if r := s.ChatRuns[sid]; r != nil && !r.Done {
		return nil
	}
	token := &chatRun{SID: sid, Subscribers: map[chan []byte]bool{}}
	s.ChatRuns[sid] = token
	return token
}

func (s *Server) ownsChatRun(sid string, token *chatRun) bool {
	s.ChatMu.Lock()
	defer s.ChatMu.Unlock()
	r := s.ChatRuns[safeChatID(sid)]
	return token != nil && r == token && !token.Done && !token.Canceled
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
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- b:
			default:
			}
		}
	}
}

func (s *Server) endChatRunOwned(sid string, token *chatRun) {
	sid = safeChatID(sid)
	s.ChatMu.Lock()
	r := s.ChatRuns[sid]
	if token != nil && r != token {
		s.ChatMu.Unlock()
		return
	}
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
		if rr := s.ChatRuns[sid]; rr == r && rr.Done {
			delete(s.ChatRuns, sid)
		}
		s.ChatMu.Unlock()
	}()
}

func (s *Server) endChatRun(sid string) { s.endChatRunOwned(sid, nil) }

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
	ch := make(chan []byte, chatRunSubscriberBuffer)
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
    client = agent.llmclients[int(idx)]
    backend = client.backend
    name = str(getattr(backend, 'name', '') or '')
    model = str(getattr(backend, 'model', '') or '')
    provider = type(backend).__name__
    items.append({'index': int(idx), 'label': text, 'name': name, 'provider': provider, 'model': model, 'active': bool(active)})
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
	if draft, importErr := s.loadModelsFromOfficialMyKey(false); importErr == nil {
		annotateChatLLMProviders(llms, draft.Profiles)
	}
	return llms, nil
}

type chatProviderModel struct {
	provider string
	model    string
	order    int
	sequence int
}

func annotateChatLLMProviders(llms []map[string]interface{}, profiles []modelconfig.Profile) {
	configured := make([]chatProviderModel, 0)
	sequence := 0
	for _, profile := range profiles {
		provider := chatProviderDisplayName(profile)
		configs := profile.ModelConfigs
		if len(configs) == 0 {
			configs = make([]modelconfig.ModelConfig, 0, len(profile.Models))
			for _, model := range profile.Models {
				configs = append(configs, modelconfig.ModelConfig{Model: model})
			}
		}
		for _, config := range configs {
			model := strings.TrimSpace(config.Model)
			if model == "" {
				continue
			}
			order := int(^uint(0) >> 1)
			if config.SortOrder != nil {
				order = *config.SortOrder
			}
			configured = append(configured, chatProviderModel{provider: provider, model: model, order: order, sequence: sequence})
			sequence++
		}
	}
	sort.SliceStable(configured, func(i, j int) bool {
		if configured[i].order == configured[j].order {
			return configured[i].sequence < configured[j].sequence
		}
		return configured[i].order < configured[j].order
	})

	used := make([]bool, len(configured))
	unresolved := make([]int, 0)
	for i, item := range llms {
		model := chatLLMModel(item)
		if i < len(configured) && (model == "" || configured[i].model == model) {
			applyChatProviderModel(item, configured[i])
			used[i] = true
			continue
		}
		unresolved = append(unresolved, i)
	}
	for _, llmIndex := range unresolved {
		item := llms[llmIndex]
		model := chatLLMModel(item)
		for configuredIndex, candidate := range configured {
			if used[configuredIndex] || candidate.model != model {
				continue
			}
			applyChatProviderModel(item, candidate)
			used[configuredIndex] = true
			break
		}
	}
}

func applyChatProviderModel(item map[string]interface{}, configured chatProviderModel) {
	item["provider"] = configured.provider
	if chatLLMModel(item) == "" {
		item["model"] = configured.model
	}
}

func chatLLMModel(item map[string]interface{}) string {
	value, ok := item["model"]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func chatProviderDisplayName(profile modelconfig.Profile) string {
	name := strings.TrimSpace(profile.VarName)
	for _, prefix := range []string{"native_oai_config", "native_claude_config", "oai_config", "claude_config"} {
		if strings.HasPrefix(name, prefix) {
			name = strings.TrimPrefix(name, prefix)
			name = strings.TrimPrefix(name, "_")
			break
		}
	}
	if name != "" {
		return name
	}
	return "Unknown provider"
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
	cmd.Env = chatWorkerEnvironment(cfg, root, sid)
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

func chatWorkerEnvironment(cfg config.AppConfig, root, sid string) []string {
	env := pythonEnvWithAdminProxy(cfg, "PYTHONUNBUFFERED=1", "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8", "GA_ROOT="+root, "GA_ULTRAPLAN_BROWSER=0")
	filtered := make([]string, 0, len(env)+1)
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(key, "GA_ADMIN_SESSION_ID") {
			continue
		}
		filtered = append(filtered, kv)
	}
	return append(filtered, "GA_ADMIN_SESSION_ID="+safeChatID(sid))
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
func (s *Server) mutateChatSession(sid string, token *chatRun, mutate func(*chatSession) error) (chatSession, error) {
	if !s.ownsChatRun(sid, token) {
		return chatSession{}, fmt.Errorf("chat run is no longer owned")
	}
	s.SessionMu.Lock()
	defer s.SessionMu.Unlock()
	if s.chatSessionMutationHook != nil {
		s.chatSessionMutationHook()
	}
	cs, err := loadChatSession(s.CfgStore.Cfg, sid)
	if err != nil {
		return chatSession{}, err
	}
	if err = mutate(&cs); err != nil {
		return chatSession{}, err
	}
	err = saveChatSessionLocked(s.CfgStore.Cfg, cs)
	return cs, err
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
func mergeChatMessageLists(first, second []chatMessage) []chatMessage {
	out := make([]chatMessage, 0, len(first)+len(second))
	seen := make(map[string]bool, len(first)+len(second))
	appendUnique := func(items []chatMessage) {
		for _, msg := range items {
			if msg.ID != "" && seen[msg.ID] {
				continue
			}
			if msg.ID != "" {
				seen[msg.ID] = true
			}
			out = append(out, msg)
		}
	}
	appendUnique(first)
	appendUnique(second)
	return out
}

func (s *Server) saveChatSessionMerged(cs chatSession) error {
	s.SessionMu.Lock()
	defer s.SessionMu.Unlock()
	latest, err := loadChatSession(s.CfgStore.Cfg, cs.ID)
	if err != nil {
		return err
	}
	cs.Messages = mergeChatMessageLists(latest.Messages, cs.Messages)
	return saveChatSession(s.CfgStore.Cfg, cs)
}

func (s *Server) saveChatSessionExact(cs chatSession) error {
	if s.chatExactSaveHook != nil {
		if err := s.chatExactSaveHook(cs); err != nil {
			return err
		}
	}
	s.SessionMu.Lock()
	defer s.SessionMu.Unlock()
	return saveChatSessionLocked(s.CfgStore.Cfg, cs)
}

func (s *Server) persistChatSessionIfMissing(cs chatSession) error {
	s.SessionMu.Lock()
	defer s.SessionMu.Unlock()
	_, err := os.Stat(chatSessionPath(s.CfgStore.Cfg, cs.ID))
	if err == nil {
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return saveChatSessionLocked(s.CfgStore.Cfg, cs)
}

func saveChatSession(cfg config.AppConfig, cs chatSession) error {
	return saveChatSessionLocked(cfg, cs)
}

func saveChatSessionLocked(cfg config.AppConfig, cs chatSession) error {
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
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' || r == ']':
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
