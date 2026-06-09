package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/modelconfig"
)

func TestGoalsStartRouteLaunchesReflectRunner(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "reflect"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reflect", "goal_mode.py"), []byte("# test reflect hook\n"), 0644); err != nil {
		t.Fatal(err)
	}
	agent := `import json, os, sys, time
state_path = os.environ.get('GOAL_STATE')
if state_path:
    with open(state_path, 'r', encoding='utf-8') as f:
        state = json.load(f)
    state['runner_args'] = sys.argv[1:]
    state['runner_cwd'] = os.getcwd()
    state['goal_state_env'] = state_path
    print('goal runner stdout marker', flush=True)
    print('goal runner stderr marker', file=sys.stderr, flush=True)
    with open(state_path, 'w', encoding='utf-8') as f:
        json.dump(state, f)
time.sleep(30)
`
	if err := os.WriteFile(filepath.Join(root, "agentmain.py"), []byte(agent), 0644); err != nil {
		t.Fatal(err)
	}
	h := newGoalTestServer(t, root).Routes()
	body := `{"objective":"route start smoke","budget_minutes":2,"max_turns":7,"llm_no":1}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/goals/start", strings.NewReader(body))
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("start status=%d want=200 body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		OK   bool `json:"ok"`
		Goal struct {
			ID            string `json:"id"`
			Objective     string `json:"objective"`
			BudgetSeconds int    `json:"budget_seconds"`
			MaxTurns      int    `json:"max_turns"`
			PID           int    `json:"pid"`
			Running       bool   `json:"running"`
			StateFile     string `json:"state_file"`
			LogFile       string `json:"log_file"`
			LLMNo         *int   `json:"llm_no"`
		} `json:"goal"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Goal.ID == "" || resp.Goal.PID <= 0 || !resp.Goal.Running || resp.Goal.Objective != "route start smoke" || resp.Goal.BudgetSeconds != 120 || resp.Goal.MaxTurns != 7 || resp.Goal.LLMNo == nil || *resp.Goal.LLMNo != 1 {
		t.Fatalf("unexpected start response: %#v body=%s", resp, rr.Body.String())
	}
	defer func() {
		stopBody := `{"id":"` + resp.Goal.ID + `","pid":` + strconv.Itoa(resp.Goal.PID) + `}`
		stopRR := httptest.NewRecorder()
		stopReq := httptest.NewRequest(http.MethodPost, "/api/goals/stop", strings.NewReader(stopBody))
		markDangerous(stopReq)
		h.ServeHTTP(stopRR, stopReq)
		if stopRR.Code != http.StatusOK {
			t.Fatalf("stop status=%d want=200 body=%s", stopRR.Code, stopRR.Body.String())
		}
	}()
	if !strings.HasPrefix(resp.Goal.StateFile, "temp/goals/") || !strings.HasSuffix(resp.Goal.StateFile, "/state.json") || !strings.HasPrefix(resp.Goal.LogFile, "temp/goals/") || !strings.HasSuffix(resp.Goal.LogFile, "/output.log") || strings.TrimSuffix(resp.Goal.StateFile, "/state.json") != strings.TrimSuffix(resp.Goal.LogFile, "/output.log") {
		t.Fatalf("unexpected relative paths: state=%q log=%q", resp.Goal.StateFile, resp.Goal.LogFile)
	}
	statePath := filepath.Join(root, filepath.FromSlash(resp.Goal.StateFile))
	for i := 0; i < 20; i++ {
		b, err := os.ReadFile(statePath)
		if err == nil && strings.Contains(string(b), "runner_args") {
			var state map[string]any
			if err := json.Unmarshal(b, &state); err != nil {
				t.Fatal(err)
			}
			args, ok := state["runner_args"].([]any)
			if !ok || len(args) != 4 || args[0] != "--reflect" || args[1] != "reflect/goal_mode.py" || args[2] != "--llm_no" || args[3] != "1" {
				t.Fatalf("unexpected runner args in state: %#v", state["runner_args"])
			}
			if state["runner_cwd"] != root || state["goal_state_env"] != statePath {
				t.Fatalf("unexpected runner environment: cwd=%#v goal_state=%#v want cwd=%q goal_state=%q", state["runner_cwd"], state["goal_state_env"], root, statePath)
			}
			logBytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(resp.Goal.LogFile)))
			if err != nil {
				t.Fatal(err)
			}
			logText := string(logBytes)
			if !strings.Contains(logText, "goal runner stdout marker") || !strings.Contains(logText, "goal runner stderr marker") {
				t.Fatalf("runner log did not capture stdout/stderr: %q", logText)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("runner did not update state file %s", statePath)
}

