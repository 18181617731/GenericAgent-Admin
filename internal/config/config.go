package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path(), data, 0644); err != nil {
		return err
	}
	s.Cfg = cfg
	return nil
}
