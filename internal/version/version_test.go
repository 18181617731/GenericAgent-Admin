package version

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

type versionRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn versionRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

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
		{"v1.0.0-8-gabcdef-dirty", "v1.0.1", true},
		{"v1.0.0-8-gabcdef-dirty", "v0.1.3", false},
	}
	for _, c := range cases {
		if got := newer(c.current, c.latest); got != c.want {
			t.Fatalf("newer(%q,%q)=%v want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestFormalVersionRemovesDevelopmentDescribeSuffix(t *testing.T) {
	cases := map[string]string{
		"v1.0.0-8-gd8dc3ac-dirty": "v1.0.0",
		"v1.1.0-rc1-3-gabcdef":    "v1.1.0-rc1",
		"v1.0.0-dirty":            "v1.0.0",
		"v1.0.0":                  "v1.0.0",
	}
	for input, want := range cases {
		if got := formalVersion(input); got != want {
			t.Fatalf("formalVersion(%q)=%q want=%q", input, got, want)
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

func TestCurrentNormalizesInjectedDevelopmentDescribeToFormalVersion(t *testing.T) {
	oldVersion := Version
	defer func() { Version = oldVersion }()
	Version = "v1.0.0-8-gd8dc3ac-dirty"
	if got := Current().Version; got != "v1.0.0" {
		t.Fatalf("Current().Version=%q want v1.0.0", got)
	}
}

func TestDefaultUpdateRepositoryTargetsFork(t *testing.T) {
	oldURL := repoLatestURL
	defer func() { repoLatestURL = oldURL }()
	SetRepoURL("")
	if repoLatestURL != "https://api.github.com/repos/18181617731/GenericAgent-Admin/releases?per_page=50" {
		t.Fatalf("repoLatestURL = %q", repoLatestURL)
	}
	cur := Current()
	if cur.UpdateRepository != "18181617731/GenericAgent-Admin" || cur.UpdateSourceURL != "https://github.com/18181617731/GenericAgent-Admin/releases" {
		t.Fatalf("Current() update source = %#v", cur)
	}
}

func TestSetRepoURLAcceptsGitHubRepositoryURL(t *testing.T) {
	oldURL := repoLatestURL
	defer func() { repoLatestURL = oldURL }()
	SetRepoURL("https://github.com/example/custom-admin.git")
	if repoLatestURL != "https://api.github.com/repos/example/custom-admin/releases?per_page=50" {
		t.Fatalf("repoLatestURL = %q", repoLatestURL)
	}
}

func TestFetchLatestSelectsHighestFormalVersionInsteadOfMostRecentlyPublishedOldTag(t *testing.T) {
	oldURL := repoLatestURL
	defer func() { repoLatestURL = oldURL }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Release{
			{TagName: "v1.0.0", PublishedAt: time.Date(2026, 7, 14, 6, 32, 0, 0, time.UTC)},
			{TagName: "v0.1.3", PublishedAt: time.Date(2026, 7, 14, 10, 2, 0, 0, time.UTC)},
			{TagName: "v1.1.0-rc1", PublishedAt: time.Date(2026, 7, 14, 11, 0, 0, 0, time.UTC), Prerelease: true},
		})
	}))
	defer srv.Close()
	repoLatestURL = srv.URL

	got, err := fetchLatest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.TagName != "v1.0.0" {
		t.Fatalf("fetchLatest tag=%q want v1.0.0", got.TagName)
	}
}

func TestCheckEnablesUpdateForHigherFormalReleaseFromReleaseList(t *testing.T) {
	oldURL, oldVersion := repoLatestURL, Version
	defer func() { repoLatestURL, Version = oldURL, oldVersion }()
	wantAsset := fmt.Sprintf("ga-admin-v1.0.1-%s-%s.zip", runtime.GOOS, runtime.GOARCH)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Release{
			{TagName: "v0.1.3", PublishedAt: time.Now()},
			{TagName: "v1.0.1", PublishedAt: time.Now().Add(-time.Hour), Assets: []Asset{
				{Name: wantAsset, BrowserDownloadURL: srvURLForTest(r, wantAsset)},
				{Name: wantAsset + ".sha256", BrowserDownloadURL: srvURLForTest(r, wantAsset+".sha256")},
			}},
		})
	}))
	defer srv.Close()
	repoLatestURL = srv.URL
	Version = "v1.0.0"

	got, err := Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Update || got.Latest == nil || got.Latest.TagName != "v1.0.1" || got.Asset == nil || got.Asset.Name != wantAsset || got.Checksum == nil {
		t.Fatalf("Check()=%#v, want applicable v1.0.1 update", got)
	}
}

