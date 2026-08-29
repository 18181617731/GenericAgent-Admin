package api

import (
	"errors"
	"io"
	"time"
)

func (s *localCmdSession) readLoop() {
	buffer := make([]byte, 32<<10)
	for {
		count, err := s.process.Read(buffer)
		if count > 0 {
			s.appendData(buffer[:count])
		}
		if err != nil {
			return
		}
	}
}

func (s *localCmdSession) waitLoop() {
	code, err := s.process.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.status != "running" {
		return
	}
	now := time.Now()
	s.lastActivity, s.finished, s.status = now, now, "exited"
	if err != nil {
		s.appendEventLocked(localCmdEvent{kind: "error", message: err.Error()})
	} else {
		s.exitCode = &code
		s.appendEventLocked(localCmdEvent{kind: "exit", exit: code})
	}
	s.signalLocked()
}

func (s *localCmdSession) appendData(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.lastActivity = time.Now()
	s.appendEventLocked(localCmdEvent{kind: "data", data: append([]byte(nil), data...)})
	s.signalLocked()
}

func (s *localCmdSession) appendEventLocked(event localCmdEvent) {
	s.seq++
	event.seq = s.seq
	if event.kind == "data" {
		if len(event.data) > localCmdOutputLimit {
			event.data = event.data[len(event.data)-localCmdOutputLimit:]
		}
		s.outputBytes += len(event.data)
	}
	s.events = append(s.events, event)
	for s.outputBytes > localCmdOutputLimit && len(s.events) > 0 {
		oldest := s.events[0]
		s.events = s.events[1:]
		s.outputBytes -= len(oldest.data)
	}
}

func (s *localCmdSession) signalLocked() {
	close(s.notify)
	s.notify = make(chan struct{})
}

func (s *localCmdSession) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.status = "closed"
	s.finished = time.Now()
	s.signalLocked()
	process := s.process
	s.mu.Unlock()
	_ = process.Kill()
	_ = process.Close()
}

func (s *localCmdSession) write(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	s.mu.Lock()
	if s.closed || s.status != "running" {
		s.mu.Unlock()
		return errors.New("local CMD session is not running")
	}
	process := s.process
	s.lastActivity = time.Now()
	s.mu.Unlock()
	count, err := process.Write(data)
	if err == nil && count != len(data) {
		return io.ErrShortWrite
	}
	return err
}

func (s *localCmdSession) resize(cols, rows int) error {
	if err := validateLocalCmdSize(cols, rows); err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed || s.status != "running" {
		s.mu.Unlock()
		return errors.New("local CMD session is not running")
	}
	process := s.process
	s.lastActivity = time.Now()
	s.mu.Unlock()
	return process.Resize(cols, rows)
}

func (s *localCmdSession) expired(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == "running" {
		if now.Sub(s.lastActivity) > localCmdIdleTimeout {
			return true
		}
		return s.connections == 0 && now.Sub(s.lastActivity) > localCmdConnectionGrace
	}
	if s.connections > 0 {
		return false
	}
	return now.Sub(s.finished) > localCmdFinishedRetention
}

func (s *localCmdSession) attach() {
	s.mu.Lock()
	s.connections++
	s.lastActivity = time.Now()
	s.mu.Unlock()
}

func (s *localCmdSession) detach() {
	s.mu.Lock()
	if s.connections > 0 {
		s.connections--
	}
	s.lastActivity = time.Now()
	s.mu.Unlock()
}

func (s *localCmdSession) eventSnapshot(from uint64) ([]localCmdEvent, <-chan struct{}, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := make([]localCmdEvent, 0, len(s.events))
	for _, event := range s.events {
		if event.seq > from {
			events = append(events, localCmdEvent{kind: event.kind, seq: event.seq, data: append([]byte(nil), event.data...), exit: event.exit, message: event.message})
		}
	}
	truncated := len(s.events) > 0 && from+1 < s.events[0].seq
	if len(s.events) == 0 && s.seq > from {
		truncated = true
	}
	return events, s.notify, s.status != "running", truncated
}

func (s *localCmdSession) metadata() localCmdMetadata {
	s.mu.Lock()
	defer s.mu.Unlock()
	return localCmdMetadata{ID: s.id, Path: s.path, Status: s.status, Seq: s.seq, ExitCode: s.exitCode, Created: s.created, LastActivity: s.lastActivity}
}

type localCmdMetadata struct {
	ID           string
	Path         string
	Status       string
	Seq          uint64
	ExitCode     *int
	Created      time.Time
	LastActivity time.Time
}
