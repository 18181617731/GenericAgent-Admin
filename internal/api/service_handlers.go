package api

import (
	"net/http"
	"strings"

	"genericagent-admin-go/internal/service"
)

func (s *Server) services(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
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
	for i := range items {
		items[i].Autostart = auto[items[i].Name]
	}
	return items
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Svc.Summary()) }

type nameReq struct {
	Name string `json:"name"`
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
	svc, err := s.Svc.Start(q.Name)
	if err != nil {
		bad(w, 404, err.Error())
		return
	}
	writeJSON(w, svc)
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

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/logs/")
	lines := s.CfgStore.Cfg.LogTailLines
	writeJSON(w, map[string]interface{}{"name": name, "lines": s.Svc.Logs(name, lines)})
}
