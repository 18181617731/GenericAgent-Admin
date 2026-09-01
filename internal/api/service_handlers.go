package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"genericagent-admin-go/internal/ga"
	"genericagent-admin-go/internal/service"
)

func (s *Server) services(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	manager, instanceID, ok := s.serviceManagerForHTTP(w, r)
	if !ok {
		return
	}
	setResolvedInstanceHeader(w, instanceID)
	writeJSON(w, s.servicesWithAutostart(manager))
}

func (s *Server) servicesWithAutostart(manager *service.Manager) []service.ServiceInfo {
	items := manager.Discover()
	auto := map[string]bool{}
	for _, name := range s.CfgStore.Snapshot().ServiceAutostart {
		auto[name] = true
	}
	if manager == s.Svc {
		items = append(items, s.adminFeishuServiceInfo())
	}
	models := s.CfgStore.Snapshot().ServiceModels
	for i := range items {
		items[i].Autostart = auto[items[i].Name]
		if models != nil && service.SupportsModelConfiguration(items[i]) {
			if no, ok := models[items[i].Name]; ok {
				n := no
				items[i].ModelNo = &n
			}
		}
	}
	return items
}

func (s *Server) adminFeishuServiceInfo() service.ServiceInfo {
	running, pid, startedAt := s.chatFeishuBridgeStatus()
	return service.ServiceInfo{
		Name:      adminFeishuServiceName,
		Kind:      "frontend",
		Command:   []string{"ga-admin", "feishu-admin-sync"},
		WorkDir:   s.CfgStore.Snapshot().GARoot,
		Running:   running,
		PID:       pid,
		StartedAt: startedAt,
		Managed:   true,
		NoLogs:    true,
	}
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	manager, instanceID, ok := s.serviceManagerForHTTP(w, r)
	if !ok {
		return
	}
	setResolvedInstanceHeader(w, instanceID)
	summary := manager.Summary()
	if manager == s.Svc {
		summary["total"]++
		if s.IsChatFeishuBridgeRunning() {
			summary["running"]++
		} else {
			summary["stopped"]++
		}
	}
	writeJSON(w, summary)
}

type nameReq struct {
	Name   string            `json:"name"`
	Params map[string]string `json:"params,omitempty"`
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	manager, instanceID, ok := s.serviceManagerForHTTP(w, r)
	if !ok {
		return
	}
	var q nameReq
	if err := decode(r, &q); err != nil {
		bad(w, 400, err.Error())
		return
	}
	svc, err := s.startServiceWithManager(manager, q.Name, q.Params)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrServiceNotFound) {
			status = http.StatusNotFound
		}
		bad(w, status, err.Error())
		return
	}
	setResolvedInstanceHeader(w, instanceID)
	writeJSON(w, svc)
}

func (s *Server) startServiceByName(name string, params map[string]string) (service.ServiceInfo, error) {
	return s.startServiceWithManager(s.Svc, name, params)
}

func (s *Server) startServiceWithManager(manager *service.Manager, name string, params map[string]string) (service.ServiceInfo, error) {
	if manager == nil {
		return service.ServiceInfo{}, fmt.Errorf("service manager unavailable")
	}
	name = strings.TrimSpace(name)
	if name == adminFeishuServiceName {
		if manager != s.Svc {
			return service.ServiceInfo{}, service.ErrServiceNotFound
		}
		if err := s.StartChatFeishuBridge(); err != nil {
			return service.ServiceInfo{}, err
		}
		return s.adminFeishuServiceInfo(), nil
	}
	svc, ok := manager.Find(name)
	if !ok {
		return service.ServiceInfo{}, service.ErrServiceNotFound
	}
	if !service.SupportsManualLifecycle(svc) {
		return svc, service.ErrWorkflowManaged
	}
	if service.SupportsModelConfiguration(svc) && (params == nil || strings.TrimSpace(params["llm_no"]) == "") {
		if models := s.CfgStore.Snapshot().ServiceModels; models != nil {
			if no, ok := models[name]; ok {
				if params == nil {
					params = map[string]string{}
				}
				params["llm_no"] = strconv.Itoa(no)
			}
		}
	}
	if _, statErr := os.Stat(filepath.Join(manager.GARoot, "llmcore.py")); statErr == nil {
		if _, err := ga.EnsureUsageTelemetry(manager.GARoot); err != nil {
			return svc, err
		}
	}
	return manager.StartWithParams(name, params)
}

