package pyfind

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// stubProbe replaces the real subprocess probe with a set of interpreters that
// are declared to have GA's dependencies, and records the order in which
// candidates were offered. Every candidate outside the set fails, which is what
// a uv-managed CPython without requests does in production.
func stubProbe(t *testing.T, usable ...string) *[]string {
	t.Helper()
	ok := map[string]bool{}
	for _, u := range usable {
		ok[strings.ToLower(u)] = true
	}
	var tried []string
	prev := probeInterpreter
	probeInterpreter = func(p string) bool {
		tried = append(tried, p)
		return ok[strings.ToLower(p)]
	}
	ResetProbeCache()
	t.Cleanup(func() {
		probeInterpreter = prev
		ResetProbeCache()
	})
	return &tried
}

// The regression this package exists for: a freshly installed instance has no
// virtualenv and no configured interpreter, so path-only resolution hands back
// a host interpreter that never had GA's requirements installed. Chat then
// reported no models and the worker exited before done. A sibling root that can
// import the dependencies must win instead.
func TestResolveUsableFallsBackWhenRootHasNoUsableInterpreter(t *testing.T) {
	appData := isolateHost(t)
	host := uvPython(t, appData, "3.12.7")
	instanceRoot := t.TempDir()
	sibling := t.TempDir()
	want := writeExecutable(t, filepath.Join(sibling, ".venv", "Scripts", "python.exe"))

	tried := stubProbe(t, want)
	if got := ResolveUsable(instanceRoot, "", []string{sibling}); got != want {
		t.Fatalf("ResolveUsable(no root venv) = %q, want sibling %q", got, want)
	}
	// Path-only resolution is what shipped the bug; assert it really would have
	// picked the dependency-less host, so this test fails if the fix is undone.
	if base := Resolve(instanceRoot, ""); base != host {
		t.Fatalf("Resolve(no root venv) = %q, want host %q (fixture lost its teeth)", base, host)
	}
	if len(*tried) == 0 {
		t.Fatal("no interpreter was probed; resolution stayed path-only")
	}
}

// A root virtualenv that can import the dependencies still wins over both the
// caller's fallbacks and the host, so the fix does not reroute healthy
// instances onto another root's interpreter.
func TestResolveUsablePrefersRootVenvWhenItWorks(t *testing.T) {
	appData := isolateHost(t)
	uvPython(t, appData, "3.12.7")
	root := t.TempDir()
	sibling := t.TempDir()
	siblingPy := writeExecutable(t, filepath.Join(sibling, ".venv", "Scripts", "python.exe"))
	want := writeExecutable(t, filepath.Join(root, ".venv", "Scripts", "python.exe"))

	stubProbe(t, want, siblingPy)
	if got := ResolveUsable(root, "", []string{sibling}); got != want {
		t.Fatalf("ResolveUsable(healthy root) = %q, want root venv %q", got, want)
	}
}

// An explicitly configured interpreter is the operator's decision and outranks
// everything else, exactly as in Resolve.
func TestResolveUsablePrefersConfiguredInterpreter(t *testing.T) {
	isolateHost(t)
	root := t.TempDir()
	rootPy := writeExecutable(t, filepath.Join(root, ".venv", "Scripts", "python.exe"))
	configured := writeExecutable(t, filepath.Join(t.TempDir(), "custom", "python.exe"))

	tried := stubProbe(t, configured, rootPy)
	if got := ResolveUsable(root, "  "+configured+"\t", nil); got != configured {
		t.Fatalf("ResolveUsable(configured) = %q, want %q", got, configured)
	}
	if len(*tried) != 1 || (*tried)[0] != configured {
		t.Fatalf("probe order = %v, want only the configured interpreter", *tried)
	}
}

// Candidates are offered most specific first. Ordering is the whole contract:
// probing is what makes a wrong answer visible, but only the order decides
// which right answer is chosen.
func TestResolveUsableCandidateOrder(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("uv host layout is windows-specific")
	}
	appData := isolateHost(t)
	host := uvPython(t, appData, "3.12.7")
	root := t.TempDir()
	fallbackRoot := t.TempDir()
	rootPy := writeExecutable(t, filepath.Join(root, ".venv", "Scripts", "python.exe"))
	fallbackPy := writeExecutable(t, filepath.Join(fallbackRoot, "venv", "Scripts", "python.exe"))
	configured := filepath.Join(t.TempDir(), "custom", "python.exe")

	// Nothing is usable, so every candidate gets probed and the order shows.
	tried := stubProbe(t)
	ResolveUsable(root, configured, []string{fallbackRoot})
	want := []string{configured, rootPy, fallbackPy, host}
	if len(*tried) != len(want) {
		t.Fatalf("probed %v, want %v", *tried, want)
	}
	for i := range want {
		if !strings.EqualFold((*tried)[i], want[i]) {
			t.Fatalf("probe order = %v, want %v", *tried, want)
		}
	}
}

