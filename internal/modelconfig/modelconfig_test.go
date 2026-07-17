package modelconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
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

func TestExportPreservesChannelsAndOtherMyKeySource(t *testing.T) {
	root := t.TempDir()
	const original = `# User-maintained settings must survive model saves.
telegram_bot_token = "telegram-secret"
telegram_allowed_users = ["alice"]
custom_settings = {"theme": "dark"}

native_oai_config_old = {
    "apikey": "sk-old-secret",
    "apibase": "https://old.example/v1",
    "model": "old-model",
}
`
	active := filepath.Join(root, "mykey.py")
	if err := os.WriteFile(active, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	profiles := []Profile{{
		VarName:       "native_oai_config_new",
		SourceVarName: "native_oai_config_old",
		Type:          "native_oai",
		Name:          "new",
		APIBase:       "https://new.example/v1",
		Model:         "new-model",
		APIKey:        "sk-new-secret",
	}}
	if _, err := Export(root, profiles, true); err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	data, err := os.ReadFile(active)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		`telegram_bot_token = "telegram-secret"`,
		`telegram_allowed_users = ["alice"]`,
		`custom_settings = {"theme": "dark"}`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Export() removed unrelated mykey.py source %q:\n%s", want, text)
		}
	}
	if got := strings.Count(text, "# >>> GA Admin managed models >>>"); got != 1 {
		t.Fatalf("managed model block begin count = %d, want 1:\n%s", got, text)
	}
	if got := strings.Count(text, "# <<< GA Admin managed models <<<"); got != 1 {
		t.Fatalf("managed model block end count = %d, want 1:\n%s", got, text)
	}

	draft, err := ImportMyKeyWithPython(root, "", true)
	if err != nil {
		t.Fatalf("ImportMyKeyWithPython() error = %v", err)
	}
	if len(draft.Profiles) != 1 || draft.Profiles[0].VarName != "native_oai_config_new" {
		t.Fatalf("runtime profiles = %#v, want only newly managed profile", draft.Profiles)
	}

	profiles[0].Model = "newer-model"
	if _, err := Export(root, profiles, true); err != nil {
		t.Fatalf("second Export() error = %v", err)
	}
	data, err = os.ReadFile(active)
	if err != nil {
		t.Fatal(err)
	}
	text = string(data)
	if got := strings.Count(text, "# >>> GA Admin managed models >>>"); got != 1 {
		t.Fatalf("second Export() managed block count = %d, want 1:\n%s", got, text)
	}
	if got := strings.Count(text, `telegram_bot_token = "telegram-secret"`); got != 1 {
		t.Fatalf("second Export() channel assignment count = %d, want 1:\n%s", got, text)
	}
}

func TestExportMigratesLegacyRenderedModelsBeforeApplyingGlobalOrder(t *testing.T) {
	root := t.TempDir()
	profiles := []Profile{
		{VarName: "native_oai_config_a", Type: "native_oai", Name: "A", APIBase: "https://a.example/v1", APIKey: "sk-a", ModelConfigs: []ModelConfig{{Model: "a-one"}}},
		{VarName: "native_claude_config_b", Type: "native_claude", Name: "B", APIBase: "https://b.example/v1", APIKey: "sk-b", ModelConfigs: []ModelConfig{{Model: "b-one"}}},
	}
	legacy, err := Render(profiles)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	active := filepath.Join(root, "mykey.py")
	if err := os.WriteFile(active, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}

	zero, one := 0, 1
	profiles[0].ModelConfigs[0].SortOrder = &one
	profiles[1].ModelConfigs[0].SortOrder = &zero
	if _, err := Export(root, profiles, true); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	data, err := os.ReadFile(active)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Count(text, "native_oai_config_a =") != 1 || strings.Count(text, "native_claude_config_b =") != 1 {
		t.Fatalf("legacy model assignments were not removed:\n%s", text)
	}
	first := strings.Index(text, "native_claude_config_b =")
	second := strings.Index(text, "native_oai_config_a =")
	if first < 0 || second < 0 || first >= second {
		t.Fatalf("runtime declaration order does not follow saved global order:\n%s", text)
	}
	if strings.Count(text, managedModelsBegin) != 1 || strings.Count(text, managedModelsEnd) != 1 {
		t.Fatalf("managed model block count is invalid:\n%s", text)
	}
}

