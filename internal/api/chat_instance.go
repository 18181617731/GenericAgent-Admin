package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"genericagent-admin-go/internal/config"
)

// chatRuntime owns all mutable in-memory chat state for one GA instance.
// Keeping the mutexes with the maps prevents request-scoped Server copies from
// accidentally copying a live mutex.
type chatRuntime struct {
	chatMu    sync.Mutex
	sessionMu sync.Mutex
	usageMu   sync.Mutex
	runs      map[string]*chatRun
	workers   map[string]*chatWorker
	titleJobs map[string]bool
}

type chatRuntimeRegistry struct {
	mu      sync.Mutex
	entries map[string]*chatRuntime
}

func newChatRuntimeRegistry() *chatRuntimeRegistry {
	return &chatRuntimeRegistry{entries: make(map[string]*chatRuntime)}
}

func (r *chatRuntimeRegistry) runtime(instanceID string) *chatRuntime {
	instanceID = strings.TrimSpace(instanceID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if runtime := r.entries[instanceID]; runtime != nil {
		return runtime
	}
	runtime := &chatRuntime{
		runs:      make(map[string]*chatRun),
		workers:   make(map[string]*chatWorker),
		titleJobs: make(map[string]bool),
	}
	r.entries[instanceID] = runtime
	return runtime
}

func requestedInstanceID(r *http.Request) string {
	instanceID := strings.TrimSpace(r.URL.Query().Get("instance_id"))
	if instanceID == "" {
		instanceID = strings.TrimSpace(r.Header.Get("X-GA-Instance-ID"))
	}
	return instanceID
}

func (s *Server) chatServerForRequest(r *http.Request) (*Server, string, error) {
	baseStore := s.BaseCfgStore
	if baseStore == nil {
		baseStore = s.CfgStore
	}
	if baseStore == nil {
		return nil, "", fmt.Errorf("config store is not initialized")
	}

	instanceID := requestedInstanceID(r)
	instance, ok := baseStore.Cfg.Instance(instanceID)
	if !ok {
		// Preserve the legacy single-instance test/server setup, which has no
		// instance registry yet.
		if instanceID == "" && len(baseStore.Cfg.Instances) == 0 {
			clone := *s
			clone.BaseCfgStore = baseStore
			return &clone, "", nil
		}
		return nil, instanceID, fmt.Errorf("instance %q not found", instanceID)
	}
	instanceID = instance.ID

	cfg := baseStore.Cfg
	cfg.GARoot = instance.GARoot
	cfg.PythonPath = instance.PythonPath
	cfg.EffectivePython = instance.EffectivePython
	runtimeID := instanceID
	if instanceID == "default" {
		// The migrated legacy instance keeps both the legacy data directory and
		// in-memory runtime. This prevents a first config save from moving an
		// active legacy chat into instances/default halfway through its lifetime.
		cfg.ChatDataDir = baseStore.Cfg.ChatDataDir
		runtimeID = ""
	} else {
		cfg.ChatDataDir = filepath.Join(baseStore.Cfg.ChatDataDir, "instances", instanceID)
	}
	instanceStore := &config.Store{Root: baseStore.Root, Cfg: cfg}

	registry := s.ChatRuntimes
	if registry == nil {
		registry = newChatRuntimeRegistry()
		s.ChatRuntimes = registry
	}
	runtime := registry.runtime(runtimeID)
	clone := *s
	clone.CfgStore = instanceStore
	clone.BaseCfgStore = baseStore
	clone.ChatMu = &runtime.chatMu
	clone.SessionMu = &runtime.sessionMu
	clone.UsageMu = &runtime.usageMu
	clone.ChatRuns = runtime.runs
	clone.ChatWorkers = runtime.workers
	clone.ChatTitleJobs = runtime.titleJobs
	return &clone, instanceID, nil
}

func (s *Server) withChatInstance(next func(*Server, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chatServer, instanceID, err := s.chatServerForRequest(r)
		if err != nil {
			bad(w, http.StatusNotFound, err.Error())
			return
		}
		setResolvedInstanceHeader(w, instanceID)
		next(chatServer, w, r)
	}
}
