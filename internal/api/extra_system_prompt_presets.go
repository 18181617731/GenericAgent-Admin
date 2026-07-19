package api

import (
	"net/http"
	"strings"

	"genericagent-admin-go/internal/config"
)

const (
	maxExtraSystemPromptPresets    = 100
	maxExtraSystemPromptNameLen    = 120
	maxExtraSystemPromptContentLen = 32000
)

func normalizeExtraSystemPromptPresets(items []config.ExtraSystemPromptPreset) ([]config.ExtraSystemPromptPreset, string) {
	if len(items) > maxExtraSystemPromptPresets {
		return nil, "too many extra system prompt presets"
	}
	out := make([]config.ExtraSystemPromptPreset, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		item.Content = strings.TrimSpace(item.Content)
		if item.ID == "" || item.Name == "" || item.Content == "" {
			return nil, "preset id, name, and content are required"
		}
		if len(item.ID) > 100 || strings.ContainsAny(item.ID, "/\\\x00\r\n\t") {
			return nil, "preset id is invalid"
		}
		if len(item.Name) > maxExtraSystemPromptNameLen {
			return nil, "preset name is too long"
		}
		if len(item.Content) > maxExtraSystemPromptContentLen {
			return nil, "preset content is too long"
		}
		if seen[item.ID] {
			return nil, "preset id must be unique"
		}
		seen[item.ID] = true
		out = append(out, item)
	}
	return out, ""
}

func findExtraSystemPromptPreset(items []config.ExtraSystemPromptPreset, id string) (config.ExtraSystemPromptPreset, bool) {
	id = strings.TrimSpace(id)
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return config.ExtraSystemPromptPreset{}, false
}

func (s *Server) extraSystemPromptPresets(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, map[string]interface{}{"presets": s.CfgStore.Cfg.ExtraSystemPromptPresets})
		return
	}
	if r.Method != http.MethodPut {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Presets []config.ExtraSystemPromptPreset `json:"presets"`
	}
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	presets, validationError := normalizeExtraSystemPromptPresets(req.Presets)
	if validationError != "" {
		bad(w, http.StatusBadRequest, validationError)
		return
	}
	s.ConfigMu.Lock()
	defer s.ConfigMu.Unlock()
	cfg := s.CfgStore.Cfg
	cfg.ExtraSystemPromptPresets = presets
	if err := s.CfgStore.Save(cfg); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"presets": s.CfgStore.Cfg.ExtraSystemPromptPresets})
}
