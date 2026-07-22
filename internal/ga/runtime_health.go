package ga

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const runtimeProbeMarker = "GA_ADMIN_RUNTIME="

var corePythonDependencies = []RuntimeDependency{
	{Module: "requests", Package: "requests"},
	{Module: "bs4", Package: "beautifulsoup4"},
	{Module: "bottle", Package: "bottle"},
	{Module: "simple_websocket_server", Package: "simple-websocket-server"},
	{Module: "aiohttp", Package: "aiohttp"},
}

type RuntimeDependency struct {
	Module  string `json:"module"`
	Package string `json:"package"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

type RuntimeHealth struct {
	OK                     bool                `json:"ok"`
	PythonOK               bool                `json:"python_ok"`
	PythonPath             string              `json:"python_path,omitempty"`
	PythonVersion          string              `json:"python_version,omitempty"`
	Dependencies           []RuntimeDependency `json:"dependencies"`
	MissingModules         []string            `json:"missing_modules,omitempty"`
	AgentmainOK            bool                `json:"agentmain_ok"`
	AgentmainError         string              `json:"agentmain_error,omitempty"`
	UltraplanOK            bool                `json:"ultraplan_ok"`
	UltraplanMissing       []string            `json:"ultraplan_missing,omitempty"`
	UltraplanError         string              `json:"ultraplan_error,omitempty"`
	LegacyUltraplanScripts []string            `json:"legacy_ultraplan_scripts,omitempty"`
	Repairable             bool                `json:"repairable"`
	ProbeError             string              `json:"probe_error,omitempty"`
	DurationMS             int64               `json:"duration_ms"`
}

type runtimeProbePayload struct {
	PythonPath       string                       `json:"python_path"`
	PythonVersion    string                       `json:"python_version"`
	Dependencies     map[string]runtimeProbeEntry `json:"dependencies"`
	AgentmainOK      bool                         `json:"agentmain_ok"`
	AgentmainError   string                       `json:"agentmain_error"`
	UltraplanOK      bool                         `json:"ultraplan_ok"`
	UltraplanMissing []string                     `json:"ultraplan_missing"`
	UltraplanError   string                       `json:"ultraplan_error"`
}

type runtimeProbeEntry struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

var runRuntimeProbeCommand = func(ctx context.Context, root, python string) (string, error) {
	cmd := exec.CommandContext(ctx, python, "-c", runtimeProbeScript)
	cmd.Dir = root
	hideChildWindow(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func BuildRuntimeHealth(root, python string) Health {
	health := BuildHealth(root)
	if strings.TrimSpace(root) == "" {
		return health
	}
	runtimeHealth := ProbeRuntime(root, python)
	health.Runtime = &runtimeHealth
	health.Generated = time.Now()
	health.Checks["python_runtime"] = checkState(runtimeHealth.PythonOK, "unavailable")
	health.Checks["core_dependencies"] = checkState(len(runtimeHealth.MissingModules) == 0 && runtimeHealth.PythonOK, "missing")
	health.Checks["agentmain_import"] = checkState(runtimeHealth.AgentmainOK, "failed")
	health.Checks["ultraplan_api"] = checkState(runtimeHealth.UltraplanOK, "incompatible")
	health.Checks["legacy_ultraplan_scripts"] = checkState(len(runtimeHealth.LegacyUltraplanScripts) == 0, "repairable")

	if !runtimeHealth.PythonOK {
		health.Errors = append(health.Errors, "Python 运行失败: "+firstNonEmpty(runtimeHealth.ProbeError, "无法执行已配置的解释器"))
	} else {
		if len(runtimeHealth.MissingModules) > 0 {
			health.Errors = append(health.Errors, "核心依赖缺失: "+strings.Join(runtimeHealth.MissingModules, ", "))
		}
		if !runtimeHealth.AgentmainOK {
			health.Errors = append(health.Errors, "agentmain 导入失败: "+firstNonEmpty(runtimeHealth.AgentmainError, "未知异常"))
		}
		if !runtimeHealth.UltraplanOK {
			detail := firstNonEmpty(runtimeHealth.UltraplanError, strings.Join(runtimeHealth.UltraplanMissing, ", "))
			health.Errors = append(health.Errors, "UltraPlan API 检查失败: "+detail)
		}
	}
	if len(runtimeHealth.LegacyUltraplanScripts) > 0 {
		health.Errors = append(health.Errors, fmt.Sprintf("发现 %d 个旧 UltraPlan 脚本引用已移除的 plan", len(runtimeHealth.LegacyUltraplanScripts)))
	}
	health.OK = health.OK && runtimeHealth.OK
	return health
}

func ProbeRuntime(root, python string) RuntimeHealth {
	started := time.Now()
	result := RuntimeHealth{Dependencies: dependencyTemplate()}
	result.LegacyUltraplanScripts = FindLegacyUltraplanScripts(root)
	python = strings.TrimSpace(python)
	if python == "" {
		python = "python"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := runRuntimeProbeCommand(ctx, root, python)
	result.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.ProbeError = "Python 运行时检查超时（30 秒）"
		} else {
			result.ProbeError = trimDiagnostic(firstNonEmpty(extractProbeError(out), err.Error()))
		}
		result.Repairable = len(result.LegacyUltraplanScripts) > 0
		return result
	}
	payload, parseErr := parseRuntimeProbe(out)
	if parseErr != nil {
		result.ProbeError = trimDiagnostic(parseErr.Error())
		result.Repairable = len(result.LegacyUltraplanScripts) > 0
		return result
	}
	result.PythonOK = true
	result.PythonPath = payload.PythonPath
	result.PythonVersion = payload.PythonVersion
	for i := range result.Dependencies {
		entry := payload.Dependencies[result.Dependencies[i].Module]
		result.Dependencies[i].OK = entry.OK
		result.Dependencies[i].Error = trimDiagnostic(entry.Error)
		if !entry.OK {
			result.MissingModules = append(result.MissingModules, result.Dependencies[i].Module)
		}
	}
	result.AgentmainOK = payload.AgentmainOK
	result.AgentmainError = trimDiagnostic(payload.AgentmainError)
	result.UltraplanOK = payload.UltraplanOK
	result.UltraplanMissing = payload.UltraplanMissing
	result.UltraplanError = trimDiagnostic(payload.UltraplanError)
	result.Repairable = len(result.MissingModules) > 0 || len(result.LegacyUltraplanScripts) > 0
	result.OK = len(result.MissingModules) == 0 && result.AgentmainOK && result.UltraplanOK && len(result.LegacyUltraplanScripts) == 0
	return result
}

func MissingDependencyPackages(runtimeHealth *RuntimeHealth) []string {
	if runtimeHealth == nil {
		return nil
	}
	packages := make([]string, 0, len(runtimeHealth.Dependencies))
	for _, dependency := range runtimeHealth.Dependencies {
		if !dependency.OK {
			packages = append(packages, dependency.Package)
		}
	}
	return packages
}

func FindLegacyUltraplanScripts(root string) []string {
	pattern := filepath.Join(root, "temp", "ultraplan_*", "admin_chat_ultraplan.py")
	paths, _ := filepath.Glob(pattern)
	legacy := make([]string, 0)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil || !isLegacyUltraplanScript(string(data)) {
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err == nil {
			legacy = append(legacy, filepath.ToSlash(rel))
		}
	}
	sort.Strings(legacy)
	return legacy
}

func RepairLegacyUltraplanScripts(root string) ([]string, error) {
	paths := FindLegacyUltraplanScripts(root)
	repaired := make([]string, 0, len(paths))
	for _, rel := range paths {
		path := filepath.Join(root, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			return repaired, err
		}
		content := strings.ReplaceAll(string(data), "from assets.ga_ultraplan import plan, phase, parallel", "from assets.ga_ultraplan import phase, parallel")
		content = removeExactPythonLine(content, "plan(RUN_DIR)")
		info, err := os.Stat(path)
		if err != nil {
			return repaired, err
		}
		if err := os.WriteFile(path, []byte(content), info.Mode().Perm()); err != nil {
			return repaired, err
		}
		repaired = append(repaired, rel)
	}
	return repaired, nil
}

func parseRuntimeProbe(output string) (runtimeProbePayload, error) {
	index := strings.LastIndex(output, runtimeProbeMarker)
	if index < 0 {
		return runtimeProbePayload{}, fmt.Errorf("运行时探针未返回有效结果: %s", trimDiagnostic(output))
	}
	line := output[index+len(runtimeProbeMarker):]
	if newline := strings.IndexAny(line, "\r\n"); newline >= 0 {
		line = line[:newline]
	}
	var payload runtimeProbePayload
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &payload); err != nil {
		return runtimeProbePayload{}, fmt.Errorf("运行时探针结果无法解析: %w", err)
	}
	return payload, nil
}

func dependencyTemplate() []RuntimeDependency {
	result := make([]RuntimeDependency, len(corePythonDependencies))
	copy(result, corePythonDependencies)
	return result
}

func checkState(ok bool, failed string) string {
	if ok {
		return "ok"
	}
	return failed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func extractProbeError(output string) string {
	if index := strings.LastIndex(output, runtimeProbeMarker); index >= 0 {
		return strings.TrimSpace(output[:index])
	}
	return strings.TrimSpace(output)
}

func trimDiagnostic(value string) string {
	value = strings.TrimSpace(value)
	const maxDiagnosticBytes = 2000
	if len(value) <= maxDiagnosticBytes {
		return value
	}
	return "..." + value[len(value)-maxDiagnosticBytes:]
}

func isLegacyUltraplanScript(content string) bool {
	return strings.Contains(content, "from assets.ga_ultraplan import plan, phase, parallel") || hasExactPythonLine(content, "plan(RUN_DIR)")
}

func hasExactPythonLine(content, target string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == target {
			return true
		}
	}
	return false
}

func removeExactPythonLine(content, target string) string {
	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.TrimSpace(line) != target {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, newline)
}
