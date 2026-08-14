package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"genericagent-admin-go/internal/config"
)

func TestChatSessionsStateFiltersAndLegacyJSONDefaultsArchivedFalse(t *testing.T) {
	root := t.TempDir()
	s := newGoalTestServer(t, root)
	updateTestConfig(t, s.CfgStore, func(cfg *config.AppConfig) {
		cfg.ChatDataDir = filepath.Join(root, "chat-data")
	})
	for _, session := range []chatSession{
		{ID: "active", Title: "Active", UpdatedAt: 30},
		{ID: "archived", Title: "Archived", UpdatedAt: 20, Archived: true},
	} {
		if err := saveChatSessionLocked(s.CfgStore.Snapshot(), session); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(chatSessionDir(s.CfgStore.Snapshot()), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chatSessionPath(s.CfgStore.Snapshot(), "legacy"), []byte(`{"id":"legacy","title":"Legacy","updated_at":10,"messages":[],"settings":{}}`), 0644); err != nil {
		t.Fatal(err)
	}

	h := s.Routes()
	for _, tc := range []struct {
		name string
		path string
		want map[string]bool
	}{
		{name: "default active", path: "/api/chat/sessions", want: map[string]bool{"active": true, "legacy": true}},
		{name: "active", path: "/api/chat/sessions?state=active", want: map[string]bool{"active": true, "legacy": true}},
		{name: "archived", path: "/api/chat/sessions?state=archived", want: map[string]bool{"archived": true}},
		{name: "all", path: "/api/chat/sessions?state=all", want: map[string]bool{"active": true, "archived": true, "legacy": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			var payload struct {
				Sessions []struct {
					ID       string `json:"id"`
					Archived bool   `json:"archived"`
				} `json:"sessions"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			got := make(map[string]bool, len(payload.Sessions))
			for _, session := range payload.Sessions {
				got[session.ID] = session.Archived
			}
			if len(got) != len(tc.want) {
				t.Fatalf("sessions=%v want=%v", got, tc.want)
			}
			for id := range tc.want {
				archived, ok := got[id]
				if !ok {
					t.Fatalf("missing session %q in %v", id, got)
				}
				if archived != (id == "archived") {
					t.Fatalf("session %q archived=%v, want %v", id, archived, id == "archived")
				}
			}
		})
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/chat/sessions?state=unknown", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid state status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestChatArchivePreservesUpdatedAtAndClearsPinOnRestore(t *testing.T) {
	root := t.TempDir()
	s := newGoalTestServer(t, root)
	updateTestConfig(t, s.CfgStore, func(cfg *config.AppConfig) {
		cfg.ChatDataDir = filepath.Join(root, "chat-data")
	})
	const sid = "archive-target"
	if err := saveChatSessionPreserveUpdatedAtLocked(s.CfgStore.Snapshot(), chatSession{ID: sid, Title: "Target", UpdatedAt: 777, Pinned: true}); err != nil {
		t.Fatal(err)
	}
	h := s.Routes()

	patch := func(archived bool) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/chat/archive/"+sid, strings.NewReader(`{"archived":`+map[bool]string{true: "true", false: "false"}[archived]+`}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		return rr
	}
	for _, archived := range []bool{true, false} {
		rr := patch(archived)
		if rr.Code != http.StatusOK {
			t.Fatalf("archived=%v status=%d body=%s", archived, rr.Code, rr.Body.String())
		}
		var response struct {
			Archived  bool  `json:"archived"`
			Pinned    bool  `json:"pinned"`
			UpdatedAt int64 `json:"updated_at"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Archived != archived || response.Pinned || response.UpdatedAt != 777 {
			t.Fatalf("response=%+v want archived=%v, unpinned, updated_at=777", response, archived)
		}
		stored, err := loadChatSession(s.CfgStore.Snapshot(), sid)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Archived != archived || stored.Pinned || stored.UpdatedAt != 777 {
			t.Fatalf("stored=%+v want archived=%v, unpinned, updated_at=777", stored, archived)
		}
	}
}

func TestChatArchiveRejectsRunningSession(t *testing.T) {
	root := t.TempDir()
	s := newGoalTestServer(t, root)
	updateTestConfig(t, s.CfgStore, func(cfg *config.AppConfig) {
		cfg.ChatDataDir = filepath.Join(root, "chat-data")
	})
	const sid = "running-archive-target"
	if err := saveChatSessionLocked(s.CfgStore.Snapshot(), chatSession{ID: sid, Title: "Running"}); err != nil {
		t.Fatal(err)
	}
	token := s.beginChatRun(sid)
	if token == nil {
		t.Fatal("beginChatRun returned nil")
	}
	defer s.endChatRunOwned(sid, token)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/chat/archive/"+sid, strings.NewReader(`{"archived":true}`))
	req.Header.Set("Content-Type", "application/json")
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusConflict, rr.Body.String())
	}
	stored, err := loadChatSession(s.CfgStore.Snapshot(), sid)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Archived {
		t.Fatal("running session was archived")
	}
}