func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	manager, instanceID, ok := s.serviceManagerForHTTP(w, r)
	if !ok {
		return
	}
	var q nameReq
	if err := decode(r, &q); err != nil {
		bad(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(q.Name) == adminFeishuServiceName {
		if manager != s.Svc {
			bad(w, http.StatusNotFound, "service not found")
			return
		}
		s.StopChatFeishuBridge()
		setResolvedInstanceHeader(w, instanceID)
		writeJSON(w, s.adminFeishuServiceInfo())
		return
	}
	if err := manager.Stop(q.Name); err != nil {
		bad(w, 400, err.Error())
		return
	}
	svc, _ := manager.Find(q.Name)
	setResolvedInstanceHeader(w, instanceID)
	writeJSON(w, svc)
}

func (s *Server) stopAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	manager, instanceID, ok := s.serviceManagerForHTTP(w, r)
	if !ok {
		return
	}
	manager.StopAll()
	if manager == s.Svc {
		s.StopChatFeishuBridge()
	}
	setResolvedInstanceHeader(w, instanceID)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) serviceAutostart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	manager, instanceID, ok := s.serviceManagerForHTTP(w, r)
	if !ok {
		return
	}
	var q struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := decode(r, &q); err != nil || strings.TrimSpace(q.Name) == "" {
		bad(w, 400, "bad request")
		return
	}
	q.Name = strings.TrimSpace(q.Name)
	if q.Name == adminFeishuServiceName {
		if manager != s.Svc {
			bad(w, http.StatusNotFound, "service not found")
			return
		}
	} else {
		svc, ok := manager.Find(q.Name)
		if !ok {
			bad(w, 404, "service not found")
			return
		}
		if q.Enabled && !service.SupportsManualLifecycle(svc) {
			bad(w, 400, "service autostart is managed by its Goal or checklist workflow")
			return
		}
	}
	cfg := s.CfgStore.Snapshot()
	seen := map[string]bool{}
	next := []string{}
	for _, name := range cfg.ServiceAutostart {
		if name == q.Name || seen[name] {
			continue
		}
		seen[name] = true
		next = append(next, name)
	}
	if q.Enabled {
		next = append(next, q.Name)
	}
	cfg.ServiceAutostart = next
	if err := s.CfgStore.Save(cfg); err != nil {
		bad(w, 500, err.Error())
		return
	}
	setResolvedInstanceHeader(w, instanceID)
	writeJSON(w, map[string]interface{}{"ok": true, "services": s.servicesWithAutostart(manager)})
}

func (s *Server) serviceModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	manager, instanceID, ok := s.serviceManagerForHTTP(w, r)
	if !ok {
		return
	}
	var q struct {
		Name  string `json:"name"`
		LLMNo *int   `json:"llm_no"`
	}
	if err := decode(r, &q); err != nil || strings.TrimSpace(q.Name) == "" {
		bad(w, 400, "bad request")
		return
	}
	svc, ok := manager.Find(q.Name)
	if !ok {
		bad(w, 404, "service not found")
		return
	}
	if q.LLMNo != nil && !service.SupportsModelConfiguration(svc) {
		bad(w, 400, "service does not support model configuration")
		return
	}
	cfg := s.CfgStore.Snapshot()
	models := map[string]int{}
	for k, v := range cfg.ServiceModels {
		models[k] = v
	}
	if q.LLMNo == nil {
		delete(models, q.Name)
	} else {
		models[q.Name] = *q.LLMNo
	}
	cfg.ServiceModels = models
	if err := s.CfgStore.Save(cfg); err != nil {
		bad(w, 500, err.Error())
		return
	}
	setResolvedInstanceHeader(w, instanceID)
	writeJSON(w, map[string]interface{}{"ok": true, "services": s.servicesWithAutostart(manager)})
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	manager, instanceID, ok := s.serviceManagerForHTTP(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/logs/"))
	if strings.HasSuffix(name, "/stream") {
		s.logStream(w, r, manager, instanceID, strings.TrimSpace(strings.TrimSuffix(name, "/stream")))
		return
	}
	if name == "" {
		bad(w, http.StatusBadRequest, "service name required")
		return
	}
	if _, ok := manager.Find(name); !ok {
		bad(w, http.StatusNotFound, "service not found")
		return
	}
	lines, err := s.requestedLogLines(r)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	setResolvedInstanceHeader(w, instanceID)
	writeJSON(w, map[string]interface{}{"name": name, "lines": manager.Logs(name, lines)})
}

func (s *Server) requestedLogLines(r *http.Request) (int, error) {
	lines := s.CfgStore.Snapshot().LogTailLines
	raw := strings.TrimSpace(r.URL.Query().Get("lines"))
	if raw == "" {
		return lines, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 || parsed > 5000 {
		return 0, fmt.Errorf("lines must be an integer between 1 and 5000")
	}
	return parsed, nil
}

func (s *Server) logStream(w http.ResponseWriter, r *http.Request, manager *service.Manager, instanceID, name string) {
	if name == "" {
		bad(w, http.StatusBadRequest, "service name required")
		return
	}
	if _, ok := manager.Find(name); !ok {
		bad(w, http.StatusNotFound, "service not found")
		return
	}
	lines, err := s.requestedLogLines(r)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		bad(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	snapshot, events, cancel := manager.Subscribe(name, lines)
	defer cancel()
	setResolvedInstanceHeader(w, instanceID)
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	writeLogSSE(w, "snapshot", map[string]interface{}{"lines": snapshot})
	flusher.Flush()

	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-events:
			if !open {
				return
			}
			if event.Reset {
				writeLogSSE(w, "reset", map[string]interface{}{"lines": event.Lines})
			} else {
				writeLogSSE(w, "log", map[string]interface{}{"line": event.Line})
			}
			flusher.Flush()
		case <-keepAlive.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func writeLogSSE(w http.ResponseWriter, event string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}
