package version

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		current string
		latest  string
		want    bool
	}{
		{"dev", "v0.0.7", true},
		{"unknown", "v0.0.7", true},
		{"0.0.6", "v0.0.7", true},
		{"0.0.7", "v0.0.7", false},
		{"0.0.8", "v0.0.7", false},
		{"0.0.10", "v0.0.9", false},
		{"0.1.0", "v0.0.9", false},
	}
	for _, c := range cases {
		if got := newer(c.current, c.latest); got != c.want {
			t.Fatalf("newer(%q,%q)=%v want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestSelectAssets(t *testing.T) {
	want := fmt.Sprintf("ga-admin-v1.2.3-%s-%s.zip", runtime.GOOS, runtime.GOARCH)
	rel := Release{Assets: []Asset{
		{Name: "other.zip"},
		{Name: want},
		{Name: want + ".sha256"},
	}}
	asset, sum := selectAssets(rel)
	if asset == nil || asset.Name != want {
		t.Fatalf("asset=%#v want %s", asset, want)
	}
	if sum == nil || sum.Name != want+".sha256" {
		t.Fatalf("sum=%#v want %s.sha256", sum, want)
	}
}

func TestSelectAssetsRequiresExactPlatformSuffix(t *testing.T) {
	wantSuffix := fmt.Sprintf("%s-%s.zip", runtime.GOOS, runtime.GOARCH)
	rel := Release{Assets: []Asset{
		{Name: "ga-admin-linux-amd64.zip"},
		{Name: "ga-admin-linux-amd64.zip.sha256"},
		{Name: "ga-admin-" + wantSuffix + ".sha256"},
	}}
	asset, sum := selectAssets(rel)
	if asset != nil {
		t.Fatalf("asset=%#v want nil when platform zip is absent", asset)
	}
	if sum == nil || sum.Name != "ga-admin-"+wantSuffix+".sha256" {
		t.Fatalf("sum=%#v want platform checksum without accepting a checksum as zip", sum)
	}
}

func TestEffectiveVersionFallsBackToGit(t *testing.T) {
	oldVersion := Version
	defer func() { Version = oldVersion }()
	Version = "dev"
	got := effectiveVersion()
	if got == "" || got == "unknown" {
		t.Fatalf("effectiveVersion()=%q, want non-empty fallback or dev", got)
	}
}

func TestCurrentUsesInjectedVersion(t *testing.T) {
	oldVersion, oldCommit := Version, Commit
	defer func() { Version, Commit = oldVersion, oldCommit }()
	Version = "1.2.3"
	Commit = "abc1234"
	cur := Current()
	if cur.Version != "1.2.3" || cur.Commit != "abc1234" {
		t.Fatalf("Current()=%#v, want injected version/commit", cur)
	}
	if cur.Runtime == "" || cur.GOOS == "" || cur.GOARCH == "" {
		t.Fatalf("Current()=%#v, want runtime/platform diagnostics", cur)
	}
}

func TestVerifySHA256(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "app.zip")
	if err := os.WriteFile(file, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("payload"))
	sumFile := filepath.Join(dir, "app.zip.sha256")
	if err := os.WriteFile(sumFile, []byte(fmt.Sprintf("%x  app.zip\n", sum)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifySHA256(file, sumFile); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sumFile, []byte("deadbeef app.zip\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifySHA256(file, sumFile); err == nil {
		t.Fatal("expected mismatch")
	}
}

func TestWindowsUpdateScriptQuotesVariablesSafely(t *testing.T) {
	script := windowsUpdateScript(
		`C:\Program Files\GA Admin\ga-admin.exe`,
		`C:\Temp\new ga-admin.exe`,
		`C:\Program Files\GA Admin\ga-admin.exe.bak`,
		`C:\Program Files\GA Admin\cmd\chat_worker.py`,
		`C:\Temp\cmd\chat_worker.py`,
		`C:\Program Files\GA Admin\cmd\chat_worker.py.bak`,
	)
	want := []string{
		`set "OLD=C:\Program Files\GA Admin\ga-admin.exe"`,
		`set "NEW=C:\Temp\new ga-admin.exe"`,
		`set "BAK=C:\Program Files\GA Admin\ga-admin.exe.bak"`,
		`set "WORKER=C:\Program Files\GA Admin\cmd\chat_worker.py"`,
		`set "NEW_WORKER=C:\Temp\cmd\chat_worker.py"`,
		`set "WORKER_BAK=C:\Program Files\GA Admin\cmd\chat_worker.py.bak"`,
		`move /Y "%OLD%" "%BAK%"`,
		`move /Y "%NEW%" "%OLD%"`,
		`move /Y "%NEW_WORKER%" "%WORKER%"`,
	}
	for _, w := range want {
		if !strings.Contains(script, w) {
			t.Fatalf("script missing %q in:\n%s", w, script)
		}
	}
	bad := []string{`set OLD=`, `set NEW=`, `set BAK=`, `set WORKER=`, `""C:\`, `%~dpWORKER%`}
	for _, b := range bad {
		if strings.Contains(script, b) {
			t.Fatalf("script contains unsafe quoting %q in:\n%s", b, script)
		}
	}
}

func TestWindowsUpdateScriptRestoresExeWhenWorkerMoveFails(t *testing.T) {
	script := windowsUpdateScript("old.exe", "new.exe", "old.exe.bak", "cmd/chat_worker.py", "tmp/chat_worker.py", "cmd/chat_worker.py.bak")
	want := []string{
		`for %%D in ("%WORKER%") do if not exist "%%~dpD" mkdir "%%~dpD"`,
		`if exist "%WORKER%" move /Y "%WORKER%" "%WORKER_BAK%"`,
		`if exist "%WORKER_BAK%" move /Y "%WORKER_BAK%" "%WORKER%"`,
		`move /Y "%OLD%" "%NEW%"`,
		`move /Y "%BAK%" "%OLD%"`,
	}
	for _, sub := range want {
		if !strings.Contains(script, sub) {
			t.Fatalf("script missing rollback step %q in:\n%s", sub, script)
		}
	}
}

func TestReleaseAssetContract(t *testing.T) {
	want := fmt.Sprintf("ga-admin-v2.0.0-%s-%s.zip", runtime.GOOS, runtime.GOARCH)
	rel := Release{Assets: []Asset{
		{Name: "ga-admin-v2.0.0-linux-amd64.zip"},
		{Name: want + ".sha256"},
		{Name: want},
	}}
	asset, checksum := selectAssets(rel)
	if asset == nil || asset.Name != want {
		t.Fatalf("zip asset=%#v want %q", asset, want)
	}
	if checksum == nil || checksum.Name != want+".sha256" {
		t.Fatalf("checksum asset=%#v want %q", checksum, want+".sha256")
	}
}

func TestCurrentIncludesBuildDate(t *testing.T) {
	oldVersion, oldCommit, oldDate := Version, Commit, Date
	defer func() { Version, Commit, Date = oldVersion, oldCommit, oldDate }()
	Version = "v9.9.9"
	Commit = "deadbee"
	Date = "2026-05-31T12:00:00Z"
	cur := Current()
	if cur.Version != Version || cur.Commit != Commit || cur.Date != Date {
		t.Fatalf("Current()=%#v, want injected version/commit/date", cur)
	}
}

func TestCurrentReportsUpdateSupportStatus(t *testing.T) {
	cur := Current()
	if runtime.GOOS == "windows" {
		if !cur.UpdateSupported || cur.UpdateUnsupportedReason != "" {
			t.Fatalf("Current()=%#v, want Windows update support", cur)
		}
		return
	}
	if cur.UpdateSupported || cur.UpdateUnsupportedReason == "" {
		t.Fatalf("Current()=%#v, want explicit non-Windows unsupported reason", cur)
	}
}

func TestBuildBatReleaseMetadataContract(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	batPath := filepath.Join(root, "build.bat")
	data, err := os.ReadFile(batPath)
	if err != nil {
		t.Fatalf("read build.bat: %v", err)
	}
	script := string(data)
	want := []string{
		`git describe --tags --dirty --always`,
		`git rev-parse --short HEAD`,
		`Get-Date`,
		`-X genericagent-admin-go/internal/version.Version=%GA_VERSION%`,
		`-X genericagent-admin-go/internal/version.Commit=%GA_COMMIT%`,
		`-X genericagent-admin-go/internal/version.Date=%GA_DATE%`,
		`go build -ldflags="%GA_LDFLAGS%" -o dist\ga-admin.exe .`,
		`copy /Y cmd\chat_worker.py dist\cmd\chat_worker.py`,
	}
	for _, w := range want {
		if !strings.Contains(script, w) {
			t.Fatalf("build.bat missing %q in:\n%s", w, script)
		}
	}
	bad := []string{
		`GenericAgent-Admin-Go/internal/version`,
		`release\`,
		`gh release`,
	}
	for _, b := range bad {
		if strings.Contains(script, b) {
			t.Fatalf("build.bat contains forbidden release/build metadata pattern %q in:\n%s", b, script)
		}
	}
}

func TestUnzipRejectsUnsafePaths(t *testing.T) {
	for _, tc := range []struct {
		name      string
		entryName string
	}{
		{name: "parent", entryName: "../escape.txt"},
		{name: "windows-separator", entryName: `..\\escape.txt`},
		{name: "nested-windows-separator", entryName: `nested\\app.txt`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			zipPath := filepath.Join(dir, "unsafe.zip")
			f, err := os.Create(zipPath)
			if err != nil {
				t.Fatal(err)
			}
			zw := zip.NewWriter(f)
			if w, err := zw.Create(tc.entryName); err != nil {
				t.Fatal(err)
			} else if _, err := w.Write([]byte("escape")); err != nil {
				t.Fatal(err)
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}

			dest := filepath.Join(dir, "dest")
			if err := unzip(zipPath, dest); err == nil || !strings.Contains(err.Error(), "unsafe zip path") {
				t.Fatalf("unzip unsafe path error = %v, want unsafe zip path", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "escape.txt")); !os.IsNotExist(err) {
				t.Fatalf("unsafe zip created escape file, stat err=%v", err)
			}
			if _, err := os.Stat(filepath.Join(dest, `nested\\app.txt`)); !os.IsNotExist(err) {
				t.Fatalf("unsafe zip created backslash-named file, stat err=%v", err)
			}
		})
	}
}

func TestUnzipRemovesFileOnEntryReadError(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "corrupt.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	hdr := &zip.FileHeader{Name: "bad.txt", Method: zip.Store}
	if w, err := zw.CreateHeader(hdr); err != nil {
		t.Fatal(err)
	} else if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	idx := strings.Index(string(data), "hello")
	if idx < 0 {
		t.Fatal("zip payload not found")
	}
	data[idx] = 'H'
	if err := os.WriteFile(zipPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "dest")
	err = unzip(zipPath, dest)
	if err == nil {
		t.Fatal("unzip corrupt entry error = nil")
	}
	if _, statErr := os.Stat(filepath.Join(dest, "bad.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("corrupt extracted file should be removed, stat err=%v", statErr)
	}
	matches, globErr := filepath.Glob(filepath.Join(dest, ".bad.txt-*.tmp"))
	if globErr != nil {
		t.Fatalf("glob temp files: %v", globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("corrupt extracted temp files should be removed: %v", matches)
	}
}

func TestUnzipExtractsRegularFile(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "safe.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	if w, err := zw.Create("nested/app.txt"); err != nil {
		t.Fatal(err)
	} else if _, err := w.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "dest")
	if err := unzip(zipPath, dest); err != nil {
		t.Fatalf("unzip safe file: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "nested", "app.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ok" {
		t.Fatalf("extracted content = %q", got)
	}
}

func TestFindFileReportsWalkError(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	got, err := findFile(missing, "ga-admin.exe")
	if err == nil {
		t.Fatalf("findFile(%q) = %q, nil error; want walk error", missing, got)
	}
	if !strings.Contains(err.Error(), "walk package") {
		t.Fatalf("findFile error = %v, want walk package context", err)
	}
}

func TestFindFileReturnsCaseInsensitiveMatch(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "nested", "GA-ADMIN.EXE")
	if err := os.MkdirAll(filepath.Dir(want), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("exe"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := findFile(dir, "ga-admin.exe")
	if err != nil {
		t.Fatalf("findFile: %v", err)
	}
	if got != want {
		t.Fatalf("findFile = %q, want %q", got, want)
	}
}

func TestStartApplyLatestReportsInitialStatusWriteError(t *testing.T) {
	oldStatus := statusPathOverride
	statusPathOverride = t.TempDir()
	defer func() { statusPathOverride = oldStatus }()

	st, err := StartApplyLatest()
	if err == nil {
		t.Fatalf("expected status write error, got status %+v", st)
	}
	if st.Running || st.Stage != "error" || st.Progress != 100 || st.Error == "" {
		t.Fatalf("unexpected failed status: %+v", st)
	}
	if !strings.Contains(err.Error(), "write update status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCurrentUpdateStatusReportsCorruptStatusFile(t *testing.T) {
	oldStatus := statusPathOverride
	statusPathOverride = filepath.Join(t.TempDir(), "ga-admin-update-status.json")
	defer func() { statusPathOverride = oldStatus }()

	if err := os.WriteFile(statusPathOverride, []byte("{not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	st := CurrentUpdateStatus()
	if st.Running || st.Stage != "error" || st.Progress != 100 || st.Error == "" {
		t.Fatalf("corrupt status = %+v, want readable error status", st)
	}
	if !strings.Contains(st.Message, "读取升级状态失败") || !strings.Contains(st.Error, "invalid character") {
		t.Fatalf("corrupt status message/error = %+v", st)
	}
	if st.UpdatedAt.IsZero() || st.EndedAt.IsZero() {
		t.Fatalf("corrupt status timestamps missing: %+v", st)
	}
}

func TestStartApplyLatestChecksumFailureWritesReadableStatus(t *testing.T) {
	oldURL := repoLatestURL
	oldStatus := statusPathOverride
	statusPathOverride = filepath.Join(t.TempDir(), "ga-admin-update-status.json")
	defer func() { repoLatestURL = oldURL; statusPathOverride = oldStatus }()

	zipPath := filepath.Join(t.TempDir(), "ga-admin-v9.9.9-windows-amd64.zip")
	makeUpdateZip(t, zipPath)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			_ = json.NewEncoder(w).Encode(Release{TagName: "v9.9.9", Assets: []Asset{
				{Name: "ga-admin-v9.9.9-windows-amd64.zip", BrowserDownloadURL: serverURL(r, "/asset.zip")},
				{Name: "ga-admin-v9.9.9-windows-amd64.zip.sha256", BrowserDownloadURL: serverURL(r, "/asset.zip.sha256")},
			}})
		case "/asset.zip":
			http.ServeFile(w, r, zipPath)
		case "/asset.zip.sha256":
			_, _ = w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  ga-admin-v9.9.9-windows-amd64.zip\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	repoLatestURL = server.URL + "/latest"

	st, err := StartApplyLatest()
	if err != nil {
		t.Fatalf("StartApplyLatest: %v", err)
	}
	if !st.Running || st.Stage != "queued" {
		t.Fatalf("initial status = %+v", st)
	}
	final := waitUpdateDone(t)
	if final.Running || final.Stage != "error" {
		t.Fatalf("final status = %+v", final)
	}
	if !strings.Contains(final.Error, "sha256 mismatch") || final.Script != "" {
		t.Fatalf("unexpected error/script: %+v", final)
	}
	if final.Progress != 100 || final.EndedAt.IsZero() || final.Check == nil {
		t.Fatalf("incomplete final status: %+v", final)
	}
	fromAPI := CurrentUpdateStatus()
	if fromAPI.Stage != "error" || !strings.Contains(fromAPI.Message, "sha256 mismatch") {
		t.Fatalf("readable persisted status = %+v", fromAPI)
	}
}

func TestFetchLatestReportsInvalidRequestURL(t *testing.T) {
	oldURL := repoLatestURL
	repoLatestURL = "http://[::1"
	defer func() { repoLatestURL = oldURL }()

	_, err := fetchLatest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "create github release request") {
		t.Fatalf("fetchLatest error = %v, want request creation context", err)
	}
}

func TestFetchLatestRejectsDeclaredOversizedMetadata(t *testing.T) {
	oldURL := repoLatestURL
	defer func() { repoLatestURL = oldURL }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(maxUpdateMetadataBytes+1))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	repoLatestURL = srv.URL

	_, err := fetchLatest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "github release metadata too large") {
		t.Fatalf("fetchLatest error = %v, want metadata size limit", err)
	}
}

func TestFetchLatestRejectsStreamingOversizedMetadata(t *testing.T) {
	oldURL := repoLatestURL
	defer func() { repoLatestURL = oldURL }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"tag_name\":\"v0.0.29\",\"assets\":\""))
		for i := int64(0); i < maxUpdateMetadataBytes; i += 1024 {
			_, _ = w.Write([]byte(strings.Repeat("x", 1024)))
		}
		_, _ = w.Write([]byte("\"}"))
	}))
	defer srv.Close()
	repoLatestURL = srv.URL

	_, err := fetchLatest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "github release metadata too large") {
		t.Fatalf("fetchLatest error = %v, want streaming metadata size limit", err)
	}
}

func TestFetchLatestTimesOutWaitingForResponseHeaders(t *testing.T) {
	oldURL := repoLatestURL
	oldClient := updateHTTPClient
	defer func() { repoLatestURL = oldURL; updateHTTPClient = oldClient }()

	updateHTTPClient = &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 25 * time.Millisecond}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"tag_name":"v0.0.29"}`))
	}))
	defer srv.Close()
	repoLatestURL = srv.URL

	_, err := fetchLatest(context.Background())
	var netErr interface{ Timeout() bool }
	if err == nil || !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("fetchLatest error = %v, want response header timeout", err)
	}
}

