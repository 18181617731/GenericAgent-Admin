package ga

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const chatWorkerCompileScript = `
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
compile(path.read_text(encoding="utf-8"), str(path), "exec")
`

type ChatWorkerRuntime struct {
	OK         bool   `json:"ok"`
	ScriptPath string `json:"script_path,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

var runChatWorkerCompileCheck = func(ctx context.Context, root, python, script string) (string, error) {
	cmd := exec.CommandContext(ctx, python, "-c", chatWorkerCompileScript, script)
	cmd.Dir = root
	hideChildWindow(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func ProbeChatWorkerRuntime(root, python, script string) (result ChatWorkerRuntime) {
	started := time.Now()
	result = ChatWorkerRuntime{ScriptPath: strings.TrimSpace(script)}
	defer func() { result.DurationMS = time.Since(started).Milliseconds() }()
	if result.ScriptPath == "" {
		result.Error = "未找到 Chat 对话运行组件"
		return result
	}
	content, err := os.ReadFile(result.ScriptPath)
	if err != nil {
		result.Error = "无法读取 Chat 对话运行组件: " + trimDiagnostic(err.Error())
		return result
	}
	if err := validateChatWorkerHooks(string(content)); err != nil {
		result.Error = err.Error()
		return result
	}
	python = strings.TrimSpace(python)
	if python == "" {
		python = "python"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := runChatWorkerCompileCheck(ctx, root, python, result.ScriptPath)
	if err != nil {
		if ctx.Err() != nil {
			result.Error = "Chat 对话运行组件语法检查超时（10 秒）"
		} else {
			result.Error = "Chat 对话运行组件无法编译: " + trimDiagnostic(firstNonEmpty(out, err.Error()))
		}
		return result
	}
	result.OK = true
	return result
}

func ApplyChatWorkerRuntimeHealth(health *Health, runtime ChatWorkerRuntime) {
	if health == nil {
		return
	}
	if health.Checks == nil {
		health.Checks = map[string]string{}
	}
	health.Checks["chat_runtime"] = checkState(runtime.OK, "failed")
	if health.Runtime != nil {
		health.Runtime.ChatWorkerOK = runtime.OK
		health.Runtime.ChatWorkerPath = runtime.ScriptPath
		health.Runtime.ChatWorkerError = runtime.Error
		health.Runtime.OK = health.Runtime.OK && runtime.OK
	}
	if !runtime.OK {
		health.Errors = append(health.Errors, "Chat 对话运行组件检查失败: "+firstNonEmpty(runtime.Error, "未知异常"))
		health.OK = false
	}
}

func validateChatWorkerHooks(content string) error {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !hasPythonFunctionDefinition(normalized, "_install_outbound_model_hooks") {
		return fmt.Errorf("Chat 对话运行组件缺少模型路由钩子，普通对话会在启动前失败")
	}
	if !strings.Contains(normalized, "restore_model_hooks = _install_outbound_model_hooks(agent)") || !strings.Contains(normalized, "finally:\n        restore_model_hooks()") {
		return fmt.Errorf("Chat 对话运行组件的模型路由钩子不完整")
	}
	return nil
}

func hasPythonFunctionDefinition(content, name string) bool {
	prefix := "def " + name + "("
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
