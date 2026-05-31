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