func TestDownloadTimesOutWaitingForResponseHeadersAndLeavesNoFile(t *testing.T) {
	oldClient := updateHTTPClient
	defer func() { updateHTTPClient = oldClient }()

	updateHTTPClient = &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 25 * time.Millisecond}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "asset.zip")

	err := download(context.Background(), srv.URL, dest, maxUpdatePackageBytes)
	var netErr interface{ Timeout() bool }
	if err == nil || !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("download error = %v, want response header timeout", err)
	}
	if _, statErr := os.Stat(dest); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("timed-out download should not create dest, stat err=%v", statErr)
	}
}

func TestDownloadReportsInvalidRequestURL(t *testing.T) {
	err := download(context.Background(), "http://[::1", filepath.Join(t.TempDir(), "asset.zip"), maxUpdatePackageBytes)
	if err == nil || !strings.Contains(err.Error(), "create download request") {
		t.Fatalf("download error = %v, want request creation context", err)
	}
}

func TestDownloadRemovesPartialFileOnBodyReadError(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "asset.zip")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("response writer does not support hijacking")
		}
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		_, _ = bufrw.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\npartial")
		_ = bufrw.Flush()
		_ = conn.Close()
	}))
	defer srv.Close()

	err := download(context.Background(), srv.URL, dest, maxUpdatePackageBytes)
	if err == nil {
		t.Fatal("download error = nil, want truncated body error")
	}
	if _, statErr := os.Stat(dest); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("partial download should be removed, stat err=%v", statErr)
	}
	matches, globErr := filepath.Glob(filepath.Join(dir, ".asset.zip-*.tmp"))
	if globErr != nil {
		t.Fatalf("glob temp files: %v", globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("partial download temp files should be removed: %v", matches)
	}
}

