package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"genericagent-admin-go/internal/hatchpet"
)

func TestHatchPetStatusUsesConfiguredGARootAndReportsMemory(t *testing.T) {
	gaRoot := t.TempDir()
	h := newServiceHandlerTestServer(t, gaRoot).Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/hatch-pet/status", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var body struct {
		SkillName        string                 `json:"skill_name"`
		DefaultExportDir string                 `json:"default_export_dir"`
		ExportPath       string                 `json:"export_path"`
		Exported         bool                   `json:"exported"`
		Files            int                    `json:"files"`
		Missing          []string               `json:"missing"`
		Memory           *hatchpet.MemoryStatus `json:"memory"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode status response: %v body=%s", err, rr.Body.String())
	}

	expectedExportDir := filepath.Join(gaRoot, "tools", hatchpet.SkillName)
	if body.SkillName != hatchpet.SkillName {
		t.Fatalf("skill_name=%q want %q", body.SkillName, hatchpet.SkillName)
	}
	if body.DefaultExportDir != expectedExportDir || body.ExportPath != expectedExportDir {
		t.Fatalf("export dir/path = %q/%q want %q", body.DefaultExportDir, body.ExportPath, expectedExportDir)
	}
	if body.Exported || body.Files == 0 || len(body.Missing) == 0 {
		t.Fatalf("fresh temp root should report missing embedded export files: %+v", body)
	}
	if body.Memory == nil {
		t.Fatalf("status response omitted memory summary for configured GA root: %+v", body)
	}
	if body.Memory.GARoot != filepath.Clean(gaRoot) {
		t.Fatalf("memory ga_root=%q want %q", body.Memory.GARoot, filepath.Clean(gaRoot))
	}
	if body.Memory.MemoryDir != filepath.Join(gaRoot, "memory") {
		t.Fatalf("memory_dir=%q want %q", body.Memory.MemoryDir, filepath.Join(gaRoot, "memory"))
	}
	if body.Memory.Installed || body.Memory.Files == 0 || len(body.Memory.Missing) == 0 {
		t.Fatalf("fresh temp root should report missing memory SOPs without installing them: %+v", body.Memory)
	}
}

func TestHatchPetStatusRejectsNonGET(t *testing.T) {
	h := newServiceHandlerTestServer(t, t.TempDir()).Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/hatch-pet/status", strings.NewReader(`{}`))
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed || !strings.Contains(rr.Body.String(), "method not allowed") {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusMethodNotAllowed, rr.Body.String())
	}
}
