package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/modelconfig"
	"genericagent-admin-go/internal/service"
)

func newModelTestServer(t *testing.T, gaRoot string) *Server {
	t.Helper()
	cfg := config.NewStore(t.TempDir())
	cfg.Cfg.GARoot = gaRoot
	models := modelconfig.NewStore(t.TempDir())
	return New(cfg, service.NewManager(cfg.Cfg.GARoot, cfg.Cfg.BufferLines), models, nil)
}

func TestModelsRawAndPreviewMethodContracts(t *testing.T) {
	s := newModelTestServer(t, t.TempDir())
	h := s.Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/raw", strings.NewReader(`{}`))
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed || !strings.Contains(rr.Body.String(), "method not allowed") {
		t.Fatalf("raw POST status=%d want=%d body=%s", rr.Code, http.StatusMethodNotAllowed, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/models/preview", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed || !strings.Contains(rr.Body.String(), "method not allowed") {
		t.Fatalf("preview GET status=%d want=%d body=%s", rr.Code, http.StatusMethodNotAllowed, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/models/preview", strings.NewReader(`{"profiles":[]}`))
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"python"`) {
		t.Fatalf("preview POST status=%d want=200 body=%s", rr.Code, rr.Body.String())
	}
}

func TestModelsRawRequiresDangerousConfirm(t *testing.T) {
	root := t.TempDir()
	writeTestMyKey(t, root, "sk-raw-secret")
	s := newModelTestServer(t, root)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models/raw", nil)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusPreconditionRequired {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusPreconditionRequired, rr.Body.String())
	}
}

func TestModelsSaveRequiresDangerousConfirm(t *testing.T) {
	s := newModelTestServer(t, t.TempDir())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/models", strings.NewReader(`{"profiles":[]}`))
	req.Header.Set("Content-Type", "application/json")
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusPreconditionRequired {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusPreconditionRequired, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/models", strings.NewReader(`{"profiles":[]}`))
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("confirmed status=%d want=200 body=%s", rr.Code, rr.Body.String())
	}
}

func TestModelsRawWithDangerousConfirmReturnsUnmaskedSecret(t *testing.T) {
	root := t.TempDir()
	s := newModelTestServer(t, root)
	if _, err := s.Models.Save([]modelconfig.Profile{{
		VarName: "api_config_main",
		Type:    "openai",
		Name:    "main",
		APIBase: "https://api.example/v1",
		Model:   "gpt-test",
		APIKey:  "sk-raw-secret",
	}}); err != nil {
		t.Fatalf("seed Save() error = %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models/raw", nil)
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=200 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "sk-raw-secret") {
		t.Fatalf("raw response did not include unmasked secret: %s", rr.Body.String())
	}
}

func TestModelsImportMyKeyRevealRequiresDangerousConfirm(t *testing.T) {
	gaRoot := t.TempDir()
	writeTestMyKey(t, gaRoot, "sk-test-secret-value")
	s := newModelTestServer(t, gaRoot)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/import-mykey", strings.NewReader(`{"reveal":true,"save":false}`))
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusPreconditionRequired {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusPreconditionRequired, rr.Body.String())
	}
}

func TestModelsImportMyKeyRevealWithDangerousConfirm(t *testing.T) {
	gaRoot := t.TempDir()
	writeTestMyKey(t, gaRoot, "sk-test-secret-value")
	s := newModelTestServer(t, gaRoot)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/import-mykey", strings.NewReader(`{"reveal":true,"save":false}`))
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=200 body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "sk-test-secret-value") || strings.Contains(body, `"masked":true`) {
		t.Fatalf("reveal response did not include unmasked secret metadata: %s", body)
	}
}

func TestModelsImportMyKeyMasksByDefaultAndDoesNotSave(t *testing.T) {
	gaRoot := t.TempDir()
	writeTestMyKey(t, gaRoot, "sk-test-secret-value")
	s := newModelTestServer(t, gaRoot)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/import-mykey", strings.NewReader(`{"reveal":false,"save":false}`))
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "sk-test-secret-value") || !strings.Contains(body, `"masked":true`) || !strings.Contains(body, "sk-****alue") {
		t.Fatalf("masked import leaked or missing mask metadata: %s", body)
	}
}

func TestModelsImportMyKeyRefusesToSaveMaskedProfiles(t *testing.T) {
	gaRoot := t.TempDir()
	writeTestMyKey(t, gaRoot, "sk-test-secret-value")
	s := newModelTestServer(t, gaRoot)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/import-mykey", strings.NewReader(`{"reveal":false,"save":true}`))
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusPreconditionRequired {
		t.Fatalf("status=%d want=428 body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/models/import-mykey", strings.NewReader(`{"reveal":false,"save":true}`))
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=400 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "refusing to save masked") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestModelsExportRejectsMaskedAPIKey(t *testing.T) {
	s := newModelTestServer(t, t.TempDir())
	payload := map[string]interface{}{
		"overwrite_active": false,
		"profiles":         []modelconfig.Profile{{VarName: "api_config_main", Type: "openai", Name: "main", APIBase: "https://api.example/v1", Model: "gpt", APIKey: "sk-****alue"}},
	}
	data, _ := json.Marshal(payload)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/export", bytes.NewReader(data))
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=400 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "masked apikey") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestModelsExportRequiresDangerousConfirm(t *testing.T) {
	s := newModelTestServer(t, t.TempDir())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/export", strings.NewReader(`{"profiles":[]}`))
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusPreconditionRequired {
		t.Fatalf("status=%d want=428 body=%s", rr.Code, rr.Body.String())
	}
}

func TestModelsExportPreservesExistingSecretWhenSubmittedBlank(t *testing.T) {
	root := t.TempDir()
	s := newModelTestServer(t, root)
	seed := []modelconfig.Profile{{
		VarName: "api_config_main",
		Type:    "openai",
		Name:    "main",
		APIBase: "https://api.example/v1",
		Model:   "gpt-test",
		APIKey:  "sk-real-secret",
	}}
	if _, err := s.Models.Save(seed); err != nil {
		t.Fatalf("seed Save() error = %v", err)
	}
	payload := map[string]interface{}{
		"overwrite_active": true,
		"profiles": []modelconfig.Profile{{
			VarName: "api_config_main",
			Type:    "openai",
			Name:    "main",
			APIBase: "https://api.example/v1",
			Model:   "gpt-updated",
			APIKey:  "",
		}},
	}
	body, _ := json.Marshal(payload)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/export", bytes.NewReader(body))
	s.modelsExport(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	active, err := os.ReadFile(filepath.Join(root, "mykey.py"))
	if err != nil {
		t.Fatalf("read mykey.py: %v", err)
	}
	if text := string(active); !strings.Contains(text, "sk-real-secret") || strings.Contains(text, "\"apikey\": \"\"") {
		t.Fatalf("active mykey.py did not preserve secret:\n%s", text)
	}
	raw, err := s.Models.Load(true)
	if err != nil {
		t.Fatalf("Load(true) error = %v", err)
	}
	if got := raw.Profiles[0].APIKey; got != "sk-real-secret" {
		t.Fatalf("saved APIKey = %q, want preserved secret", got)
	}
}

func writeTestMyKey(t *testing.T, root, key string) {
	t.Helper()
	text := "api_config_main = {\n" +
		"    'name': 'main',\n" +
		"    'apibase': 'https://api.example/v1',\n" +
		"    'model': 'gpt-test',\n" +
		"    'apikey': '" + key + "',\n" +
		"}\n"
	if err := os.WriteFile(filepath.Join(root, "mykey.py"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}