func waitUpdateDone(t *testing.T) UpdateStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st := CurrentUpdateStatus()
		if !st.Running && st.Stage != "queued" && st.Stage != "" {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("update did not finish: %+v", CurrentUpdateStatus())
	return UpdateStatus{}
}

func makeUpdateZip(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create("ga-admin.exe")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("new exe"))
	w, err = zw.Create("cmd/chat_worker.py")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("new worker"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func serverURL(r *http.Request, path string) string {
	return "http://" + r.Host + path
}

func TestWriteStatusCreatesParentAndCleansTempFiles(t *testing.T) {
	oldStatus := statusPathOverride
	root := filepath.Join(t.TempDir(), "missing", "state")
	statusPathOverride = filepath.Join(root, "ga-admin-update-status.json")
	defer func() { statusPathOverride = oldStatus }()

	st := UpdateStatus{ID: "atomic-test", Stage: "queued", Progress: 7, Message: "ok"}
	if err := writeStatus(st); err != nil {
		t.Fatalf("writeStatus: %v", err)
	}
	b, err := os.ReadFile(statusPathOverride)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !json.Valid(b) || !strings.Contains(string(b), "atomic-test") {
		t.Fatalf("status file = %q", string(b))
	}
	matches, err := filepath.Glob(filepath.Join(root, ".ga-admin-update-status.json-*.tmp"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("leftover temp files: %v", matches)
	}
}

func TestDownloadRejectsContentLengthAboveLimit(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "asset.zip")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "12")
		_, _ = w.Write([]byte("too large"))
	}))
	defer srv.Close()

	err := download(context.Background(), srv.URL, dest, 4)
	if err == nil || !strings.Contains(err.Error(), "download too large") {
		t.Fatalf("download error = %v, want download too large", err)
	}
	if _, statErr := os.Stat(dest); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("oversized download should not create dest, stat err=%v", statErr)
	}
}

