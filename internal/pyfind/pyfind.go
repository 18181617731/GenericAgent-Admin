// Package pyfind resolves the Python interpreter that GenericAgent tooling
// should execute for a given GA root.
//
// It exists so the Admin API, the goal runner, and the model-config importer
// share one resolution order instead of three copies that drift apart. The
// copies used to disagree about the Microsoft Store python stub, which made
// the models page fail with a bare "exit status 9009" on instances that have
// no virtualenv of their own.
package pyfind

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Resolve returns the interpreter to run for gaRoot.
//
// A configured interpreter always wins and is returned untouched; callers that
// need it validated should stat it themselves. Otherwise a virtualenv inside
// gaRoot is preferred over anything on the host, because that is the
// interpreter the instance was provisioned with. Only when the root has no
// virtualenv does discovery fall back to the host: on Windows the newest
// uv-managed CPython, then a PATH python that is not the Store stub, then a
// bare "python" as a last resort; on POSIX, PATH python3 then python.
//
// The Store stub is skipped on purpose: it is an App Execution Alias that
// exits 9009 with no output, so any command built on it fails in a way that
// looks like a bug in the caller.
func Resolve(gaRoot, configured string) string {
	if c := strings.TrimSpace(configured); c != "" {
		return c
	}
	if p := firstExistingFile(rootCandidates(strings.TrimSpace(gaRoot))); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		if p := LatestUVWindowsPython(); p != "" {
			return p
		}
		if p := lookUsable("python"); p != "" {
			return p
		}
		return "python"
	}
	if p := lookUsable("python3"); p != "" {
		return p
	}
	if p := lookUsable("python"); p != "" {
		return p
	}
	return "python3"
}

// LatestUVWindowsPython returns the newest uv-managed CPython on Windows, or
// an empty string when uv manages no interpreter. uv has shipped both
// %APPDATA%\uv\python and %LOCALAPPDATA%\uv\python as its install root, so
// both are searched.
func LatestUVWindowsPython() string {
	var matches []string
	for _, env := range []string{"APPDATA", "LOCALAPPDATA"} {
		base := strings.TrimSpace(os.Getenv(env))
		if base == "" {
			continue
		}
		found, err := filepath.Glob(filepath.Join(base, "uv", "python", "cpython-*", "python.exe"))
		if err != nil {
			continue
		}
		matches = append(matches, found...)
	}
	if len(matches) == 0 {
		return ""
	}
	// Sort by the cpython-<version>-<platform> directory name so the choice
	// does not depend on which install root happened to be scanned first.
	sort.SliceStable(matches, func(i, j int) bool {
		return strings.ToLower(filepath.Base(filepath.Dir(matches[i]))) <
			strings.ToLower(filepath.Base(filepath.Dir(matches[j])))
	})
	for i := len(matches) - 1; i >= 0; i-- {
		if existsFile(matches[i]) {
			return matches[i]
		}
	}
	return ""
}

// IsWindowsAppsPythonAlias reports whether p is a Microsoft Store App
// Execution Alias for Python (python.exe / python3.exe under
// Microsoft\WindowsApps). Those aliases exit 9009 and print nothing when no
// Store Python is installed.
func IsWindowsAppsPythonAlias(p string) bool {
	clean := strings.ToLower(strings.TrimSpace(p))
	if clean == "" {
		return false
	}
	clean = strings.ReplaceAll(clean, "\\", "/")
	if !strings.Contains(clean, "/microsoft/windowsapps/") {
		return false
	}
	return strings.HasPrefix(clean[strings.LastIndex(clean, "/")+1:], "python")
}

// rootCandidates lists the interpreters a GA root may ship, most specific
// first. Both virtualenv layouts are checked on every platform: a root
// provisioned on POSIX and later opened on Windows (or a root under WSL or a
// bind mount) still carries bin/python, and that interpreter is a better
// answer than anything discovered on the host.
func rootCandidates(gaRoot string) []string {
	if gaRoot == "" {
		return nil
	}
	return []string{
		filepath.Join(gaRoot, ".venv", "Scripts", "python.exe"),
		filepath.Join(gaRoot, "venv", "Scripts", "python.exe"),
		filepath.Join(gaRoot, ".venv", "bin", "python"),
		filepath.Join(gaRoot, "venv", "bin", "python"),
	}
}

func firstExistingFile(candidates []string) string {
	for _, c := range candidates {
		if existsFile(c) {
			return c
		}
	}
	return ""
}

func lookUsable(name string) string {
	p, err := exec.LookPath(name)
	if err != nil || IsWindowsAppsPythonAlias(p) {
		return ""
	}
	return p
}

func existsFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
