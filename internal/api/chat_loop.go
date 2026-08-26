package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	chatLoopDefaultMaxRounds = 10
	chatLoopMaxRounds        = 100

	// The controller occasionally answers in prose instead of the decision
	// protocol. One corrective re-ask costs far less than dropping a loop that
	// was otherwise healthy.
	chatLoopControllerAttempts = 2
	chatLoopAdvancePrompt      = "\u76ee\u6807\u5c1a\u672a\u5b8c\u6210\uff0c\u8bf7\u57fa\u4e8e\u5f53\u524d\u8fdb\u5c55\u81ea\u4e3b\u63a8\u8fdb\u3002"

	chatLoopStatusWaiting    = "waiting"
	chatLoopStatusRunning    = "running"
	chatLoopStatusEvaluating = "evaluating"
	chatLoopStatusStopped    = "stopped"
	chatLoopStatusCompleted  = "completed"
	chatLoopStatusError      = "error"
	chatLoopStatusPaused     = "paused"
)

var (
	chatLoopNextPromptTagRE = regexp.MustCompile(`(?is)</?next_prompt\s*>`)
	chatLoopContinueTagRE   = regexp.MustCompile(`(?is)</?loop_continue\s*>`)
	chatLoopCompleteTagRE   = regexp.MustCompile(`(?is)</?loop_complete\s*>`)
	errChatLoopStale        = errors.New("stale chat loop decision")
)

const (
	maxChatLoopRecords         = 40
	maxChatLoopRecordTextRunes = 360
)

func boundedChatLoopRecordText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxChatLoopRecordTextRunes {
		return text
	}
	return string(runes[:maxChatLoopRecordTextRunes]) + "..."
}

func appendChatLoopRecord(state *chatLoopState, phase, summary, prompt string) {
	if state == nil {
		return
	}
	state.Records = append(state.Records, chatLoopRecord{
		AtMS:    time.Now().UnixMilli(),
		Round:   state.Round,
		Phase:   boundedChatLoopRecordText(phase),
		Summary: boundedChatLoopRecordText(summary),
		Prompt:  boundedChatLoopRecordText(prompt),
	})
	if len(state.Records) > maxChatLoopRecords {
		state.Records = append([]chatLoopRecord(nil), state.Records[len(state.Records)-maxChatLoopRecords:]...)
	}
}

func appendChatLoopTerminalRecord(state *chatLoopState, status, reason string) {
	phase := "stopped"
	summary := "Loop stopped."
	switch {
	case status == chatLoopStatusCompleted && reason == "controller_no_action":
		phase = "no_action"
		summary = "Controller reported no further action."
	case status == chatLoopStatusCompleted:
		phase = "complete"
		summary = "Controller marked the objective complete."
	case status == chatLoopStatusError:
		phase = "error"
		summary = "Controller evaluation failed."
	case status == chatLoopStatusPaused:
		phase = "paused"
		summary = "Loop paused."
	}
	appendChatLoopRecord(state, phase, summary, "")
}

func normalizeChatLoopMaxRounds(value int) int {
	if value <= 0 {
		return chatLoopDefaultMaxRounds
	}
	if value > chatLoopMaxRounds {
		return chatLoopMaxRounds
	}
	return value
}

func chatLoopHasTerminalAssistant(cs chatSession) bool {
	if len(cs.Messages) == 0 {
		return false
	}
	last := cs.Messages[len(cs.Messages)-1]
	return last.Role == "assistant" && !last.Error && strings.TrimSpace(last.Content) != ""
}

func chatLoopControllerPrompt(objective string, round, maxRounds int) string {
	return fmt.Sprintf(`You are the supervisor for an autonomous task loop. Do not perform the task yourself and do not plan or prescribe the next action.

Loop objective (quoted): %q
Completed automatic rounds: %d of %d.

Review the conversation and make exactly one binary decision:
1. If the objective is fully complete, output exactly <loop_complete>brief completion reason</loop_complete>.
2. If the objective is not fully complete, output exactly <loop_continue>continue</loop_continue>.

Do not describe what to do next. Do not provide a plan, instruction, summary, explanation, markdown fence, or any text outside the single chosen XML element.`, objective, round, maxRounds)
}

