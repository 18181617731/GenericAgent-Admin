package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/modelconfig"
	"genericagent-admin-go/internal/service"
)

func newChatCommandTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Store{Root: t.TempDir(), Cfg: config.Default()}
	cfg.Cfg.GARoot = t.TempDir()
	cfg.Cfg.ChatDataDir = t.TempDir()
	return New(cfg, service.NewManager(cfg.Cfg.GARoot, cfg.Cfg.BufferLines), modelconfig.NewStore(t.TempDir()), nil)
}

func TestParseImmediateChatCommand(t *testing.T) {
	tests := []struct {
		prompt string
		known  bool
		name   string
		mode   string
		count  int
		names  []string
		err    bool
	}{
		{" /scheduler ", true, "/scheduler", "list", 1, nil, false},
		{"/SCHEDULER run alpha beta", true, "/scheduler", "start", 1, []string{"alpha", "beta"}, false},
		{"/scheduler start", true, "/scheduler", "start", 1, nil, true},
		{"/rewind", true, "/rewind", "", 1, nil, false},
		{"/rewind 2", true, "/rewind", "", 2, nil, false},
		{"/rewind zero", true, "/rewind", "", 1, nil, true},
		{"/rewind 0", true, "/rewind", "", 1, nil, true},
		{"/clear extra", true, "/clear", "", 1, nil, true},
		{"/export", true, "/export", "last", 1, nil, false},
		{"/export all transcript", true, "/export", "all", 1, nil, false},
		{"/help", true, "/help", "", 1, nil, false},
		{"/status", true, "/status", "", 1, nil, false},
		{"/verbose", true, "/verbose", "", 1, nil, false},
		{"/resume", false, "/resume", "", 1, nil, false},
		{"/unknown x", false, "", "", 0, nil, false},
		{"normal prompt", false, "", "", 0, nil, false},
	}
	for _, tt := range tests {
		t.Run(strings.ReplaceAll(tt.prompt, "/", "_"), func(t *testing.T) {
			got, known, err := parseImmediateChatCommand(tt.prompt)
			if known != tt.known || (err != nil) != tt.err || got.Name != tt.name || got.Mode != tt.mode || got.Count != tt.count || !reflect.DeepEqual(got.Names, tt.names) {
				t.Fatalf("got=%+v known=%v err=%v", got, known, err)
			}
		})
	}
}

func TestRewindSessionProjectionAndNoMutationOnRangeError(t *testing.T) {
	original := chatSession{
		Messages:    []chatMessage{{ID: "u1", Role: "user", Content: "one"}, {ID: "a1", Role: "assistant", Content: "A"}, {ID: "u2", Role: "user", Content: "two"}, {ID: "a2", Role: "assistant", Content: "B"}},
		RawHistory:  []map[string]interface{}{{"role": "user", "content": "one"}, {"role": "assistant", "tool_calls": []interface{}{map[string]interface{}{"id": "c1"}}}, {"role": "tool", "tool_call_id": "c1", "content": "secret"}, {"role": "assistant", "content": "A"}, {"role": "user", "content": "two"}, {"role": "assistant", "content": "B"}},
		HistoryInfo: []interface{}{map[string]interface{}{"old": true}}, Working: map[string]interface{}{"active": true},
	}
	bad := original
	bad.Messages = append([]chatMessage(nil), original.Messages...)
	bad.RawHistory = append([]map[string]interface{}(nil), original.RawHistory...)
	before, _ := json.Marshal(bad)
	if err := rewindSession(&bad, 3, map[string]interface{}{}); err == nil {
		t.Fatal("expected out-of-range error")
	}
	after, _ := json.Marshal(bad)
	if string(before) != string(after) {
		t.Fatalf("out-of-range mutation: before=%s after=%s", before, after)
	}

	out := map[string]interface{}{}
	if err := rewindSession(&original, 2, out); err != nil {
		t.Fatal(err)
	}
	if out["prefill"] != "one" || len(original.Messages) != 0 || len(original.RawHistory) != 0 || len(original.HistoryInfo) != 0 || original.Working != nil {
		t.Fatalf("projection=%+v result=%+v", original, out)
	}
}