func TestExportRejectsUnsafeSourceAssignmentsWithoutWriting(t *testing.T) {
	tests := []struct {
		name       string
		original   string
		wantErrSub string
	}{
		{
			name: "duplicate assignments",
			original: "native_oai_config_old = {'model': 'one'}\n" +
				"native_oai_config_old = {'model': 'two'}\n",
			wantErrSub: "multiple top-level assignments found",
		},
		{
			name:       "chained assignment",
			original:   "alias = native_oai_config_old = {'model': 'one'}\n",
			wantErrSub: "not a standalone single-target assignment",
		},
		{
			name:       "invalid syntax",
			original:   "native_oai_config_old = {\n",
			wantErrSub: "mykey.py syntax error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			active := filepath.Join(root, "mykey.py")
			if err := os.WriteFile(active, []byte(tt.original), 0600); err != nil {
				t.Fatal(err)
			}
			profiles := []Profile{{
				VarName:       "native_oai_config_new",
				SourceVarName: "native_oai_config_old",
				Type:          "native_oai",
				Name:          "new",
				APIBase:       "https://new.example/v1",
				Model:         "new-model",
				APIKey:        "sk-new-secret",
			}}

			if _, err := Export(root, profiles, true); err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Fatalf("Export() error = %v, want substring %q", err, tt.wantErrSub)
			}
			got, err := os.ReadFile(active)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.original {
				t.Fatalf("mykey.py changed after rejected export:\n%s", got)
			}
			backups, err := filepath.Glob(filepath.Join(root, "mykey.py.bak-*"))
			if err != nil {
				t.Fatal(err)
			}
			if len(backups) != 0 {
				t.Fatalf("rejected export created backups: %v", backups)
			}
		})
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

func TestEmptyProviderValidatesAndRoundTripsThroughMyKey(t *testing.T) {
	root := t.TempDir()
	profiles := []Profile{{
		VarName: "native_oai_config_empty",
		Type:    "native_oai",
		Name:    "Empty provider",
		APIBase: "https://api.empty.example/v1",
		APIKey:  "sk-empty-secret",
	}}

	if err := Validate(profiles); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if _, err := Export(root, profiles, true); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "mykey.py"))
	if err != nil {
		t.Fatalf("read mykey.py: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "\nnative_oai_config_empty =") {
		t.Fatalf("empty provider must not render a discoverable GA model config:\n%s", text)
	}
	if !strings.Contains(text, "_ga_admin_provider_groups") || !strings.Contains(text, "native_oai_config_empty") {
		t.Fatalf("empty provider metadata missing:\n%s", text)
	}

	draft, err := ImportMyKeyWithPython(root, "", true)
	if err != nil {
		t.Fatalf("ImportMyKeyWithPython() error = %v", err)
	}
	if len(draft.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want one empty provider: %#v", len(draft.Profiles), draft.Profiles)
	}
	got := draft.Profiles[0]
	if got.VarName != profiles[0].VarName || got.Type != profiles[0].Type || got.Name != profiles[0].Name || got.APIBase != profiles[0].APIBase || got.APIKey != profiles[0].APIKey {
		t.Fatalf("round-tripped provider = %#v, want %#v", got, profiles[0])
	}
	if len(got.ModelConfigs) != 0 || len(got.Models) != 0 || got.Model != "" {
		t.Fatalf("empty provider gained models: %#v", got)
	}
}

func TestExportImportKeepsProviderModelsGrouped(t *testing.T) {
	root := t.TempDir()
	profiles := []Profile{{
		VarName: "native_oai_config_acme",
		Type:    "native_oai",
		Name:    "Acme",
		APIBase: "https://api.acme.example/v1",
		Model:   "acme-chat",
		Models:  []string{"acme-chat", "acme-reasoning"},
		APIKey:  "sk-real-secret",
	}}
	if _, err := Export(root, profiles, true); err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	draft, err := ImportMyKeyWithPython(root, "", true)
	if err != nil {
		t.Fatalf("ImportMyKeyWithPython() error = %v", err)
	}
	if len(draft.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want one provider: %#v", len(draft.Profiles), draft.Profiles)
	}
	got := draft.Profiles[0]
	if got.VarName != "native_oai_config_acme" {
		t.Fatalf("provider var_name = %q, want native_oai_config_acme", got.VarName)
	}
	if got.Model != "acme-chat" {
		t.Fatalf("primary model = %q, want acme-chat", got.Model)
	}
	if len(got.Models) != 2 || got.Models[0] != "acme-chat" || got.Models[1] != "acme-reasoning" {
		t.Fatalf("provider models = %#v, want both exported models", got.Models)
	}
}

