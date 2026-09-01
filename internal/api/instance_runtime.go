package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/service"
)

type instanceRuntime struct {
	config  config.InstanceConfig
	manager *service.Manager
}

// instanceManagerRegistry keeps one stable service manager per configured
// instance ID. Stable manager identity is important because each manager owns
// its child-process handles, log buffers, and subscribers.
type instanceManagerRegistry struct {
	mu        sync.RWMutex
	entries   map[string]*instanceRuntime
	defaultID string
	fallback  *service.Manager
}

func newInstanceManagerRegistry(cfg config.AppConfig, fallback *service.Manager) *instanceManagerRegistry {
	r := &instanceManagerRegistry{
		entries:  make(map[string]*instanceRuntime, len(cfg.Instances)),
		fallback: fallback,
	}
	for _, instance := range cfg.Instances {
		var manager *service.Manager
		if instance.ID == cfg.DefaultInstanceID && fallback != nil {
			manager = fallback
			manager.SetRoot(instance.GARoot, instance.EffectivePython, cfg.BufferLines)
		} else {
			manager = service.NewManagerWithPython(instance.GARoot, instance.EffectivePython, cfg.BufferLines)
		}
		manager.SetUsageDir(usageEventDir(scopedAppConfig(cfg, instance)))
		r.entries[instance.ID] = &instanceRuntime{config: instance, manager: manager}
	}
	r.defaultID = cfg.DefaultInstanceID
	return r
}

func (r *instanceManagerRegistry) validateTransition(next config.AppConfig) error {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.validateTransitionLocked(next)
}

func (r *instanceManagerRegistry) validateTransitionLocked(next config.AppConfig) error {
	nextByID := make(map[string]config.InstanceConfig, len(next.Instances))
	for _, instance := range next.Instances {
		nextByID[strings.TrimSpace(instance.ID)] = instance
	}
	for id, current := range r.entries {
		desired, exists := nextByID[id]
		if exists && !instanceRuntimeIdentityChanged(current.config, desired) {
			continue
		}
		if current.manager != nil && current.manager.HasRunningProcesses() {
			if !exists {
				return fmt.Errorf("instance %q owns running services and cannot be removed", id)
			}
			return fmt.Errorf("instance %q owns running services and its GA root or Python path cannot be changed", id)
		}
	}
	return nil
}

