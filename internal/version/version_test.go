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
	"slices"
	"strings"
	"sync"
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
		`$Old = 'C:\Program Files\GA Admin\ga-admin.exe'`,
		`$New = 'C:\Temp\new ga-admin.exe'`,
		`$Backup = 'C:\Program Files\GA Admin\ga-admin.exe.bak'`,
		`$Worker = 'C:\Program Files\GA Admin\cmd\chat_worker.py'`,
		`$NewWorker = 'C:\Temp\cmd\chat_worker.py'`,
		`$WorkerBackup = 'C:\Program Files\GA Admin\cmd\chat_worker.py.bak'`,
		`$Worldline = 'C:\Program Files\GA Admin\cmd\frontends\worldline.py'`,
		`$NewWorldline = 'C:\Temp\cmd\frontends\worldline.py'`,
		`$WorldlineBackup = 'C:\Program Files\GA Admin\cmd\frontends\worldline.py.bak'`,
		`$RestartScript = 'C:\Temp\restart-update.ps1'`,
		`$LogFile = Join-Path $PSScriptRoot 'apply-update.log'`,
		`Move-Item -LiteralPath $Old -Destination $Backup`,
		`Move-Item -LiteralPath $New -Destination $Old`,
		`Move-Item -LiteralPath $NewWorker -Destination $Worker`,
		`Move-Item -LiteralPath $NewWorldline -Destination $Worldline`,
		`& $RestartScript`,
		`$RestartExit = $LASTEXITCODE`,
		`exit $RestartExit`,
	}
	for _, w := range want {
		if !strings.Contains(script, w) {
			t.Fatalf("script missing %q in:\n%s", w, script)
		}
	}
	bad := []string{`cmd.exe /D /Q /C`, `schtasks.exe`, `""C:\`, `%~dpWORKER%`}
	for _, b := range bad {
		if strings.Contains(script, b) {
			t.Fatalf("script contains unsafe quoting %q in:\n%s", b, script)
		}
	}
}

func TestWindowsUpdateScriptReplacesRuntimeAndInvokesRestart(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows update script contract")
	}
	root := filepath.Join(t.TempDir(), "update files")
	installed := filepath.Join(root, "installed")
	staged := filepath.Join(root, "staged")
	if err := os.MkdirAll(filepath.Join(installed, "cmd", "frontends"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(staged, "cmd", "frontends"), 0755); err != nil {
		t.Fatal(err)
	}
	oldExe := filepath.Join(installed, "ga-admin.exe")
	newExe := filepath.Join(staged, "ga-admin.exe")
	worker := filepath.Join(installed, "cmd", "chat_worker.py")
	newWorker := filepath.Join(staged, "cmd", "chat_worker.py")
	worldline := filepath.Join(installed, "cmd", "frontends", "worldline.py")
	newWorldline := filepath.Join(staged, "cmd", "frontends", "worldline.py")
	for path, content := range map[string]string{
		oldExe:       "old-exe",
		newExe:       "new-exe",
		worker:       "old-worker",
		newWorker:    "new-worker",
		worldline:    "old-worldline",
		newWorldline: "new-worldline",
	} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	restartScript := filepath.Join(root, "restart update.ps1")
	restartLog := filepath.Join(root, "restart-update.log")
	restartContent := fmt.Sprintf("Set-Content -LiteralPath %s -Value 'started' -Encoding UTF8\nexit 0\n", powerShellSingleQuoted(restartLog))
	if err := os.WriteFile(restartScript, []byte(restartContent), 0600); err != nil {
		t.Fatal(err)
	}
	applyScript := filepath.Join(root, "apply update.ps1")
	content := windowsUpdateScript(oldExe, newExe, oldExe+".bak", worker, newWorker, worker+".bak", worldline, newWorldline, worldline+".bak", restartScript)
	if err := os.WriteFile(applyScript, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-File", applyScript)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("apply script failed: %v output=%s\nscript:\n%s", err, output, content)
	}
	for path, want := range map[string]string{
		oldExe:    "new-exe",
		worker:    "new-worker",
		worldline: "new-worldline",
	} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("updated file %s = %q, want %q (err=%v)", path, got, want, err)
		}
	}
	applyLog, err := os.ReadFile(filepath.Join(root, "apply-update.log"))
	if err != nil || !strings.Contains(string(applyLog), "runtime files ready") {
		t.Fatalf("apply log missing success marker: %q err=%v", applyLog, err)
	}
	if _, err := os.Stat(restartLog); err != nil {
		t.Fatalf("restart script was not invoked: %v", err)
	}
}

func TestWindowsUpdateScriptRestoresExeWhenWorkerMoveFails(t *testing.T) {
	script := windowsUpdateScript("old.exe", "new.exe", "old.exe.bak", "cmd/chat_worker.py", "tmp/chat_worker.py", "cmd/chat_worker.py.bak", "cmd/frontends/worldline.py", "tmp/cmd/frontends/worldline.py", "cmd/frontends/worldline.py.bak", "restart-update.ps1")
	want := []string{
		`$WorkerHadOriginal = Test-Path -LiteralPath $Worker`,
		`Move-Item -LiteralPath $Worker -Destination $WorkerBackup`,
		`Restore-File $Worker $WorkerBackup $WorkerHadOriginal`,
		`Restore-File $Worldline $WorldlineBackup $WorldlineHadOriginal`,
		`Move-Item -LiteralPath $Old -Destination $New`,
		`Move-Item -LiteralPath $Backup -Destination $Old`,
		`Restore-OldVersion`,
	}
	for _, sub := range want {
		if !strings.Contains(script, sub) {
			t.Fatalf("script missing rollback step %q in:\n%s", sub, script)
		}
	}
}

func TestWindowsUpdateScriptRollsBackWhenUpdatedProcessCannotStart(t *testing.T) {
	script := windowsUpdateScript("old.exe", "new.exe", "old.exe.bak", "cmd/chat_worker.py", "tmp/cmd/chat_worker.py", "cmd/chat_worker.py.bak", "cmd/frontends/worldline.py", "tmp/cmd/frontends/worldline.py", "cmd/frontends/worldline.py.bak", "restart-update.ps1")
	want := []string{
		`& $RestartScript`,
		`$RestartExit = $LASTEXITCODE`,
		`Write-ApplyLog "restart script exit=$RestartExit"`,
		`Restore-OldVersion`,
		`$LaunchArgs = @('--headless','--no-browser','--port','8787','--app-root','.')`,
		`exit 1`,
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
		`$Old = 'C:\Program Files\GA Admin\ga-admin.exe'`,
		`$OldDir = 'C:\Program Files\GA Admin'`,
		`$LogFile = Join-Path $PSScriptRoot 'restart-update.log'`,
		`Write-RestartLog "launcher started target=$ExpectedVersion expected_port=$ExpectedPort old=$Old"`,
		`Start-Sleep -Seconds 3`,
		`for ($attempt = 1; $attempt -le 10; $attempt++)`,
		`for ($probe = 1; $probe -le 30; $probe++)`,
		`$LaunchArgs = @('--headless','--no-browser','--port','8787','--app-root','.')`,
		`Get-NetTCPConnection -State Listen -OwningProcess $process.Id`,
		`Where-Object { $_.LocalPort -eq $ExpectedPort }`,
		`Invoke-RestMethod -Uri $HealthURL`,
		`if ($process.HasExited) { break }`,
		`version=$($info.version) verified`,
		`Stop-Process -Id $process.Id -Force`,
		`if (Test-Path -LiteralPath $WorldlineBackup)`,
		`if (Test-Path -LiteralPath $WorkerBackup)`,
		`Move-Item -LiteralPath $Backup -Destination $Old`,
		`Start-Process -FilePath $Old -ArgumentList $LaunchArgs -WorkingDirectory $OldDir`,
	}
	for _, sub := range want {
		if !strings.Contains(script, sub) {
			t.Fatalf("restart script missing %q in:\n%s", sub, script)
		}
	}
	if strings.Contains(script, "schtasks.exe") || strings.Contains(script, "Remove-RestartTask") {
		t.Fatalf("restart script must not depend on Task Scheduler:\n%s", script)
	}
}

func TestWindowsDirectRestartScriptRunsQuotedPathAndReturnsExitCode(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows process launch contract")
	}
	dir := filepath.Join(t.TempDir(), "restart files")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "restart probe.ps1")
	marker := filepath.Join(dir, "restart marker.txt")
	content := fmt.Sprintf("$Marker = %s\nStart-Sleep -Seconds 2\nSet-Content -LiteralPath $Marker -Value 'detached' -Encoding UTF8\n", powerShellSingleQuoted(marker))
	if err := os.WriteFile(script, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	run := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-File", script)
	if output, err := run.CombinedOutput(); err != nil {
		t.Fatalf("direct restart script failed: %v output=%s", err, output)
	}
	data, err := os.ReadFile(marker)
	if err != nil || !strings.Contains(string(data), "detached") {
		t.Fatalf("direct restart command did not run quoted script %s: data=%q err=%v", script, data, err)
	}
}

func TestWindowsUpdateCommandUsesBackgroundCmdLauncher(t *testing.T) {
	cmd := updateScriptCommand("windows", `C:\Temp\ga-admin-update\apply-update.ps1`)
	want := []string{"cmd.exe", "/D", "/Q", "/C", "start", "", "/B", "powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-File", `C:\Temp\ga-admin-update\apply-update.ps1`}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("update command args = %#v, want %#v", cmd.Args, want)
	}
}

func TestWindowsDetachedUpdateScriptSurvivesLauncherExit(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows detached process contract")
	}
	dir := filepath.Join(t.TempDir(), "detached update")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "detached marker.txt")
	script := filepath.Join(dir, "delayed update.ps1")
	content := fmt.Sprintf("Start-Sleep -Seconds 2\nSet-Content -LiteralPath %s -Value 'detached' -Encoding UTF8\n", powerShellSingleQuoted(marker))
	if err := os.WriteFile(script, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	launcher := updateScriptCommand("windows", script)
	hideChildWindow(launcher)
	if err := launcher.Start(); err != nil {
		t.Fatalf("detached update launcher failed: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(marker); err == nil && strings.Contains(string(data), "detached") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("detached update script stopped with its launcher: %s", script)
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
		`goos: windows`,
		`goarch: amd64`,
		`goarch: arm64`,
		`-eq 2`,
		`cp cmd/frontends/worldline.py dist/cmd/frontends/worldline.py`,
		`GOOS="$(go env GOHOSTOS)" GOARCH="$(go env GOHOSTARCH)" CGO_ENABLED=0 go run ./cmd/package-chat-runtime`,
		`from frontends.worldline import RewindStore, restore_plan, tree_from_store`,
		`test -f dist/legacy-upgrade/cmd/frontends/worldline.py`,
		`target_commitish: ${{ github.sha }}`,
		`needs: [prepare, build]`,
		`$hostArch = if ($env:PROCESSOR_ARCHITECTURE -match 'ARM64')`,
		`Skipping execution metadata probe for cross-compiled`,
		`if: matrix.goos == 'windows' && matrix.goarch == 'amd64'`,
	}
	for _, item := range want {
		if !strings.Contains(workflow, item) {
			t.Fatalf("release workflow missing %q", item)
		}
	}
	if got := strings.Count(workflow, `goos: windows`); got != 2 {
		t.Fatalf("release workflow Windows matrix entries = %d, want 2", got)
	}
	for _, forbidden := range []string{
		`uses: actions/checkout@v4`,
		`ref: ${{ inputs.tag || github.ref_name }}`,
		`goos: darwin`,
		`goos: linux`,
		`-eq 6`,
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

func TestUpdatePayloadUsesReleaseTopLevelDirectory(t *testing.T) {
	assetName := "ga-admin-v9.9.9-windows-amd64.zip"
	zipPath := filepath.Join(t.TempDir(), assetName)
	makeUpdateZip(t, zipPath)
	dest := filepath.Join(t.TempDir(), "unzipped")
	if err := unzip(zipPath, dest); err != nil {
		t.Fatalf("unzip update package: %v", err)
	}

	gotExe, gotWorker, err := updatePayload(dest, assetName, "ga-admin.exe")
	if err != nil {
		t.Fatalf("updatePayload: %v", err)
	}
	root := filepath.Join(dest, strings.TrimSuffix(assetName, ".zip"))
	if want := filepath.Join(root, "ga-admin.exe"); gotExe != want {
		t.Fatalf("executable = %q, want %q", gotExe, want)
	}
	if want := filepath.Join(root, "cmd", "chat_worker.py"); gotWorker != want {
		t.Fatalf("worker = %q, want %q", gotWorker, want)
	}
}

func TestUpdatePayloadRejectsUnexpectedTopLevelLayout(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
	}{
		{name: "flat package", paths: []string{"ga-admin.exe", "cmd/chat_worker.py"}},
		{name: "extra top-level entry", paths: []string{"ga-admin-v9.9.9-windows-amd64/ga-admin.exe", "ga-admin-v9.9.9-windows-amd64/cmd/chat_worker.py", "README.txt"}},
		{name: "wrong root name", paths: []string{"other/ga-admin.exe", "other/cmd/chat_worker.py"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range tt.paths {
				path := filepath.Join(dir, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("payload"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			_, _, err := updatePayload(dir, "ga-admin-v9.9.9-windows-amd64.zip", "ga-admin.exe")
			if err == nil || !strings.Contains(err.Error(), "exactly one top-level directory") {
				t.Fatalf("updatePayload error = %v, want top-level directory error", err)
			}
		})
	}
}

func TestValidateReleaseManifestRejectsMismatchedAsset(t *testing.T) {
	check := CheckResult{Latest: &Release{TagName: "v1.0.76"}, Asset: &Asset{Name: "ga-admin-v1.0.76-windows-amd64.zip"}}
	manifest := ReleaseManifest{Version: "v1.0.76", Commit: "abc1234", GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Asset: "ga-admin-v1.0.75-windows-amd64.zip"}
	if err := validateReleaseManifest(manifest, check); err == nil || !strings.Contains(err.Error(), "资产校验失败") {
		t.Fatalf("mismatched release asset was accepted: %v", err)
	}
}

func TestValidateCandidateBuildRejectsWrongVersionAndPlatform(t *testing.T) {
	check := CheckResult{Latest: &Release{TagName: "v1.0.76"}, Asset: &Asset{Name: "ga-admin-v1.0.76-windows-amd64.zip"}}
	manifest := ReleaseManifest{Version: "v1.0.76", Commit: "abc1234", GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Asset: check.Asset.Name}
	candidate := BuildInfo{Version: "v1.0.75", Commit: "abc1234", GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	if err := validateCandidateBuild(candidate, manifest, check); err == nil || !strings.Contains(err.Error(), "版本校验失败") {
		t.Fatalf("wrong candidate version was accepted: %v", err)
	}
	candidate.Version = "v1.0.76"
	candidate.GOARCH = "arm64"
	if err := validateCandidateBuild(candidate, manifest, check); err == nil || !strings.Contains(err.Error(), "平台校验失败") {
		t.Fatalf("wrong candidate platform was accepted: %v", err)
	}
}

func TestWindowsRestartScriptRequiresExpectedPortAndVersionHealth(t *testing.T) {
	script := windowsRestartScript("old.exe", "new.exe", "old.exe.bak", "worker", "worker.bak", "worldline", "worldline.bak", launchContext{AppRoot: `C:\GA`, Port: 18787}, BuildInfo{Version: "v1.0.76"})
	for _, want := range []string{
		`$ExpectedVersion = 'v1.0.76'`,
		`$ExpectedPort = 18787`,
		`$HealthURL = 'http://127.0.0.1:18787/api/version/info'`,
		`Where-Object { $_.LocalPort -eq $ExpectedPort }`,
		`if ([string]$info.version -eq [string]$ExpectedVersion)`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("restart health contract missing %q in:\n%s", want, script)
		}
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
	if st.Running || st.Stage != "failed" || st.Progress != 100 || st.Error == "" {
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
	if final.Running || final.Stage != "failed" {
		t.Fatalf("final status = %+v", final)
	}
	if !strings.Contains(final.Error, "sha256 mismatch") || final.Script != "" {
		t.Fatalf("unexpected error/script: %+v", final)
	}
	if final.Progress != 100 || final.EndedAt.IsZero() || final.Check == nil {
		t.Fatalf("incomplete final status: %+v", final)
	}
	fromAPI := CurrentUpdateStatus()
	if fromAPI.Stage != "failed" || !strings.Contains(fromAPI.Message, "sha256 mismatch") {
		t.Fatalf("readable persisted status = %+v", fromAPI)
	}
}

func TestStartApplyLatestWaitsForRestartAuthorizationBeforeLaunchingHelper(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("self-update is currently supported on Windows only")
	}

	oldURL := repoLatestURL
	oldStatus := statusPathOverride
	oldRuntime := currentApplyRuntime
	defer func() {
		repoLatestURL = oldURL
		statusPathOverride = oldStatus
		currentApplyRuntime = oldRuntime
	}()

	installRoot := t.TempDir()
	statusPathOverride = filepath.Join(installRoot, "ga-admin-update-status.json")
	runningExe := filepath.Join(installRoot, "ga-admin.exe")
	if err := os.WriteFile(runningExe, []byte("copied-helper-binary"), 0755); err != nil {
		t.Fatal(err)
	}

	assetName := fmt.Sprintf("ga-admin-v999.0.0-%s-%s.zip", runtime.GOOS, runtime.GOARCH)
	zipPath := filepath.Join(t.TempDir(), assetName)
	makeUpdateZip(t, zipPath)
	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(zipData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			_ = json.NewEncoder(w).Encode(Release{TagName: "v999.0.0", Assets: []Asset{
				{Name: assetName, BrowserDownloadURL: serverURL(r, "/asset.zip")},
				{Name: assetName + ".sha256", BrowserDownloadURL: serverURL(r, "/asset.zip.sha256")},
			}})
		case "/asset.zip":
			http.ServeFile(w, r, zipPath)
		case "/asset.zip.sha256":
			_, _ = fmt.Fprintf(w, "%x  %s\n", sum, assetName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	repoLatestURL = server.URL + "/latest"

	type launchRecord struct {
		helperPath   string
		manifestPath string
		manifest     UpdateManifest
	}
	launched := make(chan launchRecord, 1)
	exitScheduled := make(chan struct{}, 1)
	currentApplyRuntime = applyRuntimeDeps{
		executable: func() (string, error) { return runningExe, nil },
		launchHelper: func(helperPath, manifestPath string) error {
			manifest, err := readUpdateManifest(manifestPath)
			if err != nil {
				return err
			}
			launched <- launchRecord{helperPath: helperPath, manifestPath: manifestPath, manifest: manifest}
			return transitionUpdate(manifest.OperationID, func(st *UpdateStatus) error {
				st.Stage = "waiting_for_exit"
				st.Progress = 88
				st.Message = "helper owns update status"
				return nil
			})
		},
		scheduleExit: func() { exitScheduled <- struct{}{} },
	}

	initial, err := StartApplyLatest()
	if err != nil {
		t.Fatalf("StartApplyLatest: %v", err)
	}
	var ready UpdateStatus
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		ready = CurrentUpdateStatus()
		if ready.Stage == "ready" || ready.Stage == "failed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if ready.Stage != "ready" || ready.Progress != 90 || !ready.Running {
		t.Fatalf("download did not stop at ready: %+v", ready)
	}
	select {
	case record := <-launched:
		t.Fatalf("helper launched before restart authorization: %+v", record)
	default:
	}
	select {
	case <-exitScheduled:
		t.Fatal("parent exit scheduled before restart authorization")
	default:
	}
	if err := transitionUpdate(initial.ID, func(st *UpdateStatus) error {
		st.Error = "previous helper launch failed"
		return nil
	}); err != nil {
		t.Fatalf("seed retry error: %v", err)
	}

	authorized, err := AuthorizeRestart(initial.ID)
	if err != nil {
		t.Fatalf("AuthorizeRestart: %v", err)
	}
	if authorized.ID != initial.ID {
		t.Fatalf("authorized operation = %+v, initial=%+v", authorized, initial)
	}
	if authorized.Error != "" {
		t.Fatalf("successful retry retained stale error: %+v", authorized)
	}

	var record launchRecord
	select {
	case record = <-launched:
	case <-time.After(time.Second):
		t.Fatalf("copied helper was not launched after authorization; status=%+v", CurrentUpdateStatus())
	}
	select {
	case <-exitScheduled:
	case <-time.After(time.Second):
		t.Fatal("parent exit was not scheduled after authorization")
	}

	if record.manifest.OperationID != initial.ID || record.manifest.StatusPath != statusPathOverride {
		t.Fatalf("manifest identity/status = %+v, initial=%+v", record.manifest, initial)
	}
	if record.manifest.OriginalExe != runningExe || record.manifest.OldPID != os.Getpid() {
		t.Fatalf("manifest process identity = %+v", record.manifest)
	}
	helperDir := filepath.Dir(record.helperPath)
	if helperDir != filepath.Dir(record.manifestPath) {
		t.Fatalf("helper/manifest are not durable siblings: helper=%q manifest=%q", record.helperPath, record.manifestPath)
	}
	originalWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if record.manifest.WorkingDir != originalWorkingDir {
		t.Fatalf("restart working directory = %q, want %q", record.manifest.WorkingDir, originalWorkingDir)
	}
	helperData, err := os.ReadFile(record.helperPath)
	if err != nil {
		t.Fatalf("read copied helper: %v", err)
	}
	if string(helperData) != "copied-helper-binary" {
		t.Fatalf("copied helper bytes = %q", helperData)
	}
	if _, err := os.Stat(record.manifest.StagedExe); err != nil {
		t.Fatalf("staged executable missing: %v", err)
	}
	if _, err := os.Stat(record.manifest.StagedWorker); err != nil {
		t.Fatalf("staged worker missing: %v", err)
	}
	for _, arg := range record.manifest.OriginalArgs {
		if strings.HasPrefix(arg, "--update-helper") || strings.HasPrefix(arg, "--update-confirm") {
			t.Fatalf("internal update argument leaked into restart args: %#v", record.manifest.OriginalArgs)
		}
	}

	time.Sleep(150 * time.Millisecond)
	final := CurrentUpdateStatus()
	if final.Stage != "waiting_for_exit" || final.Progress != 88 || !final.Running {
		t.Fatalf("parent overwrote helper-owned status: %+v", final)
	}
	if final.Script != "" {
		t.Fatalf("legacy update script leaked into helper transaction: %+v", final)
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
	rootName := strings.TrimSuffix(filepath.Base(path), ".zip")
	if rootName == "" {
		t.Fatalf("invalid update zip path %q", path)
	}
	w, err := zw.Create(rootName + "/ga-admin.exe")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("new exe"))
	w, err = zw.Create(rootName + "/cmd/chat_worker.py")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("new worker"))
	w, err = zw.Create(rootName + "/cmd/frontends/worldline.py")
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

func TestCurrentUpdateStatusDoesNotInferSuccessAfterRelaunch(t *testing.T) {
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
	if !got.Running || got.Stage != "restarting" || got.Progress != 95 {
		t.Fatalf("status must await helper confirmation, got %+v", got)
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

func TestNormalizeStatusForVersionClearsStaleActiveStatusAfterManualInstall(t *testing.T) {
	st := UpdateStatus{
		ID:            "manual-install-test",
		Running:       true,
		Stage:         "downloading",
		Progress:      42,
		Message:       "downloading",
		SourceVersion: "v0.2.4",
		TargetVersion: "v0.2.5",
		Check: &CheckResult{
			Current: BuildInfo{Version: "v0.2.4"},
			Latest:  &Release{TagName: "v0.2.5"},
			Update:  true,
		},
	}
	current := BuildInfo{Version: "v0.2.5"}

	got := normalizeStatusForVersion(st, current.Version, current)
	if got.Running || got.Stage != "done" || got.Progress != 100 {
		t.Fatalf("stale active status should be completed, got %+v", got)
	}
	if got.InstalledVersion != current.Version || got.ConfirmedVersion != current.Version {
		t.Fatalf("installed versions = %q/%q, want %q", got.InstalledVersion, got.ConfirmedVersion, current.Version)
	}
	if got.Check == nil || got.Check.Current.Version != current.Version || got.Check.Update {
		t.Fatalf("check snapshot was not refreshed, got %+v", got.Check)
	}
	if got.ConfirmedAt.IsZero() || got.EndedAt.IsZero() {
		t.Fatalf("completed status should have confirmation/end timestamps: %+v", got)
	}
}

func TestNormalizeStatusForVersionLeavesReadyTransactionPendingAuthorization(t *testing.T) {
	st := UpdateStatus{
		ID:            "ready-test",
		Running:       true,
		Stage:         "ready",
		Progress:      90,
		Message:       "ready",
		TargetVersion: "v0.2.5",
	}

	got := normalizeStatusForVersion(st, "v0.2.5", BuildInfo{Version: "v0.2.5"})
	if !got.Running || got.Stage != st.Stage || got.Progress != st.Progress || got.Message != st.Message {
		t.Fatalf("ready update must remain pending restart authorization, got %+v", got)
	}
}

func TestNormalizeStatusForVersionLeavesUnreachedActiveStatus(t *testing.T) {
	st := UpdateStatus{
		ID:            "unreached-test",
		Running:       true,
		Stage:         "downloading",
		Progress:      42,
		Message:       "downloading",
		TargetVersion: "v0.2.5",
	}

	got := normalizeStatusForVersion(st, "v0.2.4", BuildInfo{Version: "v0.2.4"})
	if !got.Running || got.Stage != st.Stage || got.Progress != st.Progress || got.Message != st.Message {
		t.Fatalf("unreached update should remain active, got %+v", got)
	}
}

// TestCheckRealNetwork verifies the timeout fix with real GitHub API.
// Skip in CI to avoid flakiness; run manually with: go test -v -run TestCheckRealNetwork
func TestCheckRealNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real network test in short mode")
	}

	// Configure proxy if available
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:7897")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:7897")

	SetRepoURL("https://api.github.com/repos/Fwind43/GenericAgent-Admin/releases/latest")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	t.Log("Fetching real GitHub release metadata with proxy...")
	start := time.Now()
	result, err := Check(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Check failed after %.1fs: %v", elapsed.Seconds(), err)
	}

	t.Logf("✓ Check succeeded in %.1fs", elapsed.Seconds())
	t.Logf("  Current: %s", result.Current.Version)
	if result.Latest != nil {
		t.Logf("  Latest: %s", result.Latest.TagName)
	}
	t.Logf("  Update: %v", result.Update)

	if result.Latest == nil {
		t.Fatal("Latest release should not be nil")
	}
	if result.Latest.TagName == "" {
		t.Fatal("Latest TagName should not be empty")
	}
	if !strings.HasPrefix(result.Latest.TagName, "v") && !strings.HasPrefix(result.Latest.TagName, "0.") {
		t.Fatalf("Latest version has unexpected format: %s", result.Latest.TagName)
	}
}

func TestUpdateTransactionWaitTimeoutLeavesInstallationUntouched(t *testing.T) {
	root := t.TempDir()
	oldExe := filepath.Join(root, "ga-admin.exe")
	oldWorker := filepath.Join(root, "cmd", "chat_worker.py")
	stagedExe := filepath.Join(root, "stage", "ga-admin.exe")
	stagedWorker := filepath.Join(root, "stage", "cmd", "chat_worker.py")
	for path, content := range map[string]string{
		oldExe: "old-exe", oldWorker: "old-worker", stagedExe: "new-exe", stagedWorker: "new-worker",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	oldStatus := statusPathOverride
	statusPathOverride = filepath.Join(root, "ga-admin-update-status.json")
	defer func() { statusPathOverride = oldStatus }()
	if err := writeStatus(UpdateStatus{ID: "timeout-op", Running: true, Stage: "prepared"}); err != nil {
		t.Fatal(err)
	}

	manifest := UpdateManifest{
		OperationID: "timeout-op", OldPID: os.Getpid(), SourceVersion: "v1.0.0", TargetVersion: "v2.0.0",
		OriginalExe: oldExe, StagedExe: stagedExe, BackupExe: oldExe + ".timeout-op.bak",
		Worker: oldWorker, StagedWorker: stagedWorker, WorkerBackup: oldWorker + ".timeout-op.bak",
		StatusPath: statusPathOverride, WorkingDir: root, ExitTimeout: 25 * time.Millisecond,
	}
	if err := runReplacementTransaction(manifest, defaultTransactionDeps()); err == nil || !strings.Contains(err.Error(), "old process") {
		t.Fatalf("transaction error = %v, want old process timeout", err)
	}
	for path, want := range map[string]string{oldExe: "old-exe", oldWorker: "old-worker"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v; want %q", path, got, err, want)
		}
	}
}

func TestBuildUpdateManifestUsesOperationScopedBackups(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "ga-admin.exe")
	worker := filepath.Join(root, "cmd", "chat_worker.py")
	legacyExeBackup := exe + ".bak"
	legacyWorkerBackup := worker + ".bak"
	for _, path := range []string{legacyExeBackup, legacyWorkerBackup} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("legacy-backup"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	manifest, err := buildUpdateManifest(updateManifestInput{
		OperationID: "update-123-456", SourceVersion: "v1.0.0", TargetVersion: "v2.0.0",
		OriginalExe: exe, StagedExe: filepath.Join(root, "stage", "ga-admin.exe"),
		Worker: worker, StagedWorker: filepath.Join(root, "stage", "chat_worker.py"),
		StatusPath: filepath.Join(root, "ga-admin-update-status.json"), WorkingDir: root,
	})
	if err != nil {
		t.Fatalf("buildUpdateManifest: %v", err)
	}
	if manifest.BackupExe == legacyExeBackup || manifest.WorkerBackup == legacyWorkerBackup {
		t.Fatalf("manifest reuses legacy fixed backup paths: %+v", manifest)
	}
	if !strings.Contains(filepath.Base(manifest.BackupExe), manifest.OperationID) ||
		!strings.Contains(filepath.Base(manifest.WorkerBackup), manifest.OperationID) {
		t.Fatalf("backup paths are not operation-scoped: exe=%q worker=%q", manifest.BackupExe, manifest.WorkerBackup)
	}
}

func TestUpdateTransactionRestartsOriginalAfterPreLaunchFailure(t *testing.T) {
	root := t.TempDir()
	oldExe := filepath.Join(root, "ga-admin.exe")
	stagedExe := filepath.Join(root, "stage", "ga-admin.exe")
	for path, content := range map[string]string{oldExe: "old-exe", stagedExe: "new-exe"} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	oldStatus := statusPathOverride
	statusPathOverride = filepath.Join(root, "ga-admin-update-status.json")
	defer func() { statusPathOverride = oldStatus }()
	operationID := "pre-launch-failure-op"
	if err := writeStatus(UpdateStatus{ID: operationID, Running: true, Stage: "prepared"}); err != nil {
		t.Fatal(err)
	}
	manifest := UpdateManifest{
		OperationID: operationID, SourceVersion: "v1.0.0", TargetVersion: "v2.0.0",
		OriginalExe: oldExe, StagedExe: stagedExe, BackupExe: oldExe + ".pre-launch-failure-op.bak",
		StatusPath: statusPathOverride, WorkingDir: root,
	}
	launches := []bool{}
	deps := defaultTransactionDeps()
	deps.waitPIDExit = func(int, time.Duration) error { return nil }
	deps.installFile = func(string, string, os.FileMode) error { return errors.New("injected install failure") }
	deps.launch = func(_ UpdateManifest, confirmation bool) (*exec.Cmd, <-chan error, error) {
		launches = append(launches, confirmation)
		return nil, nil, nil
	}
	if err := runReplacementTransaction(manifest, deps); err == nil || !strings.Contains(err.Error(), "injected install failure") {
		t.Fatalf("transaction error = %v, want injected install failure", err)
	}
	if !slices.Equal(launches, []bool{false}) {
		t.Fatalf("launch confirmations = %v, want one original-service restart", launches)
	}
	got, err := os.ReadFile(oldExe)
	if err != nil || string(got) != "old-exe" {
		t.Fatalf("original executable = %q, %v; want restored old executable", got, err)
	}
}

func TestUpdateTransactionPartialWorkerFailureRestoresWholeSet(t *testing.T) {
	root := t.TempDir()
	oldExe := filepath.Join(root, "ga-admin.exe")
	oldWorker := filepath.Join(root, "cmd", "chat_worker.py")
	stagedExe := filepath.Join(root, "stage", "ga-admin.exe")
	stagedWorker := filepath.Join(root, "stage", "cmd", "chat_worker.py")
	for path, content := range map[string]string{
		oldExe: "old-exe", oldWorker: "old-worker", stagedExe: "new-exe", stagedWorker: "new-worker",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	oldStatus := statusPathOverride
	statusPathOverride = filepath.Join(root, "ga-admin-update-status.json")
	defer func() { statusPathOverride = oldStatus }()
	if err := writeStatus(UpdateStatus{ID: "rollback-op", Running: true, Stage: "prepared"}); err != nil {
		t.Fatal(err)
	}

	manifest := UpdateManifest{
		OperationID: "rollback-op", SourceVersion: "v1.0.0", TargetVersion: "v2.0.0",
		OriginalExe: oldExe, StagedExe: stagedExe, BackupExe: oldExe + ".rollback-op.bak",
		Worker: oldWorker, StagedWorker: stagedWorker, WorkerBackup: oldWorker + ".rollback-op.bak",
		StatusPath: statusPathOverride, WorkingDir: root,
	}
	deps := defaultTransactionDeps()
	install := deps.installFile
	deps.installFile = func(src, dest string, perm os.FileMode) error {
		if sameFilePath(dest, oldWorker) {
			return errors.New("injected worker install failure")
		}
		return install(src, dest, perm)
	}
	deps.launch = func(_ UpdateManifest, confirmation bool) (*exec.Cmd, <-chan error, error) {
		if confirmation {
			t.Fatal("unexpected replacement launch")
		}
		return nil, nil, nil
	}
	if err := runReplacementTransaction(manifest, deps); err == nil || !strings.Contains(err.Error(), "injected worker") {
		t.Fatalf("transaction error = %v, want injected worker failure", err)
	}
	for path, want := range map[string]string{oldExe: "old-exe", oldWorker: "old-worker"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v; want restored %q", path, got, err, want)
		}
	}
	st := CurrentUpdateStatus()
	if st.Stage != "failed" || st.RollbackResult != "restored" || st.Running {
		t.Fatalf("rollback status = %+v", st)
	}
}

func TestUpdateTransactionPublishesStartingStageBeforeLaunch(t *testing.T) {
	root := t.TempDir()
	oldExe := filepath.Join(root, "ga-admin.exe")
	stagedExe := filepath.Join(root, "stage", "ga-admin.exe")
	for path, content := range map[string]string{oldExe: "old-exe", stagedExe: "new-exe"} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	oldStatus := statusPathOverride
	statusPathOverride = filepath.Join(root, "ga-admin-update-status.json")
	defer func() { statusPathOverride = oldStatus }()
	operationID := "synchronous-confirm-op"
	if err := writeStatus(UpdateStatus{ID: operationID, Running: true, Stage: "prepared", TargetVersion: effectiveVersion()}); err != nil {
		t.Fatal(err)
	}

	manifest := UpdateManifest{
		OperationID: operationID, SourceVersion: "v1.0.0", TargetVersion: effectiveVersion(),
		OriginalExe: oldExe, StagedExe: stagedExe, BackupExe: oldExe + ".bak",
		StatusPath: statusPathOverride, WorkingDir: root, ConfirmTimeout: time.Second,
	}
	child := exec.Command(os.Args[0], "-test.run=^$")
	child.Process, _ = os.FindProcess(os.Getpid())
	deps := defaultTransactionDeps()
	deps.waitPIDExit = func(int, time.Duration) error { return nil }
	deps.stopChild = func(*exec.Cmd, <-chan error) {}
	deps.launch = func(_ UpdateManifest, confirmation bool) (*exec.Cmd, <-chan error, error) {
		if !confirmation {
			t.Fatal("unexpected rollback launch")
		}
		if err := ConfirmUpdateReady(operationID); err != nil {
			return nil, nil, fmt.Errorf("synchronous confirmation failed: %w", err)
		}
		return child, make(chan error), nil
	}
	deps.sleep = func(time.Duration) {}
	if err := runReplacementTransaction(manifest, deps); err != nil {
		t.Fatalf("runReplacementTransaction: %v", err)
	}
	st := CurrentUpdateStatus()
	if st.Stage != "done" || st.ConfirmedVersion != effectiveVersion() {
		t.Fatalf("transaction status = %+v", st)
	}
}

func TestBuildUpdateManifestPreservesRestartContract(t *testing.T) {
	root := t.TempDir()
	exeName := "ga-admin"
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}
	exe := filepath.Join(root, exeName)
	worker := filepath.Join(root, "cmd", "chat_worker.py")
	work := filepath.Join(t.TempDir(), "prepared")
	status := filepath.Join(root, "ga-admin-update-status.json")
	args := []string{
		"--headless",
		"--update-confirm", "stale-operation",
		"--port=19090",
		"--update-helper=stale-manifest.json",
		"--app-root", filepath.Join(root, "state"),
	}

	manifest, err := buildUpdateManifest(updateManifestInput{
		OperationID:   "update-contract-1",
		SourceVersion: "v1.0.0",
		TargetVersion: "v1.1.0",
		OldPID:        1234,
		OriginalExe:   exe,
		StagedExe:     filepath.Join(work, exeName),
		Worker:        worker,
		StagedWorker:  filepath.Join(work, "chat_worker.py"),
		StatusPath:    status,
		WorkingDir:    filepath.Join(root, "launch-cwd"),
		OriginalArgs:  args,
	})
	if err != nil {
		t.Fatalf("buildUpdateManifest: %v", err)
	}
	if manifest.StatusPath != status || manifest.WorkingDir != filepath.Join(root, "launch-cwd") {
		t.Fatalf("manifest paths = status %q cwd %q", manifest.StatusPath, manifest.WorkingDir)
	}
	if filepath.Dir(manifest.BackupExe) != filepath.Dir(exe) {
		t.Fatalf("executable backup %q is not on the install target directory", manifest.BackupExe)
	}
	if filepath.Dir(manifest.WorkerBackup) != filepath.Dir(worker) {
		t.Fatalf("worker backup %q is not on the worker target directory", manifest.WorkerBackup)
	}
	wantArgs := []string{"--headless", "--port=19090", "--app-root", filepath.Join(root, "state")}
	if !slices.Equal(manifest.OriginalArgs, wantArgs) {
		t.Fatalf("restart args = %#v, want %#v", manifest.OriginalArgs, wantArgs)
	}
	if manifest.ExitTimeout <= 0 || manifest.ConfirmTimeout <= 0 || manifest.StabilityTime <= 0 {
		t.Fatalf("manifest timeouts are not populated: %+v", manifest)
	}
	if err := validateUpdateManifest(manifest); err != nil {
		t.Fatalf("manifest does not satisfy transaction contract: %v", err)
	}
}

func TestWriteUpdateManifestAndCopiedHelperAreDurable(t *testing.T) {
	work := t.TempDir()
	source := filepath.Join(work, "running.exe")
	if err := os.WriteFile(source, []byte("helper-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := UpdateManifest{
		OperationID: "copy-helper-1", SourceVersion: "v1.0.0", TargetVersion: "v1.1.0",
		OldPID: 123, OriginalExe: filepath.Join(work, "ga-admin.exe"), StagedExe: filepath.Join(work, "new.exe"),
		BackupExe: filepath.Join(work, "ga-admin.exe.bak"), StatusPath: filepath.Join(work, "status.json"),
		WorkingDir: work, ExitTimeout: time.Second, ConfirmTimeout: time.Second, StabilityTime: time.Second,
	}

	helperPath, manifestPath, err := prepareUpdateHelper(work, source, manifest)
	if err != nil {
		t.Fatalf("prepareUpdateHelper: %v", err)
	}
	gotHelper, err := os.ReadFile(helperPath)
	if err != nil {
		t.Fatalf("read copied helper: %v", err)
	}
	if string(gotHelper) != "helper-binary" || filepath.Dir(helperPath) != work {
		t.Fatalf("copied helper = %q at %q", gotHelper, helperPath)
	}
	gotManifest, err := readUpdateManifest(manifestPath)
	if err != nil {
		t.Fatalf("read prepared manifest: %v", err)
	}
	if gotManifest.OperationID != manifest.OperationID || gotManifest.StatusPath != manifest.StatusPath {
		t.Fatalf("prepared manifest = %+v", gotManifest)
	}
}

func TestConfirmUpdateReadyRequiresMatchingOperation(t *testing.T) {
	oldStatus := statusPathOverride
	statusPathOverride = filepath.Join(t.TempDir(), "ga-admin-update-status.json")
	defer func() { statusPathOverride = oldStatus }()
	if err := writeStatus(UpdateStatus{ID: "expected-op", Running: true, Stage: "starting_replacement", TargetVersion: effectiveVersion()}); err != nil {
		t.Fatal(err)
	}

	if err := ConfirmUpdateReady("other-op"); !errors.Is(err, ErrUpdateSuperseded) {
		t.Fatalf("ConfirmUpdateReady error = %v, want ErrUpdateSuperseded", err)
	}
	st := CurrentUpdateStatus()
	if st.Stage == "done" || st.ID != "expected-op" {
		t.Fatalf("mismatched confirmation changed status: %+v", st)
	}
}

func TestConfirmUpdateReadyRejectsVersionMismatch(t *testing.T) {
	oldStatus := statusPathOverride
	statusPathOverride = filepath.Join(t.TempDir(), "ga-admin-update-status.json")
	defer func() { statusPathOverride = oldStatus }()
	if err := writeStatus(UpdateStatus{ID: "version-op", Running: true, Stage: "starting_replacement", TargetVersion: "v999.0.0"}); err != nil {
		t.Fatal(err)
	}

	if err := ConfirmUpdateReady("version-op"); err == nil || !strings.Contains(err.Error(), "version mismatch") {
		t.Fatalf("ConfirmUpdateReady error = %v, want version mismatch", err)
	}
	st := CurrentUpdateStatus()
	if st.Stage != "failed" || st.Running || st.Stage == "done" {
		t.Fatalf("version mismatch status = %+v", st)
	}
}
