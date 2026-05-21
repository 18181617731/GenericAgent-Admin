package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type bbsPost struct {
	ID        int        `json:"id"`
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	Author    string     `json:"author"`
	Tags      []string   `json:"tags,omitempty"`
	CreatedAt string     `json:"created_at"`
	UpdatedAt string     `json:"updated_at"`
	Replies   []bbsReply `json:"replies,omitempty"`
}

type bbsReply struct {
	ID        int    `json:"id"`
	Author    string `json:"author"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type bbsState struct {
	BoardKey  string    `json:"board_key"`
	NextID    int       `json:"next_id"`
	NextReply int       `json:"next_reply_id"`
	Posts     []bbsPost `json:"posts"`
}

type bbsConfig struct {
	Mode     string `json:"mode"`
	BaseURL  string `json:"base_url"`
	BoardKey string `json:"board_key"`
}

var bbsMu sync.Mutex

func (s *Server) bbsDir() string        { return filepath.Join(s.CfgStore.Root, "data") }
func (s *Server) bbsPath() string       { return filepath.Join(s.bbsDir(), "bbs.json") }
func (s *Server) bbsConfigPath() string { return filepath.Join(s.bbsDir(), "bbs_config.json") }

func normalizeBBSConfig(c bbsConfig) bbsConfig {
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	if c.Mode != "external" {
		c.Mode = "builtin"
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.BoardKey = strings.TrimSpace(c.BoardKey)
	if c.BoardKey == "" {
		c.BoardKey = "ga-team"
	}
	return c
}

func (s *Server) loadBBSConfig() (bbsConfig, error) {
	cfg := normalizeBBSConfig(bbsConfig{Mode: "builtin", BoardKey: "ga-team"})
	data, err := os.ReadFile(s.bbsConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return normalizeBBSConfig(cfg), nil
}

func (s *Server) saveBBSConfig(cfg bbsConfig) error {
	cfg = normalizeBBSConfig(cfg)
	if err := os.MkdirAll(s.bbsDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.bbsConfigPath(), data, 0644)
}

func (s *Server) builtinBBSBaseURL() string {
	host := s.CfgStore.Cfg.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d", host, s.CfgStore.Cfg.Port)
}

func (s *Server) loadBBS() (bbsState, error) {
	st := bbsState{BoardKey: "ga-team", NextID: 1, NextReply: 1, Posts: []bbsPost{}}
	data, err := os.ReadFile(s.bbsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return st, nil
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, err
	}
	if st.BoardKey == "" {
		st.BoardKey = "ga-team"
	}
	if st.NextID <= 0 {
		st.NextID = 1
	}
	if st.NextReply <= 0 {
		st.NextReply = 1
	}
	if st.Posts == nil {
		st.Posts = []bbsPost{}
	}
	for _, p := range st.Posts {
		if p.ID >= st.NextID {
			st.NextID = p.ID + 1
		}
		for _, rp := range p.Replies {
			if rp.ID >= st.NextReply {
				st.NextReply = rp.ID + 1
			}
		}
	}
	return st, nil
}

func (s *Server) saveBBS(st bbsState) error {
	if err := os.MkdirAll(s.bbsDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.bbsPath(), data, 0644)
}

func bbsClientKey(r *http.Request) string {
	if k := r.Header.Get("X-API-Key"); k != "" {
		return k
	}
	return r.URL.Query().Get("key")
}

func bbsAllowed(st bbsState, r *http.Request) bool {
	k := strings.TrimSpace(st.BoardKey)
	return k == "" || bbsClientKey(r) == k
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
		posts, proxyErr := s.fetchExternalBBSPosts(cfg, 1)
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
		var cfg bbsConfig
		if err := decode(r, &cfg); err != nil {
			bad(w, 400, err.Error())
			return
		}
		cfg = normalizeBBSConfig(cfg)
		if cfg.Mode == "external" && cfg.BaseURL == "" {
			bad(w, 400, "external base_url required")
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
		data, _ := io.ReadAll(r.Body)
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(r.Method, u.String(), body)
	if err != nil {
		bad(w, 500, err.Error())
		return true
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.BoardKey != "" {
		req.Header.Set("X-API-Key", cfg.BoardKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		bad(w, 502, err.Error())
		return true
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
	return true
}

func (s *Server) fetchExternalBBSPosts(cfg bbsConfig, limit int) ([]bbsPost, error) {
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
	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	if cfg.BoardKey != "" {
		req.Header.Set("X-API-Key", cfg.BoardKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("external BBS returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var posts []bbsPost
	if err := json.NewDecoder(resp.Body).Decode(&posts); err != nil {
		return nil, err
	}
	return posts, nil
}

func (s *Server) bbsPosts(w http.ResponseWriter, r *http.Request) {
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
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 200 {
			limit = 50
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
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id <= 0 {
		bad(w, 400, "id required")
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