func TestExportImportPreservesPerModelAdvancedConfig(t *testing.T) {
	root := t.TempDir()
	data := []byte(`{"profiles":[{"var_name":"native_oai_config_acme","type":"native_oai","name":"Acme","apibase":"https://api.acme.example/v1","apikey":"sk-real-secret","model_configs":[{"model":"acme-chat","reasoning_effort":"low","read_timeout":120},{"model":"acme-reasoning","reasoning_effort":"high","read_timeout":600}]}]}`)
	var input Draft
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if _, err := Export(root, input.Profiles, true); err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	draft, err := ImportMyKeyWithPython(root, "", true)
	if err != nil {
		t.Fatalf("ImportMyKeyWithPython() error = %v", err)
	}
	if len(draft.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want one provider: %#v", len(draft.Profiles), draft.Profiles)
	}
	encoded, err := json.Marshal(draft.Profiles[0])
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var got struct {
		ModelConfigs []struct {
			Model           string `json:"model"`
			ReasoningEffort string `json:"reasoning_effort"`
			ReadTimeout     *int   `json:"read_timeout"`
		} `json:"model_configs"`
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("Unmarshal result error = %v", err)
	}
	if len(got.ModelConfigs) != 2 {
		t.Fatalf("model_configs = %#v, want two entries", got.ModelConfigs)
	}
	if got.ModelConfigs[0].Model != "acme-chat" || got.ModelConfigs[0].ReasoningEffort != "low" || got.ModelConfigs[0].ReadTimeout == nil || *got.ModelConfigs[0].ReadTimeout != 120 {
		t.Fatalf("first model config = %#v", got.ModelConfigs[0])
	}
	if got.ModelConfigs[1].Model != "acme-reasoning" || got.ModelConfigs[1].ReasoningEffort != "high" || got.ModelConfigs[1].ReadTimeout == nil || *got.ModelConfigs[1].ReadTimeout != 600 {
		t.Fatalf("second model config = %#v", got.ModelConfigs[1])
	}
}

func TestExportImportPreservesDisabledModelWithoutExposingItToGA(t *testing.T) {
	root := t.TempDir()
	data := []byte(`{"profiles":[{"var_name":"native_oai_config_acme","type":"native_oai","name":"Acme","apibase":"https://api.acme.example/v1","apikey":"sk-real-secret","model_configs":[{"model":"active-model","enabled":true},{"model":"missing-model","enabled":false,"auto_disabled":true,"availability":"unavailable","availability_checked_at":"2026-07-15T02:00:00Z","availability_detail":"HTTP 404","availability_latency_ms":125,"read_timeout":600}]}]}`)
	var input Draft
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	rendered, err := Render(input.Profiles)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(rendered, "\nnative_oai_config_acme_2 =") {
		t.Fatalf("disabled model was rendered as a GA model variable:\n%s", rendered)
	}
	if !strings.Contains(rendered, `"auto_disabled": True`) || !strings.Contains(rendered, `"enabled": False`) {
		t.Fatalf("disabled model metadata was not rendered as valid Python:\n%s", rendered)
	}
	if _, err := Export(root, input.Profiles, true); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	draft, err := ImportMyKeyWithPython(root, "", true)
	if err != nil {
		t.Fatalf("ImportMyKeyWithPython() error = %v", err)
	}
	if len(draft.Profiles) != 1 || len(draft.Profiles[0].ModelConfigs) != 2 {
		t.Fatalf("imported disabled model config missing: %#v", draft.Profiles)
	}
	missing := draft.Profiles[0].ModelConfigs[1]
	if ModelConfigEnabled(missing) || !missing.AutoDisabled || missing.Availability != "unavailable" || missing.AvailabilityDetail != "HTTP 404" || missing.AvailabilityLatencyMS != 125 || missing.ReadTimeout == nil || *missing.ReadTimeout != 600 {
		t.Fatalf("disabled model config = %#v", missing)
	}
}

