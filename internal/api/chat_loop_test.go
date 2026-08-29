package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"genericagent-admin-go/internal/config"
)

func newChatLoopTestServer(t *testing.T) *Server {
	t.Helper()
	root := t.TempDir()
	s := newGoalTestServer(t, root)
	updateTestConfig(t, s.CfgStore, func(cfg *config.AppConfig) {
		cfg.ChatDataDir = filepath.Join(root, "chat-data")
	})
	return s
}

func saveChatLoopTestSession(t *testing.T, s *Server, cs chatSession) {
	t.Helper()
	if cs.Title == "" {
		cs.Title = "Loop test"
	}
	if cs.Messages == nil {
		cs.Messages = []chatMessage{}
	}
	if err := saveChatSessionLocked(s.CfgStore.Snapshot(), cs); err != nil {
		t.Fatalf("save chat session: %v", err)
	}
}

func TestProcessNextQueuedMessageStartsAfterCompletedRun(t *testing.T) {
	s := newChatLoopTestServer(t)
	sid := "queue-after-completed-run"
	saveChatLoopTestSession(t, s, chatSession{
		ID:       sid,
		Messages: []chatMessage{{ID: "assistant-old", Role: "assistant", Content: "done"}},
		QueuedMessages: []chatQueuedMessage{{
			ID: "queued-1", Text: "send me next",
		}},
	})

	completed := s.beginChatRun(sid)
	if completed == nil {
		t.Fatal("failed to create completed run fixture")
	}
	s.endChatRunOwned(sid, completed)
	if s.chatRunActive(sid) {
		t.Fatal("completed run is still active")
	}

	s.processNextQueuedMessage(sid)

	s.ChatMu.Lock()
	started := s.ChatRuns[sid]
	s.ChatMu.Unlock()
	if started == nil || started == completed {
		t.Fatalf("queued run token = %p, want a new token after completed %p", started, completed)
	}

	s.ChatMu.Lock()
	events := append([][]byte(nil), started.Events...)
	s.ChatMu.Unlock()
	if len(events) < 2 {
		t.Fatalf("queued run events = %q, want user before queue_item_start", events)
	}
	var userEvent struct {
		Type    string      `json:"type"`
		Message chatMessage `json:"message"`
	}
	if err := json.Unmarshal(events[0], &userEvent); err != nil {
		t.Fatalf("decode queued user event: %v", err)
	}
	if userEvent.Type != "user" || userEvent.Message.ID == "" || userEvent.Message.Role != "user" || userEvent.Message.Content != "send me next" {
		t.Fatalf("queued user event = %#v", userEvent)
	}
	var startEvent struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(events[1], &startEvent); err != nil || startEvent.Type != "queue_item_start" {
		t.Fatalf("event after queued user = %q, decoded=%#v err=%v", events[1], startEvent, err)
	}

	stored, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.QueuedMessages) != 0 {
		t.Fatalf("queued messages = %#v, want consumed", stored.QueuedMessages)
	}
	if len(stored.Messages) < 3 || stored.Messages[len(stored.Messages)-2].Role != "user" || stored.Messages[len(stored.Messages)-2].Content != "send me next" {
		t.Fatalf("messages = %#v, want queued user message followed by pending assistant", stored.Messages)
	}
	if stored.Messages[len(stored.Messages)-2].ID != userEvent.Message.ID {
		t.Fatalf("stream user id = %q, persisted user id = %q", userEvent.Message.ID, stored.Messages[len(stored.Messages)-2].ID)
	}
}

