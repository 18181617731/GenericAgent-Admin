package ga

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRuntimeHealthReportsMissingDependencyAndImportError(t *testing.T) {
	root := healthyStaticRoot(t)
	stubRuntimeProbe(t, `GA_ADMIN_RUNTIME={"python_path":"C:\\Python\\python.exe","python_version":"3.12.1","dependencies":{"requests":{"ok":false,"error":"No module named requests"},"bs4":{"ok":true},"bottle":{"ok":true},"simple_websocket_server":{"ok":true},"aiohttp":{"ok":true}},"agentmain_ok":false,"agentmain_error":"ModuleNotFoundError: No module named 'requests'","ultraplan_ok":true,"ultraplan_missing":[]}`)

	health := BuildRuntimeHealth(root, "python")

	if health.OK || health.Runtime == nil || health.Runtime.PythonPath != `C:\Python\python.exe` {
		t.Fatalf("expected failed runtime health with resolved Python, got %#v", health)
	}
	if health.Checks["core_dependencies"] != "missing" || health.Checks["agentmain_import"] != "failed" {
		t.Fatalf("unexpected checks: %#v", health.Checks)
	}
	if !containsHealthItem(health.Errors, "requests") || !containsHealthItem(health.Errors, "agentmain") {
		t.Fatalf("expected actionable dependency/import errors, got %v", health.Errors)
	}
	if got := MissingDependencyPackages(health.Runtime); len(got) != 1 || got[0] != "requests" {
		t.Fatalf("missing packages = %v", got)
	}
}

func TestBuildRuntimeHealthReportsPythonExecutionFailure(t *testing.T) {
	root := healthyStaticRoot(t)
	previous := runRuntimeProbeCommand
	runRuntimeProbeCommand = func(context.Context, string, string) (string, error) {
		return "python unavailable", os.ErrNotExist
	}
	t.Cleanup(func() { runRuntimeProbeCommand = previous })

	health := BuildRuntimeHealth(root, "missing-python")

	if health.OK || health.Runtime == nil || health.Runtime.PythonOK {
		t.Fatalf("expected Python failure, got %#v", health)
	}
	if health.Checks["python_runtime"] != "unavailable" || !containsHealthItem(health.Errors, "Python 运行失败") {
		t.Fatalf("unexpected Python diagnostics: %#v", health)
	}
}

func TestBuildRuntimeHealthReportsIncompatibleUltraplanAPI(t *testing.T) {
	root := healthyStaticRoot(t)
	stubRuntimeProbe(t, `GA_ADMIN_RUNTIME={"python_path":"python","python_version":"3.12.1","dependencies":{"requests":{"ok":true},"bs4":{"ok":true},"bottle":{"ok":true},"simple_websocket_server":{"ok":true},"aiohttp":{"ok":true}},"agentmain_ok":true,"ultraplan_ok":false,"ultraplan_missing":["parallel"]}`)

	health := BuildRuntimeHealth(root, "python")

	if health.OK || health.Checks["ultraplan_api"] != "incompatible" || !containsHealthItem(health.Errors, "parallel") {
		t.Fatalf("expected incompatible UltraPlan diagnostics, got %#v", health)
	}
}

func TestRepairLegacyUltraplanScriptsIsIdempotent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "temp", "ultraplan_demo", "admin_chat_ultraplan.py")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	legacy := "from assets.ga_ultraplan import plan, phase, parallel\r\nplan(RUN_DIR)\r\nprint('中文保持')\r\n"
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	repaired, err := RepairLegacyUltraplanScripts(root)
	if err != nil || len(repaired) != 1 {
		t.Fatalf("repair = %v, %v", repaired, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "plan(RUN_DIR)") || strings.Contains(content, "import plan") || !strings.Contains(content, "中文保持") {
		t.Fatalf("unexpected repaired content: %q", content)
	}
	repaired, err = RepairLegacyUltraplanScripts(root)
	if err != nil || len(repaired) != 0 {
		t.Fatalf("second repair should be a no-op: %v, %v", repaired, err)
	}
}

func healthyStaticRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, rel := range []string{"agentmain.py", "llmcore.py", "assets/tools_schema.json"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("stub"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func stubRuntimeProbe(t *testing.T, output string) {
	t.Helper()
	previous := runRuntimeProbeCommand
	runRuntimeProbeCommand = func(context.Context, string, string) (string, error) { return output, nil }
	t.Cleanup(func() { runRuntimeProbeCommand = previous })
}