func TestRenderUsesGlobalModelSortOrderAcrossProviders(t *testing.T) {
	data := []byte(`{"profiles":[{"var_name":"native_oai_config_a","type":"native_oai","name":"Provider A","apibase":"https://a.example/v1","apikey":"sk-a-secret","model_configs":[{"model":"a-one","sort_order":0},{"model":"a-two","sort_order":2}]},{"var_name":"native_claude_config_b","type":"native_claude","name":"Provider B","apibase":"https://b.example/v1","apikey":"sk-b-secret","model_configs":[{"model":"b-one","sort_order":1}]}]}`)
	var input Draft
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	rendered, err := Render(input.Profiles)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	first := strings.Index(rendered, "\nnative_oai_config_a =")
	second := strings.Index(rendered, "\nnative_claude_config_b =")
	third := strings.Index(rendered, "\nnative_oai_config_a_2 =")
	if first < 0 || second < 0 || third < 0 || !(first < second && second < third) {
		t.Fatalf("render order does not follow sort_order (want A1, B1, A2):\n%s", rendered)
	}
	runtimeSection := strings.Split(rendered, "# Admin-only provider grouping metadata")[0]
	if strings.Contains(runtimeSection, `"sort_order"`) {
		t.Fatalf("sort_order is admin metadata and must not enter model dictionaries:\n%s", rendered)
	}
	for _, modelID := range []string{"a-one", "b-one", "a-two"} {
		want := fmt.Sprintf(`"name": %q`, modelID)
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered model %q does not use its model ID as name:\n%s", modelID, rendered)
		}
	}
	if strings.Count(rendered, "Provider A") != 1 || strings.Count(rendered, "Provider B") != 1 {
		t.Fatalf("provider names must remain only in provider grouping metadata:\n%s", rendered)
	}
}

func TestImportMyKeyPreservesGlobalModelDeclarationOrderAcrossGroupedProviders(t *testing.T) {
	root := t.TempDir()
	mykey := `native_oai_config_a = {
    "apikey": "sk-a-secret",
    "apibase": "https://a.example/v1",
    "model": "a-one",
}

native_claude_config_b = {
    "apikey": "sk-b-secret",
    "apibase": "https://b.example/v1",
    "model": "b-one",
}

native_oai_config_a_2 = {
    "apikey": "sk-a-secret",
    "apibase": "https://a.example/v1",
    "model": "a-two",
}
`
	if err := os.WriteFile(filepath.Join(root, "mykey.py"), []byte(mykey), 0600); err != nil {
		t.Fatalf("write mykey.py: %v", err)
	}

	draft, err := ImportMyKeyWithPython(root, "", true)
	if err != nil {
		t.Fatalf("ImportMyKeyWithPython() error = %v", err)
	}
	encoded, err := json.Marshal(draft.Profiles)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var profiles []struct {
		VarName      string `json:"var_name"`
		ModelConfigs []struct {
			Model     string `json:"model"`
			SortOrder *int   `json:"sort_order"`
		} `json:"model_configs"`
	}
	if err := json.Unmarshal(encoded, &profiles); err != nil {
		t.Fatalf("Unmarshal result error = %v", err)
	}
	orders := map[string]int{}
	for _, profile := range profiles {
		for _, config := range profile.ModelConfigs {
			if config.SortOrder == nil {
				t.Fatalf("model %q has no imported sort_order: %s", config.Model, encoded)
			}
			orders[config.Model] = *config.SortOrder
		}
	}
	if orders["a-one"] != 0 || orders["b-one"] != 1 || orders["a-two"] != 2 {
		t.Fatalf("imported declaration orders = %#v, want a-one=0 b-one=1 a-two=2", orders)
	}
}

func TestImportLegacyMyKeyGroupsProfilesByProviderIdentity(t *testing.T) {
	root := t.TempDir()
	mykey := `native_oai_config_gpt55 = {
    "apikey": "sk-shared-real-secret",
    "apibase": "https://code.example/v1/",
    "model": "gpt-5.5",
}

native_oai_config_gpt55_2 = {
    "apikey": "sk-shared-real-secret",
    "apibase": "https://code.example/v1",
    "model": "gpt-5.4",
}
`
	if err := os.WriteFile(filepath.Join(root, "mykey.py"), []byte(mykey), 0600); err != nil {
		t.Fatalf("write legacy mykey.py: %v", err)
	}

	draft, err := ImportMyKeyWithPython(root, "", false)
	if err != nil {
		t.Fatalf("ImportMyKeyWithPython() error = %v", err)
	}
	if len(draft.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want one provider: %#v", len(draft.Profiles), draft.Profiles)
	}
	got := draft.Profiles[0]
	if got.VarName != "native_oai_config_gpt55" {
		t.Fatalf("provider var_name = %q, want first legacy variable", got.VarName)
	}
	if len(got.Models) != 2 || got.Models[0] != "gpt-5.5" || got.Models[1] != "gpt-5.4" {
		t.Fatalf("provider models = %#v, want both legacy models", got.Models)
	}
	if got.APIKey != "sk-****cret" {
		t.Fatalf("masked provider key = %q, want masked secret", got.APIKey)
	}
}

