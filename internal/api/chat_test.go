package api

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"genericagent-admin-go/internal/config"
)

func TestParseLLMJSONArrayFromMixedOutputIgnoresGAStartupLogs(t *testing.T) {
	out := []byte("[ContextGuard] installed\r\n[MemoryLauncher] native\r\n[Info] Load mykeys from E:\\AITools\\GenericAgent\\mykey.py\r\n" +
		`[{"index":0,"label":"NativeOAISession/gpt-5.5/cpa","name":"gpt-5.5/cpa","model":"cpa","active":true},{"index":1,"label":"NativeOAISession/deepseek-v4-pro/newapi","name":"deepseek-v4-pro/newapi","model":"newapi","active":false}]` +
		"\r\n[DelegationHintGuard] installed")

	llms, err := parseLLMJSONArrayFromMixedOutput(out)
	if err != nil {
		t.Fatalf("parse mixed GA output: %v", err)
	}
	if len(llms) != 2 {
		t.Fatalf("len(llms)=%d want=2: %#v", len(llms), llms)
	}
	if llms[0]["name"] != "gpt-5.5/cpa" || llms[1]["name"] != "deepseek-v4-pro/newapi" {
		t.Fatalf("unexpected llms: %#v", llms)
	}
}

func TestMarkChatLLMActiveUsesSessionLLMNo(t *testing.T) {
	llms := []map[string]interface{}{
		{"index": float64(0), "active": true},
		{"index": float64(3), "active": false},
	}

	markChatLLMActive(llms, 3)

	if llms[0]["active"] != false {
		t.Fatalf("llms[0].active=%v want false", llms[0]["active"])
	}
	if llms[1]["active"] != true {
		t.Fatalf("llms[1].active=%v want true", llms[1]["active"])
	}
}

