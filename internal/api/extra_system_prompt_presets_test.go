package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"genericagent-admin-go/internal/config"
)

func TestExtraSystemPromptPresetsRoutePersistsReplaceSet(t *testing.T) {
	store := config.NewStore(t.TempDir())
	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore = store
	h := s.Routes()

	get := httptest.NewRequest(http.MethodGet, "/api/extra-system-prompt-presets", nil)
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, get)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRR.Code, getRR.Body.String())
	}

	body := `{"presets":[{"id":" review ","name":" Review ","content":" Check carefully. "}]}`
	unconfirmed := httptest.NewRequest(http.MethodPut, "/api/extra-system-prompt-presets", strings.NewReader(body))
	unconfirmedRR := httptest.NewRecorder()
	h.ServeHTTP(unconfirmedRR, unconfirmed)
	if unconfirmedRR.Code != http.StatusPreconditionRequired {
		t.Fatalf("unconfirmed PUT status=%d body=%s", unconfirmedRR.Code, unconfirmedRR.Body.String())
	}

	put := httptest.NewRequest(http.MethodPut, "/api/extra-system-prompt-presets", strings.NewReader(body))
	markDangerous(put)
	putRR := httptest.NewRecorder()
	h.ServeHTTP(putRR, put)
	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", putRR.Code, putRR.Body.String())
	}

	reloaded := config.NewStore(store.Root)
	got := reloaded.Cfg.ExtraSystemPromptPresets
	want := []config.ExtraSystemPromptPreset{{ID: "review", Name: "Review", Content: "Check carefully."}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("reloaded presets=%#v want=%#v", got, want)
	}
}

func TestExtraSystemPromptPresetsRejectInvalidReplaceWithoutMutation(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Root = t.TempDir()
	s.CfgStore.Cfg.ExtraSystemPromptPresets = []config.ExtraSystemPromptPreset{{ID: "keep", Name: "Keep", Content: "Keep me"}}
	if err := s.CfgStore.Save(s.CfgStore.Cfg); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/extra-system-prompt-presets", strings.NewReader(`{"presets":[{"id":"dup","name":"One","content":"1"},{"id":"dup","name":"Two","content":"2"}]}`))
	markDangerous(req)
	rr := httptest.NewRecorder()
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	reloaded := config.NewStore(s.CfgStore.Root)
	if got := reloaded.Cfg.ExtraSystemPromptPresets; len(got) != 1 || got[0].ID != "keep" {
		t.Fatalf("invalid replace mutated config: %#v", got)
	}
}

func TestChatSaveSettingsPresetBindingAndCompatibilitySemantics(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	s.CfgStore.Cfg.ExtraSystemPromptPresets = []config.ExtraSystemPromptPreset{
		{ID: "review", Name: "Review", Content: "Review this carefully."},
	}

	save := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/chat/settings/preset-session", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.Routes().ServeHTTP(rr, req)
		return rr
	}
	load := func() chatSession {
		t.Helper()
		cs, err := loadChatSession(s.CfgStore.Cfg, "preset-session")
		if err != nil {
			t.Fatal(err)
		}
		return cs
	}

	if rr := save(`{"llm_no":0,"extra_sys_prompt_preset_id":"review"}`); rr.Code != http.StatusOK {
		t.Fatalf("bind status=%d body=%s", rr.Code, rr.Body.String())
	}
	bound := load()
	if bound.ExtraSysPromptPresetID != "review" || len(bound.ExtraSysPrompts) != 1 || bound.ExtraSysPrompts[0] != "Review this carefully." {
		t.Fatalf("bound session=%#v", bound)
	}

	// Omission preserves both the stable binding and its resolved snapshot.
	if rr := save(`{"llm_no":1,"reasoning_effort":"high"}`); rr.Code != http.StatusOK {
		t.Fatalf("preserve status=%d body=%s", rr.Code, rr.Body.String())
	}
	preserved := load()
	if preserved.ExtraSysPromptPresetID != "review" || len(preserved.ExtraSysPrompts) != 1 || preserved.ExtraSysPrompts[0] != "Review this carefully." {
		t.Fatalf("omitted ID did not preserve binding: %#v", preserved)
	}

	// Deleting a global preset does not rewrite historical session identity/snapshot.
	s.CfgStore.Cfg.ExtraSystemPromptPresets = nil
	orphan := load()
	if orphan.ExtraSysPromptPresetID != "review" || len(orphan.ExtraSysPrompts) != 1 || orphan.ExtraSysPrompts[0] != "Review this carefully." {
		t.Fatalf("deleted preset did not preserve orphan snapshot: %#v", orphan)
	}

	if rr := save(`{"llm_no":1,"extra_sys_prompt_preset_id":""}`); rr.Code != http.StatusOK {
		t.Fatalf("clear status=%d body=%s", rr.Code, rr.Body.String())
	}
	cleared := load()
	if cleared.ExtraSysPromptPresetID != "" || len(cleared.ExtraSysPrompts) != 0 {
		t.Fatalf("clear left preset data: %#v", cleared)
	}
}

func TestChatSaveSettingsLegacyPromptsOverridePresetID(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	s.CfgStore.Cfg.ExtraSystemPromptPresets = []config.ExtraSystemPromptPreset{{ID: "review", Name: "Review", Content: "Preset"}}

	payload, err := json.Marshal(map[string]interface{}{
		"llm_no":                     0,
		"extra_sys_prompt_preset_id": "review",
		"extra_sys_prompts":          []string{"  legacy snapshot  "},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/chat/settings/legacy-session", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	cs, err := loadChatSession(s.CfgStore.Cfg, "legacy-session")
	if err != nil {
		t.Fatal(err)
	}
	if cs.ExtraSysPromptPresetID != "" || len(cs.ExtraSysPrompts) != 1 || cs.ExtraSysPrompts[0] != "legacy snapshot" {
		t.Fatalf("legacy field precedence failed: %#v", cs)
	}
}

func TestChatSaveSettingsRejectsUnknownPresetWithoutMutation(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	s.CfgStore.Cfg.ExtraSystemPromptPresets = []config.ExtraSystemPromptPreset{{ID: "known", Name: "Known", Content: "Known snapshot"}}

	seed := httptest.NewRequest(http.MethodPost, "/api/chat/settings/unknown-session", strings.NewReader(`{"llm_no":0,"extra_sys_prompt_preset_id":"known"}`))
	seed.Header.Set("Content-Type", "application/json")
	seedRR := httptest.NewRecorder()
	s.Routes().ServeHTTP(seedRR, seed)
	if seedRR.Code != http.StatusOK {
		t.Fatalf("seed status=%d body=%s", seedRR.Code, seedRR.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/chat/settings/unknown-session", strings.NewReader(`{"llm_no":0,"extra_sys_prompt_preset_id":"missing"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown status=%d body=%s", rr.Code, rr.Body.String())
	}
	cs, err := loadChatSession(s.CfgStore.Cfg, "unknown-session")
	if err != nil {
		t.Fatal(err)
	}
	if cs.ExtraSysPromptPresetID != "known" || len(cs.ExtraSysPrompts) != 1 || cs.ExtraSysPrompts[0] != "Known snapshot" {
		t.Fatalf("unknown preset mutated session: %#v", cs)
	}
}
