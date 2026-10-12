package version

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadTelemetryPreservesMirrorFallback(t *testing.T) {
	oldClient, oldMirrors := downloadHTTPClient, updateMirrorPrefixes
	t.Cleanup(func() { downloadHTTPClient, updateMirrorPrefixes = oldClient, oldMirrors })
	t.Setenv("GA_ADMIN_UPDATE_MIRRORS", "")
	t.Setenv("GA_ADMIN_UPDATE_DISABLE_MIRRORS", "false")
	updateMirrorPrefixes = []string{"https://mirror.test/"}
	var requests []string
	downloadHTTPClient = &http.Client{Transport: versionRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.URL.Hostname())
		if req.URL.Hostname() == "github.com" {
			return &http.Response{StatusCode: 503, Status: "503 Service Unavailable", Body: io.NopCloser(strings.NewReader("retry"))}, nil
		}
		return &http.Response{StatusCode: 200, ContentLength: 5, Body: io.NopCloser(strings.NewReader("asset"))}, nil
	})}
	request := downloadTelemetryRequest{URL: "https://github.com/example/admin/releases/download/v1.2.3/asset.zip", Dest: filepath.Join(t.TempDir(), "asset.zip"), MaxBytes: 10, ExpectedBytes: 5}
	var states []DownloadProgress
	source, err := downloadReleaseAssetWithTelemetry(context.Background(), request, func(_ string, state DownloadProgress) { states = append(states, state) })
	if err != nil || source != "镜像 mirror.test" || strings.Join(requests, ",") != "github.com,mirror.test" {
		t.Fatalf("fallback failed: source=%q requests=%v err=%v", source, requests, err)
	}
	data, err := os.ReadFile(request.Dest)
	final := states[len(states)-1]
	if err != nil || string(data) != "asset" || !final.Complete || final.Percent != 100 || final.Attempt != 2 || final.DownloadedBytes != 5 {
		t.Fatalf("fallback content/telemetry mismatch: %q %+v %v", data, final, err)
	}
}
