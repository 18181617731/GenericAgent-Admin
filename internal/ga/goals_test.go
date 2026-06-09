package ga

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSanitizeGoalID(t *testing.T) {
	got := sanitizeGoalID(" ../goal-admin_01!!中文 ")
	if got != "goal-admin_01" {
		t.Fatalf("sanitizeGoalID() = %q", got)
	}
}

func TestListGoalsSkipsInvalidAndSorts(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	older := filepath.Join(temp, goalStatePrefix+"older.json")
	newer := filepath.Join(temp, goalStatePrefix+"newer.json")
	invalid := filepath.Join(temp, goalStatePrefix+"bad.json")
	writeStateForTest(t, older, GoalState{Objective: "old", BudgetSeconds: 60, StartTime: float64(time.Now().Add(-time.Minute).Unix()), MaxTurns: 10, Status: "running"})
	writeStateForTest(t, newer, GoalState{Objective: "new", BudgetSeconds: 120, StartTime: float64(time.Now().Unix()), MaxTurns: 20, Status: "done"})
	if err := os.WriteFile(invalid, []byte("{"), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(older, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, now, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(invalid, now.Add(time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	items, err := ListGoals(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3 including corrupt placeholder: %#v", len(items), items)
	}
	if items[0].ID != "bad" || items[1].ID != "newer" || items[2].ID != "older" {
		t.Fatalf("items not sorted by mtime desc: %#v", items)
	}
	if items[0].Status != "unknown" || items[0].ErrorClass != "state_corrupt" || items[0].StateReadable {
		t.Fatalf("corrupt state should be recoverable placeholder: %#v", items[0])
	}
	if !strings.HasPrefix(items[1].StateFile, "temp/") || !strings.HasPrefix(items[1].LogFile, "temp/") {
		t.Fatalf("paths should be GA-root relative slash paths: %#v", items[1])
	}
}

func TestMetaFromStateUsesEndTimeForFinishedRuns(t *testing.T) {
	root := t.TempDir()
	start := float64(time.Now().Add(-2 * time.Hour).Unix())
	end := start + 90
	meta := metaFromState(root, "finished", filepath.Join(root, "temp", goalStatePrefix+"finished.json"), filepath.Join(root, "temp", goalStatePrefix+"finished.log"), GoalState{Objective: "finished", BudgetSeconds: 600, StartTime: start, EndTime: &end, TurnsUsed: 2, MaxTurns: 5, Status: "done", PID: os.Getpid()}, time.Now())
	if meta.ElapsedSeconds < 89 || meta.ElapsedSeconds > 91 {
		t.Fatalf("ElapsedSeconds = %d, want about 90 from end_time-start_time", meta.ElapsedSeconds)
	}
	if meta.RemainingSeconds < 509 || meta.RemainingSeconds > 511 {
		t.Fatalf("RemainingSeconds = %d, want about 510", meta.RemainingSeconds)
	}
	if meta.BudgetPercent != 15 || meta.TurnPercent != 40 {
		t.Fatalf("progress percent = budget %d turn %d, want 15/40", meta.BudgetPercent, meta.TurnPercent)
	}
	if meta.Running {
		t.Fatalf("finished goal with end_time must not be reported running even if stale pid is alive: %#v", meta)
	}
}

func TestMetaFromStateFreezesExitedRunningStateAtModTime(t *testing.T) {
	root := t.TempDir()
	startTime := time.Now().Add(-2 * time.Hour)
	modTime := startTime.Add(2 * time.Minute)
	meta := metaFromState(root, "stale", filepath.Join(root, "temp", goalStatePrefix+"stale.json"), filepath.Join(root, "temp", goalStatePrefix+"stale.log"), GoalState{Objective: "stale", BudgetSeconds: 600, StartTime: float64(startTime.Unix()), TurnsUsed: 6, MaxTurns: 5, Status: "running", PID: 0}, modTime)
	if meta.Status != "unknown" || meta.RawStatus != "running" || meta.ErrorClass != "stale_running" || meta.LastEvent != "stale_running" || meta.Running {
		t.Fatalf("stale running state should be reported as unknown stale/not running: %#v", meta)
	}
	if meta.ElapsedSeconds < 119 || meta.ElapsedSeconds > 121 {
		t.Fatalf("ElapsedSeconds = %d, want frozen near 120 from mod_time-start_time", meta.ElapsedSeconds)
	}
	if meta.RemainingSeconds < 479 || meta.RemainingSeconds > 481 {
		t.Fatalf("RemainingSeconds = %d, want frozen near 480", meta.RemainingSeconds)
	}
	if meta.BudgetPercent != 20 || meta.TurnPercent != 100 {
		t.Fatalf("progress percent = budget %d turn %d, want 20/100", meta.BudgetPercent, meta.TurnPercent)
	}
}

func TestMetaFromStateClampsNegativeProgressInputs(t *testing.T) {
	root := t.TempDir()
	future := float64(time.Now().Add(time.Hour).Unix())
	meta := metaFromState(root, "future", filepath.Join(root, "temp", goalStatePrefix+"future.json"), filepath.Join(root, "temp", goalStatePrefix+"future.log"), GoalState{Objective: "future", BudgetSeconds: 60, StartTime: future, TurnsUsed: -3, MaxTurns: 10, Status: "done"}, time.Now())
	if meta.ElapsedSeconds != 0 || meta.RemainingSeconds != 60 {
		t.Fatalf("future start should clamp elapsed/remaining, got elapsed=%d remaining=%d", meta.ElapsedSeconds, meta.RemainingSeconds)
	}
	if meta.BudgetPercent != 0 || meta.TurnPercent != 0 {
		t.Fatalf("negative progress should clamp percents to zero: %#v", meta)
	}
	if meta.TurnsUsed != 0 {
		t.Fatalf("negative turns_used should be reported as zero, got %d", meta.TurnsUsed)
	}
}

func TestMetaFromStateClampsTurnsUsedAboveMax(t *testing.T) {
	root := t.TempDir()
	meta := metaFromState(root, "over", filepath.Join(root, "temp", goalStatePrefix+"over.json"), filepath.Join(root, "temp", goalStatePrefix+"over.log"), GoalState{Objective: "over", BudgetSeconds: 60, StartTime: float64(time.Now().Unix()), TurnsUsed: 12, MaxTurns: 5, Status: "done"}, time.Now())
	if meta.TurnsUsed != 5 || meta.TurnPercent != 100 {
		t.Fatalf("turns_used above max should be reported at cap with 100%% progress: %#v", meta)
	}
}

func TestGoalOutputTailsLogAndDefaults(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	id := "abc_123"
	statePath := filepath.Join(temp, goalStatePrefix+id+".json")
	logPath := filepath.Join(temp, goalStatePrefix+id+".log")
	writeStateForTest(t, statePath, GoalState{Objective: "tail", BudgetSeconds: 30, StartTime: float64(time.Now().Unix()), MaxTurns: 5, Status: "running"})
	if err := os.WriteFile(logPath, []byte("0123456789"), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := GoalOutput(root, id, 4)
	out, meta := res.Output, res.Goal
	if err != nil {
		t.Fatal(err)
	}
	if out != "6789" {
		t.Fatalf("tail = %q, want 6789", out)
	}
	if !res.Truncated || res.OutputStatus != "tail_truncated" || res.BytesReturned != 4 || res.TotalBytes != 10 || res.LinesReturned != 1 || res.TotalLines != 1 || res.RequestedBytes != 4 || res.MaxBytes != 4 || res.DefaultBytes != defaultGoalOutputBytes || res.DefaultBytesUsed || res.MaxBytesCapped {
		t.Fatalf("unexpected output metadata for tail: %#v", res)
	}
	if meta.ID != id || meta.Objective != "tail" || !meta.LogExists || meta.MissingLog {
		t.Fatalf("unexpected meta/log flags: %#v", meta)
	}

	res, err = GoalOutput(root, id, 10)
	out = res.Output
	if err != nil {
		t.Fatal(err)
	}
	if out != "0123456789" {
		t.Fatalf("exact-size tail = %q, want full log", out)
	}
	if res.Truncated || res.OutputStatus != "full" || res.BytesReturned != 10 || res.TotalBytes != 10 || res.LinesReturned != 1 || res.TotalLines != 1 || res.RequestedBytes != 10 || res.MaxBytes != 10 || res.DefaultBytes != defaultGoalOutputBytes || res.DefaultBytesUsed || res.MaxBytesCapped {
		t.Fatalf("unexpected output metadata for full tail: %#v", res)
	}

	res, err = GoalOutput(root, id, 0)
	out = res.Output
	if err != nil {
		t.Fatal(err)
	}
	if out != "0123456789" {
		t.Fatalf("default max_bytes output = %q, want full small log", out)
	}
	if res.RequestedBytes != 0 || res.MaxBytes != defaultGoalOutputBytes || res.DefaultBytes != defaultGoalOutputBytes || !res.DefaultBytesUsed || res.MaxBytesCapped {
		t.Fatalf("expected default max_bytes metadata: %#v", res)
	}

	res, err = GoalOutput(root, id, int64(maxGoalOutputBytes+1024))
	out = res.Output
	if err != nil {
		t.Fatal(err)
	}
	if out != "0123456789" {
		t.Fatalf("oversized max_bytes should be capped without changing small output, got %q", out)
	}
	if res.RequestedBytes != int64(maxGoalOutputBytes+1024) || res.MaxBytes != maxGoalOutputBytes || res.DefaultBytes != defaultGoalOutputBytes || !res.MaxBytesCapped {
		t.Fatalf("expected cap metadata for oversized max_bytes: %#v", res)
	}
}

func TestGoalOutputTailsLogWithoutSplittingUTF8(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	id := "utf8_tail"
	statePath := filepath.Join(temp, goalStatePrefix+id+".json")
	logPath := filepath.Join(temp, goalStatePrefix+id+".log")
	writeStateForTest(t, statePath, GoalState{Objective: "utf8", BudgetSeconds: 30, StartTime: float64(time.Now().Unix()), MaxTurns: 5, Status: "done"})
	if err := os.WriteFile(logPath, []byte("prefix界TAIL"), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := GoalOutput(root, id, 6)
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "TAIL" {
		t.Fatalf("UTF-8 aligned tail = %q, want TAIL", res.Output)
	}
	if !res.Truncated || res.TotalBytes != int64(len([]byte("prefix界TAIL"))) || res.BytesReturned != int64(len([]byte("TAIL"))) || res.LinesReturned != 1 || res.TotalLines != 1 {
		t.Fatalf("unexpected UTF-8 tail metadata: %#v", res)
	}
}

func TestGoalOutputAllowsMissingLog(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	id := "missing_log"
	writeStateForTest(t, filepath.Join(temp, goalStatePrefix+id+".json"), GoalState{Objective: "missing log", BudgetSeconds: 30, StartTime: float64(time.Now().Unix()), MaxTurns: 5, Status: "done"})

	res, err := GoalOutput(root, id, 100)
	out, meta := res.Output, res.Goal
	if err != nil {
		t.Fatalf("GoalOutput error = %v, want nil for a not-yet-created log", err)
	}
	if out != "" || res.OutputStatus != "missing_log" || meta.ID != id || meta.LogFile != filepath.ToSlash(filepath.Join("temp", goalStatePrefix+id+".log")) || meta.LogExists || !meta.MissingLog || res.DefaultBytesUsed {
		t.Fatalf("unexpected output/meta for missing log: out=%q meta=%#v", out, meta)
	}
}

func TestGoalOutputRejectsSymlinkLogOutsideTemp(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "secret.log")
	if err := os.WriteFile(secret, []byte("do-not-read"), 0644); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(temp, "linked-out")
	if err := os.Symlink(outside, linkDir); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	id := "symlink_log"
	statePath := filepath.Join(temp, goalStatePrefix+id+".json")
	writeStateForTest(t, statePath, GoalState{Objective: "symlink", BudgetSeconds: 30, StartTime: float64(time.Now().Unix()), MaxTurns: 5, Status: "done", LogFile: filepath.ToSlash(filepath.Join("temp", "linked-out", "secret.log"))})

	res, err := GoalOutput(root, id, 100)
	if err != nil {
		t.Fatalf("GoalOutput should fall back to default missing log instead of reading symlink target: %v", err)
	}
	if res.Output != "" || res.OutputStatus != "missing_log" || strings.Contains(res.Output, "do-not-read") {
		t.Fatalf("symlinked log outside temp should not be read: %#v", res)
	}
	if res.Goal.LogFile != filepath.ToSlash(filepath.Join("temp", goalStatePrefix+id+".log")) || res.Goal.LogExists || !res.Goal.MissingLog {
		t.Fatalf("unexpected fallback log metadata: %#v", res.Goal)
	}
}

func TestGoalOutputEmptyLogMetadata(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	id := "empty_log"
	writeStateForTest(t, filepath.Join(temp, goalStatePrefix+id+".json"), GoalState{Objective: "empty log", BudgetSeconds: 30, StartTime: float64(time.Now().Unix()), MaxTurns: 5, Status: "done"})
	if err := os.WriteFile(filepath.Join(temp, goalStatePrefix+id+".log"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	res, err := GoalOutput(root, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "" || res.OutputStatus != "empty_log" || res.Truncated || res.BytesReturned != 0 || res.TotalBytes != 0 || res.LinesReturned != 0 || res.TotalLines != 0 {
		t.Fatalf("unexpected empty-log output metadata: %#v", res)
	}
	if !res.Goal.LogExists || res.Goal.MissingLog || !res.DefaultBytesUsed || res.RequestedBytes != 0 || res.MaxBytes != defaultGoalOutputBytes {
		t.Fatalf("unexpected empty-log request/meta fields: %#v", res)
	}
}

func TestGoalOutputCapsMaxBytes(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	id := "big_tail"
	statePath := filepath.Join(temp, goalStatePrefix+id+".json")
	logPath := filepath.Join(temp, goalStatePrefix+id+".log")
	writeStateForTest(t, statePath, GoalState{Objective: "big", BudgetSeconds: 30, StartTime: float64(time.Now().Unix()), MaxTurns: 5, Status: "done"})
	log := strings.Repeat("a", maxGoalOutputBytes) + "TAIL"
	if err := os.WriteFile(logPath, []byte(log), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := GoalOutput(root, id, int64(maxGoalOutputBytes+1024))
	out := res.Output
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != maxGoalOutputBytes || strings.HasPrefix(out, "aaaaTAIL") || !strings.HasSuffix(out, "TAIL") {
		t.Fatalf("output cap/tail mismatch: len=%d suffix=%q", len(out), out[len(out)-8:])
	}
	if !res.Truncated || res.BytesReturned != maxGoalOutputBytes || res.TotalBytes != int64(len(log)) || res.RequestedBytes != int64(maxGoalOutputBytes+1024) || res.MaxBytes != maxGoalOutputBytes || res.DefaultBytes != defaultGoalOutputBytes || !res.MaxBytesCapped {
		t.Fatalf("unexpected cap metadata: %#v", res)
	}
}

func TestGoalOutputRejectsUnsafeID(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	id := "safe-id"
	writeStateForTest(t, filepath.Join(temp, goalStatePrefix+id+".json"), GoalState{Objective: "safe", BudgetSeconds: 10, StartTime: float64(time.Now().Unix()), MaxTurns: 1, Status: "done"})
	if err := os.WriteFile(filepath.Join(temp, goalStatePrefix+id+".log"), []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, badID := range []string{"../safe-id", "safe/id", "safe id", "中文"} {
		if _, err := GoalOutput(root, badID, 100); err == nil || !strings.Contains(err.Error(), "invalid goal id") {
			t.Fatalf("GoalOutput(%q) err=%v, want invalid goal id", badID, err)
		}
	}
	res, err := GoalOutput(root, id, 100)
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "ok" || res.Goal.ID != id {
		t.Fatalf("valid id output/meta mismatch: %#v", res)
	}
}

func TestStartGoalTruncatesPreexistingOutputLog(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "agentmain.py"), []byte("# fake\n"), 0644); err != nil {
		t.Fatal(err)
	}
	reflectDir := filepath.Join(root, "reflect")
	if err := os.MkdirAll(reflectDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reflectDir, "goal_mode.py"), []byte("# fake\n"), 0644); err != nil {
		t.Fatal(err)
	}
	venvScripts := filepath.Join(root, ".venv", "Scripts")
	if err := os.MkdirAll(venvScripts, 0755); err != nil {
		t.Fatal(err)
	}
	fakePython := filepath.Join(venvScripts, "python.exe")
	if err := os.WriteFile(fakePython, []byte("not an executable"), 0644); err != nil {
		t.Fatal(err)
	}

	oldNewGoalID := newGoalID
	newGoalID = func() string { return "fixed_log_id" }
	t.Cleanup(func() { newGoalID = oldNewGoalID })
	goalDir := standardGoalDir(root, "fixed_log_id")
	if err := os.MkdirAll(goalDir, 0755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(goalDir, goalStandardOutputFile)
	if err := os.WriteFile(logPath, []byte("stale previous output\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := StartGoal(root, GoalStartOptions{Objective: "start fails", BudgetSeconds: 60, MaxTurns: 1})
	if err == nil {
		t.Fatal("StartGoal error = nil, want failure for invalid interpreter")
	}
	logBytes, readLogErr := os.ReadFile(logPath)
	if readLogErr != nil {
		t.Fatal(readLogErr)
	}
	if strings.Contains(string(logBytes), "stale previous output") {
		t.Fatalf("output log was appended instead of truncated: %q", string(logBytes))
	}
	if !strings.Contains(string(logBytes), "[goal start failed]") {
		t.Fatalf("failure log missing marker: %q", string(logBytes))
	}
}

func TestStartGoalFailurePersistsFailedStateAndLog(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "agentmain.py"), []byte("# fake\n"), 0644); err != nil {
		t.Fatal(err)
	}
	reflectDir := filepath.Join(root, "reflect")
	if err := os.MkdirAll(reflectDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reflectDir, "goal_mode.py"), []byte("# fake\n"), 0644); err != nil {
		t.Fatal(err)
	}
	venvScripts := filepath.Join(root, ".venv", "Scripts")
	if err := os.MkdirAll(venvScripts, 0755); err != nil {
		t.Fatal(err)
	}
	fakePython := filepath.Join(venvScripts, "python.exe")
	if err := os.WriteFile(fakePython, []byte("not an executable"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := StartGoal(root, GoalStartOptions{Objective: "start fails", BudgetSeconds: 60, MaxTurns: 1})
	if err == nil {
		t.Fatal("StartGoal error = nil, want failure for invalid interpreter")
	}

	files, globErr := filepath.Glob(filepath.Join(root, "temp", goalStandardDir, "*", goalStandardStateFile))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(files) != 1 {
		t.Fatalf("state files = %d, want 1: %v", len(files), files)
	}
	state, _, readErr := readGoalState(files[0])
	if readErr != nil {
		t.Fatal(readErr)
	}
	if state.Status != "start_failed" || state.EndTime == nil || state.PID != 0 {
		t.Fatalf("unexpected failed state: %#v", state)
	}
	logPath := filepath.Join(filepath.Dir(files[0]), goalStandardOutputFile)
	logBytes, readLogErr := os.ReadFile(logPath)
	if readLogErr != nil {
		t.Fatal(readLogErr)
	}
	if !strings.Contains(string(logBytes), "[goal start failed]") {
		t.Fatalf("failure log missing marker: %q", string(logBytes))
	}
}
func TestTasklistHasExactPID(t *testing.T) {
	out := []byte("\"Image Name\",\"PID\",\"Session Name\",\"Session#\",\"Mem Usage\"\r\n\"python.exe\",\"12345\",\"Console\",\"1\",\"10,000 K\"\r\n")
	if !tasklistHasExactPID(out, 12345) {
		t.Fatal("expected exact PID match")
	}
	if tasklistHasExactPID(out, 2345) {
		t.Fatal("must not match PID substrings")
	}
	if tasklistHasExactPID([]byte("INFO: No tasks are running which match the specified criteria.\r\n"), 12345) {
		t.Fatal("INFO no-match output must not be reported running")
	}
}

func TestIsPIDRunningCurrentProcess(t *testing.T) {
	if !isPIDRunning(os.Getpid()) {
		t.Fatal("current process should be reported running")
	}
	if isPIDRunning(0) {
		t.Fatal("pid 0 should not be reported running")
	}
}

func TestStopGoalRequiresExactPID(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	id := "stop_mismatch"
	statePath := filepath.Join(temp, goalStatePrefix+id+".json")
	writeStateForTest(t, statePath, GoalState{Objective: "stop", BudgetSeconds: 60, StartTime: float64(time.Now().Unix()), MaxTurns: 5, Status: "running", PID: 12345})

	_, err := StopGoal(root, id, 54321)
	if err == nil || !strings.Contains(err.Error(), "exact PID mismatch") {
		t.Fatalf("StopGoal error = %v, want exact PID mismatch", err)
	}
	state, _, readErr := readGoalState(statePath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if state.Status != "running" || state.EndTime != nil || state.PID != 12345 {
		t.Fatalf("state should not change after mismatch: %#v", state)
	}
}

func TestStartGoalValidatesBounds(t *testing.T) {
	root := t.TempDir()
	negativeLLMNo := -1
	cases := []struct {
		name string
		opt  GoalStartOptions
		want string
	}{
		{name: "empty objective", opt: GoalStartOptions{Objective: "  ", BudgetSeconds: 60}, want: "objective is required"},
		{name: "zero budget", opt: GoalStartOptions{Objective: "ok", BudgetSeconds: 0}, want: "budget_seconds must be positive"},
		{name: "negative budget", opt: GoalStartOptions{Objective: "ok", BudgetSeconds: -1}, want: "budget_seconds must be positive"},
		{name: "objective too large", opt: GoalStartOptions{Objective: strings.Repeat("x", maxGoalObjectiveBytes+1), BudgetSeconds: 60}, want: "objective exceeds"},
		{name: "budget too large", opt: GoalStartOptions{Objective: "ok", BudgetSeconds: maxGoalBudgetSeconds + 1}, want: "budget_seconds exceeds"},
		{name: "turns negative", opt: GoalStartOptions{Objective: "ok", BudgetSeconds: 60, MaxTurns: -1}, want: "max_turns must be non-negative"},
		{name: "turns too large", opt: GoalStartOptions{Objective: "ok", BudgetSeconds: 60, MaxTurns: maxGoalTurns + 1}, want: "max_turns exceeds"},
		{name: "negative llm no", opt: GoalStartOptions{Objective: "ok", BudgetSeconds: 60, LLMNo: &negativeLLMNo}, want: "llm_no must be non-negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := StartGoal(root, tc.opt)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("StartGoal error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestStartGoalRecordsStartFailure(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("uses Windows PATHEXT-free fake python command semantics")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "agentmain.py"), []byte("# fake"), 0644); err != nil {
		t.Fatal(err)
	}
	reflectDir := filepath.Join(root, "reflect")
	if err := os.MkdirAll(reflectDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reflectDir, "goal_mode.py"), []byte("# fake"), 0644); err != nil {
		t.Fatal(err)
	}
	venvScripts := filepath.Join(root, ".venv", "Scripts")
	if err := os.MkdirAll(venvScripts, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(venvScripts, "python.exe"), []byte("not a real executable"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := StartGoal(root, GoalStartOptions{Objective: "should fail", BudgetSeconds: 5, MaxTurns: 1})
	if err == nil {
		t.Fatal("StartGoal returned nil error for invalid python executable")
	}
	goals, listErr := ListGoals(root)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(goals) != 1 {
		t.Fatalf("len(goals)=%d, want 1: %#v", len(goals), goals)
	}
	if goals[0].Status != "failed" || goals[0].RawStatus != "start_failed" || goals[0].ErrorClass != "start_failed" || goals[0].EndTime == nil || goals[0].Running {
		t.Fatalf("unexpected failed goal meta: %#v", goals[0])
	}
	res, outErr := GoalOutput(root, goals[0].ID, 4096)
	out := res.Output
	if outErr != nil {
		t.Fatal(outErr)
	}
	if !strings.Contains(out, "[goal start failed]") {
		t.Fatalf("failure log missing marker: %q", out)
	}
}

func TestStopGoalSuccessClearsPIDAndEnds(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	id := "stop_success"
	statePath := filepath.Join(temp, goalStatePrefix+id+".json")
	start := float64(time.Now().Add(-time.Minute).Unix())
	writeStateForTest(t, statePath, GoalState{Objective: "stop", BudgetSeconds: 60, StartTime: start, MaxTurns: 3, Status: "running", PID: 22222})

	oldKill := killGoalPID
	defer func() { killGoalPID = oldKill }()
	killed := 0
	killGoalPID = func(pid int) error {
		killed = pid
		return nil
	}

	meta, err := StopGoal(root, id, 22222)
	if err != nil {
		t.Fatalf("StopGoal error = %v", err)
	}
	if killed != 22222 {
		t.Fatalf("kill pid = %d, want 22222", killed)
	}
	if meta.Status != "cancelled" || meta.RawStatus != "stopped_by_admin" || meta.PID != 0 || meta.PIDTrusted || meta.Running || meta.EndTime == nil {
		t.Fatalf("unexpected stopped meta: %#v", meta)
	}
	state, _, err := readGoalState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "stopped_by_admin" || state.PID != 0 || state.EndTime == nil {
		t.Fatalf("unexpected stopped state: %#v", state)
	}
}

func TestStopGoalRejectsInvalidIDAndPIDMismatch(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	id := "stop_safe"
	statePath := filepath.Join(temp, goalStatePrefix+id+".json")
	start := float64(time.Now().Unix())
	writeStateForTest(t, statePath, GoalState{Objective: "stop", BudgetSeconds: 60, StartTime: start, MaxTurns: 3, Status: "running", PID: 12345})

	if _, err := StopGoal(root, " ../!!! ", 0); err == nil || !strings.Contains(err.Error(), "invalid goal id") {
		t.Fatalf("StopGoal invalid id error = %v", err)
	}
	if _, err := StopGoal(root, id, 0); err == nil || !strings.Contains(err.Error(), "pid is required") {
		t.Fatalf("StopGoal missing PID error = %v", err)
	}
	if _, err := StopGoal(root, id, 54321); err == nil || !strings.Contains(err.Error(), "exact PID mismatch") {
		t.Fatalf("StopGoal PID mismatch error = %v", err)
	}
	state, _, err := readGoalState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "running" || state.EndTime != nil || state.PID != 12345 {
		t.Fatalf("StopGoal mismatch mutated state: %#v", state)
	}
}

func TestStopGoalRejectsEndedStateWithStalePID(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	id := "stop_ended"
	statePath := filepath.Join(temp, goalStatePrefix+id+".json")
	start := float64(time.Now().Add(-time.Minute).Unix())
	end := start + 10
	writeStateForTest(t, statePath, GoalState{Objective: "done", BudgetSeconds: 60, StartTime: start, EndTime: &end, MaxTurns: 3, Status: "done", PID: 22222})

	oldKill := killGoalPID
	defer func() { killGoalPID = oldKill }()
	killGoalPID = func(pid int) error {
		t.Fatalf("StopGoal must not kill stale PID %d for ended state", pid)
		return nil
	}

	_, err := StopGoal(root, id, 22222)
	if err == nil || !strings.Contains(err.Error(), "goal is not running") {
		t.Fatalf("StopGoal ended-state error = %v", err)
	}
	state, _, readErr := readGoalState(statePath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if state.Status != "done" || state.PID != 22222 || state.EndTime == nil {
		t.Fatalf("ended state should not change: %#v", state)
	}
}

func TestDeleteGoalAllowsEndedStateWithLiveReusedPID(t *testing.T) {
	root := t.TempDir()
	temp := filepath.Join(root, "temp")
	if err := os.MkdirAll(temp, 0755); err != nil {
		t.Fatal(err)
	}
	id := "delete_ended"
	statePath := filepath.Join(temp, goalStatePrefix+id+".json")
	logPath := filepath.Join(temp, goalStatePrefix+id+".log")
	start := float64(time.Now().Add(-time.Minute).Unix())
	end := start + 10
	writeStateForTest(t, statePath, GoalState{Objective: "done", BudgetSeconds: 60, StartTime: start, EndTime: &end, MaxTurns: 3, Status: "done", PID: os.Getpid()})
	if err := os.WriteFile(logPath, []byte("done"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := DeleteGoal(root, id); err != nil {
		t.Fatalf("DeleteGoal ended state with live stale PID error = %v", err)
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state file should be deleted, stat err=%v", err)
	}
	if _, err := os.Stat(logPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("log file should be deleted, stat err=%v", err)
	}
}

func writeStateForTest(t *testing.T, path string, state GoalState) {
	t.Helper()
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestGoalCommandEnvForcesUTF8AndState(t *testing.T) {
	env := goalCommandEnv([]string{"PYTHONIOENCODING=gbk", "PATH=x"}, `C:\tmp\goal.json`)
	got := map[string]string{}
	for _, item := range env {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 {
			got[parts[0]] = parts[1]
		}
	}
	if got["GOAL_STATE"] != `C:\tmp\goal.json` {
		t.Fatalf("GOAL_STATE = %q", got["GOAL_STATE"])
	}
	if got["PYTHONIOENCODING"] != "utf-8" {
		t.Fatalf("PYTHONIOENCODING = %q, want utf-8", got["PYTHONIOENCODING"])
	}
	if got["PYTHONUTF8"] != "1" {
		t.Fatalf("PYTHONUTF8 = %q, want 1", got["PYTHONUTF8"])
	}
	count := 0
	for _, item := range env {
		if strings.HasPrefix(item, "PYTHONIOENCODING=") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("PYTHONIOENCODING occurrences = %d, env=%#v", count, env)
	}
}
