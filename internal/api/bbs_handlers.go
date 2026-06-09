package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxBBSProxyBodyBytes     = 1 << 20
	maxBBSStatusBodyBytes    = 1 << 20
	bbsHTTPClientTimeout     = 30 * time.Second
	bbsResponseHeaderTimeout = 15 * time.Second
)

var (
	bbsProxyHTTPClient  = newBBSHTTPClient()
	bbsStatusHTTPClient = newBBSHTTPClient()
)

func newBBSHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = bbsResponseHeaderTimeout
	return &http.Client{Timeout: bbsHTTPClientTimeout, Transport: transport}
}

func (s *Server) bbsStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	cfg, err := s.loadBBSConfig()
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	builtinURL := s.builtinBBSBaseURL()
	if cfg.Mode == "external" {
		base := cfg.BaseURL
		if base == "" {
			writeJSON(w, map[string]any{"enabled": false, "mode": cfg.Mode, "base_url": "", "board_key": cfg.BoardKey, "builtin_base_url": builtinURL, "error": "external base_url is empty"})
			return
		}
		posts, proxyErr := s.fetchExternalBBSPosts(r.Context(), cfg, 1)
		resp := map[string]any{"enabled": proxyErr == nil, "mode": cfg.Mode, "base_url": base, "board_key": cfg.BoardKey, "builtin_base_url": builtinURL, "posts": len(posts), "readme": base + "/readme?key=" + cfg.BoardKey}
		if proxyErr != nil {
			resp["error"] = proxyErr.Error()
		}
		writeJSON(w, resp)
		return
	}
	bbsMu.Lock()
	defer bbsMu.Unlock()
	st, err := s.loadBBS()
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"enabled": true, "mode": "builtin", "base_url": builtinURL, "board_key": st.BoardKey, "builtin_base_url": builtinURL, "posts": len(st.Posts), "path": s.bbsPath(), "readme": builtinURL + "/readme?key=" + st.BoardKey})
}

func (s *Server) bbsConfigHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := s.loadBBSConfig()
		if err != nil {
			bad(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"mode": cfg.Mode, "base_url": cfg.BaseURL, "board_key": cfg.BoardKey, "builtin_base_url": s.builtinBBSBaseURL()})
	case http.MethodPost:
		if !requireDangerousHeader(w, r) {
			return
		}
		var cfg bbsConfig
		if err := decode(r, &cfg); err != nil {
			bad(w, 400, err.Error())
			return
		}
		cfg, err := validateBBSConfig(cfg)
		if err != nil {
			bad(w, 400, err.Error())
			return
		}
		if err := s.saveBBSConfig(cfg); err != nil {
			bad(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"mode": cfg.Mode, "base_url": cfg.BaseURL, "board_key": cfg.BoardKey, "builtin_base_url": s.builtinBBSBaseURL()})
	default:
		bad(w, 405, "method not allowed")
	}
}

func (s *Server) proxyExternalBBS(w http.ResponseWriter, r *http.Request, endpoint string) bool {
	cfg, err := s.loadBBSConfig()
	if err != nil {
		bad(w, 500, err.Error())
		return true
	}
	if cfg.Mode != "external" {
		return false
	}
	if cfg.BaseURL == "" {
		bad(w, 400, "external base_url is empty")
		return true
	}
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		bad(w, 400, err.Error())
		return true
	}
	u := base.ResolveReference(&url.URL{Path: strings.TrimRight(base.Path, "/") + endpoint})
	q := r.URL.Query()
	if q.Get("key") == "" && cfg.BoardKey != "" {
		q.Set("key", cfg.BoardKey)
	}
	u.RawQuery = q.Encode()
	var body io.Reader
	if r.Body != nil {
		data, err := io.ReadAll(io.LimitReader(r.Body, maxBBSProxyBodyBytes+1))
		if err != nil {
			bad(w, 400, err.Error())
			return true
		}
		if len(data) > maxBBSProxyBodyBytes {
			bad(w, http.StatusRequestEntityTooLarge, "BBS proxy request body too large")
			return true
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, u.String(), body)
	if err != nil {
		bad(w, 500, err.Error())
		return true
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.BoardKey != "" {
		req.Header.Set("X-API-Key", cfg.BoardKey)
	}
	resp, err := bbsProxyHTTPClient.Do(req)
	if err != nil {
		bad(w, 502, err.Error())
		return true
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBBSProxyBodyBytes+1))
	closeErr := resp.Body.Close()
	if err != nil {
		bad(w, 502, err.Error())
		return true
	}
	if closeErr != nil {
		bad(w, 502, fmt.Errorf("close upstream response body: %w", closeErr).Error())
		return true
	}
	if len(data) > maxBBSProxyBodyBytes {
		bad(w, http.StatusBadGateway, "BBS proxy response body too large")
		return true
	}
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(data); err != nil {
		return true
	}
	return true
}

