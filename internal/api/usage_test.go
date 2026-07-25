package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUsageOverviewAggregatesSessionsWithoutDoubleCounting(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	cfg := s.CfgStore.Cfg
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
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	cfg := s.CfgStore.Cfg
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

func TestUsageOverviewMissingDirectoryIsEmptyAndReadOnly(t *testing.T) {
	s := newGoalTestServer(t, t.TempDir())
	s.CfgStore.Cfg.ChatDataDir = t.TempDir()
	dir := chatSessionDir(s.CfgStore.Cfg)

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
