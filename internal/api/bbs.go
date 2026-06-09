package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
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
	cfg, err := validateBBSConfig(c)
	if err != nil {
		return bbsConfig{Mode: "builtin", BoardKey: "ga-team"}
	}
	return cfg
}

func validateBBSConfig(c bbsConfig) (bbsConfig, error) {
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	if c.Mode == "" {
		c.Mode = "builtin"
	}
	if c.Mode != "builtin" && c.Mode != "external" {
		return c, fmt.Errorf("invalid bbs mode %q", c.Mode)
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.BoardKey = strings.TrimSpace(c.BoardKey)
	if c.BoardKey == "" {
		c.BoardKey = "ga-team"
	}
	if strings.ContainsFunc(c.BoardKey, unicode.IsSpace) {
		return c, errors.New("board_key must not contain whitespace")
	}
	if c.Mode == "external" {
		if c.BaseURL == "" {
			return c, errors.New("base_url required for external bbs")
		}
		u, err := url.Parse(c.BaseURL)
		if err != nil || u.Scheme == "" {
			return c, errors.New("base_url must be an absolute http(s) URL")
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return c, errors.New("base_url scheme must be http or https")
		}
		if u.Host == "" {
			return c, errors.New("base_url must be an absolute http(s) URL")
		}
		if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return c, errors.New("base_url must not include userinfo, query, or fragment")
		}
		if isLocalOrPrivateBBSHost(u.Hostname()) {
			return c, errors.New("base_url host must not be localhost or a private IP")
		}
	} else {
		c.BaseURL = ""
	}
	return c, nil
}

func isLocalOrPrivateBBSHost(host string) bool {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	addr, err := netip.ParseAddr(h)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast()
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
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeBBSFileAtomic(s.bbsConfigPath(), data, 0644)
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
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeBBSFileAtomic(s.bbsPath(), data, 0644)
}

func writeBBSFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
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
