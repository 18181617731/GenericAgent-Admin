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

// scopedAppConfig creates the request-local view of an instance. The base
// config remains the source of truth for the instance registry and Admin-wide
// settings, while handlers that operate on a GA project see the selected
// root as their own cfg.GARoot.
func scopedAppConfig(base config.AppConfig, instance config.InstanceConfig) config.AppConfig {
	cfg := base
	cfg.GARoot = strings.TrimSpace(instance.GARoot)
	cfg.PythonPath = strings.TrimSpace(instance.PythonPath)
	cfg.EffectivePython = strings.TrimSpace(instance.EffectivePython)
	// A deleted virtualenv should make scoped read endpoints report the GA
	// root's current state instead of making the whole request fail validation.
	// Runtime repair remains unscoped because it owns the persistence of a new
	// interpreter path.
	if !usableConfiguredPython(cfg.PythonPath) {
		cfg.PythonPath = ""
	}
	if !usableConfiguredPython(cfg.EffectivePython) {
		cfg.EffectivePython = ""
	}
	cfg.DefaultInstanceID = strings.TrimSpace(instance.ID)
	cfg.Instances = []config.InstanceConfig{instance}
	if instance.ID == protectedDefaultInstanceID {
		// The migrated legacy instance keeps the original chat data location.
		// New instances get a separate data directory below.
	} else if strings.TrimSpace(base.ChatDataDir) != "" {
		cfg.ChatDataDir = filepath.Join(base.ChatDataDir, "instances", instance.ID)
	}
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