func srvURLForTest(r *http.Request, path string) string {
	return "http://" + r.Host + "/" + path
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
		`C:\Program Files\GA Admin\cmd\frontends\worldline.py`,
		`C:\Temp\cmd\frontends\worldline.py`,
		`C:\Program Files\GA Admin\cmd\frontends\worldline.py.bak`,
		`C:\Temp\restart-update.ps1`,
	)
	want := []string{
		`set "OLD=C:\Program Files\GA Admin\ga-admin.exe"`,
		`set "NEW=C:\Temp\new ga-admin.exe"`,
		`set "BAK=C:\Program Files\GA Admin\ga-admin.exe.bak"`,
		`set "WORKER=C:\Program Files\GA Admin\cmd\chat_worker.py"`,
		`set "NEW_WORKER=C:\Temp\cmd\chat_worker.py"`,
		`set "WORKER_BAK=C:\Program Files\GA Admin\cmd\chat_worker.py.bak"`,
		`set "WORLDLINE=C:\Program Files\GA Admin\cmd\frontends\worldline.py"`,
		`set "NEW_WORLDLINE=C:\Temp\cmd\frontends\worldline.py"`,
		`set "WORLDLINE_BAK=C:\Program Files\GA Admin\cmd\frontends\worldline.py.bak"`,
		`set "RESTART_SCRIPT=C:\Temp\restart-update.ps1"`,
		`move /Y "%OLD%" "%BAK%"`,
		`move /Y "%NEW%" "%OLD%"`,
		`move /Y "%NEW_WORKER%" "%WORKER%"`,
		`move /Y "%NEW_WORLDLINE%" "%WORLDLINE%"`,
		`powershell.exe -NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File "%RESTART_SCRIPT%"`,
		`if errorlevel 1 goto launch_failed`,
		`:launch_failed`,
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
	script := windowsUpdateScript("old.exe", "new.exe", "old.exe.bak", "cmd/chat_worker.py", "tmp/chat_worker.py", "cmd/chat_worker.py.bak", "cmd/frontends/worldline.py", "tmp/cmd/frontends/worldline.py", "cmd/frontends/worldline.py.bak", "restart-update.ps1")
	want := []string{
		`for %%D in ("%WORKER%") do if not exist "%%~dpD" mkdir "%%~dpD"`,
		`if exist "%WORKER%" (`,
		`move /Y "%WORKER%" "%WORKER_BAK%"`,
		`if exist "%WORKER_BAK%" move /Y "%WORKER_BAK%" "%WORKER%"`,
		`move /Y "%OLD%" "%NEW%"`,
		`move /Y "%BAK%" "%OLD%"`,
		`:runtime_files_failed`,
		`if exist "%WORLDLINE_BAK%" move /Y "%WORLDLINE_BAK%" "%WORLDLINE%"`,
	}
	for _, sub := range want {
		if !strings.Contains(script, sub) {
			t.Fatalf("script missing rollback step %q in:\n%s", sub, script)
		}
	}
}

