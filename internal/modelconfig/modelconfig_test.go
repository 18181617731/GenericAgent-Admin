package modelconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProfileAcceptsBooleanFakeCCSystemPrompt(t *testing.T) {
	data := []byte(`{"profiles":[{"var_name":"api_config_main","type":"native_claude","name":"main","apibase":"https://api.example/v1","model":"claude-test","apikey":"sk-real-secret","fake_cc_system_prompt":true}]}`)
	var draft Draft
	if err := json.Unmarshal(data, &draft); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(draft.Profiles) != 1 || draft.Profiles[0].FakeCCSystemPrompt == nil || !bool(*draft.Profiles[0].FakeCCSystemPrompt) {
		t.Fatalf("FakeCCSystemPrompt = %#v, want true", draft.Profiles)
	}
	rendered, err := Render(draft.Profiles)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(rendered, `"fake_cc_system_prompt": True`) {
		t.Fatalf("rendered fake_cc_system_prompt not Python bool:\n%s", rendered)
	}
}

func TestProfileAcceptsLegacyStringFakeCCSystemPrompt(t *testing.T) {
	data := []byte(`{"profiles":[{"var_name":"api_config_main","type":"native_claude","name":"main","apibase":"https://api.example/v1","model":"claude-test","apikey":"sk-real-secret","fake_cc_system_prompt":"false"}]}`)
	var draft Draft
	if err := json.Unmarshal(data, &draft); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(draft.Profiles) != 1 || draft.Profiles[0].FakeCCSystemPrompt == nil || bool(*draft.Profiles[0].FakeCCSystemPrompt) {
		t.Fatalf("FakeCCSystemPrompt = %#v, want false", draft.Profiles)
	}
	rendered, err := Render(draft.Profiles)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(rendered, `"fake_cc_system_prompt": False`) {
		t.Fatalf("rendered fake_cc_system_prompt not Python false:\n%s", rendered)
	}
}

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

func TestStoreSavePreservesExistingSecretWhenSubmittedBlank(t *testing.T) {
	root := t.TempDir()
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
		t.Fatalf("seed Save() error = %v", err)
	}
	profiles[0].APIKey = ""
	profiles[0].Model = "gpt-updated"
	if _, err := store.Save(profiles); err != nil {
		t.Fatalf("Save(blank secret) error = %v", err)
	}
	raw, err := store.Load(true)
	if err != nil {
		t.Fatalf("Load(true) error = %v", err)
	}
	if got := raw.Profiles[0].APIKey; got != "sk-real-secret" {
		t.Fatalf("preserved APIKey = %q, want old secret", got)
	}
	if got := raw.Profiles[0].Model; got != "gpt-updated" {
		t.Fatalf("updated model = %q", got)
	}
}

func TestStoreSaveAllowsMaskedSecret(t *testing.T) {
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
	if _, err := store.Save(profiles); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	raw, err := store.Load(true)
	if err != nil {
		t.Fatalf("Load(true) error = %v", err)
	}
	if got := raw.Profiles[0].APIKey; got != "sk-****cret" {
		t.Fatalf("saved APIKey = %q, want masked placeholder", got)
	}
}

