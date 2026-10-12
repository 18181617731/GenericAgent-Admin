package api

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"genericagent-admin-go/internal/config"
)

type lifecycleWriteCloser struct{ closes atomic.Int32 }

func (*lifecycleWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (w *lifecycleWriteCloser) Close() error              { w.closes.Add(1); return nil }

func lifecycleWorker(sid string) *chatWorker {
	return &chatWorker{SID: sid, Stdin: &lifecycleWriteCloser{}, Stdout: io.NopCloser(strings.NewReader(""))}
}

func TestChatWorkerConcurrentAcquisitionSharesStartup(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	t.Cleanup(s.CloseChatWorkers)
	var starts atomic.Int32
	entered, proceed := make(chan struct{}), make(chan struct{})
	old := startChatWorkerFunc
	startChatWorkerFunc = func(_ config.AppConfig, sid string) (*chatWorker, error) {
		if starts.Add(1) == 1 {
			close(entered)
		}
		<-proceed
		return lifecycleWorker(sid), nil
	}
	defer func() { startChatWorkerFunc = old }()
	const clients = 8
	workers := make(chan *chatWorker, clients)
	var wg sync.WaitGroup
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w, err := s.getChatWorker("shared")
			if err != nil {
				t.Errorf("getChatWorker: %v", err)
			}
			workers <- w
		}()
	}
	<-entered
	time.Sleep(20 * time.Millisecond)
	close(proceed)
	wg.Wait()
	close(workers)
	if n := starts.Load(); n != 1 {
		t.Errorf("concurrent requests started %d workers, want 1", n)
	}
	var first *chatWorker
	for w := range workers {
		if first == nil {
			first = w
		}
		if w != first {
			t.Error("concurrent callers received different workers")
		}
	}
}

func TestChatWorkerShutdownRejectsLateStartup(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	t.Cleanup(s.CloseChatWorkers)
	entered, proceed := make(chan struct{}), make(chan struct{})
	created := lifecycleWorker("closing")
	old := startChatWorkerFunc
	startChatWorkerFunc = func(config.AppConfig, string) (*chatWorker, error) {
		close(entered)
		<-proceed
		return created, nil
	}
	defer func() { startChatWorkerFunc = old }()
	result := make(chan error, 1)
	go func() { _, err := s.getChatWorker("closing"); result <- err }()
	<-entered
	s.CloseChatWorkers()
	close(proceed)
	if err := <-result; err == nil {
		t.Error("startup published a worker after shutdown")
	}
	s.ChatMu.Lock()
	retained := len(s.ChatWorkers)
	s.ChatMu.Unlock()
	if retained != 0 {
		t.Errorf("shutdown retained %d workers", retained)
	}
}

func TestChatWorkerIdleRetirementHonorsAllLeasesAndIgnoresStaleTimer(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	t.Cleanup(s.CloseChatWorkers)
	worker := lifecycleWorker("idle")
	s.ChatWorkers["idle"] = worker
	first, err := s.getChatWorker("idle")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.getChatWorker("idle")
	if err != nil {
		t.Fatal(err)
	}
	s.releaseChatWorker("idle", first)
	if worker.Users != 1 || worker.IdleCleanup != nil {
		t.Fatal("retirement scheduled while a caller was active")
	}
	s.releaseChatWorker("idle", second)
	oldTimer := worker.IdleCleanup
	if oldTimer == nil {
		t.Fatal("last release did not schedule retirement")
	}
	borrowed, err := s.getChatWorker("idle")
	if err != nil {
		t.Fatal(err)
	}
	s.retireIdleChatWorker("idle", oldTimer)
	if s.ChatWorkers["idle"] != worker || worker.Dead {
		t.Fatal("stale callback retired a borrowed worker")
	}
	s.releaseChatWorker("idle", borrowed)
	newTimer := worker.IdleCleanup
	if newTimer == nil || newTimer == oldTimer {
		t.Fatal("new idle period did not receive a fresh timer")
	}
	newTimer.Stop()
	s.retireIdleChatWorker("idle", oldTimer)
	if s.ChatWorkers["idle"] != worker {
		t.Fatal("old idle period retired a new idle period")
	}
	s.retireIdleChatWorker("idle", newTimer)
	s.retireIdleChatWorker("idle", newTimer)
	if len(s.ChatWorkers) != 0 || !worker.Dead {
		t.Fatal("idle retirement retained the worker")
	}
	if closes := worker.Stdin.(*lifecycleWriteCloser).closes.Load(); closes != 1 {
		t.Fatalf("pipe closed %d times, want 1", closes)
	}
}

