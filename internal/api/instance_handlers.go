package api

import (
	"net/http"
	"strings"

	"genericagent-admin-go/internal/config"
)

type instanceIDRequest struct {
	ID string `json:"id"`
}

func (s *Server) instancesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, map[string]interface{}{
		"items":               s.CfgStore.Cfg.Instances,
		"default_instance_id": s.CfgStore.Cfg.DefaultInstanceID,
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
	if instance.ID == "" {
		bad(w, http.StatusBadRequest, "instance id is required")
		return
	}

	s.ConfigMu.Lock()
	defer s.ConfigMu.Unlock()
	cfg := cloneConfigWithInstances(s.CfgStore.Cfg)
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
	writeInstanceMutationResult(w, s.CfgStore.Cfg)
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
	cfg := cloneConfigWithInstances(s.CfgStore.Cfg)
	found := false
	for i := range cfg.Instances {
		if cfg.Instances[i].ID == instance.ID {
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
	writeInstanceMutationResult(w, s.CfgStore.Cfg)
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
	defer s.ConfigMu.Unlock()
	cfg := cloneConfigWithInstances(s.CfgStore.Cfg)
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
		bad(w, http.StatusNotFound, "instance not found")
		return
	}
	if req.ID == cfg.DefaultInstanceID && len(kept) > 0 {
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
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	writeInstanceMutationResult(w, s.CfgStore.Cfg)
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
	cfg := cloneConfigWithInstances(s.CfgStore.Cfg)
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
	writeInstanceMutationResult(w, s.CfgStore.Cfg)
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