func (s *Server) fetchExternalBBSPosts(ctx context.Context, cfg bbsConfig, limit int) (posts []bbsPost, err error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("external base_url is empty")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	u = u.ResolveReference(&url.URL{Path: strings.TrimRight(u.Path, "/") + "/posts"})
	q := u.Query()
	q.Set("limit", strconv.Itoa(limit))
	if cfg.BoardKey != "" {
		q.Set("key", cfg.BoardKey)
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create external BBS request: %w", err)
	}
	if cfg.BoardKey != "" {
		req.Header.Set("X-API-Key", cfg.BoardKey)
	}
	resp, err := bbsStatusHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close external BBS response body: %w", closeErr)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 512))
		if readErr != nil {
			return nil, fmt.Errorf("read external BBS error response: %w", readErr)
		}
		return nil, fmt.Errorf("external BBS returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	limited := http.MaxBytesReader(nil, resp.Body, maxBBSStatusBodyBytes)
	dec := json.NewDecoder(limited)
	if err := dec.Decode(&posts); err != nil {
		return nil, err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("external BBS response must contain a single JSON value")
		}
		return nil, err
	}
	return posts, nil
}

func (s *Server) bbsPosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	if r.Method == http.MethodPost && !requireDangerousHeader(w, r) {
		return
	}
	if !s.proxyExternalBBS(w, r, "/posts") {
		s.bbsPostsCore(w, r, true)
	}
}

func (s *Server) bbsPostsCompat(w http.ResponseWriter, r *http.Request) { s.bbsPostsCore(w, r, false) }

func (s *Server) bbsPostsCore(w http.ResponseWriter, r *http.Request, admin bool) {
	bbsMu.Lock()
	defer bbsMu.Unlock()
	st, err := s.loadBBS()
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	if !admin && !bbsAllowed(st, r) {
		bad(w, 403, "invalid board key")
		return
	}
	if r.Method == http.MethodGet {
		limit := 50
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				bad(w, 400, "limit must be a positive integer")
				return
			}
			if parsed > 200 {
				bad(w, 400, "limit must be <= 200")
				return
			}
			limit = parsed
		}
		posts := append([]bbsPost(nil), st.Posts...)
		sort.Slice(posts, func(i, j int) bool { return posts[i].ID > posts[j].ID })
		if len(posts) > limit {
			posts = posts[:limit]
		}
		writeJSON(w, posts)
		return
	}
	if r.Method == http.MethodPost {
		var p struct {
			Title, Content, Author string
			Tags                   []string `json:"tags"`
		}
		if err := decode(r, &p); err != nil {
			bad(w, 400, err.Error())
			return
		}
		if strings.TrimSpace(p.Title) == "" || strings.TrimSpace(p.Content) == "" {
			bad(w, 400, "title and content required")
			return
		}
		now := time.Now().Format(time.RFC3339)
		post := bbsPost{ID: st.NextID, Title: strings.TrimSpace(p.Title), Content: p.Content, Author: strings.TrimSpace(p.Author), Tags: p.Tags, CreatedAt: now, UpdatedAt: now, Replies: []bbsReply{}}
		if post.Author == "" {
			post.Author = "admin"
		}
		st.NextID++
		st.Posts = append(st.Posts, post)
		if err := s.saveBBS(st); err != nil {
			bad(w, 500, err.Error())
			return
		}
		writeJSON(w, post)
		return
	}
	bad(w, 405, "method not allowed")
}

