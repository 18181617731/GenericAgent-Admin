package api

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	localCmdMaxSessions       = 3
	localCmdOutputLimit       = 2 << 20
	localCmdMaxInput          = 64 << 10
	localCmdIdleTimeout       = 20 * time.Minute
	localCmdConnectionGrace   = 2 * time.Minute
	localCmdFinishedRetention = 5 * time.Minute
	localCmdCleanupInterval   = time.Minute
	localCmdDefaultCols       = 120
	localCmdDefaultRows       = 32
	localCmdMaxCols           = 500
	localCmdMaxRows           = 200
)

type localCmdEvent struct {
	kind    string
	seq     uint64
	data    []byte
	exit    int
	message string
}

type localCmdSession struct {
	mu           sync.Mutex
	id           string
	path         string
	process      localCmdProcess
	created      time.Time
	lastActivity time.Time
	finished     time.Time
	status       string
	exitCode     *int
	seq          uint64
	events       []localCmdEvent
	outputBytes  int
	notify       chan struct{}
	connections  int
	closed       bool
}

type localCmdRegistry struct {
	mu       sync.Mutex
	sessions map[string]*localCmdSession
	stop     chan struct{}
	done     chan struct{}
	closeOne sync.Once
}

func newLocalCmdRegistry() *localCmdRegistry {
	r := &localCmdRegistry{sessions: make(map[string]*localCmdSession), stop: make(chan struct{}), done: make(chan struct{})}
	go r.cleanupLoop()
	return r
}

func (r *localCmdRegistry) create(path string, cols, rows int) (*localCmdSession, error) {
	if err := validateLocalCmdSize(cols, rows); err != nil {
		return nil, err
	}
	r.mu.Lock()
	if len(r.sessions) >= localCmdMaxSessions {
		r.mu.Unlock()
		return nil, errors.New("local CMD session limit reached")
	}
	r.mu.Unlock()
	process, err := newLocalCmdProcessFunc(path, cols, rows)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	id, err := newLocalCmdID()
	if err != nil {
		_ = process.Close()
		return nil, err
	}
	session := &localCmdSession{id: id, path: path, process: process, created: now, lastActivity: now, status: "running", notify: make(chan struct{})}
	r.mu.Lock()
	if len(r.sessions) >= localCmdMaxSessions {
		r.mu.Unlock()
		_ = process.Kill()
		_ = process.Close()
		return nil, errors.New("local CMD session limit reached")
	}
	r.sessions[id] = session
	r.mu.Unlock()
	go session.readLoop()
	go session.waitLoop()
	return session, nil
}

func newLocalCmdID() (string, error) {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate local CMD session id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func validateLocalCmdSize(cols, rows int) error {
	if cols < 0 || cols > localCmdMaxCols || rows < 0 || rows > localCmdMaxRows || (cols == 0 && rows != 0) || (rows == 0 && cols != 0) {
		return fmt.Errorf("terminal size must be 1-%d columns and 1-%d rows, or omitted", localCmdMaxCols, localCmdMaxRows)
	}
	return nil
}

func (r *localCmdRegistry) get(id string) (*localCmdSession, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	return s, ok
}

func (r *localCmdRegistry) delete(id string) bool {
	r.mu.Lock()
	s, ok := r.sessions[id]
	if ok {
		delete(r.sessions, id)
	}
	r.mu.Unlock()
	if ok {
		s.close()
	}
	return ok
}

func (r *localCmdRegistry) close() {
	r.closeOne.Do(func() {
		close(r.stop)
		<-r.done
		r.mu.Lock()
		sessions := make([]*localCmdSession, 0, len(r.sessions))
		for id, session := range r.sessions {
			delete(r.sessions, id)
			sessions = append(sessions, session)
		}
		r.mu.Unlock()
		for _, session := range sessions {
			session.close()
		}
	})
}

func (r *localCmdRegistry) cleanupLoop() {
	ticker := time.NewTicker(localCmdCleanupInterval)
	defer ticker.Stop()
	defer close(r.done)
	for {
		select {
		case <-ticker.C:
			r.cleanupExpired(time.Now())
		case <-r.stop:
			return
		}
	}
}

func (r *localCmdRegistry) cleanupExpired(now time.Time) int {
	r.mu.Lock()
	var expired []string
	for id, session := range r.sessions {
		if session.expired(now) {
			expired = append(expired, id)
		}
	}
	r.mu.Unlock()
	for _, id := range expired {
		r.delete(id)
	}
	return len(expired)
}
