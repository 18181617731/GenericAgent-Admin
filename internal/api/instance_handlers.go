package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"genericagent-admin-go/internal/config"
)

type instanceIDRequest struct {
	ID string `json:"id"`
}

type instanceInstallRequest struct {
	ID string `json:"id"`
}

const automaticInstanceBaseID = "genericagent"

func (s *Server) instanceInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.instanceInstallsAvailable() {
		bad(w, http.StatusServiceUnavailable, "instance installer is shutting down")
		return
	}
	var req instanceInstallRequest
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	id := req.ID
	if !isAutomaticInstanceID(id) {
		bad(w, http.StatusBadRequest, "id must be 1-64 characters, start with a letter or number, and contain only letters, numbers, '.', '_' or '-'")
		return
	}

	s.ConfigMu.Lock()
	defer s.ConfigMu.Unlock()

	cfg := cloneConfigWithInstances(s.CfgStore.Snapshot())
	for _, current := range cfg.Instances {
		if current.ID == id {
			bad(w, http.StatusConflict, "instance id already exists: "+id)
			return
		}
		if strings.EqualFold(strings.TrimSpace(current.Name), id) {
			bad(w, http.StatusConflict, "instance name already exists: "+id)
			return
		}
	}

	instance := config.InstanceConfig{
		ID:              id,
		Name:            id,
		GARoot:          filepath.Join(s.CfgStore.Root, id),
		PythonPath:      cfg.PythonPath,
		EffectivePython: cfg.EffectivePython,
		InitStatus:      config.InstanceInitStatusInitializing,
	}
	dest, err := s.automaticInstanceDestination(instance)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.Mkdir(dest, 0755); err != nil {
		if os.IsExist(err) {
			bad(w, http.StatusConflict, "instance destination already exists: "+dest)
			return
		}
		bad(w, http.StatusInternalServerError, "create instance directory: "+err.Error())
		return
	}
	keepInstall := false
	defer func() {
		if !keepInstall {
			_ = os.RemoveAll(dest)
		}
	}()
	cfg.Instances = append(cfg.Instances, instance)
	if err := s.CfgStore.Save(cfg); err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !s.startInstanceInstall(instance) {
		cfg.Instances = cfg.Instances[:len(cfg.Instances)-1]
		if err := s.CfgStore.Save(cfg); err != nil {
			bad(w, http.StatusInternalServerError, "instance installer is shutting down; roll back config: "+err.Error())
			return
		}
		bad(w, http.StatusServiceUnavailable, "instance installer is shutting down")
		return
	}
	keepInstall = true
	writeJSON(w, map[string]interface{}{
		"ok":                  true,
		"items":               cfg.Instances,
		"default_instance_id": cfg.DefaultInstanceID,
		"instance":            instance,
	})
}

func (s *Server) instancesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, map[string]interface{}{
		"items":               s.CfgStore.Snapshot().Instances,
		"default_instance_id": s.CfgStore.Snapshot().DefaultInstanceID,
	})
}

func (s *Server) instanceCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var instance config.InstanceConfig
	if err := decode(r, &instance); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	instance.ID = strings.TrimSpace(instance.ID)
	// Initialization state is owned by the server. Manual instance creation
	// cannot impersonate or enqueue an automatic installation.
	instance.InitStatus = ""
	instance.InitError = ""
	if instance.ID == "" {
		bad(w, http.StatusBadRequest, "instance id is required")
		return
	}

	s.ConfigMu.Lock()
	defer s.ConfigMu.Unlock()
	cfg := cloneConfigWithInstances(s.CfgStore.Snapshot())
	for _, current := range cfg.Instances {
		if current.ID == instance.ID {
			bad(w, http.StatusConflict, "instance id already exists")
			return
		}
	}
	cfg.Instances = append(cfg.Instances, instance)
	if len(cfg.Instances) == 1 {
		cfg.DefaultInstanceID = instance.ID
	}
	if err := s.saveConfigAndReconcile(cfg); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	writeInstanceMutationResult(w, s.CfgStore.Snapshot())
}