func TestChatPythonForConfigPrefersConfiguredPythonPath(t *testing.T) {
	root := t.TempDir()
	venvDir := filepath.Join(root, ".venv", "bin")
	venvPython := filepath.Join(venvDir, "python")
	if runtime.GOOS == "windows" {
		venvDir = filepath.Join(root, ".venv", "Scripts")
		venvPython = filepath.Join(venvDir, "python.exe")
	}
	if err := os.MkdirAll(venvDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(venvPython, []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}
	configured := filepath.Join(t.TempDir(), "configured-python")

	got := chatPythonForConfig(config.AppConfig{GARoot: root, PythonPath: configured})

	if got != configured {
		t.Fatalf("chatPythonForConfig()=%q want configured python %q", got, configured)
	}
}

func TestMarkChatLLMActiveAllowsIndexZero(t *testing.T) {
	llms := []map[string]interface{}{
		{"index": "0", "active": false},
		{"index": "3", "active": true},
	}

	markChatLLMActive(llms, 0)

	if llms[0]["active"] != true {
		t.Fatalf("llms[0].active=%v want true", llms[0]["active"])
	}
	if llms[1]["active"] != false {
		t.Fatalf("llms[1].active=%v want false", llms[1]["active"])
	}
}

func TestChatPostPropagatesLLMNoZeroAndPersistsWorkerStartError(t *testing.T) {
	old := startChatWorkerFunc
	startChatWorkerFunc = func(config.AppConfig, string) (*chatWorker, error) {
		return nil, fmt.Errorf("boom")
	}
	defer func() { startChatWorkerFunc = old }()

	root := t.TempDir()
	s := newGoalTestServer(t, root)
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	h := s.Routes()

	req := httptest.NewRequest(http.MethodPost, "/api/chat/session-a", strings.NewReader(`{"prompt":"hello","settings":{"llm_no":0},"client_user_id":"u1"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("post status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"type":"error"`) || !strings.Contains(rr.Body.String(), "boom") {
		t.Fatalf("expected streamed worker error, got %q", rr.Body.String())
	}

	cs, err := loadChatSession(s.CfgStore.Cfg, "session-a")
	if err != nil {
		t.Fatal(err)
	}
	if cs.Settings.LLMNo != 0 {
		t.Fatalf("LLMNo=%d want 0", cs.Settings.LLMNo)
	}
	if len(cs.Messages) != 2 || cs.Messages[1].Role != "assistant" || !cs.Messages[1].Error || !strings.Contains(cs.Messages[1].Content, "boom") {
		t.Fatalf("unexpected messages: %#v", cs.Messages)
	}
}

func TestChatPostSendsPriorMessagesAndRawHistoryToWorker(t *testing.T) {
	var captured map[string]interface{}
	old := startChatWorkerFunc
	startChatWorkerFunc = func(config.AppConfig, string) (*chatWorker, error) {
		stdinR, stdinW := io.Pipe()
		stdoutR, stdoutW := io.Pipe()
		go func() {
			defer stdinR.Close()
			defer stdoutW.Close()
			_ = json.NewDecoder(stdinR).Decode(&captured)
			done := chatMessage{ID: "a2", Role: "assistant", Content: "ok", CreatedAt: time.Now().Unix()}
			rawHistory := []map[string]interface{}{
				{"role": "user", "content": []map[string]interface{}{{"type": "text", "text": "first question"}}},
				{"role": "assistant", "content": []map[string]interface{}{{"type": "tool_result", "tool_name": "calc", "content": "42"}}},
				{"role": "assistant", "content": []map[string]interface{}{{"type": "text", "text": "ok"}}},
			}
			_ = json.NewEncoder(stdoutW).Encode(map[string]interface{}{"type": "done", "message": done, "raw_history": rawHistory})
		}()
		return &chatWorker{SID: "session-hist", Stdin: stdinW, Stdout: stdoutR}, nil
	}
	defer func() { startChatWorkerFunc = old }()

	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	seedRawHistory := []map[string]interface{}{
		{"role": "user", "content": []map[string]interface{}{{"type": "text", "text": "first question"}}},
		{"role": "assistant", "content": []map[string]interface{}{{"type": "tool_result", "tool_name": "search", "content": "tool data"}}},
	}
	seed := chatSession{
		ID: "session-hist", Title: "History", UpdatedAt: time.Now().Unix(), Settings: chatSettings{LLMNo: 2}, RawHistory: seedRawHistory,
		Messages: []chatMessage{
			{ID: "u0", Role: "user", Content: "first question", CreatedAt: 1},
			{ID: "a0", Role: "assistant", Content: "first answer", CreatedAt: 2},
		},
	}
	if err := saveChatSession(s.CfgStore.Cfg, seed); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/chat/session-hist", strings.NewReader(`{"prompt":"second question","client_user_id":"u1"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if captured == nil {
		t.Fatalf("worker request was not captured")
	}
	if captured["prompt"] != "second question" {
		t.Fatalf("prompt=%#v", captured["prompt"])
	}
	if captured["llm_no"].(float64) != 2 {
		t.Fatalf("llm_no=%#v want 2", captured["llm_no"])
	}
	history, ok := captured["history"].([]interface{})
	if !ok || len(history) != 2 {
		t.Fatalf("history=%#v want two prior messages only", captured["history"])
	}
	first := history[0].(map[string]interface{})
	second := history[1].(map[string]interface{})
	if first["role"] != "user" || first["content"] != "first question" || second["role"] != "assistant" || second["content"] != "first answer" {
		t.Fatalf("unexpected structured history: %#v", history)
	}
	rawHistory, ok := captured["raw_history"].([]interface{})
	if !ok || len(rawHistory) != len(seedRawHistory) {
		t.Fatalf("raw_history=%#v want prior backend history", captured["raw_history"])
	}
	rawSecond := rawHistory[1].(map[string]interface{})
	rawSecondContent := rawSecond["content"].([]interface{})[0].(map[string]interface{})
	if rawSecondContent["type"] != "tool_result" || rawSecondContent["content"] != "tool data" {
		t.Fatalf("raw_history missing tool result: %#v", rawHistory)
	}
	if strings.Contains(rr.Body.String(), "first question") || strings.Contains(rr.Body.String(), "first answer") || strings.Contains(rr.Body.String(), "raw_history") || strings.Contains(rr.Body.String(), "tool_result") {
		t.Fatalf("stream unexpectedly leaked prior/raw history: %s", rr.Body.String())
	}
	stored, err := loadChatSession(s.CfgStore.Cfg, "session-hist")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.RawHistory) != 3 {
		t.Fatalf("stored raw_history len=%d want 3: %#v", len(stored.RawHistory), stored.RawHistory)
	}
	storedContent := stored.RawHistory[1]["content"].([]interface{})[0].(map[string]interface{})
	if storedContent["type"] != "tool_result" || storedContent["content"] != "42" {
		t.Fatalf("stored raw_history not updated from worker: %#v", stored.RawHistory)
	}
}

