package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChannelsGetReadsMyKeyAndMasksSecrets(t *testing.T) {
	root := t.TempDir()
	writeTestChannelsMyKey(t, root, "old-secret")
	h := newGoalTestServer(t, root).Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/channels status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp channelsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode channels response: %v", err)
	}
	if resp.Path != filepath.Join(root, "mykey.py") {
		t.Fatalf("path=%q want mykey.py under temp root", resp.Path)
	}
	if !resp.Exists {
		t.Fatalf("exists=false for temp mykey.py")
	}
	appID := findChannelField(t, resp.Profiles, "fs_app_id")
	if appID.Value != "cli-old" || !appID.HasValue {
		t.Fatalf("fs_app_id value=%q has=%v", appID.Value, appID.HasValue)
	}
	secret := findChannelField(t, resp.Profiles, "fs_app_secret")
	if secret.Value != "" || !secret.HasValue {
		t.Fatalf("secret should be masked but marked present; value=%q has=%v", secret.Value, secret.HasValue)
	}
}

func TestChannelsGetIgnoresUnsafeGARoot(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "mykey.py"), []byte(`fs_app_id = "should-not-read"\n`), 0600); err != nil {
		t.Fatal(err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldwd) }()
	h := newGoalTestServer(t, ".").Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/channels status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp channelsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode channels response: %v", err)
	}
	if resp.Path != "" || resp.Exists {
		t.Fatalf("unsafe root should not expose cwd mykey.py; path=%q exists=%v", resp.Path, resp.Exists)
	}
	if got := findChannelField(t, resp.Profiles, "fs_app_id"); got.Value != "" || got.HasValue {
		t.Fatalf("unsafe root read cwd mykey.py: value=%q has=%v", got.Value, got.HasValue)
	}
}

func TestChannelsPutRequiresDangerousConfirm(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/channels", bytes.NewReader([]byte(`{"profiles":[]}`)))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusPreconditionRequired {
		t.Fatalf("PUT /api/channels without confirm status=%d want=%d body=%s", rr.Code, http.StatusPreconditionRequired, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "X-GA-Confirm") {
		t.Fatalf("missing confirm guidance in body: %s", rr.Body.String())
	}
}

func TestChannelsPutBlankSecretPreservesExistingMyKeyValue(t *testing.T) {
	root := t.TempDir()
	writeTestChannelsMyKey(t, root, "old-secret")
	h := newGoalTestServer(t, root).Routes()
	body := []byte(`{
		"profiles": [{"id":"feishu","fields": [
			{"name":"fs_app_id","value":"cli-new"},
			{"name":"fs_app_secret","value":""},
			{"name":"fs_allowed_users","value":"bob, alice"},
			{"name":"fs_public_access","value":"true"}
		]}]
	}`)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/channels", bytes.NewReader(body))
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT /api/channels status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	updatedBytes, err := os.ReadFile(filepath.Join(root, "mykey.py"))
	if err != nil {
		t.Fatalf("read temp mykey.py: %v", err)
	}
	updated := string(updatedBytes)
	for _, want := range []string{
		`fs_app_id = "cli-new"`,
		`fs_allowed_users = ["alice","bob"]`,
		`fs_public_access = True`,
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated mykey.py missing %q:\n%s", want, updated)
		}
	}
	if !strings.Contains(updated, `fs_app_secret = "old-secret"`) {
		t.Fatalf("blank secret did not preserve existing secret:\n%s", updated)
	}
	if strings.Contains(updated, "mykey_admin_channels") {
		t.Fatalf("unexpected legacy overlay reference in mykey.py:\n%s", updated)
	}
}

