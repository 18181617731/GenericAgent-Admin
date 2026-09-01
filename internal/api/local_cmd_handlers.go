package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type localCmdCreateRequest struct {
	Path string `json:"path"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

type localCmdInputRequest struct {
	Data     string `json:"data"`
	Base64   string `json:"base64"`
	Encoding string `json:"encoding"`
}

type localCmdResizeRequest struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

type localCmdWireEvent struct {
	Type         string `json:"type"`
	Seq          uint64 `json:"seq"`
	Data         string `json:"data,omitempty"`
	Status       string `json:"status,omitempty"`
	Path         string `json:"path,omitempty"`
	ExitCode     *int   `json:"exit_code,omitempty"`
	Message      string `json:"message,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
	ReplayFrom   uint64 `json:"replay_from,omitempty"`
	Created      string `json:"created,omitempty"`
	LastActivity string `json:"last_activity,omitempty"`
}

func (s *Server) localCmdCreateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	registry, instanceID, err := s.localCmdRegistryForRequest(r)
	if err != nil {
		status := http.StatusInternalServerError
		var notFound *chatInstanceNotFoundError
		if errors.As(err, &notFound) || strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		bad(w, status, err.Error())
		return
	}
	setResolvedInstanceHeader(w, instanceID)
	if !localCmdSupported() {
		bad(w, http.StatusNotImplemented, errLocalCmdRemoteUnsupported.Error())
		return
	}
	var req localCmdCreateRequest
	if err := decodeLocalCmdJSON(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	dir, err := validateLocalCmdDirectory(req.Path)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	cols, rows := normalizedLocalCmdSize(req.Cols, req.Rows)
	session, err := registry.create(dir, cols, rows)
	if err != nil {
		bad(w, localCmdErrorStatus(err), err.Error())
		return
	}
	writeJSON(w, localCmdSessionResponse(session.metadata()))
}

func decodeLocalCmdJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, localCmdMaxInput*2))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func normalizedLocalCmdSize(cols, rows int) (int, int) {
	if cols == 0 {
		cols = localCmdDefaultCols
	}
	if rows == 0 {
		rows = localCmdDefaultRows
	}
	return cols, rows
}

func localCmdSessionResponse(meta localCmdMetadata) map[string]any {
	return map[string]any{"id": meta.ID, "path": meta.Path, "status": meta.Status, "seq": meta.Seq, "exit_code": meta.ExitCode, "created": meta.Created.Format(time.RFC3339Nano), "last_activity": meta.LastActivity.Format(time.RFC3339Nano)}
}

func (s *Server) localCmdSessionRoute(w http.ResponseWriter, r *http.Request) {
	id, action, ok := parseLocalCmdSessionPath(r.URL.Path)
	if !ok {
		bad(w, http.StatusNotFound, "local CMD session path not found")
		return
	}
	registry, instanceID, err := s.localCmdRegistryForRequest(r)
	if err != nil {
		status := http.StatusInternalServerError
		var notFound *chatInstanceNotFoundError
		if errors.As(err, &notFound) || strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		bad(w, status, err.Error())
		return
	}
	setResolvedInstanceHeader(w, instanceID)
	session, found := registry.get(id)
	if !found {
		bad(w, http.StatusNotFound, "local CMD session not found")
		return
	}
	switch action {
	case "":
		s.localCmdSessionStatus(w, r, registry, session)
	case "stream":
		s.localCmdSessionStream(w, r, session)
	case "input":
		s.localCmdSessionInput(w, r, session)
	case "resize":
		s.localCmdSessionResize(w, r, session)
	default:
		bad(w, http.StatusNotFound, "local CMD session action not found")
	}
}

func parseLocalCmdSessionPath(path string) (string, string, bool) {
	const prefix = "/api/local-cmd/sessions/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(path[len(prefix):], "/"), "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func (s *Server) localCmdSessionStatus(w http.ResponseWriter, r *http.Request, registry *localCmdRegistry, session *localCmdSession) {
	if r.Method == http.MethodDelete {
		if registry.delete(session.id) {
			writeJSON(w, map[string]any{"ok": true, "id": session.id})
			return
		}
		bad(w, http.StatusNotFound, "local CMD session not found")
		return
	}
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, localCmdSessionResponse(session.metadata()))
}

func (s *Server) localCmdSessionStream(w http.ResponseWriter, r *http.Request, session *localCmdSession) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	from, err := parseLocalCmdFrom(r)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		bad(w, http.StatusInternalServerError, "streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	session.attach()
	defer session.detach()
	meta := session.metadata()
	events, wait, done, truncated := session.eventSnapshot(from)
	if err := writeLocalCmdEvent(w, flusher, localCmdWireEvent{Type: "sync", Seq: meta.Seq, Status: meta.Status, Path: meta.Path, ExitCode: meta.ExitCode, Created: meta.Created.Format(time.RFC3339Nano), LastActivity: meta.LastActivity.Format(time.RFC3339Nano), Truncated: truncated, ReplayFrom: from}); err != nil {
		return
	}
	from, err = writeLocalCmdStreamEvents(w, flusher, from, events)
	if err != nil {
		return
	}
	if done {
		return
	}
	for {
		select {
		case <-wait:
			events, wait, done, _ = session.eventSnapshot(from)
			from, err = writeLocalCmdStreamEvents(w, flusher, from, events)
			if err != nil {
				return
			}
			if done {
				return
			}
		case <-r.Context().Done():
			return
		case <-time.After(30 * time.Second):
			meta = session.metadata()
			if err := writeLocalCmdEvent(w, flusher, localCmdWireEvent{Type: "sync", Seq: meta.Seq, Status: meta.Status, Path: meta.Path, ExitCode: meta.ExitCode}); err != nil {
				return
			}
			if meta.Status != "running" {
				return
			}
		}
	}
}

func writeLocalCmdStreamEvents(w http.ResponseWriter, flusher http.Flusher, from uint64, events []localCmdEvent) (uint64, error) {
	for _, event := range events {
		if err := writeLocalCmdEvent(w, flusher, localCmdWireEventFrom(event)); err != nil {
			return from, err
		}
		from = event.seq
	}
	return from, nil
}

func parseLocalCmdFrom(r *http.Request) (uint64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("from"))
	if raw == "" {
		return 0, nil
	}
	from, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, errors.New("from must be a non-negative sequence number")
	}
	return from, nil
}

func writeLocalCmdEvent(w http.ResponseWriter, flusher http.Flusher, event localCmdWireEvent) error {
	if err := json.NewEncoder(w).Encode(event); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func localCmdWireEventFrom(event localCmdEvent) localCmdWireEvent {
	result := localCmdWireEvent{Type: event.kind, Seq: event.seq, Message: event.message}
	if event.kind == "data" {
		result.Data = base64.StdEncoding.EncodeToString(event.data)
	}
	if event.kind == "exit" {
		result.ExitCode = &event.exit
	}
	return result
}
