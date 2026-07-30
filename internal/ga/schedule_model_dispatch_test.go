package ga

import (
	"os"
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

func TestEnsureScheduleModelDispatchPatchesBothScriptsOnce(t *testing.T) {
	root := t.TempDir()
	reflectDir := filepath.Join(root, "reflect")
	if err := os.MkdirAll(reflectDir, 0755); err != nil {
		t.Fatal(err)
	}
	scheduler := "def check():\n        # \u89e6\u53d1\n        _logger.info('trigger')\n        ts = now.strftime('%Y-%m-%d_%H%M')\n        rpt = os.path.join(DONE, f'{ts}_{tid}.md')\n        prompt = task.get('prompt', '')\n        return (f'old {prompt}')\n\n    return None\n"
	agent := "            if task and task == '/exit': break\n            if task:\n                print(f'[Reflect] triggered: {task[:80]}')\n                dq = agent.put_task(task, source='reflect')\n                result = 'old'\n            time.sleep(getattr(mod, 'INTERVAL', 5))\n"
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
	if _, err := os.Stat(filepath.Join(reflectDir, "scheduler.py.ga-admin.bak")); err != nil {
		t.Fatal(err)
	}
	second, err := EnsureScheduleModelDispatch(root)
	if err != nil || len(second.Updated) != 0 {
		t.Fatalf("second dispatch result/error = %#v %v", second, err)
	}
}
