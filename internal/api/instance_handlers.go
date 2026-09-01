package api

import (
	"fmt"
	"io"
	"mime/multipart"
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
	ID          string `json:"id"`
	UseTemplate *bool  `json:"use_template,omitempty"`
}

type instanceCreateRequest struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	GARoot           string `json:"ga_root"`
	PythonPath       string `json:"python_path,omitempty"`
	SourceInstanceID string `json:"source_instance_id,omitempty"`
	CopyMemory       bool   `json:"copy_memory,omitempty"`
	CopyMyKey        bool   `json:"copy_mykey,omitempty"`
}

const automaticInstanceBaseID = "genericagent"
const protectedDefaultInstanceID = "default"
const instanceTemplateMaxBytes int64 = 512 << 20

func (s *Server) parseInstanceInstallRequest(w http.ResponseWriter, r *http.Request) (instanceInstallRequest, multipart.File, error) {
	var req instanceInstallRequest
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		return req, nil, decode(r, &req)
	}
	r.Body = http.MaxBytesReader(w, r.Body, instanceTemplateMaxBytes+(1<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return req, nil, fmt.Errorf("parse upload (maximum 512 MiB): %w", err)
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	req.ID = r.FormValue("id")
	if raw := strings.TrimSpace(r.FormValue("use_template")); raw != "" {
		useTemplate := !strings.EqualFold(raw, "false") && raw != "0"
		req.UseTemplate = &useTemplate
	}
	file, header, err := r.FormFile("template")
	if err == http.ErrMissingFile {
		return req, nil, nil
	}
	if err != nil {
		return req, nil, fmt.Errorf("read template upload: %w", err)
	}
	if !strings.EqualFold(filepath.Ext(header.Filename), ".zip") {
		file.Close()
		return req, nil, fmt.Errorf("template must be a .zip file")
	}
	return req, file, nil
}

func (s *Server) stageInstanceTemplate(id string, src multipart.File) (string, error) {
	if src == nil {
		return "", nil
	}
	dir := filepath.Join(s.CfgStore.Root, ".instance-install-archives")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, id+"-*.zip.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	keepTemp := false
	defer func() {
		if !keepTemp {
			_ = os.Remove(tmpPath)
		}
	}()
	n, copyErr := io.Copy(tmp, io.LimitReader(src, instanceTemplateMaxBytes+1))
	closeErr := tmp.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n > instanceTemplateMaxBytes {
		return "", fmt.Errorf("template exceeds 512 MiB")
	}
	if err := validateGenericAgentTemplateZip(tmpPath); err != nil {
		return "", err
	}
	keepTemp = true
	return tmpPath, nil
}

func (s *Server) instanceInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.instanceInstallsAvailable() {
		bad(w, http.StatusServiceUnavailable, "instance installer is shutting down")
		return
	}
	req, template, err := s.parseInstanceInstallRequest(w, r)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	if template != nil {
		defer template.Close()
	}
	id := req.ID
	if !isAutomaticInstanceID(id) {
		bad(w, http.StatusBadRequest, "id must be 1-64 characters, start with a letter or number, and contain only letters, numbers, '.', '_' or '-'")
		return
	}

	archivePath, err := s.stageInstanceTemplate(id, template)
	if err != nil {
		bad(w, http.StatusBadRequest, "invalid template archive: "+err.Error())
		return
	}
	keepArchive := false
	if archivePath != "" {
		defer func() {
			if !keepArchive {
				_ = os.Remove(archivePath)
			}
		}()
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
	useTemplate := req.UseTemplate == nil || *req.UseTemplate
	if archivePath != "" {
		if err := s.promoteInstanceTemplate(archivePath); err != nil {
			bad(w, http.StatusInternalServerError, "save reusable template archive: "+err.Error())
			return
		}
		archivePath = ""
		useTemplate = true
	}
	if useTemplate {
		var err error
		archivePath, err = s.snapshotReusableInstanceTemplate(id)
		if err != nil {
			bad(w, http.StatusInternalServerError, "prepare reusable template archive: "+err.Error())
			return
		}
	}

	instance := config.InstanceConfig{
		ID:              id,
		Name:            id,
		GARoot:          filepath.Join(s.CfgStore.Root, automaticInstanceBaseID, "instances", id),
		PythonPath:      cfg.PythonPath,
		EffectivePython: cfg.EffectivePython,
		InitStatus:      config.InstanceInitStatusInitializing,
		InitStage:       "queued",
		InitProgress:    5,
	}
	dest, err := s.automaticInstanceDestination(instance)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		bad(w, http.StatusInternalServerError, "create instances directory: "+err.Error())
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
	keepArchive = true
	writeJSON(w, map[string]interface{}{
		"ok":                  true,
		"items":               cfg.Instances,
		"default_instance_id": cfg.DefaultInstanceID,
		"template_available":  s.reusableInstanceTemplateAvailable(),
		"instance":            instance,
	})
}

func (s *Server) instancesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cfg := s.CfgStore.Snapshot()
	writeJSON(w, map[string]interface{}{
		"items":               cfg.Instances,
		"default_instance_id": cfg.DefaultInstanceID,
		"template_available":  s.reusableInstanceTemplateAvailable(),
	})
}

