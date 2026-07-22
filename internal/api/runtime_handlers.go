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

func (s *Server) gaRuntimeRepair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	root, err := setupRequestRoot(r, s.CfgStore.Cfg.GARoot)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	python := resolvePythonForRoot(root, s.CfgStore.Cfg.EffectivePython)
	result := runtimeRepairResult{Python: python, CheckedAt: time.Now().Format(time.RFC3339)}
	result.Before = ga.BuildRuntimeHealth(root, python)
	if result.Before.Runtime == nil {
		result.Errors = append(result.Errors, "无法读取 GA 运行时状态")
		result.After = result.Before
		writeJSON(w, result)
		return
	}

	packages := ga.MissingDependencyPackages(result.Before.Runtime)
	if len(packages) > 0 && result.Before.Runtime.PythonOK {
		ctx, cancel := context.WithTimeout(r.Context(), setupCommandTimeout)
		args := []string{"-m", "pip", "install", "--disable-pip-version-check", "--no-input"}
		args = append(args, packages...)
		output, installErr := runSetupCommandOutputFunc(ctx, root, python, args...)
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

	if legacy := result.Before.Runtime.LegacyUltraplanScripts; len(legacy) > 0 {
		repaired, repairErr := ga.RepairLegacyUltraplanScripts(root)
		if repairErr != nil {
			result.Errors = append(result.Errors, "迁移旧 UltraPlan 脚本失败: "+repairErr.Error())
		}
		result.Repaired = append(result.Repaired, repaired...)
		if len(repaired) > 0 {
			result.Operations = append(result.Operations, "已迁移旧 UltraPlan 脚本: "+strings.Join(repaired, ", "))
		}
		if len(repaired) != len(legacy) && repairErr == nil {
			result.Errors = append(result.Errors, "部分旧 UltraPlan 脚本未能迁移")
		}
	}

	result.After = ga.BuildRuntimeHealth(root, python)
	if result.After.Runtime != nil && isAbsoluteExistingFile(result.After.Runtime.PythonPath) {
		result.Python = result.After.Runtime.PythonPath
		cfg := s.CfgStore.Cfg
		if cfg.PythonPath != result.Python {
			cfg.GARoot = root
			cfg.PythonPath = result.Python
			if saveErr := s.CfgStore.Save(cfg); saveErr != nil {
				result.Errors = append(result.Errors, "保存已验证 Python 路径失败: "+saveErr.Error())
			} else {
				s.Svc.SetRoot(cfg.GARoot, cfg.EffectivePython, cfg.BufferLines)
				result.Operations = append(result.Operations, "已固定本次验证成功的 Python: "+result.Python)
			}
		}
	}
	result.OK = result.After.OK && len(result.Errors) == 0
	writeJSON(w, result)
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
