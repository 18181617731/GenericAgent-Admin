package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type AppConfig struct {
	GARoot           string   `json:"ga_root"`
	ChatDataDir      string   `json:"chat_data_dir"`
	Host             string   `json:"host"`
	Port             int      `json:"port"`
	LogTailLines     int      `json:"log_tail_lines"`
	BufferLines      int      `json:"buffer_lines"`
	PythonPath       string   `json:"python_path"`
	ProxyMode        string   `json:"proxy_mode"` // off | system | custom
	HTTPProxy        string   `json:"http_proxy"`
	HTTPSProxy       string   `json:"https_proxy"`
	AllProxy         string   `json:"all_proxy"`
	NoProxy          string   `json:"no_proxy"`
	ServiceAutostart []string `json:"service_autostart"`
}

func Validate(cfg AppConfig) error {
	if cfg.Port < 0 {
		return fmt.Errorf("port must be positive")
	}
	if cfg.LogTailLines < 0 {
		return fmt.Errorf("log_tail_lines must be positive")
	}
	if cfg.BufferLines < 0 {
		return fmt.Errorf("buffer_lines must be positive")
	}
	if root := strings.TrimSpace(cfg.GARoot); root != "" {
		st, err := os.Stat(root)
		if err != nil {
			return fmt.Errorf("ga_root does not exist: %w", err)
		}
		if !st.IsDir() {
			return fmt.Errorf("ga_root is not a directory")
		}
	}
	if chatDir := strings.TrimSpace(cfg.ChatDataDir); chatDir != "" {
		if st, err := os.Stat(chatDir); err == nil && !st.IsDir() {
			return fmt.Errorf("chat_data_dir is not a directory")
		}
	}
	if py := strings.TrimSpace(cfg.PythonPath); py != "" {
		st, err := os.Stat(py)
		if err != nil {
			return fmt.Errorf("python_path does not exist: %w", err)
		}
		if st.IsDir() {
			return fmt.Errorf("python_path is a directory")
		}
	}
	switch strings.TrimSpace(cfg.ProxyMode) {
	case "", "off", "system":
	case "custom":
		for name, value := range map[string]string{"http_proxy": cfg.HTTPProxy, "https_proxy": cfg.HTTPSProxy, "all_proxy": cfg.AllProxy} {
			if err := validateProxyURL(name, value); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("proxy_mode must be off, system, or custom")
	}
	return nil
}

func validateProxyURL(name, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%s must be a valid proxy URL", name)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "socks5", "socks5h":
		return nil
	default:
		return fmt.Errorf("%s has unsupported proxy scheme %q", name, u.Scheme)
	}
}

func DefaultChatDataDir() string {
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "GenericAgent-Admin")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".genericagent-admin")
	}
	return "GenericAgent-Admin"
}

func Default() AppConfig {
	return AppConfig{GARoot: "E:/Work/GenericAgent", ChatDataDir: DefaultChatDataDir(), Host: "127.0.0.1", Port: 8787, LogTailLines: 200, BufferLines: 1000, ProxyMode: "off"}
}

type Store struct {
	Root string
	Cfg  AppConfig
}

func NewStore(root string) *Store {
	s := &Store{Root: root, Cfg: Default()}
	_ = s.Load()
	return s
}

func (s *Store) path() string { return filepath.Join(s.Root, "config.local.json") }

func (s *Store) Load() error {
	data, err := os.ReadFile(s.path())
	if err != nil {
		return nil
	}
	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	if cfg.ChatDataDir == "" {
		cfg.ChatDataDir = DefaultChatDataDir()
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 8787
	}
	if cfg.LogTailLines == 0 {
		cfg.LogTailLines = 200
	}
	if cfg.BufferLines == 0 {
		cfg.BufferLines = 1000
	}
	if cfg.ProxyMode == "" {
		cfg.ProxyMode = "off"
	}
	s.Cfg = cfg
	return nil
}

func (s *Store) Save(cfg AppConfig) error {
	if strings.TrimSpace(cfg.ChatDataDir) == "" {
		cfg.ChatDataDir = DefaultChatDataDir()
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 8787
	}
	if cfg.LogTailLines == 0 {
		cfg.LogTailLines = 200
	}
	if cfg.BufferLines == 0 {
		cfg.BufferLines = 1000
	}
	if cfg.ProxyMode == "" {
		cfg.ProxyMode = "off"
	}
	if err := Validate(cfg); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(s.path(), data, 0644); err != nil {
		return err
	}
	s.Cfg = cfg
	return nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
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
	if err = os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}