func TestWindowsUpdateScriptRollsBackWhenUpdatedProcessCannotStart(t *testing.T) {
	script := windowsUpdateScript("old.exe", "new.exe", "old.exe.bak", "cmd/chat_worker.py", "tmp/chat_worker.py", "cmd/chat_worker.py.bak", "cmd/frontends/worldline.py", "tmp/cmd/frontends/worldline.py", "cmd/frontends/worldline.py.bak", "restart-update.ps1")
	want := []string{
		`powershell.exe -NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File "%RESTART_SCRIPT%"`,
		`if errorlevel 1 goto launch_failed`,
		`if exist "%WORKER_BAK%" move /Y "%WORKER_BAK%" "%WORKER%"`,
		`if exist "%WORLDLINE_BAK%" move /Y "%WORLDLINE_BAK%" "%WORLDLINE%"`,
		`move /Y "%OLD%" "%NEW%"`,
		`move /Y "%BAK%" "%OLD%"`,
		`powershell.exe -NoProfile -NonInteractive -WindowStyle Hidden -Command "Start-Process -FilePath $env:OLD -WorkingDirectory $env:OLD_DIR -WindowStyle Hidden"`,
		`exit /b 1`,
	}
	for _, sub := range want {
		if !strings.Contains(script, sub) {
			t.Fatalf("script missing launch rollback step %q in:\n%s", sub, script)
		}
	}
}

func TestWindowsRestartScriptWaitsForARealListenerAndRollsBack(t *testing.T) {
	script := windowsRestartScript(
		`C:\Program Files\GA Admin\ga-admin.exe`,
		`C:\Temp\new ga-admin.exe`,
		`C:\Program Files\GA Admin\ga-admin.exe.bak`,
		`C:\Program Files\GA Admin\cmd\chat_worker.py`,
		`C:\Program Files\GA Admin\cmd\chat_worker.py.bak`,
		`C:\Program Files\GA Admin\cmd\frontends\worldline.py`,
		`C:\Program Files\GA Admin\cmd\frontends\worldline.py.bak`,
	)
	want := []string{
		`param([switch]$Run)`,
		`if (-not $Run)`,
		`$PSCommandPath + $quote + ' -Run'`,
		`Invoke-CimMethod -ClassName Win32_Process -MethodName Create`,
		`$Old = 'C:\Program Files\GA Admin\ga-admin.exe'`,
		`$OldDir = 'C:\Program Files\GA Admin'`,
		`$LogFile = Join-Path $PSScriptRoot 'restart-update.log'`,
		`Write-RestartLog "launcher started old=$Old"`,
		`Start-Sleep -Seconds 3`,
		`for ($attempt = 1; $attempt -le 10; $attempt++)`,
		`Get-NetTCPConnection -State Listen -OwningProcess $process.Id`,
		`if (-not $process.HasExited -and $listener) {`,
		`listener=$($listener.LocalPort) verified`,
		`Stop-Process -Id $process.Id -Force`,
		`if (Test-Path -LiteralPath $WorldlineBackup)`,
		`if (Test-Path -LiteralPath $WorkerBackup)`,
		`Move-Item -LiteralPath $Backup -Destination $Old`,
		`Start-Process -FilePath $Old -WorkingDirectory $OldDir`,
	}
	for _, sub := range want {
		if !strings.Contains(script, sub) {
			t.Fatalf("restart script missing %q in:\n%s", sub, script)
		}
	}
}

