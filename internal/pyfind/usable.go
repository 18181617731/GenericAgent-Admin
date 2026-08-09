package pyfind

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// probeModules are the third-party imports GA needs at runtime. agentmain
// itself imports nothing external at module scope, so importing it proves
// nothing; llmcore is what pulls requests/urllib3, and a missing requests is
// exactly what makes the chat worker die with ModuleNotFoundError and the
// models list come back empty.
var probeModules = []string{"requests", "urllib3"}

const probeTimeout = 6 * time.Second

// probeInterpreter reports whether p can import GA's runtime dependencies.
// It is a variable so tests can assert the candidate order without paying for
// real subprocesses.
var probeInterpreter = func(p string) bool {
	if strings.TrimSpace(p) == "" || IsWindowsAppsPythonAlias(p) {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, p, "-c", "import "+strings.Join(probeModules, ", "))
	hideProbeWindow(cmd)
	return cmd.Run() == nil
}

var probeCache struct {
	mu sync.Mutex
	m  map[string]bool
}

// probeUsable memoizes probe results. Resolution runs on every chat request and
// on every models refresh; without the cache each one would spawn a handful of
// interpreters just to answer a question whose answer does not change while the
// process lives.
func probeUsable(p string) bool {
	key := strings.TrimSpace(p)
	if key == "" {
		return false
	}
	probeCache.mu.Lock()
	if probeCache.m == nil {
		probeCache.m = map[string]bool{}
	}
	if ok, seen := probeCache.m[key]; seen {
		probeCache.mu.Unlock()
		return ok
	}
	probeCache.mu.Unlock()

	ok := probeInterpreter(key)

	probeCache.mu.Lock()
	probeCache.m[key] = ok
	probeCache.mu.Unlock()
	return ok
}

// ResetProbeCache clears memoized probe results. Callers use it after
// installing dependencies into an interpreter, so a root that just gained
// requests is not judged by a stale answer.
func ResetProbeCache() {
	probeCache.mu.Lock()
	probeCache.m = nil
	probeCache.mu.Unlock()
}

// ResolveUsable picks an interpreter for gaRoot that can actually import GA's
// runtime dependencies.
//
// Resolve answers a narrower question: which interpreter path exists, most
// specific first. That is the right answer for provisioning and for status
// reporting, but it is the wrong answer for running GA code. A freshly
// installed instance has no virtualenv of its own and no configured
// interpreter, so Resolve falls through to a host interpreter (a uv-managed
// CPython, typically) that has never had GA's requirements installed. Chat then
// reported "no models" and the worker exited before done, because both children
// died on `import requests`.
//
// Candidates are tried in the same order Resolve prefers them - configured,
// then a virtualenv inside the root, then fallbacks supplied by the caller
// (sibling instance roots and the admin's own GA root), then host discovery -
// and the first one that imports the dependencies wins. When nothing imports
// them, the Resolve answer is returned unchanged: the caller's error message
// about a missing module is more useful than a silent empty string.
func ResolveUsable(gaRoot, configured string, fallbacks []string) string {
	base := Resolve(gaRoot, configured)
	for _, c := range usableCandidates(gaRoot, configured, fallbacks) {
		if probeUsable(c) {
			return c
		}
	}
	return base
}

// usableCandidates lists every interpreter worth probing for gaRoot, in
// preference order and without duplicates.
func usableCandidates(gaRoot, configured string, fallbacks []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		key := strings.ToLower(p)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, p)
	}

	add(configured)
	for _, c := range rootCandidates(strings.TrimSpace(gaRoot)) {
		if existsFile(c) {
			add(c)
		}
	}
	for _, f := range fallbacks {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		// A fallback may be either an interpreter or another GA root; accept
		// both so callers can pass instance roots straight through.
		if existsFile(f) {
			add(f)
			continue
		}
		for _, c := range rootCandidates(f) {
			if existsFile(c) {
				add(c)
			}
		}
	}
	if runtime.GOOS == "windows" {
		add(LatestUVWindowsPython())
		add(lookUsable("python"))
	} else {
		add(lookUsable("python3"))
		add(lookUsable("python"))
	}
	return out
}
