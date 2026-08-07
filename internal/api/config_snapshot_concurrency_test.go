package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"genericagent-admin-go/internal/config"
)

func TestInstancesListConcurrentConfigUpdates(t *testing.T) {
	s := newConfigTestServer(t)
	h := s.Routes()
	cfg := cloneConfigWithInstances(s.CfgStore.Snapshot())
	cfg.Instances = make([]config.InstanceConfig, 256)
	instancesRoot := t.TempDir()
	for i := range cfg.Instances {
		id := "probe-" + strconv.Itoa(i)
		gaRoot := filepath.Join(instancesRoot, id)
		if err := os.MkdirAll(gaRoot, 0755); err != nil {
			t.Fatal(err)
		}
		cfg.Instances[i] = config.InstanceConfig{ID: id, Name: "probe " + strconv.Itoa(i), GARoot: gaRoot, InitStatus: config.InstanceInitStatusInitializing}
	}
	cfg.DefaultInstanceID = cfg.Instances[0].ID
	updateTestConfig(t, s.CfgStore, func(current *config.AppConfig) {
		*current = cfg
	})

	type instancesResponse struct {
		Items             []config.InstanceConfig `json:"items"`
		DefaultInstanceID string                  `json:"default_instance_id"`
	}

	start := make(chan struct{})
	errs := make(chan error, 16)
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(reader int) {
			defer wg.Done()
			<-start
			for i := 0; i < 200; i++ {
				rr := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
				h.ServeHTTP(rr, req)
				if rr.Code != http.StatusOK {
					errs <- fmt.Errorf("reader %d iteration %d: status=%d body=%s", reader, i, rr.Code, rr.Body.String())
					return
				}
				var got instancesResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
					errs <- fmt.Errorf("reader %d iteration %d: decode response: %w", reader, i, err)
					return
				}
				if len(got.Items) != len(cfg.Instances) || got.Items[0].ID != "probe-0" || got.Items[len(got.Items)-1].ID != "probe-255" {
					errs <- fmt.Errorf("reader %d iteration %d: invalid items snapshot (len=%d)", reader, i, len(got.Items))
					return
				}
				if got.DefaultInstanceID != "" && got.DefaultInstanceID != "probe-0" {
					errs <- fmt.Errorf("reader %d iteration %d: invalid default instance %q", reader, i, got.DefaultInstanceID)
					return
				}
			}
		}(g)
	}
	close(start)
	for i := 0; i < 200; i++ {
		s.ConfigMu.Lock()
		next := cloneConfigWithInstances(s.CfgStore.Snapshot())
		next.DefaultInstanceID = "probe-0"
		if i%2 == 0 {
			next.DefaultInstanceID = ""
		}
		updateTestConfig(t, s.CfgStore, func(current *config.AppConfig) {
			*current = next
		})
		s.ConfigMu.Unlock()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
