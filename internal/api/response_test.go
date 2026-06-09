package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSONErrorResponsesUseDetailField(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	for _, tc := range []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantDetail string
	}{
		{name: "method not allowed", method: http.MethodPost, path: "/api/version/info", wantStatus: http.StatusMethodNotAllowed, wantDetail: "method not allowed"},
		{name: "health rejects non-get", method: http.MethodPost, path: "/api/health", wantStatus: http.StatusMethodNotAllowed, wantDetail: "method not allowed"},
		{name: "ga inventory rejects non-get", method: http.MethodPost, path: "/api/ga/inventory", wantStatus: http.StatusMethodNotAllowed, wantDetail: "method not allowed"},
		{name: "ga health rejects non-get", method: http.MethodPost, path: "/api/ga/health", wantStatus: http.StatusMethodNotAllowed, wantDetail: "method not allowed"},
		{name: "ga control rejects non-get", method: http.MethodPost, path: "/api/ga/control", wantStatus: http.StatusMethodNotAllowed, wantDetail: "method not allowed"},
		{name: "bad json", method: http.MethodPost, path: "/api/files/write", body: `not-json`, wantStatus: http.StatusPreconditionRequired, wantDetail: "dangerous operation requires X-GA-Confirm"},
		{name: "bad query", method: http.MethodGet, path: "/api/files/tail?path=sample.log&lines=0", wantStatus: http.StatusBadRequest, wantDetail: "lines must be a positive integer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			h.ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
				t.Fatalf("Content-Type=%q want application/json", ct)
			}
			var got struct {
				Detail string `json:"detail"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
				t.Fatalf("error body is not JSON: %v body=%s", err, rr.Body.String())
			}
			if !strings.Contains(got.Detail, tc.wantDetail) {
				t.Fatalf("detail=%q want contains %q", got.Detail, tc.wantDetail)
			}
		})
	}
}

func TestRecoverPanicsReturnsJSONInternalServerError(t *testing.T) {
	h := recoverPanics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("secret panic detail")
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/panic", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusInternalServerError, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type=%q want application/json", ct)
	}
	var got struct {
		OK     bool   `json:"ok"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("panic recovery body is not JSON: %v body=%s", err, rr.Body.String())
	}
	if got.OK || got.Detail != "internal server error" {
		t.Fatalf("unexpected recovery payload: %#v", got)
	}
	if strings.Contains(rr.Body.String(), "secret panic detail") {
		t.Fatalf("panic detail leaked in response: %s", rr.Body.String())
	}
}

func TestDecodeRejectsOversizedJSONBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"payload":"`+strings.Repeat("x", maxJSONBodyBytes)+`"}`))
	var got map[string]string
	if err := decode(req, &got); !errors.Is(err, errRequestBodyTooLarge) {
		t.Fatalf("decode oversized error=%v want %v", err, errRequestBodyTooLarge)
	}
}

func TestDecodeLimitedAllowsBodiesAboveDefaultCap(t *testing.T) {
	// A payload larger than the default cap (e.g. a base64 image upload) must be
	// accepted when the endpoint raises the limit, otherwise sending images fails.
	payload := strings.Repeat("x", maxJSONBodyBytes*2)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"payload":"`+payload+`"}`))
	var got map[string]string
	if err := decodeLimited(req, &got, maxChatPostBodyBytes); err != nil {
		t.Fatalf("decodeLimited large body err=%v want nil", err)
	}
	if got["payload"] != payload {
		t.Fatalf("decodeLimited did not preserve payload (len=%d)", len(got["payload"]))
	}
}

func TestDecodeLimitedStillRejectsBeyondLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"payload":"`+strings.Repeat("x", maxJSONBodyBytes)+`"}`))
	var got map[string]string
	if err := decodeLimited(req, &got, maxJSONBodyBytes); !errors.Is(err, errRequestBodyTooLarge) {
		t.Fatalf("decodeLimited oversized error=%v want %v", err, errRequestBodyTooLarge)
	}
}

func TestDecodeKeepsSingleJSONValueValidation(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"ok":true} {"extra":true}`))
	var got map[string]bool
	if err := decode(req, &got); err == nil || !strings.Contains(err.Error(), "single JSON value") {
		t.Fatalf("decode multi-value error=%v", err)
	}
}

type closeErrorBody struct {
	io.Reader
}

func (closeErrorBody) Close() error {
	return errors.New("close boom")
}

func TestDecodeReportsBodyCloseError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Body = closeErrorBody{Reader: strings.NewReader(`{"ok":true}`)}
	var got map[string]bool
	if err := decode(req, &got); err == nil || !strings.Contains(err.Error(), "close boom") {
		t.Fatalf("decode close error=%v", err)
	}
	if !got["ok"] {
		t.Fatalf("decode did not populate value before close error: %#v", got)
	}
}

func TestOversizedJSONRouteReturns413(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	rr := httptest.NewRecorder()
	body := `{"title":"` + strings.Repeat("x", maxJSONBodyBytes) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/files/write", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GA-Confirm", "dangerous")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusRequestEntityTooLarge, rr.Body.String())
	}
	var got struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("error body is not JSON: %v body=%s", err, rr.Body.String())
	}
	if !strings.Contains(got.Detail, errRequestBodyTooLarge.Error()) {
		t.Fatalf("detail=%q want contains %q", got.Detail, errRequestBodyTooLarge.Error())
	}
}

func TestJSONRoutesRejectTrailingJSONValues(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
		mark   bool
	}{
		{name: "channels put", method: http.MethodPut, path: "/api/channels", body: `{"profiles":[]} {"extra":true}`, mark: true},
		{name: "channels test", method: http.MethodPost, path: "/api/channels/test", body: `{"profile_id":"feishu"} {"extra":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.mark {
				markDangerous(req)
			}
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), "single JSON value") {
				t.Fatalf("body missing single JSON value guidance: %s", rr.Body.String())
			}
		})
	}
}

func TestChannelsRejectOversizedJSONBody(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/channels/test", strings.NewReader(`{"profile_id":"`+strings.Repeat("x", maxJSONBodyBytes)+`"}`))
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusRequestEntityTooLarge, rr.Body.String())
	}
}
