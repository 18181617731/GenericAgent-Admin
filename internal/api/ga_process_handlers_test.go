package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/modelconfig"
	"genericagent-admin-go/internal/service"
)

func newGAProcessHandlerTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Store{Root: t.TempDir(), Cfg: config.Default()}
	models := modelconfig.NewStore(t.TempDir())
	return New(cfg, service.NewManager(cfg.Cfg.GARoot, cfg.Cfg.BufferLines), models, nil)
}

func TestGAProcessMutationsRejectTrailingJSON(t *testing.T) {
	h := newGAProcessHandlerTestServer(t).Routes()
	for _, path := range []string{
		"/api/ga/processes/kill",
		"/api/ga/processes/adopt",
	} {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"pid":1}{}`))
			markDangerous(req)
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "single JSON value") {
				t.Fatalf("body did not report trailing JSON: %s", rr.Body.String())
			}
		})
	}
}