func TestChatWorkerEOFAppendsCurrentTurnToRawHistoryFallback(t *testing.T) {
	var captured map[string]interface{}
	old := startChatWorkerFunc
	startChatWorkerFunc = func(config.AppConfig, string) (*chatWorker, error) {
		stdinR, stdinW := io.Pipe()
		stdoutR, stdoutW := io.Pipe()
		go func() {
			defer stdinR.Close()
			defer stdoutW.Close()
			_ = json.NewDecoder(stdinR).Decode(&captured)
			_ = json.NewEncoder(stdoutW).Encode(map[string]interface{}{"type": "delta", "delta": "partial answer"})
		}()
		return &chatWorker{SID: "session-eof", Stdin: stdinW, Stdout: stdoutR}, nil
	}
	defer func() { startChatWorkerFunc = old }()

	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	seedRawHistory := []map[string]interface{}{
		{"role": "user", "content": []map[string]interface{}{{"type": "text", "text": "first question"}}},
		{"role": "assistant", "content": []map[string]interface{}{{"type": "tool_result", "tool_name": "search", "content": "tool data"}}},
	}
	seed := chatSession{
		ID: "session-eof", Title: "History", UpdatedAt: time.Now().Unix(), RawHistory: seedRawHistory,
		Messages: []chatMessage{
			{ID: "u0", Role: "user", Content: "first question", CreatedAt: 1},
			{ID: "a0", Role: "assistant", Content: "first answer", CreatedAt: 2},
		},
	}
	if err := saveChatSession(s.CfgStore.Cfg, seed); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/chat/session-eof", strings.NewReader(`{"prompt":"second question","client_user_id":"u1"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if captured == nil {
		t.Fatalf("worker request was not captured")
	}
	if rawHistory, ok := captured["raw_history"].([]interface{}); !ok || len(rawHistory) != len(seedRawHistory) {
		t.Fatalf("worker raw_history=%#v want prior backend history", captured["raw_history"])
	}
	if strings.Contains(rr.Body.String(), "raw_history") || strings.Contains(rr.Body.String(), "tool_result") {
		t.Fatalf("stream unexpectedly leaked raw history: %s", rr.Body.String())
	}

	stored, err := loadChatSession(s.CfgStore.Cfg, "session-eof")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.RawHistory) != len(seedRawHistory)+2 {
		t.Fatalf("raw_history len=%d want %d: %#v", len(stored.RawHistory), len(seedRawHistory)+2, stored.RawHistory)
	}
	keptTool := stored.RawHistory[1]["content"].([]interface{})[0].(map[string]interface{})
	if keptTool["type"] != "tool_result" || keptTool["content"] != "tool data" {
		t.Fatalf("prior tool_result not preserved: %#v", stored.RawHistory)
	}
	userContent := stored.RawHistory[len(stored.RawHistory)-2]["content"].([]interface{})[0].(map[string]interface{})
	assistantContent := stored.RawHistory[len(stored.RawHistory)-1]["content"].([]interface{})[0].(map[string]interface{})
	if stored.RawHistory[len(stored.RawHistory)-2]["role"] != "user" || userContent["text"] != "second question" {
		t.Fatalf("current user not appended to raw_history: %#v", stored.RawHistory)
	}
	if stored.RawHistory[len(stored.RawHistory)-1]["role"] != "assistant" || !strings.Contains(fmt.Sprint(assistantContent["text"]), "partial answer") {
		t.Fatalf("partial assistant not appended to raw_history: %#v", stored.RawHistory)
	}
}