func TestWindowsDetachedRestartCommandLaunchesQuotedScript(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows process launch contract")
	}
	dir := filepath.Join(t.TempDir(), "restart files")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "restart probe.ps1")
	launcher := filepath.Join(dir, "launch probe.cmd")
	marker := filepath.Join(dir, "restart marker.txt")
	content := fmt.Sprintf("param([switch]$Run)\n%s\n$Marker = %s\nStart-Sleep -Milliseconds 300\nSet-Content -LiteralPath $Marker -Value 'detached' -Encoding UTF8\n", windowsCIMSelfLaunchBlock(), powerShellSingleQuoted(marker))
	if err := os.WriteFile(script, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	launcherContent := "@echo off\r\npowershell.exe -NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File \"%~dp0restart probe.ps1\"\r\n"
	if err := os.WriteFile(launcher, []byte(launcherContent), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("cmd", "/D", "/Q", "/C", launcher)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("detached restart command failed: %v output=%s", err, output)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(marker); err == nil && strings.Contains(string(data), "detached") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("detached restart command did not run quoted script %s", script)
}

func TestWindowsUpdateCommandRunsScriptDirectlyWithoutVisibleStartShell(t *testing.T) {
	cmd := updateScriptCommand("windows", `C:\Temp\ga-admin-update\apply-update.cmd`)
	want := []string{"cmd", "/D", "/Q", "/C", `C:\Temp\ga-admin-update\apply-update.cmd`}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("update command args = %#v, want %#v", cmd.Args, want)
	}
	for _, arg := range cmd.Args {
		if strings.EqualFold(arg, "start") {
			t.Fatalf("update command must not spawn a visible start shell: %#v", cmd.Args)
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

func TestReleaseAssetDigestAvoidsChecksumDownload(t *testing.T) {
	data := []byte("verified release")
	sum := sha256.Sum256(data)
	digest := fmt.Sprintf("%x", sum)
	asset := &Asset{Digest: "sha256:" + digest}
	got, ok := releaseAssetSHA256(asset)
	if !ok || got != digest {
		t.Fatalf("release digest=(%q,%v) want (%q,true)", got, ok, digest)
	}
	path := filepath.Join(t.TempDir(), "release.zip")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifySHA256Value(path, got); err != nil {
		t.Fatalf("verify release digest: %v", err)
	}
	if _, ok := releaseAssetSHA256(&Asset{Digest: "sha256:not-hex"}); ok {
		t.Fatal("malformed release digest was accepted")
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
		`git describe --tags --abbrev^=0 --match^=v[0-9]*`,
		`git rev-parse --short HEAD`,
		`Get-Date`,
		`where npm.cmd`,
		`%ProgramFiles%\nodejs\npm.cmd`,
		`--prefix web ci`,
		`[Security.Cryptography.SHA256]::Create()`,
		`[IO.File]::ReadAllBytes('web\package-lock.json')`,
		`web\node_modules\.ga-admin-package-lock.sha256`,
		`Frontend dependencies are up to date; skipping npm ci.`,
		`where go.exe`,
		`%ProgramFiles%\Go\bin\go.exe`,
		`-X genericagent-admin-go/internal/version.Version=%GA_VERSION%`,
		`-X genericagent-admin-go/internal/version.Commit=%GA_COMMIT%`,
		`-X genericagent-admin-go/internal/version.Date=%GA_DATE%`,
		`"%GO_EXE%" build -ldflags="%GA_LDFLAGS%" -o dist\ga-admin.exe .`,
		`"%GO_EXE%" run .\cmd\package-chat-runtime --worker cmd\chat_worker.py --worldline cmd\frontends\worldline.py --output dist\cmd\chat_worker.py`,
		`copy /Y cmd\frontends\worldline.py dist\cmd\frontends\worldline.py`,
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

func TestRunBatOneClickContract(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	data, err := os.ReadFile(filepath.Join(root, "run.bat"))
	if err != nil {
		t.Fatalf("read run.bat: %v", err)
	}
	script := string(data)
	for _, want := range []string{
		`call "%~dp0build.bat"`,
		`if errorlevel 1`,
		`if not exist "%~dp0dist\ga-admin.exe"`,
		`start "" /D "%~dp0dist" "%~dp0dist\ga-admin.exe"`,
		`pause`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("run.bat missing %q in:\n%s", want, script)
		}
	}
}

func TestReleaseWorkflowSupportsNewManualVersionTags(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release-assets.yml"))
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}
	workflow := string(data)
	want := []string{
		`source_ref="$GITHUB_SHA"`,
		`git ls-remote --exit-code --tags`,
		`uses: actions/checkout@v5`,
		`ref: ${{ needs.prepare.outputs.source_ref }}`,
		`uses: actions/setup-go@v6`,
		`uses: actions/setup-node@v5`,
		`uses: actions/upload-artifact@v5`,
		`uses: actions/download-artifact@v5`,
		`cp cmd/frontends/worldline.py dist/cmd/frontends/worldline.py`,
		`GOOS="$(go env GOHOSTOS)" GOARCH="$(go env GOHOSTARCH)" CGO_ENABLED=0 go run ./cmd/package-chat-runtime`,
		`from frontends.worldline import RewindStore, restore_plan, tree_from_store`,
		`test -f dist/legacy-upgrade/cmd/frontends/worldline.py`,
		`target_commitish: ${{ github.sha }}`,
		`needs: [prepare, build]`,
	}
	for _, item := range want {
		if !strings.Contains(workflow, item) {
			t.Fatalf("release workflow missing %q", item)
		}
	}
	for _, forbidden := range []string{
		`uses: actions/checkout@v4`,
		`ref: ${{ inputs.tag || github.ref_name }}`,
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("release workflow still contains unsupported pattern %q", forbidden)
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

func TestFinishApplyStatusDoesNotReportSuccessBeforeRestart(t *testing.T) {
	st := UpdateStatus{ID: "restart-pending", Running: true, Stage: "preparing", Progress: 85}
	got := finishApplyStatus(st, ApplyResult{
		OK: true, Message: "升级包已就绪，正在重启服务", Script: "apply-update.cmd", Restarting: true,
	})
	if !got.Running || got.Stage != "restarting" || got.Progress != 95 {
		t.Fatalf("restart was reported as terminal success: %+v", got)
	}
	if !got.EndedAt.IsZero() || got.Script != "apply-update.cmd" {
		t.Fatalf("restart-pending status lost metadata: %+v", got)
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

func TestFetchLatestReportsMissingPublishedRelease(t *testing.T) {
	oldURL := repoLatestURL
	defer func() { repoLatestURL = oldURL }()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	repoLatestURL = srv.URL
	_, err := fetchLatest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "尚未发布 GitHub Release") {
		t.Fatalf("fetchLatest error = %v", err)
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
	oldClient := downloadHTTPClient
	oldDelay := downloadRetryDelay
	defer func() { downloadHTTPClient = oldClient; downloadRetryDelay = oldDelay }()

	downloadHTTPClient = &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 25 * time.Millisecond}}
	downloadRetryDelay = 0
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

func TestDownloadRetriesTransientResponses(t *testing.T) {
	oldClient := downloadHTTPClient
	oldDelay := downloadRetryDelay
	defer func() { downloadHTTPClient = oldClient; downloadRetryDelay = oldDelay }()

	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < downloadMaxAttempts {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("release asset"))
	}))
	defer srv.Close()
	downloadHTTPClient = srv.Client()
	downloadRetryDelay = 0
	dest := filepath.Join(t.TempDir(), "asset.zip")

	if err := download(context.Background(), srv.URL, dest, maxUpdatePackageBytes); err != nil {
		t.Fatalf("download after retry: %v", err)
	}
	if attempts != downloadMaxAttempts {
		t.Fatalf("download attempts=%d want %d", attempts, downloadMaxAttempts)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "release asset" {
		t.Fatalf("downloaded data=%q err=%v", data, err)
	}
}

func TestReleaseDownloadCandidatesUseMirrorsOnlyForGitHubAssets(t *testing.T) {
	oldMirrors := updateMirrorPrefixes
	updateMirrorPrefixes = []string{"https://gh-proxy.com/", "https://ghfast.top/"}
	defer func() { updateMirrorPrefixes = oldMirrors }()
	t.Setenv("GA_ADMIN_UPDATE_MIRRORS", "https://custom.example/;not-a-url")
	t.Setenv("GA_ADMIN_UPDATE_DISABLE_MIRRORS", "false")

	raw := "https://github.com/example/admin/releases/download/v1.2.3/ga-admin.zip"
	candidates := releaseDownloadCandidates(raw)
	if len(candidates) != 4 {
		t.Fatalf("candidates=%#v want direct + custom + 2 defaults", candidates)
	}
	if candidates[0].URL != raw || candidates[0].Label != "GitHub 直连" {
		t.Fatalf("first candidate=%#v", candidates[0])
	}
	if candidates[1].URL != "https://custom.example/"+raw || candidates[1].Label != "镜像 custom.example" {
		t.Fatalf("custom candidate=%#v", candidates[1])
	}
	if got := releaseDownloadCandidates("https://downloads.example/release.zip"); len(got) != 1 {
		t.Fatalf("non-GitHub candidates=%#v want direct only", got)
	}
	t.Setenv("GA_ADMIN_UPDATE_DISABLE_MIRRORS", "true")
	if got := releaseDownloadCandidates(raw); len(got) != 1 {
		t.Fatalf("disabled mirror candidates=%#v want direct only", got)
	}
}

func TestDownloadReleaseAssetFallsBackToMirror(t *testing.T) {
	oldClient := downloadHTTPClient
	oldDelay := downloadRetryDelay
	oldMirrors := updateMirrorPrefixes
	defer func() {
		downloadHTTPClient = oldClient
		downloadRetryDelay = oldDelay
		updateMirrorPrefixes = oldMirrors
	}()
	t.Setenv("GA_ADMIN_UPDATE_MIRRORS", "")
	t.Setenv("GA_ADMIN_UPDATE_DISABLE_MIRRORS", "false")
	updateMirrorPrefixes = []string{"https://mirror.test/"}
	downloadRetryDelay = 0
	requests := make([]string, 0, 2)
	downloadHTTPClient = &http.Client{Transport: versionRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.URL.String())
		if req.URL.Hostname() == "github.com" {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Status: "503 Service Unavailable", Header: make(http.Header), Body: io.NopCloser(strings.NewReader("temporary")), Request: req}, nil
		}
		if req.URL.Hostname() == "mirror.test" {
			body := "verified asset"
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), ContentLength: int64(len(body)), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		}
		return nil, fmt.Errorf("unexpected host %s", req.URL.Hostname())
	})}
	dest := filepath.Join(t.TempDir(), "asset.zip")
	raw := "https://github.com/example/admin/releases/download/v1.2.3/asset.zip"

	source, err := downloadReleaseAsset(context.Background(), raw, dest, maxUpdatePackageBytes)
	if err != nil {
		t.Fatalf("downloadReleaseAsset: %v", err)
	}
	if source != "镜像 mirror.test" {
		t.Fatalf("source=%q", source)
	}
	if len(requests) != 2 || requests[0] != raw || requests[1] != "https://mirror.test/"+raw {
		t.Fatalf("requests=%#v", requests)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "verified asset" {
		t.Fatalf("downloaded data=%q err=%v", data, err)
	}
}