// A fallback may be given as an interpreter path rather than a GA root, since
// callers hold configured interpreters for other instances.
func TestResolveUsableAcceptsInterpreterFallback(t *testing.T) {
	isolateHost(t)
	want := writeExecutable(t, filepath.Join(t.TempDir(), "other", "python.exe"))

	stubProbe(t, want)
	if got := ResolveUsable(t.TempDir(), "", []string{want}); got != want {
		t.Fatalf("ResolveUsable(interpreter fallback) = %q, want %q", got, want)
	}
}

// When nothing can import the dependencies, resolution must not collapse to an
// empty string: the caller runs the path-only answer and surfaces Python's own
// ModuleNotFoundError, which names the missing package.
func TestResolveUsableFallsBackToResolveWhenNothingIsUsable(t *testing.T) {
	appData := isolateHost(t)
	host := uvPython(t, appData, "3.12.7")

	stubProbe(t)
	if got := ResolveUsable(t.TempDir(), "", nil); got != host {
		t.Fatalf("ResolveUsable(nothing usable) = %q, want Resolve answer %q", got, host)
	}
}

// Missing candidate paths are skipped rather than probed, and duplicates are
// probed once. Resolution runs on every chat request and every models refresh,
// so a duplicate here is a duplicate process spawn there.
func TestResolveUsableSkipsMissingAndDuplicateCandidates(t *testing.T) {
	isolateHost(t)
	root := t.TempDir()
	rootPy := writeExecutable(t, filepath.Join(root, ".venv", "Scripts", "python.exe"))

	tried := stubProbe(t)
	ResolveUsable(root, "", []string{root, rootPy, filepath.Join(t.TempDir(), "gone.exe")})
	if len(*tried) != 1 || !strings.EqualFold((*tried)[0], rootPy) {
		t.Fatalf("probed %v, want the root venv once", *tried)
	}
}

// The real probe must reject the Store alias without executing it: the alias
// exits 9009 and prints nothing, so running it would waste a process and,
// worse, look like an ordinary import failure.
func TestProbeInterpreterRejectsStoreAliasWithoutRunning(t *testing.T) {
	alias := filepath.Join(t.TempDir(), "Microsoft", "WindowsApps", "python.exe")
	if err := os.MkdirAll(filepath.Dir(alias), 0755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if probeInterpreter(alias) {
		t.Fatalf("probeInterpreter(%q) = true, want false for a Store alias", alias)
	}
	if probeInterpreter("   ") {
		t.Fatal("probeInterpreter(blank) = true, want false")
	}
}

// Probing spawns processes, so results are memoized. The cache must be
// per-path, resettable, and safe to hit from the concurrent chat handlers that
// resolve interpreters.
func TestProbeCacheMemoizesPerPathAndResets(t *testing.T) {
	calls := map[string]int{}
	var mu sync.Mutex
	prev := probeInterpreter
	probeInterpreter = func(p string) bool {
		mu.Lock()
		calls[p]++
		mu.Unlock()
		return p == "good"
	}
	ResetProbeCache()
	t.Cleanup(func() {
		probeInterpreter = prev
		ResetProbeCache()
	})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			probeUsable("good")
			probeUsable("bad")
		}()
	}
	wg.Wait()

	if !probeUsable("good") || probeUsable("bad") {
		t.Fatal("probeUsable returned the wrong verdicts")
	}
	mu.Lock()
	goodCalls, badCalls := calls["good"], calls["bad"]
	mu.Unlock()
	if goodCalls == 0 || badCalls == 0 {
		t.Fatalf("probe was never run: good=%d bad=%d", goodCalls, badCalls)
	}

	ResetProbeCache()
	probeUsable("good")
	mu.Lock()
	after := calls["good"]
	mu.Unlock()
	if after <= goodCalls {
		t.Fatalf("calls after reset = %d, want more than %d (cache was not cleared)", after, goodCalls)
	}
}
