package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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

	blockedPostCases := []string{"/posts", "/posts?key=wrong"}
	for _, path := range blockedPostCases {
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)))
		if rr.Code != http.StatusForbidden {
			t.Fatalf("compat post %s code=%d want=403 body=%s", path, rr.Code, rr.Body.String())
		}
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/posts?key=ga-team", bytes.NewReader([]byte(`{"title":"worker task","content":"from worker","author":"worker"}`))))
	if rr.Code != http.StatusOK {
		t.Fatalf("compat post valid key code=%d body=%s", rr.Code, rr.Body.String())
	}
	var compatPost bbsPost
	if err := json.Unmarshal(rr.Body.Bytes(), &compatPost); err != nil {
		t.Fatalf("decode compat post: %v", err)
	}
	if compatPost.ID != 2 || compatPost.Author != "worker" {
		t.Fatalf("unexpected compat post: %#v", compatPost)
	}

	blockedReplyCases := []string{"/reply", "/reply?key=wrong"}
	for _, path := range blockedReplyCases {
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(`{"post_id":2,"content":"blocked"}`))))
		if rr.Code != http.StatusForbidden {
			t.Fatalf("compat reply %s code=%d want=403 body=%s", path, rr.Code, rr.Body.String())
		}
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

func TestBBSBuiltInWritesRejectOversizedBodies(t *testing.T) {
	root := t.TempDir()
	cfg := config.NewStore(root)
	srv := New(cfg, nil, nil, nil)
	h := srv.Routes()

	for _, tc := range []struct {
		name    string
		method  string
		path    string
		confirm bool
	}{
		{name: "admin config", method: http.MethodPost, path: "/api/bbs/config", confirm: true},
		{name: "admin posts", method: http.MethodPost, path: "/api/bbs/posts", confirm: true},
		{name: "compat posts", method: http.MethodPost, path: "/posts?key=ga-team"},
		{name: "compat reply", method: http.MethodPost, path: "/reply?key=ga-team"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"content":"` + strings.Repeat("x", maxJSONBodyBytes) + `"}`
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(body))
			if tc.confirm {
				req.Header.Set("X-GA-Confirm", "dangerous")
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("%s code=%d want=%d body=%s", tc.name, rr.Code, http.StatusRequestEntityTooLarge, rr.Body.String())
			}
		})
	}
}

func TestBBSBuiltInWritesRejectTrailingJSONValues(t *testing.T) {
	root := t.TempDir()
	cfg := config.NewStore(root)
	srv := New(cfg, nil, nil, nil)
	h := srv.Routes()

	for _, tc := range []struct {
		name    string
		method  string
		path    string
		body    string
		confirm bool
	}{
		{name: "admin config", method: http.MethodPost, path: "/api/bbs/config", body: `{"mode":"builtin"} {"extra":true}`, confirm: true},
		{name: "admin posts", method: http.MethodPost, path: "/api/bbs/posts", body: `{"title":"task","content":"body"} {"extra":true}`, confirm: true},
		{name: "compat posts", method: http.MethodPost, path: "/posts?key=ga-team", body: `{"title":"task","content":"body"} {"extra":true}`},
		{name: "compat reply", method: http.MethodPost, path: "/reply?key=ga-team", body: `{"post_id":1,"content":"done"} {"extra":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.confirm {
				req.Header.Set("X-GA-Confirm", "dangerous")
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("%s code=%d want=%d body=%s", tc.name, rr.Code, http.StatusBadRequest, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "single JSON value") {
				t.Fatalf("%s body missing single JSON value guidance: %s", tc.name, rr.Body.String())
			}
		})
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

func TestBBSDefaultHTTPClientsUseTimeouts(t *testing.T) {
	for name, client := range map[string]*http.Client{
		"proxy":  bbsProxyHTTPClient,
		"status": bbsStatusHTTPClient,
	} {
		if client == nil {
			t.Fatalf("%s client is nil", name)
		}
		if client == http.DefaultClient {
			t.Fatalf("%s client uses http.DefaultClient", name)
		}
		if client.Timeout != bbsHTTPClientTimeout {
			t.Fatalf("%s client timeout=%s want=%s", name, client.Timeout, bbsHTTPClientTimeout)
		}
		transport, ok := client.Transport.(*http.Transport)
		if !ok || transport == nil {
			t.Fatalf("%s client transport=%T want *http.Transport", name, client.Transport)
		}
		if transport.ResponseHeaderTimeout != bbsResponseHeaderTimeout {
			t.Fatalf("%s response header timeout=%s want=%s", name, transport.ResponseHeaderTimeout, bbsResponseHeaderTimeout)
		}
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

func TestBBSStatusLimitsExternalResponseBodyAndRejectsTrailingJSON(t *testing.T) {
	root := t.TempDir()
	cfg := config.NewStore(root)
	srv := New(cfg, nil, nil, nil)
	if err := srv.saveBBSConfig(bbsConfig{Mode: "external", BaseURL: "http://bbs.example", BoardKey: "ga-team"}); err != nil {
		t.Fatal(err)
	}

	oldClient := bbsStatusHTTPClient
	t.Cleanup(func() { bbsStatusHTTPClient = oldClient })

	cases := []struct {
		name string
		body string
		want string
	}{
		{"oversized", `[` + strings.Repeat(`{"id":1,"title":"x","content":"y","author":"z"},`, 40000) + `{}`, "http: request body too large"},
		{"trailing json", `[] {}`, "single JSON value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bbsStatusHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(tc.body)),
					Request:    r,
				}, nil
			})}

			req := httptest.NewRequest(http.MethodGet, "/api/bbs/status", nil)
			rr := httptest.NewRecorder()
			srv.Routes().ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status code=%d body=%s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), `"enabled":false`) || !strings.Contains(rr.Body.String(), tc.want) {
				t.Fatalf("body=%s want disabled status with %q", rr.Body.String(), tc.want)
			}
		})
	}
}

