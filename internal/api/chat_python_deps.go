package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/pyfind"
)

// chatLLMListError keeps the interpreter, root and raw Python output attached to
// a failed model listing. The message is unchanged from the plain fmt.Errorf it
// replaces, so existing log readers see the same text, but callers can now
// recover the pieces a diagnosis needs instead of re-parsing a flat string.
type chatLLMListError struct {
	Stage  string
	Python string
	Root   string
	Output string
	Err    error
}

func (e *chatLLMListError) Error() string {
	return fmt.Sprintf("%s: %v: %s", e.Stage, e.Err, strings.TrimSpace(e.Output))
}

func (e *chatLLMListError) Unwrap() error { return e.Err }

const (
	chatLLMDiagGARoot        = "ga_root_unusable"
	chatLLMDiagMissingModule = "missing_python_module"
	chatLLMDiagPython        = "python_unusable"
	chatLLMDiagNoModels      = "no_models_configured"
	chatLLMDiagUnknown       = "unknown"
)

// chatLLMDiagnosis explains an empty model list. The chat UI used to render a
// bare "no models found" for every cause, which sent first-time users hunting
// through logs for what is almost always one missing pip package.
type chatLLMDiagnosis struct {
	Code            string   `json:"code"`
	Fixable         bool     `json:"fixable"`
	Python          string   `json:"python"`
	GARoot          string   `json:"ga_root"`
	MissingModules  []string `json:"missing_modules,omitempty"`
	InstallPackages []string `json:"install_packages,omitempty"`
	InstallCommand  string   `json:"install_command,omitempty"`
	Hint            string   `json:"hint"`
	Detail          string   `json:"detail,omitempty"`
}

// maxRepairPackages bounds what a single repair may install. Missing module
// names come from Python's own traceback, so the list is normally one or two
// entries; a much longer one means the output is not what we think it is.
const maxRepairPackages = 12

var missingModulePattern = regexp.MustCompile(`No module named '([^']+)'`)

// moduleNamePattern accepts only Python identifiers. Missing module names end
// up as pip arguments, so anything that could pass for a flag is dropped.
var moduleNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// parseMissingPythonModules pulls importable top-level names out of one or more
// ModuleNotFoundError tracebacks, in the order Python reported them.
func parseMissingPythonModules(output string) []string {
	var out []string
	seen := map[string]bool{}
	for _, match := range missingModulePattern.FindAllStringSubmatch(output, -1) {
		name := strings.TrimSpace(match[1])
		// A failed submodule import names the whole path; the installable unit
		// is its top-level package.
		if idx := strings.Index(name, "."); idx > 0 {
			name = name[:idx]
		}
		if !moduleNamePattern.MatchString(name) || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
		if len(out) >= maxRepairPackages {
			break
		}
	}
	return out
}

// pythonModulePipPackages lists the GA runtime imports whose distribution name
// differs from the module name, so a repair installs the package the traceback
// actually asked for.
var pythonModulePipPackages = map[string]string{
	"Crypto":                  "pycryptodome",
	"OpenSSL":                 "pyopenssl",
	"PIL":                     "pillow",
	"bs4":                     "beautifulsoup4",
	"botpy":                   "qq-botpy",
	"cv2":                     "opencv-python",
	"dingtalk_stream":         "dingtalk-stream",
	"dotenv":                  "python-dotenv",
	"fitz":                    "pymupdf",
	"jwt":                     "pyjwt",
	"lark_oapi":               "lark-oapi",
	"simple_websocket_server": "simple-websocket-server",
	"socks":                   "pysocks",
	"telegram":                "python-telegram-bot",
	"wecom_aibot_sdk":         "wecom-aibot-sdk",
	"yaml":                    "pyyaml",
}

func pipPackageForModule(module string) string {
	module = strings.TrimSpace(module)
	if pkg, ok := pythonModulePipPackages[module]; ok {
		return pkg
	}
	return module
}

func pipPackagesForModules(modules []string) []string {
	pkgs := make([]string, 0, len(modules))
	for _, module := range modules {
		if pkg := pipPackageForModule(module); pkg != "" {
			pkgs = append(pkgs, pkg)
		}
	}
	return pkgs
}

func pipInstallArgs(packages []string) []string {
	args := []string{"-m", "pip", "install", "-i", defaultPipIndexURL}
	return append(args, packages...)
}

func pipInstallCommand(python string, packages []string) string {
	if strings.TrimSpace(python) == "" {
		python = "python"
	}
	return python + " " + strings.Join(pipInstallArgs(packages), " ")
}

