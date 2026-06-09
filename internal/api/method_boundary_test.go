package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusEndpointsRejectNonGET(t *testing.T) {
	h := newConfigTestServer(t).Routes()
	for _, path := range []string{
		"/api/version/status",
		"/api/hatch-pet/status",
		"/api/autostart/status",
	} {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, path, nil)
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status=%d want=405 body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}
