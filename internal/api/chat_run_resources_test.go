package api

import (
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestChatRunRepeatedEndDoesNotRetainGoroutines(t *testing.T) {
	s := &Server{ChatMu: &sync.Mutex{}, ChatRuns: map[string]*chatRun{"repeat": {ID: "run"}}}
	t.Cleanup(s.CloseChatWorkers)
	before := runtime.NumGoroutine()
	for i := 0; i < 128; i++ {
		s.endChatRun("repeat")
		s.endChatRun("missing")
	}
	time.Sleep(20 * time.Millisecond)
	if growth := runtime.NumGoroutine() - before; growth > 8 {
		t.Fatalf("repeated completion retained %d goroutines; want bounded cleanup", growth)
	}
}

func TestChatRunReplacementReleasesReplayPayload(t *testing.T) {
	s := &Server{ChatMu: &sync.Mutex{}, ChatRuns: map[string]*chatRun{}}
	t.Cleanup(s.CloseChatWorkers)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < 12; i++ {
		run := s.beginChatRun("replace")
		if run == nil {
			t.Fatal("could not begin replacement run")
		}
		run.Events = [][]byte{make([]byte, 2*1024*1024)}
		run.Events[0][0] = byte(i)
		s.endChatRunOwned("replace", run)
	}
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(s)
	growth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("12 replacements, 2 MiB each: post-GC heap growth = %.2f MiB", float64(growth)/(1024*1024))
	if growth > 6*1024*1024 {
		t.Fatalf("obsolete runs retain %d bytes beyond the current replay snapshot", growth)
	}
}

func TestChatRunReplayCleanupIsIdempotentAndOwned(t *testing.T) {
	s := &Server{ChatMu: &sync.Mutex{}, ChatRuns: map[string]*chatRun{}}
	t.Cleanup(s.CloseChatWorkers)
	old := s.beginChatRun("owned")
	s.endChatRunOwned("owned", old)
	timer := old.ReplayCleanup
	if timer == nil {
		t.Fatal("missing replay cleanup")
	}
	s.endChatRunOwned("owned", old)
	if old.ReplayCleanup != timer {
		t.Fatal("duplicate end replaced cleanup timer")
	}
	current := s.beginChatRun("owned")
	if current == nil {
		t.Fatal("could not replace completed run")
	}
	s.endChatRunOwned("owned", old)
	s.expireChatRunReplay("owned", timer)
	if s.ChatRuns["owned"] != current || current.Done {
		t.Fatal("old cleanup affected replacement run")
	}
	s.endChatRunOwned("owned", current)
	s.expireChatRunReplay("owned", timer)
	if s.ChatRuns["owned"] != current {
		t.Fatal("stale cleanup removed current replay")
	}
	current.ReplayCleanup.Stop()
	s.expireChatRunReplay("owned", current.ReplayCleanup)
	s.expireChatRunReplay("owned", current.ReplayCleanup)
	if _, ok := s.ChatRuns["owned"]; ok {
		t.Fatal("expired run remains retained")
	}
}

func TestChatRunReplaySurvivesCompletionWithCursor(t *testing.T) {
	s := &Server{ChatMu: &sync.Mutex{}, ChatRuns: map[string]*chatRun{}}
	t.Cleanup(s.CloseChatWorkers)
	run := s.beginChatRun("replay")
	s.publishChatRun("replay", map[string]interface{}{"type": "delta", "text": "first"})
	s.publishChatRun("replay", map[string]interface{}{"type": "done", "text": "last"})
	s.endChatRunOwned("replay", run)
	w := httptest.NewRecorder()
	s.streamChatRun(w, httptest.NewRequest("GET", "/", nil), "replay", 1)
	if !strings.Contains(w.Body.String(), "last") || strings.Contains(w.Body.String(), "first") {
		t.Fatalf("replay cursor changed: %s", w.Body.String())
	}
}

func TestChatRunCancelHandshakeDefersReplayCleanup(t *testing.T) {
	s := &Server{ChatMu: &sync.Mutex{}, ChatRuns: map[string]*chatRun{}}
	t.Cleanup(s.CloseChatWorkers)
	run := s.beginChatRun("cancel")
	run.Canceled = true
	s.endChatRunOwned("cancel", run)
	if run.Done || run.ReplayCleanup != nil {
		t.Fatal("cleanup started before cancel persistence handshake")
	}
	run.CancelReady = true
	s.endChatRunOwned("cancel", run)
	if !run.Done || run.ReplayCleanup == nil {
		t.Fatal("ready cancellation did not schedule replay cleanup")
	}
}