func TestDownloadReleaseAssetTimesOutDirectAttemptWithoutCancelingMirror(t *testing.T) {
	oldClient := downloadHTTPClient
	oldDelay := downloadRetryDelay
	oldMirrors := updateMirrorPrefixes
	oldTimeout := releaseDirectTimeout
	defer func() {
		downloadHTTPClient = oldClient
		downloadRetryDelay = oldDelay
		updateMirrorPrefixes = oldMirrors
		releaseDirectTimeout = oldTimeout
	}()
	t.Setenv("GA_ADMIN_UPDATE_MIRRORS", "")
	t.Setenv("GA_ADMIN_UPDATE_DISABLE_MIRRORS", "false")
	updateMirrorPrefixes = []string{"https://mirror.test/"}
	downloadRetryDelay = 0
	releaseDirectTimeout = 15 * time.Millisecond
	downloadHTTPClient = &http.Client{Transport: versionRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() == "github.com" {
			<-req.Context().Done()
			return nil, req.Context().Err()
		}
		body := "mirror after timeout"
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), ContentLength: int64(len(body)), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})}
	dest := filepath.Join(t.TempDir(), "asset.zip")

	source, err := downloadReleaseAsset(context.Background(), "https://github.com/example/admin/releases/download/v1.2.3/asset.zip", dest, maxUpdatePackageBytes)
	if err != nil {
		t.Fatalf("downloadReleaseAsset after direct timeout: %v", err)
	}
	if source != "镜像 mirror.test" {
		t.Fatalf("source=%q", source)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "mirror after timeout" {
		t.Fatalf("downloaded data=%q err=%v", data, err)
	}
}

