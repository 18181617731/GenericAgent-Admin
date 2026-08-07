package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"genericagent-admin-go/internal/config"
)

func TestUsageOverviewAggregatesSessionsWithoutDoubleCounting(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	updateTestConfig(t, s.CfgStore, func(cfg *config.AppConfig) {
		cfg.ChatDataDir = t.TempDir()
	})
	cfg := s.CfgStore.Snapshot()
	firstDay := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.Local).Unix()
	secondDay := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.Local).UnixMilli()

	first := chatSession{
		ID:    "alpha",
		Title: "Alpha session",
		Messages: []chatMessage{
			{Role: "user", Content: "secret prompt", Usage: map[string]int{"input_tokens": 999}},
			{
				Role:      "assistant",
				ModelID:   "model-a",
				Content:   "secret response",
				CreatedAt: firstDay,
				Usage:     map[string]int{"input_tokens": 10, "output_tokens": 4, "total_tokens": 14},
				Usages: []map[string]int{
					{"input_tokens": 6, "output_tokens": 2},
					{"input_tokens": 4, "output_tokens": 2},
				},
			},
		},
	}
	second := chatSession{
		ID:    "beta",
		Title: "Beta session",
		Messages: []chatMessage{{
			Role:      "assistant",
			ModelID:   "model-b",
			CreatedAt: secondDay,
			Usages: []map[string]int{
				{"prompt_tokens": 3, "completion_tokens": 2, "cached_tokens": 1},
				{"input_tokens": 2, "output_tokens": 1},
			},
		}},
	}
	for _, session := range []chatSession{first, second} {
		if err := saveChatSession(cfg, session); err != nil {
			t.Fatalf("save session %q: %v", session.ID, err)
		}
	}
	if err := os.WriteFile(filepath.Join(chatSessionDir(cfg), "broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("write broken session: %v", err)
	}

	rr := httptest.NewRecorder()
	s.usageOverview(rr, httptest.NewRequest(http.MethodGet, "/api/usage/overview", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got usageOverviewResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.SessionCount != 2 || got.AssistantReplies != 2 || got.SkippedSessions != 1 {
		t.Fatalf("counts=%+v", got)
	}
	if got.Totals.InputTokens != 15 || got.Totals.OutputTokens != 7 || got.Totals.TotalTokens != 22 {
		t.Fatalf("totals=%+v want input=15 output=7 total=22", got.Totals)
	}
	if got.Totals.Other["cached_tokens"] != 1 {
		t.Fatalf("other=%+v", got.Totals.Other)
	}
	if len(got.Models) != 2 || got.Models[0].Name != "model-a" || got.Models[0].Totals.TotalTokens != 14 {
		t.Fatalf("models=%+v", got.Models)
	}
	if len(got.Sessions) != 2 {
		t.Fatalf("sessions=%+v", got.Sessions)
	}
	if len(got.Daily) != 2 || got.Daily[0].Date != "2026-07-23" || got.Daily[0].Totals.TotalTokens != 14 || got.Daily[1].Date != "2026-07-24" || got.Daily[1].Totals.TotalTokens != 8 {
		t.Fatalf("daily=%+v", got.Daily)
	}
	body := rr.Body.String()
	if containsAny(body, "secret prompt", "secret response") {
		t.Fatalf("response leaked message content: %s", body)
	}
}

func TestUsageOverviewOmitsUnknownModelBreakdown(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	updateTestConfig(t, s.CfgStore, func(cfg *config.AppConfig) {
		cfg.ChatDataDir = t.TempDir()
	})
	cfg := s.CfgStore.Snapshot()
	if err := saveChatSession(cfg, chatSession{
		ID: "legacy",
		Messages: []chatMessage{{
			Role:  "assistant",
			Usage: map[string]int{"input_tokens": 7, "output_tokens": 3, "total_tokens": 10},
		}},
	}); err != nil {
		t.Fatalf("save legacy session: %v", err)
	}

	rr := httptest.NewRecorder()
	s.usageOverview(rr, httptest.NewRequest(http.MethodGet, "/api/usage/overview", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got usageOverviewResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.AssistantReplies != 1 || got.Totals.TotalTokens != 10 {
		t.Fatalf("legacy usage omitted from totals: %+v", got)
	}
	if len(got.Models) != 0 {
		t.Fatalf("legacy usage created unknown model breakdown: %+v", got.Models)
	}
}

func TestUsageLedgerSurvivesSessionDeletion(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	updateTestConfig(t, s.CfgStore, func(cfg *config.AppConfig) {
		cfg.ChatDataDir = t.TempDir()
	})
	cfg := s.CfgStore.Snapshot()
	cs := chatSession{ID: "deleted-later", Title: "Deleted later", Messages: []chatMessage{{
		ID: "reply-1", Role: "assistant", ModelID: "model-x", CreatedAt: 123,
		Usage: map[string]int{"input_tokens": 7, "output_tokens": 3, "total_tokens": 10},
	}}}
	if err := saveChatSession(cfg, cs); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if _, _, err := s.loadOrMigrateUsageLedger(); err != nil {
		t.Fatalf("migrate usage: %v", err)
	}
	if err := os.Remove(chatSessionPath(cfg, cs.ID)); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	rr := httptest.NewRecorder()
	s.usageOverview(rr, httptest.NewRequest(http.MethodGet, "/api/usage/overview", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got usageOverviewResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.AssistantReplies != 1 || got.Totals.TotalTokens != 10 || len(got.Sessions) != 1 {
		t.Fatalf("usage changed after session deletion: %+v", got)
	}
}

func TestRecordSessionUsageMigratesExistingSessionsFirst(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	updateTestConfig(t, s.CfgStore, func(cfg *config.AppConfig) {
		cfg.ChatDataDir = t.TempDir()
	})
	cfg := s.CfgStore.Snapshot()
	old := chatSession{ID: "old", Messages: []chatMessage{{
		ID: "old-reply", Role: "assistant", Usage: map[string]int{"total_tokens": 10},
	}}}
	if err := saveChatSession(cfg, old); err != nil {
		t.Fatalf("save old session: %v", err)
	}
	fresh := chatSession{ID: "fresh", Messages: []chatMessage{{
		ID: "fresh-reply", Role: "assistant", Usage: map[string]int{"total_tokens": 20},
	}}}
	if err := s.recordSessionUsage(fresh); err != nil {
		t.Fatalf("record fresh usage: %v", err)
	}
	ledger, err := readUsageLedger(cfg)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if len(ledger.Entries) != 2 {
		t.Fatalf("ledger entries=%d, want 2: %+v", len(ledger.Entries), ledger.Entries)
	}
}

func TestUsageOverviewMissingDirectoryIsEmptyAndReadOnly(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	updateTestConfig(t, s.CfgStore, func(cfg *config.AppConfig) {
		cfg.ChatDataDir = t.TempDir()
	})
	dir := chatSessionDir(s.CfgStore.Snapshot())

	rr := httptest.NewRecorder()
	s.usageOverview(rr, httptest.NewRequest(http.MethodGet, "/api/usage/overview", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("GET created session directory or returned unexpected stat error: %v", err)
	}
	var got usageOverviewResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Models == nil || got.Sessions == nil || got.SessionCount != 0 {
		t.Fatalf("empty response=%+v", got)
	}
}

func TestUsageOverviewRejectsNonGet(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	rr := httptest.NewRecorder()
	s.usageOverview(rr, httptest.NewRequest(http.MethodPost, "/api/usage/overview", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestUsageOverviewFiltersAndPaginatesUsageRecords(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	cfg := s.CfgStore.Cfg
	if err := os.MkdirAll(chatDataDir(cfg), 0o755); err != nil {
		t.Fatalf("create chat data directory: %v", err)
	}
	first := time.Date(2026, time.July, 27, 9, 0, 0, 0, time.Local).Unix()
	second := time.Date(2026, time.July, 28, 9, 0, 0, 0, time.Local).Unix()
	llmNo := 4
	ledger := usageLedger{Entries: []usageLedgerEntry{
		{Key: "one", SessionID: "session-one", Title: "First session", MessageID: "m1", ModelID: "model-a", LLMNo: &llmNo, CreatedAt: first, ElapsedMS: 1200, Totals: usageTotals{InputTokens: 10, OutputTokens: 4, TotalTokens: 14, Other: map[string]int{"cached_tokens": 2}}},
		{Key: "two", SessionID: "session-two", Title: "Second session", MessageID: "m2", ModelID: "model-a", CreatedAt: second, ElapsedMS: 2300, Totals: usageTotals{InputTokens: 20, OutputTokens: 8, TotalTokens: 28, Other: map[string]int{"cached_tokens": 3}}},
		{Key: "three", SessionID: "session-three", Title: "Other session", MessageID: "m3", ModelID: "model-b", CreatedAt: second, Totals: usageTotals{InputTokens: 30, OutputTokens: 9, TotalTokens: 39}},
	}}
	if err := writeUsageLedger(cfg, ledger); err != nil {
		t.Fatalf("write usage ledger: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/usage/overview?from=2026-07-27&to=2026-07-28&model=model-a&page=2&page_size=1", nil)
	recorder := httptest.NewRecorder()
	s.usageOverview(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response usageOverviewResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.RecordTotal != 2 || response.RecordPage != 2 || response.RecordPageSize != 1 || response.RecordTotalPages != 2 {
		t.Fatalf("record pagination=%+v", response)
	}
	if len(response.Records) != 1 || response.Records[0].ID != "one" || response.Records[0].CachedTokens != 2 || response.Records[0].ElapsedMS != 1200 {
		t.Fatalf("records=%+v", response.Records)
	}
	if response.Records[0].CreatedAtMS != first*1000 {
		t.Fatalf("created_at_ms=%d want %d", response.Records[0].CreatedAtMS, first*1000)
	}
	if len(response.RecordModels) != 2 || len(response.RecordProviders) != 0 {
		t.Fatalf("record filter options models=%v providers=%v", response.RecordModels, response.RecordProviders)
	}
}

func TestUsageExportIsReadOnlyAndReturnsCSV(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	cfg := s.CfgStore.Cfg
	if err := os.MkdirAll(chatDataDir(cfg), 0o755); err != nil {
		t.Fatalf("create chat data directory: %v", err)
	}
	entry := usageLedgerEntry{Key: "export-one", SessionID: "session", Title: "Export session", ModelID: "model-export", CreatedAt: time.Now().Unix(), Totals: usageTotals{InputTokens: 5, OutputTokens: 2, TotalTokens: 7}}
	if err := writeUsageLedger(cfg, usageLedger{Entries: []usageLedgerEntry{entry}}); err != nil {
		t.Fatalf("write usage ledger: %v", err)
	}
	path := usageLedgerPath(cfg)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat ledger: %v", err)
	}
	recorder := httptest.NewRecorder()
	s.usageExport(recorder, httptest.NewRequest(http.MethodGet, "/api/usage/export?model=model-export", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Header().Get("Content-Type"), "text/csv") || !strings.Contains(recorder.Header().Get("Content-Disposition"), "usage-records.csv") {
		t.Fatalf("headers=%v", recorder.Header())
	}
	if !strings.Contains(recorder.Body.String(), "model-export") || !strings.Contains(recorder.Body.String(), "Export session") || !strings.Contains(recorder.Body.String(), "历史记录未保存服务商") {
		t.Fatalf("csv=%q", recorder.Body.String())
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat ledger after export: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Fatalf("export changed ledger: before=%v/%d after=%v/%d", before.ModTime(), before.Size(), after.ModTime(), after.Size())
	}
}

func TestUsageOverviewRejectsInvalidRecordQuery(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	recorder := httptest.NewRecorder()
	s.usageOverview(recorder, httptest.NewRequest(http.MethodGet, "/api/usage/overview?from=not-a-date", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestEnrichUsageLedgerMetadataReadsSessionDetails(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	cfg := s.CfgStore.Cfg
	llmNo := 3
	session := chatSession{ID: "metadata-session", Title: "Renamed session", Messages: []chatMessage{{
		ID: "reply", Role: "assistant", ModelID: "model-meta", LLMNo: &llmNo, CreatedAt: 99, ElapsedMS: 456,
	}}}
	if err := saveChatSession(cfg, session); err != nil {
		t.Fatalf("save session: %v", err)
	}
	ledger := usageLedger{Entries: []usageLedgerEntry{{
		Key: usageEntryKey(session.ID, "reply", 0), SessionID: session.ID, ModelID: session.Messages[0].ModelID,
		CreatedAt: 99, Totals: usageTotals{TotalTokens: 1},
	}}}
	enriched := enrichUsageLedgerMetadata(cfg, ledger)
	if len(enriched.Entries) != 1 || enriched.Entries[0].Title != "Renamed session" || enriched.Entries[0].LLMNo == nil || *enriched.Entries[0].LLMNo != llmNo || enriched.Entries[0].ElapsedMS != 456 {
		t.Fatalf("enriched=%+v", enriched.Entries)
	}
}

func TestUsageEventsAreIngestedIdempotentlyIncludingInputOnlyUsage(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	cfg := s.CfgStore.Cfg
	if err := os.MkdirAll(usageEventDir(cfg), 0o755); err != nil {
		t.Fatalf("create usage event directory: %v", err)
	}
	event := usageEvent{
		ID: "event-input-only", Channel: "autonomous", Source: "reflect/autonomous.py",
		SessionID: "auto-session", SessionName: "自主进化", ModelID: "model-auto",
		ReasoningEffort: "max", CreatedAt: 123456, Totals: map[string]int{"input_tokens": 17},
	}
	contents, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usageEventDir(cfg), "ga-autonomous-test.jsonl"), append(contents, '\n'), 0o644); err != nil {
		t.Fatalf("write usage event: %v", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		recorder := httptest.NewRecorder()
		s.usageOverview(recorder, httptest.NewRequest(http.MethodGet, "/api/usage/overview", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("attempt=%d status=%d body=%s", attempt, recorder.Code, recorder.Body.String())
		}
		var response usageOverviewResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.RecordTotal != 1 || response.Totals.InputTokens != 17 || response.Totals.TotalTokens != 17 {
			t.Fatalf("attempt=%d response=%+v", attempt, response)
		}
		if len(response.Records) != 1 || response.Records[0].Channel != "autonomous" || response.Records[0].ReasoningEffort != "max" {
			t.Fatalf("attempt=%d records=%+v", attempt, response.Records)
		}
	}
	ledger, err := readUsageLedger(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger.Entries) != 1 {
		t.Fatalf("ledger entries=%d, want 1: %+v", len(ledger.Entries), ledger.Entries)
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if len(needle) > 0 && len(value) >= len(needle) {
			for i := 0; i+len(needle) <= len(value); i++ {
				if value[i:i+len(needle)] == needle {
					return true
				}
			}
		}
	}
	return false
}

func TestNormalizedMessageUsagePrefersUsagesOverZeroUsage(t *testing.T) {
	// A terminal SSE event can overwrite the aggregated usage map with all-zero
	// values while the per-turn usages array keeps the real numbers.
	message := chatMessage{
		Role:    "assistant",
		ModelID: "model-z",
		Usage:   map[string]int{"input_tokens": 0, "output_tokens": 0, "cached_tokens": 0},
		Usages: []map[string]int{
			{"input_tokens": 120, "output_tokens": 30},
			{"input_tokens": 80, "output_tokens": 20, "cached_tokens": 7},
		},
	}
	totals, ok := normalizedMessageUsage(message)
	if !ok {
		t.Fatal("expected usage to be reported")
	}
	if totals.InputTokens != 200 || totals.OutputTokens != 50 || totals.TotalTokens != 250 {
		t.Fatalf("totals=%+v want input=200 output=50 total=250", totals)
	}
	if totals.Other["cached_tokens"] != 7 {
		t.Fatalf("other=%+v want cached_tokens=7", totals.Other)
	}
}

func TestNormalizedMessageUsageIncludesModernCacheCategoriesInInput(t *testing.T) {
	message := chatMessage{
		Role:    "assistant",
		ModelID: "claude-modern",
		Usages: []map[string]int{
			{"input_tokens": 100, "cache_creation_tokens": 40, "cache_read_tokens": 160, "output_tokens": 30},
		},
	}
	totals, ok := normalizedMessageUsage(message)
	if !ok {
		t.Fatal("expected usage to be reported")
	}
	if totals.InputTokens != 300 || totals.OutputTokens != 30 || totals.TotalTokens != 330 {
		t.Fatalf("totals=%+v want input=300 output=30 total=330", totals)
	}
	if totals.Other["cache_creation_tokens"] != 40 || totals.Other["cache_read_tokens"] != 160 {
		t.Fatalf("other=%+v want cache creation=40 read=160", totals.Other)
	}
}

func TestNormalizedMessageUsageDoesNotDoubleCountLegacyCachedTokens(t *testing.T) {
	message := chatMessage{
		Role:  "assistant",
		Usage: map[string]int{"input_tokens": 100, "cached_tokens": 80, "output_tokens": 30},
	}
	totals, ok := normalizedMessageUsage(message)
	if !ok {
		t.Fatal("expected usage to be reported")
	}
	if totals.InputTokens != 100 || totals.TotalTokens != 130 {
		t.Fatalf("totals=%+v want legacy input=100 total=130", totals)
	}
}

func TestNormalizedMessageUsageFallsBackToLegacyUsage(t *testing.T) {
	message := chatMessage{
		Role:    "assistant",
		ModelID: "legacy",
		Usage:   map[string]int{"input_tokens": 11, "output_tokens": 5},
	}
	totals, ok := normalizedMessageUsage(message)
	if !ok {
		t.Fatal("expected usage to be reported")
	}
	if totals.InputTokens != 11 || totals.OutputTokens != 5 || totals.TotalTokens != 16 {
		t.Fatalf("totals=%+v want input=11 output=5 total=16", totals)
	}
}

func TestRecordSessionUsageRefreshesStaleZeroTotals(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	updateTestConfig(t, s.CfgStore, func(cfg *config.AppConfig) {
		cfg.ChatDataDir = t.TempDir()
	})
	cfg := s.CfgStore.Snapshot()
	created := time.Date(2026, time.July, 25, 9, 0, 0, 0, time.Local).Unix()

	session := chatSession{
		ID:    "gamma",
		Title: "Gamma session",
		Messages: []chatMessage{{
			ID:        "m1",
			Role:      "assistant",
			ModelID:   "model-c",
			CreatedAt: created,
			Usages:    []map[string]int{{"input_tokens": 40, "output_tokens": 9}},
		}},
	}

	// Seed the ledger with a stale all-zero entry for the same key.
	stale := usageEntriesFromSession(session)
	if len(stale) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(stale))
	}
	stale[0].Totals = usageTotals{}
	stale[0].ModelID = ""
	if err := writeUsageLedger(cfg, usageLedger{Entries: stale}); err != nil {
		t.Fatalf("seed ledger: %v", err)
	}

	if err := s.recordSessionUsage(session); err != nil {
		t.Fatalf("record session usage: %v", err)
	}

	ledger, err := readUsageLedger(cfg)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if len(ledger.Entries) != 1 {
		t.Fatalf("entries=%+v want exactly 1", ledger.Entries)
	}
	entry := ledger.Entries[0]
	if entry.Totals.InputTokens != 40 || entry.Totals.OutputTokens != 9 {
		t.Fatalf("totals=%+v want input=40 output=9", entry.Totals)
	}
	if entry.ModelID != "model-c" {
		t.Fatalf("model_id=%q want model-c", entry.ModelID)
	}
}
