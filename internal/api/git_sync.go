package api

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	gaGitSyncStrategy = "daily_git_pull_merge_push"
	maxGitSyncOutput  = 24 << 10
)

var runGAAutoSyncFunc = runGAAutoSync

func gaGitSyncScript(root string) string {
	return filepath.Join(root, "sche_tasks", "git_autosync.py")
}

func gaGitSyncAvailable(root string) bool {
	info, err := os.Stat(gaGitSyncScript(root))
	return err == nil && !info.IsDir()
}

func runGAAutoSync(ctx context.Context, python, script, root string) (string, error) {
	python = strings.TrimSpace(python)
	if python == "" {
		python = "python"
	}
	cmd := exec.CommandContext(ctx, python, "-X", "utf8", script, "--repo", root)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "PYTHONUTF8=1")
	hideChildWindow(cmd)
	out, err := cmd.CombinedOutput()
	text := limitGitSyncOutput(strings.TrimSpace(string(out)))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, fmt.Errorf("%s sync failed: %w", gaGitSyncStrategy, err)
	}
	return text, nil
}

func limitGitSyncOutput(output string) string {
	if len(output) <= maxGitSyncOutput {
		return output
	}
	return output[:maxGitSyncOutput] + "\n... 输出已截断"
}

func gitStatusConflict(status string) bool {
	for _, line := range strings.Split(status, "\n") {
		if len(line) < 2 {
			continue
		}
		xy := line[:2]
		if strings.Contains(xy, "U") || xy == "AA" || xy == "DD" {
			return true
		}
	}
	return false
}

func gitChangedFileCount(status string) int {
	count := 0
	for _, line := range strings.Split(status, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
