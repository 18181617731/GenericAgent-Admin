package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/modelconfig"
)

// applyProjectConfigFromInstance projects all state that belongs to the GA
// project into the legacy AppConfig fields consumed by existing handlers and
// the settings UI. The fields are copied rather than aliased because this
// config is often used as a request-local runtime view.
func applyProjectConfigFromInstance(cfg *config.AppConfig, instance config.InstanceConfig) {
	if cfg == nil {
		return
	}
	cfg.GARoot = strings.TrimSpace(instance.GARoot)
	cfg.ChatDataDir = strings.TrimSpace(instance.ChatDataDir)
	cfg.PythonPath = strings.TrimSpace(instance.PythonPath)
	cfg.EffectivePython = strings.TrimSpace(instance.EffectivePython)
	cfg.BootstrapDone = instance.BootstrapDone
	cfg.ServiceAutostart = append([]string(nil), instance.ServiceAutostart...)
	if instance.ServiceModels == nil {
		cfg.ServiceModels = nil
	} else {
		cfg.ServiceModels = make(map[string]int, len(instance.ServiceModels))
		for name, llmNo := range instance.ServiceModels {
			cfg.ServiceModels[name] = llmNo
		}
	}
	cfg.ModelProbeProviders = append([]string(nil), instance.ModelProbeProviders...)
	if instance.ChatTitleModel == nil {
		cfg.ChatTitleModel = nil
	} else {
		model := *instance.ChatTitleModel
		cfg.ChatTitleModel = &model
	}
	cfg.ChatDefaultLLMNo = instance.ChatDefaultLLMNo
	cfg.SlashCommands = append([]config.SlashCommandItem(nil), instance.SlashCommands...)
	cfg.ExtraSystemPromptPresets = append([]config.ExtraSystemPromptPreset(nil), instance.ExtraSystemPromptPresets...)
}

// applyProjectConfigToInstance performs the inverse projection used when a
// legacy settings payload is saved for a selected instance.
func applyProjectConfigToInstance(instance *config.InstanceConfig, cfg config.AppConfig) {
	if instance == nil {
		return
	}
	instance.GARoot = strings.TrimSpace(cfg.GARoot)
	instance.ChatDataDir = strings.TrimSpace(cfg.ChatDataDir)
	instance.PythonPath = strings.TrimSpace(cfg.PythonPath)
	instance.EffectivePython = strings.TrimSpace(cfg.EffectivePython)
	instance.BootstrapDone = cfg.BootstrapDone
	instance.ServiceAutostart = append([]string(nil), cfg.ServiceAutostart...)
	if cfg.ServiceModels == nil {
		instance.ServiceModels = nil
	} else {
		instance.ServiceModels = make(map[string]int, len(cfg.ServiceModels))
		for name, llmNo := range cfg.ServiceModels {
			instance.ServiceModels[name] = llmNo
		}
	}
	instance.ModelProbeProviders = append([]string(nil), cfg.ModelProbeProviders...)
	if cfg.ChatTitleModel == nil {
		instance.ChatTitleModel = nil
	} else {
		model := *cfg.ChatTitleModel
		instance.ChatTitleModel = &model
	}
	instance.ChatDefaultLLMNo = cfg.ChatDefaultLLMNo
	instance.SlashCommands = append([]config.SlashCommandItem(nil), cfg.SlashCommands...)
	instance.ExtraSystemPromptPresets = append([]config.ExtraSystemPromptPreset(nil), cfg.ExtraSystemPromptPresets...)
}

func mirrorDefaultProjectConfig(cfg *config.AppConfig) {
	if cfg == nil || len(cfg.Instances) == 0 {
		return
	}
	for _, instance := range cfg.Instances {
		if instance.ID == cfg.DefaultInstanceID {
			applyProjectConfigFromInstance(cfg, instance)
			return
		}
	}
}

func instanceChatDataDir(base config.AppConfig, instance config.InstanceConfig) string {
	if chatDir := strings.TrimSpace(instance.ChatDataDir); chatDir != "" {
		return chatDir
	}
	if instance.ID == protectedDefaultInstanceID {
		return strings.TrimSpace(base.ChatDataDir)
	}
	if chatDir := strings.TrimSpace(base.ChatDataDir); chatDir != "" {
		return filepath.Join(chatDir, "instances", instance.ID)
	}
	return ""
}