func TestChatNewSessionReportsUnwritableDataDir(t *testing.T) {
	root := t.TempDir()
	s := newGoalTestServer(t, root)
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}
	s.CfgStore.Cfg.ChatDataDir = blocked

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat/session/new", nil)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSaveChatUploadsRejectsTooManyFiles(t *testing.T) {
	cfg := config.AppConfig{GARoot: t.TempDir(), ChatDataDir: t.TempDir()}
	files := make([]chatUpload, maxChatUploadFiles+1)
	for i := range files {
		files[i] = chatUpload{Name: fmt.Sprintf("f%d.txt", i), DataURL: base64.StdEncoding.EncodeToString([]byte("x"))}
	}

	if _, _, err := saveChatUploads(cfg, files); err == nil || !strings.Contains(err.Error(), "too many upload files") {
		t.Fatalf("saveChatUploads too many files err = %v", err)
	}
}

func TestSaveChatUploadsRejectsTooLargeFile(t *testing.T) {
	cfg := config.AppConfig{GARoot: t.TempDir(), ChatDataDir: t.TempDir()}
	tooLarge := make([]byte, maxChatUploadBytesPerFile+1)
	encoded := base64.StdEncoding.EncodeToString(tooLarge)

	if _, _, err := saveChatUploads(cfg, []chatUpload{{Name: "big.bin", DataURL: encoded}}); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("saveChatUploads too large file err = %v", err)
	}
}

func TestSaveChatUploadsRejectsTooLargeTotal(t *testing.T) {
	cfg := config.AppConfig{GARoot: t.TempDir(), ChatDataDir: t.TempDir()}
	chunk := make([]byte, maxChatUploadBytesTotal/3+1)
	encoded := base64.StdEncoding.EncodeToString(chunk)
	files := []chatUpload{
		{Name: "a.bin", DataURL: encoded},
		{Name: "b.bin", DataURL: encoded},
		{Name: "c.bin", DataURL: encoded},
	}

	if _, _, err := saveChatUploads(cfg, files); err == nil || !strings.Contains(err.Error(), "chat uploads too large") {
		t.Fatalf("saveChatUploads total too large err = %v", err)
	}
}

func TestSaveChatUploadsUsesImageRefsForVisionFiles(t *testing.T) {
	cfg := config.AppConfig{GARoot: t.TempDir(), ChatDataDir: t.TempDir()}
	encoded := base64.StdEncoding.EncodeToString([]byte("fake image bytes"))

	saved, refs, err := saveChatUploads(cfg, []chatUpload{{
		Name:    "photo.png",
		Type:    "image/png",
		DataURL: "data:image/png;base64," + encoded,
	}})
	if err != nil {
		t.Fatalf("saveChatUploads: %v", err)
	}
	if len(saved) != 1 || len(refs) != 1 {
		t.Fatalf("saved=%d refs=%d", len(saved), len(refs))
	}
	path, _ := saved[0]["path"].(string)
	if refs[0] != "[image:"+path+"]" {
		t.Fatalf("image ref=%q want [image:%s]", refs[0], path)
	}
}

func TestSaveChatUploadsKeepsFileRefsForNonImages(t *testing.T) {
	cfg := config.AppConfig{GARoot: t.TempDir(), ChatDataDir: t.TempDir()}
	encoded := base64.StdEncoding.EncodeToString([]byte("hello"))

	saved, refs, err := saveChatUploads(cfg, []chatUpload{{
		Name:    "notes.txt",
		Type:    "text/plain",
		DataURL: encoded,
	}})
	if err != nil {
		t.Fatalf("saveChatUploads: %v", err)
	}
	path, _ := saved[0]["path"].(string)
	if len(refs) != 1 || refs[0] != "[FILE:"+path+"]" {
		t.Fatalf("file refs=%#v want [FILE:%s]", refs, path)
	}
}

