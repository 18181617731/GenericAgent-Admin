package api

import (
	"errors"
	"sync"
	"time"
)

// Persistent workers are a warm cache, not the owner of session history.
// All callers hold a lease until their entire RPC/stream (including Mu waits)
// has finished. Retirement never runs while a lease is outstanding.
const chatWorkerIdleTimeout = 10 * time.Minute

func (s *Server) chatWorkersClosedLocked() bool {
	if s.ChatRuntime != nil {
		return s.ChatRuntime.workersClosing
	}
	return s.chatWorkersClosing
}

func (s *Server) getChatWorker(sid string) (*chatWorker, error) {
	sid = safeChatID(sid)
	for {
		s.ChatMu.Lock()
		if s.chatWorkersClosedLocked() {
			s.ChatMu.Unlock()
			return nil, errors.New("chat workers are closing")
		}
		if s.ChatWorkers == nil {
			s.ChatWorkers = make(map[string]*chatWorker)
		}
		if worker := s.ChatWorkers[sid]; worker != nil && !worker.Dead {
			if worker.Ready != nil {
				ready := worker.Ready
				s.ChatMu.Unlock()
				<-ready
				// StartErr is immutable after Ready closes.
				if worker.StartErr != nil {
					return nil, worker.StartErr
				}
				continue
			}
			if worker.IdleCleanup != nil {
				worker.IdleCleanup.Stop()
				worker.IdleCleanup = nil
			}
			worker.Users++
			s.ChatMu.Unlock()
			return worker, nil
		}
		slot := &chatWorker{SID: sid, Ready: make(chan struct{})}
		s.ChatWorkers[sid] = slot
		s.ChatMu.Unlock()

		worker, err := startChatWorkerFunc(s.CfgStore.Snapshot(), sid)
		s.ChatMu.Lock()
		if s.chatWorkersClosedLocked() || s.ChatWorkers[sid] != slot || slot.Dead {
			err = errors.New("chat worker startup was canceled")
		} else if err == nil && worker == nil {
			err = errors.New("chat worker startup returned no worker")
		}
		if err != nil {
			if s.ChatWorkers[sid] == slot {
				delete(s.ChatWorkers, sid)
			}
		} else {
			worker.Users = 1
			s.ChatWorkers[sid] = worker
		}
		slot.StartErr = err
		close(slot.Ready)
		s.ChatMu.Unlock()
		if err != nil {
			stopChatWorker(worker)
			return nil, err
		}
		return worker, nil
	}
}

func (s *Server) releaseChatWorker(sid string, worker *chatWorker) {
	if worker == nil {
		return
	}
	sid = safeChatID(sid)
	s.ChatMu.Lock()
	defer s.ChatMu.Unlock()
	if worker.Users > 0 {
		worker.Users--
	}
	if s.ChatWorkers[sid] != worker || worker.Dead || worker.Users != 0 || worker.IdleCleanup != nil {
		return
	}
	var timer *time.Timer
	timer = time.AfterFunc(chatWorkerIdleTimeout, func() { s.retireIdleChatWorker(sid, timer) })
	worker.IdleCleanup = timer
}

// The callback holds only the session ID and timer, not an obsolete worker.
func (s *Server) retireIdleChatWorker(sid string, timer *time.Timer) {
	s.ChatMu.Lock()
	worker := s.ChatWorkers[sid]
	if worker == nil || worker.IdleCleanup != timer || worker.Users != 0 || worker.Ready != nil {
		s.ChatMu.Unlock()
		return
	}
	worker.IdleCleanup = nil
	worker.Dead = true
	delete(s.ChatWorkers, sid)
	s.ChatMu.Unlock()
	stopChatWorker(worker)
}

func (s *Server) dropChatWorker(sid string, worker *chatWorker) {
	sid = safeChatID(sid)
	s.ChatMu.Lock()
	if s.ChatWorkers[sid] == worker {
		delete(s.ChatWorkers, sid)
	}
	if worker != nil {
		worker.Dead = true
		if worker.IdleCleanup != nil {
			worker.IdleCleanup.Stop()
			worker.IdleCleanup = nil
		}
	}
	s.ChatMu.Unlock()
	stopChatWorker(worker)
}

func stopChatWorker(worker *chatWorker) {
	if worker == nil {
		return
	}
	worker.stopOnce.Do(func() {
		if worker.Cmd != nil && worker.Cmd.Process != nil {
			_ = worker.Cmd.Process.Kill()
			_, _ = worker.Cmd.Process.Wait()
		}
		if worker.Stdin != nil {
			_ = worker.Stdin.Close()
		}
		if worker.Stdout != nil {
			_ = worker.Stdout.Close()
		}
		if worker.Stderr != nil {
			_ = worker.Stderr.Close()
		}
	})
}

// CloseChatWorkers terminates workers for all instance runtimes and rejects
// in-flight startups. Subprocess waits happen outside runtime/registry locks.
func (s *Server) CloseChatWorkers() {
	if s == nil {
		return
	}
	var runtimes []*chatRuntime
	if s.ChatRuntimes != nil {
		s.ChatRuntimes.mu.Lock()
		s.ChatRuntimes.closed = true
		for _, runtime := range s.ChatRuntimes.entries {
			runtimes = append(runtimes, runtime)
		}
		s.ChatRuntimes.mu.Unlock()
	}
	seen := make(map[*sync.Mutex]bool)
	closeRuntime := func(server *Server) {
		if server.ChatMu == nil || seen[server.ChatMu] {
			return
		}
		seen[server.ChatMu] = true
		server.closeChatWorkerRuntime()
	}
	closeRuntime(s)
	for _, runtime := range runtimes {
		closeRuntime(&Server{ChatMu: &runtime.chatMu, ChatWorkers: runtime.workers, ChatRuns: runtime.runs, ChatRuntime: runtime})
	}
}

func (s *Server) closeChatWorkerRuntime() {
	var workers []*chatWorker
	s.ChatMu.Lock()
	if s.ChatRuntime != nil {
		s.ChatRuntime.workersClosing = true
	} else {
		s.chatWorkersClosing = true
	}
	for _, run := range s.ChatRuns {
		if run != nil && run.ReplayCleanup != nil {
			run.ReplayCleanup.Stop()
		}
	}
	for sid, worker := range s.ChatWorkers {
		if worker != nil {
			worker.Dead = true
			if worker.IdleCleanup != nil {
				worker.IdleCleanup.Stop()
				worker.IdleCleanup = nil
			}
			workers = append(workers, worker)
		}
		delete(s.ChatWorkers, sid)
	}
	s.ChatMu.Unlock()
	for _, worker := range workers {
		stopChatWorker(worker)
	}
}
