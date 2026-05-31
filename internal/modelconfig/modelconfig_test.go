package modelconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStoreSaveCreatesRootAndLoadsMaskedSecrets(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing", "models")
	store := NewStore(root)
	profiles := []Profile{{
		VarName: "api_config_main",
		Type:    "openai",
		Name:    "main",
		APIBase: "https://api.example/v1",
		Model:   "gpt-test",
		APIKey:  "sk-real-secret",
	}}
	if _, err := store.Save(profiles); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	st, err := os.Stat(filepath.Join(root, "model_profiles.json"))
	if err != nil {
		t.Fatalf("saved file missing: %v", err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0600 {
		t.Fatalf("saved file perm = %v, want 0600", st.Mode().Perm())
	}
	draft, err := store.Load(false)
	if err != nil {
		t.Fatalf("Load(false) error = %v", err)
	}
	if got := draft.Profiles[0].APIKey; got != "******" {
		t.Fatalf("masked APIKey = %q, want ******", got)
	}
	raw, err := store.Load(true)
	if err != nil {
		t.Fatalf("Load(true) error = %v", err)
	}
	if got := raw.Profiles[0].APIKey; got != "sk-real-secret" {
		t.Fatalf("raw APIKey = %q", got)
	}
}

func TestStoreSaveRejectsMaskedSecretWithoutWriting(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	profiles := []Profile{{
		VarName: "api_config_main",
		Type:    "openai",
		Name:    "main",
		APIBase: "https://api.example/v1",
		Model:   "gpt-test",
		APIKey:  "sk-****cret",
	}}
	if _, err := store.Save(profiles); err == nil || !strings.Contains(err.Error(), "masked apikey") {
		t.Fatalf("Save() error = %v, want masked apikey", err)
	}
	if _, err := os.Stat(filepath.Join(root, "model_profiles.json")); !os.IsNotExist(err) {
		t.Fatalf("model_profiles.json exists or unexpected stat error: %v", err)
	}
}

func TestExportWritesGeneratedAndActivatesAtomically(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing", "ga")
	profiles := []Profile{{
		VarName: "api_config_main",
		Type:    "openai",
		Name:    "main",
		APIBase: "https://api.example/v1",
		Model:   "gpt-test",
		APIKey:  "sk-real-secret",
	}}
	res, err := Export(root, profiles, true)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if res["activated"] != true {
		t.Fatalf("activated = %v, want true", res["activated"])
	}
	for _, name := range []string{"mykey_admin.generated.py", "mykey.py"} {
		p := filepath.Join(root, name)
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
		if !strings.Contains(string(data), "sk-real-secret") || !strings.Contains(string(data), "api_config_main") {
			t.Fatalf("%s content missing rendered profile: %q", name, string(data))
		}
		if st, err := os.Stat(p); err != nil {
			t.Fatalf("stat %s: %v", name, err)
		} else if runtime.GOOS != "windows" && st.Mode().Perm() != 0600 {
			t.Fatalf("%s perm = %v, want 0600", name, st.Mode().Perm())
		}
	}
}

func TestExportBacksUpExistingActive(t *testing.T) {
	root := t.TempDir()
	active := filepath.Join(root, "mykey.py")
	old := []byte("old active")
	if err := os.WriteFile(active, old, 0600); err != nil {
		t.Fatalf("seed active: %v", err)
	}
	profiles := []Profile{{
		VarName: "api_config_main",
		Type:    "openai",
		Name:    "main",
		APIBase: "https://api.example/v1",
		Model:   "gpt-test",
		APIKey:  "sk-real-secret",
	}}
	res, err := Export(root, profiles, true)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	bak, ok := res["backup_path"].(string)
	if !ok || bak == "" {
		t.Fatalf("backup_path = %#v, want path", res["backup_path"])
	}
	data, err := os.ReadFile(bak)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(data) != string(old) {
		t.Fatalf("backup content = %q, want %q", string(data), string(old))
	}
	activeData, err := os.ReadFile(active)
	if err != nil {
		t.Fatalf("read active: %v", err)
	}
	if string(activeData) == string(old) || !strings.Contains(string(activeData), "sk-real-secret") {
		t.Fatalf("active not replaced with rendered key: %q", string(activeData))
	}
}

func TestRenderRejectsUnmarshalableExtraValue(t *testing.T) {
	profiles := []Profile{{
		VarName: "api_config_main",
		Type:    "openai",
		Name:    "main",
		APIBase: "https://api.example/v1",
		Model:   "gpt-test",
		APIKey:  "sk-real-secret",
		Extra: map[string]interface{}{
			"bad": func() {},
		},
	}}
	_, err := Render(profiles)
	if err == nil || !strings.Contains(err.Error(), "render \"bad\"") {
		t.Fatalf("Render() error = %v, want render bad", err)
	}
}
