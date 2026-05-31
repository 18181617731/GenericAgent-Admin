package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDangerousConfirmWrapperRejectsMissingHeader(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	for _, tc := range dangerousConfirmRouteCases() {
		t.Run(tc.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusPreconditionRequired {
				t.Fatalf("%s %s status=%d want=%d body=%s", tc.method, tc.path, rr.Code, http.StatusPreconditionRequired, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "X-GA-Confirm") {
				t.Fatalf("%s %s missing confirm guidance in body: %s", tc.method, tc.path, rr.Body.String())
			}
		})
	}
}

func TestDangerousConfirmWrapperAllowsConfirmedRequestsToReachValidation(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	for _, tc := range safeValidationDangerousConfirmRouteCases() {
		t.Run(tc.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			markDangerous(req)
			h.ServeHTTP(rr, req)
			if rr.Code == http.StatusPreconditionRequired {
				t.Fatalf("%s %s confirmed request was still blocked by confirm wrapper: body=%s", tc.method, tc.path, rr.Body.String())
			}
			if rr.Code == http.StatusOK {
				t.Fatalf("%s %s validation payload unexpectedly succeeded; test should not perform side effects", tc.method, tc.path)
			}
		})
	}
}

type dangerousConfirmRouteCase struct {
	method string
	path   string
	body   string
}

func dangerousConfirmRouteCases() []dangerousConfirmRouteCase {
	return []dangerousConfirmRouteCase{
		{http.MethodPost, "/api/version/update", `{}`},
		{http.MethodPost, "/api/ga/git-update", `{}`},
		{http.MethodPost, "/api/tmwebdriver/repair", `{}`},
		{http.MethodPost, "/api/bbs/config", `{}`},
		{http.MethodPost, "/api/bbs/posts", `{}`},
		{http.MethodPost, "/api/bbs/reply", `{}`},
		{http.MethodPost, "/api/files/write", `{}`},
		{http.MethodPost, "/api/schedule/task", `{}`},
		{http.MethodPost, "/api/schedule/create", `{}`},
		{http.MethodPost, "/api/schedule/delete", `{}`},
		{http.MethodPost, "/api/schedule/toggle", `{}`},
		{http.MethodPost, "/api/goals/start", `{}`},
		{http.MethodPost, "/api/goals/stop", `{}`},
		{http.MethodPost, "/api/goals/delete", `{}`},
		{http.MethodPut, "/api/config", `not-json`},
		{http.MethodPost, "/api/setup/install", `{}`},
		{http.MethodPost, "/api/autostart/enable", `{}`},
		{http.MethodPost, "/api/autostart/disable", `{}`},
		{http.MethodPost, "/api/services/start", `{}`},
		{http.MethodPost, "/api/services/stop", `{}`},
		{http.MethodPost, "/api/services/stop-all", `{}`},
		{http.MethodPost, "/api/services/autostart", `{}`},
		{http.MethodPost, "/api/models/export", `{}`},
	}
}

func safeValidationDangerousConfirmRouteCases() []dangerousConfirmRouteCase {
	return []dangerousConfirmRouteCase{
		{http.MethodPost, "/api/files/write", `{}`},
		{http.MethodPost, "/api/schedule/delete", `{}`},
		{http.MethodPost, "/api/schedule/toggle", `{}`},
		{http.MethodPost, "/api/goals/start", `{}`},
		{http.MethodPost, "/api/goals/stop", `{}`},
		{http.MethodPost, "/api/goals/delete", `{}`},
		{http.MethodPut, "/api/config", `not-json`},
	}
}