func TestBBSAdminRoutesRejectUnsupportedMethodsBeforeExternalProxy(t *testing.T) {
	root := t.TempDir()
	cfg := config.NewStore(root)
	srv := New(cfg, nil, nil, nil)
	if err := srv.saveBBSConfig(bbsConfig{Mode: "external", BaseURL: "http://bbs.example", BoardKey: "ga-team"}); err != nil {
		t.Fatal(err)
	}

	oldClient := bbsProxyHTTPClient
	upstreamHit := false
	bbsProxyHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		upstreamHit = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Request:    r,
		}, nil
	})}
	t.Cleanup(func() { bbsProxyHTTPClient = oldClient })

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/api/bbs/posts"},
		{http.MethodPost, "/api/bbs/post?id=1"},
		{http.MethodGet, "/api/bbs/reply"},
		{http.MethodPost, "/api/bbs/readme"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			upstreamHit = false
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
			req.Header.Set("X-GA-Confirm", "dangerous")
			rr := httptest.NewRecorder()
			srv.Routes().ServeHTTP(rr, req)
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s code=%d body=%s", tc.method, tc.path, rr.Code, rr.Body.String())
			}
			if upstreamHit {
				t.Fatalf("%s %s reached external upstream before method contract", tc.method, tc.path)
			}
		})
	}
}

func TestBBSCompatRoutesRejectUnsupportedMethods(t *testing.T) {
	root := t.TempDir()
	cfg := config.NewStore(root)
	srv := New(cfg, nil, nil, nil)
	h := srv.Routes()

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/posts?key=ga-team"},
		{http.MethodPost, "/post?id=1&key=ga-team"},
		{http.MethodGet, "/reply?key=ga-team"},
		{http.MethodPost, "/readme?key=ga-team"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s code=%d body=%s", tc.method, tc.path, rr.Code, rr.Body.String())
			}
		})
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

func TestBBSConfigRejectsInvalidExternalSettings(t *testing.T) {
	root := t.TempDir()
	srv := newGoalTestServer(t, root)

	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing base url", `{"mode":"external","board_key":"team"}`, "base_url required"},
		{"relative base url", `{"mode":"external","base_url":"bbs.local","board_key":"team"}`, "absolute http(s) URL"},
		{"unsupported scheme", `{"mode":"external","base_url":"file:///tmp/bbs","board_key":"team"}`, "scheme must be http or https"},
		{"url query", `{"mode":"external","base_url":"https://bbs.example/api?key=leak","board_key":"team"}`, "must not include userinfo"},
		{"localhost", `{"mode":"external","base_url":"http://localhost:8787/bbs","board_key":"team"}`, "must not be localhost"},
		{"localhost subdomain", `{"mode":"external","base_url":"http://admin.localhost/bbs","board_key":"team"}`, "must not be localhost"},
		{"ipv4 loopback", `{"mode":"external","base_url":"http://127.0.0.1:8787/bbs","board_key":"team"}`, "private IP"},
		{"ipv4 private", `{"mode":"external","base_url":"http://10.0.0.2/bbs","board_key":"team"}`, "private IP"},
		{"ipv6 loopback", `{"mode":"external","base_url":"http://[::1]:8787/bbs","board_key":"team"}`, "private IP"},
		{"ipv6 link local", `{"mode":"external","base_url":"http://[fe80::1]/bbs","board_key":"team"}`, "private IP"},
		{"key whitespace", `{"mode":"external","base_url":"https://bbs.example/api","board_key":"team key"}`, "board_key must not contain whitespace"},
		{"invalid mode", `{"mode":"remote","base_url":"https://bbs.example/api","board_key":"team"}`, "invalid bbs mode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/bbs/config", strings.NewReader(tc.body))
			markDangerous(req)
			srv.bbsConfigHandler(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), tc.want) {
				t.Fatalf("body %q does not contain %q", rr.Body.String(), tc.want)
			}
		})
	}
}

func TestBBSConfigNormalizesValidModes(t *testing.T) {
	root := t.TempDir()
	srv := newGoalTestServer(t, root)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/bbs/config", strings.NewReader(`{"mode":"external","base_url":" https://bbs.example/api/ ","board_key":" team "}`))
	markDangerous(req)
	srv.bbsConfigHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	loaded, err := srv.loadBBSConfig()
	if err != nil {
		t.Fatalf("load bbs config: %v", err)
	}
	if loaded.Mode != "external" || loaded.BaseURL != "https://bbs.example/api" || loaded.BoardKey != "team" {
		t.Fatalf("unexpected external config: %+v", loaded)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/bbs/config", strings.NewReader(`{"mode":"builtin","base_url":"https://ignored.example","board_key":""}`))
	markDangerous(req)
	srv.bbsConfigHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	loaded, err = srv.loadBBSConfig()
	if err != nil {
		t.Fatalf("load bbs config: %v", err)
	}
	if loaded.Mode != "builtin" || loaded.BaseURL != "" || loaded.BoardKey != "ga-team" {
		t.Fatalf("unexpected builtin config: %+v", loaded)
	}
}
