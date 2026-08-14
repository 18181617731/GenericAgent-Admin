package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestParseChatLoopDecisionUsesLastNonEmptyDecisionElement(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    chatLoopDecision
	}{
		{
			name:    "prompt after echoed completion template",
			content: "Valid shapes: <loop_complete>brief reason</loop_complete>\n<next_prompt>inspect the desktop</next_prompt>",
			want:    chatLoopDecision{Prompt: "inspect the desktop"},
		},
		{
			name:    "completion after earlier prompt",
			content: "Earlier: <next_prompt>inspect the desktop</next_prompt>\n<loop_complete>all files listed</loop_complete>",
			want:    chatLoopDecision{Complete: true},
		},
		{
			name:    "empty trailing completion is ignored",
			content: "<next_prompt>inspect the desktop</next_prompt><loop_complete> </loop_complete>",
			want:    chatLoopDecision{Prompt: "inspect the desktop"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseChatLoopDecision(tt.content)
			if err != nil {
				t.Fatalf("parseChatLoopDecision() error = %v", err)
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

	type firstRoundCall struct {
		sid    string
		epoch  int64
		prompt string
	}
	calls := make(chan firstRoundCall, 1)
	oldContinue := continueChatLoopFunc
	defer func() { continueChatLoopFunc = oldContinue }()
	continueChatLoopFunc = func(_ *Server, gotSID string, epoch int64, prompt string) {
		calls <- firstRoundCall{sid: gotSID, epoch: epoch, prompt: prompt}
	}

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
	if !startPayload.Loop.Enabled || startPayload.Loop.Status != chatLoopStatusEvaluating || startPayload.Loop.MaxRounds != chatLoopMaxRounds {
		t.Fatalf("start loop = %#v", startPayload.Loop)
	}
	if startPayload.Loop.ControllerPrompt != "Finish the release" || startPayload.Loop.Epoch != 1 {
		t.Fatalf("start loop metadata = %#v", startPayload.Loop)
	}
	select {
	case call := <-calls:
		if call.sid != sid || call.epoch != 1 || call.prompt != "Finish the release" {
			t.Fatalf("first round call = %#v", call)
		}
	case <-time.After(time.Second):
		t.Fatal("empty chat loop did not schedule its first worker round")
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

func TestChatLoopPromptFingerprintIgnoresCaseAndSpacing(t *testing.T) {
	base := chatLoopPromptFingerprint("Run the failing tests again")
	if base == "" {
		t.Fatal("fingerprint of a real prompt is empty")
	}
	if got := chatLoopPromptFingerprint("  run   the failing\ttests\nagain "); got != base {
		t.Fatalf("fingerprint is sensitive to case or spacing: %q vs %q", got, base)
	}
	if got := chatLoopPromptFingerprint("run the passing tests again"); got == base {
		t.Fatal("different prompts share a fingerprint")
	}
	if got := chatLoopPromptFingerprint("   "); got != "" {
		t.Fatalf("blank prompt fingerprint = %q, want empty", got)
	}
	if strings.Contains(base, "run") {
		t.Fatalf("fingerprint leaks prompt text: %q", base)
	}
}

func TestEvaluateChatLoopRetriesUnusableControllerReply(t *testing.T) {
	s := newChatLoopTestServer(t)
	sid := "loop-controller-retry"
	const unusableReply = "Sure, I think the agent should probably keep going for a while."
	cs := chatSession{ID: sid, Loop: chatLoopState{
		Enabled:          true,
		Status:           chatLoopStatusEvaluating,
		Epoch:            3,
		Round:            1,
		MaxRounds:        5,
		ControllerPrompt: "ship the release",
	}}
	saveChatLoopTestSession(t, s, cs)

	old := runOneShotBTWWorkerFunc
	defer func() { runOneShotBTWWorkerFunc = old }()
	var prompts []string
	runOneShotBTWWorkerFunc = func(_ config.AppConfig, _ string, req map[string]interface{}) (chatMessage, error) {
		prompt, _ := req["prompt"].(string)
		prompts = append(prompts, prompt)
		if len(prompts) == 1 {
			return chatMessage{Content: unusableReply}, nil
		}
		return chatMessage{Content: "<loop_complete>everything shipped</loop_complete>"}, nil
	}

	s.evaluateChatLoop(sid, 3, cs)

	if len(prompts) != 2 {
		t.Fatalf("controller calls = %d, want 2", len(prompts))
	}
	if strings.Contains(prompts[0], "previous reply was rejected") {
		t.Fatalf("first attempt already carried the corrective instruction: %q", prompts[0])
	}
	if !strings.Contains(prompts[1], "previous reply was rejected") {
		t.Fatalf("retry attempt lost the corrective instruction: %q", prompts[1])
	}
	persisted, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Loop.Enabled || persisted.Loop.Status != chatLoopStatusCompleted || persisted.Loop.StopReason != "controller_complete" {
		t.Fatalf("loop after successful retry = %#v", persisted.Loop)
	}
	retries := 0
	for _, record := range persisted.Loop.Records {
		if record.Phase == "retry" {
			retries++
			if record.Summary != "Controller reply was unusable; asking once more." || record.Prompt != "" {
				t.Fatalf("retry record = %#v", record)
			}
		}
	}
	if retries != 1 {
		t.Fatalf("retry records = %d, want 1: %#v", retries, persisted.Loop.Records)
	}
	recordsJSON, err := json.Marshal(persisted.Loop.Records)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(recordsJSON, []byte(unusableReply)) {
		t.Fatalf("controller output leaked into observer records: %s", recordsJSON)
	}
}

func TestEvaluateChatLoopFailsAfterRetryBudget(t *testing.T) {
	s := newChatLoopTestServer(t)
	sid := "loop-controller-retry-budget"
	cs := chatSession{ID: sid, Loop: chatLoopState{
		Enabled:          true,
		Status:           chatLoopStatusEvaluating,
		Epoch:            2,
		MaxRounds:        5,
		ControllerPrompt: "ship the release",
	}}
	saveChatLoopTestSession(t, s, cs)

	old := runOneShotBTWWorkerFunc
	defer func() { runOneShotBTWWorkerFunc = old }()
	calls := 0
	runOneShotBTWWorkerFunc = func(config.AppConfig, string, map[string]interface{}) (chatMessage, error) {
		calls++
		return chatMessage{Content: "still not following the protocol"}, nil
	}

	s.evaluateChatLoop(sid, 2, cs)

	if calls != chatLoopControllerAttempts {
		t.Fatalf("controller calls = %d, want %d", calls, chatLoopControllerAttempts)
	}
	persisted, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Loop.Enabled || persisted.Loop.Status != chatLoopStatusError {
		t.Fatalf("loop after exhausted retries = %#v", persisted.Loop)
	}
	if !strings.HasPrefix(persisted.Loop.StopReason, "controller_protocol_error") {
		t.Fatalf("stop reason = %q", persisted.Loop.StopReason)
	}
}

func TestContinueChatLoopStopsARepeatingController(t *testing.T) {
	s := newChatLoopTestServer(t)
	sid := "loop-controller-stalled"
	const repeated = "Run the failing tests again"
	saveChatLoopTestSession(t, s, chatSession{ID: sid, Loop: chatLoopState{
		Enabled:               true,
		Status:                chatLoopStatusEvaluating,
		Epoch:                 5,
		Round:                 2,
		MaxRounds:             50,
		ControllerPrompt:      "make the suite green",
		LastPromptFingerprint: chatLoopPromptFingerprint(repeated),
		RepeatStreak:          chatLoopMaxPromptRepeats - 1,
	}})

	s.continueChatLoop(sid, 5, "  run the FAILING tests   again ")

	persisted, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Loop.Enabled || persisted.Loop.Status != chatLoopStatusStopped || persisted.Loop.StopReason != "controller_stalled" {
		t.Fatalf("stalled loop = %#v", persisted.Loop)
	}
	if persisted.Loop.Round != 2 {
		t.Fatalf("stalled loop consumed a round: %#v", persisted.Loop)
	}
	if persisted.Loop.Epoch != 6 {
		t.Fatalf("stalled loop epoch = %d, want 6", persisted.Loop.Epoch)
	}
	if len(persisted.Messages) != 0 {
		t.Fatalf("stalled loop queued messages: %#v", persisted.Messages)
	}
	last := persisted.Loop.Records[len(persisted.Loop.Records)-1]
	if last.Phase != "stalled" || last.Prompt != "" {
		t.Fatalf("stalled record = %#v", last)
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
