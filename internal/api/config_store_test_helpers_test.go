package api

import (
	"testing"

	"genericagent-admin-go/internal/config"
)

func newTestConfigStore(t *testing.T, root string, cfg config.AppConfig) *config.Store {
	t.Helper()
	store, err := config.NewRuntimeStore(root, cfg)
	if err != nil {
		t.Fatalf("create runtime config store: %v", err)
	}
	return store
}

func updateTestConfig(t *testing.T, store *config.Store, update func(*config.AppConfig)) {
	t.Helper()
	if err := store.UpdateRuntime(update); err != nil {
		t.Fatalf("update runtime config: %v", err)
	}
}
