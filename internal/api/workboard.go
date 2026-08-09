package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const workboardFileName = "workboard.json"

var workboardStatuses = []string{"backlog", "active", "review", "done"}
var workboardRisks = map[string]bool{"low": true, "medium": true, "high": true}

type workboardItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Owner     string `json:"owner"`
	Risk      string `json:"risk"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type workboardState struct {
	Items []workboardItem `json:"items"`
}

func (s *Server) workboard(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.WorkboardMu.Lock()
		defer s.WorkboardMu.Unlock()
		state, err := s.readWorkboard()
		if err != nil {
			bad(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]interface{}{"items": state.Items, "statuses": workboardStatuses})
	case http.MethodPost:
		var req struct {
			Title string `json:"title"`
			Owner string `json:"owner"`
			Risk  string `json:"risk"`
		}
		if err := decodeWorkboardJSON(r, &req); err != nil {
			bad(w, http.StatusBadRequest, err.Error())
			return
		}
		req.Title = strings.TrimSpace(req.Title)
		req.Owner = strings.TrimSpace(req.Owner)
		req.Risk = strings.ToLower(strings.TrimSpace(req.Risk))
		if req.Title == "" || len([]rune(req.Title)) > 160 {
			bad(w, http.StatusBadRequest, "title must be 1-160 characters")
			return
		}
		if len([]rune(req.Owner)) > 80 {
			bad(w, http.StatusBadRequest, "owner must be at most 80 characters")
			return
		}
		if !workboardRisks[req.Risk] {
			bad(w, http.StatusBadRequest, "risk must be low, medium, or high")
			return
		}
		id, err := newWorkboardID()
		if err != nil {
			bad(w, http.StatusInternalServerError, "could not create work item id")
			return
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		item := workboardItem{ID: id, Title: req.Title, Owner: req.Owner, Risk: req.Risk, Status: "backlog", CreatedAt: now, UpdatedAt: now}
		s.WorkboardMu.Lock()
		defer s.WorkboardMu.Unlock()
		state, err := s.readWorkboard()
		if err == nil {
			state.Items = append(state.Items, item)
			err = s.writeWorkboard(state)
		}
		if err != nil {
			bad(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, item)
	default:
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) workboardItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/workboard/"))
	if id == "" || strings.Contains(id, "/") {
		bad(w, http.StatusBadRequest, "invalid work item id")
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := decodeWorkboardJSON(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if workboardStatusIndex(req.Status) < 0 {
		bad(w, http.StatusBadRequest, "status must be backlog, active, review, or done")
		return
	}

	s.WorkboardMu.Lock()
	defer s.WorkboardMu.Unlock()
	state, err := s.readWorkboard()
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range state.Items {
		if state.Items[i].ID != id {
			continue
		}
		current := workboardStatusIndex(state.Items[i].Status)
		next := workboardStatusIndex(req.Status)
		if current < 0 || next-current > 1 || current-next > 1 {
			bad(w, http.StatusConflict, "status transition must move one stage at a time")
			return
		}
		state.Items[i].Status = req.Status
		state.Items[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := s.writeWorkboard(state); err != nil {
			bad(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, state.Items[i])
		return
	}
	bad(w, http.StatusNotFound, "work item not found")
}

func (s *Server) workboardPath() string {
	return filepath.Join(s.CfgStore.Snapshot().ChatDataDir, workboardFileName)
}

func (s *Server) readWorkboard() (workboardState, error) {
	var state workboardState
	data, err := os.ReadFile(s.workboardPath())
	if errors.Is(err, os.ErrNotExist) {
		state.Items = []workboardItem{}
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read workboard: %w", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("decode workboard: %w", err)
	}
	if state.Items == nil {
		state.Items = []workboardItem{}
	}
	return state, nil
}

func (s *Server) writeWorkboard(state workboardState) error {
	path := s.workboardPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create workboard directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode workboard: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".workboard-*.tmp")
	if err != nil {
		return fmt.Errorf("create workboard temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write workboard: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace workboard: %w", err)
	}
	return nil
}

func decodeWorkboardJSON(r *http.Request, dst interface{}) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func newWorkboardID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func workboardStatusIndex(status string) int {
	for i, candidate := range workboardStatuses {
		if status == candidate {
			return i
		}
	}
	return -1
}