func chatLoopControllerRetryPrompt(objective string, round, maxRounds int) string {
	return chatLoopControllerPrompt(objective, round, maxRounds) + `

Your previous reply was rejected. Answer with exactly one loop_complete or loop_continue element and nothing else.`
}

type chatLoopDecision struct {
	Complete bool
}

func extractLastChatLoopElementAt(content string, tagRE *regexp.Regexp) (string, int, bool) {
	tags := tagRE.FindAllStringIndex(content, -1)
	for i := len(tags) - 1; i > 0; i-- {
		closeTag := content[tags[i][0]:tags[i][1]]
		openTag := content[tags[i-1][0]:tags[i-1][1]]
		if strings.HasPrefix(strings.ToLower(closeTag), "</") && !strings.HasPrefix(strings.ToLower(openTag), "</") {
			value := strings.TrimSpace(content[tags[i-1][1]:tags[i][0]])
			if value != "" {
				return value, tags[i][1], true
			}
		}
	}
	return "", -1, false
}

func extractLastChatLoopElement(content string, tagRE *regexp.Regexp) (string, bool) {
	value, _, ok := extractLastChatLoopElementAt(content, tagRE)
	return value, ok
}

func parseChatLoopNextPrompt(content string) string {
	prompt, _ := extractLastChatLoopElement(content, chatLoopNextPromptTagRE)
	return prompt
}

func parseChatLoopDecision(content string) (chatLoopDecision, error) {
	_, continueEnd, hasContinue := extractLastChatLoopElementAt(content, chatLoopContinueTagRE)
	_, completeEnd, hasComplete := extractLastChatLoopElementAt(content, chatLoopCompleteTagRE)
	if hasComplete && (!hasContinue || completeEnd > continueEnd) {
		return chatLoopDecision{Complete: true}, nil
	}
	if hasContinue {
		return chatLoopDecision{}, nil
	}
	return chatLoopDecision{}, errors.New("controller returned no usable loop_complete or loop_continue element")
}

func (s *Server) chatLoopStart(w http.ResponseWriter, r *http.Request, sid string) {
	var req struct {
		Objective        string `json:"objective"`
		ControllerPrompt string `json:"controller_prompt"`
		ControllerLLMNo  *int   `json:"controller_llm_no"`
		MaxRounds        int    `json:"max_rounds"`
	}
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		objective = strings.TrimSpace(req.ControllerPrompt)
	}
	if objective == "" {
		bad(w, http.StatusBadRequest, "objective is required")
		return
	}

	sid = safeChatID(sid)
	running := s.chatRunActive(sid)
	s.SessionMu.Lock()
	cs, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err == nil {
		if cs.ID == "" {
			cs = chatSession{
				ID:         sid,
				Title:      "New chat",
				UpdatedAt:  time.Now().Unix(),
				Messages:   []chatMessage{},
				Settings:   s.defaultChatSettings(),
				RawHistory: []map[string]interface{}{},
			}
		}
		controllerLLMNo := cs.Settings.LLMNo
		if req.ControllerLLMNo != nil {
			controllerLLMNo = *req.ControllerLLMNo
		}
		cs.Loop = chatLoopState{
			Enabled:          true,
			Status:           chatLoopStatusWaiting,
			Epoch:            cs.Loop.Epoch + 1,
			Round:            0,
			MaxRounds:        normalizeChatLoopMaxRounds(req.MaxRounds),
			ControllerPrompt: objective,
			ControllerLLMNo:  controllerLLMNo,
		}
		appendChatLoopRecord(&cs.Loop, "started", "Loop started.", "")
		if running {
			cs.Loop.Status = chatLoopStatusRunning
		} else if len(cs.Messages) == 0 {
			// An empty session has no completed run for the controller to evaluate.
			// Queue the objective itself as the first worker round instead of leaving
			// the loop waiting forever for a terminal event that cannot arrive.
			cs.Loop.Status = chatLoopStatusEvaluating
		}
		err = saveChatSessionLocked(s.CfgStore.Snapshot(), cs)
	}
	s.SessionMu.Unlock()
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}

	if !running && len(cs.Messages) == 0 {
		go continueChatLoopFunc(s, sid, cs.Loop.Epoch, objective)
	} else if !running && chatLoopHasTerminalAssistant(cs) {
		s.afterChatRunTerminal(sid, true)
	}
	writeJSON(w, map[string]interface{}{"ok": true, "loop": cs.Loop})
}

