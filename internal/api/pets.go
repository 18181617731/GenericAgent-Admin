package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type petsIndexFile struct {
	Version     int            `json:"version,omitempty"`
	Runtime     string         `json:"runtime,omitempty"`
	Description string         `json:"description,omitempty"`
	Pets        []petIndexItem `json:"pets"`
}

type petIndexItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Manifest    string `json:"manifest"`
	Spritesheet string `json:"spritesheet"`
}

type petManifest struct {
	PetID       string `json:"pet_id"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Runtime     string `json:"runtime"`
	AssetBase   string `json:"asset_base"`
	Spritesheet string `json:"spritesheet"`
	Atlas       struct {
		Columns     int `json:"columns"`
		Rows        int `json:"rows"`
		FrameWidth  int `json:"frame_width"`
		FrameHeight int `json:"frame_height"`
	} `json:"atlas"`
}

type petListItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Manifest    string `json:"manifest"`
	Spritesheet string `json:"spritesheet"`
	AssetBase   string `json:"asset_base,omitempty"`
	Runtime     string `json:"runtime,omitempty"`
	Columns     int    `json:"columns,omitempty"`
	Rows        int    `json:"rows,omitempty"`
	FrameWidth  int    `json:"frame_width,omitempty"`
	FrameHeight int    `json:"frame_height,omitempty"`
}

type petsState struct {
	ActivePetID string `json:"active_pet_id"`
}

func (s *Server) petsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	items, idx, err := s.loadPetsCatalog()
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	active := s.ActivePetID()
	writeJSON(w, map[string]any{"ok": true, "runtime": idx.Runtime, "description": idx.Description, "active_pet_id": active, "pets": items})
}

func (s *Server) petsActiveHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		active := s.ActivePetID()
		writeJSON(w, map[string]any{"ok": true, "active_pet_id": active})
	case http.MethodPost:
		var req struct {
			PetID string `json:"pet_id"`
		}
		if err := decode(r, &req); err != nil {
			bad(w, http.StatusBadRequest, err.Error())
			return
		}
		petID := strings.TrimSpace(req.PetID)
		if petID == "" {
			bad(w, http.StatusBadRequest, "pet_id is required")
			return
		}
		items, _, err := s.loadPetsCatalog()
		if err != nil {
			bad(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !petExists(items, petID) {
			bad(w, http.StatusNotFound, "pet not found")
			return
		}
		if s.PetSwitch != nil {
			if err := s.PetSwitch(petID); err != nil {
				bad(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		if err := s.savePetsState(petsState{ActivePetID: petID}); err != nil {
			bad(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "active_pet_id": petID})
	default:
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func petExists(items []petListItem, id string) bool {
	for _, it := range items {
		if it.ID == id {
			return true
		}
	}
	return false
}

func (s *Server) loadPetsCatalog() ([]petListItem, petsIndexFile, error) {
	var idx petsIndexFile
	data, err := s.readStaticOrPublic("ga-admin-pets/pets.json")
	if err != nil {
		return nil, idx, fmt.Errorf("read pets index: %w", err)
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, idx, err
	}
	items := make([]petListItem, 0, len(idx.Pets))
	for _, p := range idx.Pets {
		id := strings.TrimSpace(p.ID)
		if id == "" {
			continue
		}
		item := petListItem{ID: id, Name: firstNonEmpty(p.Name, id), Manifest: p.Manifest, Spritesheet: p.Spritesheet}
		manifestPath := strings.TrimPrefix(strings.TrimPrefix(p.Manifest, "/"), "./")
		if manifestPath != "" {
			if raw, err := s.readStaticOrPublic(manifestPath); err == nil {
				var mf petManifest
				if json.Unmarshal(raw, &mf) == nil {
					item.Name = firstNonEmpty(mf.DisplayName, item.Name)
					item.Description = mf.Description
					item.AssetBase = mf.AssetBase
					item.Runtime = mf.Runtime
					item.Columns = mf.Atlas.Columns
					item.Rows = mf.Atlas.Rows
					item.FrameWidth = mf.Atlas.FrameWidth
					item.FrameHeight = mf.Atlas.FrameHeight
					if item.Spritesheet == "" && mf.AssetBase != "" && mf.Spritesheet != "" {
						item.Spritesheet = strings.TrimRight(mf.AssetBase, "/") + "/" + mf.Spritesheet
					}
				}
			}
		}
		items = append(items, item)
	}
	return items, idx, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (s *Server) readStaticOrPublic(name string) ([]byte, error) {
	clean, err := cleanStaticAssetName(name)
	if err != nil {
		return nil, err
	}
	if s.Static != nil {
		if data, err := fs.ReadFile(s.Static, clean); err == nil {
			return data, nil
		}
	}
	for _, base := range []string{"web/dist", "web/public"} {
		if data, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(clean))); err == nil {
			return data, nil
		}
	}
	return nil, os.ErrNotExist
}

func cleanStaticAssetName(name string) (string, error) {
	raw := strings.TrimSpace(name)
	if raw == "" {
		return "", os.ErrNotExist
	}
	if strings.Contains(raw, `\`) {
		return "", os.ErrPermission
	}
	slash := filepath.ToSlash(raw)
	fromSlash := filepath.FromSlash(slash)
	if filepath.VolumeName(fromSlash) != "" {
		return "", os.ErrPermission
	}
	for _, seg := range strings.Split(slash, "/") {
		if seg == ".." {
			return "", os.ErrPermission
		}
	}
	clean := path.Clean("/" + slash)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "." || clean == "" {
		return "", os.ErrNotExist
	}
	return clean, nil
}

func (s *Server) petsStatePath() string {
	root := "."
	if s != nil && s.CfgStore != nil && s.CfgStore.Root != "" {
		root = s.CfgStore.Root
	}
	return filepath.Join(root, "pets.local.json")
}

func (s *Server) ActivePetID() string {
	items, _, err := s.loadPetsCatalog()
	if err != nil {
		return ""
	}
	active := strings.TrimSpace(s.loadPetsState().ActivePetID)
	if active != "" && petExists(items, active) {
		return active
	}
	if len(items) > 0 {
		return items[0].ID
	}
	return ""
}

func (s *Server) loadPetsState() petsState {
	var st petsState
	data, err := os.ReadFile(s.petsStatePath())
	if err == nil {
		_ = json.Unmarshal(data, &st)
	}
	return st
}

func (s *Server) savePetsState(st petsState) error {
	if strings.TrimSpace(st.ActivePetID) == "" {
		return errors.New("active pet id is empty")
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.petsStatePath(), append(data, '\n'), 0644)
}
