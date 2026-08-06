package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInstanceInstallDownloadsRegistersAndAllocatesUniqueDestination(t *testing.T) {
	s := newConfigTestServer(t)
	oldDownload := downloadAndExtractGenericAgentArchive
	t.Cleanup(func() { downloadAndExtractGenericAgentArchive = oldDownload })

	var destinations []string
	downloadAndExtractGenericAgentArchive = func(ctx context.Context, dest string) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		destinations = append(destinations, dest)
		if err := os.MkdirAll(filepath.Join(dest, "assets"), 0755); err != nil {
			return "", err
		}
		for _, rel := range []string{"agentmain.py", "llmcore.py", filepath.Join("assets", "tools_schema.json")} {
			if err := os.WriteFile(filepath.Join(dest, rel), []byte("# fixture\n"), 0644); err != nil {
				return "", err
			}
		}
		return "fixture archive extracted", nil
	}

	h := s.Routes()
	for i := 0; i < 2; i++ {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/instances/install", nil)
		markDangerous(req)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("install %d status=%d body=%s", i+1, rr.Code, rr.Body.String())
		}
		var response struct {
			ArchiveURL      string `json:"archive_url"`
			DefaultInstance string `json:"default_instance_id"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode install %d response: %v", i+1, err)
		}
		if response.ArchiveURL != genericAgentArchiveURL {
			t.Fatalf("archive_url=%q want %q", response.ArchiveURL, genericAgentArchiveURL)
		}
		if response.DefaultInstance != automaticInstanceBaseID {
			t.Fatalf("default_instance_id=%q want %q", response.DefaultInstance, automaticInstanceBaseID)
		}
	}

	wantDestinations := []string{
		filepath.Join(s.CfgStore.Root, "instances", "genericagent"),
		filepath.Join(s.CfgStore.Root, "instances", "genericagent-2"),
	}
	if len(destinations) != len(wantDestinations) {
		t.Fatalf("destinations=%v want %v", destinations, wantDestinations)
	}
	for i, want := range wantDestinations {
		if destinations[i] != want {
			t.Errorf("destination %d=%q want %q", i, destinations[i], want)
		}
		if _, err := os.Stat(want); err != nil {
			t.Errorf("installed destination %q: %v", want, err)
		}
	}
	if got := s.CfgStore.Cfg.Instances; len(got) != 2 {
		t.Fatalf("instances=%v want 2 entries", got)
	} else {
		if got[0].ID != "genericagent" || got[0].Name != "GenericAgent" {
			t.Errorf("first instance=%+v", got[0])
		}
		if got[1].ID != "genericagent-2" || got[1].Name != "GenericAgent 2" {
			t.Errorf("second instance=%+v", got[1])
		}
		for i := range got {
			wantRoot := wantDestinations[i]
			if got[i].GARoot != wantRoot {
				t.Errorf("instance %d ga_root=%q want %q", i, got[i].GARoot, wantRoot)
			}
		}
	}
	if s.CfgStore.Cfg.DefaultInstanceID != "genericagent" {
		t.Errorf("default instance=%q want genericagent", s.CfgStore.Cfg.DefaultInstanceID)
	}
}

func TestInstanceInstallRequiresDangerousConfirmation(t *testing.T) {
	s := newConfigTestServer(t)
	called := false
	oldDownload := downloadAndExtractGenericAgentArchive
	t.Cleanup(func() { downloadAndExtractGenericAgentArchive = oldDownload })
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

func TestInstanceInstallFailureRemovesPartialDestination(t *testing.T) {
	s := newConfigTestServer(t)
	oldDownload := downloadAndExtractGenericAgentArchive
	t.Cleanup(func() { downloadAndExtractGenericAgentArchive = oldDownload })
	var partialDest string
	downloadAndExtractGenericAgentArchive = func(_ context.Context, dest string) (string, error) {
		partialDest = dest
		if err := os.MkdirAll(dest, 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dest, "partial.zip"), []byte("partial"), 0644); err != nil {
			return "", err
		}
		return "", errors.New("network interrupted")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/instances/install", nil)
	markDangerous(req)
	s.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadGateway, rr.Body.String())
	}
	if partialDest == "" {
		t.Fatal("download stub was not called")
	}
	if _, err := os.Stat(partialDest); !os.IsNotExist(err) {
		t.Fatalf("partial destination still exists or could not be inspected: %v", err)
	}
	if len(s.CfgStore.Cfg.Instances) != 0 || s.CfgStore.Cfg.DefaultInstanceID != "" {
		t.Fatalf("config changed after failed install: %+v", s.CfgStore.Cfg)
	}
}

func TestDownloadGenericAgentArchiveWithFallbackUsesPrimary(t *testing.T) {
	var urls []string
	download := func(_ context.Context, archiveURL, dest string) (string, error) {
		urls = append(urls, archiveURL)
		if dest != "target" {
			t.Fatalf("dest=%q want target", dest)
		}
		return "primary extracted", nil
	}

	out, err := downloadGenericAgentArchiveWithFallback(context.Background(), "target", download)
	if err != nil {
		t.Fatalf("download error: %v", err)
	}
	if out != "primary extracted" {
		t.Fatalf("output=%q", out)
	}
	if len(urls) != 1 || urls[0] != genericAgentArchiveURL {
		t.Fatalf("urls=%v want primary only", urls)
	}
}

func TestDownloadGenericAgentArchiveWithFallbackUsesCodeload(t *testing.T) {
	var urls []string
	primaryHadDeadline := false
	fallbackHadDeadline := false
	download := func(ctx context.Context, archiveURL, _ string) (string, error) {
		urls = append(urls, archiveURL)
		_, hasDeadline := ctx.Deadline()
		if archiveURL == genericAgentArchiveURL {
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
