package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
