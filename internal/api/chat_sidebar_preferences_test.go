package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func sidebarRequest(t *testing.T, h http.Handler, method, body string, status int) map[string]json.RawMessage {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(method, "/api/chat/sidebar-preferences?instance_id=not-an-instance", strings.NewReader(body)))
	if rr.Code != status {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if status != 200 {
		return nil
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("preferences must not be cached")
	}
	var result struct {
		Preferences map[string]json.RawMessage `json:"preferences"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Preferences
}

func TestChatSidebarPreferencesPersistMergeAndInitialize(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	h := s.Routes()
	if sidebarRequest(t, h, "GET", "", 200) != nil {
		t.Fatal("fresh preferences should be null")
	}
	first := `{"preferences":{"sort":"priority","showPinned":false,"sectionOrder":["recent","pinned","projects","conductors"]},"initialize":true}`
	sidebarRequest(t, h, "PUT", first, 200)
	before, err := os.ReadFile(s.chatSidebarPreferencesPath())
	if err != nil {
		t.Fatal(err)
	}
	sidebarRequest(t, h, "PUT", first, 200)
	after, _ := os.ReadFile(s.chatSidebarPreferencesPath())
	if !bytes.Equal(before, after) {
		t.Fatal("repeated initialize changed persisted state")
	}
	sidebarRequest(t, h, "PUT", `{"preferences":{"sort":"updated"},"initialize":true}`, 200)
	after, _ = os.ReadFile(s.chatSidebarPreferencesPath())
	if !bytes.Equal(before, after) {
		t.Fatal("another browser overwrote initial settings")
	}
	sidebarRequest(t, h, "PUT", `{"preferences":{"showProjects":false}}`, 200)
	// Re-create Server to prove state comes from the file, not the original object.
	restarted := New(s.BaseCfgStore, nil, s.Models, nil)
	got := sidebarRequest(t, restarted.Routes(), "GET", "", 200)
	if string(got["sort"]) != `"priority"` || string(got["showPinned"]) != "false" || string(got["showProjects"]) != "false" {
		t.Fatalf("lost preferences: %v", got)
	}
}

func TestChatSidebarPreferencesConcurrentAndInvalid(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	h := s.Routes()
	var wg sync.WaitGroup
	for _, body := range []string{`{"preferences":{"showPinned":false}}`, `{"preferences":{"showRecent":false}}`, `{"preferences":{"sort":"priority"}}`} {
		wg.Add(1)
		go func(body string) { defer wg.Done(); sidebarRequest(t, h, "PUT", body, 200) }(body)
	}
	wg.Wait()
	got := sidebarRequest(t, h, "GET", "", 200)
	if len(got) != 3 {
		t.Fatalf("concurrent update lost fields: %v", got)
	}
	before, _ := os.ReadFile(s.chatSidebarPreferencesPath())
	for _, body := range []string{`{"preferences":null}`, `{"preferences":{"showPinned":null}}`, `{"preferences":{"sort":"bad"}}`, `{"preferences":{"sectionOrder":["unknown"]}}`, `{"preferences":{"unknown":true}}`, `{"preferences":{"showRecent":"false"}}`, `{"preferences":{}} {}`, `{"preferences":{},"unknown":true}`, `{"preferences":{"expandedProjectNames":["` + strings.Repeat("x", 65<<10) + `"]}}`} {
		sidebarRequest(t, h, "PUT", body, 400)
	}
	sidebarRequest(t, h, "POST", `{}`, 405)
	after, _ := os.ReadFile(s.chatSidebarPreferencesPath())
	if !bytes.Equal(before, after) {
		t.Fatal("invalid requests changed persistent state")
	}
}
