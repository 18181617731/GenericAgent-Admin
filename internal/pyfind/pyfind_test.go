package pyfind

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// isolateHost points host discovery at empty directories so a test only sees
// the interpreters it creates itself. Without it the results would depend on
// whether the machine running the tests happens to have uv or a PATH python.
func isolateHost(t *testing.T) (appData string) {
	t.Helper()
	appData = t.TempDir()
	t.Setenv("APPDATA", appData)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	return appData
}

func writeExecutable(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, nil, 0755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}

// uvPython creates a fake uv-managed CPython under base, mirroring the
// cpython-<version>-<platform> layout uv installs.
func uvPython(t *testing.T, base, version string) string {
	t.Helper()
	dir := "cpython-" + version + "-windows-x86_64-none"
	return writeExecutable(t, filepath.Join(base, "uv", "python", dir, "python.exe"))
}

func TestResolvePrefersConfiguredInterpreter(t *testing.T) {
	isolateHost(t)
	root := t.TempDir()
	writeExecutable(t, filepath.Join(root, ".venv", "Scripts", "python.exe"))
	configured := filepath.Join(t.TempDir(), "custom", "python.exe")
	if got := Resolve(root, "  "+configured+"\t"); got != configured {
		t.Fatalf("Resolve(configured) = %q, want %q", got, configured)
	}
}

// A virtualenv inside the root must win over every host interpreter, in all
// four layouts. Checking only the Scripts layout before falling back to uv is
// what made an instance with its own virtualenv run a uv python instead.
func TestResolvePrefersEveryRootLayoutOverHostInterpreter(t *testing.T) {
	for _, layout := range [][]string{
		{".venv", "Scripts", "python.exe"},
		{"venv", "Scripts", "python.exe"},
		{".venv", "bin", "python"},
		{"venv", "bin", "python"},
	} {
		name := strings.Join(layout, "/")
		t.Run(name, func(t *testing.T) {
			appData := isolateHost(t)
			uvPython(t, appData, "3.12.11")
			root := t.TempDir()
			want := writeExecutable(t, filepath.Join(append([]string{root}, layout...)...))
			if got := Resolve(root, ""); got != want {
				t.Fatalf("Resolve(%s) = %q, want %q", name, got, want)
			}
		})
	}
}

func TestResolvePrefersScriptsLayoutWhenRootHasBoth(t *testing.T) {
	isolateHost(t)
	root := t.TempDir()
	want := writeExecutable(t, filepath.Join(root, ".venv", "Scripts", "python.exe"))
	writeExecutable(t, filepath.Join(root, ".venv", "bin", "python"))
	writeExecutable(t, filepath.Join(root, "venv", "Scripts", "python.exe"))
	if got := Resolve(root, ""); got != want {
		t.Fatalf("Resolve(both layouts) = %q, want %q", got, want)
	}
}

func TestResolveSkipsDirectoriesShapedLikeInterpreters(t *testing.T) {
	isolateHost(t)
	root := t.TempDir()
	shadow := filepath.Join(root, ".venv", "Scripts", "python.exe")
	if err := os.MkdirAll(shadow, 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", shadow, err)
	}
	want := writeExecutable(t, filepath.Join(root, "venv", "bin", "python"))
	if got := Resolve(root, ""); got != want {
		t.Fatalf("Resolve(directory candidate) = %q, want %q", got, want)
	}
}

func TestLatestUVWindowsPythonPicksNewestAcrossInstallRoots(t *testing.T) {
	appData := isolateHost(t)
	uvPython(t, appData, "3.11.9")
	want := uvPython(t, os.Getenv("LOCALAPPDATA"), "3.12.11")
	if got := LatestUVWindowsPython(); got != want {
		t.Fatalf("LatestUVWindowsPython() = %q, want %q", got, want)
	}
}

func TestLatestUVWindowsPythonEmptyWhenUVManagesNothing(t *testing.T) {
	isolateHost(t)
	if got := LatestUVWindowsPython(); got != "" {
		t.Fatalf("LatestUVWindowsPython() = %q, want empty", got)
	}
}

func TestIsWindowsAppsPythonAlias(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"store python", `C:\Users\dev\AppData\Local\Microsoft\WindowsApps\python.exe`, true},
		{"store python3", `C:\Users\dev\AppData\Local\Microsoft\WindowsApps\python3.exe`, true},
		{"mixed case forward slashes", `c:/users/dev/appdata/local/MICROSOFT/WindowsApps/Python.exe`, true},
		{"padded", "  C:\\Users\\dev\\AppData\\Local\\Microsoft\\WindowsApps\\python.exe  ", true},
		{"other store alias", `C:\Users\dev\AppData\Local\Microsoft\WindowsApps\wt.exe`, false},
		{"real interpreter", `C:\Python312\python.exe`, false},
		{"uv interpreter", `C:\Users\dev\AppData\Roaming\uv\python\cpython-3.12.11-windows-x86_64-none\python.exe`, false},
		{"blank", "   ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsWindowsAppsPythonAlias(tc.path); got != tc.want {
				t.Fatalf("IsWindowsAppsPythonAlias(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestResolveHostOrderOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only resolution order")
	}
	t.Run("uv beats PATH python", func(t *testing.T) {
		appData := isolateHost(t)
		want := uvPython(t, appData, "3.12.11")
		pathDir := t.TempDir()
		writeExecutable(t, filepath.Join(pathDir, "python.exe"))
		t.Setenv("PATH", pathDir)
		if got := Resolve(t.TempDir(), ""); got != want {
			t.Fatalf("Resolve(uv and PATH python) = %q, want uv %q", got, want)
		}
	})
	t.Run("PATH python when uv manages nothing", func(t *testing.T) {
		isolateHost(t)
		pathDir := t.TempDir()
		want := writeExecutable(t, filepath.Join(pathDir, "python.exe"))
		t.Setenv("PATH", pathDir)
		if got := Resolve(t.TempDir(), ""); got != want {
			t.Fatalf("Resolve(PATH python) = %q, want %q", got, want)
		}
	})
	// The Store alias exits 9009 with no output, so it must never be handed
	// back as a usable interpreter even though LookPath finds it.
	t.Run("store stub is skipped", func(t *testing.T) {
		isolateHost(t)
		stubDir := filepath.Join(t.TempDir(), "Microsoft", "WindowsApps")
		writeExecutable(t, filepath.Join(stubDir, "python.exe"))
		t.Setenv("PATH", stubDir)
		if got := Resolve(t.TempDir(), ""); got != "python" {
			t.Fatalf("Resolve(store stub only) = %q, want bare python", got)
		}
	})
}

func TestResolveHostOrderOnPosix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only resolution order")
	}
	isolateHost(t)
	pathDir := t.TempDir()
	writeExecutable(t, filepath.Join(pathDir, "python"))
	want := writeExecutable(t, filepath.Join(pathDir, "python3"))
	t.Setenv("PATH", pathDir)
	if got := Resolve(t.TempDir(), ""); got != want {
		t.Fatalf("Resolve(posix PATH) = %q, want python3 %q", got, want)
	}
}
