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
