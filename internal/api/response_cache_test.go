package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCachedJSONCompressionAndValidation(t *testing.T) {
	payload := map[string]any{"content": strings.Repeat("snapshot ", 10000)}
	plainBody, _ := json.Marshal(payload)
	request := func(encoding, tag string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/snapshot", nil)
		r.Header.Set("Accept-Encoding", encoding)
		r.Header.Set("If-None-Match", tag)
		w := httptest.NewRecorder()
		writeCachedJSON(w, r, payload)
		return w
	}
	plain := request("identity", "")
	packed := request("gzip", "")
	if packed.Code != 200 || packed.Header().Get("Content-Encoding") != "gzip" || packed.Body.Len() >= len(plainBody)/4 {
		t.Fatalf("compression failed: headers=%v bytes=%d/%d", packed.Header(), packed.Body.Len(), len(plainBody))
	}
	zr, err := gzip.NewReader(packed.Body)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	zr.Close()
	if !bytes.Equal(decoded, plainBody) {
		t.Fatal("gzip changed the snapshot")
	}
	if plain.Header().Get("ETag") == packed.Header().Get("ETag") {
		t.Fatal("strong validators must distinguish representations")
	}
	for _, tag := range []string{packed.Header().Get("ETag"), "W/" + packed.Header().Get("ETag"), `"old", ` + packed.Header().Get("ETag"), "*"} {
		unchanged := request("gzip", tag)
		if unchanged.Code != 304 || unchanged.Body.Len() != 0 || unchanged.Header().Get("Content-Encoding") != "" {
			t.Fatalf("invalid 304: status=%d bytes=%d headers=%v", unchanged.Code, unchanged.Body.Len(), unchanged.Header())
		}
		if unchanged.Header().Get("Vary") != "Accept-Encoding" || unchanged.Header().Get("Cache-Control") != "private, no-cache" {
			t.Fatal("missing private variant cache policy")
		}
	}
	if request("identity", packed.Header().Get("ETag")).Code != 200 {
		t.Fatal("gzip validator matched identity response")
	}
	if request("gzip;q=0, *;q=1", "").Header().Get("Content-Encoding") != "" {
		t.Fatal("ignored explicit gzip opt-out")
	}
	oldTag := packed.Header().Get("ETag")
	payload["content"] = "changed"
	if request("gzip", oldTag).Code != 200 {
		t.Fatal("changed payload returned 304")
	}
	t.Logf("snapshot: identity=%d bytes gzip=%d bytes; unchanged=0 body bytes", len(plainBody), packed.Body.Len())
}

func TestCachedJSONSmallAndErrorResponses(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/snapshot", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	writeCachedJSON(w, r, map[string]bool{"running": true})
	if w.Code != 200 || w.Header().Get("Content-Encoding") != "" || w.Header().Get("ETag") == "" {
		t.Fatal("small response should not be compressed")
	}
	w = httptest.NewRecorder()
	writeCachedJSON(w, r, make(chan int))
	if w.Code != 500 || w.Header().Get("ETag") != "" {
		t.Fatal("serialization failure must not be cached")
	}
}

func TestChatListConditionalCacheTracksLiveState(t *testing.T) {
	s := newChatLoopTestServer(t)
	cs := chatSession{ID: "list-cache", Title: "Before", UpdatedAt: 123}
	saveChatLoopTestSession(t, s, cs)
	request := func(tag string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/chat/sessions", nil)
		r.Header.Set("If-None-Match", tag)
		w := httptest.NewRecorder()
		s.chatSessions(w, r)
		return w
	}
	first := request("")
	tag := first.Header().Get("ETag")
	if first.Code != 200 || tag == "" {
		t.Fatal("list must have a validator")
	}
	if cached := request(tag); cached.Code != 304 || cached.Body.Len() != 0 {
		t.Fatal("unchanged list sent another body")
	}
	// Running changes without a persisted UpdatedAt change.
	run := s.beginChatRun(cs.ID)
	if run == nil {
		t.Fatal("failed to start fixture run")
	}
	defer s.endChatRunOwned(cs.ID, run)
	running := request(tag)
	var got struct {
		Sessions []struct {
			Running bool `json:"running"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(running.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if running.Code != 200 || running.Header().Get("ETag") == tag || len(got.Sessions) != 1 || !got.Sessions[0].Running {
		t.Fatal("live state was hidden by cache")
	}
	s.endChatRunOwned(cs.ID, run)
	cs.Title = "After"
	saveChatLoopTestSession(t, s, cs)
	changed := request(tag)
	if changed.Code != 200 || changed.Header().Get("ETag") == tag || !strings.Contains(changed.Body.String(), `"After"`) {
		t.Fatal("metadata change did not invalidate list")
	}
}