// pythonLooksLaunchable reports whether the interpreter can plausibly be run at
// all. A path that no longer exists and the Microsoft Store alias both fail
// before Python prints anything, so no traceback is available to parse.
func pythonLooksLaunchable(python string) bool {
	p := strings.TrimSpace(python)
	if p == "" || pyfind.IsWindowsAppsPythonAlias(p) {
		return false
	}
	if filepath.IsAbs(p) || strings.ContainsAny(p, `/\`) {
		st, err := os.Stat(p)
		return err == nil && !st.IsDir()
	}
	_, err := exec.LookPath(p)
	return err == nil
}

func gaRootHasAgentMain(root string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	st, err := os.Stat(filepath.Join(root, "agentmain.py"))
	return err == nil && !st.IsDir()
}

func truncateDiagnosisDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	const limit = 600
	if len(detail) <= limit {
		return detail
	}
	return detail[:limit] + "…"
}

// diagnoseChatLLMList classifies why the chat model list came back empty.
// err == nil with zero models is its own diagnosis: GA started fine and simply
// has nothing configured, which needs a trip to the models page rather than a
// dependency install.
func diagnoseChatLLMList(cfg config.AppConfig, modelCount int, err error) *chatLLMDiagnosis {
	if err == nil && modelCount > 0 {
		return nil
	}
	root := strings.TrimSpace(cfg.GARoot)
	python := ""
	output := ""
	var listErr *chatLLMListError
	if errors.As(err, &listErr) {
		python = strings.TrimSpace(listErr.Python)
		if candidate := strings.TrimSpace(listErr.Root); candidate != "" {
			root = candidate
		}
		output = listErr.Output
	}
	if python == "" {
		python = chatPythonForConfig(cfg)
	}
	diag := &chatLLMDiagnosis{Code: chatLLMDiagUnknown, Python: python, GARoot: root}
	if err != nil {
		diag.Detail = truncateDiagnosisDetail(err.Error())
	}

	missing := parseMissingPythonModules(output)
	switch {
	case err == nil:
		diag.Code = chatLLMDiagNoModels
		diag.Hint = "GA 能正常启动，但还没有配置任何模型。请到「模型」页面导入或填写模型服务商配置。"
	case !gaRootHasAgentMain(root):
		diag.Code = chatLLMDiagGARoot
		if root == "" {
			diag.Hint = "还没有配置 GA 目录。请先在设置里完成首次安装向导。"
		} else {
			diag.Hint = fmt.Sprintf("GA 目录里找不到 agentmain.py：%s。请在设置里重新指定 GA 目录。", root)
		}
	case len(missing) > 0:
		diag.Code = chatLLMDiagMissingModule
		diag.MissingModules = missing
		diag.InstallPackages = pipPackagesForModules(missing)
		diag.InstallCommand = pipInstallCommand(python, diag.InstallPackages)
		diag.Fixable = pythonLooksLaunchable(python)
		diag.Hint = fmt.Sprintf("Python 环境缺少 GA 运行依赖：%s。手动修复可执行：%s", strings.Join(diag.InstallPackages, ", "), diag.InstallCommand)
	case !pythonLooksLaunchable(python):
		diag.Code = chatLLMDiagPython
		diag.Hint = fmt.Sprintf("配置的 Python 解释器无法启动：%s。请在设置里重新选择 Python，或让向导创建虚拟环境。", python)
	default:
		diag.Hint = "GA 返回模型列表失败。展开详情查看 Python 输出。"
	}
	return diag
}

func chatDiagnosisPayload(diag *chatLLMDiagnosis) map[string]interface{} {
	if diag == nil {
		return nil
	}
	payload := map[string]interface{}{
		"code":    diag.Code,
		"fixable": diag.Fixable,
		"python":  diag.Python,
		"ga_root": diag.GARoot,
		"hint":    diag.Hint,
	}
	if len(diag.MissingModules) > 0 {
		payload["missing_modules"] = diag.MissingModules
	}
	if len(diag.InstallPackages) > 0 {
		payload["install_packages"] = diag.InstallPackages
	}
	if diag.InstallCommand != "" {
		payload["install_command"] = diag.InstallCommand
	}
	if diag.Detail != "" {
		payload["detail"] = diag.Detail
	}
	return payload
}

const chatDepsInstallTimeout = 5 * time.Minute

// chatPythonInstallDeps installs the packages the chat interpreter is missing.
// The missing set is re-derived here rather than taken from the request: these
// names become pip arguments, and the server already knows the answer.
func (s *Server) chatPythonInstallDeps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cfg := s.CfgStore.Snapshot()
	llms, listErr := s.listGARuntimeLLMs(cfg)
	diag := diagnoseChatLLMList(cfg, len(llms), listErr)
	if diag == nil {
		writeJSON(w, map[string]interface{}{"ok": true, "already_ok": true, "llm_count": len(llms)})
		return
	}
	if diag.Code != chatLLMDiagMissingModule || !diag.Fixable || len(diag.InstallPackages) == 0 {
		writeJSON(w, map[string]interface{}{"ok": false, "error": diag.Hint, "diagnosis": chatDiagnosisPayload(diag)})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), chatDepsInstallTimeout)
	defer cancel()
	args := pipInstallArgs(diag.InstallPackages)
	cmd := exec.CommandContext(ctx, diag.Python, args...)
	if strings.TrimSpace(diag.GARoot) != "" {
		cmd.Dir = diag.GARoot
	}
	hideChildWindow(cmd)
	cmd.Env = pythonEnvWithAdminProxy(cfg)
	out, runErr := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		bad(w, http.StatusGatewayTimeout, "pip install timed out")
		return
	}
	// Interpreter usability is memoized for the life of the process, so a root
	// that just gained requests would otherwise keep being judged unusable.
	pyfind.ResetProbeCache()

	refreshed := s.CfgStore.Snapshot()
	llms, listErr = s.listGARuntimeLLMs(refreshed)
	after := diagnoseChatLLMList(refreshed, len(llms), listErr)
	resp := map[string]interface{}{
		"ok":        runErr == nil && len(llms) > 0,
		"python":    diag.Python,
		"packages":  diag.InstallPackages,
		"command":   pipInstallCommand(diag.Python, diag.InstallPackages),
		"output":    truncateDiagnosisDetail(string(out)),
		"llm_count": len(llms),
	}
	if runErr != nil {
		resp["error"] = runErr.Error()
	}
	if payload := chatDiagnosisPayload(after); payload != nil {
		resp["diagnosis"] = payload
	}
	writeJSON(w, resp)
}
