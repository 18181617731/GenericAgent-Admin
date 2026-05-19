package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/ga"
	"genericagent-admin-go/internal/modelconfig"
	"genericagent-admin-go/internal/service"
)

type Server struct {
	CfgStore *config.Store
	Svc      *service.Manager
	Models   *modelconfig.Store
	Static   fs.FS
}

func New(cfg *config.Store, svc *service.Manager, models *modelconfig.Store, static fs.FS) *Server {
	return &Server{CfgStore: cfg, Svc: svc, Models: models, Static: static}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/ga/inventory", s.gaInventory)
	mux.HandleFunc("/api/ga/health", s.gaHealth)
	mux.HandleFunc("/api/schedule/tasks", s.scheduleTasks)
	mux.HandleFunc("/api/schedule/toggle", s.scheduleToggle)
	mux.HandleFunc("/api/config", s.configHandler)
	mux.HandleFunc("/api/services", s.services)
	mux.HandleFunc("/api/services/summary", s.summary)
	mux.HandleFunc("/api/services/start", s.start)
	mux.HandleFunc("/api/services/stop", s.stop)
	mux.HandleFunc("/api/services/stop-all", s.stopAll)
	mux.HandleFunc("/api/logs/", s.logs)
	mux.HandleFunc("/api/models", s.models)
	mux.HandleFunc("/api/models/raw", s.modelsRaw)
	mux.HandleFunc("/api/models/preview", s.modelsPreview)
	mux.HandleFunc("/api/models/import-mykey", s.modelsImportMyKey)
	mux.HandleFunc("/api/models/export", s.modelsExport)
	mux.HandleFunc("/", s.static)
	return cors(mux)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func bad(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": msg})
}
func decode(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{"ok": true, "config": s.CfgStore.Cfg, "services": s.Svc.Summary()})
}

func (s *Server) gaInventory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, ga.BuildInventory(s.CfgStore.Cfg.GARoot))
}

func (s *Server) gaHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, ga.BuildHealth(s.CfgStore.Cfg.GARoot))
}

func (s *Server) scheduleTasks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, ga.BuildSchedule(s.CfgStore.Cfg.GARoot))
}

func (s *Server) scheduleToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var req struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := decode(r, &req); err != nil || req.ID == "" {
		bad(w, 400, "bad request")
		return
	}
	task, err := ga.ToggleTask(s.CfgStore.Cfg.GARoot, req.ID, req.Enabled)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "task": task})
}

func (s *Server) configHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		writeJSON(w, s.CfgStore.Cfg)
		return
	}
	if r.Method == "PUT" {
		var c config.AppConfig
		if err := decode(r, &c); err != nil {
			bad(w, 400, err.Error())
			return
		}
		if err := s.CfgStore.Save(c); err != nil {
			bad(w, 400, err.Error())
			return
		}
		s.Svc.SetRoot(c.GARoot, c.BufferLines)
		writeJSON(w, c)
		return
	}
	bad(w, 405, "method not allowed")
}
func (s *Server) services(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		bad(w, 405, "method not allowed")
		return
	}
	writeJSON(w, s.Svc.Discover())
}
func (s *Server) summary(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Svc.Summary()) }

type nameReq struct {
	Name string `json:"name"`
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
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
	s.Svc.StopAll()
	writeJSON(w, map[string]bool{"ok": true})
}
func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/logs/")
	lines := s.CfgStore.Cfg.LogTailLines
	writeJSON(w, map[string]interface{}{"name": name, "lines": s.Svc.Logs(name, lines)})
}

func (s *Server) models(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		d, err := s.Models.Load(false)
		if err != nil {
			bad(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]interface{}{"profiles": d.Profiles, "updated_at": d.UpdatedAt, "source": modelconfig.SourceStatus(s.CfgStore.Cfg.GARoot)})
		return
	}
	if r.Method == "PUT" {
		var p struct {
			Profiles []modelconfig.Profile `json:"profiles"`
		}
		if err := decode(r, &p); err != nil {
			bad(w, 400, err.Error())
			return
		}
		d, err := s.Models.Save(p.Profiles)
		if err != nil {
			bad(w, 400, err.Error())
			return
		}
		writeJSON(w, d)
		return
	}
	bad(w, 405, "method not allowed")
}
func (s *Server) modelsRaw(w http.ResponseWriter, r *http.Request) {
	d, err := s.Models.Load(true)
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, d)
}
func (s *Server) modelsPreview(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Profiles []modelconfig.Profile `json:"profiles"`
	}
	if err := decode(r, &p); err != nil {
		bad(w, 400, err.Error())
		return
	}
	txt, err := modelconfig.Render(p.Profiles)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]string{"python": txt})
}
func (s *Server) modelsImportMyKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		bad(w, 405, "method not allowed")
		return
	}
	var p struct {
		Reveal bool `json:"reveal"`
		Save   bool `json:"save"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := decode(r, &p); err != nil {
			bad(w, 400, err.Error())
			return
		}
	}
	d, err := modelconfig.ImportMyKey(s.CfgStore.Cfg.GARoot, p.Reveal)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	if p.Save {
		if saved, err := s.Models.Save(d.Profiles); err == nil {
			d = saved
		} else {
			bad(w, 400, err.Error())
			return
		}
	}
	writeJSON(w, map[string]interface{}{"profiles": d.Profiles, "updated_at": d.UpdatedAt, "saved": p.Save, "masked": !p.Reveal})
}
func (s *Server) modelsExport(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Profiles        []modelconfig.Profile `json:"profiles"`
		OverwriteActive bool                  `json:"overwrite_active"`
	}
	if err := decode(r, &p); err != nil {
		bad(w, 400, err.Error())
		return
	}
	_, _ = s.Models.Save(p.Profiles)
	res, err := modelconfig.Export(s.CfgStore.Cfg.GARoot, p.Profiles, p.OverwriteActive)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, res)
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if s.Static == nil {
		bad(w, 404, "web dist not embedded")
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	p = path.Clean(p)
	data, err := fs.ReadFile(s.Static, p)
	if err != nil {
		data, err = fs.ReadFile(s.Static, "index.html")
		if err != nil {
			bad(w, 404, fmt.Sprintf("not found: %s", p))
			return
		}
		p = "index.html"
	}
	if strings.HasSuffix(p, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
	} else if strings.HasSuffix(p, ".css") {
		w.Header().Set("Content-Type", "text/css")
	} else if strings.HasSuffix(p, ".html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	_, _ = w.Write(data)
}
