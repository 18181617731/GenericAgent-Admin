package version

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

const (
	defaultUpdateRepository = "18181617731/GenericAgent-Admin"
	defaultRepoLatestURL    = "https://api.github.com/repos/" + defaultUpdateRepository + "/releases?per_page=50"
)

var repoLatestURL = defaultRepoLatestURL

// SetRepoURL overrides the default update repo URL (e.g. from config.local.json).
func SetRepoURL(url string) {
	url = strings.TrimSpace(url)
	if url == "" {
		repoLatestURL = defaultRepoLatestURL
		return
	}
	const githubPrefix = "https://github.com/"
	if strings.HasPrefix(url, githubPrefix) {
		repository := strings.Trim(strings.TrimPrefix(url, githubPrefix), "/")
		repository = strings.TrimSuffix(repository, ".git")
		if strings.Count(repository, "/") == 1 {
			repoLatestURL = "https://api.github.com/repos/" + repository + "/releases?per_page=50"
			return
		}
	}
	repoLatestURL = url
}

const (
	updateResponseHeaderTimeout   = 15 * time.Second
	downloadResponseHeaderTimeout = 90 * time.Second
	downloadMaxAttempts           = 3
)

var (
	updateHTTPClient   = &http.Client{Transport: updateHTTPTransport(updateResponseHeaderTimeout, true)}
	downloadHTTPClient = &http.Client{Transport: updateHTTPTransport(downloadResponseHeaderTimeout, true)}
	downloadRetryDelay = 500 * time.Millisecond
)

func updateHTTPTransport(headerTimeout time.Duration, allowHTTP2 bool) http.RoundTripper {
	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ResponseHeaderTimeout: headerTimeout,
			ForceAttemptHTTP2:     allowHTTP2,
		}
	}
	clone := tr.Clone()
	clone.ResponseHeaderTimeout = headerTimeout
	clone.ForceAttemptHTTP2 = allowHTTP2
	return clone
}

const (
	maxUpdateMetadataBytes = 2 << 20
	maxUpdatePackageBytes  = 256 << 20
	maxUpdateChecksumBytes = 1 << 20
)

type BuildInfo struct {
	Version                 string `json:"version"`
	Commit                  string `json:"commit"`
	Date                    string `json:"date"`
	GOOS                    string `json:"goos"`
	GOARCH                  string `json:"goarch"`
	Runtime                 string `json:"runtime"`
	Exe                     string `json:"exe"`
	UpdateSupported         bool   `json:"update_supported"`
	UpdateUnsupportedReason string `json:"update_unsupported_reason,omitempty"`
	UpdateRepository        string `json:"update_repository"`
	UpdateSourceURL         string `json:"update_source_url"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest,omitempty"`
}

type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	Assets      []Asset   `json:"assets"`
}