func (s *Server) chatLoopStop(w http.ResponseWriter, r *http.Request, sid string) {
	sid = safeChatID(sid)
	s.SessionMu.Lock()
	cs, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err == nil {
		if cs.ID == "" {
			err = fmt.Errorf("chat session not found")
		} else {
			cs.Loop.Enabled = false
			cs.Loop.Status = chatLoopStatusStopped
			cs.Loop.StopReason = "user"
			cs.Loop.Epoch++
			err = saveChatSessionLocked(s.CfgStore.Snapshot(), cs)
		}
	}
	s.SessionMu.Unlock()
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishChatLoopState(sid, cs.Loop)
	writeJSON(w, map[string]interface{}{"ok": true, "loop": cs.Loop})
}

func (s *Server) publishChatLoopState(sid string, state chatLoopState) {
	s.publishChatRun(sid, map[string]interface{}{"type": "loop", "loop": state})
}

func (s *Server) afterChatRunTerminal(sid string, success bool) {
	if !success {
		return
	}
	sid = safeChatID(sid)

	s.SessionMu.Lock()
	cs, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil {
		s.SessionMu.Unlock()
		return
	}

	// Check queued messages first
	if success && len(cs.QueuedMessages) > 0 {
		s.SessionMu.Unlock()
		go s.processNextQueuedMessage(sid)
		return
	}

	// Then check loop mode
	if !cs.Loop.Enabled || cs.Loop.Status == chatLoopStatusEvaluating {
		s.SessionMu.Unlock()
		return
	}
	cs.Loop.MaxRounds = normalizeChatLoopMaxRounds(cs.Loop.MaxRounds)
	if cs.Loop.Round >= cs.Loop.MaxRounds {
		cs.Loop.Enabled = false
		cs.Loop.Status = chatLoopStatusCompleted
		cs.Loop.StopReason = "max_rounds"
		appendChatLoopRecord(&cs.Loop, "complete", "Loop reached its configured round limit.", "")
		cs.Loop.Epoch++
		_ = saveChatSessionLocked(s.CfgStore.Snapshot(), cs)
		s.SessionMu.Unlock()
		s.publishChatLoopState(sid, cs.Loop)
		return
	}
	cs.Loop.Status = chatLoopStatusEvaluating
	cs.Loop.StopReason = ""
	appendChatLoopRecord(&cs.Loop, "checking", "Observer is checking the latest run.", "")
	epoch := cs.Loop.Epoch
	if err = saveChatSessionLocked(s.CfgStore.Snapshot(), cs); err != nil {
		s.SessionMu.Unlock()
		return
	}
	s.SessionMu.Unlock()
	s.publishChatLoopState(sid, cs.Loop)
	go s.evaluateChatLoop(sid, epoch, cs)
}

func (s *Server) evaluateChatLoop(sid string, epoch int64, cs chatSession) {
	state := cs.Loop
	cmdReq := map[string]interface{}{
		"op":                "btw",
		"history":           cs.Messages,
		"raw_history":       cs.RawHistory,
		"history_info":      cs.HistoryInfo,
		"working":           cs.Working,
		"workspace":         cs.Workspace,
		"project_mode":      cs.ProjectMode,
		"extra_sys_prompts": cs.ExtraSysPrompts,
		"llm_no":            state.ControllerLLMNo,
		"reasoning_effort":  cs.Settings.ReasoningEffort,
		"ga_root":           s.CfgStore.Snapshot().GARoot,
	}
	var decision chatLoopDecision
	var parseErr error
	for attempt := 0; attempt < chatLoopControllerAttempts; attempt++ {
		if attempt == 0 {
			cmdReq["prompt"] = chatLoopControllerPrompt(state.ControllerPrompt, state.Round, state.MaxRounds)
		} else {
			cmdReq["prompt"] = chatLoopControllerRetryPrompt(state.ControllerPrompt, state.Round, state.MaxRounds)
		}
		msg, err := runOneShotBTWWorkerFunc(s.CfgStore.Snapshot(), sid+"-loop", cmdReq)
		if err != nil {
			s.finishChatLoop(sid, epoch, chatLoopStatusError, "controller_error: "+err.Error())
			return
		}
		decision, parseErr = parseChatLoopDecision(msg.Content)
		if parseErr == nil {
			break
		}
		if attempt+1 < chatLoopControllerAttempts && !s.recordChatLoopRetry(sid, epoch) {
			return
		}
	}
	if parseErr != nil {
		s.finishChatLoop(sid, epoch, chatLoopStatusError, "controller_protocol_error: "+parseErr.Error())
		return
	}
	if decision.Complete {
		s.finishChatLoop(sid, epoch, chatLoopStatusCompleted, "controller_complete")
		return
	}
	s.continueChatLoop(sid, epoch, chatLoopAdvancePrompt)
}