func (s *Server) instanceCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req instanceCreateRequest
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	instance := config.InstanceConfig{
		ID:         strings.TrimSpace(req.ID),
		Name:       strings.TrimSpace(req.Name),
		GARoot:     strings.TrimSpace(req.GARoot),
		PythonPath: strings.TrimSpace(req.PythonPath),
	}
	if instance.Name == "" {
		instance.Name = instance.ID
	}
	sourceID := strings.TrimSpace(req.SourceInstanceID)
	if sourceID == "" && (req.CopyMemory || req.CopyMyKey) {
		bad(w, http.StatusBadRequest, "copy options require source_instance_id")
		return
	}
	// Initialization state is owned by the server. Manual instance creation
	// cannot impersonate or enqueue an automatic installation.
	instance.InitStatus = ""
	instance.InitError = ""
	instance.InitStage = ""
	instance.InitProgress = 0
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
	if sourceID != "" {
		source, ok := cfg.Instance(sourceID)
		if !ok {
			bad(w, http.StatusNotFound, "source instance not found")
			return
		}
		if strings.EqualFold(strings.TrimSpace(source.InitStatus), config.InstanceInitStatusInitializing) {
			bad(w, http.StatusConflict, "source instance is initializing")
			return
		}
		if strings.TrimSpace(source.GARoot) == "" {
			bad(w, http.StatusBadRequest, "source instance has no GA root")
			return
		}
		if instance.GARoot == "" {
			if !isAutomaticInstanceID(instance.ID) {
				bad(w, http.StatusBadRequest, "an automatic instance id is required when destination ga_root is empty")
				return
			}
			instance.GARoot = filepath.Join(s.CfgStore.Root, automaticInstanceBaseID, "instances", instance.ID)
		}
		if err := cloneGenericAgentProject(source.GARoot, instance.GARoot, req.CopyMemory, req.CopyMyKey); err != nil {
			bad(w, http.StatusBadRequest, err.Error())
			return
		}
		instance.InitStatus = config.InstanceInitStatusReady
		instance.InitStage = "complete"
		instance.InitProgress = 100
		if instance.PythonPath == "" {
			instance.PythonPath = strings.TrimSpace(source.PythonPath)
		}
		if instance.EffectivePython == "" {
			instance.EffectivePython = strings.TrimSpace(source.EffectivePython)
		}
		// A project clone inherits its non-runtime Admin preferences, while
		// service autostart stays off to avoid unexpectedly starting a second
		// copy of a port-owning service after restart. Memory and mykey remain
		// explicit opt-in file-copy choices above.
		instance.ChatDataDir = filepath.Join(cfg.ChatDataDir, "instances", instance.ID)
		instance.BootstrapDone = source.BootstrapDone
		instance.ServiceModels = cloneIntMap(source.ServiceModels)
		instance.ModelProbeProviders = append([]string(nil), source.ModelProbeProviders...)
		if source.ChatTitleModel != nil {
			model := *source.ChatTitleModel
			instance.ChatTitleModel = &model
		}
		instance.ChatDefaultLLMNo = source.ChatDefaultLLMNo
		instance.SlashCommands = append([]config.SlashCommandItem(nil), source.SlashCommands...)
		instance.ExtraSystemPromptPresets = append([]config.ExtraSystemPromptPreset(nil), source.ExtraSystemPromptPresets...)
	}
	cfg.Instances = append(cfg.Instances, instance)
	if len(cfg.Instances) == 1 {
		cfg.DefaultInstanceID = instance.ID
	}
	if err := s.saveConfigAndReconcile(cfg); err != nil {
		if sourceID != "" {
			_ = os.RemoveAll(instance.GARoot)
		}
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	if sourceID != "" {
		saved := s.CfgStore.Snapshot()
		created, _ := saved.Instance(instance.ID)
		writeJSON(w, map[string]interface{}{
			"ok":                      true,
			"items":                   saved.Instances,
			"default_instance_id":     saved.DefaultInstanceID,
			"instance":                created,
			"copied_from_instance_id": sourceID,
			"copy_memory":             req.CopyMemory,
			"copy_mykey":              req.CopyMyKey,
		})
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
			instance.ChatDataDir = current.ChatDataDir
			instance.BootstrapDone = current.BootstrapDone
			instance.ServiceAutostart = append([]string(nil), current.ServiceAutostart...)
			instance.ServiceModels = cloneIntMap(current.ServiceModels)
			instance.ModelProbeProviders = append([]string(nil), current.ModelProbeProviders...)
			if current.ChatTitleModel != nil {
				model := *current.ChatTitleModel
				instance.ChatTitleModel = &model
			}
			instance.ChatDefaultLLMNo = current.ChatDefaultLLMNo
			instance.SlashCommands = append([]config.SlashCommandItem(nil), current.SlashCommands...)
			instance.ExtraSystemPromptPresets = append([]config.ExtraSystemPromptPreset(nil), current.ExtraSystemPromptPresets...)
			instance.InitStatus = current.InitStatus
			instance.InitError = current.InitError
			instance.InitStage = current.InitStage
			instance.InitProgress = current.InitProgress
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
	if s.ChatRuntimes != nil && s.ChatRuntimes.hasActiveWork(req.ID) {
		bad(w, http.StatusConflict, "instance has active chat work; stop the chat, loop, or title generation before deleting it")
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
	if req.ID == protectedDefaultInstanceID {
		s.ConfigMu.Unlock()
		bad(w, http.StatusConflict, "the default instance cannot be deleted")
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
	s.removeInstanceRuntimeState(req.ID)
	if err := os.Remove(s.instanceTemplateArchivePath(req.ID)); err != nil && !os.IsNotExist(err) {
		bad(w, http.StatusInternalServerError, "remove instance template: "+err.Error())
		return
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

func cloneIntMap(values map[string]int) map[string]int {
	if values == nil {
		return nil
	}
	cloned := make(map[string]int, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func writeInstanceMutationResult(w http.ResponseWriter, cfg config.AppConfig) {
	writeJSON(w, map[string]interface{}{
		"ok":                  true,
		"items":               cfg.Instances,
		"default_instance_id": cfg.DefaultInstanceID,
	})
}
