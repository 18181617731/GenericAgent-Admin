package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"genericagent-admin-go/internal/config"
)

func TestBuiltInBBSCompatFlow(t *testing.T) {
	root := t.TempDir()
	cfg := config.NewStore(root)
	cfg.Cfg.Host = "127.0.0.1"
	cfg.Cfg.Port = 8787
	srv := New(cfg, nil, nil, nil)
	h := srv.Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/bbs/status", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status code=%d body=%s", rr.Code, rr.Body.String())
	}
	var status map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status["board_key"] != "ga-team" || !strings.HasSuffix(status["path"].(string), filepath.Join("data", "bbs.json")) {
		t.Fatalf("unexpected status: %#v", status)
	}

	blockedCfg := httptest.NewRecorder()
	h.ServeHTTP(blockedCfg, httptest.NewRequest(http.MethodPost, "/api/bbs/config", bytes.NewReader([]byte(`{"mode":"builtin"}`))))
	if blockedCfg.Code != http.StatusPreconditionRequired {
		t.Fatalf("unguarded bbs config code=%d body=%s", blockedCfg.Code, blockedCfg.Body.String())
	}

	body := []byte(`{"title":"task one","content":"please handle","author":"admin","tags":["task"]}`)

	blocked := httptest.NewRecorder()
	h.ServeHTTP(blocked, httptest.NewRequest(http.MethodPost, "/api/bbs/posts", bytes.NewReader(body)))
	if blocked.Code != http.StatusPreconditionRequired {
		t.Fatalf("unguarded api create code=%d body=%s", blocked.Code, blocked.Body.String())
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/bbs/posts", bytes.NewReader(body))
	req.Header.Set("X-GA-Confirm", "dangerous")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create code=%d body=%s", rr.Code, rr.Body.String())
	}
	var post bbsPost
	if err := json.Unmarshal(rr.Body.Bytes(), &post); err != nil {
		t.Fatalf("decode post: %v", err)
	}
	if post.ID != 1 || post.Title != "task one" || post.Author != "admin" {
		t.Fatalf("unexpected post: %#v", post)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/posts?key=ga-team&limit=5", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("compat posts code=%d body=%s", rr.Code, rr.Body.String())
	}
	var posts []bbsPost
	if err := json.Unmarshal(rr.Body.Bytes(), &posts); err != nil || len(posts) != 1 || posts[0].ID != 1 {
		t.Fatalf("unexpected compat posts err=%v posts=%#v", err, posts)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/reply?key=ga-team", bytes.NewReader([]byte(`{"post_id":1,"author":"worker","content":"done"}`))))
	if rr.Code != http.StatusOK {
		t.Fatalf("compat reply code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/post?key=ga-team&id=1", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("compat post code=%d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &post); err != nil {
		t.Fatalf("decode final post: %v", err)
	}
	if len(post.Replies) != 1 || post.Replies[0].Author != "worker" || post.Replies[0].Content != "done" {
		t.Fatalf("unexpected replies: %#v", post.Replies)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/posts?key=wrong", nil))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("wrong key code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestBBSCompatContractReadmeKeyAndValidation(t *testing.T) {
	root := t.TempDir()
	cfg := config.NewStore(root)
	cfg.Cfg.Host = "127.0.0.1"
	cfg.Cfg.Port = 8787
	srv := New(cfg, nil, nil, nil)
	h := srv.Routes()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readme?key=wrong", nil))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("readme wrong key code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readme?key=ga-team", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "GET /posts?limit=10&key=BOARD_KEY") || !strings.Contains(rr.Body.String(), "POST /reply?key=BOARD_KEY") {
		t.Fatalf("unexpected readme code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/posts?key=ga-team", bytes.NewReader([]byte(`{"title":"","content":"missing title"}`))))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty title code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/reply?key=ga-team", bytes.NewReader([]byte(`{"post_id":404,"content":"lost"}`))))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing post reply code=%d body=%s", rr.Code, rr.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/posts?limit=2", nil)
	req.Header.Set("X-API-Key", "ga-team")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("header key list code=%d body=%s", rr.Code, rr.Body.String())
	}
	var posts []bbsPost
	if err := json.Unmarshal(rr.Body.Bytes(), &posts); err != nil || len(posts) != 0 {
		t.Fatalf("unexpected empty posts err=%v posts=%#v", err, posts)
	}

	for _, path := range []string{"/posts?limit=0&key=ga-team", "/posts?limit=500&key=ga-team", "/posts?limit=abc&key=ga-team", "/post?id=abc&key=ga-team"} {
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("invalid query %s code=%d body=%s", path, rr.Code, rr.Body.String())
		}
	}
}

func TestBBSExternalProxyRejectsOversizedBody(t *testing.T) {
	root := t.TempDir()
	cfg := config.NewStore(root)
	upstreamHit := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	srv := New(cfg, nil, nil, nil)
	if err := srv.saveBBSConfig(bbsConfig{Mode: "external", BaseURL: upstream.URL, BoardKey: "ga-team"}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/bbs/posts", strings.NewReader(strings.Repeat("x", maxBBSProxyBodyBytes+1)))
	req.Header.Set("X-GA-Confirm", "dangerous")
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized proxy body code=%d body=%s", rr.Code, rr.Body.String())
	}
	if upstreamHit {
		t.Fatal("oversized proxy body reached upstream")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("upstream read failed") }
func (errReader) Close() error             { return nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBBSExternalProxyReportsUpstreamBodyReadError(t *testing.T) {
	root := t.TempDir()
	cfg := config.NewStore(root)
	srv := New(cfg, nil, nil, nil)
	if err := srv.saveBBSConfig(bbsConfig{Mode: "external", BaseURL: "http://bbs.example", BoardKey: "ga-team"}); err != nil {
		t.Fatal(err)
	}

	oldClient := bbsProxyHTTPClient
	bbsProxyHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       errReader{},
			Request:    r,
		}, nil
	})}
	t.Cleanup(func() { bbsProxyHTTPClient = oldClient })

	req := httptest.NewRequest(http.MethodGet, "/api/bbs/posts", nil)
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("read-error proxy code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "upstream read failed") {
		t.Fatalf("body=%s want upstream read error", rr.Body.String())
	}
}