func TestGoalsStartRejectsTooLargeBudgetMinutesBeforeLaunch(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "reflect"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reflect", "goal_mode.py"), []byte("# test reflect hook\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agentmain.py"), []byte("raise SystemExit('should not launch')\n"), 0644); err != nil {
		t.Fatal(err)
	}

	h := newGoalTestServer(t, root).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/goals/start", strings.NewReader(`{"objective":"too long","budget_minutes":43201}`))
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("start status=%d want=400 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "43200") {
		t.Fatalf("unexpected error body: %s", rr.Body.String())
	}
	matches, err := filepath.Glob(filepath.Join(root, "temp", "goal_admin_*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("too-large budget should not create goal state files: %v", matches)
	}
}

func TestGoalsStartRejectsNegativeBudgetSecondsBeforeLaunch(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "reflect"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reflect", "goal_mode.py"), []byte("# test reflect hook\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agentmain.py"), []byte("raise SystemExit('should not launch')\n"), 0644); err != nil {
		t.Fatal(err)
	}

	h := newGoalTestServer(t, root).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/goals/start", strings.NewReader(`{"objective":"negative seconds","budget_seconds":-5,"budget_minutes":1}`))
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("start status=%d want=400 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "budget_seconds") {
		t.Fatalf("unexpected error body: %s", rr.Body.String())
	}
	matches, err := filepath.Glob(filepath.Join(root, "temp", "goal_admin_*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("negative budget should not create goal state files: %v", matches)
	}
}

func TestGoalsStartRejectsMultipleJSONValuesBeforeLaunch(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "reflect"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reflect", "goal_mode.py"), []byte("# test reflect hook\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agentmain.py"), []byte("raise SystemExit('should not launch')\n"), 0644); err != nil {
		t.Fatal(err)
	}

	h := newGoalTestServer(t, root).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/goals/start", strings.NewReader(`{"objective":"multi json"}{"objective":"second"}`))
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("start status=%d want=400 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "single JSON value") {
		t.Fatalf("unexpected error body: %s", rr.Body.String())
	}
	matches, err := filepath.Glob(filepath.Join(root, "temp", "goal_admin_*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("multi-json request should not create goal state files: %v", matches)
	}
}