func (r *instanceManagerRegistry) reconcile(next config.AppConfig) (*service.Manager, error) {
	if r == nil {
		return nil, fmt.Errorf("instance manager registry is not initialized")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.validateTransitionLocked(next); err != nil {
		return nil, err
	}

	nextEntries := make(map[string]*instanceRuntime, len(next.Instances))
	for _, instance := range next.Instances {
		id := strings.TrimSpace(instance.ID)
		if current, ok := r.entries[id]; ok {
			current.manager.SetRoot(instance.GARoot, instance.EffectivePython, next.BufferLines)
			current.manager.SetUsageDir(usageEventDir(scopedAppConfig(next, instance)))
			current.config = instance
			nextEntries[id] = current
			continue
		}
		manager := service.NewManagerWithPython(instance.GARoot, instance.EffectivePython, next.BufferLines)
		manager.SetUsageDir(usageEventDir(scopedAppConfig(next, instance)))
		nextEntries[id] = &instanceRuntime{
			config:  instance,
			manager: manager,
		}
	}
	r.entries = nextEntries
	r.defaultID = strings.TrimSpace(next.DefaultInstanceID)
	if current, ok := r.entries[r.defaultID]; ok {
		return current.manager, nil
	}
	if len(r.entries) == 0 {
		if r.fallback != nil {
			r.fallback.SetRoot(next.GARoot, next.EffectivePython, next.BufferLines)
			r.fallback.SetUsageDir(usageEventDir(next))
		}
		return r.fallback, nil
	}
	return nil, fmt.Errorf("default instance %q has no runtime manager", r.defaultID)
}

func (r *instanceManagerRegistry) manager(instanceID string) (*service.Manager, string, bool) {
	if r == nil {
		return nil, "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id := strings.TrimSpace(instanceID)
	if id == "" {
		id = r.defaultID
	}
	if current, ok := r.entries[id]; ok && current.manager != nil {
		return current.manager, id, true
	}
	if id == "" && len(r.entries) == 0 && r.fallback != nil {
		return r.fallback, "", true
	}
	return nil, id, false
}

func (r *instanceManagerRegistry) managers() []*service.Manager {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[*service.Manager]bool{}
	result := make([]*service.Manager, 0, len(r.entries)+1)
	for _, current := range r.entries {
		if current.manager != nil && !seen[current.manager] {
			seen[current.manager] = true
			result = append(result, current.manager)
		}
	}
	if len(r.entries) == 0 && r.fallback != nil && !seen[r.fallback] {
		result = append(result, r.fallback)
	}
	return result
}

func instanceRuntimeIdentityChanged(current, next config.InstanceConfig) bool {
	return !sameRuntimePath(current.GARoot, next.GARoot) || !sameRuntimePath(current.PythonPath, next.PythonPath)
}

func sameRuntimePath(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		if runtime.GOOS == "windows" {
			return strings.EqualFold(a, b)
		}
		return a == b
	}
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func (s *Server) saveConfigAndReconcile(next config.AppConfig) error {
	store := s.baseConfigStore()
	if store == nil {
		return fmt.Errorf("config store is not initialized")
	}
	if s.InstanceManagers == nil {
		s.InstanceManagers = newInstanceManagerRegistry(store.Snapshot(), s.Svc)
	}
	if err := s.InstanceManagers.validateTransition(next); err != nil {
		return err
	}
	previous := store.Snapshot()
	if err := store.Save(next); err != nil {
		return err
	}
	manager, err := s.InstanceManagers.reconcile(store.Snapshot())
	if err != nil {
		rollbackErr := store.Save(previous)
		if rollbackErr == nil {
			_, _ = s.InstanceManagers.reconcile(store.Snapshot())
		}
		if rollbackErr != nil {
			return fmt.Errorf("apply instance runtime: %v (config rollback failed: %v)", err, rollbackErr)
		}
		return err
	}
	s.Svc = manager
	return nil
}

// saveScopedInstanceProjectConfig persists a projected request config back to
// the selected instance in the base registry. Request-local stores are useful
// for reads, but saving into one would discard the change as soon as the HTTP
// request ends.
func (s *Server) saveScopedInstanceProjectConfig(projected config.AppConfig) error {
	baseStore := s.baseConfigStore()
	if baseStore == nil {
		return fmt.Errorf("config store is not initialized")
	}
	if s.BaseCfgStore == nil || s.BaseCfgStore == s.CfgStore {
		return s.saveConfigAndReconcile(projected)
	}
	base := baseStore.Snapshot()
	if len(base.Instances) == 0 {
		return s.saveConfigAndReconcile(projected)
	}
	instanceID := strings.TrimSpace(projected.DefaultInstanceID)
	selected, ok := base.Instance(instanceID)
	if !ok {
		return fmt.Errorf("instance %q not found", instanceID)
	}
	next := cloneConfigWithInstances(base)
	for i := range next.Instances {
		if next.Instances[i].ID != selected.ID {
			continue
		}
		applyProjectConfigToInstance(&next.Instances[i], projected)
		if selected.ID == base.DefaultInstanceID {
			// The compatibility mirror must follow a default-instance mutation;
			// otherwise config.normalize would correctly treat the stale top-level
			// value as a legacy edit and undo the nested change.
			applyProjectConfigFromInstance(&next, next.Instances[i])
		}
		break
	}
	next.DefaultInstanceID = base.DefaultInstanceID
	return s.saveConfigAndReconcile(next)
}

func (s *Server) serviceManagerForRequest(r *http.Request) (*service.Manager, string, error) {
	instanceID := strings.TrimSpace(r.URL.Query().Get("instance_id"))
	if instanceID == "" {
		instanceID = strings.TrimSpace(r.Header.Get("X-GA-Instance-ID"))
	}
	if s.InstanceManagers == nil {
		if instanceID == "" && s.Svc != nil {
			return s.Svc, "", nil
		}
		return nil, instanceID, fmt.Errorf("instance %q not found", instanceID)
	}
	manager, resolvedID, ok := s.InstanceManagers.manager(instanceID)
	if !ok {
		return nil, resolvedID, fmt.Errorf("instance %q not found", resolvedID)
	}
	return manager, resolvedID, nil
}

func (s *Server) projectInstanceSettings(instanceID string) (config.InstanceConfig, bool) {
	store := s.baseConfigStore()
	if store == nil {
		return config.InstanceConfig{}, false
	}
	cfg := store.Snapshot()
	if len(cfg.Instances) == 0 {
		return config.InstanceConfig{}, false
	}
	return cfg.Instance(instanceID)
}

func (s *Server) serviceManagerForHTTP(w http.ResponseWriter, r *http.Request) (*service.Manager, string, bool) {
	manager, instanceID, err := s.serviceManagerForRequest(r)
	if err != nil {
		bad(w, http.StatusNotFound, err.Error())
		return nil, instanceID, false
	}
	return manager, instanceID, true
}

func setResolvedInstanceHeader(w http.ResponseWriter, instanceID string) {
	if instanceID != "" {
		w.Header().Set("X-GA-Instance-ID", instanceID)
	}
}