func TestImportLegacyMyKeyDoesNotGroupDifferentKeysWithSameMask(t *testing.T) {
	root := t.TempDir()
	mykey := `native_oai_config_one = {
    "apikey": "sk-a-first-secret-tail",
    "apibase": "https://code.example/v1",
    "model": "gpt-one",
}

native_oai_config_two = {
    "apikey": "sk-a-other-secret-tail",
    "apibase": "https://code.example/v1",
    "model": "gpt-two",
}
`
	if err := os.WriteFile(filepath.Join(root, "mykey.py"), []byte(mykey), 0600); err != nil {
		t.Fatalf("write legacy mykey.py: %v", err)
	}

	draft, err := ImportMyKeyWithPython(root, "", false)
	if err != nil {
		t.Fatalf("ImportMyKeyWithPython() error = %v", err)
	}
	if len(draft.Profiles) != 2 {
		t.Fatalf("profiles len = %d, want different-key providers kept separate: %#v", len(draft.Profiles), draft.Profiles)
	}
	if draft.Profiles[0].APIKey != draft.Profiles[1].APIKey {
		t.Fatalf("test fixture masks differ: %q vs %q", draft.Profiles[0].APIKey, draft.Profiles[1].APIKey)
	}
}

func TestExportPreservesUTF8ProfileNameRoundTrip(t *testing.T) {
	root := t.TempDir()
	wantName := "主模型-中文"
	profiles := []Profile{{
		VarName: "native_oai_config1",
		Type:    "native_oai",
		Name:    wantName,
		APIBase: "https://api.example/v1",
		Model:   "gpt-test",
		APIKey:  "sk-real-secret",
	}}
	if _, err := Export(root, profiles, true); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "mykey.py"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !utf8.Valid(data) {
		t.Fatal("mykey.py is not valid UTF-8")
	}
	text := string(data)
	if !strings.Contains(text, managedModelsBegin) ||
		!strings.Contains(text, "# -*- coding: utf-8 -*-\n") ||
		!strings.Contains(text, wantName) {
		t.Fatalf("mykey.py did not preserve UTF-8 name:\n%s", text)
	}

	// Import must force UTF-8 even if the parent process has a legacy locale.
	t.Setenv("PYTHONIOENCODING", "cp936")
	draft, err := ImportMyKeyWithPython(root, "", true)
	if err != nil {
		t.Fatalf("ImportMyKeyWithPython() error = %v", err)
	}
	if len(draft.Profiles) != 1 || draft.Profiles[0].Name != wantName {
		t.Fatalf("round-trip name = %#v, want %q", draft.Profiles, wantName)
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

func TestExportKeepsOnlyTwoNewestMyKeyBackups(t *testing.T) {
	root := t.TempDir()
	active := filepath.Join(root, "mykey.py")
	if err := os.WriteFile(active, []byte("current"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("mykey.py.bak-20200101-00000%d", i)
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}

	profiles := []Profile{{
		VarName: "api_config_rotation",
		Type:    "openai",
		Name:    "rotation",
		APIBase: "https://api.example/v1",
		Model:   "gpt-test",
		APIKey:  "sk-test",
	}}
	res, err := Export(root, profiles, true)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	backups, err := filepath.Glob(filepath.Join(root, "mykey.py.bak-*"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(backups)
	if len(backups) != myKeyBackupLimit {
		t.Fatalf("backup count = %d, want %d: %v", len(backups), myKeyBackupLimit, backups)
	}
	newBackup, _ := res["backup_path"].(string)
	if backups[1] != newBackup || filepath.Base(backups[0]) != "mykey.py.bak-20200101-000003" {
		t.Fatalf("kept backups = %v, new backup = %q", backups, newBackup)
	}
}

func TestExportPrunesBackupsWhenActiveMyKeyIsMissing(t *testing.T) {
	root := t.TempDir()
	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("mykey.py.bak-20200101-00000%d", i)
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	profiles := []Profile{{VarName: "api_config_new", Type: "openai", Name: "new", APIBase: "https://api.example/v1", Model: "gpt-test", APIKey: "sk-test"}}
	if _, err := Export(root, profiles, true); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	backups, err := filepath.Glob(filepath.Join(root, "mykey.py.bak-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != myKeyBackupLimit {
		t.Fatalf("backup count = %d, want %d: %v", len(backups), myKeyBackupLimit, backups)
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