// recordChatLoopRetry reports whether the loop is still live and owned by this
// evaluation, so a stopped loop never spends a second controller call. The
// record itself is advisory: failing to persist it must not strand the loop.
func (s *Server) recordChatLoopRetry(sid string, epoch int64) bool {
	s.SessionMu.Lock()
	cs, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil || !cs.Loop.Enabled || cs.Loop.Epoch != epoch || cs.Loop.Status != chatLoopStatusEvaluating {
		s.SessionMu.Unlock()
		return false
	}
	appendChatLoopRecord(&cs.Loop, "retry", "Controller reply was unusable; asking once more.", "")
	saved := saveChatSessionLocked(s.CfgStore.Snapshot(), cs) == nil
	s.SessionMu.Unlock()
	if saved {
		s.publishChatLoopState(sid, cs.Loop)
	}
	return true
}

func (s *Server) finishChatLoop(sid string, epoch int64, status, reason string) {
	s.SessionMu.Lock()
	cs, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil || !cs.Loop.Enabled || cs.Loop.Epoch != epoch {
		s.SessionMu.Unlock()
		return
	}
	cs.Loop.Enabled = false
	cs.Loop.Status = status
	cs.Loop.StopReason = reason
	appendChatLoopTerminalRecord(&cs.Loop, status, reason)
	cs.Loop.Epoch++
	err = saveChatSessionLocked(s.CfgStore.Snapshot(), cs)
	s.SessionMu.Unlock()
	if err == nil {
		s.publishChatLoopState(sid, cs.Loop)
	}
}

var continueChatLoopFunc = func(s *Server, sid string, epoch int64, prompt string) {
	s.continueChatLoop(sid, epoch, prompt)
}

