package version

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

var repoLatestURL = "https://api.github.com/repos/Fwind43/GenericAgent-Admin/releases/latest"

const updateResponseHeaderTimeout = 15 * time.Second

var updateHTTPClient = &http.Client{Transport: updateHTTPTransport()}

func updateHTTPTransport() http.RoundTripper {
	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ResponseHeaderTimeout: updateResponseHeaderTimeout,
		}
	}
	clone := tr.Clone()
	clone.ResponseHeaderTimeout = updateResponseHeaderTimeout
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
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
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
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Script  string `json:"script,omitempty"`
}

type UpdateStatus struct {
	ID        string       `json:"id,omitempty"`
	PID       int          `json:"pid,omitempty"`
	Running   bool         `json:"running"`
	Stage     string       `json:"stage"`
	Progress  int          `json:"progress"`
	Message   string       `json:"message"`
	Error     string       `json:"error,omitempty"`
	Script    string       `json:"script,omitempty"`
	Check     *CheckResult `json:"check,omitempty"`
	StartedAt time.Time    `json:"started_at,omitempty"`
	UpdatedAt time.Time    `json:"updated_at,omitempty"`
	EndedAt   time.Time    `json:"ended_at,omitempty"`
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
	return readStatusLocked()
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
	return normalizeStatusAfterRestart(st)
}

func normalizeStatusAfterRestart(st UpdateStatus) UpdateStatus {
	if !st.Running {
		return st
	}
	switch st.Stage {
	case "restarting", "applying":
		if st.PID != 0 && st.PID == os.Getpid() {
			return st
		}
		now := time.Now()
		st.Running = false
		st.Stage = "done"
		st.Progress = 100
		if st.Message == "" {
			st.Message = "升级已应用，服务已重启"
		}
		st.UpdatedAt = now
		st.EndedAt = now
	}
	return st
}

func writeStatus(st UpdateStatus) error {
	updateMu.Lock()
	defer updateMu.Unlock()
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
	cur := readStatusLocked()
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
		st.Running = false
		st.Stage = "done"
		st.Progress = 100
		st.Message = res.Message
		st.Script = res.Script
		st.EndedAt = time.Now()
		_ = writeStatus(st)
	}()
	return st, nil
}

func Current() BuildInfo {
	exe, _ := os.Executable()
	supported, reason := updateSupportStatus()
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
	}
}

func updateSupportStatus() (bool, string) {
	if runtime.GOOS != "windows" {
		return false, "one-click self update is currently implemented for Windows packages"
	}
	return true, ""
}

func effectiveVersion() string {
	v := strings.TrimSpace(Version)
	if v != "" && v != "dev" && v != "unknown" {
		return v
	}
	if out, ok := gitOutput("describe", "--tags", "--dirty", "--always"); ok {
		out = strings.TrimSpace(out)
		if out != "" {
			return out
		}
	}
	if v != "" {
		return v
	}
	return "dev"
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
	if check.Asset == nil || check.Checksum == nil {
		return ApplyResult{}, errors.New("missing release asset or checksum for current platform")
	}
	if runtime.GOOS != "windows" {
		return ApplyResult{}, errors.New("one-click self update is currently implemented for Windows packages")
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
	sumPath := filepath.Join(work, check.Checksum.Name)
	emit("downloading", "正在下载升级包", 25, &check)
	if err := download(ctx, check.Asset.BrowserDownloadURL, zipPath, maxUpdatePackageBytes); err != nil {
		return ApplyResult{}, err
	}
	emit("downloading_checksum", "正在下载校验文件", 55, &check)
	if err := download(ctx, check.Checksum.BrowserDownloadURL, sumPath, maxUpdateChecksumBytes); err != nil {
		return ApplyResult{}, err
	}
	emit("verifying", "正在校验 SHA256", 65, &check)
	if err := verifySHA256(zipPath, sumPath); err != nil {
		return ApplyResult{}, err
	}
	dir := filepath.Join(work, "unzipped")
	emit("extracting", "正在解压升级包", 75, &check)
	if err := unzip(zipPath, dir); err != nil {
		return ApplyResult{}, err
	}
	emit("preparing", "正在准备替换脚本", 85, &check)
	newExe, err := findFile(dir, "ga-admin.exe")
	if err != nil {
		return ApplyResult{}, err
	}
	newWorker, workerErr := findFile(dir, "chat_worker.py")
	if workerErr != nil {
		return ApplyResult{}, fmt.Errorf("chat_worker.py missing from package: %w", workerErr)
	}
	worker := filepath.Join(filepath.Dir(exe), "cmd", "chat_worker.py")
	script := filepath.Join(work, "apply-update.cmd")
	backup := exe + ".bak"
	workerBackup := worker + ".bak"
	content := windowsUpdateScript(exe, newExe, backup, worker, newWorker, workerBackup)
	if err := writeFileAtomic(script, []byte(content), 0600); err != nil {
		return ApplyResult{}, err
	}
	emit("restarting", "升级包已就绪，正在重启服务", 95, &check)
	cmd := exec.Command("cmd", "/C", "start", "", script)
	cmd.Dir = work
	hideChildWindow(cmd)
	if err := cmd.Start(); err != nil {
		return ApplyResult{}, err
	}
	go func() { time.Sleep(500 * time.Millisecond); exitProcess(0) }()
	return ApplyResult{OK: true, Message: "update downloaded; restarting", Script: script}, nil
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
  if exist "%%WORKER%%" move /Y "%%WORKER%%" "%%WORKER_BAK%%" >nul 2>nul
  move /Y "%%NEW_WORKER%%" "%%WORKER%%" >nul
  if errorlevel 1 (
    if exist "%%WORKER_BAK%%" move /Y "%%WORKER_BAK%%" "%%WORKER%%" >nul 2>nul
    move /Y "%%OLD%%" "%%NEW%%" >nul 2>nul
    move /Y "%%BAK%%" "%%OLD%%" >nul 2>nul
    exit /b 1
  )
)
start "" "%%OLD%%"
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
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
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

func download(ctx context.Context, url, dest string, maxBytes int64) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close download response: %w", closeErr)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	if maxBytes > 0 && resp.ContentLength > maxBytes {
		return fmt.Errorf("download too large: %d bytes exceeds limit %d", resp.ContentLength, maxBytes)
	}
	r := resp.Body
	if maxBytes > 0 {
		r = http.MaxBytesReader(nil, resp.Body, maxBytes)
	}
	if err := writeStreamAtomic(dest, r, 0600); err != nil {
		return fmt.Errorf("write download file: %w", err)
	}
	return nil
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
