package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverExcludesGoalModeFromGenericReflectList(t *testing.T) {
	root := t.TempDir()
	reflectDir := filepath.Join(root, "reflect")
	if err := os.MkdirAll(reflectDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"goal_mode.py", "autonomous.py", "custom_reflect.py"} {
		if err := os.WriteFile(filepath.Join(reflectDir, name), []byte("# test\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	items := NewManager(root, 100).Discover()
	seen := map[string]int{}
	for _, item := range items {
		seen[item.Name]++
	}
	if seen[filepath.ToSlash(filepath.Join("reflect", "goal_mode.py"))] != 1 {
		t.Fatalf("goal_mode.py should appear exactly once as the dedicated lifecycle entry, seen=%d items=%#v", seen[filepath.ToSlash(filepath.Join("reflect", "goal_mode.py"))], items)
	}
	if seen[filepath.ToSlash(filepath.Join("reflect", "autonomous.py"))] != 1 {
		t.Fatalf("autonomous.py should remain discoverable once, seen=%d", seen[filepath.ToSlash(filepath.Join("reflect", "autonomous.py"))])
	}
	if seen[filepath.ToSlash(filepath.Join("reflect", "custom_reflect.py"))] != 1 {
		t.Fatalf("custom reflect should remain discoverable once, seen=%d", seen[filepath.ToSlash(filepath.Join("reflect", "custom_reflect.py"))])
	}
}

func TestDiscoverIncludesHubFrontend(t *testing.T) {
	root := t.TempDir()
	frontendsDir := filepath.Join(root, "frontends")
	if err := os.MkdirAll(frontendsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frontendsDir, "hub.py"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	items := NewManager(root, 100).Discover()
	for _, item := range items {
		if item.Name == "frontends/hub.py" {
			if item.Kind != "frontend" {
				t.Fatalf("hub kind=%s", item.Kind)
			}
			if len(item.Command) != 2 || item.Command[1] != "frontends/hub.py" {
				t.Fatalf("hub command=%#v", item.Command)
			}
			return
		}
	}
	t.Fatalf("frontends/hub.py was not discovered: %#v", items)
}

func TestDiscoverIncludesChannelFrontendApps(t *testing.T) {
	root := t.TempDir()
	frontendsDir := filepath.Join(root, "frontends")
	if err := os.MkdirAll(frontendsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fsapp.py", "wecomapp.py", "dingtalkapp.py", "notbot.py", "_hiddenapp.py"} {
		if err := os.WriteFile(filepath.Join(frontendsDir, name), []byte("# test\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	items := NewManager(root, 100).Discover()
	seen := map[string]ServiceInfo{}
	for _, item := range items {
		seen[item.Name] = item
	}
	for _, name := range []string{"fsapp.py", "wecomapp.py", "dingtalkapp.py"} {
		rel := filepath.ToSlash(filepath.Join("frontends", name))
		item, ok := seen[rel]
		if !ok {
			t.Fatalf("missing channel frontend %s in %#v", rel, items)
		}
		if item.Kind != "frontend" {
			t.Fatalf("%s kind=%s", rel, item.Kind)
		}
		if len(item.Command) != 2 || item.Command[1] != rel {
			t.Fatalf("%s command=%#v", rel, item.Command)
		}
	}
	if _, ok := seen[filepath.ToSlash(filepath.Join("frontends", "notbot.py"))]; ok {
		t.Fatalf("notbot.py should not be discovered: %#v", items)
	}
	if _, ok := seen[filepath.ToSlash(filepath.Join("frontends", "_hiddenapp.py"))]; ok {
		t.Fatalf("_hiddenapp.py should not be discovered: %#v", items)
	}
}

func TestCommandLineMatchesServiceRequiresExactScriptPath(t *testing.T) {
	root := filepath.Clean(filepath.Join(t.TempDir(), "ga-root"))
	py := filepath.Join(root, ".venv", "Scripts", "python.exe")
	cmd := []string{py, filepath.ToSlash(filepath.Join("reflect", "custom_reflect.py"))}

	ownAbs := py + " " + filepath.Join(root, "reflect", "custom_reflect.py")
	if !commandLineMatchesService(ownAbs, root, cmd) {
		t.Fatalf("expected absolute GA script command to match")
	}

	ownRel := py + " reflect/custom_reflect.py"
	if !commandLineMatchesService(ownRel, root, cmd) {
		t.Fatalf("expected relative GA script command to match")
	}

	otherSameBase := py + " " + filepath.Join(filepath.Dir(root), "other-root", "reflect", "custom_reflect.py")
	if commandLineMatchesService(otherSameBase, root, cmd) {
		t.Fatalf("same basename under another root must not match")
	}

	otherSimilarRel := py + " other/reflect/custom_reflect.py"
	if commandLineMatchesService(otherSimilarRel, root, cmd) {
		t.Fatalf("relative path with extra prefix must not match")
	}
}

func TestDiscoverServiceInfoIncludesWorkDir(t *testing.T) {
	root := t.TempDir()
	reflectDir := filepath.Join(root, "reflect")
	if err := os.MkdirAll(reflectDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reflectDir, "custom_reflect.py"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	items := NewManager(root, 100).Discover()
	if len(items) == 0 {
		t.Fatal("expected discovered services")
	}
	for _, item := range items {
		if item.WorkDir != root {
			t.Fatalf("%s WorkDir=%q want %q", item.Name, item.WorkDir, root)
		}
	}
}

func TestStopRejectsUnknownServiceName(t *testing.T) {
	m := NewManager(t.TempDir(), 100)
	if err := m.Stop("missing.py"); err == nil {
		t.Fatal("Stop() unknown service should return an error")
	}
}

func TestReadPipeContinuesAfterOversizedLine(t *testing.T) {
	m := NewManager(t.TempDir(), 10)
	m.readPipe("svc.py", strings.NewReader(strings.Repeat("x", maxLogLineBytes+64)+"\nafter\n"))

	logs := m.Logs("svc.py", 10)
	if len(logs) != 2 {
		t.Fatalf("logs len=%d want=2 logs=%#v", len(logs), logs)
	}
	if !strings.HasSuffix(logs[0], " [truncated]") {
		t.Fatalf("oversized line should be marked truncated, got %.80q", logs[0])
	}
	if len(logs[0]) > maxLogLineBytes+len(" [truncated]") {
		t.Fatalf("truncated line len=%d exceeds cap", len(logs[0]))
	}
	if logs[1] != "after" {
		t.Fatalf("second line lost after oversized line: %#v", logs)
	}
}

func TestManagerSubscribeDeliversTailLogResetAndCancel(t *testing.T) {
	m := NewManager(t.TempDir(), 10)
	m.readPipe("svc.py", strings.NewReader("one\ntwo\nthree\n"))

	snapshot, events, cancel := m.Subscribe("svc.py", 2)
	if got := strings.Join(snapshot, ","); got != "two,three" {
		t.Fatalf("snapshot=%q want=%q", got, "two,three")
	}

	m.readPipe("svc.py", strings.NewReader("four\n"))
	if event := <-events; event.Reset || event.Line != "four" {
		t.Fatalf("log event=%#v", event)
	}

	m.mu.Lock()
	m.resetLocked("svc.py", []string{"fresh"})
	m.mu.Unlock()
	if event := <-events; !event.Reset || strings.Join(event.Lines, ",") != "fresh" {
		t.Fatalf("reset event=%#v", event)
	}

	cancel()
	cancel()
	if event, open := <-events; open {
		t.Fatalf("subscription remained open after cancel: %#v", event)
	}
}

func TestManagerSubscribeSlowClientCatchesUpWithReset(t *testing.T) {
	m := NewManager(t.TempDir(), 3)
	var input strings.Builder
	for i := 0; i < 257; i++ {
		fmt.Fprintf(&input, "line-%03d\n", i)
	}

	_, events, cancel := m.Subscribe("svc.py", 3)
	defer cancel()
	m.readPipe("svc.py", strings.NewReader(input.String()))

	event := <-events
	if !event.Reset {
		t.Fatalf("slow subscriber event=%#v, want reset", event)
	}
	if got := strings.Join(event.Lines, ","); got != "line-254,line-255,line-256" {
		t.Fatalf("reset lines=%q", got)
	}
}

func TestBuildServiceArgsReflectLLMNo(t *testing.T) {
	svc := ServiceInfo{Name: "reflect/custom.py", Kind: "reflect", Command: []string{"python", "reflect/custom.py"}}
	args, err := buildServiceArgs(svc, map[string]string{"llm_no": " 12 "})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if joined != "reflect/custom.py --llm_no 12" {
		t.Fatalf("args=%q", joined)
	}
}

func TestBuildServiceArgsRejectsInvalidReflectLLMNo(t *testing.T) {
	svc := ServiceInfo{Name: "reflect/custom.py", Kind: "reflect", Command: []string{"python", "reflect/custom.py"}}
	if _, err := buildServiceArgs(svc, map[string]string{"llm_no": "1 --bad"}); err == nil {
		t.Fatal("expected invalid llm_no to be rejected")
	}
}

func TestBuildServiceArgsIgnoresParamsForNonReflectService(t *testing.T) {
	svc := ServiceInfo{Name: "launch.py", Kind: "core", Command: []string{"python", "launch.py"}}
	args, err := buildServiceArgs(svc, map[string]string{"llm_no": "9"})
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(args, " "); joined != "launch.py" {
		t.Fatalf("args=%q", joined)
	}
}

func TestManagerPythonPrefersEffectivePythonOverVenv(t *testing.T) {
	root := t.TempDir()
	venvPython := filepath.Join(root, ".venv", "Scripts", "python.exe")
	if err := os.MkdirAll(filepath.Dir(venvPython), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(venvPython, []byte("stub"), 0755); err != nil {
		t.Fatal(err)
	}
	effective := filepath.Join(root, "managed-python.exe")
	m := NewManager(root, 100)
	m.SetRoot(root, effective, 100)
	if got := m.python(); got != effective {
		t.Fatalf("python()=%q, want effective python %q", got, effective)
	}
}

func newSharedHubTestManager(t *testing.T, pid int) *Manager {
	t.Helper()
	root := t.TempDir()
	hubPath := filepath.Join(root, "frontends", "hub.py")
	if err := os.MkdirAll(filepath.Dir(hubPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hubPath, []byte("raise SystemExit('must not start')\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := NewManager(root, 100)
	m.EffectivePython = filepath.Join(root, "missing-python.exe")
	m.sharedHubPID = func(port int) (int, bool) {
		if port != 19736 {
			t.Fatalf("shared Hub probe port=%d want=19736", port)
		}
		return pid, true
	}
	return m
}

func TestDiscoverReportsExternalHubAsSharedUnmanaged(t *testing.T) {
	m := newSharedHubTestManager(t, 4242)
	svc, ok := m.Find("frontends/hub.py")
	if !ok {
		t.Fatal("shared Hub was not discovered")
	}
	if !svc.Running || !svc.Shared || svc.Managed {
		t.Fatalf("unexpected shared Hub state: %+v", svc)
	}
	if svc.PID == nil || *svc.PID != 4242 {
		t.Fatalf("shared Hub PID=%v want=4242", svc.PID)
	}
}

func TestStartReusesExternalSharedHubWithoutLaunching(t *testing.T) {
	m := newSharedHubTestManager(t, 4242)
	svc, err := m.Start("frontends/hub.py")
	if err != nil {
		t.Fatal(err)
	}
	if !svc.Running || !svc.Shared || svc.Managed || svc.PID == nil || *svc.PID != 4242 {
		t.Fatalf("unexpected reused Hub state: %+v", svc)
	}
	m.mu.Lock()
	_, launched := m.procs["frontends/hub.py"]
	m.mu.Unlock()
	if launched {
		t.Fatal("external shared Hub must not be launched or adopted as a managed process")
	}
}

func TestStopRejectsExternalSharedHub(t *testing.T) {
	m := newSharedHubTestManager(t, 4242)
	err := m.Stop("frontends/hub.py")
	if err == nil || !strings.Contains(err.Error(), "cannot be stopped here") {
		t.Fatalf("Stop() error=%v, want shared ownership rejection", err)
	}
}

func TestSharedHubCommandLineMatcher(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  string
		want bool
	}{
		{name: "windows python", cmd: `D:\\GA\\.venv\\Scripts\\python.exe D:\\GA\\frontends\\hub.py`, want: true},
		{name: "quoted python", cmd: `"/opt/ga/.venv/bin/python" "/opt/ga/frontends/hub.py"`, want: true},
		{name: "wrong executable", cmd: `node /opt/ga/frontends/hub.py`, want: false},
		{name: "wrong script", cmd: `python /opt/ga/frontends/chatapp.py`, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSharedHubCommandLine(tc.cmd); got != tc.want {
				t.Fatalf("isSharedHubCommandLine(%q)=%v want=%v", tc.cmd, got, tc.want)
			}
		})
	}
}

func TestSharedHubPort(t *testing.T) {
	t.Setenv("GA_HUB_PORT", "")
	if got := sharedHubPort(); got != 19736 {
		t.Fatalf("default sharedHubPort()=%d want=19736", got)
	}
	t.Setenv("GA_HUB_PORT", "23456")
	if got := sharedHubPort(); got != 23456 {
		t.Fatalf("configured sharedHubPort()=%d want=23456", got)
	}
	t.Setenv("GA_HUB_PORT", "invalid")
	if got := sharedHubPort(); got != 19736 {
		t.Fatalf("invalid sharedHubPort()=%d want=19736", got)
	}
}