func (s *Server) continueChatLoop(sid string, epoch int64, prompt string) {
	token := s.beginChatRun(sid)
	if token == nil {
		s.deferChatLoopForActiveRun(sid, epoch)
		return
	}

	runStartedAtMS := time.Now().UnixMilli()
	userMsg := chatMessage{ID: newChatID(), Role: "user", Content: prompt, CreatedAt: time.Now().Unix()}
	pendingMsg := chatMessage{ID: newChatID(), Role: "assistant", CreatedAt: time.Now().Unix(), RunStartedAtMS: runStartedAtMS}
	var cs chatSession
	var terminalLoop *chatLoopState
	owned, saveErr := s.saveChatRunPending(sid, token, pendingMsg.ID, runStartedAtMS, func() error {
		s.SessionMu.Lock()
		defer s.SessionMu.Unlock()
		latest, err := loadChatSession(s.CfgStore.Snapshot(), sid)
		if err != nil {
			return err
		}
		if !latest.Loop.Enabled || latest.Loop.Epoch != epoch || latest.Loop.Status != chatLoopStatusEvaluating {
			return errChatLoopStale
		}
		latest.Loop.MaxRounds = normalizeChatLoopMaxRounds(latest.Loop.MaxRounds)
		if latest.Loop.Round >= latest.Loop.MaxRounds {
			latest.Loop.Enabled = false
			latest.Loop.Status = chatLoopStatusCompleted
			latest.Loop.StopReason = "max_rounds"
			appendChatLoopRecord(&latest.Loop, "complete", "Loop reached its configured round limit.", "")
			latest.Loop.Epoch++
			if err := saveChatSessionLocked(s.CfgStore.Snapshot(), latest); err != nil {
				return err
			}
			finished := latest.Loop
			terminalLoop = &finished
			return errChatLoopStale
		}
		latest.Loop.Round++
		latest.Loop.Status = chatLoopStatusRunning
		latest.Loop.StopReason = ""
		appendChatLoopRecord(&latest.Loop, "continue", "Supervisor requested another round.", "")
		latest.Messages = append(latest.Messages, userMsg, pendingMsg)
		latest.UpdatedAt = time.Now().Unix()
		updateChatTitle(&latest)
		if err := saveChatSessionLocked(s.CfgStore.Snapshot(), latest); err != nil {
			return err
		}
		cs = latest
		return nil
	})
	if !owned || saveErr != nil {
		s.endChatRunOwned(sid, token)
		if terminalLoop != nil {
			s.publishChatLoopState(sid, *terminalLoop)
		}
		if saveErr != nil && !errors.Is(saveErr, errChatLoopStale) {
			s.finishChatLoop(sid, epoch, chatLoopStatusError, "persist_error: "+saveErr.Error())
		}
		return
	}

	s.publishChatLoopState(sid, cs.Loop)
	s.publishChatRun(sid, map[string]interface{}{"type": "user", "message": userMsg})
	workerHistory := append([]chatMessage(nil), cs.Messages...)
	for i := len(workerHistory) - 1; i >= 0; i-- {
		if workerHistory[i].ID == userMsg.ID {
			workerHistory = workerHistory[:i]
			break
		}
	}
	cmdReq := map[string]interface{}{
		"prompt":                   prompt,
		"history":                  workerHistory,
		"raw_history":              cs.RawHistory,
		"history_info":             cs.HistoryInfo,
		"working":                  cs.Working,
		"workspace":                cs.Workspace,
		"project_mode":             cs.ProjectMode,
		"extra_sys_prompts":        cs.ExtraSysPrompts,
		"llm_no":                   cs.Settings.LLMNo,
		"reasoning_effort":         cs.Settings.ReasoningEffort,
		"ga_root":                  s.CfgStore.Snapshot().GARoot,
		"_ga_pending_assistant_id": pendingMsg.ID,
		"_ga_run_started_at_ms":    runStartedAtMS,
	}
	go s.runChatWorkerOwned(sid, token, cs, cmdReq)
}

func (s *Server) deferChatLoopForActiveRun(sid string, epoch int64) {
	s.SessionMu.Lock()
	cs, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err == nil && cs.Loop.Enabled && cs.Loop.Epoch == epoch && cs.Loop.Status == chatLoopStatusEvaluating {
		cs.Loop.Status = chatLoopStatusRunning
		err = saveChatSessionLocked(s.CfgStore.Snapshot(), cs)
	}
	s.SessionMu.Unlock()
	if err == nil {
		s.publishChatLoopState(sid, cs.Loop)
	}
}

func (s *Server) recoverChatLoopsAfterRestart() error {
	if s.ChatRuntime == nil {
		return nil
	}
	s.ChatRuntime.loopRecoveryOnce.Do(func() {
		cfg := s.CfgStore.Snapshot()
		if err := ensureChatDataMigrated(cfg); err != nil {
			s.ChatRuntime.loopRecoveryErr = err
			return
		}
		if err := os.MkdirAll(chatSessionDir(cfg), 0755); err != nil {
			s.ChatRuntime.loopRecoveryErr = err
			return
		}
		entries, err := os.ReadDir(chatSessionDir(cfg))
		if err != nil {
			s.ChatRuntime.loopRecoveryErr = err
			return
		}

		s.SessionMu.Lock()
		defer s.SessionMu.Unlock()
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			sid := strings.TrimSuffix(entry.Name(), ".json")
			cs, err := loadChatSession(cfg, sid)
			if err != nil {
				continue
			}
			next, changed := normalizePersistedChatLoop(cs.Loop)
			if !changed {
				continue
			}
			cs.Loop = next
			if err := saveChatSessionPreserveUpdatedAtLocked(cfg, cs); err != nil {
				s.ChatRuntime.loopRecoveryErr = err
				return
			}
		}
	})
	return s.ChatRuntime.loopRecoveryErr
}

func normalizePersistedChatLoop(state chatLoopState) (chatLoopState, bool) {
	if !state.Enabled {
		return state, false
	}
	state.Enabled = false
	state.Status = chatLoopStatusPaused
	state.StopReason = "server_restart"
	state.Epoch++
	state.MaxRounds = normalizeChatLoopMaxRounds(state.MaxRounds)
	return state, true
}

