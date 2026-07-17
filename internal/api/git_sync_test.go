package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGaGitStatusDistinguishesRemoteLatestFromSynchronized(t *testing.T) {
	root := gitSyncTestRoot(t)
	oldRunGit := runGitCommandFunc
	t.Cleanup(func() { runGitCommandFunc = oldRunGit })
	runGitCommandFunc = func(_ context.Context, _ string, args ...string) (string, error) {
		switch strings.Join(args, " ") {
		case "branch --show-current":
			return "company_ga", nil
		case "rev-parse --short HEAD":
			return "abc1234", nil
		case "status --short":
			return " M memory/global_mem.txt\n?? report.md", nil
		case "rev-parse --abbrev-ref --symbolic-full-name @{u}":
			return "origin/company_ga", nil
		case "rev-list --left-right --count HEAD...@{u}":
			return "2 0", nil
		default:
			t.Fatalf("unexpected git command: %s", strings.Join(args, " "))
			return "", nil
		}
	}

	status, err := gaGitStatusForRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if status["remote_latest"] != true || status["synchronized"] != false {
		t.Fatalf("unexpected sync state: %#v", status)
	}
	if status["ahead"] != 2 || status["changed_files"] != 2 || status["strategy_available"] != true {
		t.Fatalf("missing pending sync metadata: %#v", status)
	}
}

func TestGaGitStatusFetchesOriginOnly(t *testing.T) {
	root := gitSyncTestRoot(t)
	s := newConfigTestServer(t)
	s.CfgStore.Cfg.GARoot = root
	oldRunGit := runGitCommandFunc
	t.Cleanup(func() { runGitCommandFunc = oldRunGit })
	fetchArgs := ""
	runGitCommandFunc = func(_ context.Context, _ string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		if len(args) > 0 && args[0] == "fetch" {
			fetchArgs = joined
			return "", nil
		}
		return gitSyncCleanCommand(t, joined)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ga/git-status?remote=1", nil)
	s.gaGitStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if fetchArgs != "fetch origin --prune" {
		t.Fatalf("fetch command=%q", fetchArgs)
	}
}

func TestGaGitStatusBlocksNonOriginTrackingBranch(t *testing.T) {
	root := gitSyncTestRoot(t)
	oldRunGit := runGitCommandFunc
	t.Cleanup(func() { runGitCommandFunc = oldRunGit })
	runGitCommandFunc = func(_ context.Context, _ string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		if joined == "rev-parse --abbrev-ref --symbolic-full-name @{u}" {
			return "upstream/company_ga", nil
		}
		return gitSyncCleanCommand(t, joined)
	}

	status, err := gaGitStatusForRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if status["tracking_matches_origin"] != false || status["synchronized"] != false {
		t.Fatalf("non-origin tracking must not be synchronized: %#v", status)
	}
	if status["expected_origin"] != "origin/company_ga" {
		t.Fatalf("expected origin target missing: %#v", status)
	}
}

func TestGaGitUpdateRunsDailyAutoSyncStrategy(t *testing.T) {
	root := gitSyncTestRoot(t)
	s := newConfigTestServer(t)
	s.CfgStore.Cfg.GARoot = root
	s.CfgStore.Cfg.EffectivePython = "python-test"
	oldRunGit := runGitCommandFunc
	oldAutoSync := runGAAutoSyncFunc
	t.Cleanup(func() { runGitCommandFunc = oldRunGit; runGAAutoSyncFunc = oldAutoSync })
	commitCalls := 0
	statusCalls := 0
	runGitCommandFunc = func(_ context.Context, _ string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		switch joined {
		case "rev-parse --short HEAD":
			commitCalls++
			if commitCalls == 1 {
				return "before1", nil
			}
			return "after22", nil
		case "status --short":
			statusCalls++
			if statusCalls == 1 {
				return " M memory/global_mem.txt", nil
			}
			return "", nil
		default:
			return gitSyncCleanCommand(t, joined)
		}
	}
	runGAAutoSyncFunc = func(_ context.Context, python, script, repo string) (string, error) {
		if python != "python-test" || script != gaGitSyncScript(root) || repo != root {
			t.Fatalf("unexpected autosync invocation: %q %q %q", python, script, repo)
		}
		return "同步状态: 全部成功", nil
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ga/git-update", strings.NewReader("{}"))
	s.gaGitUpdate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["strategy"] != gaGitSyncStrategy || got["synchronized"] != true || got["changed"] != true {
		t.Fatalf("unexpected autosync response: %#v", got)
	}
	if got["sync_output"] != "同步状态: 全部成功" || got["before"] != "before1" || got["after"] != "after22" {
		t.Fatalf("missing autosync result: %#v", got)
	}
}

func TestGaGitUpdateRejectsNonOriginTrackingBranch(t *testing.T) {
	root := gitSyncTestRoot(t)
	s := newConfigTestServer(t)
	s.CfgStore.Cfg.GARoot = root
	oldRunGit := runGitCommandFunc
	oldAutoSync := runGAAutoSyncFunc
	t.Cleanup(func() { runGitCommandFunc = oldRunGit; runGAAutoSyncFunc = oldAutoSync })
	runGitCommandFunc = func(_ context.Context, _ string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		if joined == "rev-parse --abbrev-ref --symbolic-full-name @{u}" {
			return "upstream/company_ga", nil
		}
		return gitSyncCleanCommand(t, joined)
	}
	runGAAutoSyncFunc = func(context.Context, string, string, string) (string, error) {
		t.Fatal("autosync must not run for non-origin tracking")
		return "", nil
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ga/git-update", strings.NewReader("{}"))
	s.gaGitUpdate(rr, req)
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "origin/company_ga") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func gitSyncTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	script := gaGitSyncScript(root)
	if err := os.MkdirAll(filepath.Dir(script), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("# test"), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}

func gitSyncCleanCommand(t *testing.T, joined string) (string, error) {
	t.Helper()
	switch joined {
	case "branch --show-current":
		return "company_ga", nil
	case "rev-parse --short HEAD":
		return "clean123", nil
	case "status --short":
		return "", nil
	case "rev-parse --abbrev-ref --symbolic-full-name @{u}":
		return "origin/company_ga", nil
	case "rev-list --left-right --count HEAD...@{u}":
		return "0 0", nil
	default:
		t.Fatalf("unexpected git command: %s", joined)
		return "", nil
	}
}