type CheckResult struct {
	Current   BuildInfo `json:"current"`
	Latest    *Release  `json:"latest,omitempty"`
	Update    bool      `json:"update"`
	Asset     *Asset    `json:"asset,omitempty"`
	Checksum  *Asset    `json:"checksum,omitempty"`
	Message   string    `json:"message,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

type ApplyResult struct {
	OK         bool   `json:"ok"`
	Message    string `json:"message"`
	Script     string `json:"script,omitempty"`
	Restarting bool   `json:"restarting,omitempty"`
}

type UpdateStatus struct {
	ID             string       `json:"id,omitempty"`
	PID            int          `json:"pid,omitempty"`
	Running        bool         `json:"running"`
	Stage          string       `json:"stage"`
	Progress       int          `json:"progress"`
	Message        string       `json:"message"`
	Error          string       `json:"error,omitempty"`
	Script         string       `json:"script,omitempty"`
	AppliedVersion string       `json:"applied_version,omitempty"`
	Check          *CheckResult `json:"check,omitempty"`
	StartedAt      time.Time    `json:"started_at,omitempty"`
	UpdatedAt      time.Time    `json:"updated_at,omitempty"`
	EndedAt        time.Time    `json:"ended_at,omitempty"`
}

var (
	updateMu           sync.Mutex
	statusPathOverride string
	exitProcess        = os.Exit
)

func statusPath() string {
	if statusPathOverride != "" {
		return statusPathOverride
	}
	exe, err := os.Executable()
	if err == nil && exe != "" {
		return filepath.Join(filepath.Dir(exe), "ga-admin-update-status.json")
	}
	return filepath.Join(os.TempDir(), "ga-admin-update-status.json")
}

func CurrentUpdateStatus() UpdateStatus {
	updateMu.Lock()
	defer updateMu.Unlock()
	return currentStatusLocked()
}

func readStatusLocked() UpdateStatus {
	var st UpdateStatus
	b, err := os.ReadFile(statusPath())
	if err != nil {
		return st
	}
	if err := json.Unmarshal(b, &st); err != nil {
		now := time.Now()
		return UpdateStatus{
			Running:   false,
			Stage:     "error",
			Progress:  100,
			Message:   "读取升级状态失败: " + err.Error(),
			Error:     err.Error(),
			UpdatedAt: now,
			EndedAt:   now,
		}
	}
	return st
}

func currentStatusLocked() UpdateStatus {
	stored := readStatusLocked()
	normalized := normalizeStatusAfterRestart(stored)
	if statusNeedsPersistence(stored, normalized) {
		_ = writeStatusLocked(normalized)
	}
	return normalized
}

func statusNeedsPersistence(stored, normalized UpdateStatus) bool {
	if normalized.Running || (normalized.Stage != "done" && normalized.Stage != "error") {
		return false
	}
	return stored.Running != normalized.Running ||
		stored.Stage != normalized.Stage ||
		stored.Message != normalized.Message ||
		stored.Error != normalized.Error ||
		stored.AppliedVersion != normalized.AppliedVersion
}

func normalizeStatusAfterRestart(st UpdateStatus) UpdateStatus {
	if !st.Running {
		if st.Stage == "done" && st.AppliedVersion == "" && st.Check != nil && st.Check.Latest != nil {
			return verifyAppliedVersion(st)
		}
		return st
	}
	switch st.Stage {
	case "restarting", "applying":
		if st.PID != 0 && st.PID == os.Getpid() {
			return st
		}
		return verifyAppliedVersion(st)
	}
	return st
}

func verifyAppliedVersion(st UpdateStatus) UpdateStatus {
	now := time.Now()
	st.Running = false
	st.Progress = 100
	target := ""
	if st.Check != nil && st.Check.Latest != nil {
		target = formalVersion(st.Check.Latest.TagName)
	}
	current := formalVersion(effectiveVersion())
	if target != "" && current == target {
		st.Stage = "done"
		st.Message = fmt.Sprintf("升级成功，当前版本 %s", current)
		st.Error = ""
		st.AppliedVersion = current
	} else {
		st.Stage = "error"
		if target == "" {
			st.Error = "升级后无法确认目标版本"
		} else {
			st.Error = fmt.Sprintf("升级未生效：当前版本 %s，目标版本 %s", current, target)
		}
		st.Message = st.Error
	}
	st.UpdatedAt = now
	st.EndedAt = now
	return st
}

func writeStatus(st UpdateStatus) error {
	updateMu.Lock()
	defer updateMu.Unlock()
	return writeStatusLocked(st)
}

func writeStatusLocked(st UpdateStatus) error {
	st.UpdatedAt = time.Now()
	if st.ID == "" {
		st.ID = fmt.Sprintf("update-%d", st.UpdatedAt.Unix())
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(statusPath(), b, 0600)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func StartApplyLatest() (UpdateStatus, error) {
	updateMu.Lock()
	cur := currentStatusLocked()
	updateMu.Unlock()
	if cur.Running {
		return cur, nil
	}
	now := time.Now()
	st := UpdateStatus{ID: fmt.Sprintf("update-%d", now.Unix()), PID: os.Getpid(), Running: true, Stage: "queued", Progress: 1, Message: "升级任务已启动", StartedAt: now, UpdatedAt: now}
	if err := writeStatus(st); err != nil {
		st.Running = false
		st.Stage = "error"
		st.Progress = 100
		st.Error = err.Error()
		st.Message = "写入升级状态失败: " + err.Error()
		st.EndedAt = time.Now()
		return st, fmt.Errorf("write update status: %w", err)
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		res, err := applyLatest(ctx, func(stage, msg string, progress int, check *CheckResult) {
			if progress < 0 {
				progress = 0
			}
			if progress > 100 {
				progress = 100
			}
			st.Stage, st.Message, st.Progress = stage, msg, progress
			if check != nil {
				st.Check = check
			}
			_ = writeStatus(st)
		})
		if err != nil {
			st.Running = false
			st.Stage = "error"
			st.Progress = 100
			st.Error = err.Error()
			st.Message = err.Error()
			st.EndedAt = time.Now()
			_ = writeStatus(st)
			return
		}
		st = finishApplyStatus(st, res)
		_ = writeStatus(st)
	}()
	return st, nil
}

func finishApplyStatus(st UpdateStatus, res ApplyResult) UpdateStatus {
	st.Script = res.Script
	st.Message = res.Message
	if res.Restarting {
		st.Running = true
		st.Stage = "restarting"
		st.Progress = 95
		st.EndedAt = time.Time{}
		return st
	}
	st.Running = false
	st.Stage = "done"
	st.Progress = 100
	st.EndedAt = time.Now()
	return st
}

func Current() BuildInfo {
	exe, _ := os.Executable()
	supported, reason := updateSupportStatus()
	repository, sourceURL := currentUpdateSource()
	return BuildInfo{
		Version:                 effectiveVersion(),
		Commit:                  effectiveCommit(),
		Date:                    Date,
		GOOS:                    runtime.GOOS,
		GOARCH:                  runtime.GOARCH,
		Runtime:                 runtime.Version(),
		Exe:                     exe,
		UpdateSupported:         supported,
		UpdateUnsupportedReason: reason,
		UpdateRepository:        repository,
		UpdateSourceURL:         sourceURL,
	}
}

func currentUpdateSource() (string, string) {
	const apiPrefix = "https://api.github.com/repos/"
	if strings.HasPrefix(repoLatestURL, apiPrefix) {
		path := strings.TrimPrefix(repoLatestURL, apiPrefix)
		marker := strings.Index(path, "/releases")
		if marker < 0 {
			return repoLatestURL, repoLatestURL
		}
		repository := path[:marker]
		if strings.Count(repository, "/") == 1 {
			return repository, "https://github.com/" + repository + "/releases"
		}
	}
	return repoLatestURL, repoLatestURL
}

func updateSupportStatus() (bool, string) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return false, "one-click self update is only implemented for Windows and Linux packages"
	}
	return true, ""
}

func effectiveVersion() string {
	v := strings.TrimSpace(Version)
	if v != "" && v != "dev" && v != "unknown" {
		return formalVersion(v)
	}
	if out, ok := gitOutput("describe", "--tags", "--abbrev=0", "--match", "v[0-9]*"); ok {
		out = strings.TrimSpace(out)
		if out != "" {
			return formalVersion(out)
		}
	}
	if v != "" {
		return v
	}
	return "dev"
}

func formalVersion(value string) string {
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "-dirty"))
	parts := strings.Split(value, "-")
	if len(parts) >= 3 && strings.HasPrefix(parts[len(parts)-1], "g") {
		if _, err := strconv.Atoi(parts[len(parts)-2]); err == nil {
			value = strings.Join(parts[:len(parts)-2], "-")
		}
	}
	if value == "" {
		return "dev"
	}
	return value
}

func effectiveCommit() string {
	c := strings.TrimSpace(Commit)
	if c != "" && c != "unknown" {
		return c
	}
	if out, ok := gitOutput("rev-parse", "--short", "HEAD"); ok {
		out = strings.TrimSpace(out)
		if out != "" {
			return out
		}
	}
	if c != "" {
		return c
	}
	return "unknown"
}

func gitOutput(args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	hideChildWindow(cmd)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	b, err := cmd.Output()
	if err != nil || ctx.Err() != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

func Check(ctx context.Context) (CheckResult, error) {
	cur := Current()
	rel, err := fetchLatest(ctx)
	res := CheckResult{Current: cur, CheckedAt: time.Now()}
	if err != nil {
		res.Message = err.Error()
		return res, err
	}
	res.Latest = rel
	asset, sum := selectAssets(*rel)
	res.Asset, res.Checksum = asset, sum
	res.Update = newer(cur.Version, rel.TagName)
	if asset == nil {
		res.Message = "no asset for current platform"
	}
	return res, nil
}

func ApplyLatest(ctx context.Context) (ApplyResult, error) {
	return applyLatest(ctx, nil)
}

func applyLatest(ctx context.Context, progress func(stage, msg string, pct int, check *CheckResult)) (ApplyResult, error) {
	emit := func(stage, msg string, pct int, check *CheckResult) {
		if progress != nil {
			progress(stage, msg, pct, check)
		}
	}
	emit("checking", "正在检查最新版本", 5, nil)
	check, err := Check(ctx)
	if err != nil {
		return ApplyResult{}, err
	}
	emit("checked", "已检查版本信息", 15, &check)
	if !check.Update {
		return ApplyResult{OK: true, Message: "already up to date"}, nil
	}
	if check.Asset == nil {
		return ApplyResult{}, errors.New("missing release asset for current platform")
	}
	assetDigest, hasAssetDigest := releaseAssetSHA256(check.Asset)
	if !hasAssetDigest && check.Checksum == nil {
		return ApplyResult{}, errors.New("missing release asset digest or checksum for current platform")
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return ApplyResult{}, errors.New("one-click self update is only implemented for Windows and Linux packages")
	}
	exe, err := os.Executable()
	if err != nil {
		return ApplyResult{}, err
	}
	work, err := os.MkdirTemp("", "ga-admin-update-*")
	if err != nil {
		return ApplyResult{}, err
	}
	zipPath := filepath.Join(work, check.Asset.Name)
	emit("downloading", "正在下载升级包", 25, &check)
	if err := download(ctx, check.Asset.BrowserDownloadURL, zipPath, maxUpdatePackageBytes); err != nil {
		return ApplyResult{}, err
	}
	sumPath := ""
	if !hasAssetDigest {
		sumPath = filepath.Join(work, check.Checksum.Name)
		emit("downloading_checksum", "正在下载校验文件", 55, &check)
		if err := download(ctx, check.Checksum.BrowserDownloadURL, sumPath, maxUpdateChecksumBytes); err != nil {
			return ApplyResult{}, err
		}
	}
	emit("verifying", "正在校验 SHA256", 65, &check)
	if hasAssetDigest {
		if err := verifySHA256Value(zipPath, assetDigest); err != nil {
			return ApplyResult{}, err
		}
	} else {
		if err := verifySHA256(zipPath, sumPath); err != nil {
			return ApplyResult{}, err
		}
	}
	dir := filepath.Join(work, "unzipped")
	emit("extracting", "正在解压升级包", 75, &check)
	if err := unzip(zipPath, dir); err != nil {
		return ApplyResult{}, err
	}
	emit("preparing", "正在准备替换脚本", 85, &check)
	binName := "ga-admin"
	if runtime.GOOS == "windows" {
		binName = "ga-admin.exe"
	}
	newExe, err := findFile(dir, binName)
	if err != nil {
		return ApplyResult{}, err
	}
	newWorker, workerErr := findFile(dir, "chat_worker.py")
	if workerErr != nil {
		return ApplyResult{}, fmt.Errorf("chat_worker.py missing from package: %w", workerErr)
	}
	worker := filepath.Join(filepath.Dir(exe), "cmd", "chat_worker.py")
	backup := exe + ".bak"
	workerBackup := worker + ".bak"

	var content string
	var script string
	if runtime.GOOS == "windows" {
		script = filepath.Join(work, "apply-update.cmd")
		content = windowsUpdateScript(exe, newExe, backup, worker, newWorker, workerBackup)
	} else {
		script = filepath.Join(work, "apply-update.sh")
		content = linuxUpdateScript(exe, newExe, backup, worker, newWorker, workerBackup)
	}
	if err := writeFileAtomic(script, []byte(content), 0600); err != nil {
		return ApplyResult{}, err
	}
	emit("restarting", "升级包已就绪，正在重启服务", 95, &check)
	cmd := updateScriptCommand(runtime.GOOS, script)
	cmd.Dir = work
	hideChildWindow(cmd)
	if err := cmd.Start(); err != nil {
		return ApplyResult{}, fmt.Errorf("启动升级脚本失败: %w", err)
	}
	go func() { time.Sleep(500 * time.Millisecond); exitProcess(0) }()
	return ApplyResult{OK: true, Message: "升级包已就绪，正在重启服务", Script: script, Restarting: true}, nil
}

func updateScriptCommand(goos, script string) *exec.Cmd {
	if goos == "windows" {
		return exec.Command("cmd", "/D", "/Q", "/C", script)
	}
	return exec.Command("bash", script)
}

func windowsUpdateScript(oldExe, newExe, backup, worker, newWorker, workerBackup string) string {
	return fmt.Sprintf(`@echo off
setlocal
set "OLD=%s"
set "NEW=%s"
set "BAK=%s"
set "WORKER=%s"
set "NEW_WORKER=%s"
set "WORKER_BAK=%s"
set "WORKER_HAD_ORIGINAL=0"
for %%%%D in ("%%OLD%%") do set "OLD_DIR=%%%%~dpD"
for /L %%%%i in (1,1,30) do (
  move /Y "%%OLD%%" "%%BAK%%" >nul 2>nul && goto replaced
  timeout /t 1 /nobreak >nul
)
echo failed to replace %%OLD%%
exit /b 1
:replaced
move /Y "%%NEW%%" "%%OLD%%" >nul
if errorlevel 1 (move /Y "%%BAK%%" "%%OLD%%" >nul 2>nul & exit /b 1)
if not "%%NEW_WORKER%%"=="" (
  for %%%%D in ("%%WORKER%%") do if not exist "%%%%~dpD" mkdir "%%%%~dpD"
  if exist "%%WORKER%%" (
    set "WORKER_HAD_ORIGINAL=1"
    move /Y "%%WORKER%%" "%%WORKER_BAK%%" >nul 2>nul
  )
  move /Y "%%NEW_WORKER%%" "%%WORKER%%" >nul
  if errorlevel 1 (
    if exist "%%WORKER_BAK%%" move /Y "%%WORKER_BAK%%" "%%WORKER%%" >nul 2>nul
    move /Y "%%OLD%%" "%%NEW%%" >nul 2>nul
    move /Y "%%BAK%%" "%%OLD%%" >nul 2>nul
    exit /b 1
  )
)
start "" /D "%%OLD_DIR%%" "%%OLD%%"
if errorlevel 1 goto launch_failed
exit /b 0
:launch_failed
if "%%WORKER_HAD_ORIGINAL%%"=="1" (
  if exist "%%WORKER%%" del /Q "%%WORKER%%" >nul 2>nul
  if exist "%%WORKER_BAK%%" move /Y "%%WORKER_BAK%%" "%%WORKER%%" >nul 2>nul
) else if exist "%%WORKER%%" (
  del /Q "%%WORKER%%" >nul 2>nul
)
move /Y "%%OLD%%" "%%NEW%%" >nul 2>nul
move /Y "%%BAK%%" "%%OLD%%" >nul 2>nul
start "" /D "%%OLD_DIR%%" "%%OLD%%"
exit /b 1
`, oldExe, newExe, backup, worker, newWorker, workerBackup)
}

func linuxUpdateScript(oldExe, newExe, backup, worker, newWorker, workerBackup string) string {
	return fmt.Sprintf(`#!/bin/bash
OLD="%s"
NEW="%s"
BAK="%s"
WORKER="%s"
NEW_WORKER="%s"
WORKER_BAK="%s"
for i in $(seq 1 30); do
  mv "$OLD" "$BAK" 2>/dev/null && break
  sleep 1
done
if [ ! -f "$BAK" ]; then
  echo "failed to replace $OLD"
  exit 1
fi
cp "$NEW" "$OLD"
if [ $? -ne 0 ]; then
  mv "$BAK" "$OLD"
  exit 1
fi
chmod +x "$OLD"
if [ -n "$NEW_WORKER" ]; then
  mkdir -p "$(dirname "$WORKER")" 2>/dev/null
  [ -f "$WORKER" ] && cp "$WORKER" "$WORKER_BAK"
  cp "$NEW_WORKER" "$WORKER"
  if [ $? -ne 0 ]; then
    [ -f "$WORKER_BAK" ] && cp "$WORKER_BAK" "$WORKER"
    cp "$BAK" "$OLD"
    exit 1
  fi
fi
exec "$OLD"
`, oldExe, newExe, backup, worker, newWorker, workerBackup)
}

func fetchLatest(ctx context.Context) (rel *Release, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, repoLatestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create github release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ga-admin-updater")
	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close github release response: %w", closeErr)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		if resp.StatusCode == http.StatusNotFound {
			repository, _ := currentUpdateSource()
			return nil, fmt.Errorf("更新仓库 %s 尚未发布 GitHub Release", repository)
		}
		return nil, fmt.Errorf("github release check failed: %s %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var out Release
	if resp.ContentLength > maxUpdateMetadataBytes {
		return nil, fmt.Errorf("github release metadata too large: %d bytes exceeds limit %d", resp.ContentLength, maxUpdateMetadataBytes)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxUpdateMetadataBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > maxUpdateMetadataBytes {
		return nil, fmt.Errorf("github release metadata too large: exceeds limit %d", maxUpdateMetadataBytes)
	}
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var releases []Release
		if err := json.Unmarshal(trimmed, &releases); err != nil {
			return nil, err
		}
		selected, err := selectLatestRelease(releases)
		if err != nil {
			return nil, err
		}
		return selected, nil
	}
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func selectLatestRelease(releases []Release) (*Release, error) {
	selectHighest := func(includePrerelease bool) *Release {
		var selected *Release
		for i := range releases {
			candidate := &releases[i]
			if candidate.Draft || (!includePrerelease && candidate.Prerelease) || !isVersionTag(candidate.TagName) {
				continue
			}
			if selected == nil {
				selected = candidate
				continue
			}
			comparison := compareSemver(strings.TrimPrefix(candidate.TagName, "v"), strings.TrimPrefix(selected.TagName, "v"))
			if comparison > 0 || (comparison == 0 && candidate.PublishedAt.After(selected.PublishedAt)) {
				selected = candidate
			}
		}
		return selected
	}
	if selected := selectHighest(false); selected != nil {
		return selected, nil
	}
	if selected := selectHighest(true); selected != nil {
		return selected, nil
	}
	return nil, errors.New("更新仓库尚未发布有效的语义版本 Release")
}

func isVersionTag(tag string) bool {
	var major, minor, patch int
	_, err := fmt.Sscanf(strings.TrimPrefix(strings.TrimSpace(tag), "v"), "%d.%d.%d", &major, &minor, &patch)
	return err == nil
}

func selectAssets(rel Release) (*Asset, *Asset) {
	want := fmt.Sprintf("%s-%s.zip", runtime.GOOS, runtime.GOARCH)
	var zipAsset, sumAsset *Asset
	for i := range rel.Assets {
		a := &rel.Assets[i]
		if strings.HasSuffix(a.Name, want) {
			zipAsset = a
		}
		if strings.HasSuffix(a.Name, want+".sha256") {
			sumAsset = a
		}
	}
	return zipAsset, sumAsset
}

func newer(current, latest string) bool {
	c := strings.TrimPrefix(strings.TrimSpace(current), "v")
	l := strings.TrimPrefix(strings.TrimSpace(latest), "v")
	if c == "" || c == "dev" || c == "unknown" {
		return true
	}
	return compareSemver(l, c) > 0
}

func compareSemver(a, b string) int {
	ap, bp := splitVer(a), splitVer(b)
	for i := 0; i < 3; i++ {
		if ap[i] > bp[i] {
			return 1
		}
		if ap[i] < bp[i] {
			return -1
		}
	}
	return strings.Compare(a, b)
}
func splitVer(s string) [3]int {
	var out [3]int
	parts := strings.Split(strings.Split(s, "-")[0], ".")
	for i := 0; i < len(parts) && i < 3; i++ {
		fmt.Sscanf(parts[i], "%d", &out[i])
	}
	return out
}

func download(ctx context.Context, url, dest string, maxBytes int64) error {
	var lastErr error
	attempts := 0
	for attempt := 1; attempt <= downloadMaxAttempts; attempt++ {
		attempts = attempt
		retryable, err := downloadOnce(ctx, url, dest, maxBytes)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable || attempt == downloadMaxAttempts {
			break
		}
		if err := waitDownloadRetry(ctx, downloadRetryDelay*time.Duration(attempt)); err != nil {
			return err
		}
	}
	return fmt.Errorf("下载失败（已尝试 %d 次）: %w", attempts, lastErr)
}

func downloadOnce(ctx context.Context, url, dest string, maxBytes int64) (retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("create download request: %w", err)
	}
	req.Header.Set("User-Agent", "ga-admin-updater")
	resp, err := downloadHTTPClient.Do(req)
	if err != nil {
		return retryableDownloadError(ctx, err), err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close download response: %w", closeErr)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return retryableDownloadStatus(resp.StatusCode), fmt.Errorf("download failed: %s", resp.Status)
	}
	if maxBytes > 0 && resp.ContentLength > maxBytes {
		return false, fmt.Errorf("download too large: %d bytes exceeds limit %d", resp.ContentLength, maxBytes)
	}
	r := resp.Body
	if maxBytes > 0 {
		r = http.MaxBytesReader(nil, resp.Body, maxBytes)
	}
	if err := writeStreamAtomic(dest, r, 0600); err != nil {
		return retryableDownloadError(ctx, err), fmt.Errorf("write download file: %w", err)
	}
	return false, nil
}

func retryableDownloadStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooEarly ||
		status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func retryableDownloadError(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr) || errors.Is(err, io.ErrUnexpectedEOF)
}

func waitDownloadRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func writeStreamAtomic(path string, r io.Reader, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func verifySHA256(file, sumFile string) error {
	data, err := os.ReadFile(sumFile)
	if err != nil {
		return err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return errors.New("empty sha256 file")
	}
	want := strings.ToLower(fields[0])
	return verifySHA256Value(file, want)
}

func releaseAssetSHA256(asset *Asset) (string, bool) {
	if asset == nil {
		return "", false
	}
	digest := strings.TrimSpace(strings.ToLower(asset.Digest))
	const prefix = "sha256:"
	if !strings.HasPrefix(digest, prefix) {
		return "", false
	}
	digest = strings.TrimPrefix(digest, prefix)
	decoded, err := hex.DecodeString(digest)
	return digest, err == nil && len(decoded) == sha256.Size
}

func verifySHA256Value(file, want string) error {
	want = strings.TrimSpace(strings.ToLower(want))
	decoded, err := hex.DecodeString(want)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("invalid sha256 digest: %q", want)
	}
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("sha256 mismatch: got %s want %s", got, want)
	}
	return nil
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	destClean, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	destClean = filepath.Clean(destClean)
	for _, f := range r.File {
		if strings.Contains(f.Name, `\\`) {
			return fmt.Errorf("unsafe zip path: %s", f.Name)
		}
		name := filepath.Clean(f.Name)
		if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) || name == ".." {
			return fmt.Errorf("unsafe zip path: %s", f.Name)
		}
		path := filepath.Join(destClean, name)
		absPath, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		absPath = filepath.Clean(absPath)
		if absPath != destClean && !strings.HasPrefix(absPath, destClean+string(filepath.Separator)) {
			return fmt.Errorf("unsafe zip path: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(absPath, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		writeErr := writeStreamAtomic(absPath, rc, f.Mode())
		rcErr := rc.Close()
		if writeErr != nil {
			return writeErr
		}
		if rcErr != nil {
			_ = os.Remove(absPath)
			return rcErr
		}
	}
	return nil
}

func findFile(dir, name string) (string, error) {
	var hits []string
	if err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), name) {
			hits = append(hits, p)
		}
		return nil
	}); err != nil {
		return "", fmt.Errorf("walk package for %s: %w", name, err)
	}
	sort.Strings(hits)
	if len(hits) == 0 {
		return "", fmt.Errorf("%s not found in package", name)
	}
	return hits[0], nil
}
