package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"genericagent-admin-go/internal/config"
)

func newPetsTestServer(t *testing.T) *Server {
	t.Helper()
	root := t.TempDir()
	return New(&config.Store{Root: root, Cfg: config.Default()}, nil, nil, fstest.MapFS{
		"ga-admin-pets/pets.json": &fstest.MapFile{Data: []byte(`{
			"runtime":"ga-admin-pets/v1",
			"description":"test pets",
			"pets":[
				{"id":"alpha","name":"Alpha","manifest":"ga-admin-pets/alpha/manifest.json"},
				{"id":"beta","name":"Beta","manifest":"ga-admin-pets/beta/manifest.json"}
			]
		}`)},
		"ga-admin-pets/alpha/manifest.json": &fstest.MapFile{Data: []byte(`{
			"display_name":"Alpha Manifest",
			"description":"alpha pet",
			"asset_base":"ga-admin-pets/alpha",
			"spritesheet":"sprite.png",
			"atlas":{"columns":4,"rows":9,"frame_width":128,"frame_height":128}
		}`)},
		"ga-admin-pets/beta/manifest.json": &fstest.MapFile{Data: []byte(`{
			"display_name":"Beta Manifest",
			"description":"beta pet",
			"asset_base":"ga-admin-pets/beta",
			"spritesheet":"sprite.png",
			"atlas":{"columns":4,"rows":9,"frame_width":128,"frame_height":128}
		}`)},
	})
}

func TestPetsActiveFallsBackToFirstCatalogPet(t *testing.T) {
	s := newPetsTestServer(t)
	if got := s.ActivePetID(); got != "alpha" {
		t.Fatalf("ActivePetID=%q want alpha", got)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/pets/active", nil)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		ActivePetID string `json:"active_pet_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response json: %v", err)
	}
	if got.ActivePetID != "alpha" {
		t.Fatalf("active_pet_id=%q want alpha", got.ActivePetID)
	}
}

func TestPetsActivePostPersistsAndSwitches(t *testing.T) {
	s := newPetsTestServer(t)
	var switched []string
	s.PetSwitch = func(id string) error {
		switched = append(switched, id)
		return nil
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/pets/active", strings.NewReader(`{"pet_id":"beta"}`))
	req.Header.Set("X-GA-Confirm", "dangerous")
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}
	if got := s.ActivePetID(); got != "beta" {
		t.Fatalf("ActivePetID after post=%q want beta", got)
	}
	if len(switched) != 1 || switched[0] != "beta" {
		t.Fatalf("switched=%v want [beta]", switched)
	}

	data, err := os.ReadFile(filepath.Join(s.CfgStore.Root, "pets.local.json"))
	if err != nil {
		t.Fatalf("read pets.local.json: %v", err)
	}
	var st petsState
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("state json: %v", err)
	}
	if st.ActivePetID != "beta" {
		t.Fatalf("persisted active=%q want beta", st.ActivePetID)
	}
}

func TestPetsActivePostRejectsUnknownPet(t *testing.T) {
	s := newPetsTestServer(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/pets/active", strings.NewReader(`{"pet_id":"missing"}`))
	req.Header.Set("X-GA-Confirm", "dangerous")
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404 body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(s.CfgStore.Root, "pets.local.json")); !os.IsNotExist(err) {
		t.Fatalf("pets.local.json err=%v want not exist", err)
	}
}

func TestPetsActivePostRejectsTrailingJSONAndOversizedBody(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantDetail string
	}{
		{
			name:       "trailing json",
			body:       `{"pet_id":"beta"} {"pet_id":"alpha"}`,
			wantStatus: http.StatusBadRequest,
			wantDetail: "single JSON value",
		},
		{
			name:       "oversized body",
			body:       `{"pet_id":"` + strings.Repeat("x", maxJSONBodyBytes) + `"}`,
			wantStatus: http.StatusRequestEntityTooLarge,
			wantDetail: "request body too large",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newPetsTestServer(t)
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/pets/active", strings.NewReader(tc.body))
			req.Header.Set("X-GA-Confirm", "dangerous")
			s.Routes().ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), tc.wantDetail) {
				t.Fatalf("body missing %q: %s", tc.wantDetail, rr.Body.String())
			}
			if _, err := os.Stat(filepath.Join(s.CfgStore.Root, "pets.local.json")); !os.IsNotExist(err) {
				t.Fatalf("pets.local.json err=%v want not exist", err)
			}
		})
	}
}

func TestPetsMethodContracts(t *testing.T) {
	tests := []struct {
		method     string
		path       string
		body       string
		confirm    bool
		wantStatus int
	}{
		{http.MethodPost, "/api/pets", `{"pet_id":"alpha"}`, true, http.StatusMethodNotAllowed},
		{http.MethodPut, "/api/pets/active", `{"pet_id":"alpha"}`, true, http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/pets/active", `{"pet_id":"alpha"}`, false, http.StatusPreconditionRequired},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			s := newPetsTestServer(t)
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.confirm {
				req.Header.Set("X-GA-Confirm", "dangerous")
			}
			s.Routes().ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.wantStatus, rr.Body.String())
			}
		})
	}
}

func TestReadStaticOrPublicRejectsTraversal(t *testing.T) {
	s := &Server{Static: fstest.MapFS{
		"ga-admin-pets/pets.json": &fstest.MapFile{Data: []byte("pets")},
		"secret.json":             &fstest.MapFile{Data: []byte("secret")},
	}}
	if data, err := s.readStaticOrPublic("ga-admin-pets/pets.json"); err != nil || string(data) != "pets" {
		t.Fatalf("read catalog data=%q err=%v", string(data), err)
	}
	for _, name := range []string{
		"../secret.json",
		"ga-admin-pets/../secret.json",
		`ga-admin-pets\..\secret.json`,
		`ga-admin-pets\pets.json`,
		`C:/Windows/win.ini`,
		`C:\Windows\win.ini`,
		"/../secret.json",
	} {
		t.Run(name, func(t *testing.T) {
			data, err := s.readStaticOrPublic(name)
			if err == nil {
				t.Fatalf("readStaticOrPublic(%q)=%q, want error", name, string(data))
			}
		})
	}
}
