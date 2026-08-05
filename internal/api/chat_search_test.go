package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestChatSearchFindsTitlesContentAndProjects(t *testing.T) {
	root := t.TempDir()
	s := newGoalTestServer(t, root)
	s.CfgStore.Cfg.ChatDataDir = filepath.Join(root, "chat-data")
	if err := saveChatSessionLocked(s.CfgStore.Cfg, chatSession{
		ID: "title-hit", Title: "模型切换记录", UpdatedAt: 30, Messages: []chatMessage{{ID: "m1", Role: "user", Content: "验证模型顺序", CreatedAt: 30}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveChatSessionLocked(s.CfgStore.Cfg, chatSession{
		ID: "content-hit", Title: "日常记录", UpdatedAt: 20, Messages: []chatMessage{{ID: "m2", Role: "assistant", Content: "北京时间现在是下午三点", CreatedAt: 20}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveChatSessionLocked(s.CfgStore.Cfg, chatSession{
		ID: "project-hit", Title: "项目讨论", ProjectMode: "release-tools", UpdatedAt: 10, Messages: []chatMessage{{ID: "m3", Role: "user", Content: "准备发布", CreatedAt: 10}},
	}); err != nil {
		t.Fatal(err)
	}
	h := s.Routes()
	for _, tc := range []struct {
		name      string
		query     string
		scope     string
		wantID    string
		wantMatch string
	}{
		{name: "title", query: "模型切换", scope: "title", wantID: "title-hit", wantMatch: "title"},
		{name: "content", query: "北京时间", scope: "content", wantID: "content-hit", wantMatch: "content"},
		{name: "project", query: "release", scope: "project", wantID: "project-hit", wantMatch: "project"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/chat/search?q="+tc.query+"&scope="+tc.scope, nil)
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			var body struct {
				Results []chatSearchResult `json:"results"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Results) != 1 || body.Results[0].ID != tc.wantID || body.Results[0].MatchType != tc.wantMatch {
				t.Fatalf("results=%+v", body.Results)
			}
		})
	}
}

func TestChatSearchUsesRecentOrderAndSafeValidation(t *testing.T) {
	root := t.TempDir()
	s := newGoalTestServer(t, root)
	s.CfgStore.Cfg.ChatDataDir = filepath.Join(root, "chat-data")
	for _, session := range []chatSession{
		{ID: "older", Title: "相同关键词旧", UpdatedAt: 1, Messages: []chatMessage{{Content: "关键词"}}},
		{ID: "newer", Title: "相同关键词新", UpdatedAt: 2, Messages: []chatMessage{{Content: "关键词"}}},
	} {
		if err := saveChatSessionLocked(s.CfgStore.Cfg, session); err != nil {
			t.Fatal(err)
		}
	}
	h := s.Routes()
	for _, tc := range []struct {
		name string
		path string
		want int
	}{
		{name: "empty query", path: "/api/chat/search", want: http.StatusOK},
		{name: "bad scope", path: "/api/chat/search?q=x&scope=bad", want: http.StatusBadRequest},
		{name: "bad limit", path: "/api/chat/search?q=x&limit=zero", want: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rr.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/chat/search?q="+strings.Repeat("x", 161), nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("long query status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/chat/search?q=关键词&limit=1", nil))
	var body struct {
		Results []chatSearchResult `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Results) != 1 || body.Results[0].ID != "newer" {
		t.Fatalf("limited results=%+v", body.Results)
	}
}
