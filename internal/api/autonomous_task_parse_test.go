package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAutonomousTaskParseFallsBackWithoutModel(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	result := requestAutonomousTask(t, h, http.MethodPost, "/api/autonomous/tasks/parse", `{"input":"每天早上九点整理一次收件箱"}`)
	if result["fallback"] != true {
		t.Fatalf("expected fallback response=%+v", result)
	}
	parsed := result["parsed"].(map[string]interface{})
	if parsed["title"] != "每天早上九点整理一次收件箱" || parsed["objective"] != "每天早上九点整理一次收件箱" {
		t.Fatalf("fallback draft=%+v", parsed)
	}
	if parsed["priority"] != "normal" {
		t.Fatalf("fallback priority=%v", parsed["priority"])
	}
}

func TestAutonomousTaskParseRequiresConfirmation(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/autonomous/tasks/parse", strings.NewReader(`{"input":"x"}`)))
	if rr.Code != http.StatusPreconditionRequired {
		t.Fatalf("unconfirmed parse status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAutonomousTaskParseRejectsBadInput(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/autonomous/tasks/parse", strings.NewReader(`{"input":"  "}`))
	req.Header.Set("Content-Type", "application/json")
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty input status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestParseAutonomousTaskDraft(t *testing.T) {
	draft, err := parseAutonomousTaskDraft("前置说明 {\"title\":\"整理收件箱\",\"objective\":\"每日定时清理\",\"priority\":\"HIGH\",\"risk\":\"\",\"project\":\"日常\",\"next_step\":\"先列出规则\"} 后置说明", "整理收件箱")
	if err != nil {
		t.Fatal(err)
	}
	if draft.Title != "整理收件箱" || draft.Priority != "high" || draft.NextStep != "先列出规则" {
		t.Fatalf("draft=%+v", draft)
	}
	if _, err := parseAutonomousTaskDraft("no json here", "input"); err == nil {
		t.Fatal("expected error for non-JSON reply")
	}
	if _, err := parseAutonomousTaskDraft(`{"title":""}`, "input"); err == nil {
		t.Fatal("expected error for empty title")
	}
	long := strings.Repeat("长", 300)
	draft, err = parseAutonomousTaskDraft(`{"title":"`+long+`"}`, "input")
	if err != nil {
		t.Fatal(err)
	}
	if runes := []rune(draft.Title); len(runes) != 200 {
		t.Fatalf("title not truncated: %d", len(runes))
	}
}