func (s *Server) instanceUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var instance config.InstanceConfig
	if err := decode(r, &instance); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	instance.ID = strings.TrimSpace(instance.ID)
	if instance.ID == "" {
		bad(w, http.StatusBadRequest, "instance id is required")
		return
	}

	s.ConfigMu.Lock()
	defer s.ConfigMu.Unlock()
	cfg := cloneConfigWithInstances(s.CfgStore.Snapshot())
	found := false
	for i := range cfg.Instances {
		if cfg.Instances[i].ID == instance.ID {
			current := cfg.Instances[i]
			if strings.EqualFold(strings.TrimSpace(current.InitStatus), config.InstanceInitStatusInitializing) {
				bad(w, http.StatusConflict, "instance is initializing")
				return
			}
			// Initialization state is server-owned and cannot be overwritten by
			// an instance metadata update.
			instance.InitStatus = current.InitStatus
			instance.InitError = current.InitError
			cfg.Instances[i] = instance
			found = true
			break
		}
	}
	if !found {
		bad(w, http.StatusNotFound, "instance not found")
		return
	}
	if err := s.saveConfigAndReconcile(cfg); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	writeInstanceMutationResult(w, s.CfgStore.Snapshot())
}

func (s *Server) instanceDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req instanceIDRequest
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		bad(w, http.StatusBadRequest, "instance id is required")
		return
	}

	s.ConfigMu.Lock()
	cfg := cloneConfigWithInstances(s.CfgStore.Snapshot())
	kept := make([]config.InstanceConfig, 0, len(cfg.Instances))
	found := false
	for _, instance := range cfg.Instances {
		if instance.ID == req.ID {
			found = true
			continue
		}
		kept = append(kept, instance)
	}
	if !found {
		s.ConfigMu.Unlock()
		bad(w, http.StatusNotFound, "instance not found")
		return
	}
	if req.ID == cfg.DefaultInstanceID && len(kept) > 0 {
		s.ConfigMu.Unlock()
		bad(w, http.StatusConflict, "set another default instance before deleting the current default")
		return
	}
	cfg.Instances = kept
	if len(kept) == 0 {
		cfg.DefaultInstanceID = ""
		cfg.GARoot = ""
		cfg.PythonPath = ""
		cfg.EffectivePython = ""
	}
	if err := s.saveConfigAndReconcile(cfg); err != nil {
		s.ConfigMu.Unlock()
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	s.ConfigMu.Unlock()

	// Persist the removal before cancelling. The worker may be waiting for
	// ConfigMu to publish its final state, so waiting while holding that lock
	// would deadlock. Once removed, a late state publication is a no-op.
	if done := s.cancelInstanceInstall(req.ID); done != nil {
		<-done
	}
	writeInstanceMutationResult(w, s.CfgStore.Snapshot())
}

func (s *Server) instanceSetDefault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req instanceIDRequest
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	req.ID = strings.TrimSpace(req.ID)

	s.ConfigMu.Lock()
	defer s.ConfigMu.Unlock()
	cfg := cloneConfigWithInstances(s.CfgStore.Snapshot())
	found := false
	for _, instance := range cfg.Instances {
		if instance.ID == req.ID {
			found = true
			break
		}
	}
	if !found {
		bad(w, http.StatusNotFound, "instance not found")
		return
	}
	cfg.DefaultInstanceID = req.ID
	if err := s.saveConfigAndReconcile(cfg); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	writeInstanceMutationResult(w, s.CfgStore.Snapshot())
}

func cloneConfigWithInstances(cfg config.AppConfig) config.AppConfig {
	cfg.Instances = append([]config.InstanceConfig(nil), cfg.Instances...)
	return cfg
}

func writeInstanceMutationResult(w http.ResponseWriter, cfg config.AppConfig) {
	writeJSON(w, map[string]interface{}{
		"ok":                  true,
		"items":               cfg.Instances,
		"default_instance_id": cfg.DefaultInstanceID,
	})
}
