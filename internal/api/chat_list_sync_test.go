package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func listSyncRequest(t *testing.T, s *Server, items []map[string]interface{}, metadata map[string]interface{}, since string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/chat/sessions?sync=1&since="+since, nil)
	s.writeChatListSync(rec, req, items, metadata)
	var body map[string]interface{}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
	}
	return rec, body
}

func TestChatListSyncCompactAndDelta(t *testing.T) {
	s := newChatLoopTestServer(t)
	metadata := map[string]interface{}{"projects": []string{"project"}, "project_items": []string{}, "pinned_projects": []string{}, "project_order": []string{}}
	items := []map[string]interface{}{
		{"id": "a", "title": "Alpha", "updated_at": 2, "conductor": &chatConductorState{Role: "worker", ParentSessionID: "p", DispatchID: "d", Status: "running", Recovery: "interrupted", Objective: strings.Repeat("private objective ", 1000)}, "loop": chatLoopState{Enabled: true, Status: "running", Round: 3, ControllerPrompt: strings.Repeat("private prompt ", 1000)}},
		{"id": "b", "title": "Beta", "updated_at": 1},
	}
	_, full := listSyncRequest(t, s, items, metadata, "")
	if full["reset"] != true || len(full["sessions"].([]interface{})) != 2 {
		t.Fatalf("full = %#v", full)
	}
	item := full["sessions"].([]interface{})[0].(map[string]interface{})
	conductor := item["conductor"].(map[string]interface{})
	loop := item["loop"].(map[string]interface{})
	if len(conductor) != 5 || conductor["parent_session_id"] != "p" || conductor["recovery"] != "interrupted" || len(loop) != 3 || loop["round"] != float64(3) {
		t.Fatalf("compact = %#v", item)
	}
	if items[0]["conductor"].(*chatConductorState).Objective == "" || items[0]["loop"].(chatLoopState).ControllerPrompt == "" {
		t.Fatal("compaction mutated detail")
	}
	revision := full["revision"].(string)
	rec, _ := listSyncRequest(t, s, items, metadata, revision)
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("unchanged = %d %s", rec.Code, rec.Body.String())
	}
	items[0]["running"] = true // updated_at intentionally unchanged
	items = append(items[:1], map[string]interface{}{"id": "c", "title": "Gamma", "updated_at": 3})
	_, delta := listSyncRequest(t, s, items, metadata, revision)
	if delta["reset"] != false || delta["base_revision"] != revision || len(delta["session_upserts"].([]interface{})) != 2 {
		t.Fatalf("delta = %#v", delta)
	}
	removed := delta["removed_session_ids"].([]interface{})
	if len(removed) != 1 || removed[0] != "b" {
		t.Fatalf("removed = %#v", removed)
	}
	if _, ok := delta["projects"]; ok {
		t.Fatal("unchanged metadata resent")
	}
	metadata["projects"] = []string{"other"}
	_, changed := listSyncRequest(t, s, items, metadata, delta["revision"].(string))
	if len(changed["session_upserts"].([]interface{})) != 0 || changed["projects"].([]interface{})[0] != "other" {
		t.Fatalf("metadata delta = %#v", changed)
	}
}

func TestChatListSyncExpiryAndIsolation(t *testing.T) {
	s := newChatLoopTestServer(t)
	metadata := map[string]interface{}{"projects": []string{}}
	items := []map[string]interface{}{{"id": "a", "title": "start"}}
	_, full := listSyncRequest(t, s, items, metadata, "")
	oldest := full["revision"].(string)
	for i := 0; i < chatListSyncMaxSnapshots+2; i++ {
		items[0]["title"] = fmt.Sprintf("version %d", i)
		listSyncRequest(t, s, items, metadata, "")
	}
	if len(s.ChatRuntime.listSyncSnapshots) > chatListSyncMaxSnapshots || s.ChatRuntime.listSyncBytes > chatListSyncMaxBytes {
		t.Fatal("cache unbounded")
	}
	_, reset := listSyncRequest(t, s, items, metadata, oldest)
	if reset["reset"] != true || len(reset["sessions"].([]interface{})) != 1 {
		t.Fatalf("expired = %#v", reset)
	}
	other := newChatLoopTestServer(t)
	_, isolated := listSyncRequest(t, other, []map[string]interface{}{{"id": "other"}}, metadata, reset["revision"].(string))
	if isolated["reset"] != true {
		t.Fatalf("cross instance delta = %#v", isolated)
	}
	s.ChatRuntime.listSyncPath = "different-data-directory"
	_, pathReset := listSyncRequest(t, s, []map[string]interface{}{{"id": "different"}}, metadata, reset["revision"].(string))
	if pathReset["reset"] != true || len(s.ChatRuntime.listSyncSnapshots) != 1 {
		t.Fatalf("path reset = %#v", pathReset)
	}
}

func TestChatListSyncHandlerKeepsLegacyDetail(t *testing.T) {
	s := newChatLoopTestServer(t)
	cs := chatSession{ID: "sync-handler", Title: "Title", UpdatedAt: 10, Conductor: &chatConductorState{Role: "parent", Objective: "detail remains available"}, Loop: chatLoopState{Enabled: true, Status: "waiting", ControllerPrompt: "full prompt"}}
	saveChatLoopTestSession(t, s, cs)
	request := func(url string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.chatSessions(rec, httptest.NewRequest(http.MethodGet, url, nil))
		return rec
	}
	legacy := request("/api/chat/sessions")
	compact := request("/api/chat/sessions?sync=1")
	if legacy.Code != 200 || !strings.Contains(legacy.Body.String(), "detail remains available") || !strings.Contains(legacy.Body.String(), "full prompt") {
		t.Fatalf("legacy changed: %s", legacy.Body.String())
	}
	if compact.Code != 200 || strings.Contains(compact.Body.String(), "detail remains available") || strings.Contains(compact.Body.String(), "full prompt") || !strings.Contains(compact.Body.String(), `"role":"parent"`) {
		t.Fatalf("sync = %s", compact.Body.String())
	}
	legacyAgain := request("/api/chat/sessions")
	if legacyAgain.Body.String() != legacy.Body.String() {
		t.Fatal("sync mutated legacy response")
	}
}
