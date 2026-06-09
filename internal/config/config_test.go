package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStoreSaveCreatesRootAndWritesLoadableConfig(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing", "config-root")
	store := NewStore(root)
	cfg := Default()
	cfg.Host = "127.0.0.1"
	cfg.Port = 18787
	cfg.LogTailLines = 321

	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	path := filepath.Join(root, "config.local.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("saved config missing: %v", err)
	}
	if info.IsDir() {
		t.Fatalf("saved config is directory")
	}
	if got := info.Mode().Perm(); got&0222 == 0 {
		t.Fatalf("mode=%#o is not writable", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got AppConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("saved config is not valid JSON: %v\n%s", err, data)
	}
	if got.Port != cfg.Port || got.LogTailLines != cfg.LogTailLines {
		t.Fatalf("unexpected saved config: %#v", got)
	}

	reloaded := NewStore(root)
	if reloaded.Cfg.Port != cfg.Port || reloaded.Cfg.LogTailLines != cfg.LogTailLines {
		t.Fatalf("unexpected reloaded config: %#v", reloaded.Cfg)
	}
}

func TestStoreSaveCleansTempFileOnValidationError(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	cfg := Default()
	cfg.Port = -1

	if err := store.Save(cfg); err == nil {
		t.Fatalf("Save() expected validation error")
	}
	matches, err := filepath.Glob(filepath.Join(root, ".config.local.json-*.tmp"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("unexpected temp files after validation failure: %v", matches)
	}
}

func TestStoreLoadRejectsInvalidPersistedConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.local.json"), []byte(`{"port":-1}`), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store := &Store{Root: root, Cfg: Default()}
	before := store.Cfg

	err := store.Load()
	if err == nil || !strings.Contains(err.Error(), "port must be between") {
		t.Fatalf("Load() err = %v, want port validation error", err)
	}
	if !reflect.DeepEqual(store.Cfg, before) {
		t.Fatalf("Load() mutated cfg on validation error: got %#v want %#v", store.Cfg, before)
	}
}

func TestStoreLoadRejectsInvalidRuntimeBounds(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.local.json"), []byte(`{"log_tail_lines":-1,"buffer_lines":-1}`), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store := &Store{Root: root, Cfg: Default()}

	err := store.Load()
	if err == nil || !strings.Contains(err.Error(), "log_tail_lines must be positive") {
		t.Fatalf("Load() err = %v, want log tail validation error", err)
	}
	if store.Cfg.LogTailLines != Default().LogTailLines || store.Cfg.BufferLines != Default().BufferLines {
		t.Fatalf("Load() applied invalid runtime bounds: %#v", store.Cfg)
	}
}
