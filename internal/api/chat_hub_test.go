package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"genericagent-admin-go/internal/config"
)

func hubRequest(t *testing.T, h http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestEmbeddedChatHubBridgeIsMaterialized(t *testing.T) {
	path, err := writeChatHubBridgeScript()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 || !strings.Contains(string(body), "HubClient") {
		t.Fatalf("materialized bridge is invalid: %d bytes", len(body))
	}
	bridge := string(body)
	if !strings.Contains(bridge, "def stop(self):") || !strings.Contains(bridge, "set(clients) - set(wanted)") {
		t.Fatal("materialized bridge must disconnect sessions removed from Hub discovery")
	}
}

func TestChatHubAPIExposesPersistentSessionAndControlsRun(t *testing.T) {
	store := config.NewStore(t.TempDir())
	s := New(store, nil, nil, nil)
	cs := chatSession{ID: "session-1", Title: "Hub chat", Messages: []chatMessage{
		{Role: "user", Content: "first task"},
		{Role: "assistant", Content: "joined response", Outputs: []string{"step one", "step two"}},
		{Role: "user", Content: "second task"},
		{Role: "assistant", Content: "legacy response"},
	}}
	if err := saveChatSession(store.Snapshot(), cs); err != nil {
		t.Fatal(err)
	}

	const token = "test-secret"
	h := s.chatHubAPI(token)
	if got := hubRequest(t, h, "", http.MethodGet, "/sessions", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", got, http.StatusUnauthorized)
	}

	w := hubRequest(t, h, token, http.MethodGet, "/sessions", "")
	if w.Code != http.StatusOK {
		t.Fatalf("sessions status = %d: %s", w.Code, w.Body.String())
	}
	var listing struct {
		Sessions []chatHubSession `json:"sessions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Sessions) != 0 {
		t.Fatalf("sessions before opt-in = %#v, want none", listing.Sessions)
	}
	if hidden := hubRequest(t, h, token, http.MethodGet, "/session/_/session-1/outputs", ""); hidden.Code != http.StatusNotFound {
		t.Fatalf("hidden session direct access status = %d, want %d", hidden.Code, http.StatusNotFound)
	}

	toggle := httptest.NewRecorder()
	s.chatSetHubEnabled(toggle, httptest.NewRequest(http.MethodPatch, "/api/chat/hub/session-1", strings.NewReader(`{"enabled":true}`)), "session-1")
	if toggle.Code != http.StatusOK {
		t.Fatalf("enable Hub status = %d: %s", toggle.Code, toggle.Body.String())
	}
	w = hubRequest(t, h, token, http.MethodGet, "/sessions", "")
	listing.Sessions = nil
	if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Sessions) != 1 || listing.Sessions[0].Peer != "ga-admin/default/session-1" {
		t.Fatalf("sessions after opt-in = %#v", listing.Sessions)
	}

	w = hubRequest(t, h, token, http.MethodGet, "/session/_/session-1/outputs", "")
	var outputs struct {
		Tasks []map[string]interface{} `json:"tasks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &outputs); err != nil {
		t.Fatal(err)
	}
	if len(outputs.Tasks) != 2 {
		t.Fatalf("tasks = %#v, want 2", outputs.Tasks)
	}
	steps, _ := outputs.Tasks[0]["outputs"].([]interface{})
	if len(steps) != 2 || steps[0] != "step one" || steps[1] != "step two" {
		t.Fatalf("first task outputs = %#v", steps)
	}
	legacy, _ := outputs.Tasks[1]["outputs"].([]interface{})
	if len(legacy) != 1 || legacy[0] != "legacy response" {
		t.Fatalf("legacy task outputs = %#v", legacy)
	}

	run := s.beginChatRun("session-1")
	if run == nil {
		t.Fatal("beginChatRun returned nil")
	}
	w = hubRequest(t, h, token, http.MethodPost, "/session/_/session-1/put", `{"text":"remote task"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("busy put status = %d: %s", w.Code, w.Body.String())
	}
	w = hubRequest(t, h, token, http.MethodPost, "/session/_/session-1/abort", "")
	if w.Code != http.StatusOK || s.chatRunActive("session-1") {
		t.Fatalf("abort status = %d active=%v body=%s", w.Code, s.chatRunActive("session-1"), w.Body.String())
	}

	started := time.Now()
	w = hubRequest(t, h, token, http.MethodPost, "/session/_/session-1/put", `{"text":"accepted remote task"}`)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("idle put took %v; Hub requires admission ACK before its 15s timeout", elapsed)
	}
	if w.Code != http.StatusOK || !s.chatRunActive("session-1") {
		t.Fatalf("idle put status = %d active=%v body=%s", w.Code, s.chatRunActive("session-1"), w.Body.String())
	}
	w = hubRequest(t, h, token, http.MethodPost, "/session/_/session-1/abort", "")
	if w.Code != http.StatusOK {
		t.Fatalf("cleanup abort status = %d: %s", w.Code, w.Body.String())
	}

	toggle = httptest.NewRecorder()
	s.chatSetHubEnabled(toggle, httptest.NewRequest(http.MethodPatch, "/api/chat/hub/session-1", strings.NewReader(`{"enabled":false}`)), "session-1")
	if toggle.Code != http.StatusOK {
		t.Fatalf("disable Hub status = %d: %s", toggle.Code, toggle.Body.String())
	}
	w = hubRequest(t, h, token, http.MethodGet, "/sessions", "")
	listing.Sessions = nil
	if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Sessions) != 0 {
		t.Fatalf("sessions after opt-out = %#v, want none", listing.Sessions)
	}
	if hidden := hubRequest(t, h, token, http.MethodGet, "/session/_/session-1/state", ""); hidden.Code != http.StatusNotFound {
		t.Fatalf("opted-out session direct access status = %d, want %d", hidden.Code, http.StatusNotFound)
	}
}