// TestChannelsDocStyleMyKeyRoundTrip covers the exact mykey.py layout recommended
// by docs/SETUP_FEISHU.md: multi-line allowlists plus inline `#` comments. The old
// single-line parser corrupted scalar values and left dangling list lines that
// turned mykey.py into invalid Python on save.
func TestChannelsRejectUnsafeGARootOnSave(t *testing.T) {
	for _, root := range []string{"", ".", string(filepath.Separator)} {
		t.Run(root, func(t *testing.T) {
			h := newGoalTestServer(t, root).Routes()
			req := httptest.NewRequest(http.MethodPut, "/api/channels", strings.NewReader(`{"profiles":[]}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GA-Confirm", "dangerous")
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("root=%q code=%d body=%s", root, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "mykey.py") {
				t.Fatalf("unsafe root leaked writable target: %s", rec.Body.String())
			}
		})
	}
}

func TestChannelsDocStyleMyKeyRoundTrip(t *testing.T) {
	root := t.TempDir()
	text := strings.Join([]string{
		`# 飞书应用凭证`,
		`fs_app_id = "cli_realappid"      # 替换为你的 App ID`,
		`fs_app_secret = "real-secret"       # 替换为你的 App Secret`,
		`fs_allowed_users = [`,
		`    "ou_aaaa",       # 你的 Open ID`,
		`    "ou_bbbb",`,
		`]`,
		`fs_public_access = True  # 是否公开`,
		``,
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "mykey.py"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()

	// GET must parse the doc-style values without dragging in comments.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp channelsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := findChannelField(t, resp.Profiles, "fs_app_id").Value; got != "cli_realappid" {
		t.Fatalf("fs_app_id parsed=%q want cli_realappid (comment leaked?)", got)
	}
	if got := findChannelField(t, resp.Profiles, "fs_allowed_users").Value; got != "ou_aaaa,ou_bbbb" {
		t.Fatalf("fs_allowed_users parsed=%q want ou_aaaa,ou_bbbb", got)
	}
	if got := findChannelField(t, resp.Profiles, "fs_public_access").Value; got != "true" {
		t.Fatalf("fs_public_access parsed=%q want true", got)
	}

	// PUT (blank secret) must keep the real secret and rewrite the multi-line list
	// as a single valid line, leaving no dangling `"ou_..."` rows behind.
	body := []byte(`{"profiles":[{"id":"feishu","fields":[
		{"name":"fs_app_id","value":"cli_realappid"},
		{"name":"fs_app_secret","value":""},
		{"name":"fs_allowed_users","value":"ou_aaaa,ou_bbbb"},
		{"name":"fs_public_access","value":"true"}
	]}]}`)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/channels", bytes.NewReader(body))
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", rr.Code, rr.Body.String())
	}
	updatedBytes, err := os.ReadFile(filepath.Join(root, "mykey.py"))
	if err != nil {
		t.Fatal(err)
	}
	updated := string(updatedBytes)
	for _, want := range []string{
		`fs_app_id = "cli_realappid"`,
		`fs_app_secret = "real-secret"`,
		`fs_allowed_users = ["ou_aaaa","ou_bbbb"]`,
		`fs_public_access = True`,
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated mykey.py missing %q:\n%s", want, updated)
		}
	}
	// No corruption: no comment leakage into values, no dangling list rows.
	for _, bad := range []string{
		"替换为你的 App ID",
		`fs_app_id = "cli_realappid\"`,
		"\n    \"ou_aaaa\"",
		"[\"[\"]",
	} {
		if strings.Contains(updated, bad) {
			t.Fatalf("updated mykey.py contains corruption %q:\n%s", bad, updated)
		}
	}
	// Result must be valid Python: every `fs_allowed_users` is a single closed list.
	if strings.Count(updated, "fs_allowed_users = [") != 1 || strings.Count(updated, "fs_allowed_users =") != 1 {
		t.Fatalf("fs_allowed_users not a single clean assignment:\n%s", updated)
	}
}

func TestChannelsPutRejectsMalformedBoolean(t *testing.T) {
	root := t.TempDir()
	writeTestChannelsMyKey(t, root, "old-secret")
	h := newGoalTestServer(t, root).Routes()
	body := []byte(`{"profiles":[{"id":"feishu","fields":[
		{"name":"fs_app_id","value":"cli-new"},
		{"name":"fs_app_secret","value":""},
		{"name":"fs_public_access","value":"definitely"}
	]}]}`)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/channels", bytes.NewReader(body))
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("PUT malformed bool status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "fs_public_access") || !strings.Contains(rr.Body.String(), "invalid boolean") {
		t.Fatalf("response should name malformed boolean field: %s", rr.Body.String())
	}

	updated, err := os.ReadFile(filepath.Join(root, "mykey.py"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(updated), `fs_app_id = "cli-new"`) || strings.Contains(string(updated), "definitely") {
		t.Fatalf("invalid boolean write partially mutated mykey.py:\n%s", string(updated))
	}
}

func TestChannelTestEndpointRejectsMalformedBoolean(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	body := []byte(`{"profile_id":"feishu","fields":[
		{"name":"fs_app_id","value":"cli"},
		{"name":"fs_app_secret","value":"secret"},
		{"name":"fs_public_access","value":"maybe"}
	]}`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/channels/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d want=%d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "fs_public_access") || !strings.Contains(rec.Body.String(), "invalid boolean") {
		t.Fatalf("response should name malformed boolean field: %s", rec.Body.String())
	}
}

func writeTestChannelsMyKey(t *testing.T, root, secret string) {
	t.Helper()
	text := strings.Join([]string{
		`# existing config should be preserved`,
		`api_config_main = {"name": "main"}`,
		`fs_app_id = "cli-old"`,
		`fs_app_secret = "` + secret + `"`,
		`fs_allowed_users = ["zara"]`,
		`fs_public_access = False`,
		``,
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "mykey.py"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func findChannelField(t *testing.T, profiles []channelProfile, name string) channelField {
	t.Helper()
	for _, p := range profiles {
		for _, f := range p.Fields {
			if f.Name == name {
				return f
			}
		}
	}
	t.Fatalf("field %s not found", name)
	return channelField{}
}

func TestChannelTestRequestJSONRejectsUnsafeUpstreamBodies(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "malformed", body: `{"code":`, want: "invalid channel test JSON response"},
		{name: "trailing", body: `{"code":0}{"code":1}`, want: "single JSON value"},
		{name: "oversized", body: strings.Repeat("x", maxChannelTestResponseBytes+1), want: "too large"},
	}
	oldClient := channelTestHTTPClient
	defer func() { channelTestHTTPClient = oldClient }()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer ts.Close()
			channelTestHTTPClient = ts.Client()
			_, _, err := channelTestRequestJSON(http.MethodGet, ts.URL, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want substring %q", err, tc.want)
			}
		})
	}
}

func TestChannelTestEndpointUsesSavedSecret(t *testing.T) {
	root := t.TempDir()
	writeTestChannelsMyKey(t, root, "saved-secret")

	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["app_id"] != "cli-new" || body["app_secret"] != "saved-secret" {
			t.Fatalf("body=%v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"ok"}`))
	}))
	defer ts.Close()
	oldEndpoints := channelTestEndpoints
	oldClient := channelTestHTTPClient
	channelTestEndpoints.Feishu = ts.URL
	channelTestHTTPClient = ts.Client()
	defer func() { channelTestEndpoints = oldEndpoints; channelTestHTTPClient = oldClient }()

	h := newGoalTestServer(t, root).Routes()
	body := `{"profile_id":"feishu","fields":[{"name":"fs_app_id","value":"cli-new"},{"name":"fs_app_secret","value":""}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/channels/test", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp channelTestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !called || !resp.OK || !strings.Contains(resp.Message, "飞书") {
		t.Fatalf("called=%v resp=%+v", called, resp)
	}
}

func TestChannelTestMessagesRedactSecrets(t *testing.T) {
	oldEndpoints := channelTestEndpoints
	oldClient := channelTestHTTPClient
	defer func() { channelTestEndpoints = oldEndpoints; channelTestHTTPClient = oldClient }()

	secret := "super-secret-value-12345"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"upstream echoed super-secret-value-12345"}`))
	}))
	defer ts.Close()
	channelTestEndpoints.Feishu = ts.URL
	channelTestHTTPClient = ts.Client()

	ok, msg := testFeishuCredentials("cli-id", secret)
	if ok || strings.Contains(msg, secret) || !strings.Contains(msg, "[REDACTED]") {
		t.Fatalf("ok=%v msg=%q", ok, msg)
	}
}

