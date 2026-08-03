package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	result := parseAutonomousReviewDecision("model preface\n{\"decision\":\"needs_approval\",\"confidence\":\"high\",\"reason\":\"approval evidence is missing\"}")
	if result.Decision != "needs_approval" || result.Confidence != "high" || result.Reason == "" {
		t.Fatalf("parsed review = %#v", result)
	}
	invalid := parseAutonomousReviewDecision("not json")
	if invalid.Decision != "needs_approval" || invalid.Reason != "not json" {
		t.Fatalf("invalid review should remain conservative: %#v", invalid)
	}
}
