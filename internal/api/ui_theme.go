package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"genericagent-admin-go/internal/config"
)

func (s *Server) uiTheme(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]string{"theme": effectiveUITheme(s.storedUITheme())})
		return
	case http.MethodPut:
		if !requireDangerousHeader(w, r) {
			return
		}
		var req struct {
			Theme string `json:"theme"`
		}
		if err := decode(r, &req); err != nil {
			bad(w, http.StatusBadRequest, err.Error())
			return
		}
		theme := canonicalUITheme(req.Theme)
		if theme == "" {
			bad(w, http.StatusBadRequest, "ui_theme must be one of light, warm, dark")
			return
		}
		if s.ConfigMu != nil {
			s.ConfigMu.Lock()
			defer s.ConfigMu.Unlock()
		}
		cfg := s.CfgStore.Snapshot()
		cfg.UITheme = theme
		if err := s.CfgStore.Save(cfg); err != nil {
			bad(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]string{"theme": s.CfgStore.Snapshot().UITheme})
		return
	default:
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) storedUITheme() string {
	if s == nil || s.CfgStore == nil {
		return ""
	}
	return canonicalUITheme(s.CfgStore.Snapshot().UITheme)
}

func canonicalUITheme(value string) string {
	theme := strings.TrimSpace(value)
	if config.ValidUITheme(theme) {
		return theme
	}
	return ""
}

func effectiveUITheme(value string) string {
	if theme := canonicalUITheme(value); theme != "" {
		return theme
	}
	return config.DefaultUITheme
}

func injectUITheme(data []byte, theme string) []byte {
	theme = canonicalUITheme(theme)
	if theme == "" || len(data) == 0 {
		return data
	}
	encoded, err := json.Marshal(theme)
	if err != nil {
		return data
	}
	snippet := append(append([]byte(`<script>window.__GA_UI_THEME__=`), encoded...), []byte(`;</script>`)...)
	if i := bytes.Index(data, []byte("</head>")); i >= 0 {
		out := make([]byte, 0, len(data)+len(snippet))
		out = append(out, data[:i]...)
		out = append(out, snippet...)
		out = append(out, data[i:]...)
		return out
	}
	return append(snippet, data...)
}