// convertChatUploadsToMaps converts []chatUpload to []map[string]interface{} for message files
func convertChatUploadsToMaps(uploads []chatUpload) []map[string]interface{} {
	if len(uploads) == 0 {
		return nil
	}
	result := make([]map[string]interface{}, len(uploads))
	for i, upload := range uploads {
		result[i] = map[string]interface{}{
			"id":      upload.ID,
			"name":    upload.Name,
			"type":    upload.Type,
			"size":    upload.Size,
			"dataURL": upload.DataURL,
		}
	}
	return result
}

// processNextQueuedMessage executes the next queued message for a session.
// It is called after a chat run completes successfully and there are queued messages.
func (s *Server) processNextQueuedMessage(sid string) {
	sid = safeChatID(sid)

	s.SessionMu.Lock()
	cs, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil || len(cs.QueuedMessages) == 0 {
		s.SessionMu.Unlock()
		return
	}

	// Pop the first queued message
	queuedItem := cs.QueuedMessages[0]
	cs.QueuedMessages = cs.QueuedMessages[1:]
	cs.UpdatedAt = time.Now().Unix()

	if err := saveChatSessionLocked(s.CfgStore.Snapshot(), cs); err != nil {
		s.SessionMu.Unlock()
		return
	}
	s.SessionMu.Unlock()

	// Publish queue status event
	s.publishChatRun(sid, map[string]interface{}{
		"type":            "queue_item_start",
		"queue_item_id":   queuedItem.ID,
		"remaining_count": len(cs.QueuedMessages),
	})

	// Build the request payload for the queued message
	cmdReq := map[string]interface{}{
		"prompt": queuedItem.Text,
		"files":  queuedItem.Files,
	}
	if queuedItem.LLMNo > 0 {
		cmdReq["llmNo"] = queuedItem.LLMNo
	}
	if queuedItem.ReasoningEffort != "" {
		cmdReq["reasoningEffort"] = queuedItem.ReasoningEffort
	}

	// Reuse the same ownership gate as regular chat posts. Completed runs stay in
	// ChatRuns for five minutes so clients can replay their events; their mere
	// presence must not block the next queued message.
	token := s.beginChatRun(sid)
	if token == nil {
		// Another run is still active, put the item back.
		s.SessionMu.Lock()
		cs2, _ := loadChatSession(s.CfgStore.Snapshot(), sid)
		cs2.QueuedMessages = append([]chatQueuedMessage{queuedItem}, cs2.QueuedMessages...)
		_ = saveChatSessionLocked(s.CfgStore.Snapshot(), cs2)
		s.SessionMu.Unlock()
		return
	}

	// Reload session with the queued message removed
	s.SessionMu.Lock()
	cs, err = loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil {
		s.SessionMu.Unlock()
		s.endChatRunOwned(sid, token)
		return
	}

	// Create pending message
	pendingID := newChatID()
	runStartedAtMS := time.Now().UnixMilli()
	pendingMsg := chatMessage{
		ID:             pendingID,
		Role:           "assistant",
		CreatedAt:      time.Now().Unix(),
		RunStartedAtMS: runStartedAtMS,
	}

	// Create user message from queued item
	userMsg := chatMessage{
		ID:        newChatID(),
		Role:      "user",
		Content:   queuedItem.Text,
		Files:     convertChatUploadsToMaps(queuedItem.Files),
		CreatedAt: time.Now().Unix(),
	}

	cs.Messages = append(cs.Messages, userMsg, pendingMsg)
	updateChatTitle(&cs)

	// Apply settings from queued item
	if queuedItem.LLMNo > 0 {
		cs.Settings.LLMNo = queuedItem.LLMNo
	}
	if queuedItem.ReasoningEffort != "" {
		cs.Settings.ReasoningEffort = queuedItem.ReasoningEffort
	}

	cmdReq["_ga_pending_assistant_id"] = pendingID

	if err := saveChatSessionLocked(s.CfgStore.Snapshot(), cs); err != nil {
		s.SessionMu.Unlock()
		s.endChatRunOwned(sid, token)
		return
	}
	s.SessionMu.Unlock()

	// Start the worker
	go s.runChatWorkerOwned(sid, token, cs, cmdReq)
}
