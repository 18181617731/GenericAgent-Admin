package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type SlashCommandItem struct {
	Cmd     string `json:"cmd"`
	Desc    string `json:"desc"`
	Content string `json:"content,omitempty"`
}

type ExtraSystemPromptPreset struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

type ChatTitleModelRef struct {
	Enable          bool   `json:"enable"`            // false = disabled (default, saves tokens)
	ProviderVarName string `json:"provider_var_name"` // empty = follow conversation model
	Model           string `json:"model"`
	LLMNo           int    `json:"llm_no"`
}

type InstanceConfig struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	GARoot          string `json:"ga_root"`
	PythonPath      string `json:"python_path,omitempty"`
	EffectivePython string `json:"effective_python,omitempty"`
	InitStatus      string `json:"init_status,omitempty"`
	InitError       string `json:"init_error,omitempty"`
}

const (
	InstanceInitStatusInitializing = "initializing"
	InstanceInitStatusReady        = "ready"
	InstanceInitStatusFailed       = "failed"
)

type AppConfig struct {
	GARoot                   string                    `json:"ga_root"`
	ChatDataDir              string                    `json:"chat_data_dir"`
	Host                     string                    `json:"host"`
	Port                     int                       `json:"port"`
	LogTailLines             int                       `json:"log_tail_lines"`
	BufferLines              int                       `json:"buffer_lines"`
	PythonPath               string                    `json:"python_path"`
	EffectivePython          string                    `json:"effective_python,omitempty"`
	DefaultInstanceID        string                    `json:"default_instance_id,omitempty"`
	Instances                []InstanceConfig          `json:"instances,omitempty"`
	BootstrapDone            bool                      `json:"bootstrap_done"`
	ProxyMode                string                    `json:"proxy_mode"` // off | system | custom
	HTTPProxy                string                    `json:"http_proxy"`
	HTTPSProxy               string                    `json:"https_proxy"`
	AllProxy                 string                    `json:"all_proxy"`
	NoProxy                  string                    `json:"no_proxy"`
	ServiceAutostart         []string                  `json:"service_autostart"`
	ServiceModels            map[string]int            `json:"service_models,omitempty"`
	ModelProbeProviders      []string                  `json:"model_probe_providers,omitempty"`
	ChatTitleModel           *ChatTitleModelRef        `json:"chat_title_model,omitempty"`
	UpdateRepoURL            string                    `json:"update_repo_url"`
	SlashCommands            []SlashCommandItem        `json:"slash_commands,omitempty"`
	ExtraSystemPromptPresets []ExtraSystemPromptPreset `json:"extra_system_prompt_presets,omitempty"`
	// ChatDefaultLLMNo is the llm_no seeded into freshly created chat sessions.
	// It tracks the model last picked in Admin Chat so a new conversation keeps
	// using it instead of silently falling back to the first configured model.
	ChatDefaultLLMNo int `json:"chat_default_llm_no,omitempty"`
}

func validInstanceID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for i, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || i > 0 && (r == '-' || r == '_' || r == '.') {
			continue
		}
		return false
	}
	return true
}

func validateRuntimePath(name, path string, wantDir bool) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s does not exist: %w", name, err)
	}
	if wantDir && !st.IsDir() {
		return fmt.Errorf("%s is not a directory", name)
	}
	if !wantDir && st.IsDir() {
		return fmt.Errorf("%s is a directory", name)
	}
	return nil
}

