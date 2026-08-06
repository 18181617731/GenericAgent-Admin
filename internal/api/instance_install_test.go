package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"genericagent-admin-go/internal/config"
)

type instanceInstallResponse struct {
	OK                bool                    `json:"ok"`
	Items             []config.InstanceConfig `json:"items"`
	DefaultInstanceID string                  `json:"default_instance_id"`
	Instance          config.InstanceConfig   `json:"instance"`
	ArchiveURL        string                  `json:"archive_url"`
}

func installInstanceForTest(t *testing.T, h http.Handler) instanceInstallResponse {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/instances/install", nil)
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("install status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response instanceInstallResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode install response: %v; body=%s", err, rr.Body.String())
	}
	return response
}

func waitTestSignal(t *testing.T, ch <-chan string, label string) string {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
		return ""
	}
}

func waitInstanceInstallTasksForTest(t *testing.T, s *Server) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		s.instanceInstallWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for instance install tasks")
	}
}

func writeValidGenericAgentFixture(dest string) error {
	if err := os.MkdirAll(filepath.Join(dest, "assets"), 0755); err != nil {
		return err
	}
	for _, rel := range []string{"agentmain.py", "llmcore.py", filepath.Join("assets", "tools_schema.json")} {
		if err := os.WriteFile(filepath.Join(dest, rel), []byte("# fixture\n"), 0644); err != nil {
			return err
		}
	}
	return nil
}

func instanceFromStoreForTest(t *testing.T, s *Server, id string) config.InstanceConfig {
	t.Helper()
	s.ConfigMu.Lock()
	defer s.ConfigMu.Unlock()
	instance, ok := s.CfgStore.Snapshot().Instance(id)
	if !ok {
		t.Fatalf("instance %q not found in %+v", id, s.CfgStore.Snapshot().Instances)
	}
	return instance
}

