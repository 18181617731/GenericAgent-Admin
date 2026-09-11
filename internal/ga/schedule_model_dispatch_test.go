package ga

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScheduleTaskLLMNo(t *testing.T) {
	cases := []struct {
		value any
		want  int
		ok    bool
		err   bool
	}{
		{value: nil},
		{value: float64(2), want: 2, ok: true},
		{value: "3", want: 3, ok: true},
		{value: float64(2.5), err: true},
		{value: -1, err: true},
	}
	for _, tc := range cases {
		got, ok, err := ScheduleTaskLLMNo(map[string]any{"llm_no": tc.value})
		if (err != nil) != tc.err || got != tc.want || ok != tc.ok {
			t.Fatalf("value=%#v got=%d/%t/%v", tc.value, got, ok, err)
		}
	}
}

func TestScheduleModelKeyIsStableAndTaskModelKeyIsValidated(t *testing.T) {
	want := "gpt-5.6-luna\x1f自建服务商\x1fhttps://example.test/v1"
	if got := ScheduleModelKey(" gpt-5.6-luna ", " 自建服务商 ", "HTTPS://EXAMPLE.TEST/v1/"); got != want {
		t.Fatalf("ScheduleModelKey()=%q want %q", got, want)
	}
	if got, selected, err := ScheduleTaskModelKey(map[string]any{"model_key": want}); err != nil || !selected || got != want {
		t.Fatalf("ScheduleTaskModelKey()=%q/%t/%v", got, selected, err)
	}
	if _, selected, err := ScheduleTaskModelKey(map[string]any{"model_key": 3}); err == nil || selected {
		t.Fatalf("numeric model key should be rejected: selected=%t err=%v", selected, err)
	}
	if _, selected, err := ScheduleTaskModelKey(map[string]any{"model_key": "  "}); err != nil || selected {
		t.Fatalf("blank model key should mean follow scheduler: selected=%t err=%v", selected, err)
	}
}

func TestPatchSchedulerModelDispatchPreservesRunBookkeeping(t *testing.T) {
	source := `def check():
    for _ in [0]:
        run_id = 'window'
        if _load_run(run_id):
            return None
        # GA_ADMIN_SCHEDULE_MODEL_DISPATCH_SCHEDULER
        run_state = {'run_id': run_id, 'report_path': '/tmp/report.md'}
        if not _claim_run(run_id, run_state):
            return None
        _active_run = run_id
        return {'prompt': 'run', 'llm_no': None, 'task_id': 'daily',
                'run_id': run_id, 'report_path': '/tmp/report.md'}

    return None
`
	patched, changed, err := patchSchedulerModelDispatch(source)
	if err != nil || !changed {
		t.Fatalf("patch scheduler changed/error = %t %v", changed, err)
	}
	for _, needle := range []string{"_claim_run(run_id, run_state)", "_active_run = run_id", "'run_id': run_id", "'report_path': '/tmp/report.md'", "'model_key': model_key"} {
		if !strings.Contains(patched, needle) {
			t.Fatalf("patched scheduler lost %q:\n%s", needle, patched)
		}
	}
}

func TestEnsureScheduleModelDispatchPatchesBothScriptsOnce(t *testing.T) {
	root := t.TempDir()
	reflectDir := filepath.Join(root, "reflect")
	if err := os.MkdirAll(reflectDir, 0755); err != nil {
		t.Fatal(err)
	}
	scheduler := "def check():\n    for _ in [0]:\n        # \u89e6\u53d1\n        _logger.info('trigger')\n        ts = now.strftime('%Y-%m-%d_%H%M')\n        rpt = os.path.join(DONE, f'{ts}_{tid}.md')\n        prompt = task.get('prompt', '')\n        return (f'old {prompt}')\n\n    return None\n"
	agent := "def run():\n    while True:\n            if task and task == '/exit': break\n            if task:\n                print(f'[Reflect] triggered: {task[:80]}')\n                dq = agent.put_task(task, source='reflect')\n                result = 'old'\n            time.sleep(getattr(mod, 'INTERVAL', 5))\n"
	if err := os.WriteFile(filepath.Join(reflectDir, "scheduler.py"), []byte(scheduler), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agentmain.py"), []byte(agent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := EnsureScheduleModelDispatch(root)
	if err != nil || len(result.Updated) != 2 {
		t.Fatalf("dispatch result/error = %#v %v", result, err)
	}
	patchedScheduler, _ := os.ReadFile(filepath.Join(reflectDir, "scheduler.py"))
	patchedAgent, _ := os.ReadFile(filepath.Join(root, "agentmain.py"))
	if !strings.Contains(string(patchedScheduler), schedulerModelDispatchMarker) || !strings.Contains(string(patchedAgent), agentModelDispatchMarker) {
		t.Fatal("model dispatch markers were not written")
	}
	if !strings.Contains(string(patchedScheduler), "model_key") || !strings.Contains(string(patchedAgent), "task_model_key") {
		t.Fatal("patched scripts must carry the stable model identity")
	}
	if python, err := exec.LookPath("python"); err == nil {
		cmd := exec.Command(python, "-m", "py_compile", filepath.Join(reflectDir, "scheduler.py"), filepath.Join(root, "agentmain.py"))
		if output, compileErr := cmd.CombinedOutput(); compileErr != nil {
			t.Fatalf("patched Python scripts do not compile: %v\n%s", compileErr, output)
		}
	}
	if _, err := os.Stat(filepath.Join(reflectDir, "scheduler.py.ga-admin.bak")); err != nil {
		t.Fatal(err)
	}
	second, err := EnsureScheduleModelDispatch(root)
	if err != nil || len(second.Updated) != 0 {
		t.Fatalf("second dispatch result/error = %#v %v", second, err)
	}
}
