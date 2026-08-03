package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"genericagent-admin-go/internal/ga"
	"genericagent-admin-go/internal/modelconfig"
)

func TestOrderedAutonomousReviewModelsUsesPerModelSettings(t *testing.T) {
	modelRetries := 2
	modelReadTimeout := 17
	modelConnectTimeout := 4
	modelOrder := 1
	enabled := true
	profiles := []modelconfig.Profile{{
		VarName: "native_oai_config1",
		Type:    "native_oai",
		Name:    "审核服务商",
		APIBase: "https://example.test/v1",
		APIKey:  "secret",
		ModelConfigs: []modelconfig.ModelConfig{{
			Model:           "review-model",
			SortOrder:       &modelOrder,
			Enabled:         &enabled,
			MaxRetries:      &modelRetries,
			ReadTimeout:     &modelReadTimeout,
			ConnectTimeout:  &modelConnectTimeout,
			APIMode:         "responses",
			UserAgent:       "configured-review-client",
			ReasoningEffort: "max",
		}},
	}}

	models := orderedAutonomousReviewModels(profiles)
	if len(models) != 1 {
		t.Fatalf("models=%#v", models)
	}
	got := models[0]
	if got.Options.MaxRetries != modelRetries || got.Options.ReadTimeout != modelReadTimeout || got.Options.ConnectTimeout != modelConnectTimeout {
		t.Fatalf("retry settings=%+v", got.Options)
	}
	if got.Options.APIMode != "responses" || got.Options.UserAgent != "configured-review-client" || got.Options.ReasoningEffort != "max" {
		t.Fatalf("request settings=%+v", got.Options)
	}
}

func TestRunAutonomousReviewRequestRetriesTransientFailure(t *testing.T) {
	oldDelay := modelProbeRetryDelay
	oldSleep := modelProbeSleep
	modelProbeRetryDelay = func(int) time.Duration { return 0 }
	modelProbeSleep = func(time.Duration) <-chan time.Time {
		ready := make(chan time.Time, 1)
		ready <- time.Now()
		return ready
	}
	t.Cleanup(func() {
		modelProbeRetryDelay = oldDelay
		modelProbeSleep = oldSleep
	})

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer review-secret" || r.Header.Get("User-Agent") != "configured-review-client" {
			t.Errorf("headers authorization=%q user-agent=%q", r.Header.Get("Authorization"), r.Header.Get("User-Agent"))
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		if payload["model"] != "gpt-5.6-luna" || payload["reasoning_effort"] != "max" || payload["max_completion_tokens"] != float64(512) {
			t.Errorf("payload=%#v", payload)
		}
		if calls.Add(1) == 1 {
			http.Error(w, `{"error":{"message":"temporary network failure"}}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"decision\":\"approved\",\"confidence\":\"high\",\"reason\":\"证据充分\"}"}}]}`))
	}))
	defer server.Close()

	model := autonomousReviewModel{
		Profile: modelconfig.Profile{Type: "native_oai", APIBase: server.URL + "/v1", APIKey: "review-secret"},
		Config:  modelconfig.ModelConfig{Model: "gpt-5.6-luna"},
		Options: modelProbeOptions{MaxRetries: 1, ReadTimeout: 10, ConnectTimeout: 2, UserAgent: "configured-review-client", ReasoningEffort: "max", Configured: true},
	}
	reply, err := runAutonomousReviewRequest(context.Background(), model, ga.AutonomousApproval{ID: "approval-1", Title: "更新配置", Target: "mykey.py"})
	if err != nil {
		t.Fatalf("review request failed: %v", err)
	}
	if calls.Load() != 2 || !strings.Contains(reply.Text, `"decision":"approved"`) {
		t.Fatalf("calls=%d reply=%q", calls.Load(), reply.Text)
	}
}

func TestSaveAutonomousReviewFailurePersistsRetryState(t *testing.T) {
	root := t.TempDir()
	s := newGoalTestServer(t, root)
	before := time.Now()
	record, err := s.saveAutonomousReviewFailure(ga.AutonomousReviewRecord{ID: "approval-1", Attempts: 2}, errReviewTest{})
	if err != nil {
		t.Fatalf("save failure: %v", err)
	}
	if record.ReviewStatus != "fallback" || record.ReviewDecision != "needs_approval" || !record.NextRetryAt.After(before) {
		t.Fatalf("record=%+v", record)
	}
	if !strings.Contains(record.ReviewReason, "下次重新审核时会再次尝试") {
		t.Fatalf("reason=%q", record.ReviewReason)
	}
	stored, err := ga.LoadAutonomousReviews(root)
	if err != nil || len(stored) != 1 || stored[0].ID != "approval-1" {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
}

type errReviewTest struct{}

func (errReviewTest) Error() string { return "temporary provider failure" }
