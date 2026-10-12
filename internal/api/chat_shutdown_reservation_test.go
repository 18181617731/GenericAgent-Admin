package api

import "testing"

func cleanupChatLoopTestServer(t *testing.T, s *Server) {
	t.Helper()
	s.CloseChatWorkers()
	s.ChatMu.Lock()
	sessions := make(map[string]bool)
	for sid := range s.ChatRuns {
		sessions[sid] = true
	}
	for sid := range s.ChatLoopControllers {
		sessions[sid] = true
	}
	for sid := range s.ChatTitleJobs {
		sessions[sid] = true
	}
	s.ChatMu.Unlock()
	for sid := range sessions {
		cleanupChatLoopTestWorker(t, s, sid)
	}
	s.SessionMu.Lock()
	s.SessionMu.Unlock()
}

func TestClosedChatWorkersRejectLateRunReservation(t *testing.T) {
	s := newChatLoopTestServer(t)
	s.CloseChatWorkers()
	if token := s.beginChatRun("late-conductor-completion"); token != nil {
		t.Fatal("closed worker runtime accepted a late run reservation")
	}
	s.ChatMu.Lock()
	defer s.ChatMu.Unlock()
	if len(s.ChatRuns) != 0 {
		t.Fatal("rejected reservation mutated the closed runtime")
	}
}
