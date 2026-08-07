package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"genericagent-admin-go/internal/ga"
)

type runtimeRepairResult struct {
	OK         bool      `json:"ok"`
	Before     ga.Health `json:"before"`
	After      ga.Health `json:"after"`
	Python     string    `json:"python,omitempty"`
	Installed  []string  `json:"installed,omitempty"`
	Repaired   []string  `json:"repaired,omitempty"`
	Skipped    []string  `json:"skipped,omitempty"`
	Operations []string  `json:"operations,omitempty"`
	Errors     []string  `json:"errors,omitempty"`
	CheckedAt  string    `json:"checked_at"`
}

var (
	buildRuntimeHealthForRepair = func(root, python string) ga.Health {
		return (&Server{}).buildGARuntimeHealthForWorker(root, python)
	}
	runtimeRepairExecutablePath = executablePath
)

func (s *Server) gaRuntimeRepair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	root, err := setupRequestRoot(r, s.CfgStore.Snapshot().GARoot)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	python := resolvePythonForRoot(root, s.CfgStore.Snapshot().EffectivePython)
	result := runtimeRepairResult{Python: python, CheckedAt: time.Now().Format(time.RFC3339)}
	result.Before = buildRuntimeHealthForRepair(root, python)
	if result.Before.Runtime == nil {
		result.Errors = append(result.Errors, "无法读取 GA 运行时状态")
		result.After = result.Before
		writeJSON(w, result)
		return
	}
	python, ready := s.ensureManagedRuntimePython(r.Context(), root, python, &result)
	if !ready {
		result.After = buildRuntimeHealthForRepair(root, python)
		writeJSON(w, result)
		return
	}
	current := buildRuntimeHealthForRepair(root, python)
	if current.Runtime == nil {
		result.Errors = append(result.Errors, "重建 Python 环境后无法读取 GA 运行时状态")
		result.After = current
		writeJSON(w, result)
		return
	}
	repairRuntimeDependencies(r.Context(), root, python, current.Runtime, &result)
	repairLegacyRuntime(root, current.Runtime, &result)
	result.After = buildRuntimeHealthForRepair(root, python)
	s.persistRuntimePython(root, &result)
	result.OK = result.After.OK && len(result.Errors) == 0
	writeJSON(w, result)
}

func repairRuntimeDependencies(ctx context.Context, root, python string, runtime *ga.RuntimeHealth, result *runtimeRepairResult) {
	packages := ga.MissingDependencyPackages(runtime)
	if len(packages) > 0 && runtime.PythonOK {
		installCtx, cancel := context.WithTimeout(ctx, setupCommandTimeout)
		args := []string{"-m", "pip", "install", "--disable-pip-version-check", "--no-input"}
		args = append(args, packages...)
		output, installErr := runSetupCommandOutputFunc(installCtx, root, python, args...)
		cancel()
		if installErr != nil {
			result.Errors = append(result.Errors, "安装核心依赖失败: "+trimRepairOutput(output+"\n"+installErr.Error()))
		} else {
			result.Installed = append(result.Installed, packages...)
			result.Operations = append(result.Operations, "已安装缺失核心依赖: "+strings.Join(packages, ", "))
		}
	} else if len(packages) > 0 {
		result.Skipped = append(result.Skipped, "Python 不可执行，无法安装核心依赖")
	}
}

func repairLegacyRuntime(root string, runtime *ga.RuntimeHealth, result *runtimeRepairResult) {
	legacy := runtime.LegacyUltraplanScripts
	if len(legacy) == 0 {
		return
	}
	repaired, err := ga.RepairLegacyUltraplanScripts(root)
	if err != nil {
		result.Errors = append(result.Errors, "迁移旧 UltraPlan 脚本失败: "+err.Error())
	}
	result.Repaired = append(result.Repaired, repaired...)
	if len(repaired) > 0 {
		result.Operations = append(result.Operations, "已迁移旧 UltraPlan 脚本: "+strings.Join(repaired, ", "))
	}
	if len(repaired) != len(legacy) && err == nil {
		result.Errors = append(result.Errors, "部分旧 UltraPlan 脚本未能迁移")
	}
}

func (s *Server) persistRuntimePython(root string, result *runtimeRepairResult) {
	if result.After.Runtime != nil && isAbsoluteExistingFile(result.After.Runtime.PythonPath) {
		result.Python = result.After.Runtime.PythonPath
		cfg := s.CfgStore.Snapshot()
		if cfg.PythonPath != result.Python {
			cfg.GARoot = root
			cfg.PythonPath = result.Python
			cfg.EffectivePython = result.Python
			cfg.SyncDefaultInstanceFromLegacy()
			if saveErr := s.CfgStore.Save(cfg); saveErr != nil {
				result.Errors = append(result.Errors, "保存已验证 Python 路径失败: "+saveErr.Error())
			} else {
				saved := s.CfgStore.Snapshot()
				s.Svc.SetRoot(saved.GARoot, saved.EffectivePython, saved.BufferLines)
				result.Operations = append(result.Operations, "已固定本次验证成功的 Python: "+result.Python)
			}
		}
	}
}

func (s *Server) ensureManagedRuntimePython(ctx context.Context, root, python string, result *runtimeRepairResult) (string, bool) {
	managed := setupVenvPython(root)
	configured := strings.TrimSpace(s.CfgStore.Snapshot().EffectivePython)
	if isAbsoluteExistingFile(managed) || isAbsoluteExistingFile(configured) {
		return python, true
	}
	base, err := runtimeRepairExecutablePath(pythonForSetup(root, s.CfgStore.Snapshot()))
	if err != nil {
		result.Errors = append(result.Errors, "找不到可用于重建虚拟环境的 Python: "+err.Error())
		return python, false
	}
	repairCtx, cancel := context.WithTimeout(ctx, setupCommandTimeout)
	defer cancel()
	output, createErr := runSetupCommandOutputFunc(repairCtx, root, base, "-m", "venv", setupVenvDir(root))
	if createErr != nil {
		result.Errors = append(result.Errors, "重建 GA 虚拟环境失败: "+trimRepairOutput(output+"\n"+createErr.Error()))
		return python, false
	}
	if !isAbsoluteExistingFile(managed) {
		result.Errors = append(result.Errors, "虚拟环境创建完成，但未找到 Python: "+managed)
		return python, false
	}
	result.Python = managed
	result.Operations = append(result.Operations, "已在 GA 根目录重建稳定虚拟环境: "+setupVenvDir(root))
	return managed, true
}

func isAbsoluteExistingFile(path string) bool {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func trimRepairOutput(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= 1200 {
		return output
	}
	return "..." + output[len(output)-1200:]
}
