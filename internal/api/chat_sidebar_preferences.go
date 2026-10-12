package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// This is an Admin-wide UI preference, not a selected GA instance's setting.
// Keep it outside runtime/config files and serialize read-modify-write updates.
var chatSidebarPreferencesMu sync.Mutex

func (s *Server) chatSidebarPreferencesPath() string {
	store := s.BaseCfgStore
	if store == nil {
		store = s.CfgStore
	}
	return filepath.Join(chatDataDir(store.Snapshot()), "chat_sidebar_preferences.json")
}

func validateSidebarPreferences(prefs map[string]json.RawMessage) error {
	if prefs == nil {
		return errors.New("preferences must be an object")
	}
	for key, raw := range prefs {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New("preference values cannot be null")
		}
		switch key {
		case "showPinned", "showRecent", "showProjects", "showConductor", "showAllProjects", "historyExpanded", "pinnedExpanded", "projectsExpanded", "conductorsExpanded":
			var value bool
			if err := json.Unmarshal(raw, &value); err != nil {
				return errors.New(key + " must be a boolean")
			}
		case "sort":
			var value string
			if json.Unmarshal(raw, &value) != nil || (value != "updated" && value != "priority") {
				return errors.New("invalid sidebar sort")
			}
		case "sectionOrder", "recentFilter", "expandedProjectNames":
			var value []string
			if json.Unmarshal(raw, &value) != nil || len(value) > 1000 {
				return errors.New("invalid " + key)
			}
			for _, item := range value {
				if len(item) > 512 {
					return errors.New("preference entry is too long")
				}
				if key == "sectionOrder" && item != "pinned" && item != "conductors" && item != "projects" && item != "recent" {
					return errors.New("invalid sidebar section")
				}
				if key == "recentFilter" && item != "chat" && item != "project" && item != "conductor" {
					return errors.New("invalid recent filter")
				}
			}
		case "sectionLayout":
			var layout struct {
				Tabs []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Icon string `json:"icon"`
				} `json:"tabs"`
				Sections map[string]struct {
					Tabs  []string `json:"tabs"`
					Count int      `json:"count"`
				} `json:"sections"`
				Active            string `json:"active"`
				ShowActiveTabName bool   `json:"showActiveTabName"`
			}
			if json.Unmarshal(raw, &layout) != nil || len(layout.Tabs) > 12 || len(layout.Sections) > 4 || len(layout.Active) > 512 {
				return errors.New("invalid sidebar layout")
			}
			for _, tab := range layout.Tabs {
				if tab.ID == "" || len(tab.ID) > 512 || len([]rune(tab.Name)) > 40 || len(tab.Icon) > 40 {
					return errors.New("invalid sidebar tab")
				}
			}
			for key, section := range layout.Sections {
				if key != "pinned" && key != "conductors" && key != "projects" && key != "recent" {
					return errors.New("invalid sidebar section")
				}
				if len(section.Tabs) > 12 || (section.Count != 5 && section.Count != 10 && section.Count != 15 && section.Count != 20 && section.Count != 50 && section.Count != 100) {
					return errors.New("invalid sidebar section options")
				}
			}
		default:
			return errors.New("unknown sidebar preference: " + key)
		}
	}
	return nil
}

func readChatSidebarPreferences(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var prefs map[string]json.RawMessage
	if err := json.Unmarshal(data, &prefs); err != nil {
		return nil, err
	}
	return prefs, validateSidebarPreferences(prefs)
}

func (s *Server) chatSidebarPreferences(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		w.Header().Set("Allow", "GET, PUT")
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request struct {
		Preferences map[string]json.RawMessage `json:"preferences"`
		Initialize  bool                       `json:"initialize"`
	}
	if r.Method == http.MethodPut {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			bad(w, http.StatusBadRequest, "invalid sidebar preferences request")
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			bad(w, http.StatusBadRequest, "unexpected trailing data")
			return
		}
		if err := validateSidebarPreferences(request.Preferences); err != nil {
			bad(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	chatSidebarPreferencesMu.Lock()
	defer chatSidebarPreferencesMu.Unlock()
	path := s.chatSidebarPreferencesPath()
	prefs, err := readChatSidebarPreferences(path)
	if err != nil {
		bad(w, http.StatusInternalServerError, "cannot read sidebar preferences")
		return
	}
	if r.Method == http.MethodPut {
		if request.Initialize && prefs != nil {
			writeJSON(w, map[string]any{"preferences": prefs})
			return
		}
		before, _ := json.Marshal(prefs)
		if prefs == nil {
			prefs = make(map[string]json.RawMessage)
		}
		for key, value := range request.Preferences {
			prefs[key] = value
		}
		data, err := json.Marshal(prefs)
		if err != nil || len(data) > 64<<10 {
			bad(w, http.StatusBadRequest, "sidebar preferences exceed size limit")
			return
		}
		if !bytes.Equal(before, data) {
			if err := writeChatFileAtomic(path, data, 0600); err != nil {
				bad(w, http.StatusInternalServerError, "cannot save sidebar preferences")
				return
			}
		}
	}
	writeJSON(w, map[string]any{"preferences": prefs})
}
