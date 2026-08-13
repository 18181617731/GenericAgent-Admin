package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"genericagent-admin-go/internal/config"
)

func TestAppRootExplicitRootTakesPrecedence(t *testing.T) {
	explicit := t.TempDir()
	got, err := appRoot(explicit)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("appRoot(%q) = %q, want %q", explicit, got, want)
	}
}

func portableConfigFixture(t *testing.T) (string, config.AppConfig) {
	t.Helper()
	cwd := t.TempDir()
	gaRoot := filepath.Join(cwd, "GenericAgent")
	pythonPath := filepath.Join(gaRoot, ".venv", "bin", "python")
	if runtime.GOOS == "windows" {
		pythonPath = filepath.Join(gaRoot, ".venv", "Scripts", "python.exe")
	}
	if err := os.MkdirAll(filepath.Dir(pythonPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pythonPath, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.GARoot = gaRoot
	cfg.PythonPath = pythonPath
	cfg.EffectivePython = pythonPath
	return cwd, cfg
}

func writePortableLayout(t *testing.T, root string) (string, string) {
	t.Helper()
	bootstrapPy := filepath.Join(root, "bootstrap.py")
	pythonExe := filepath.Join(root, "python", "bin", "python3")
	if runtime.GOOS == "windows" {
		pythonExe = filepath.Join(root, "python", "python.exe")
	}
	if err := os.MkdirAll(filepath.Dir(pythonExe), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{bootstrapPy, pythonExe} {
		if err := os.WriteFile(path, []byte("portable-test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return bootstrapPy, pythonExe
}

func TestResolvePortableBootstrapPrefersCanonicalLayout(t *testing.T) {
	bundle := t.TempDir()
	wantBootstrap, wantPython := writePortableLayout(t, bundle)
	writePortableLayout(t, filepath.Join(bundle, "GenericAgent"))

	bootstrapPy, pythonExe, ok := resolvePortableBootstrap(bundle)
	if !ok {
		t.Fatal("resolvePortableBootstrap() ok = false")
	}
	if bootstrapPy != wantBootstrap || pythonExe != wantPython {
		t.Fatalf("resolvePortableBootstrap() = (%q, %q), want (%q, %q)", bootstrapPy, pythonExe, wantBootstrap, wantPython)
	}
}

func TestResolvePortableBootstrapSupportsLegacyNestedLayout(t *testing.T) {
	bundle := t.TempDir()
	wantBootstrap, wantPython := writePortableLayout(t, filepath.Join(bundle, "GenericAgent"))

	bootstrapPy, pythonExe, ok := resolvePortableBootstrap(bundle)
	if !ok {
		t.Fatal("resolvePortableBootstrap() ok = false")
	}
	if bootstrapPy != wantBootstrap || pythonExe != wantPython {
		t.Fatalf("resolvePortableBootstrap() = (%q, %q), want (%q, %q)", bootstrapPy, pythonExe, wantBootstrap, wantPython)
	}
}

func TestValidatePortableConfigAcceptsCurrentBundlePaths(t *testing.T) {
	cwd, cfg := portableConfigFixture(t)
	if err := validatePortableConfig(cwd, cfg); err != nil {
		t.Fatalf("validatePortableConfig() error = %v", err)
	}
}

func TestValidatePortableConfigRejectsIncompletePaths(t *testing.T) {
	cwd, valid := portableConfigFixture(t)
	tests := []struct {
		name string
		cfg  config.AppConfig
	}{
		{name: "empty ga_root", cfg: func() config.AppConfig { cfg := valid; cfg.GARoot = ""; return cfg }()},
		{name: "wrong ga_root", cfg: func() config.AppConfig { cfg := valid; cfg.GARoot = t.TempDir(); return cfg }()},
		{name: "empty python_path", cfg: func() config.AppConfig { cfg := valid; cfg.PythonPath = ""; cfg.EffectivePython = ""; return cfg }()},
		{name: "wrong python_path", cfg: func() config.AppConfig {
			cfg := valid
			cfg.PythonPath = filepath.Join(t.TempDir(), "python.exe")
			cfg.EffectivePython = ""
			return cfg
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validatePortableConfig(cwd, tt.cfg); err == nil {
				t.Fatal("validatePortableConfig() error = nil, want rejection")
			}
		})
	}
}

func TestValidatePortableConfigRejectsPathsFromExistingOldBundle(t *testing.T) {
	_, oldCfg := portableConfigFixture(t)
	newCwd, _ := portableConfigFixture(t)
	if err := validatePortableConfig(newCwd, oldCfg); err == nil {
		t.Fatal("validatePortableConfig() accepted paths from an old bundle location")
	}
}