func TestExportWritesOfficialMyKeyAtomically(t *testing.T) {
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
	p := filepath.Join(root, "mykey.py")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("mykey.py missing: %v", err)
	}
	if !strings.Contains(string(data), "sk-real-secret") || !strings.Contains(string(data), "api_config_main") {
		t.Fatalf("mykey.py content missing rendered profile: %q", string(data))
	}
	if st, err := os.Stat(p); err != nil {
		t.Fatalf("stat mykey.py: %v", err)
	} else if runtime.GOOS != "windows" && st.Mode().Perm() != 0600 {
		t.Fatalf("mykey.py perm = %v, want 0600", st.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(root, "mykey_admin.generated.py")); !os.IsNotExist(err) {
		t.Fatalf("mykey_admin.generated.py should not be written; stat err=%v", err)
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

func TestExportRejectsUnsafeGARoot(t *testing.T) {
	profiles := []Profile{{
		VarName: "api_config_main",
		Type:    "openai",
		Name:    "main",
		APIBase: "https://api.example/v1",
		Model:   "gpt-test",
		APIKey:  "sk-real-secret",
	}}
	for _, root := range []string{"", ".", filepath.VolumeName(t.TempDir()) + string(filepath.Separator)} {
		_, err := Export(root, profiles, false)
		if err == nil || !strings.Contains(err.Error(), "filesystem root") {
			t.Fatalf("Export(%q) error = %v, want filesystem root rejection", root, err)
		}
	}
}

func TestImportMyKeyExecutesCurrentFileAndUsesFinalRuntimeValues(t *testing.T) {
	root := t.TempDir()
	mykey := filepath.Join(root, "mykey.py")
	text := "native_oai_config1 = {\n" +
		"    'name': 'old-literal',\n" +
		"    'apibase': 'https://old.example/v1',\n" +
		"    'model': 'old-model',\n" +
		"    'apikey': 'sk-old-secret',\n" +
		"}\n" +
		"native_oai_config1.update({\n" +
		"    'name': 'current-runtime',\n" +
		"    'apibase': 'https://current.example/v1',\n" +
		"    'model': 'current-model',\n" +
		"    'apikey': 'sk-current-secret',\n" +
		"})\n"
	if err := os.WriteFile(mykey, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}

	draft, err := ImportMyKeyWithPython(root, "", false)
	if err != nil {
		t.Fatalf("ImportMyKeyWithPython() error = %v", err)
	}
	if len(draft.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1: %#v", len(draft.Profiles), draft.Profiles)
	}
	p := draft.Profiles[0]
	if p.Name != "current-runtime" || p.APIBase != "https://current.example/v1" || p.Model != "current-model" {
		t.Fatalf("profile = %#v, want current runtime values", p)
	}
	if p.APIKey != "sk-****cret" {
		t.Fatalf("masked APIKey = %q", p.APIKey)
	}

	raw, err := ImportMyKeyWithPython(root, "", true)
	if err != nil {
		t.Fatalf("ImportMyKeyWithPython(reveal) error = %v", err)
	}
	if raw.Profiles[0].APIKey != "sk-current-secret" {
		t.Fatalf("raw APIKey = %q", raw.Profiles[0].APIKey)
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

func TestPythonExePrefersConfiguredPath(t *testing.T) {
	configured := filepath.Join(t.TempDir(), "custom-python")
	if got := pythonExe(t.TempDir(), configured); got != configured {
		t.Fatalf("pythonExe configured = %q, want %q", got, configured)
	}
}

func TestPythonExeFindsPosixVirtualEnvBeforeFallback(t *testing.T) {
	root := t.TempDir()
	posixPython := filepath.Join(root, ".venv", "bin", "python")
	if err := os.MkdirAll(filepath.Dir(posixPython), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(posixPython, []byte(""), 0755); err != nil {
		t.Fatal(err)
	}
	if got := pythonExe(root, ""); got != posixPython {
		t.Fatalf("pythonExe posix venv = %q, want %q", got, posixPython)
	}
}

func TestPythonExeFallbackPrefersPython3OffWindows(t *testing.T) {
	got := pythonExe(t.TempDir(), "")
	want := "python3"
	if runtime.GOOS == "windows" {
		want = "python"
	}
	if got != want {
		t.Fatalf("pythonExe fallback = %q, want %q", got, want)
	}
}

func TestStoreSavePreservesExistingSecretWhenSubmittedMasked(t *testing.T) {
	root := t.TempDir()
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
		t.Fatalf("seed Save() error = %v", err)
	}
	profiles[0].APIKey = "sk-****cret"
	profiles[0].Model = "gpt-updated"
	if _, err := store.Save(profiles); err != nil {
		t.Fatalf("Save(masked secret) error = %v", err)
	}
	raw, err := store.Load(true)
	if err != nil {
		t.Fatalf("Load(true) error = %v", err)
	}
	if got := raw.Profiles[0].APIKey; got != "sk-real-secret" {
		t.Fatalf("preserved APIKey = %q, want old secret", got)
	}
	if got := raw.Profiles[0].Model; got != "gpt-updated" {
		t.Fatalf("updated model = %q", got)
	}
}

func TestRenderPreviewAllowsMaskedSecretWithoutUnmasking(t *testing.T) {
	profiles := []Profile{{
		VarName: "api_config_main",
		Type:    "openai",
		Name:    "main",
		APIBase: "https://api.example/v1",
		Model:   "gpt-test",
		APIKey:  "sk-****cret",
	}}
	rendered, err := RenderPreview(profiles)
	if err != nil {
		t.Fatalf("RenderPreview() error = %v", err)
	}
	if !strings.Contains(rendered, `"apikey": "sk-****cret"`) {
		t.Fatalf("preview did not keep masked placeholder:\n%s", rendered)
	}
	if strings.Contains(rendered, "sk-real-secret") {
		t.Fatalf("preview leaked real secret: %s", rendered)
	}
	renderedFull, err := Render(profiles)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(renderedFull, `"apikey": "sk-****cret"`) {
		t.Fatalf("render did not keep masked placeholder:\n%s", renderedFull)
	}
}