func TestReadChatWorkerLineAcceptsLargeNDJSONLine(t *testing.T) {
	payload := strings.Repeat("x", 9*1024*1024)
	input := []byte(`{"type":"delta","delta":"` + payload + `"}` + "\n")
	line, err := readChatWorkerLine(bufio.NewReaderSize(bytes.NewReader(input), 64*1024))
	if err != nil {
		t.Fatalf("readChatWorkerLine: %v", err)
	}
	if string(line) != string(input) {
		t.Fatalf("line length=%d want %d", len(line), len(input))
	}
}

func TestSaveChatUploadsSanitizesUnsafeNames(t *testing.T) {
	cfg := config.AppConfig{GARoot: t.TempDir(), ChatDataDir: t.TempDir()}
	encoded := base64.StdEncoding.EncodeToString([]byte("x"))

	saved, refs, err := saveChatUploads(cfg, []chatUpload{
		{Name: `..\evil:name?.txt`, Type: "text/plain", DataURL: encoded},
		{Name: "   ...   ", DataURL: encoded},
	})
	if err != nil {
		t.Fatalf("saveChatUploads: %v", err)
	}
	if len(saved) != 2 || len(refs) != 2 {
		t.Fatalf("saved=%d refs=%d", len(saved), len(refs))
	}
	for i, meta := range saved {
		name, _ := meta["name"].(string)
		if strings.ContainsAny(name, `\/:*?"<>|`) {
			t.Fatalf("saved[%d] unsafe name %q", i, name)
		}
		path, _ := meta["path"].(string)
		if filepath.Dir(path) != chatUploadDir(cfg) {
			t.Fatalf("saved[%d] path dir=%q want %q", i, filepath.Dir(path), chatUploadDir(cfg))
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("saved[%d] stat %q: %v", i, path, err)
		}
	}
	if !strings.Contains(saved[0]["name"].(string), "evil_name_.txt") {
		t.Fatalf("first sanitized name = %q", saved[0]["name"])
	}
	if !strings.Contains(saved[1]["name"].(string), "upload.bin") {
		t.Fatalf("fallback sanitized name = %q", saved[1]["name"])
	}
}

func TestSaveChatUploadsCleansPartialFilesOnLaterFailure(t *testing.T) {
	cfg := config.AppConfig{GARoot: t.TempDir(), ChatDataDir: t.TempDir()}
	encoded := base64.StdEncoding.EncodeToString([]byte("ok"))

	_, _, err := saveChatUploads(cfg, []chatUpload{
		{Name: "kept.txt", DataURL: encoded},
		{Name: "bad.txt", DataURL: "not-base64!"},
	})
	if err == nil {
		t.Fatalf("saveChatUploads err=nil, want decode error")
	}
	entries, readErr := os.ReadDir(chatUploadDir(cfg))
	if readErr != nil {
		t.Fatalf("read upload dir: %v", readErr)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("partial upload files left after failure: %v", names)
	}
}