func Validate(cfg AppConfig) error {
	if cfg.Port < 0 || cfg.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535")
	}
	if cfg.LogTailLines < 0 {
		return fmt.Errorf("log_tail_lines must be positive")
	}
	if cfg.BufferLines < 0 {
		return fmt.Errorf("buffer_lines must be positive")
	}
	if cfg.ChatDefaultLLMNo < 0 {
		return fmt.Errorf("chat_default_llm_no must be positive")
	}
	if cfg.ChatTitleModel != nil {
		// llm_no must always be non-negative regardless of the enable flag.
		if cfg.ChatTitleModel.LLMNo < 0 {
			return fmt.Errorf("chat_title_model.llm_no must be non-negative")
		}
		if cfg.ChatTitleModel.Enable {
			// Allow both provider_var_name and model to be empty when enabled,
			// which means "follow the current conversation model".
			provEmpty := strings.TrimSpace(cfg.ChatTitleModel.ProviderVarName) == ""
			modelEmpty := strings.TrimSpace(cfg.ChatTitleModel.Model) == ""
			if provEmpty != modelEmpty {
				return fmt.Errorf("chat_title_model: provider_var_name and model must both be set or both be empty")
			}
		}
	}
	if len(cfg.Instances) == 0 {
		if strings.TrimSpace(cfg.DefaultInstanceID) != "" {
			return fmt.Errorf("default_instance_id requires at least one instance")
		}
	} else {
		defaultID := strings.TrimSpace(cfg.DefaultInstanceID)
		if defaultID == "" {
			return fmt.Errorf("default_instance_id is required when instances are configured")
		}
		seenIDs := make(map[string]struct{}, len(cfg.Instances))
		seenNames := make(map[string]struct{}, len(cfg.Instances))
		seenRoots := make([]string, 0, len(cfg.Instances))
		defaultFound := false
		for i, instance := range cfg.Instances {
			prefix := fmt.Sprintf("instances[%d]", i)
			id := strings.TrimSpace(instance.ID)
			if !validInstanceID(id) {
				return fmt.Errorf("%s.id must be 1-64 characters and contain only letters, numbers, '.', '_' or '-'", prefix)
			}
			if _, ok := seenIDs[id]; ok {
				return fmt.Errorf("duplicate instance id %q", id)
			}
			seenIDs[id] = struct{}{}
			name := strings.TrimSpace(instance.Name)
			if name == "" || len([]rune(name)) > 100 {
				return fmt.Errorf("%s.name must be 1-100 characters", prefix)
			}
			nameKey := strings.ToLower(name)
			if _, ok := seenNames[nameKey]; ok {
				return fmt.Errorf("duplicate instance name %q", name)
			}
			seenNames[nameKey] = struct{}{}
			status := strings.ToLower(strings.TrimSpace(instance.InitStatus))
			switch status {
			case "", InstanceInitStatusInitializing, InstanceInitStatusReady, InstanceInitStatusFailed:
			default:
				return fmt.Errorf("%s.init_status must be %q, %q or %q", prefix, InstanceInitStatusInitializing, InstanceInitStatusReady, InstanceInitStatusFailed)
			}
			root := strings.TrimSpace(instance.GARoot)
			if root == "" {
				return fmt.Errorf("%s.ga_root is required", prefix)
			}
			if err := validateRuntimePath(prefix+".ga_root", root, true); err != nil {
				return err
			}
			for _, otherRoot := range seenRoots {
				if samePath(root, otherRoot) {
					return fmt.Errorf("duplicate instance ga_root %q", root)
				}
			}
			seenRoots = append(seenRoots, root)
			if err := validateRuntimePath(prefix+".python_path", instance.PythonPath, false); err != nil {
				return err
			}
			if err := validateRuntimePath(prefix+".effective_python", instance.EffectivePython, false); err != nil {
				return err
			}
			if id == defaultID {
				defaultFound = true
			}
		}
		if !defaultFound {
			return fmt.Errorf("default_instance_id %q does not reference a configured instance", defaultID)
		}
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
	if py := strings.TrimSpace(cfg.EffectivePython); py != "" {
		st, err := os.Stat(py)
		if err != nil {
			return fmt.Errorf("effective_python does not exist: %w", err)
		}
		if st.IsDir() {
			return fmt.Errorf("effective_python is a directory")
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
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		return filepath.Join(cwd, "data")
	}
	return "data"
}

func defaultChatDataDir(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return DefaultChatDataDir()
	}
	return filepath.Join(root, "data")
}

func defaultForRoot(root string) AppConfig {
	cfg := Default()
	cfg.ChatDataDir = defaultChatDataDir(root)
	return cfg
}

func Default() AppConfig {
	return AppConfig{
		GARoot:       "",
		ChatDataDir:  DefaultChatDataDir(),
		Host:         "127.0.0.1",
		Port:         8787,
		LogTailLines: 200,
		BufferLines:  1000,
		ProxyMode:    "off",
		SlashCommands: []SlashCommandItem{
			{Cmd: "/update", Desc: "git pull 更新 GA 仓库并报告影响面"},
			{Cmd: "/autorun", Desc: "进入 autonomous_operation 自主模式"},
			{Cmd: "/morphling", Desc: "启用 Morphling 蒸馏 / 吞噬外部技能"},
			{Cmd: "/goal", Desc: "进入 Goal 模式（需 condition 约束）"},
			{Cmd: "/hive", Desc: "进入 Hive 多 worker 协作模式"},
			{Cmd: "/conductor", Desc: "调用 frontends/conductor.py 多 subagent 编排"},
			{Cmd: "/continue", Desc: "Chat 历史会话搜索"},
			{Cmd: "/review", Desc: "Chat 内代码审查"},
		},
	}
}

