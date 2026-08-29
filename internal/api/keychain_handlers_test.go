package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestKeychainPythonCompatibilityVector(t *testing.T) {
	ciphertext, err := hex.DecodeString("65ab62274a71aa87566a664b7ada7fab8632fcc065b1eaed9f8d31df0a70cbb768e86f3e5f3bb6")
	if err != nil {
		t.Fatal(err)
	}
	xorKeychainForUser(ciphertext, "compat-user")
	const want = `{"alpha": "s3cret", "unicode": "value"}`
	if string(ciphertext) != want {
		t.Fatalf("Python-compatible decrypt mismatch: got %q want %q", ciphertext, want)
	}
}

func TestKeychainFileRoundTripIsIsolated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ga_keychain.enc")
	want := map[string]string{"alpha": "s3cret", "unicode": "值"}
	if err := writeKeychainFile(path, "test-user", want); err != nil {
		t.Fatalf("write: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ciphertext: %v", err)
	}
	for _, secret := range want {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatalf("keychain file contains plaintext secret %q", secret)
		}
	}
	got, err := readKeychainFile(path, "test-user")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch: got %#v want %#v", got, want)
	}

	updated := map[string]string{"beta": "replacement"}
	if err := writeKeychainFile(path, "test-user", updated); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	got, err = readKeychainFile(path, "test-user")
	if err != nil || !reflect.DeepEqual(got, updated) {
		t.Fatalf("overwrite mismatch: got %#v err=%v want %#v", got, err, updated)
	}
}

func TestReadMissingKeychainIsEmpty(t *testing.T) {
	got, err := readKeychainFile(filepath.Join(t.TempDir(), "missing.enc"), "test-user")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %#v, want empty keychain", got)
	}
}

func TestKeychainHandlerNeverReturnsSecret(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("USERNAME", "handler-test-user")

	s := &Server{}
	const secret = "contract-secret-value"
	put := httptest.NewRequest(http.MethodPut, "/api/keychain", strings.NewReader(`{"name":"alpha","value":"`+secret+`"}`))
	put.Header.Set("Content-Type", "application/json")
	putResult := httptest.NewRecorder()
	s.keychainHandler(putResult, put)
	if putResult.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", putResult.Code, putResult.Body.String())
	}
	if strings.Contains(putResult.Body.String(), secret) {
		t.Fatal("PUT response exposed secret value")
	}

	getResult := httptest.NewRecorder()
	s.keychainHandler(getResult, httptest.NewRequest(http.MethodGet, "/api/keychain", nil))
	if getResult.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getResult.Code, getResult.Body.String())
	}
	if strings.Contains(getResult.Body.String(), secret) {
		t.Fatal("GET response exposed secret value")
	}
	var response struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal(getResult.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(response.Keys, []string{"alpha"}) {
		t.Fatalf("GET keys=%#v, want alpha only", response.Keys)
	}
}

func TestValidKeychainName(t *testing.T) {
	for _, name := range []string{"alpha", "ARK_API_KEY", "密钥"} {
		if !validKeychainName(name) {
			t.Errorf("valid name rejected: %q", name)
		}
	}
	for _, name := range []string{"", " leading", "trailing ", "a/b", "a\\b", "a\nb"} {
		if validKeychainName(name) {
			t.Errorf("invalid name accepted: %q", name)
		}
	}
}