func TestUpdateMirrorPrefixRequiresHTTPSOrLoopbackHTTP(t *testing.T) {
	for _, prefix := range []string{"https://mirror.example/", "http://127.0.0.1:8080/", "http://localhost:8080/"} {
		if _, ok := validUpdateMirrorPrefix(prefix); !ok {
			t.Fatalf("valid mirror prefix rejected: %s", prefix)
		}
	}
	for _, prefix := range []string{"http://mirror.example/", "ftp://mirror.example/", "https://user:pass@mirror.example/", "not-a-url"} {
		if _, ok := validUpdateMirrorPrefix(prefix); ok {
			t.Fatalf("invalid mirror prefix accepted: %s", prefix)
		}
	}
}

func TestDownloadTransportAllowsSlowReleaseHeaders(t *testing.T) {
	transport, ok := updateHTTPTransport(downloadResponseHeaderTimeout, true).(*http.Transport)
	if !ok {
		t.Fatal("download transport is not *http.Transport")
	}
	if transport.ResponseHeaderTimeout != 90*time.Second {
		t.Fatalf("download response header timeout=%s", transport.ResponseHeaderTimeout)
	}
	if !transport.ForceAttemptHTTP2 {
		t.Fatal("release downloads must retain HTTP/2 for the configured proxy path")
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
	w, err = zw.Create("cmd/frontends/worldline.py")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("new worldline runtime"))
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
	oldVersion := Version
	statusPathOverride = filepath.Join(t.TempDir(), "ga-admin-update-status.json")
	Version = "v9.9.9"
	defer func() { statusPathOverride = oldStatus; Version = oldVersion }()

	started := time.Now().Add(-time.Minute).UTC()
	st := UpdateStatus{ID: "restart-test", PID: os.Getpid() + 1, Running: true, Stage: "restarting", Progress: 95, Message: "升级包已就绪，正在重启服务", Check: &CheckResult{Latest: &Release{TagName: "v9.9.9"}}, StartedAt: started, UpdatedAt: started}
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
	if got.Error != "" || !strings.Contains(got.Message, "v9.9.9") {
		t.Fatalf("normalized success status = %+v", got)
	}
	if got.AppliedVersion != "v9.9.9" {
		t.Fatalf("applied version = %q, want v9.9.9", got.AppliedVersion)
	}
	if got.EndedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("normalized timestamps missing: %+v", got)
	}
	var persisted UpdateStatus
	persistedData, err := os.ReadFile(statusPathOverride)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(persistedData, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Stage != "done" || persisted.AppliedVersion != "v9.9.9" {
		t.Fatalf("verified status was not persisted: %+v", persisted)
	}
}