// scopedAppConfig creates the request-local view of an instance. The base
// config remains the source of truth for the instance registry and Admin-wide
// settings, while handlers that operate on a GA project see the selected
// root as their own cfg.GARoot.
func scopedAppConfig(base config.AppConfig, instance config.InstanceConfig) config.AppConfig {
	cfg := base
	applyProjectConfigFromInstance(&cfg, instance)
	// A deleted virtualenv should make scoped read endpoints report the GA
	// root's current state instead of making the whole request fail validation.
	// Runtime repair uses the selected instance's projected config and persists
	// the discovered interpreter through the same instance-aware setup store.
	if !usableConfiguredPython(cfg.PythonPath) {
		cfg.PythonPath = ""
	}
	if !usableConfiguredPython(cfg.EffectivePython) {
		cfg.EffectivePython = ""
	}
	cfg.DefaultInstanceID = strings.TrimSpace(instance.ID)
	cfg.Instances = []config.InstanceConfig{instance}
	// The migrated legacy instance keeps the original chat data location. New
	// instances default to their own directory, while an explicitly configured
	// per-instance path remains authoritative.
	cfg.ChatDataDir = instanceChatDataDir(base, instance)
	return cfg
}

func usableConfiguredPython(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return true
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (s *Server) baseConfigStore() *config.Store {
	if s == nil {
		return nil
	}
	if s.BaseCfgStore != nil {
		// A few legacy tests and embedders replace CfgStore on a Server after
		// construction. A request-local store keeps the same root as its base;
		// a different root is an intentional replacement and should win.
		if s.CfgStore != nil && s.BaseCfgStore != s.CfgStore &&
			!sameRuntimePath(s.BaseCfgStore.Root, s.CfgStore.Root) {
			return s.CfgStore
		}
		return s.BaseCfgStore
	}
	return s.CfgStore
}

func (s *Server) instanceForRequest(r *http.Request) (config.InstanceConfig, string, error) {
	store := s.baseConfigStore()
	if store == nil {
		return config.InstanceConfig{}, "", errors.New("config store is not initialized")
	}
	cfg := store.Snapshot()
	requested := requestedInstanceID(r)
	instance, ok := cfg.Instance(requested)
	if !ok {
		// Preserve the legacy single-instance setup used before the instance
		// registry existed. A non-empty selection is never silently ignored.
		if requested == "" && len(cfg.Instances) == 0 {
			return config.InstanceConfig{}, "", nil
		}
		return config.InstanceConfig{}, requested, &chatInstanceNotFoundError{instanceID: requested}
	}
	return instance, instance.ID, nil
}

// instanceServerForRequest returns a request-local Server whose GA root,
// interpreter, model store, and service manager all belong to one instance.
// It deliberately does not mutate the process-wide config store.
func (s *Server) instanceServerForRequest(r *http.Request) (*Server, string, error) {
	instance, instanceID, err := s.instanceForRequest(r)
	if err != nil {
		return nil, instanceID, err
	}
	if instanceID == "" {
		return s, "", nil
	}
	baseStore := s.baseConfigStore()
	base := baseStore.Snapshot()
	manager, resolvedID, err := s.serviceManagerForRequest(r)
	if err != nil {
		return nil, resolvedID, err
	}
	cfg := scopedAppConfig(base, instance)
	cfg.PythonFallbackRoots = pythonFallbackRoots(base, instance.ID)
	instanceStore, err := config.NewRuntimeStore(baseStore.Root, cfg)
	if err != nil {
		return nil, instanceID, fmt.Errorf("prepare runtime config for instance %q: %w", instanceID, err)
	}
	// Reuse the existing request-local clone so chat state and other mutexes
	// remain shared only where their instance registry says they should be.
	clone := s.chatRequestServer(instanceStore, baseStore, nil)
	clone.Svc = manager
	clone.Models = modelconfig.NewStore(instance.GARoot)
	return clone, resolvedID, nil
}

func (s *Server) withInstance(next func(*Server, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceServer, instanceID, err := s.instanceServerForRequest(r)
		if err != nil {
			status := http.StatusInternalServerError
			var notFound *chatInstanceNotFoundError
			if errors.As(err, &notFound) || strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			bad(w, status, err.Error())
			return
		}
		setResolvedInstanceHeader(w, instanceID)
		next(instanceServer, w, r)
	}
}
