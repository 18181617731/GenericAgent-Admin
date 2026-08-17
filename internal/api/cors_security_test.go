package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutesRejectCrossOriginRequests(t *testing.T) {
	h := SameOriginGuard(newGoalTestServer(t, t.TempDir()).Routes())
	req := httptest.NewRequest(http.MethodGet, "/api/version/info", nil)
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
	if strings.Contains(rr.Header().Get("Access-Control-Allow-Origin"), "*") {
		t.Fatal("cross-origin response must not opt into wildcard CORS")
	}
}

func TestRoutesAllowSameOriginRequestsWithoutWildcardCORS(t *testing.T) {
	h := SameOriginGuard(newGoalTestServer(t, t.TempDir()).Routes())
	req := httptest.NewRequest(http.MethodGet, "/api/version/info", nil)
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Origin", "http://127.0.0.1:8787")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("same-origin response must not advertise CORS, got %q", got)
	}
}

func TestRoutesRejectCrossSiteFetchEvenWhenOriginLooksLocal(t *testing.T) {
	h := SameOriginGuard(newGoalTestServer(t, t.TempDir()).Routes())
	req := httptest.NewRequest(http.MethodOptions, "/api/files/write", nil)
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Origin", "http://127.0.0.1:8787")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}