func TestChatWorkerStaleDropPreservesReplacementAndClosesOnce(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	t.Cleanup(s.CloseChatWorkers)
	old, replacement := lifecycleWorker("reused"), lifecycleWorker("reused")
	s.ChatWorkers["reused"] = replacement
	s.dropChatWorker("reused", old)
	s.dropChatWorker("reused", old)
	s.releaseChatWorker("reused", old)
	if s.ChatWorkers["reused"] != replacement || replacement.Dead {
		t.Fatal("stale worker cleanup removed the replacement")
	}
	if closes := old.Stdin.(*lifecycleWriteCloser).closes.Load(); closes != 1 {
		t.Fatalf("pipe closed %d times, want 1", closes)
	}
}

func TestChatWorkerStartupErrorClosesPartialWorkerAndCanRetry(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	t.Cleanup(s.CloseChatWorkers)
	partial := lifecycleWorker("retry")
	old := startChatWorkerFunc
	starts := 0
	startChatWorkerFunc = func(_ config.AppConfig, sid string) (*chatWorker, error) {
		starts++
		if starts == 1 {
			return partial, io.ErrUnexpectedEOF
		}
		return lifecycleWorker(sid), nil
	}
	defer func() { startChatWorkerFunc = old }()
	if _, err := s.getChatWorker("retry"); err != io.ErrUnexpectedEOF {
		t.Fatalf("startup error = %v", err)
	}
	if len(s.ChatWorkers) != 0 || partial.Stdin.(*lifecycleWriteCloser).closes.Load() != 1 {
		t.Fatal("failed startup retained resources")
	}
	worker, err := s.getChatWorker("retry")
	if err != nil {
		t.Fatal(err)
	}
	s.releaseChatWorker("retry", worker)
	if starts != 2 || worker.IdleCleanup == nil {
		t.Fatal("startup did not recover")
	}
}

func TestChatWorkerShutdownClosesEveryInstanceAndRejectsFutureRuntime(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	s.ChatRuntimes = newChatRuntimeRegistry()
	s.ChatRuntime = s.ChatRuntimes.runtime("")
	s.ChatMu = &s.ChatRuntime.chatMu
	s.ChatWorkers = s.ChatRuntime.workers
	s.ChatRuns = s.ChatRuntime.runs
	otherRuntime := s.ChatRuntimes.runtime("other")
	other := &Server{CfgStore: s.CfgStore, ChatMu: &otherRuntime.chatMu, ChatWorkers: otherRuntime.workers, ChatRuns: otherRuntime.runs, ChatRuntime: otherRuntime}
	first, second := lifecycleWorker("same"), lifecycleWorker("same")
	s.ChatWorkers["same"] = first
	other.ChatWorkers["same"] = second
	s.CloseChatWorkers()
	s.CloseChatWorkers()
	if len(s.ChatWorkers) != 0 || len(other.ChatWorkers) != 0 {
		t.Fatal("shutdown retained an instance worker")
	}
	for _, worker := range []*chatWorker{first, second} {
		if worker.Stdin.(*lifecycleWriteCloser).closes.Load() != 1 {
			t.Fatal("shutdown did not close each worker exactly once")
		}
	}
	if _, err := other.getChatWorker("same"); err == nil {
		t.Fatal("request clone reopened a closed runtime")
	}
	futureRuntime := s.ChatRuntimes.runtime("future")
	future := &Server{CfgStore: s.CfgStore, ChatMu: &futureRuntime.chatMu, ChatWorkers: futureRuntime.workers, ChatRuntime: futureRuntime}
	if _, err := future.getChatWorker("same"); err == nil {
		t.Fatal("new instance reopened workers after shutdown")
	}
}

