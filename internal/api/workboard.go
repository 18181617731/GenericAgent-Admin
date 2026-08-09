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

type workboardEvidence struct {
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

type workboardProposal struct {
	Summary  string              `json:"summary"`
	Evidence []workboardEvidence `json:"evidence"`
}

type workboardEvent struct {
	Action   string `json:"action"`
	Actor    string `json:"actor"`
	Note     string `json:"note,omitempty"`
	Revision int    `json:"revision"`
	At       string `json:"at"`
}

type workboardItem struct {
	ID                 string            `json:"id"`
	Title              string            `json:"title"`
	Outcome            string            `json:"outcome"`
	AcceptanceCriteria []string          `json:"acceptance_criteria"`
	Owner              string            `json:"owner"`
	Risk               string            `json:"risk"`
	Status             string            `json:"status"`
	Revision           int               `json:"revision"`
	Proposal           workboardProposal `json:"proposal"`
	Instructions       string            `json:"instructions"`
	Events             []workboardEvent  `json:"events"`
	CreatedAt          string            `json:"created_at"`
	UpdatedAt          string            `json:"updated_at"`
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
			Title              string   `json:"title"`
			Outcome            string   `json:"outcome"`
			AcceptanceCriteria []string `json:"acceptance_criteria"`
			Owner              string   `json:"owner"`
			Risk               string   `json:"risk"`
		}
		if err := decodeWorkboardJSON(r, &req); err != nil {
			bad(w, http.StatusBadRequest, err.Error())
			return
		}
		req.Title = strings.TrimSpace(req.Title)
		req.Outcome = strings.TrimSpace(req.Outcome)
		req.Owner = strings.TrimSpace(req.Owner)
		req.Risk = strings.ToLower(strings.TrimSpace(req.Risk))
		if req.Title == "" || len([]rune(req.Title)) > 160 {
			bad(w, http.StatusBadRequest, "title must be 1-160 characters")
			return
		}
		if req.Outcome == "" || len([]rune(req.Outcome)) > 1000 {
			bad(w, http.StatusBadRequest, "outcome must be 1-1000 characters")
			return
		}
		criteria, err := normalizeWorkboardLines(req.AcceptanceCriteria, 12, 500)
		if err != nil || len(criteria) == 0 {
			bad(w, http.StatusBadRequest, "acceptance_criteria must contain 1-12 non-empty items of at most 500 characters")
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
		item := workboardItem{
			ID: id, Title: req.Title, Outcome: req.Outcome, AcceptanceCriteria: criteria,
			Owner: req.Owner, Risk: req.Risk, Status: "backlog", Revision: 1,
			Proposal:  workboardProposal{Evidence: []workboardEvidence{}},
			Events:    []workboardEvent{{Action: "created", Actor: "human", Revision: 1, At: now}},
			CreatedAt: now, UpdatedAt: now,
		}
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
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/workboard/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "commands" {
		bad(w, http.StatusBadRequest, "expected /api/workboard/{id}/commands")
		return
	}
	id := parts[0]
	var req struct {
		Action           string              `json:"action"`
		ExpectedRevision int                 `json:"expected_revision"`
		Note             string              `json:"note"`
		Proposal         string              `json:"proposal"`
		Evidence         []workboardEvidence `json:"evidence"`
	}
	if err := decodeWorkboardJSON(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	req.Note = strings.TrimSpace(req.Note)
	req.Proposal = strings.TrimSpace(req.Proposal)
	if req.ExpectedRevision < 1 {
		bad(w, http.StatusBadRequest, "expected_revision must be positive")
		return
	}
	if len([]rune(req.Note)) > 2000 || len([]rune(req.Proposal)) > 4000 {
		bad(w, http.StatusBadRequest, "note or proposal is too long")
		return
	}
	if err := validateWorkboardCommand(&req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
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
		item := &state.Items[i]
		if item.ID != id {
			continue
		}
		if item.Revision != req.ExpectedRevision {
			bad(w, http.StatusConflict, fmt.Sprintf("stale work item: expected revision %d, current revision is %d; refresh and review before retrying", req.ExpectedRevision, item.Revision))
			return
		}
		next, actor, err := applyWorkboardCommand(item, req.Action, req.Note, req.Proposal, req.Evidence)
		if err != nil {
			bad(w, http.StatusConflict, err.Error())
			return
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		item.Status = next
		item.Revision++
		item.UpdatedAt = now
		item.Events = append(item.Events, workboardEvent{Action: req.Action, Actor: actor, Note: req.Note, Revision: item.Revision, At: now})
		if err := s.writeWorkboard(state); err != nil {
			bad(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, item)
		return
	}
	bad(w, http.StatusNotFound, "work item not found")
}

func validateWorkboardCommand(req *struct {
	Action           string              `json:"action"`
	ExpectedRevision int                 `json:"expected_revision"`
	Note             string              `json:"note"`
	Proposal         string              `json:"proposal"`
	Evidence         []workboardEvidence `json:"evidence"`
}) error {
	if req.Action != "start" && req.Action != "submit_proposal" && req.Action != "return" && req.Action != "approve" {
		return errors.New("action must be start, submit_proposal, return, or approve")
	}
	if req.Action == "return" && req.Note == "" {
		return errors.New("return requires instructions in note")
	}
	if req.Action == "submit_proposal" {
		if req.Proposal == "" || len(req.Evidence) == 0 || len(req.Evidence) > 12 {
			return errors.New("submit_proposal requires a proposal and 1-12 evidence items")
		}
		for i := range req.Evidence {
			req.Evidence[i].Label = strings.TrimSpace(req.Evidence[i].Label)
			req.Evidence[i].Detail = strings.TrimSpace(req.Evidence[i].Detail)
			if req.Evidence[i].Label == "" || req.Evidence[i].Detail == "" || len([]rune(req.Evidence[i].Label)) > 120 || len([]rune(req.Evidence[i].Detail)) > 1000 {
				return errors.New("evidence label and detail are required and must fit their limits")
			}
		}
	}
	return nil
}

func applyWorkboardCommand(item *workboardItem, action, note, proposal string, evidence []workboardEvidence) (string, string, error) {
	switch action {
	case "start":
		if item.Status != "backlog" {
			return "", "", errors.New("start requires backlog status")
		}
		return "active", "agent", nil
	case "submit_proposal":
		if item.Status != "active" {
			return "", "", errors.New("submit_proposal requires active status")
		}
		item.Proposal = workboardProposal{Summary: proposal, Evidence: evidence}
		item.Instructions = ""
		return "review", "agent", nil
	case "return":
		if item.Status != "review" {
			return "", "", errors.New("return requires review status")
		}
		item.Instructions = note
		return "active", "human", nil
	case "approve":
		if item.Status != "review" {
			return "", "", errors.New("approve requires review status")
		}
		return "done", "human", nil
	}
	return "", "", errors.New("unsupported action")
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
	for i := range state.Items {
		if state.Items[i].Revision < 1 {
			state.Items[i].Revision = 1
		}
		if state.Items[i].AcceptanceCriteria == nil {
			state.Items[i].AcceptanceCriteria = []string{}
		}
		if state.Items[i].Proposal.Evidence == nil {
			state.Items[i].Proposal.Evidence = []workboardEvidence{}
		}
		if state.Items[i].Events == nil {
			state.Items[i].Events = []workboardEvent{}
		}
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

func normalizeWorkboardLines(values []string, maxItems, maxRunes int) ([]string, error) {
	if len(values) > maxItems {
		return nil, errors.New("too many items")
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len([]rune(value)) > maxRunes {
			return nil, errors.New("invalid item")
		}
		normalized = append(normalized, value)
	}
	return normalized, nil
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