func TestChatSaveSettingsRejectsMalformedJSON(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	req := httptest.NewRequest(http.MethodPost, "/api/chat/settings/session-bad", strings.NewReader(`{"llm_no":`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if _, err := os.Stat(chatSessionPath(s.CfgStore.Cfg, "session-bad")); !os.IsNotExist(err) {
		t.Fatalf("malformed settings request should not create session file, stat err=%v", err)
	}
}

func TestChatSaveSettingsPersistsValidJSON(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	req := httptest.NewRequest(http.MethodPost, "/api/chat/settings/session-ok", strings.NewReader(`{"llm_no":3}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	cs, err := loadChatSession(s.CfgStore.Cfg, "session-ok")
	if err != nil {
		t.Fatal(err)
	}
	if cs.Settings.LLMNo != 3 {
		t.Fatalf("settings not persisted: %#v", cs.Settings)
	}
}

func TestSaveChatSessionReportsCreateDirError(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.AppConfig{GARoot: t.TempDir(), ChatDataDir: blocked}

	if err := saveChatSession(cfg, chatSession{ID: "mkdir-fail"}); err == nil {
		t.Fatalf("saveChatSession err=nil, want create dir error")
	}
}

func TestSaveChatUploadsReportsCreateDirError(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.AppConfig{GARoot: t.TempDir(), ChatDataDir: blocked}
	encoded := base64.StdEncoding.EncodeToString([]byte("x"))

	if _, _, err := saveChatUploads(cfg, []chatUpload{{Name: "x.txt", DataURL: encoded}}); err == nil {
		t.Fatalf("saveChatUploads err=nil, want create dir error")
	}
}

func TestChatSessionsReportsUnwritableDataDir(t *testing.T) {
	root := t.TempDir()
	s := newGoalTestServer(t, root)
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}
	s.CfgStore.Cfg.ChatDataDir = blocked

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/chat/sessions", nil)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestLoadChatSessionReportsCorruptJSON(t *testing.T) {
	cfg := config.AppConfig{GARoot: t.TempDir(), ChatDataDir: t.TempDir()}
	if err := os.MkdirAll(chatSessionDir(cfg), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chatSessionPath(cfg, "bad-json"), []byte("{"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := loadChatSession(cfg, "bad-json")
	if err == nil {
		t.Fatal("expected corrupt session JSON error")
	}
}

func TestChatGetSessionReportsCorruptJSON(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	if err := os.MkdirAll(chatSessionDir(s.CfgStore.Cfg), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chatSessionPath(s.CfgStore.Cfg, "bad-json"), []byte("{"), 0644); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/chat/session/bad-json", nil)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestChatSessionsReportsMigrationCreateDirError(t *testing.T) {
	gaRoot := t.TempDir()
	legacyDir := legacyChatSessionDir(gaRoot)
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "legacy.json"), []byte(`{"id":"legacy"}`), 0644); err != nil {
		t.Fatal(err)
	}
	chatDataPath := filepath.Join(t.TempDir(), "chat-data-file")
	if err := os.WriteFile(chatDataPath, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	s := newGoalTestServer(t, gaRoot)
	s.CfgStore.Cfg.ChatDataDir = chatDataPath
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/chat/sessions", nil)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestLoadChatSessionReportsMigrationCreateDirError(t *testing.T) {
	gaRoot := t.TempDir()
	legacyDir := legacyChatSessionDir(gaRoot)
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "legacy.json"), []byte(`{"id":"legacy"}`), 0644); err != nil {
		t.Fatal(err)
	}
	chatDataPath := filepath.Join(t.TempDir(), "chat-data-file")
	if err := os.WriteFile(chatDataPath, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.AppConfig{GARoot: gaRoot, ChatDataDir: chatDataPath}
	if _, err := loadChatSession(cfg, "legacy"); err == nil {
		t.Fatal("expected migration create directory error")
	}
}

func TestChatWriteRoutesRejectTrailingJSONValues(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	for _, tc := range []struct {
		name string
		path string
		body string
	}{
		{name: "rename", path: "/api/chat/session/chat-trailing", body: `{"title":"new"} {"extra":true}`},
		{name: "settings", path: "/api/chat/settings/chat-trailing", body: `{"llm_no":0} {"extra":true}`},
		{name: "post", path: "/api/chat/chat-trailing", body: `{"prompt":"hello"} {"extra":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			method := http.MethodPost
			if tc.name == "rename" {
				method = http.MethodPatch
			}
			req := httptest.NewRequest(method, tc.path, strings.NewReader(tc.body))
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "single JSON value") {
				t.Fatalf("body missing single JSON value guidance: %s", rr.Body.String())
			}
		})
	}
}

func TestChatWriteRoutesRejectOversizedJSONBody(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	for _, tc := range []struct {
		name       string
		path       string
		body       string
		wantStatus int
	}{
		{name: "rename", path: "/api/chat/session/chat-big", body: `{"title":"` + strings.Repeat("x", maxJSONBodyBytes) + `"}`, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "settings", path: "/api/chat/settings/chat-big", body: `{"provider":"` + strings.Repeat("x", maxJSONBodyBytes) + `"}`, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "post", path: "/api/chat/chat-big", body: `{"prompt":"` + strings.Repeat("x", int(maxChatPostBodyBytes)) + `"}`, wantStatus: http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			method := http.MethodPost
			if tc.name == "rename" {
				method = http.MethodPatch
			}
			req := httptest.NewRequest(method, tc.path, strings.NewReader(tc.body))
			h.ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), errRequestBodyTooLarge.Error()) {
				t.Fatalf("body missing too-large guidance: %s", rr.Body.String())
			}
		})
	}
}

