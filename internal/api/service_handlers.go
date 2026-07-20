package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"genericagent-admin-go/internal/service"
)

func (s *Server) services(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, 405, "method not allowed")
		return
	}
	writeJSON(w, s.servicesWithAutostart())
}

func (s *Server) servicesWithAutostart() []service.ServiceInfo {
	items := s.Svc.Discover()
	auto := map[string]bool{}
	for _, name := range s.CfgStore.Cfg.ServiceAutostart {
		auto[name] = true
	}
	models := s.CfgStore.Cfg.ServiceModels
	for i := range items {
		items[i].Autostart = auto[items[i].Name]
		if models != nil {
			if no, ok := models[items[i].Name]; ok {
				n := no
				items[i].ModelNo = &n
			}
		}
	}
	return items
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, s.Svc.Summary())
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
	var q nameReq
	if err := decode(r, &q); err != nil {
		bad(w, 400, err.Error())
		return
	}
	svc, err := s.startServiceByName(q.Name, q.Params)
	if err != nil {
		bad(w, 404, err.Error())
		return
	}
	writeJSON(w, svc)
}

func (s *Server) startServiceByName(name string, params map[string]string) (service.ServiceInfo, error) {
	if params == nil || strings.TrimSpace(params["llm_no"]) == "" {
		if models := s.CfgStore.Cfg.ServiceModels; models != nil {
			if no, ok := models[name]; ok {
				if params == nil {
					params = map[string]string{}
				}
				params["llm_no"] = strconv.Itoa(no)
			}
		}
	}
	return s.Svc.StartWithParams(name, params)
}

func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var q nameReq
	if err := decode(r, &q); err != nil {
		bad(w, 400, err.Error())
		return
	}
	if err := s.Svc.Stop(q.Name); err != nil {
		bad(w, 400, err.Error())
		return
	}
	svc, _ := s.Svc.Find(q.Name)
	writeJSON(w, svc)
}

func (s *Server) stopAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	s.Svc.StopAll()
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) serviceAutostart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
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
	if _, ok := s.Svc.Find(q.Name); !ok {
		bad(w, 404, "service not found")
		return
	}
	cfg := s.CfgStore.Cfg
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
	writeJSON(w, map[string]interface{}{"ok": true, "services": s.servicesWithAutostart()})
}

func (s *Server) serviceModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
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
	if _, ok := s.Svc.Find(q.Name); !ok {
		bad(w, 404, "service not found")
		return
	}
	cfg := s.CfgStore.Cfg
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
	writeJSON(w, map[string]interface{}{"ok": true, "services": s.servicesWithAutostart()})
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.Svc == nil {
		bad(w, http.StatusInternalServerError, "service manager unavailable")
		return
	}
	name := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/logs/"))
	if strings.HasSuffix(name, "/stream") {
		s.logStream(w, r, strings.TrimSpace(strings.TrimSuffix(name, "/stream")))
		return
	}
	if name == "" {
		bad(w, http.StatusBadRequest, "service name required")
		return
	}
	if _, ok := s.Svc.Find(name); !ok {
		bad(w, http.StatusNotFound, "service not found")
		return
	}
	lines, err := s.requestedLogLines(r)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"name": name, "lines": s.Svc.Logs(name, lines)})
}

func (s *Server) requestedLogLines(r *http.Request) (int, error) {
	lines := s.CfgStore.Cfg.LogTailLines
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

func (s *Server) logStream(w http.ResponseWriter, r *http.Request, name string) {
	if name == "" {
		bad(w, http.StatusBadRequest, "service name required")
		return
	}
	if _, ok := s.Svc.Find(name); !ok {
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

	snapshot, events, cancel := s.Svc.Subscribe(name, lines)
	defer cancel()
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
