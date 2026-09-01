package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestAutonomousApprovalReviewAutoApprovesEligibleModelResult(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	draft := "## 1. 补充只读检查说明\n" +
		"- 来源：R-test\n" +
		"- 要解决的问题：让后续执行有明确的只读检查说明\n" +
		"- 落地目标：memory/read_only_check.md\n" +
		"- 状态：待批未落地\n" +
		"- 核查证据：目标文件不存在，检查范围已明确\n" +
		"- 风险：低\n" +
		"- 下一步：用户批准后写入文档\n"
	if err := os.WriteFile(filepath.Join(root, "temp", "pending_drafts.md"), []byte(draft), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "temp", "TODO.txt"), []byte("# TODO\n"), 0644); err != nil {
		t.Fatal(err)
	}

	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("review endpoint=%q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer review-secret" {
			t.Errorf("authorization header=%q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"choices\":[{\"message\":{\"content\":\"{\\\"decision\\\":\\\"approved\\\",\\\"risk\\\":\\\"low\\\",\\\"confidence\\\":\\\"high\\\",\\\"reason\\\":\\\"证据充分且无需人工复核\\\",\\\"problem\\\":\\\"补充只读检查说明以便后续执行有统一依据\\\"}\"}}]}"))
	}))
	defer modelServer.Close()

	enabled, order := true, 0
	if _, err := modelconfig.Export(root, []modelconfig.Profile{{
		VarName: "native_oai_config_review",
		Type:    "native_oai",
		Name:    "审核模型",
		APIBase: modelServer.URL + "/v1",
		APIKey:  "review-secret",
		ModelConfigs: []modelconfig.ModelConfig{{
			Model:     "review-model",
			Enabled:   &enabled,
			SortOrder: &order,
		}},
	}}, true); err != nil {
		t.Fatalf("export review model: %v", err)
	}

	s := newGoalTestServer(t, root)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/autonomous/approvals/review", strings.NewReader("{\"automatic\":false}"))
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("review status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var automatic bool
	var reviewed, autoApproved int
	var overview ga.AutonomousApprovalOverview
	if err := json.Unmarshal(body["automatic"], &automatic); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body["reviewed"], &reviewed); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body["auto_approved"], &autoApproved); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body["overview"], &overview); err != nil {
		t.Fatal(err)
	}
	if automatic || reviewed != 1 || autoApproved != 1 || len(overview.Items) != 1 {
		t.Fatalf("review response automatic=%v reviewed=%d auto_approved=%d overview=%+v", automatic, reviewed, autoApproved, overview)
	}
	item := overview.Items[0]
	if item.State != "approved" || item.Decision != "approved" || item.DecisionSource != "model_auto" || item.ReviewStatus != "model" || item.ReviewRisk != "low" {
		t.Fatalf("reviewed item=%+v", item)
	}
	todo, err := os.ReadFile(filepath.Join(root, "temp", "TODO.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(todo), "模型自动批准") || !strings.Contains(string(todo), "ga-admin-approval:") {
		t.Fatalf("auto-approved TODO entry=%s", todo)
	}
	decisions, err := os.ReadFile(filepath.Join(root, "temp", "autonomous_approval_decisions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(decisions), "\"source\": \"model_auto\"") {
		t.Fatalf("decision source missing=%s", decisions)
	}
}

func TestShouldAutoApproveAutonomousReviewRequiresSafeEvidence(t *testing.T) {
	baseItem := ga.AutonomousApproval{State: "pending", Evidence: "目标和范围已核验"}
	baseRecord := ga.AutonomousReviewRecord{ReviewStatus: "model", ReviewDecision: "approved", ReviewRisk: "low", ReviewConfidence: "high", ReviewReason: "证据充分"}
	cases := []struct {
		name   string
		item   ga.AutonomousApproval
		record ga.AutonomousReviewRecord
		want   bool
	}{
		{name: "eligible", item: baseItem, record: baseRecord, want: true},
		{name: "high risk", item: baseItem, record: ga.AutonomousReviewRecord{ReviewStatus: "model", ReviewDecision: "approved", ReviewRisk: "high", ReviewConfidence: "high", ReviewReason: "证据充分"}},
		{name: "unknown risk", item: baseItem, record: ga.AutonomousReviewRecord{ReviewStatus: "model", ReviewDecision: "approved", ReviewRisk: "unknown", ReviewConfidence: "high", ReviewReason: "证据充分"}},
		{name: "low confidence", item: baseItem, record: ga.AutonomousReviewRecord{ReviewStatus: "model", ReviewDecision: "approved", ReviewRisk: "low", ReviewConfidence: "low", ReviewReason: "证据充分"}},
		{name: "missing evidence", item: ga.AutonomousApproval{State: "pending"}, record: baseRecord},
		{name: "explicit blocker", item: ga.AutonomousApproval{State: "pending", Evidence: "报告已阻塞，等待人工复核"}, record: baseRecord},
		{name: "negated manual review", item: baseItem, record: ga.AutonomousReviewRecord{ReviewStatus: "model", ReviewDecision: "approved", ReviewRisk: "low", ReviewConfidence: "high", ReviewReason: "证据充分，无需人工复核"}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldAutoApproveAutonomousReview(tc.item, tc.record); got != tc.want {
				t.Fatalf("should auto approve=%v, want %v", got, tc.want)
			}
		})
	}
}

type errReviewTest struct{}

func (errReviewTest) Error() string { return "temporary provider failure" }