func TestChatGuideCancelsActiveRunAndStartsSelectedQueueItem(t *testing.T) {
	capturedReq := make(chan map[string]interface{}, 1)
	releaseWorker := make(chan struct{})
	oldStart := startChatWorkerFunc
	startChatWorkerFunc = func(config.AppConfig, string) (*chatWorker, error) {
		stdinR, stdinW := io.Pipe()
		stdoutR, stdoutW := io.Pipe()
		go func() {
			defer stdinR.Close()
			defer stdoutW.Close()
			var req map[string]interface{}
			_ = json.NewDecoder(stdinR).Decode(&req)
			capturedReq <- req
			<-releaseWorker
		}()
		return &chatWorker{SID: "guide-interrupts-active-run", Stdin: stdinW, Stdout: stdoutR}, nil
	}
	defer func() {
		close(releaseWorker)
		startChatWorkerFunc = oldStart
	}()

	s := newChatLoopTestServer(t)
	sid := "guide-interrupts-active-run"
	const pendingID = "assistant-active"
	saveChatLoopTestSession(t, s, chatSession{
		ID: sid,
		Messages: []chatMessage{
			{ID: "user-active", Role: "user", Content: "current request", CreatedAt: 1},
			{ID: pendingID, Role: "assistant", CreatedAt: 2, RunStartedAtMS: 1234},
		},
		QueuedMessages: []chatQueuedMessage{
			{ID: "queued-first", Text: "leave me queued"},
			{ID: "queued-guide", Text: "run this guidance now"},
		},
	})

	active := s.beginChatRun(sid)
	if active == nil {
		t.Fatal("failed to create active run")
	}
	s.ChatMu.Lock()
	active.PendingAssistantID = pendingID
	active.RunStartedAtMS = 1234
	s.ChatMu.Unlock()
	s.publishChatRun(sid, map[string]interface{}{"type": "delta", "delta": "partial answer"})

	rr := httptest.NewRecorder()
	s.chatGuidePost(rr, httptest.NewRequest(http.MethodPost, "/api/chat/guide/"+sid+"/queued-guide", nil), sid, "queued-guide")
	if rr.Code != http.StatusOK {
		t.Fatalf("guide status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"started"`) {
		t.Fatalf("guide body=%s, want started", rr.Body.String())
	}
	if !active.Canceled {
		t.Fatal("guide did not cancel the active run")
	}

	s.ChatMu.Lock()
	started := s.ChatRuns[sid]
	s.ChatMu.Unlock()
	if started == nil || started == active {
		t.Fatalf("guide run token = %p, want replacement for %p", started, active)
	}
	defer s.endChatRunOwned(sid, started)

	stored, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.QueuedMessages) != 1 || stored.QueuedMessages[0].ID != "queued-first" {
		t.Fatalf("queued messages = %#v, want untouched first item", stored.QueuedMessages)
	}
	if len(stored.Messages) < 4 {
		t.Fatalf("messages = %#v, want canceled output and guided turn", stored.Messages)
	}
	if got := stored.Messages[len(stored.Messages)-2]; got.Role != "user" || got.Content != "run this guidance now" {
		t.Fatalf("guided user message = %#v", got)
	}
	var canceled chatMessage
	for _, msg := range stored.Messages {
		if msg.ID == pendingID {
			canceled = msg
			break
		}
	}
	if canceled.Content != "partial answer\n\n[\u7528\u6237\u624b\u52a8\u4e2d\u6b62\u751f\u6210]" || !canceled.Error {
		t.Fatalf("canceled partial message = %#v", canceled)
	}

	select {
	case req := <-capturedReq:
		if req["prompt"] != "run this guidance now" {
			t.Fatalf("worker prompt = %#v", req["prompt"])
		}
		history, ok := req["history"].([]interface{})
		if !ok || len(history) != 2 {
			t.Fatalf("worker history = %#v, want interrupted user/assistant pair only", req["history"])
		}
		first, _ := history[0].(map[string]interface{})
		second, _ := history[1].(map[string]interface{})
		if first["content"] != "current request" || second["content"] != "partial answer\n\n[\u7528\u6237\u624b\u52a8\u4e2d\u6b62\u751f\u6210]" {
			t.Fatalf("worker history lost interrupted turn: %#v", history)
		}
		rawHistory, ok := req["raw_history"].([]interface{})
		if !ok || len(rawHistory) != 2 {
			t.Fatalf("worker raw_history = %#v, want interrupted raw turn", req["raw_history"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for guided worker request")
	}
}

func TestParseChatLoopNextPromptUsesLastCompleteTag(t *testing.T) {
	content := "analysis <next_prompt>first</next_prompt> tail <next_prompt>  final action  </next_prompt>"
	if got := parseChatLoopNextPrompt(content); got != "final action" {
		t.Fatalf("parseChatLoopNextPrompt() = %q, want final action", got)
	}
	if got := parseChatLoopNextPrompt("<next_prompt>unfinished"); got != "" {
		t.Fatalf("parseChatLoopNextPrompt(incomplete) = %q, want empty", got)
	}
}

func TestParseChatLoopNextPromptDoesNotIncludeEchoedTemplateTail(t *testing.T) {
	content := `<next_prompt>...</next_prompt> block or do not emit a <next_prompt> tag.
- Keep the next prompt self-contained and focused on the highest-value next action.

<summary>尚未查看桌面，需要主代理执行检查。</summary>
<next_prompt>请检查当前桌面内容，并简要列出可见的文件、文件夹或窗口。</next_prompt>`
	want := "请检查当前桌面内容，并简要列出可见的文件、文件夹或窗口。"
	if got := parseChatLoopNextPrompt(content); got != want {
		t.Fatalf("parseChatLoopNextPrompt(echoed template) = %q, want %q", got, want)
	}
}

func TestChatLoopAdvancePromptDelegatesWithoutPlanning(t *testing.T) {
	if got, want := chatLoopAdvancePrompt, "\u76ee\u6807\u5c1a\u672a\u5b8c\u6210\uff0c\u8bf7\u57fa\u4e8e\u5f53\u524d\u8fdb\u5c55\u81ea\u4e3b\u63a8\u8fdb\u3002"; got != want {
		t.Fatalf("advance prompt = %q, want %q", got, want)
	}
}

func TestParseChatLoopDecisionAcceptsOnlyBinaryVerdicts(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    chatLoopDecision
		wantErr bool
	}{
		{
			name:    "continue",
			content: "<loop_continue>continue</loop_continue>",
			want:    chatLoopDecision{},
		},
		{
			name:    "complete",
			content: "<loop_complete>all work is verified</loop_complete>",
			want:    chatLoopDecision{Complete: true},
		},
		{
			name:    "last verdict wins",
			content: "<loop_complete>template</loop_complete>\n<loop_continue>continue</loop_continue>",
			want:    chatLoopDecision{},
		},
		{
			name:    "planned next action is rejected",
			content: "<next_prompt>inspect the desktop</next_prompt>",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseChatLoopDecision(tt.content)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseChatLoopDecision() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("parseChatLoopDecision() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNormalizePersistedChatLoopPausesActiveState(t *testing.T) {
	state := chatLoopState{
		Enabled:          true,
		Status:           chatLoopStatusEvaluating,
		Epoch:            7,
		Round:            3,
		MaxRounds:        0,
		ControllerPrompt: "ship it",
	}
	got, changed := normalizePersistedChatLoop(state)
	if !changed {
		t.Fatal("normalizePersistedChatLoop() did not report a change")
	}
	if got.Enabled || got.Status != chatLoopStatusPaused || got.StopReason != "server_restart" {
		t.Fatalf("normalized state = %#v, want disabled paused/server_restart", got)
	}
	if got.Epoch != 8 || got.Round != 3 || got.MaxRounds != chatLoopDefaultMaxRounds || got.ControllerPrompt != state.ControllerPrompt {
		t.Fatalf("normalized state lost progress: %#v", got)
	}

	inactive := chatLoopState{Status: chatLoopStatusCompleted, Epoch: 4, MaxRounds: 5}
	if same, changed := normalizePersistedChatLoop(inactive); changed || !reflect.DeepEqual(same, inactive) {
		t.Fatalf("inactive state changed: got %#v changed=%v", same, changed)
	}
}

func TestChatLoopStartAndStopAPI(t *testing.T) {
	s := newChatLoopTestServer(t)
	sid := "loop-start-stop"
	saveChatLoopTestSession(t, s, chatSession{ID: sid})

	start := httptest.NewRecorder()
	startReq := httptest.NewRequest(http.MethodPost, "/api/chat/loop/"+sid+"/start", bytes.NewBufferString(`{"objective":"Finish the release","max_rounds":999}`))
	startReq.Header.Set("Content-Type", "application/json")
	s.chatHandler(start, startReq)
	if start.Code != http.StatusOK {
		t.Fatalf("start status = %d: %s", start.Code, start.Body.String())
	}
	var startPayload struct {
		Loop chatLoopState `json:"loop"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &startPayload); err != nil {
		t.Fatal(err)
	}
	if !startPayload.Loop.Enabled || startPayload.Loop.Status != chatLoopStatusWaiting || startPayload.Loop.MaxRounds != chatLoopMaxRounds {
		t.Fatalf("start loop = %#v", startPayload.Loop)
	}
	if startPayload.Loop.ControllerPrompt != "Finish the release" || startPayload.Loop.Epoch != 1 {
		t.Fatalf("start loop metadata = %#v", startPayload.Loop)
	}

	stop := httptest.NewRecorder()
	s.chatHandler(stop, httptest.NewRequest(http.MethodPost, "/api/chat/loop/"+sid+"/stop", nil))
	if stop.Code != http.StatusOK {
		t.Fatalf("stop status = %d: %s", stop.Code, stop.Body.String())
	}
	persisted, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Loop.Enabled || persisted.Loop.Status != chatLoopStatusStopped || persisted.Loop.StopReason != "user" {
		t.Fatalf("persisted stopped loop = %#v", persisted.Loop)
	}
	if persisted.Loop.Epoch != 2 {
		t.Fatalf("stopped epoch = %d, want 2", persisted.Loop.Epoch)
	}
}

func TestChatLoopStateAppearsInSessionAPIs(t *testing.T) {
	s := newChatLoopTestServer(t)
	sid := "loop-projection"
	want := chatLoopState{
		Enabled:          true,
		Status:           chatLoopStatusRunning,
		Epoch:            6,
		Round:            2,
		MaxRounds:        12,
		ControllerPrompt: "complete rollout",
	}
	saveChatLoopTestSession(t, s, chatSession{ID: sid, UpdatedAt: 42, Loop: want})

	stateRec := httptest.NewRecorder()
	s.chatState(stateRec, httptest.NewRequest(http.MethodGet, "/api/chat/state/"+sid, nil), sid)
	if stateRec.Code != http.StatusOK {
		t.Fatalf("state status = %d: %s", stateRec.Code, stateRec.Body.String())
	}
	var statePayload struct {
		Loop chatLoopState `json:"loop"`
	}
	if err := json.Unmarshal(stateRec.Body.Bytes(), &statePayload); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(statePayload.Loop, want) {
		t.Fatalf("state loop = %#v, want %#v", statePayload.Loop, want)
	}

	listRec := httptest.NewRecorder()
	s.chatSessions(listRec, httptest.NewRequest(http.MethodGet, "/api/chat/sessions", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("sessions status = %d: %s", listRec.Code, listRec.Body.String())
	}
	var listPayload struct {
		Sessions []struct {
			ID   string        `json:"id"`
			Loop chatLoopState `json:"loop"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listPayload); err != nil {
		t.Fatal(err)
	}
	if len(listPayload.Sessions) != 1 || listPayload.Sessions[0].ID != sid || !reflect.DeepEqual(listPayload.Sessions[0].Loop, want) {
		t.Fatalf("sessions payload = %#v", listPayload.Sessions)
	}
}

func TestChatStateShipsActiveRunIdentity(t *testing.T) {
	s := newChatLoopTestServer(t)
	sid := "state-run-identity"
	token := s.beginChatRun(sid)
	if token == nil {
		t.Fatal("beginChatRun returned nil")
	}
	const pendingID = "assistant-pending"
	const startedAtMS int64 = 1787725441243
	owned, err := s.saveChatRunPending(sid, token, pendingID, startedAtMS, func() error { return nil })
	if err != nil || !owned {
		t.Fatalf("saveChatRunPending owned=%v err=%v", owned, err)
	}

	rec := httptest.NewRecorder()
	s.chatState(rec, httptest.NewRequest(http.MethodGet, "/api/chat/state/"+sid, nil), sid)
	if rec.Code != http.StatusOK {
		t.Fatalf("state status = %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Running            bool   `json:"running"`
		PendingAssistantID string `json:"pending_assistant_id"`
		RunStartedAtMS     int64  `json:"run_started_at_ms"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Running || payload.PendingAssistantID != pendingID || payload.RunStartedAtMS != startedAtMS {
		t.Fatalf("state run identity = %#v", payload)
	}
}

func TestRecoverChatLoopsAfterRestartRunsOncePerRuntime(t *testing.T) {
	s := newChatLoopTestServer(t)
	firstID := "loop-recover-first"
	saveChatLoopTestSession(t, s, chatSession{ID: firstID, Loop: chatLoopState{
		Enabled: true, Status: chatLoopStatusRunning, Epoch: 2, Round: 1, MaxRounds: 4,
	}})

	if err := s.recoverChatLoopsAfterRestart(); err != nil {
		t.Fatal(err)
	}
	first, err := loadChatSession(s.CfgStore.Snapshot(), firstID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Loop.Enabled || first.Loop.Status != chatLoopStatusPaused || first.Loop.StopReason != "server_restart" || first.Loop.Epoch != 3 {
		t.Fatalf("recovered loop = %#v", first.Loop)
	}

	secondID := "loop-created-after-recovery"
	secondWant := chatLoopState{Enabled: true, Status: chatLoopStatusRunning, Epoch: 9, MaxRounds: 5}
	saveChatLoopTestSession(t, s, chatSession{ID: secondID, Loop: secondWant})
	if err := s.recoverChatLoopsAfterRestart(); err != nil {
		t.Fatal(err)
	}
	second, err := loadChatSession(s.CfgStore.Snapshot(), secondID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(second.Loop, secondWant) {
		t.Fatalf("second recovery reran: loop = %#v, want %#v", second.Loop, secondWant)
	}
}

func TestStreamChatRunExposesRunIdentity(t *testing.T) {
	s := newChatLoopTestServer(t)
	sid := "loop-stream-identity"
	s.ChatMu.Lock()
	s.ChatRuns[sid] = &chatRun{
		SID:                sid,
		Done:               true,
		PendingAssistantID: "assistant-loop-round-2",
		RunStartedAtMS:     987654321,
		Events:             [][]byte{[]byte(`{"type":"done"}`)},
	}
	s.ChatMu.Unlock()

	rec := httptest.NewRecorder()
	s.streamChatRun(rec, httptest.NewRequest(http.MethodGet, "/api/chat/stream/"+sid, nil), sid, 0)
	if got := rec.Header().Get("X-Chat-Pending-ID"); got != "assistant-loop-round-2" {
		t.Fatalf("X-Chat-Pending-ID = %q", got)
	}
	if got := rec.Header().Get("X-Chat-Run-Started-At-Ms"); got != "987654321" {
		t.Fatalf("X-Chat-Run-Started-At-Ms = %q", got)
	}
	if got := rec.Body.String(); got != "{\"type\":\"done\"}\n" {
		t.Fatalf("stream body = %q", got)
	}
}

func TestLateTerminalSavePreservesLatestChatLoopState(t *testing.T) {
	s := newChatLoopTestServer(t)
	sid := "loop-late-terminal"
	oldSnapshot := chatSession{
		ID:       sid,
		Title:    "Loop",
		Messages: []chatMessage{{ID: "assistant-old", Role: "assistant", Content: "done"}},
		Loop:     chatLoopState{Enabled: true, Status: chatLoopStatusRunning, Epoch: 3, Round: 1, MaxRounds: 8},
	}
	latestLoop := chatLoopState{Enabled: false, Status: chatLoopStatusStopped, Epoch: 4, Round: 1, MaxRounds: 8, StopReason: "user_stopped"}
	latest := oldSnapshot
	latest.Loop = latestLoop
	saveChatLoopTestSession(t, s, latest)

	if err := s.saveChatSessionMerged(oldSnapshot); err != nil {
		t.Fatal(err)
	}
	persisted, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted.Loop, latestLoop) {
		t.Fatalf("late terminal save regressed loop = %#v, want %#v", persisted.Loop, latestLoop)
	}
}

func TestAppendChatLoopRecordBoundsHistoryAndUnicode(t *testing.T) {
	state := chatLoopState{}
	longSummary := strings.Repeat("界", maxChatLoopRecordTextRunes+9)
	for i := 0; i < maxChatLoopRecords+5; i++ {
		state.Round = i
		appendChatLoopRecord(&state, "checking", longSummary, "next")
	}
	if len(state.Records) != maxChatLoopRecords {
		t.Fatalf("records length = %d, want %d", len(state.Records), maxChatLoopRecords)
	}
	if state.Records[0].Round != 5 || state.Records[len(state.Records)-1].Round != maxChatLoopRecords+4 {
		t.Fatalf("bounded records kept wrong rounds: first=%d last=%d", state.Records[0].Round, state.Records[len(state.Records)-1].Round)
	}
	wantSummary := strings.Repeat("界", maxChatLoopRecordTextRunes) + "..."
	if got := state.Records[0].Summary; got != wantSummary {
		t.Fatalf("bounded Unicode summary = %q, want %q", got, wantSummary)
	}
	if got := len([]rune(state.Records[0].Summary)); got != maxChatLoopRecordTextRunes+3 {
		t.Fatalf("bounded Unicode summary rune count = %d", got)
	}

	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"records"`)) {
		t.Fatalf("serialized loop omitted records: %s", encoded)
	}
}

func TestFinishChatLoopRecordDoesNotExposeControllerOutput(t *testing.T) {
	s := newChatLoopTestServer(t)
	sid := "loop-record-redaction"
	const secretControllerOutput = "RAW_CONTROLLER_OUTPUT_MUST_NOT_REACH_RECORDS"
	saveChatLoopTestSession(t, s, chatSession{
		ID:       sid,
		Messages: []chatMessage{},
		Loop: chatLoopState{
			Enabled:   true,
			Status:    chatLoopStatusEvaluating,
			Epoch:     7,
			MaxRounds: 10,
		},
	})

	// The terminal helper receives diagnostic reasons, but the user-facing
	// observer record must remain a fixed verdict summary rather than exposing
	// controller output or hidden reasoning.
	s.finishChatLoop(sid, 7, chatLoopStatusError, "controller_error: "+secretControllerOutput)
	persisted, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.Loop.Records) != 1 {
		t.Fatalf("terminal records = %#v", persisted.Loop.Records)
	}
	record := persisted.Loop.Records[0]
	if record.Phase != "error" || record.Summary != "Controller evaluation failed." || record.Prompt != "" {
		t.Fatalf("terminal record = %#v", record)
	}
	recordsJSON, err := json.Marshal(persisted.Loop.Records)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(recordsJSON, []byte(secretControllerOutput)) {
		t.Fatalf("controller output leaked into observer records: %s", recordsJSON)
	}
}