func TestDownloadRejectsStreamingBodyAboveLimitAndRemovesPartial(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "asset.zip")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Del("Content-Length")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = w.Write([]byte("123456789"))
	}))
	defer srv.Close()

	err := download(context.Background(), srv.URL, dest, 4)
	if err == nil || !strings.Contains(err.Error(), "http: request body too large") {
		t.Fatalf("download error = %v, want request body too large", err)
	}
	if _, statErr := os.Stat(dest); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("partial oversized download should be removed, stat err=%v", statErr)
	}
}

func TestCurrentUpdateLimitsPinPackageAndChecksumCeilings(t *testing.T) {
	if maxUpdateMetadataBytes != 2<<20 {
		t.Fatalf("maxUpdateMetadataBytes=%d want %d", maxUpdateMetadataBytes, 2<<20)
	}
	if maxUpdatePackageBytes != 256<<20 {
		t.Fatalf("maxUpdatePackageBytes=%d want %d", maxUpdatePackageBytes, 256<<20)
	}
	if maxUpdateChecksumBytes != 1<<20 {
		t.Fatalf("maxUpdateChecksumBytes=%d want %d", maxUpdateChecksumBytes, 1<<20)
	}
}

func TestCurrentUpdateStatusNormalizesRestartingAfterRelaunch(t *testing.T) {
	oldStatus := statusPathOverride
	statusPathOverride = filepath.Join(t.TempDir(), "ga-admin-update-status.json")
	defer func() { statusPathOverride = oldStatus }()

	started := time.Now().Add(-time.Minute).UTC()
	st := UpdateStatus{ID: "restart-test", PID: os.Getpid() + 1, Running: true, Stage: "restarting", Progress: 95, Message: "升级包已就绪，正在重启服务", StartedAt: started, UpdatedAt: started}
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statusPathOverride, b, 0600); err != nil {
		t.Fatal(err)
	}

	got := CurrentUpdateStatus()
	if got.Running || got.Stage != "done" || got.Progress != 100 {
		t.Fatalf("normalized status = %+v", got)
	}
	if got.EndedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("normalized timestamps missing: %+v", got)
	}
}

func TestNormalizeStatusAfterRestartLeavesActiveDownloadRunning(t *testing.T) {
	st := UpdateStatus{ID: "download-test", Running: true, Stage: "downloading", Progress: 35, Message: "downloading"}
	got := normalizeStatusAfterRestart(st)
	if !got.Running || got.Stage != st.Stage || got.Progress != st.Progress || got.Message != st.Message {
		t.Fatalf("status should remain active download, got %+v", got)
	}
}

func TestNormalizeStatusAfterRestartLeavesCurrentProcessRestarting(t *testing.T) {
	st := UpdateStatus{ID: "same-process-test", PID: os.Getpid(), Running: true, Stage: "restarting", Progress: 95, Message: "restarting"}
	got := normalizeStatusAfterRestart(st)
	if !got.Running || got.Stage != st.Stage || got.Progress != st.Progress || got.Message != st.Message {
		t.Fatalf("current process restarting status should remain running, got %+v", got)
	}
}
