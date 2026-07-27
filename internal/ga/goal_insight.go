package ga

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// GoalInsightEntry is one parsed timeline step from the goal session's model log.
type GoalInsightEntry struct {
	Turn int     `json:"turn,omitempty"`
	Time string  `json:"time,omitempty"`
	TS   float64 `json:"ts,omitempty"`
	Text string  `json:"text"`
}

// GoalInsight is the parsed "process timeline" of a running/finished goal.
type GoalInsight struct {
	Found     bool               `json:"found"`
	Reason    string             `json:"reason,omitempty"`
	LogFile   string             `json:"log_file,omitempty"`
	UpdatedAt float64            `json:"updated_at,omitempty"`
	Wakes     int                `json:"wakes,omitempty"`
	Total     int                `json:"total"`
	Truncated bool               `json:"truncated,omitempty"`
	Entries   []GoalInsightEntry `json:"entries"`
}

const (
	insightProbeBytes       = 8 * 1024
	insightMaxEntries       = 200
	insightMaxTextRunes     = 300
	insightMaxParseBytes    = 8 * 1024 * 1024
	insightMtimeSlackSec    = 180
	insightFallbackWindow   = 48 * time.Hour
	insightNegativeCacheTTL = 5 * time.Second
)

var (
	insightBlockRe   = regexp.MustCompile(`(?m)^=== (Prompt|Response) === (\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})[^\n]*\n`)
	insightWakeRe    = regexp.MustCompile(`第\s*(\d+)\s*次唤醒`)
	insightSummaryRe = regexp.MustCompile(`(?s)<summary>\s*(.*?)\s*</summary>`)
	insightSpaceRe   = regexp.MustCompile(`\s+`)

	insightMu    sync.Mutex
	insightCache = map[string]*goalInsightCacheItem{}
)

type goalInsightCacheItem struct {
	logPath   string
	modTime   time.Time
	size      int64
	scannedAt time.Time
	payload   GoalInsight
}

// GoalInsightByState locates the model_responses session log belonging to the
// goal described by stateFile and parses per-turn <summary> snapshots from it.
// stateFile must resolve to a known goal state file under root (whitelist).
func GoalInsightByState(root, stateFile string) (GoalInsight, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return GoalInsight{}, errors.New("ga root not configured")
	}
	stateFile = strings.TrimSpace(stateFile)
	if stateFile == "" {
		return GoalInsight{}, errors.New("state_file is required")
	}
	if !filepath.IsAbs(stateFile) {
		stateFile = filepath.Join(root, stateFile)
	}
	stateFile = filepath.Clean(stateFile)
	known, err := goalStateFiles(root)
	if err != nil {
		return GoalInsight{}, err
	}
	match := ""
	for _, p := range known {
		if strings.EqualFold(filepath.Clean(p), stateFile) {
			match = filepath.Clean(p)
			break
		}
	}
	if match == "" {
		return GoalInsight{}, errors.New("unknown goal state file")
	}
	state, _, err := readGoalState(match)
	if err != nil {
		return GoalInsight{}, fmt.Errorf("read goal state: %w", err)
	}

	insightMu.Lock()
	defer insightMu.Unlock()
	now := time.Now()
	cacheKey := strings.ToLower(match)
	item := insightCache[cacheKey]
	if item != nil {
		if item.logPath != "" {
			if st, err := os.Stat(item.logPath); err == nil && st.ModTime().Equal(item.modTime) && st.Size() == item.size {
				return item.payload, nil
			}
		} else if now.Sub(item.scannedAt) < insightNegativeCacheTTL {
			return item.payload, nil
		}
	}
	logPath := ""
	if item != nil && item.logPath != "" {
		if _, err := os.Stat(item.logPath); err == nil {
			logPath = item.logPath
		}
	}
	if logPath == "" {
		logPath = findGoalSessionLog(root, state)
	}
	if logPath == "" {
		payload := GoalInsight{Found: false, Reason: "session log not found", Entries: []GoalInsightEntry{}}
		insightCache[cacheKey] = &goalInsightCacheItem{scannedAt: now, payload: payload}
		return payload, nil
	}
	st, err := os.Stat(logPath)
	if err != nil {
		return GoalInsight{}, fmt.Errorf("stat session log: %w", err)
	}
	payload, err := parseGoalSessionLog(logPath, insightMaxEntries)
	if err != nil {
		return GoalInsight{}, err
	}
	payload.LogFile = relGoalPath(root, logPath)
	payload.UpdatedAt = float64(st.ModTime().UnixNano()) / 1e9
	insightCache[cacheKey] = &goalInsightCacheItem{logPath: logPath, modTime: st.ModTime(), size: st.Size(), scannedAt: now, payload: payload}
	return payload, nil
}

