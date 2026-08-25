package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeLocalCmdProcess struct {
	mu     sync.Mutex
	input  bytes.Buffer
	outR   *io.PipeReader
	outW   *io.PipeWriter
	done   chan struct{}
	closed bool
}

func newFakeLocalCmdProcess() *fakeLocalCmdProcess {
	read, write := io.Pipe()
	return &fakeLocalCmdProcess{outR: read, outW: write, done: make(chan struct{})}
}

func (p *fakeLocalCmdProcess) Read(data []byte) (int, error) { return p.outR.Read(data) }

func (p *fakeLocalCmdProcess) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, io.ErrClosedPipe
	}
	return p.input.Write(data)
}

func (p *fakeLocalCmdProcess) Resize(int, int) error { return nil }

func (p *fakeLocalCmdProcess) Kill() error {
	p.finish()
	return nil
}

func (p *fakeLocalCmdProcess) Wait() (int, error) {
	<-p.done
	return 0, nil
}

func (p *fakeLocalCmdProcess) Close() error {
	p.finish()
	return nil
}

func (p *fakeLocalCmdProcess) finish() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	close(p.done)
	_ = p.outW.Close()
	p.mu.Unlock()
}

func (p *fakeLocalCmdProcess) emit(data []byte) error {
	_, err := p.outW.Write(data)
	return err
}

func installFakeLocalCmdFactory(t *testing.T) (*fakeLocalCmdProcess, *localCmdRegistry) {
	t.Helper()
	process := newFakeLocalCmdProcess()
	old := newLocalCmdProcessFunc
	newLocalCmdProcessFunc = func(string, int, int) (localCmdProcess, error) { return process, nil }
	t.Cleanup(func() { newLocalCmdProcessFunc = old })
	registry := newLocalCmdRegistry()
	t.Cleanup(registry.close)
	return process, registry
}

func TestLocalCmdRegistryReplaysAndCapsOutput(t *testing.T) {
	process, registry := installFakeLocalCmdFactory(t)
	dir := t.TempDir()
	session, err := registry.create(dir, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if session.id == "" || len(session.id) < 20 {
		t.Fatalf("session id = %q, want unpredictable id", session.id)
	}
	if err := process.emit([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	events, _, done, truncated := session.eventSnapshot(0)
	if done || truncated || len(events) == 0 || string(events[0].data) != "hello" {
		t.Fatalf("events=%#v done=%v truncated=%v", events, done, truncated)
	}
	session.appendData(bytes.Repeat([]byte{'x'}, localCmdOutputLimit+100))
	session.mu.Lock()
	gotBytes := session.outputBytes
	session.mu.Unlock()
	if gotBytes > localCmdOutputLimit {
		t.Fatalf("output bytes=%d, want <=%d", gotBytes, localCmdOutputLimit)
	}
}

func TestLocalCmdRegistryEnforcesSessionLimit(t *testing.T) {
	_, registry := installFakeLocalCmdFactory(t)
	for i := 0; i < localCmdMaxSessions; i++ {
		if _, err := registry.create(t.TempDir(), 80, 24); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if _, err := registry.create(t.TempDir(), 80, 24); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("fourth session error=%v, want limit", err)
	}
}

func TestLocalCmdInputDecodingAndLimits(t *testing.T) {
	process, registry := installFakeLocalCmdFactory(t)
	session, err := registry.create(t.TempDir(), 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{localCmdSessions: registry}
	req := httptest.NewRequest(http.MethodPost, "/api/local-cmd/sessions/"+session.id+"/input", strings.NewReader(`{"base64":"aGVsbG8="}`))
	req.Header.Set("X-GA-Confirm", "dangerous")
	rr := httptest.NewRecorder()
	server.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("input status=%d body=%s", rr.Code, rr.Body.String())
	}
	process.mu.Lock()
	got := process.input.String()
	process.mu.Unlock()
	if got != "hello" {
		t.Fatalf("input=%q, want hello", got)
	}
	large, _ := json.Marshal(localCmdInputRequest{Data: strings.Repeat("x", localCmdMaxInput+1)})
	req = httptest.NewRequest(http.MethodPost, "/api/local-cmd/sessions/"+session.id+"/input", bytes.NewReader(large))
	req.Header.Set("X-GA-Confirm", "dangerous")
	rr = httptest.NewRecorder()
	server.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large input status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestLocalCmdStreamIsReplayableNDJSON(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	session := &localCmdSession{id: "test-session", path: dir, status: "exited", created: now, lastActivity: now, finished: now, notify: make(chan struct{})}
	session.appendData([]byte("REMOTE_CMD_OK\r\n"))
	code := 0
	session.appendEventLocked(localCmdEvent{kind: "exit", exit: code})
	registry := &localCmdRegistry{sessions: map[string]*localCmdSession{session.id: session}, stop: make(chan struct{}), done: make(chan struct{})}
	server := &Server{localCmdSessions: registry}
	req := httptest.NewRequest(http.MethodGet, "/api/local-cmd/sessions/"+session.id+"/stream?from=0", nil)
	rr := httptest.NewRecorder()
	server.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("stream status=%d body=%s", rr.Code, rr.Body.String())
	}
	lines := strings.Split(strings.TrimSpace(rr.Body.String()), "\n")
	if len(lines) < 3 || !strings.Contains(lines[0], `"type":"sync"`) || !strings.Contains(lines[1], `"type":"data"`) || !strings.Contains(lines[2], `"type":"exit"`) {
		t.Fatalf("stream lines=%q", lines)
	}
}

func TestLocalCmdDirectoryValidationStillAllowsChineseSpacePath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "中文 local cmd")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	got, err := validateLocalCmdDirectory("  " + dir + "  ")
	if err != nil || got != filepath.Clean(dir) {
		t.Fatalf("validated path=%q err=%v", got, err)
	}
}

func TestLocalCmdSessionExpiryHonorsActiveIdleTimeout(t *testing.T) {
	now := time.Now()
	session := &localCmdSession{status: "running", connections: 1, lastActivity: now.Add(-localCmdIdleTimeout - time.Second)}
	if !session.expired(now) {
		t.Fatal("active idle session should expire after idle timeout")
	}
	session.lastActivity = now.Add(-localCmdConnectionGrace - time.Second)
	if session.expired(now) {
		t.Fatal("active session should not use detached connection grace")
	}
	session.connections = 0
	if !session.expired(now) {
		t.Fatal("detached session should expire after connection grace")
	}
}