func TestVerifiedUpdateHistorySurvivesLaterLocalRebuild(t *testing.T) {
	oldStatus := statusPathOverride
	oldVersion := Version
	statusPathOverride = filepath.Join(t.TempDir(), "ga-admin-update-status.json")
	Version = "v9.9.9"
	defer func() { statusPathOverride = oldStatus; Version = oldVersion }()

	st := UpdateStatus{
		ID: "persistent-success", PID: os.Getpid() + 1, Running: true,
		Stage: "restarting", Progress: 95,
		Check: &CheckResult{Latest: &Release{TagName: "v9.9.9"}},
	}
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statusPathOverride, b, 0600); err != nil {
		t.Fatal(err)
	}

	first := CurrentUpdateStatus()
	if first.Stage != "done" || first.AppliedVersion != "v9.9.9" {
		t.Fatalf("first verification = %+v", first)
	}
	Version = "v1.0.2"
	second := CurrentUpdateStatus()
	if second.Stage != "done" || second.AppliedVersion != "v9.9.9" || second.Error != "" {
		t.Fatalf("later local rebuild rewrote update history: %+v", second)
	}
}

func TestCurrentUpdateStatusRejectsFalseSuccessWhenVersionDidNotChange(t *testing.T) {
	oldStatus := statusPathOverride
	oldVersion := Version
	statusPathOverride = filepath.Join(t.TempDir(), "ga-admin-update-status.json")
	Version = "v1.0.2"
	defer func() { statusPathOverride = oldStatus; Version = oldVersion }()

	started := time.Now().Add(-time.Minute).UTC()
	st := UpdateStatus{
		ID: "restart-failed-test", PID: os.Getpid() + 1, Running: true,
		Stage: "restarting", Progress: 95, Message: "升级包已就绪，正在重启服务",
		Check: &CheckResult{Latest: &Release{TagName: "v1.0.3"}}, StartedAt: started, UpdatedAt: started,
	}
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statusPathOverride, b, 0600); err != nil {
		t.Fatal(err)
	}

	got := CurrentUpdateStatus()
	if got.Running || got.Stage != "error" || got.Progress != 100 {
		t.Fatalf("false success was not rejected: %+v", got)
	}
	if !strings.Contains(got.Error, "当前版本 v1.0.2") || !strings.Contains(got.Error, "目标版本 v1.0.3") {
		t.Fatalf("false success error is not actionable: %+v", got)
	}
}