type closeErrReader struct{ *strings.Reader }

func (closeErrReader) Close() error { return errors.New("upstream close failed") }

func TestBBSExternalProxyReportsUpstreamBodyCloseError(t *testing.T) {
	root := t.TempDir()
	cfg := config.NewStore(root)
	srv := New(cfg, nil, nil, nil)
	if err := srv.saveBBSConfig(bbsConfig{Mode: "external", BaseURL: "http://bbs.example", BoardKey: "ga-team"}); err != nil {
		t.Fatal(err)
	}

	oldClient := bbsProxyHTTPClient
	bbsProxyHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       closeErrReader{strings.NewReader(`[]`)},
			Request:    r,
		}, nil
	})}
	t.Cleanup(func() { bbsProxyHTTPClient = oldClient })

	req := httptest.NewRequest(http.MethodGet, "/api/bbs/posts", nil)
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("close-error proxy code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "close upstream response body") {
		t.Fatalf("body=%s want upstream close error", rr.Body.String())
	}
}

func TestBBSStatusReportsExternalBodyCloseError(t *testing.T) {
	root := t.TempDir()
	cfg := config.NewStore(root)
	srv := New(cfg, nil, nil, nil)
	if err := srv.saveBBSConfig(bbsConfig{Mode: "external", BaseURL: "http://bbs.example", BoardKey: "ga-team"}); err != nil {
		t.Fatal(err)
	}

	oldClient := bbsStatusHTTPClient
	bbsStatusHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       closeErrReader{strings.NewReader(`[]`)},
			Request:    r,
		}, nil
	})}
	t.Cleanup(func() { bbsStatusHTTPClient = oldClient })

	req := httptest.NewRequest(http.MethodGet, "/api/bbs/status", nil)
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "close external BBS response body") {
		t.Fatalf("body=%s want external close error", rr.Body.String())
	}
}

func TestBBSPersistenceWritesAtomicallyAndCreatesDirectory(t *testing.T) {
	root := t.TempDir()
	cfg := config.NewStore(root)
	srv := New(cfg, nil, nil, nil)

	if err := srv.saveBBSConfig(bbsConfig{Mode: "external", BaseURL: "http://bbs.example/", BoardKey: " team "}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := srv.saveBBS(bbsState{BoardKey: "team", NextID: 2, NextReply: 1, Posts: []bbsPost{{ID: 1, Title: "hello", Content: "world"}}}); err != nil {
		t.Fatalf("save bbs: %v", err)
	}

	for _, path := range []string{srv.bbsConfigPath(), srv.bbsPath()} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !json.Valid(b) {
			t.Fatalf("%s contains invalid JSON: %q", path, string(b))
		}
		matches, err := filepath.Glob(path + "-*.tmp")
		if err != nil {
			t.Fatalf("glob temp files for %s: %v", path, err)
		}
		if len(matches) != 0 {
			t.Fatalf("leftover temp files for %s: %v", path, matches)
		}
	}

	loadedCfg, err := srv.loadBBSConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loadedCfg.Mode != "external" || loadedCfg.BaseURL != "http://bbs.example" || loadedCfg.BoardKey != "team" {
		t.Fatalf("unexpected config: %+v", loadedCfg)
	}
	loaded, err := srv.loadBBS()
	if err != nil {
		t.Fatalf("load bbs: %v", err)
	}
	if loaded.NextID != 2 || len(loaded.Posts) != 1 || loaded.Posts[0].Title != "hello" {
		t.Fatalf("unexpected bbs state: %+v", loaded)
	}
}
