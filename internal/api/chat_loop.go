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
	startFirstRun := false
	loopEpoch := int64(0)
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
		} else {
			// A new chat has no completed assistant turn that could trigger the
			// normal afterChatRunTerminal -> controller evaluation chain. Queue
			// the objective itself as the first main-agent turn instead of leaving
			// the loop in a permanent waiting/0-of-N state.
			startFirstRun = len(cs.Messages) == 0
		}
		loopEpoch = cs.Loop.Epoch
		err = saveChatSessionLocked(s.CfgStore.Snapshot(), cs)
	}
	s.SessionMu.Unlock()
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}

	if startFirstRun {
		s.queueChatLoopRun(chatLoopRunRequest{
			sid:            sid,
			epoch:          loopEpoch,
			prompt:         objective,
			expectedStatus: chatLoopStatusWaiting,
			phase:          "starting",
			summary:        "First task turn queued.",
		})
		// Return the state after the first turn was admitted so the browser can
		// attach to the run immediately, even when the worker starts quickly.
		s.SessionMu.Lock()
		if latest, loadErr := loadChatSession(s.CfgStore.Snapshot(), sid); loadErr == nil {
			cs = latest
		}
		s.SessionMu.Unlock()
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
		s.failChatLoopAfterRun(sid)
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

func (s *Server) failChatLoopAfterRun(sid string) {
	sid = safeChatID(sid)
	s.SessionMu.Lock()
	cs, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	epoch := cs.Loop.Epoch
	active := err == nil && cs.Loop.Enabled
	s.SessionMu.Unlock()
	if active {
		s.finishChatLoop(sid, epoch, chatLoopStatusError, "agent_error")
	}
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

type chatLoopRunRequest struct {
	sid            string
	epoch          int64
	prompt         string
	expectedStatus string
	phase          string
	summary        string
}

func (s *Server) saveChatLoopRun(req chatLoopRunRequest, token *chatRun, userMsg, pendingMsg chatMessage, startedAtMS int64) (chatSession, bool, *chatLoopState, error) {
	var cs chatSession
	var terminalLoop *chatLoopState
	owned, saveErr := s.saveChatRunPending(req.sid, token, pendingMsg.ID, startedAtMS, func() error {
		s.SessionMu.Lock()
		defer s.SessionMu.Unlock()
		latest, err := loadChatSession(s.CfgStore.Snapshot(), req.sid)
		if err != nil {
			return err
		}
		if !latest.Loop.Enabled || latest.Loop.Epoch != req.epoch || latest.Loop.Status != req.expectedStatus {
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
		phase := req.phase
		if phase == "" {
			phase = "continue"
		}
		appendChatLoopRecord(&latest.Loop, phase, req.summary, "")
		latest.Messages = append(latest.Messages, userMsg, pendingMsg)
		latest.UpdatedAt = time.Now().Unix()
		updateChatTitle(&latest)
		if err := saveChatSessionLocked(s.CfgStore.Snapshot(), latest); err != nil {
			return err
		}
		cs = latest
		return nil
	})
	return cs, owned, terminalLoop, saveErr
}

func (s *Server) launchChatLoopRun(req chatLoopRunRequest, token *chatRun, cs chatSession, userMsg, pendingMsg chatMessage, runStartedAtMS int64) {
	s.publishChatLoopState(req.sid, cs.Loop)
	s.publishChatRun(req.sid, map[string]interface{}{"type": "user", "message": userMsg})
	workerHistory := append([]chatMessage(nil), cs.Messages...)
	for i := len(workerHistory) - 1; i >= 0; i-- {
		if workerHistory[i].ID == userMsg.ID {
			workerHistory = workerHistory[:i]
			break
		}
	}
	cmdReq := map[string]interface{}{
		"prompt":                   req.prompt,
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
	go s.runChatWorkerOwned(req.sid, token, cs, cmdReq)
}

func (s *Server) continueChatLoop(sid string, epoch int64, prompt string) {
	s.queueChatLoopRun(chatLoopRunRequest{
		sid:            sid,
		epoch:          epoch,
		prompt:         prompt,
		expectedStatus: chatLoopStatusEvaluating,
		phase:          "continue",
		summary:        "Controller queued the next step.",
	})
}

func (s *Server) queueChatLoopRun(req chatLoopRunRequest) {
	token := s.beginChatRun(req.sid)
	if token == nil {
		s.deferChatLoopForActiveRun(req.sid, req.epoch)
		return
	}

	runStartedAtMS := time.Now().UnixMilli()
	userMsg := chatMessage{ID: newChatID(), Role: "user", Content: req.prompt, CreatedAt: time.Now().Unix()}
	pendingMsg := chatMessage{ID: newChatID(), Role: "assistant", CreatedAt: time.Now().Unix(), RunStartedAtMS: runStartedAtMS}
	cs, owned, terminalLoop, saveErr := s.saveChatLoopRun(req, token, userMsg, pendingMsg, runStartedAtMS)
	if !owned || saveErr != nil {
		s.endChatRunOwned(req.sid, token)
		if terminalLoop != nil {
			s.publishChatLoopState(req.sid, *terminalLoop)
		}
		if saveErr != nil && !errors.Is(saveErr, errChatLoopStale) {
			s.finishChatLoop(req.sid, req.epoch, chatLoopStatusError, "persist_error: "+saveErr.Error())
		}
		return
	}
	s.launchChatLoopRun(req, token, cs, userMsg, pendingMsg, runStartedAtMS)
}

func (s *Server) deferChatLoopForActiveRun(sid string, epoch int64) {
	s.SessionMu.Lock()
	cs, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err == nil && cs.Loop.Enabled && cs.Loop.Epoch == epoch && (cs.Loop.Status == chatLoopStatusEvaluating || cs.Loop.Status == chatLoopStatusWaiting) {
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

// processNextQueuedMessage executes the first queued message for a session.
// It is called after a chat run completes successfully and there are queued messages.
func (s *Server) processNextQueuedMessage(sid string) bool {
	return s.processQueuedMessage(sid, "")
}

// processQueuedMessage reserves the session, removes the requested queue item,
// persists its pending assistant message, and starts the worker. An empty queueID
// selects the first item. Reserving before dequeueing prevents a competing run
// from causing an item to be removed and later reinserted in the wrong position.
func (s *Server) processQueuedMessage(sid, queueID string) bool {
	sid = safeChatID(sid)
	queueID = strings.TrimSpace(queueID)
	token := s.beginChatRun(sid)
	if token == nil {
		return false
	}

	s.SessionMu.Lock()
	cs, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil || len(cs.QueuedMessages) == 0 {
		s.SessionMu.Unlock()
		s.endChatRunOwned(sid, token)
		return false
	}
	queueIndex := 0
	if queueID != "" {
		queueIndex = -1
		for i := range cs.QueuedMessages {
			if cs.QueuedMessages[i].ID == queueID {
				queueIndex = i
				break
			}
		}
		if queueIndex < 0 {
			s.SessionMu.Unlock()
			s.endChatRunOwned(sid, token)
			return false
		}
	}
	queuedItem := cs.QueuedMessages[queueIndex]
	s.SessionMu.Unlock()

	// Publish the queue identity before removing it from persisted state. Do not
	// take ChatMu while holding SessionMu: saveChatRunPending uses the opposite
	// lock order while atomically publishing the pending assistant identity.
	s.ChatMu.Lock()
	if current := s.ChatRuns[sid]; current != token || current.Done || current.Canceled {
		s.ChatMu.Unlock()
		s.endChatRunOwned(sid, token)
		return false
	}
	token.QueueID = queuedItem.ID
	s.ChatMu.Unlock()

	// Reload under SessionMu because queue replacement may have happened while
	// the lock was released to publish the token identity.
	s.SessionMu.Lock()
	cs, err = loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil {
		s.SessionMu.Unlock()
		s.endChatRunOwned(sid, token)
		return false
	}
	queueIndex = -1
	for i := range cs.QueuedMessages {
		if cs.QueuedMessages[i].ID == queuedItem.ID {
			queueIndex = i
			queuedItem = cs.QueuedMessages[i]
			break
		}
	}
	if queueIndex < 0 {
		s.SessionMu.Unlock()
		s.endChatRunOwned(sid, token)
		return false
	}
	cs.QueuedMessages = append(cs.QueuedMessages[:queueIndex], cs.QueuedMessages[queueIndex+1:]...)
	cs.UpdatedAt = time.Now().Unix()

	pendingID := newChatID()
	runStartedAtMS := time.Now().UnixMilli()
	pendingMsg := chatMessage{
		ID:             pendingID,
		Role:           "assistant",
		CreatedAt:      time.Now().Unix(),
		RunStartedAtMS: runStartedAtMS,
	}
	queuedUserMsg := chatMessage{
		ID:        newChatID(),
		Role:      "user",
		Content:   queuedItem.Text,
		Files:     convertChatUploadsToMaps(queuedItem.Files),
		CreatedAt: time.Now().Unix(),
	}
	cs.Messages = append(cs.Messages, queuedUserMsg, pendingMsg)
	if queuedItem.LLMNo > 0 {
		cs.Settings.LLMNo = queuedItem.LLMNo
	}
	if queuedItem.ReasoningEffort != "" {
		cs.Settings.ReasoningEffort = queuedItem.ReasoningEffort
	}
	workerHistory := append([]chatMessage(nil), cs.Messages...)
	for i := len(workerHistory) - 1; i >= 0; i-- {
		if workerHistory[i].ID == queuedUserMsg.ID {
			workerHistory = workerHistory[:i]
			break
		}
	}
	cmdReq := map[string]interface{}{
		"prompt":                   queuedItem.Text,
		"files":                    queuedItem.Files,
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
		"_ga_pending_assistant_id": pendingID,
		"_ga_run_started_at_ms":    runStartedAtMS,
	}

	// Publish the pending assistant identity together with the persisted session.
	// Reattaching clients use these fields to bind live deltas to the placeholder;
	// without them a guided queue run only appears after the final session reload.
	s.SessionMu.Unlock()
	owned, saveErr := s.saveChatRunPending(sid, token, pendingID, runStartedAtMS, func() error {
		s.SessionMu.Lock()
		defer s.SessionMu.Unlock()
		return saveChatSessionLocked(s.CfgStore.Snapshot(), cs)
	})
	if !owned || saveErr != nil {
		s.endChatRunOwned(sid, token)
		return false
	}
	s.publishChatQueueChanged(sid)

	// Automatic queue consumption bypasses the frontend's optimistic guide path.
	// Publish the persisted user turn on the run stream so attached clients render
	// it immediately; replay and the frontend's message-id dedupe make reconnects safe.
	s.publishChatRun(sid, map[string]interface{}{"type": "user", "message": queuedUserMsg})

	s.ChatMu.Lock()
	if current := s.ChatRuns[sid]; current == token {
		current.PendingAssistantID = pendingID
		current.RunStartedAtMS = runStartedAtMS
	}
	s.ChatMu.Unlock()

	s.publishChatRun(sid, map[string]interface{}{
		"type":            "queue_item_start",
		"queue_item_id":   queuedItem.ID,
		"remaining_count": len(cs.QueuedMessages),
	})
	go s.runChatWorkerOwned(sid, token, cs, cmdReq)
	return true
}