type Store struct {
	Root string

	mu  sync.RWMutex
	cfg AppConfig
}

func NewStore(root string) *Store {
	s := &Store{Root: root, cfg: defaultForRoot(root)}
	_ = s.Load()
	return s
}

// NewRuntimeStore creates an in-memory store from cfg without reading from or
// writing to disk. It is useful for request-scoped derived configurations.
func NewRuntimeStore(root string, cfg AppConfig) (*Store, error) {
	cfg = normalize(cloneAppConfig(cfg), root)
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	return &Store{Root: root, cfg: cloneAppConfig(cfg)}, nil
}

// Snapshot returns a deep copy of the currently published configuration.
// Callers may freely mutate the returned value without aliasing Store state.
func (s *Store) Snapshot() AppConfig {
	if s == nil {
		return AppConfig{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAppConfig(s.cfg)
}

// UpdateRuntime atomically updates the in-memory configuration without
// persisting it. It is intended for process-local overrides such as CLI flags.
func (s *Store) UpdateRuntime(update func(*AppConfig)) error {
	if s == nil {
		return fmt.Errorf("config store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := cloneAppConfig(s.cfg)
	if update != nil {
		update(&cfg)
	}
	cfg = normalize(cfg, s.Root)
	if err := Validate(cfg); err != nil {
		return err
	}
	s.cfg = cloneAppConfig(cfg)
	return nil
}

func cloneAppConfig(cfg AppConfig) AppConfig {
	if cfg.Instances != nil {
		cloned := make([]InstanceConfig, len(cfg.Instances))
		copy(cloned, cfg.Instances)
		cfg.Instances = cloned
	}
	if cfg.ServiceAutostart != nil {
		cloned := make([]string, len(cfg.ServiceAutostart))
		copy(cloned, cfg.ServiceAutostart)
		cfg.ServiceAutostart = cloned
	}
	if cfg.ServiceModels != nil {
		cloned := make(map[string]int, len(cfg.ServiceModels))
		for name, llmNo := range cfg.ServiceModels {
			cloned[name] = llmNo
		}
		cfg.ServiceModels = cloned
	}
	if cfg.ChatTitleModel != nil {
		cloned := *cfg.ChatTitleModel
		cfg.ChatTitleModel = &cloned
	}
	if cfg.SlashCommands != nil {
		cloned := make([]SlashCommandItem, len(cfg.SlashCommands))
		copy(cloned, cfg.SlashCommands)
		cfg.SlashCommands = cloned
	}
	if cfg.ExtraSystemPromptPresets != nil {
		cloned := make([]ExtraSystemPromptPreset, len(cfg.ExtraSystemPromptPresets))
		copy(cloned, cfg.ExtraSystemPromptPresets)
		cfg.ExtraSystemPromptPresets = cloned
	}
	return cfg
}

func (s *Store) path() string { return filepath.Join(s.Root, "config.local.json") }

func effectivePython(cfg AppConfig) string {
	if py := strings.TrimSpace(cfg.PythonPath); py != "" {
		return py
	}
	return strings.TrimSpace(cfg.EffectivePython)
}

func effectiveInstancePython(instance InstanceConfig) string {
	if py := strings.TrimSpace(instance.PythonPath); py != "" {
		return py
	}
	return strings.TrimSpace(instance.EffectivePython)
}

func normalize(cfg AppConfig, root string) AppConfig {
	if strings.TrimSpace(cfg.ChatDataDir) == "" {
		cfg.ChatDataDir = defaultChatDataDir(root)
	}
	if strings.TrimSpace(cfg.Host) == "" {
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
	if strings.TrimSpace(cfg.ProxyMode) == "" {
		cfg.ProxyMode = "off"
	}

	cfg.GARoot = strings.TrimSpace(cfg.GARoot)
	cfg.PythonPath = strings.TrimSpace(cfg.PythonPath)
	cfg.EffectivePython = effectivePython(cfg)
	cfg.DefaultInstanceID = strings.TrimSpace(cfg.DefaultInstanceID)

	instances := make([]InstanceConfig, 0, len(cfg.Instances)+1)
	for _, instance := range cfg.Instances {
		instance.ID = strings.TrimSpace(instance.ID)
		instance.Name = strings.TrimSpace(instance.Name)
		instance.GARoot = strings.TrimSpace(instance.GARoot)
		instance.PythonPath = strings.TrimSpace(instance.PythonPath)
		instance.EffectivePython = effectiveInstancePython(instance)
		instance.InitStatus = strings.ToLower(strings.TrimSpace(instance.InitStatus))
		instance.InitError = strings.TrimSpace(instance.InitError)
		instances = append(instances, instance)
	}
	if len(instances) == 0 && cfg.GARoot != "" {
		instances = append(instances, InstanceConfig{
			ID:              "default",
			Name:            "Default",
			GARoot:          cfg.GARoot,
			PythonPath:      cfg.PythonPath,
			EffectivePython: cfg.EffectivePython,
		})
		cfg.DefaultInstanceID = "default"
	}
	cfg.Instances = instances

	if len(cfg.Instances) == 0 {
		cfg.DefaultInstanceID = ""
		return cfg
	}
	if cfg.DefaultInstanceID == "" {
		for _, instance := range cfg.Instances {
			if samePath(instance.GARoot, cfg.GARoot) && cfg.GARoot != "" {
				cfg.DefaultInstanceID = instance.ID
				break
			}
		}
		if cfg.DefaultInstanceID == "" {
			cfg.DefaultInstanceID = cfg.Instances[0].ID
		}
	}
	for _, instance := range cfg.Instances {
		if instance.ID != cfg.DefaultInstanceID {
			continue
		}
		// Keep the legacy single-instance fields as a compatibility mirror.
		cfg.GARoot = instance.GARoot
		cfg.PythonPath = instance.PythonPath
		cfg.EffectivePython = instance.EffectivePython
		break
	}
	return cfg
}

func samePath(a, b string) bool {
	a = filepath.Clean(strings.TrimSpace(a))
	b = filepath.Clean(strings.TrimSpace(b))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func (cfg *AppConfig) SyncDefaultInstanceFromLegacy() bool {
	if cfg == nil {
		return false
	}
	defaultID := strings.TrimSpace(cfg.DefaultInstanceID)
	for i := range cfg.Instances {
		if strings.TrimSpace(cfg.Instances[i].ID) != defaultID {
			continue
		}
		cfg.Instances[i].GARoot = strings.TrimSpace(cfg.GARoot)
		cfg.Instances[i].PythonPath = strings.TrimSpace(cfg.PythonPath)
		cfg.Instances[i].EffectivePython = effectivePython(*cfg)
		return true
	}
	return false
}

func (cfg AppConfig) Instance(id string) (InstanceConfig, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		id = strings.TrimSpace(cfg.DefaultInstanceID)
	}
	for _, instance := range cfg.Instances {
		if instance.ID == id {
			return instance, true
		}
	}
	return InstanceConfig{}, false
}

func (s *Store) Load() error {
	if s == nil {
		return fmt.Errorf("config store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path())
	if err != nil {
		return nil
	}
	cfg := defaultForRoot(s.Root)
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
	clearMissingPythonPaths(&cfg)
	cfg.ModelProbeProviders = normalizeUniqueStrings(cfg.ModelProbeProviders)
	cfg.EffectivePython = effectivePython(cfg)
	if err := Validate(cfg); err != nil {
		return err
	}
	s.cfg = cloneAppConfig(cfg)
	return nil
}

func clearMissingPythonPaths(cfg *AppConfig) {
	if cfg == nil {
		return
	}
	if pathMissing(cfg.PythonPath) {
		cfg.PythonPath = ""
	}
	if pathMissing(cfg.EffectivePython) {
		cfg.EffectivePython = ""
	}
}

func pathMissing(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

func (s *Store) Save(cfg AppConfig) error {
	if s == nil {
		return fmt.Errorf("config store is nil")
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
	cfg.ModelProbeProviders = normalizeUniqueStrings(cfg.ModelProbeProviders)
	cfg.EffectivePython = effectivePython(cfg)
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
	s.cfg = cloneAppConfig(cfg)
	return nil
}

func normalizeUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
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
