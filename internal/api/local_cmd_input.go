package api

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
)

func (s *Server) localCmdSessionInput(w http.ResponseWriter, r *http.Request, session *localCmdSession) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req localCmdInputRequest
	if err := decodeLocalCmdJSON(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	data, err := localCmdInputBytes(req)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(data) > localCmdMaxInput {
		bad(w, http.StatusRequestEntityTooLarge, "input exceeds 64 KiB")
		return
	}
	if err := session.write(data); err != nil {
		bad(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "bytes": len(data)})
}

func localCmdInputBytes(req localCmdInputRequest) ([]byte, error) {
	encoded := strings.TrimSpace(req.Base64)
	if encoded == "" && strings.EqualFold(strings.TrimSpace(req.Encoding), "base64") {
		encoded = req.Data
	}
	if encoded != "" {
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, errors.New("input base64 is invalid")
		}
		return data, nil
	}
	if req.Encoding != "" && !strings.EqualFold(req.Encoding, "raw") {
		return nil, errors.New("input encoding must be raw or base64")
	}
	return []byte(req.Data), nil
}

func (s *Server) localCmdSessionResize(w http.ResponseWriter, r *http.Request, session *localCmdSession) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req localCmdResizeRequest
	if err := decodeLocalCmdJSON(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	cols, rows := normalizedLocalCmdSize(req.Cols, req.Rows)
	if err := session.resize(cols, rows); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "cols": cols, "rows": rows})
}

func localCmdErrorStatus(err error) int {
	if errors.Is(err, errLocalCmdRemoteUnsupported) {
		return http.StatusNotImplemented
	}
	if strings.Contains(err.Error(), "session limit") {
		return http.StatusTooManyRequests
	}
	return http.StatusBadRequest
}
