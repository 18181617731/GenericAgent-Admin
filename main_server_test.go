package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
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

func TestParseInternalLaunchArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want internalLaunchOptions
	}{
		{
			name: "public arguments only",
			args: []string{"--headless", "--port", "9000"},
			want: internalLaunchOptions{PublicArgs: []string{"--headless", "--port", "9000"}},
		},
		{
			name: "helper split form",
			args: []string{"--headless", "--update-helper", `C:\\temp\\update.json`, "--no-browser"},
			want: internalLaunchOptions{
				HelperManifest: `C:\\temp\\update.json`,
				PublicArgs:     []string{"--headless", "--no-browser"},
			},
		},
		{
			name: "confirmation equal form",
			args: []string{"--port=9001", "--update-confirm=operation-7"},
			want: internalLaunchOptions{
				ConfirmOperation: "operation-7",
				PublicArgs:       []string{"--port=9001"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseInternalLaunchArgs(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseInternalLaunchArgs(%#v) = %#v, want %#v", tt.args, got, tt.want)
			}
		})
	}
}

func TestParseInternalLaunchArgsRejectsUnsafeForms(t *testing.T) {
	for _, args := range [][]string{
		{"--update-helper"},
		{"--update-helper="},
		{"--update-confirm", ""},
		{"--update-confirm=operation-1", "--update-confirm", "operation-2"},
		{"--update-helper", "manifest.json", "--update-confirm", "operation-1"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			if _, err := parseInternalLaunchArgs(args); err == nil {
				t.Fatalf("parseInternalLaunchArgs(%#v) error = nil, want rejection", args)
			}
		})
	}
}

func TestOpenListenerAndConfirmBindsBeforeConfirmation(t *testing.T) {
	var listener net.Listener
	var events []string
	got, err := openListenerAndConfirm(func() (net.Listener, error) {
		var openErr error
		listener, openErr = net.Listen("tcp", "127.0.0.1:0")
		events = append(events, "bound")
		return listener, openErr
	}, "operation-1", func(operationID string) error {
		if listener == nil {
			t.Fatal("confirmation ran before listener bind")
		}
		if operationID != "operation-1" {
			t.Fatalf("confirmation operation = %q", operationID)
		}
		events = append(events, "confirmed")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	if !reflect.DeepEqual(events, []string{"bound", "confirmed"}) {
		t.Fatalf("events = %#v", events)
	}
}

func TestOpenListenerAndConfirmClosesListenerOnConfirmationFailure(t *testing.T) {
	var listener net.Listener
	confirmErr := errors.New("confirmation failed")
	got, err := openListenerAndConfirm(func() (net.Listener, error) {
		var openErr error
		listener, openErr = net.Listen("tcp", "127.0.0.1:0")
		return listener, openErr
	}, "operation-2", func(string) error {
		return confirmErr
	})
	if !errors.Is(err, confirmErr) {
		t.Fatalf("openListenerAndConfirm() error = %v, want %v", err, confirmErr)
	}
	if got != nil {
		t.Fatalf("openListenerAndConfirm() listener = %v, want nil", got)
	}
	if listener == nil {
		t.Fatal("listener was never opened")
	}
	if closeErr := listener.Close(); closeErr == nil {
		t.Fatal("listener remained open after confirmation failure")
	}
}