func TestCurrentUpdateStatusRechecksLegacyDoneStatus(t *testing.T) {
	oldStatus := statusPathOverride
	oldVersion := Version
	statusPathOverride = filepath.Join(t.TempDir(), "ga-admin-update-status.json")
	Version = "v1.0.2"
	defer func() { statusPathOverride = oldStatus; Version = oldVersion }()

	st := UpdateStatus{
		ID: "legacy-false-success", Running: false, Stage: "done", Progress: 100,
		Message: "update downloaded; restarting",
		Check:   &CheckResult{Latest: &Release{TagName: "v1.0.4"}},
	}
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statusPathOverride, b, 0600); err != nil {
		t.Fatal(err)
	}

	got := CurrentUpdateStatus()
	if got.Stage != "error" || !strings.Contains(got.Error, "当前版本 v1.0.2") || !strings.Contains(got.Error, "目标版本 v1.0.4") {
		t.Fatalf("legacy false success was not corrected: %+v", got)
	}
}

func TestNormalizeStatusAfterRestartLeavesActiveDownloadRunning(t *testing.T) {
	st := UpdateStatus{ID: "download-test", Running: true, Stage: "downloading", Progress: 35, Message: "downloading"}
	got := normalizeStatusAfterRestart(st)
	if !got.Running || got.Stage != st.Stage || got.Progress != st.Progress || got.Message != st.Message {
		t.Fatalf("status should remain active download, got %+v", got)
	}
}

func TestNormalizeStatusAfterRestartMarksOldDownloadAsInterrupted(t *testing.T) {
	started := time.Now().Add(-time.Minute).UTC()
	st := UpdateStatus{ID: "interrupted-download", PID: os.Getpid() + 1, Running: true, Stage: "downloading", Progress: 35, Message: "downloading", StartedAt: started, UpdatedAt: started}
	got := normalizeStatusAfterRestart(st)
	if got.Running || got.Stage != "error" {
		t.Fatalf("interrupted status = %+v", got)
	}
	if got.Progress != 35 {
		t.Fatalf("progress = %d want 35", got.Progress)
	}
	if !strings.Contains(got.Error, "downloading") || !strings.Contains(got.Error, "PID") || !strings.Contains(got.Message, "重新开始升级") {
		t.Fatalf("interrupted detail is not actionable: %+v", got)
	}
	if got.EndedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("interrupted timestamps missing: %+v", got)
	}
}

func TestNormalizeStatusAfterRestartLeavesCurrentProcessRestarting(t *testing.T) {
	st := UpdateStatus{ID: "same-process-test", PID: os.Getpid(), Running: true, Stage: "restarting", Progress: 95, Message: "restarting"}
	got := normalizeStatusAfterRestart(st)
	if !got.Running || got.Stage != st.Stage || got.Progress != st.Progress || got.Message != st.Message {
		t.Fatalf("current process restarting status should remain running, got %+v", got)
	}
}