func (s *Server) bbsPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	if !s.proxyExternalBBS(w, r, "/post") {
		s.bbsPostCore(w, r, true)
	}
}

func (s *Server) bbsPostCompat(w http.ResponseWriter, r *http.Request) { s.bbsPostCore(w, r, false) }

func (s *Server) bbsPostCore(w http.ResponseWriter, r *http.Request, admin bool) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	bbsMu.Lock()
	defer bbsMu.Unlock()
	st, err := s.loadBBS()
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	if !admin && !bbsAllowed(st, r) {
		bad(w, 403, "invalid board key")
		return
	}
	rawID := strings.TrimSpace(r.URL.Query().Get("id"))
	if rawID == "" {
		bad(w, 400, "id required")
		return
	}
	id, err := strconv.Atoi(rawID)
	if err != nil || id <= 0 {
		bad(w, 400, "id must be a positive integer")
		return
	}
	for _, p := range st.Posts {
		if p.ID == id {
			writeJSON(w, p)
			return
		}
	}
	bad(w, 404, "post not found")
}

func (s *Server) bbsReply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	if !s.proxyExternalBBS(w, r, "/reply") {
		s.bbsReplyCore(w, r, true)
	}
}

func (s *Server) bbsReplyCompat(w http.ResponseWriter, r *http.Request) { s.bbsReplyCore(w, r, false) }

func (s *Server) bbsReplyCore(w http.ResponseWriter, r *http.Request, admin bool) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	bbsMu.Lock()
	defer bbsMu.Unlock()
	st, err := s.loadBBS()
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	if !admin && !bbsAllowed(st, r) {
		bad(w, 403, "invalid board key")
		return
	}
	var req struct {
		PostID  int    `json:"post_id"`
		ID      int    `json:"id"`
		Author  string `json:"author"`
		Content string `json:"content"`
	}
	if err := decode(r, &req); err != nil {
		bad(w, 400, err.Error())
		return
	}
	if req.PostID == 0 {
		req.PostID = req.ID
	}
	if req.PostID <= 0 || strings.TrimSpace(req.Content) == "" {
		bad(w, 400, "post_id and content required")
		return
	}
	for i := range st.Posts {
		if st.Posts[i].ID == req.PostID {
			if strings.TrimSpace(req.Author) == "" {
				req.Author = "agent"
			}
			rep := bbsReply{ID: st.NextReply, Author: strings.TrimSpace(req.Author), Content: req.Content, CreatedAt: time.Now().Format(time.RFC3339)}
			st.NextReply++
			st.Posts[i].Replies = append(st.Posts[i].Replies, rep)
			st.Posts[i].UpdatedAt = rep.CreatedAt
			if err := s.saveBBS(st); err != nil {
				bad(w, 500, err.Error())
				return
			}
			writeJSON(w, rep)
			return
		}
	}
	bad(w, 404, "post not found")
}

func (s *Server) bbsReadme(w http.ResponseWriter, r *http.Request) { s.bbsReadmeCore(w, r, true) }

func (s *Server) bbsReadmeCompat(w http.ResponseWriter, r *http.Request) {
	s.bbsReadmeCore(w, r, false)
}

func (s *Server) bbsReadmeCore(w http.ResponseWriter, r *http.Request, admin bool) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	bbsMu.Lock()
	st, err := s.loadBBS()
	bbsMu.Unlock()
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	if !admin && !bbsAllowed(st, r) {
		bad(w, 403, "invalid board key")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("GenericAgent Admin Built-in BBS\n\nGET /posts?limit=10&key=BOARD_KEY  list newest posts\nGET /post?id=1&key=BOARD_KEY       read one post with replies\nPOST /reply?key=BOARD_KEY           JSON {post_id, author, content}\nPOST /posts?key=BOARD_KEY           JSON {title, content, author, tags}\n\nGA worker: set reflect/agent_team_setting.json base_url to this Admin URL and board_key to the shown key.\n"))
}