// goalObjectiveNeedle returns the longest objective line, JSON-escaped the same
// way agentmain dumps prompts into model_responses logs (json string escaping).
func goalObjectiveNeedle(objective string) string {
	objective = strings.ReplaceAll(strings.TrimSpace(objective), "\r\n", "\n")
	best := ""
	for _, line := range strings.Split(objective, "\n") {
		line = strings.TrimSpace(line)
		if utf8.RuneCountInString(line) > utf8.RuneCountInString(best) {
			best = line
		}
	}
	if best == "" {
		return ""
	}
	if runes := []rune(best); len(runes) > 200 {
		best = string(runes[:200])
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(best); err != nil {
		return best
	}
	s := strings.TrimSpace(buf.String())
	s = strings.TrimPrefix(s, `"`)
	s = strings.TrimSuffix(s, `"`)
	return s
}

// findGoalSessionLog scans root/temp/model_responses for the newest session log
// that (a) is not older than the goal start, (b) is a goal-mode session, and
// (c) mentions the goal objective in its head.
func findGoalSessionLog(root string, state GoalState) string {
	dir := filepath.Join(root, "temp", "model_responses")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	needle := goalObjectiveNeedle(state.Objective)
	var cutoff time.Time
	if state.StartTime > 0 {
		cutoff = time.Unix(0, int64(state.StartTime*1e9)).Add(-insightMtimeSlackSec * time.Second)
	} else {
		cutoff = time.Now().Add(-insightFallbackWindow)
	}
	bestPath := ""
	var bestMod time.Time
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "model_responses_") || !strings.HasSuffix(name, ".txt") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().Before(cutoff) {
			continue
		}
		head, err := readFileHead(filepath.Join(dir, name), insightProbeBytes)
		if err != nil {
			continue
		}
		if !strings.Contains(head, "[Goal Mode") {
			continue
		}
		if needle != "" && !strings.Contains(head, needle) {
			continue
		}
		if bestPath == "" || info.ModTime().After(bestMod) {
			bestPath = filepath.Join(dir, name)
			bestMod = info.ModTime()
		}
	}
	return bestPath
}

func readFileHead(path string, n int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, n)
	m, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", err
	}
	return string(buf[:m]), nil
}

// parseGoalSessionLog extracts the per-turn timeline: wake numbers from goal
// prompts and <summary> snapshots from model responses.
func parseGoalSessionLog(path string, limit int) (GoalInsight, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return GoalInsight{}, err
	}
	if len(raw) > insightMaxParseBytes {
		raw = raw[len(raw)-insightMaxParseBytes:]
	}
	text := string(raw)
	matches := insightBlockRe.FindAllStringSubmatchIndex(text, -1)
	entries := make([]GoalInsightEntry, 0, 32)
	wakes := 0
	turn := 0
	for i, m := range matches {
		kind := text[m[2]:m[3]]
		stamp := text[m[4]:m[5]]
		bodyStart := m[1]
		bodyEnd := len(text)
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0]
		}
		body := text[bodyStart:bodyEnd]
		if kind == "Prompt" {
			gate := body
			if len(gate) > 400 {
				gate = gate[:400]
			}
			if strings.Contains(gate, "[Goal Mode") {
				if wm := insightWakeRe.FindStringSubmatch(body); wm != nil {
					if n, err := strconv.Atoi(wm[1]); err == nil && n > 0 {
						turn = n
						if n > wakes {
							wakes = n
						}
					}
				}
			}
			continue
		}
		sm := insightSummaryRe.FindStringSubmatch(body)
		if sm == nil {
			continue
		}
		step := strings.TrimSpace(sm[1])
		step = strings.ReplaceAll(step, `\n`, " ")
		step = strings.ReplaceAll(step, `\"`, `"`)
		step = insightSpaceRe.ReplaceAllString(step, " ")
		if step == "" {
			continue
		}
		if runes := []rune(step); len(runes) > insightMaxTextRunes {
			step = string(runes[:insightMaxTextRunes]) + "…"
		}
		ts := 0.0
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", stamp, time.Local); err == nil {
			ts = float64(t.Unix())
		}
		hhmmss := stamp
		if len(stamp) >= 19 {
			hhmmss = stamp[11:]
		}
		entries = append(entries, GoalInsightEntry{Turn: turn, Time: hhmmss, TS: ts, Text: step})
	}
	total := len(entries)
	truncated := false
	if limit > 0 && total > limit {
		entries = entries[total-limit:]
		truncated = true
	}
	return GoalInsight{Found: true, Wakes: wakes, Total: total, Truncated: truncated, Entries: entries}, nil
}
