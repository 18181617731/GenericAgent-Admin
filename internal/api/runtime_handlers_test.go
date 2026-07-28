package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/ga"
)

func TestRuntimeRepairRebuildsDeletedConfiguredVenv(t *testing.T) {
	s := newConfigTestServer(t)
	root := t.TempDir()
	missing := filepath.Join(root, "temp", ".venv", "Scripts", "python.exe")
	s.CfgStore.Cfg = config.AppConfig{GARoot: root, PythonPath: missing, EffectivePython: missing, BufferLines: 1000}
	managed := setupVenvPython(root)

	oldBuild := buildRuntimeHealthForRepair
	oldExecutable := runtimeRepairExecutablePath
	oldRun := runSetupCommandOutputFunc
	t.Cleanup(func() {
		buildRuntimeHealthForRepair = oldBuild
		runtimeRepairExecutablePath = oldExecutable
		runSetupCommandOutputFunc = oldRun
	})
	installed := false
	buildRuntimeHealthForRepair = func(_ string, python string) ga.Health {
		if python != managed {
			return ga.Health{Runtime: &ga.RuntimeHealth{PythonPath: python}}
		}
		dependency := ga.RuntimeDependency{Module: "requests", Package: "requests", OK: installed}
		return ga.Health{OK: installed, Runtime: &ga.RuntimeHealth{OK: installed, PythonOK: true, PythonPath: managed, Dependencies: []ga.RuntimeDependency{dependency}}}
	}
	runtimeRepairExecutablePath = func(string) (string, error) { return "base-python", nil }
	runSetupCommandOutputFunc = func(_ context.Context, _ string, _ string, args ...string) (string, error) {
		if len(args) >= 3 && args[0] == "-m" && args[1] == "venv" {
			if err := os.MkdirAll(filepath.Dir(managed), 0755); err != nil {
				return "", err
			}
			return "", os.WriteFile(managed, []byte("stub"), 0755)
		}
		installed = true
		return "installed", nil
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ga/runtime/repair", strings.NewReader(`{}`))
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var result runtimeRepairResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || s.CfgStore.Cfg.PythonPath != managed || s.CfgStore.Cfg.EffectivePython != managed {
		t.Fatalf("repair=%#v config=%#v", result, s.CfgStore.Cfg)
	}
}

func TestRuntimeRepairHandlesMissingHealthAfterRebuild(t *testing.T) {
	s := newConfigTestServer(t)
	root := t.TempDir()
	missing := filepath.Join(root, "temp", ".venv", "Scripts", "python.exe")
	s.CfgStore.Cfg = config.AppConfig{GARoot: root, PythonPath: missing, EffectivePython: missing}
	managed := setupVenvPython(root)

	oldBuild := buildRuntimeHealthForRepair
	oldExecutable := runtimeRepairExecutablePath
	oldRun := runSetupCommandOutputFunc
	t.Cleanup(func() {
		buildRuntimeHealthForRepair = oldBuild
		runtimeRepairExecutablePath = oldExecutable
		runSetupCommandOutputFunc = oldRun
	})
	checks := 0
	buildRuntimeHealthForRepair = func(_ string, python string) ga.Health {
		checks++
		if checks == 1 {
			return ga.Health{Runtime: &ga.RuntimeHealth{PythonPath: python}}
		}
		return ga.Health{}
	}
	runtimeRepairExecutablePath = func(string) (string, error) { return "base-python", nil }
	runSetupCommandOutputFunc = func(_ context.Context, _ string, _ string, _ ...string) (string, error) {
		if err := os.MkdirAll(filepath.Dir(managed), 0755); err != nil {
			return "", err
		}
		return "", os.WriteFile(managed, []byte("stub"), 0755)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ga/runtime/repair", strings.NewReader(`{}`))
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var result runtimeRepairResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.OK || len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "无法读取 GA 运行时状态") {
		t.Fatalf("repair=%#v", result)
	}
}
