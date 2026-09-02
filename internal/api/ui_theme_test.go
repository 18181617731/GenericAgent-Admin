package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUIThemeGetReturnsDefaultWhenUnset(t *testing.T) {
	s := newConfigTestServer(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ui/theme", nil)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["theme"] != "warm" {
		t.Fatalf("theme=%q want warm", got["theme"])
	}
}

func TestUIThemePutRequiresDangerousHeader(t *testing.T) {
	s := newConfigTestServer(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/ui/theme", bytes.NewReader([]byte(`{"theme":"dark"}`)))
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != 428 {
		t.Fatalf("status=%d want 428 body=%s", rr.Code, rr.Body.String())
	}
}

func TestUIThemePutPersistsCanonicalTheme(t *testing.T) {
	s := newConfigTestServer(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/ui/theme", bytes.NewReader([]byte(`{"theme":"dark"}`)))
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["theme"] != "dark" {
		t.Fatalf("theme=%q want dark", got["theme"])
	}
	if snap := s.CfgStore.Snapshot().UITheme; snap != "dark" {
		t.Fatalf("stored ui_theme=%q want dark", snap)
	}

	get := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/ui/theme", nil)
	s.Routes().ServeHTTP(get, getReq)
	if get.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", get.Code, get.Body.String())
	}
	got = map[string]string{}
	if err := json.Unmarshal(get.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["theme"] != "dark" {
		t.Fatalf("GET theme=%q want dark", got["theme"])
	}
}

func TestUIThemePutRejectsUnknownTheme(t *testing.T) {
	s := newConfigTestServer(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/ui/theme", bytes.NewReader([]byte(`{"theme":"not-a-theme"}`)))
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
	if snap := s.CfgStore.Snapshot().UITheme; snap == "not-a-theme" {
		t.Fatal("rejected theme was persisted")
	}
}