func TestInstanceInstallReturnsInitializingAndAllocatesUniqueDestinations(t *testing.T) {
	s := newConfigTestServer(t)
	oldDownload := downloadAndExtractGenericAgentArchive
	t.Cleanup(func() {
		s.stopInstanceInstalls()
		downloadAndExtractGenericAgentArchive = oldDownload
	})

	started := make(chan string, 2)
	release := make(chan struct{})
	downloadAndExtractGenericAgentArchive = func(ctx context.Context, dest string) (string, error) {
		started <- dest
		select {
		case <-release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		if err := writeValidGenericAgentFixture(dest); err != nil {
			return "", err
		}
		return "fixture archive extracted", nil
	}

	h := s.Routes()
	first := installInstanceForTest(t, h)
	firstDest := waitTestSignal(t, started, "first download")
	second := installInstanceForTest(t, h)
	secondDest := waitTestSignal(t, started, "second download")

	wantFirst := filepath.Join(s.CfgStore.Root, "genericagent")
	wantSecond := filepath.Join(s.CfgStore.Root, "genericagent-2")
	for label, result := range map[string]instanceInstallResponse{"first": first, "second": second} {
		if !result.OK {
			t.Errorf("%s response ok=false: %+v", label, result)
		}
		if result.Instance.InitStatus != config.InstanceInitStatusInitializing || result.Instance.InitError != "" {
			t.Errorf("%s response state=%q error=%q", label, result.Instance.InitStatus, result.Instance.InitError)
		}
		if result.ArchiveURL != genericAgentArchiveURL {
			t.Errorf("%s archive_url=%q want %q", label, result.ArchiveURL, genericAgentArchiveURL)
		}
	}
	if first.Instance.ID != "genericagent" || first.Instance.Name != "GenericAgent" || first.Instance.GARoot != wantFirst {
		t.Errorf("first instance=%+v", first.Instance)
	}
	if second.Instance.ID != "genericagent-2" || second.Instance.Name != "GenericAgent 2" || second.Instance.GARoot != wantSecond {
		t.Errorf("second instance=%+v", second.Instance)
	}
	if firstDest != wantFirst || secondDest != wantSecond {
		t.Errorf("download destinations=(%q, %q) want (%q, %q)", firstDest, secondDest, wantFirst, wantSecond)
	}
	if first.DefaultInstanceID != "genericagent" || second.DefaultInstanceID != "genericagent" {
		t.Errorf("default ids=(%q, %q) want genericagent", first.DefaultInstanceID, second.DefaultInstanceID)
	}
	if got := len(second.Items); got != 2 {
		t.Errorf("second response items=%d want 2", got)
	}
	for _, path := range []string{wantFirst, wantSecond} {
		if st, err := os.Stat(path); err != nil || !st.IsDir() {
			t.Errorf("reserved destination %q: stat=%v err=%v", path, st, err)
		}
	}
	for _, id := range []string{"genericagent", "genericagent-2"} {
		if got := instanceFromStoreForTest(t, s, id).InitStatus; got != config.InstanceInitStatusInitializing {
			t.Errorf("persisted %s state=%q want initializing", id, got)
		}
	}

	close(release)
	waitInstanceInstallTasksForTest(t, s)
	for _, id := range []string{"genericagent", "genericagent-2"} {
		instance := instanceFromStoreForTest(t, s, id)
		if instance.InitStatus != config.InstanceInitStatusReady || instance.InitError != "" {
			t.Errorf("completed %s state=%q error=%q", id, instance.InitStatus, instance.InitError)
		}
	}
}

func TestInstanceInstallRequiresDangerousConfirmation(t *testing.T) {
	s := newConfigTestServer(t)
	called := false
	oldDownload := downloadAndExtractGenericAgentArchive
	t.Cleanup(func() {
		s.stopInstanceInstalls()
		downloadAndExtractGenericAgentArchive = oldDownload
	})
	downloadAndExtractGenericAgentArchive = func(context.Context, string) (string, error) {
		called = true
		return "", errors.New("must not run")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/instances/install", nil)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code == http.StatusOK {
		t.Fatalf("install without dangerous confirmation unexpectedly succeeded: %s", rr.Body.String())
	}
	if called {
		t.Fatal("download ran without dangerous confirmation")
	}
}

func TestInstanceInstallFailurePersistsFailedStateAndCleansPartialFiles(t *testing.T) {
	s := newConfigTestServer(t)
	oldDownload := downloadAndExtractGenericAgentArchive
	t.Cleanup(func() {
		s.stopInstanceInstalls()
		downloadAndExtractGenericAgentArchive = oldDownload
	})

	started := make(chan string, 1)
	release := make(chan struct{})
	downloadAndExtractGenericAgentArchive = func(ctx context.Context, dest string) (string, error) {
		if err := os.WriteFile(filepath.Join(dest, "partial.zip"), []byte("partial"), 0644); err != nil {
			return "", err
		}
		started <- dest
		select {
		case <-release:
			return "", errors.New("network interrupted")
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	response := installInstanceForTest(t, s.Routes())
	partialDest := waitTestSignal(t, started, "failed download start")
	if response.Instance.InitStatus != config.InstanceInitStatusInitializing {
		t.Fatalf("response state=%q want initializing", response.Instance.InitStatus)
	}
	if got := instanceFromStoreForTest(t, s, response.Instance.ID).InitStatus; got != config.InstanceInitStatusInitializing {
		t.Fatalf("persisted state before release=%q want initializing", got)
	}

	close(release)
	waitInstanceInstallTasksForTest(t, s)
	instance := instanceFromStoreForTest(t, s, response.Instance.ID)
	if instance.InitStatus != config.InstanceInitStatusFailed {
		t.Errorf("state=%q want failed", instance.InitStatus)
	}
	if !strings.Contains(instance.InitError, "network interrupted") {
		t.Errorf("init_error=%q does not contain network failure", instance.InitError)
	}
	entries, err := os.ReadDir(partialDest)
	if err != nil {
		t.Fatalf("read cleaned destination: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("cleaned destination contains %v", entries)
	}
}

func TestInstanceDeleteCancelsBackgroundInstall(t *testing.T) {
	s := newConfigTestServer(t)
	oldDownload := downloadAndExtractGenericAgentArchive
	t.Cleanup(func() {
		s.stopInstanceInstalls()
		downloadAndExtractGenericAgentArchive = oldDownload
	})

	started := make(chan string, 1)
	cancelled := make(chan string, 1)
	downloadAndExtractGenericAgentArchive = func(ctx context.Context, dest string) (string, error) {
		if err := os.WriteFile(filepath.Join(dest, "partial.zip"), []byte("partial"), 0644); err != nil {
			return "", err
		}
		started <- dest
		<-ctx.Done()
		cancelled <- dest
		return "", ctx.Err()
	}

	response := installInstanceForTest(t, s.Routes())
	dest := waitTestSignal(t, started, "download start")
	body := strings.NewReader(`{"id":"` + response.Instance.ID + `"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/instances/delete", body)
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := waitTestSignal(t, cancelled, "download cancellation"); got != dest {
		t.Errorf("cancelled destination=%q want %q", got, dest)
	}
	if _, ok := s.CfgStore.Snapshot().Instance(response.Instance.ID); ok {
		t.Fatalf("deleted instance remains in config: %+v", s.CfgStore.Snapshot().Instances)
	}
	if s.CfgStore.Snapshot().DefaultInstanceID != "" {
		t.Errorf("default instance=%q want empty", s.CfgStore.Snapshot().DefaultInstanceID)
	}
	if _, err := os.Stat(filepath.Join(dest, "partial.zip")); !os.IsNotExist(err) {
		t.Errorf("partial file remains after cancellation: %v", err)
	}
}

func TestInstanceInstallResumesAfterServerRestart(t *testing.T) {
	oldDownload := downloadAndExtractGenericAgentArchive
	var calls atomic.Int32
	started := make(chan string, 2)
	downloadAndExtractGenericAgentArchive = func(ctx context.Context, dest string) (string, error) {
		call := calls.Add(1)
		started <- dest
		if call == 1 {
			<-ctx.Done()
			return "", ctx.Err()
		}
		if err := writeValidGenericAgentFixture(dest); err != nil {
			return "", err
		}
		return "resumed fixture extracted", nil
	}

	firstServer := newConfigTestServer(t)
	t.Cleanup(func() {
		firstServer.stopInstanceInstalls()
		downloadAndExtractGenericAgentArchive = oldDownload
	})
	response := installInstanceForTest(t, firstServer.Routes())
	firstDest := waitTestSignal(t, started, "initial download")
	firstServer.stopInstanceInstalls()
	if got := instanceFromStoreForTest(t, firstServer, response.Instance.ID).InitStatus; got != config.InstanceInitStatusInitializing {
		t.Fatalf("state after shutdown=%q want initializing", got)
	}

	reloaded := config.NewStore(firstServer.CfgStore.Root)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload config: %v", err)
	}
	resumedServer := New(reloaded, nil, firstServer.Models, nil)
	t.Cleanup(resumedServer.stopInstanceInstalls)
	secondDest := waitTestSignal(t, started, "resumed download")
	if secondDest != firstDest {
		t.Errorf("resumed destination=%q want %q", secondDest, firstDest)
	}
	waitInstanceInstallTasksForTest(t, resumedServer)
	instance := instanceFromStoreForTest(t, resumedServer, response.Instance.ID)
	if instance.InitStatus != config.InstanceInitStatusReady || instance.InitError != "" {
		t.Fatalf("resumed state=%q error=%q", instance.InitStatus, instance.InitError)
	}
	if calls.Load() != 2 {
		t.Errorf("download calls=%d want 2", calls.Load())
	}
}

func TestResumedInstallRefusesUnmanagedRoot(t *testing.T) {
	oldDownload := downloadAndExtractGenericAgentArchive
	var calls atomic.Int32
	downloadAndExtractGenericAgentArchive = func(context.Context, string) (string, error) {
		calls.Add(1)
		return "", errors.New("must not download")
	}
	t.Cleanup(func() { downloadAndExtractGenericAgentArchive = oldDownload })

	store := config.NewStore(t.TempDir())
	unmanaged := t.TempDir()
	marker := filepath.Join(unmanaged, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := store.Snapshot()
	cfg.Instances = []config.InstanceConfig{{
		ID:         "genericagent",
		Name:       "GenericAgent",
		GARoot:     unmanaged,
		InitStatus: config.InstanceInitStatusInitializing,
	}}
	cfg.DefaultInstanceID = "genericagent"
	if err := store.Save(cfg); err != nil {
		t.Fatalf("save unsafe fixture: %v", err)
	}

	s := New(store, nil, nil, nil)
	t.Cleanup(s.stopInstanceInstalls)
	waitInstanceInstallTasksForTest(t, s)
	if calls.Load() != 0 {
		t.Errorf("download called %d times for unmanaged root", calls.Load())
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatalf("unmanaged marker changed: data=%q err=%v", data, err)
	}
	instance := instanceFromStoreForTest(t, s, "genericagent")
	if instance.InitStatus != config.InstanceInitStatusFailed || !strings.Contains(instance.InitError, "unmanaged instance path") {
		t.Fatalf("unsafe resumed state=%q error=%q", instance.InitStatus, instance.InitError)
	}
}

func TestInstanceMutationKeepsInstallStateServerOwned(t *testing.T) {
	s := newConfigTestServer(t)
	root := t.TempDir()
	h := s.Routes()
	createBody, err := json.Marshal(config.InstanceConfig{
		ID:         "manual",
		Name:       "Manual",
		GARoot:     root,
		InitStatus: config.InstanceInitStatusInitializing,
		InitError:  "client supplied",
	})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/instances/create", strings.NewReader(string(createBody)))
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}
	created := instanceFromStoreForTest(t, s, "manual")
	if created.InitStatus != "" || created.InitError != "" {
		t.Fatalf("create accepted client install state: %+v", created)
	}

	created.Name = "Updated"
	created.InitStatus = config.InstanceInitStatusFailed
	created.InitError = "client overwrite"
	updateBody, err := json.Marshal(created)
	if err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/instances/update", strings.NewReader(string(updateBody)))
	markDangerous(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", rr.Code, rr.Body.String())
	}
	updated := instanceFromStoreForTest(t, s, "manual")
	if updated.Name != "Updated" || updated.InitStatus != "" || updated.InitError != "" {
		t.Fatalf("updated instance=%+v", updated)
	}
}

func TestInstanceUpdateRejectsInitializingInstance(t *testing.T) {
	s := newConfigTestServer(t)
	oldDownload := downloadAndExtractGenericAgentArchive
	t.Cleanup(func() {
		s.stopInstanceInstalls()
		downloadAndExtractGenericAgentArchive = oldDownload
	})
	started := make(chan string, 1)
	downloadAndExtractGenericAgentArchive = func(ctx context.Context, dest string) (string, error) {
		started <- dest
		<-ctx.Done()
		return "", ctx.Err()
	}

	response := installInstanceForTest(t, s.Routes())
	waitTestSignal(t, started, "download start")
	changed := response.Instance
	changed.Name = "Changed During Install"
	body, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/instances/update", strings.NewReader(string(body)))
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("update status=%d want=%d body=%s", rr.Code, http.StatusConflict, rr.Body.String())
	}
	if got := instanceFromStoreForTest(t, s, response.Instance.ID).Name; got != response.Instance.Name {
		t.Errorf("name changed while initializing: %q", got)
	}
}

func TestDownloadGenericAgentArchiveWithFallbackRetriesPrimaryTimeout(t *testing.T) {
	var urls []string
	primaryHadDeadline := false
	fallbackHadDeadline := true
	download := func(ctx context.Context, url, _ string) (string, error) {
		urls = append(urls, url)
		_, hasDeadline := ctx.Deadline()
		if len(urls) == 1 {
			primaryHadDeadline = hasDeadline
			return "", context.DeadlineExceeded
		}
		fallbackHadDeadline = hasDeadline
		return "fallback extracted", nil
	}

	out, err := downloadGenericAgentArchiveWithFallback(context.Background(), "target", download)
	if err != nil {
		t.Fatalf("download error: %v", err)
	}
	wantOut := "primary archive failed: context deadline exceeded\ncodeload fallback: fallback extracted"
	if out != wantOut {
		t.Fatalf("output=%q want %q", out, wantOut)
	}
	if len(urls) != 2 || urls[0] != genericAgentArchiveURL || urls[1] != genericAgentArchiveFallbackURL {
		t.Fatalf("urls=%v want primary then codeload", urls)
	}
	if !primaryHadDeadline {
		t.Fatal("primary archive context has no deadline")
	}
	if fallbackHadDeadline {
		t.Fatal("codeload fallback inherited the primary-only deadline")
	}
}

func TestDownloadGenericAgentArchiveWithFallbackDoesNotRetryCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	download := func(_ context.Context, _ string, _ string) (string, error) {
		calls++
		cancel()
		return "", context.Canceled
	}

	_, err := downloadGenericAgentArchiveWithFallback(ctx, "target", download)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v want context canceled", err)
	}
	if calls != 1 {
		t.Fatalf("download calls=%d want 1", calls)
	}
}