func TestChatSessionPathSanitizesUntrustedIDsInsideChatDataDir(t *testing.T) {
	chatDataDirRoot := t.TempDir()
	cfg := config.AppConfig{GARoot: t.TempDir(), ChatDataDir: chatDataDirRoot}
	for _, sid := range []string{"../../outside", `..\\outside`, "semi;colon", "space id", "nested/path"} {
		t.Run(sid, func(t *testing.T) {
			got := chatSessionPath(cfg, sid)
			wantRoot := chatSessionDir(cfg) + string(os.PathSeparator)
			if !strings.HasPrefix(got, wantRoot) {
				t.Fatalf("chatSessionPath(%q)=%q outside %q", sid, got, wantRoot)
			}
			base := filepath.Base(got)
			if base == sid+".json" || strings.Contains(base, "..") || strings.ContainsAny(base, `/\\ ;`) {
				t.Fatalf("chatSessionPath(%q) kept unsafe base %q", sid, base)
			}
			if filepath.Dir(got) != chatSessionDir(cfg) {
				t.Fatalf("chatSessionPath(%q) dir=%q want %q", sid, filepath.Dir(got), chatSessionDir(cfg))
			}
		})
	}
}

func TestChatWriteRoutesWithUnsafeIDsStayInsideChatDataDir(t *testing.T) {
	gaRoot := t.TempDir()
	chatDataDirRoot := t.TempDir()
	s := newGoalTestServer(t, gaRoot)
	s.CfgStore.Cfg.ChatDataDir = chatDataDirRoot
	h := s.Routes()
	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "rename", method: http.MethodPatch, path: "/api/chat/session/semi;colon", body: `{"title":"kept inside"}`},
		{name: "settings", method: http.MethodPost, path: "/api/chat/settings/space%20id", body: `{"llm_no":2}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			if _, err := os.Stat(filepath.Join(chatDataDirRoot, "outside.json")); !os.IsNotExist(err) {
				t.Fatalf("unsafe route wrote outside chat session dir: err=%v", err)
			}
			entries, err := os.ReadDir(chatSessionDir(s.CfgStore.Cfg))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Fatalf("session files=%d want=1 entries=%v", len(entries), entries)
			}
			if strings.Contains(entries[0].Name(), "outside") || strings.Contains(entries[0].Name(), "..") {
				t.Fatalf("unsafe id leaked into file name: %q", entries[0].Name())
			}
			_ = os.Remove(filepath.Join(chatSessionDir(s.CfgStore.Cfg), entries[0].Name()))
		})
	}
}

func TestChatFileRouteUsesBaseNameOnly(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	if err := os.MkdirAll(chatUploadDir(s.CfgStore.Cfg), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chatUploadDir(s.CfgStore.Cfg), "safe.txt"), []byte("safe upload"), 0644); err != nil {
		t.Fatal(err)
	}
	outsideDir := filepath.Dir(chatUploadDir(s.CfgStore.Cfg))
	if err := os.WriteFile(filepath.Join(outsideDir, "outside.txt"), []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/chat/file/..%2Foutside.txt", nil)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code == http.StatusOK || strings.Contains(rr.Body.String(), "outside") {
		t.Fatalf("chat file traversal succeeded status=%d body=%q", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/chat/file/nested/safe.txt", nil)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != "safe upload" {
		t.Fatalf("chat file basename lookup status=%d body=%q", rr.Code, rr.Body.String())
	}
}
