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
	"genericagent-admin-go/internal/ga"
)

func TestAutonomousApprovalsGetAndApprove(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0755); err != nil {
		t.Fatal(err)
	}
	source := "## 1. review proposal\n- 状态：待批未落地\n- 风险：低\n- 下一步：用户批准后执行\n"
	if err := os.WriteFile(filepath.Join(root, "temp", "pending_drafts.md"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	s := newGoalTestServer(t, root)
	h := s.Routes()

	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, httptest.NewRequest(http.MethodGet, "/api/autonomous/approvals", nil))
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var overview ga.AutonomousApprovalOverview
	if err := json.Unmarshal(getRR.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if overview.Pending != 1 || len(overview.Items) != 1 {
		t.Fatalf("overview=%+v", overview)
	}

	body := `{"id":"` + overview.Items[0].ID + `","decision":"approved"}`
	postRR := httptest.NewRecorder()
	postReq := httptest.NewRequest(http.MethodPost, "/api/autonomous/approvals", strings.NewReader(body))
	postReq.Header.Set("Content-Type", "application/json")
	markDangerous(postReq)
	h.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK || !strings.Contains(postRR.Body.String(), `"queued":true`) {
		t.Fatalf("POST status=%d body=%s", postRR.Code, postRR.Body.String())
	}
	if !strings.Contains(postRR.Body.String(), `"execution_state":"queued"`) {
		t.Fatalf("POST should expose queued execution state: %s", postRR.Body.String())
	}
}

func TestAutonomousModelReviewParsingIsStructuredAndConservative(t *testing.T) {
	parsed := parseAutonomousModelReviews("model preface\n[{\"id\":\"draft-a\",\"decision\":\"needs_approval\",\"confidence\":\"high\",\"reason\":\"approval evidence is missing\"}]")
	result, ok := parsed["draft-a"]
	if !ok || result.Decision != "needs_approval" || result.Confidence != "high" || result.Reason == "" || result.Err != "" {
		t.Fatalf("parsed review = %#v", parsed)
	}
	if got := parseAutonomousModelReviews("not json"); len(got) != 0 {
		t.Fatalf("invalid review should be ignored: %#v", got)
	}
}

func TestAutonomousModelReviewBatchUsesSelectedModel(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "agentmain.py"), []byte("# test runtime\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "temp", "autonomous_reports"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "temp", "autonomous_reports", "R49.md"), []byte("# blocked\napproval evidence is missing\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Store{Root: t.TempDir(), Cfg: config.Default()}
	cfg.Cfg.GARoot = root
	s := &Server{CfgStore: cfg}
	old := runAutonomousReviewWorkerFunc
	defer func() { runAutonomousReviewWorkerFunc = old }()
	calls := 0
	runAutonomousReviewWorkerFunc = func(_ config.AppConfig, _ string, req map[string]interface{}) (chatMessage, error) {
		calls++
		if req["llm_no"] != 2 {
			t.Fatalf("request llm_no = %#v", req["llm_no"])
		}
		return chatMessage{Content: `[{"id":"draft-r49","decision":"needs_approval","confidence":"high","reason":"approval evidence is missing"}]`}, nil
	}
	modTime := time.Now()
	items := []ga.AutonomousApproval{{ID: "draft-r49", Title: "blocked", ReviewReport: &ga.Entry{Path: "temp/autonomous_reports/R49.md", ModTime: modTime}}}
	result := s.autonomousModelReviews(autonomousReviewModel{LLMNo: 2, Model: "model-2"}, items, []int{0})
	if calls != 1 || result["draft-r49"].Decision != "needs_approval" || result["draft-r49"].Err != "" {
		t.Fatalf("batch review calls=%d result=%#v", calls, result)
	}
}
