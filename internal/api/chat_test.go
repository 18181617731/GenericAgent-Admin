package api

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