func TestChatWorkerIdleRetirementStopsRealSubprocess(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	t.Cleanup(s.CloseChatWorkers)
	cmd := exec.Command(os.Args[0], "-test.run=^TestWorldlineWorkerHelper$")
	cmd.Env = append(os.Environ(), "GA_WORLDLINE_TEST_WORKER=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	worker := &chatWorker{SID: "process", Cmd: cmd, Stdin: stdin, Stdout: stdout, Users: 1}
	s.ChatWorkers["process"] = worker
	// FindProcess owns an independent Windows handle; the worker's handle is
	// released by Process.Wait, so a second Kill is not a valid liveness probe.
	var observer *os.Process
	if runtime.GOOS == "windows" {
		observer, err = os.FindProcess(cmd.Process.Pid)
		if err != nil {
			t.Fatal(err)
		}
		defer observer.Release()
	}
	s.releaseChatWorker("process", worker)
	timer := worker.IdleCleanup
	timer.Stop()
	s.retireIdleChatWorker("process", timer)
	if len(s.ChatWorkers) != 0 {
		t.Fatal("retirement retained subprocess")
	}
	if observer != nil {
		result := make(chan error, 1)
		go func() { _, err := observer.Wait(); result <- err }()
		select {
		case err := <-result:
			if err != nil {
				t.Fatalf("observing retired subprocess: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("retired subprocess is still running")
		}
	} else if _, err := cmd.Process.Wait(); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("retirement did not reap subprocess: %v", err)
	}
	if _, err := stdin.Write([]byte("{}\n")); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("stdin remains open: %v", err)
	}
	if _, err := stdout.Read(make([]byte, 1)); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("stdout remains open: %v", err)
	}
}

func TestChatWorkerIdleReopenCarriesPersistedHistory(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	t.Cleanup(s.CloseChatWorkers)
	const sid = "reopen"
	cs := chatSession{ID: sid, Messages: []chatMessage{{ID: "u", Role: "user", Content: "kept"}}, RawHistory: []map[string]interface{}{{"role": "user", "content": "kept raw"}}, HistoryInfo: []interface{}{float64(7)}, Working: map[string]interface{}{"key_info": "kept working"}, Settings: normalizeChatSettings(chatSettings{})}
	if err := saveChatSession(s.CfgStore.Snapshot(), cs); err != nil {
		t.Fatal(err)
	}
	old := startChatWorkerFunc
	starts, checks := 0, 0
	startChatWorkerFunc = func(_ config.AppConfig, sid string) (*chatWorker, error) { starts++; return lifecycleWorker(sid), nil }
	defer func() { startChatWorkerFunc = old }()
	stop := errors.New("checked persisted payload")
	s.chatWorldlineRPCHook = func(_ string, req map[string]interface{}) error {
		checks++
		if req["history"].([]chatMessage)[0].Content != "kept" || req["raw_history"].([]map[string]interface{})[0]["content"] != "kept raw" || req["history_info"].([]interface{})[0] != float64(7) || req["working"].(map[string]interface{})["key_info"] != "kept working" {
			t.Fatal("reopen lost persisted session state")
		}
		return stop
	}
	for i := 0; i < 2; i++ {
		if _, err := s.chatWorldlineRPC(sid, map[string]interface{}{"action": "state", "activate": true}); !errors.Is(err, stop) {
			t.Fatal(err)
		}
		worker := s.ChatWorkers[sid]
		if worker.Users != 0 || worker.IdleCleanup == nil {
			t.Fatal("RPC did not release lease")
		}
		worker.IdleCleanup.Stop()
		s.retireIdleChatWorker(sid, worker.IdleCleanup)
	}
	if starts != 2 || checks != 2 {
		t.Fatalf("starts=%d payload checks=%d", starts, checks)
	}
}