func TestWeComChannelTestTransportErrorsRedactEscapedSecret(t *testing.T) {
	oldEndpoints := channelTestEndpoints
	oldClient := channelTestHTTPClient
	defer func() { channelTestEndpoints = oldEndpoints; channelTestHTTPClient = oldClient }()

	secret := "wecom secret/with+symbols"
	channelTestEndpoints.WeCom = "https://wecom.example.test/gettoken"
	channelTestHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("upstream failed: %s", r.URL.String())
	})}

	ok, msg := testWeComCredentials("corp-id", secret)
	if ok || strings.Contains(msg, secret) || strings.Contains(msg, url.QueryEscape(secret)) || !strings.Contains(msg, "[REDACTED]") {
		t.Fatalf("ok=%v msg=%q", ok, msg)
	}
}

func TestChannelTestEndpointRejectsMissingSecret(t *testing.T) {
	root := t.TempDir()
	h := newGoalTestServer(t, root).Routes()
	body := `{"profile_id":"dingtalk","fields":[{"name":"dingtalk_client_id","value":"ding-id"},{"name":"dingtalk_client_secret","value":""}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/channels/test", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp channelTestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.OK || !strings.Contains(resp.Message, "不能为空") {
		t.Fatalf("resp=%+v", resp)
	}
}