func TestMutateChatSessionPersistsAndRequiresOwnedToken(t *testing.T) {
	s := newChatCommandTestServer(t)
	sid := "command-owner"
	seed := chatSession{ID: sid, Title: "kept", Messages: []chatMessage{{ID: "u", Role: "user", Content: "prefill"}, {ID: "a", Role: "assistant", Content: "answer"}}, RawHistory: []map[string]interface{}{{"role": "user", "content": "prefill"}, {"role": "assistant", "content": "answer"}}, Settings: chatSettings{LLMNo: 2}, Workspace: "ws", ProjectMode: "proj"}
	if err := saveChatSession(s.CfgStore.Cfg, seed); err != nil {
		t.Fatal(err)
	}

	foreign := s.beginChatRun(sid)
	if foreign == nil {
		t.Fatal("missing token")
	}
	wrong := &chatRun{SID: sid}
	if _, err := s.mutateChatSession(sid, wrong, func(cs *chatSession) error { cs.Title = "bad"; return nil }); err == nil {
		t.Fatal("foreign token mutated session")
	}
	out := map[string]interface{}{}
	got, err := s.mutateChatSession(sid, foreign, func(cs *chatSession) error { return rewindSession(cs, 1, out) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 0 || got.Title != "kept" || got.Settings.LLMNo != 2 || got.Workspace != "ws" || got.ProjectMode != "proj" {
		t.Fatalf("got=%+v", got)
	}
	reloaded, err := loadChatSession(s.CfgStore.Cfg, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Messages) != 0 || len(reloaded.RawHistory) != 0 || reloaded.Workspace != "ws" || reloaded.ProjectMode != "proj" {
		t.Fatalf("reloaded=%+v", reloaded)
	}
	s.endChatRunOwned(sid, foreign)
	if next := s.beginChatRun(sid); next == nil {
		t.Fatal("reservation was not released")
	}
}

func TestMutationReservationBlocksCompetingRunAtSessionBarrier(t *testing.T) {
	s := newChatCommandTestServer(t)
	sid := "command-barrier"
	if err := saveChatSession(s.CfgStore.Cfg, chatSession{ID: sid, Messages: []chatMessage{{ID: "u", Role: "user", Content: "keep"}}}); err != nil {
		t.Fatal(err)
	}
	token := s.beginChatRun(sid)
	if token == nil {
		t.Fatal("missing command reservation")
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	s.chatSessionMutationHook = func() {
		close(entered)
		<-release
	}
	done := make(chan error, 1)
	go func() {
		_, err := s.mutateChatSession(sid, token, func(cs *chatSession) error {
			cs.Messages = nil
			return nil
		})
		done <- err
	}()
	<-entered
	if competing := s.beginChatRun(sid); competing != nil {
		close(release)
		t.Fatal("competing run started while command mutation held its reservation")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	s.endChatRunOwned(sid, token)
	if next := s.beginChatRun(sid); next == nil {
		t.Fatal("command reservation was not released")
	} else {
		s.endChatRunOwned(sid, next)
	}
}

func TestRunOwnershipDoesNotEndReplacement(t *testing.T) {
	s := newChatCommandTestServer(t)
	first := s.beginChatRun("same")
	if first == nil || s.beginChatRun("same") != nil {
		t.Fatal("active reservation not exclusive")
	}
	s.endChatRunOwned("same", first)
	second := s.beginChatRun("same")
	if second == nil {
		t.Fatal("replacement reservation failed")
	}
	s.endChatRunOwned("same", first)
	if !s.ownsChatRun("same", second) {
		t.Fatal("stale cleanup ended replacement")
	}
}

func TestChatCancelEndsImmediateRunAndReleasesSubscriber(t *testing.T) {
	s := newChatCommandTestServer(t)
	sid := "cancel-immediate"
	token := s.beginChatRun(sid)
	if token == nil {
		t.Fatal("missing run")
	}
	ch := make(chan []byte, 1)
	s.ChatMu.Lock()
	token.Subscribers[ch] = true
	s.ChatMu.Unlock()

	rr := httptest.NewRecorder()
	s.chatCancel(rr, httptest.NewRequest(http.MethodPost, "/api/chat/cancel/"+sid, nil), sid)
	if rr.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("subscriber received a value instead of closure")
		}
	default:
		t.Fatal("cancel did not close immediate run subscriber")
	}
	if s.ownsChatRun(sid, token) {
		t.Fatal("canceled run still owns session")
	}
	if next := s.beginChatRun(sid); next == nil {
		t.Fatal("cancel did not release session reservation")
	} else {
		s.endChatRunOwned(sid, next)
	}
}

func TestImmediateCommandCanceledBeforePublishHasNoResult(t *testing.T) {
	s := newChatCommandTestServer(t)
	sid := "canceled-command"
	token := s.beginChatRun(sid)
	if token == nil {
		t.Fatal("missing run")
	}
	s.endChatRunOwned(sid, token)

	rr := httptest.NewRecorder()
	if !s.maybeHandleImmediateChatCommand(rr, httptest.NewRequest(http.MethodPost, "/api/chat/"+sid, nil), sid, token, immediateChatCommand{Name: "/help"}) {
		t.Fatal("immediate command was not handled")
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("canceled command wrote response: %s", rr.Body.String())
	}
	s.ChatMu.Lock()
	events := append([][]byte(nil), token.Events...)
	s.ChatMu.Unlock()
	if len(events) != 0 {
		t.Fatalf("canceled command published events: %q", events)
	}
}

func TestCommandCatalogAndVerboseRedaction(t *testing.T) {
	seen := map[string]bool{}
	for _, row := range immediateCommandCatalog() {
		syntax := row["syntax"].(string)
		seen[strings.Fields(syntax)[0]] = true
	}
	for _, name := range []string{"/scheduler", "/resume", "/rewind", "/clear", "/export", "/help", "/status", "/verbose", "/btw"} {
		if !seen[name] {
			t.Fatalf("catalog missing %s", name)
		}
	}
	records := verboseRecords([]map[string]interface{}{{"role": "tool", "tool_call_id": "x", "arguments": "api_key=abc", "content": "token: xyz"}})
	b, _ := json.Marshal(records)
	if strings.Contains(string(b), "abc") || strings.Contains(string(b), "xyz") || !strings.Contains(string(b), "[REDACTED]") {
		t.Fatalf("unsafe verbose output: %s", b)
	}
}

func TestParseWorldlineCommand(t *testing.T) {
	cases := []struct {
		in, mode, arg, restoreMode, to string
		danger                         bool
	}{
		{"/worldline", "list", "", "", "", false},
		{"/worldline restore node-1", "restore", "node-1", "both", "at", true},
		{"/worldline restore node-2 conversation before", "restore", "node-2", "conversation", "before", true},
	}
	for _, tc := range cases {
		c, ok, err := parseImmediateChatCommand(tc.in)
		if err != nil || !ok {
			t.Fatalf("parse %q: ok=%v err=%v", tc.in, ok, err)
		}
		if c.Mode != tc.mode || c.Arg != tc.arg || c.RestoreMode != tc.restoreMode || c.To != tc.to || commandNeedsDanger(c) != tc.danger {
			t.Fatalf("parse %q = %#v", tc.in, c)
		}
	}
	for _, bad := range []string{"/worldline nope", "/worldline restore", "/worldline restore n bad", "/worldline restore n both bad"} {
		if _, ok, err := parseImmediateChatCommand(bad); !ok || err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestVisibleMessagesFromRaw(t *testing.T) {
	raw := []map[string]interface{}{{"role": "user", "content": []interface{}{map[string]interface{}{"type": "text", "text": "question"}}}, {"role": "tool", "content": "hidden"}, {"role": "assistant", "content": "answer"}}
	got := visibleMessagesFromRaw(raw)
	if len(got) != 2 || got[0].Content != "question" || got[1].Content != "answer" {
		t.Fatalf("messages: %#v", got)
	}
}

func TestWorldlineRestorePersistsRestoredSession(t *testing.T) {
	s := newChatCommandTestServer(t)
	const sid = "worldline-restore"
	seed := chatSession{
		ID:    sid,
		Title: "Restore test",
		Messages: []chatMessage{
			{ID: "old-u", Role: "user", Content: "old question", CreatedAt: 1},
			{ID: "old-a", Role: "assistant", Content: "old answer", CreatedAt: 2},
		},
		RawHistory: []map[string]interface{}{
			{"role": "user", "content": "old question"},
			{"role": "assistant", "content": "old answer"},
		},
	}
	if err := saveChatSession(s.CfgStore.Cfg, seed); err != nil {
		t.Fatal(err)
	}

	restoredRaw := []map[string]interface{}{
		{"role": "user", "content": []interface{}{map[string]interface{}{"type": "text", "text": "restored question"}}},
		{"role": "assistant", "content": "restored answer"},
	}
	requests := make(chan map[string]interface{}, 1)
	oldStart := startChatWorkerFunc
	startChatWorkerFunc = func(config.AppConfig, string) (*chatWorker, error) {
		stdinR, stdinW := io.Pipe()
		stdoutR, stdoutW := io.Pipe()
		go func() {
			defer stdinR.Close()
			defer stdoutW.Close()
			var req map[string]interface{}
			if json.NewDecoder(stdinR).Decode(&req) == nil {
				requests <- req
			}
			_ = json.NewEncoder(stdoutW).Encode(map[string]interface{}{
				"type":         "worldline",
				"action":       "restore",
				"tree":         map[string]interface{}{"nodes": []interface{}{}},
				"result":       map[string]interface{}{"node_id": "node-1"},
				"raw_history":  restoredRaw,
				"history_info": []interface{}{map[string]interface{}{"restored": true}},
				"working":      map[string]interface{}{"phase": "restored"},
			})
		}()
		return &chatWorker{SID: sid, Stdin: stdinW, Stdout: stdoutR}, nil
	}
	defer func() { startChatWorkerFunc = oldStart }()

	req := httptest.NewRequest(http.MethodPost, "/api/chat/"+sid, strings.NewReader(`{"prompt":"/worldline restore node-1 conversation before"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GA-Confirm", "dangerous")
	rr := httptest.NewRecorder()
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"type":"command_result"`) {
		t.Fatalf("restore status=%d body=%s", rr.Code, rr.Body.String())
	}
	workerReq := <-requests
	if workerReq["action"] != "restore" || workerReq["sid"] != sid || workerReq["node_id"] != "node-1" || workerReq["mode"] != "conversation" || workerReq["to"] != "before" {
		t.Fatalf("worker request: %#v", workerReq)
	}

	got, err := loadChatSession(s.CfgStore.Cfg, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 2 || got.Messages[0].Content != "restored question" || got.Messages[1].Content != "restored answer" {
		t.Fatalf("restored messages were not persisted: %#v", got.Messages)
	}
	if got.Working["phase"] != "restored" || len(got.HistoryInfo) != 1 {
		t.Fatalf("restored state was not persisted: history_info=%#v working=%#v", got.HistoryInfo, got.Working)
	}
}
