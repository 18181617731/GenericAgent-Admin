package api

import (
	"net/http"

	"genericagent-admin-go/internal/modelconfig"
)

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
		if !requireDangerousHeader(w, r) {
			return
		}
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
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !requireDangerousHeader(w, r) {
		return
	}
	d, err := s.Models.Load(true)
	if err != nil {
		bad(w, 500, err.Error())
		return
	}
	writeJSON(w, d)
}

func (s *Server) modelsPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
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
	if (p.Reveal || p.Save) && !requireDangerousHeader(w, r) {
		return
	}
	d, err := modelconfig.ImportMyKeyWithPython(s.CfgStore.Cfg.GARoot, s.CfgStore.Cfg.PythonPath, p.Reveal)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	if p.Save {
		if !p.Reveal {
			bad(w, 400, "refusing to save masked mykey import; set reveal=true with explicit authorization")
			return
		}
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
	if r.Method != http.MethodPost {
		bad(w, 405, "method not allowed")
		return
	}
	var p struct {
		Profiles        []modelconfig.Profile `json:"profiles"`
		OverwriteActive bool                  `json:"overwrite_active"`
	}
	if err := decode(r, &p); err != nil {
		bad(w, 400, err.Error())
		return
	}
	profiles, err := s.Models.MergePreservedSecrets(p.Profiles)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	res, err := modelconfig.Export(s.CfgStore.Cfg.GARoot, profiles, p.OverwriteActive)
	if err != nil {
		bad(w, 400, err.Error())
		return
	}
	if _, err := s.Models.Save(profiles); err != nil {
		bad(w, 400, err.Error())
		return
	}
	writeJSON(w, res)
}