func TestGoalsDangerousRoutesRequireConfirmHeader(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/goals/start", strings.NewReader(`{"objective":"needs confirm","budget_seconds":60}`))
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusPreconditionRequired {
		t.Fatalf("start status=%d want=428 body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "X-GA-Confirm") {
		t.Fatalf("unexpected error body: %s", rr.Body.String())
	}
}

func TestGoalsListAndOutputRoutes(t *testing.T) {
	gaRoot := t.TempDir()
	temp := filepath.Join(gaRoot, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	state := `{"objective":"route objective","budget_seconds":90,"start_time":` + startTimeJSON(time.Now()) + `,"turns_used":2,"max_turns":9,"status":"done"}`
	if err := os.WriteFile(filepath.Join(temp, "goal_admin_route_1.json"), []byte(state), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temp, "goal_admin_route_1.log"), []byte("abcdefghijklmnopqrstuvwxyz"), 0644); err != nil {
		t.Fatal(err)
	}
	missingLogState := `{"objective":"route missing log","budget_seconds":90,"start_time":` + startTimeJSON(time.Now()) + `,"turns_used":0,"max_turns":9,"status":"running"}`
	if err := os.WriteFile(filepath.Join(temp, "goal_admin_route_missing_log.json"), []byte(missingLogState), 0644); err != nil {
		t.Fatal(err)
	}
	emptyLogState := `{"objective":"route empty log","budget_seconds":90,"start_time":` + startTimeJSON(time.Now()) + `,"turns_used":1,"max_turns":9,"status":"running"}`
	if err := os.WriteFile(filepath.Join(temp, "goal_admin_route_empty_log.json"), []byte(emptyLogState), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temp, "goal_admin_route_empty_log.log"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	h := newGoalTestServer(t, gaRoot).Routes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/goals/list", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", rr.Code, rr.Body.String())
	}
	var list struct {
		Goals []struct {
			ID            string   `json:"id"`
			Objective     string   `json:"objective"`
			Status        string   `json:"status"`
			StopLevel     string   `json:"stop_level"`
			Actions       []string `json:"actions"`
			RawStatus     string   `json:"raw_status"`
			LastEvent     string   `json:"last_event"`
			ErrorClass    string   `json:"error_class"`
			StateFile     string   `json:"state_file"`
			LogFile       string   `json:"log_file"`
			LogExists     bool     `json:"log_exists"`
			MissingLog    bool     `json:"missing_log"`
			StateReadable bool     `json:"state_readable"`
		} `json:"goals"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Goals) != 3 {
		t.Fatalf("unexpected list count: %s", rr.Body.String())
	}
	seen := map[string]struct{}{}
	healthyRouteFlag := false
	missingLogFlag := false
	emptyLogFlag := false
	for _, g := range list.Goals {
		seen[g.ID] = struct{}{}
		if g.Status == "" || len(g.Actions) == 0 || g.StopLevel == "" || !g.StateReadable {
			t.Fatalf("expected normalized goal contract fields in response: %#v", g)
		}
		if !strings.HasPrefix(g.StateFile, "temp/") || !strings.HasPrefix(g.LogFile, "temp/") {
			t.Fatalf("expected relative files in response: %#v", g)
		}
		switch g.ID {
		case "route_1":
			healthyRouteFlag = g.Objective != "" && g.Status == "done" && g.RawStatus == "done" && g.LastEvent == "done" && g.ErrorClass == ""
		case "route_missing_log":
			missingLogFlag = g.MissingLog && !g.LogExists && g.Status == "unknown" && g.RawStatus == "running" && g.LastEvent != "" && g.ErrorClass != ""
		case "route_empty_log":
			emptyLogFlag = !g.MissingLog && g.LogExists && g.Status == "unknown" && g.RawStatus == "running" && g.LastEvent != "" && g.ErrorClass != ""
		}
	}
	if _, ok := seen["route_1"]; !ok || !healthyRouteFlag {
		t.Fatalf("missing healthy route_1 in list response: %s", rr.Body.String())
	}
	if _, ok := seen["route_missing_log"]; !ok {
		t.Fatalf("missing route_missing_log in list response: %s", rr.Body.String())
	}
	if _, ok := seen["route_empty_log"]; !ok {
		t.Fatalf("missing route_empty_log in list response: %s", rr.Body.String())
	}
	if !emptyLogFlag {
		t.Fatalf("expected existing empty-log flags for route_empty_log: %s", rr.Body.String())
	}
	if !missingLogFlag {
		t.Fatalf("expected missing-log flags for route_missing_log: %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/goals/output?id=route_1&max_bytes=5", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("output status = %d body=%s", rr.Code, rr.Body.String())
	}
	type goalOutputResp struct {
		Output           string `json:"output"`
		Truncated        bool   `json:"truncated"`
		BytesReturned    int64  `json:"bytes_returned"`
		TotalBytes       int64  `json:"total_bytes"`
		LinesReturned    int64  `json:"lines_returned"`
		TotalLines       int64  `json:"total_lines"`
		RequestedBytes   int64  `json:"requested_bytes"`
		MaxBytes         int64  `json:"max_bytes"`
		DefaultBytes     int64  `json:"default_bytes"`
		DefaultBytesUsed bool   `json:"default_bytes_used"`
		MaxBytesCapped   bool   `json:"max_bytes_capped"`
		OutputStatus     string `json:"output_status"`
		Goal             struct {
			ID         string   `json:"id"`
			Status     string   `json:"status"`
			StopLevel  string   `json:"stop_level"`
			Actions    []string `json:"actions"`
			RawStatus  string   `json:"raw_status"`
			LastEvent  string   `json:"last_event"`
			ErrorClass string   `json:"error_class"`
		} `json:"goal"`
	}
	var out goalOutputResp
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Output != "vwxyz" || out.Goal.ID != "route_1" || out.Goal.Status != "done" || out.Goal.StopLevel == "" || len(out.Goal.Actions) == 0 || out.Goal.RawStatus != "done" || out.Goal.LastEvent != "done" || out.Goal.ErrorClass != "" || !out.Truncated || out.BytesReturned != 5 || out.TotalBytes != 26 || out.RequestedBytes != 5 || out.MaxBytes != 5 || out.DefaultBytes != 64*1024 || out.DefaultBytesUsed || out.MaxBytesCapped || out.OutputStatus != "tail_truncated" {
		t.Fatalf("unexpected output response: %#v body=%s", out, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/goals/output?id=route_1&max_bytes=0", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("output max_bytes=0 status = %d body=%s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Output != "abcdefghijklmnopqrstuvwxyz" || out.Truncated || out.BytesReturned != 26 || out.RequestedBytes != 0 || out.MaxBytes != 64*1024 || out.DefaultBytes != 64*1024 || !out.DefaultBytesUsed || out.MaxBytesCapped || out.OutputStatus != "full" {
		t.Fatalf("unexpected default output response for max_bytes=0: %#v body=%s", out, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/goals/output?id=route_missing_log&max_bytes=5", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("missing-log output status = %d body=%s", rr.Code, rr.Body.String())
	}
	out = goalOutputResp{}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Output != "" || out.Goal.ID != "route_missing_log" || out.Goal.Status != "unknown" || out.Goal.RawStatus != "running" || out.Goal.LastEvent == "" || out.Goal.ErrorClass == "" || out.RequestedBytes != 5 || out.MaxBytes != 5 || out.DefaultBytes != 64*1024 || out.DefaultBytesUsed || out.MaxBytesCapped || out.OutputStatus != "missing_log" {
		t.Fatalf("unexpected missing-log output response: %#v body=%s", out, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/goals/output?id=route_empty_log&max_bytes=5", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("empty-log output status = %d body=%s", rr.Code, rr.Body.String())
	}
	out = goalOutputResp{}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Output != "" || out.Goal.ID != "route_empty_log" || out.Goal.Status != "unknown" || out.Goal.RawStatus != "running" || out.Goal.LastEvent == "" || out.Goal.ErrorClass == "" || out.Truncated || out.BytesReturned != 0 || out.TotalBytes != 0 || out.LinesReturned != 0 || out.TotalLines != 0 || out.RequestedBytes != 5 || out.MaxBytes != 5 || out.DefaultBytes != 64*1024 || out.DefaultBytesUsed || out.MaxBytesCapped || out.OutputStatus != "empty_log" {
		t.Fatalf("unexpected empty-log output response: %#v body=%s", out, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/goals/output?id=route_1", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("default output status = %d body=%s", rr.Code, rr.Body.String())
	}
	out = goalOutputResp{}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Output != "abcdefghijklmnopqrstuvwxyz" || out.Truncated || out.BytesReturned != 26 || out.TotalBytes != 26 || out.LinesReturned != 1 || out.TotalLines != 1 || out.RequestedBytes != 0 || out.MaxBytes != 64*1024 || out.DefaultBytes != 64*1024 || !out.DefaultBytesUsed || out.MaxBytesCapped || out.OutputStatus != "full" {
		t.Fatalf("unexpected default output response: %#v body=%s", out, rr.Body.String())
	}
	if out.Goal.ID != "route_1" {
		t.Fatalf("unexpected goal id in output response: %#v", out.Goal)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/goals/output?id=route_1&max_bytes=2097152", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("capped output status = %d body=%s", rr.Code, rr.Body.String())
	}
	out = goalOutputResp{}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Output != "abcdefghijklmnopqrstuvwxyz" || out.Truncated || out.LinesReturned != 1 || out.TotalLines != 1 || out.RequestedBytes != 2097152 || out.MaxBytes != 1024*1024 || out.DefaultBytes != 64*1024 || out.DefaultBytesUsed || !out.MaxBytesCapped || out.OutputStatus != "full" {
		t.Fatalf("unexpected capped output response: %#v body=%s", out, rr.Body.String())
	}
}

func TestGoalsRouteMethodsAndBadInput(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	cases := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{http.MethodPost, "/api/goals/list", "", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/goals/start", "", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/goals/start", `{"objective":"","budget_seconds":60}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/start", `{"objective":"ok","budget_seconds":0}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/start", `{"objective":"ok","budget_seconds":2592001}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/start", `{"objective":"ok","budget_seconds":60,"budget_minutes":1}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/start", `{"objective":"ok","budget_seconds":60,"budget_minutes":2}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/start", `{"objective":"ok","budget_minutes":43201}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/start", `{"objective":"ok","budget_minutes":9223372036854775807}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/start", `{"objective":"ok","budget_seconds":60} {"objective":"second","budget_seconds":60}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/start", `{"objective":"ok","budget_seconds":60,"max_turns":-1}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/start", `{"objective":"ok","budget_seconds":60,"max_turns":10001}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/start", `{"objective":"ok","budget_seconds":60,"llm_no":-1}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/start", `{"objective":"ok","budget_minutes":0}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/start", `{"objective":"ok","budget_minutes":-5}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/stop", `{"id":"missing","pid":0}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/stop", `{"id":"../missing","pid":12345}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/stop", `{"id":"missing","pid":12345} {"id":"other","pid":12345}`, http.StatusBadRequest},
		{http.MethodPost, "/api/goals/stop", `{"id":"missing","pid":12345}`, http.StatusBadRequest},
		{http.MethodGet, "/api/goals/output?id=../missing&max_bytes=100", "", http.StatusBadRequest},
		{http.MethodGet, "/api/goals/output?id=missing&max_bytes=abc", "", http.StatusBadRequest},
		{http.MethodGet, "/api/goals/output?id=missing&max_bytes=-1", "", http.StatusBadRequest},
		{http.MethodGet, "/api/goals/output?id=missing", "", http.StatusBadRequest},
	}
	for _, tc := range cases {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.method == http.MethodPost && (strings.HasPrefix(tc.path, "/api/goals/start") || strings.HasPrefix(tc.path, "/api/goals/stop")) {
			markDangerous(req)
		}
		h.ServeHTTP(rr, req)
		if rr.Code != tc.want {
			t.Fatalf("%s %s status=%d want=%d body=%s", tc.method, tc.path, rr.Code, tc.want, rr.Body.String())
		}
	}
}

func TestGoalsStopRouteRejectsEndedStateWithStalePID(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	start := float64(time.Now().Add(-time.Minute).Unix())
	end := start + 3
	state := map[string]interface{}{
		"objective":      "already done",
		"budget_seconds": 60,
		"start_time":     start,
		"end_time":       end,
		"max_turns":      3,
		"status":         "done",
		"pid":            99999,
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(temp, "goal_admin_ended_api.json")
	if err := os.WriteFile(statePath, b, 0644); err != nil {
		t.Fatal(err)
	}

	h := newGoalTestServer(t, root).Routes()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/goals/stop", strings.NewReader(`{"id":"ended_api","pid":99999}`))
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "goal is not running") {
		t.Fatalf("stop ended route status=%d body=%s", rr.Code, rr.Body.String())
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(b) {
		t.Fatalf("ended state should remain unchanged\nbefore=%s\nafter=%s", b, after)
	}
}

func TestDangerousMutationRoutesRejectGET(t *testing.T) {
	h := newGoalTestServer(t, t.TempDir()).Routes()
	for _, path := range []string{
		"/api/services/start",
		"/api/services/stop",
		"/api/services/stop-all",
		"/api/models/export",
	} {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("GET %s status=%d want=%d body=%s", path, rr.Code, http.StatusMethodNotAllowed, rr.Body.String())
			}
		})
	}
}

func markDangerous(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GA-Confirm", "dangerous")
}

func newGoalTestServer(t *testing.T, gaRoot string) *Server {
	t.Helper()
	cfg := &config.Store{Root: t.TempDir(), Cfg: config.Default()}
	cfg.Cfg.GARoot = gaRoot
	models := modelconfig.NewStore(t.TempDir())
	return New(cfg, nil, models, nil)
}

func startTimeJSON(t time.Time) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(float64(t.UnixNano())/1e9, 'f', 6, 64), "0"), ".")
}
