package api

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Subagent status card backend.
//
// Convention (memory/subagent.md): a subagent launched via
// `agentmain.py --task {name}` works in {GARoot}/temp/{name}/ with:
//   input.txt / input{N}.txt   - prompts from the parent agent
//   output.txt / output{N}.txt - streaming output, tail "[ROUND END]" when a round finishes
//   reply.txt                  - subagent's condensed reply for the round
//   _stop / _intervene / _keyinfo / expected.md - control & contract files
//
// The chat session transcript contains the launch command text, so task
// names are recovered by scanning message content for `--task {name}`.

var subagentTaskNameRe = regexp.MustCompile(`--task[\s=]+"?([A-Za-z0-9][A-Za-z0-9._\-]*)`)
var subagentOutputFileRe = regexp.MustCompile(`^output(\d*)\.txt$`)

type subagentStatus struct {
	Name          string `json:"name"`
	Dir           string `json:"dir"`
	Exists        bool   `json:"exists"`
	Rounds        int    `json:"rounds"`
	RoundEnded    bool   `json:"round_ended"`
	LatestSummary string `json:"latest_summary,omitempty"`
	HasReply      bool   `json:"has_reply"`
	HasExpected   bool   `json:"has_expected"`
	StopRequested bool   `json:"stop_requested"`
	Intervened    bool   `json:"intervened"`
	UpdatedAt     int64  `json:"updated_at"` // unix ms of latest output/reply mtime, 0 if unknown
}

// extractSubagentTaskNames scans chat messages for `--task {name}` launch
// commands and returns unique names in first-seen order.
func extractSubagentTaskNames(msgs []chatMessage) []string {
	seen := map[string]bool{}
	var names []string
	for _, m := range msgs {
		if m.Role != "assistant" && m.Role != "user" {
			continue
		}
		for _, g := range subagentTaskNameRe.FindAllStringSubmatch(m.Content, -1) {
			name := g[1]
			if name == "" || strings.Contains(name, "..") || seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// latestSubagentOutput returns the highest-numbered output*.txt in dir
// (output.txt < output1.txt < output2.txt ...) plus the total count.
func latestSubagentOutput(dir string) (path string, count int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", 0
	}
	type of struct {
		n    int
		name string
	}
	var outs []of
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := subagentOutputFileRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n := 0
		if m[1] != "" {
			n, _ = strconv.Atoi(m[1])
		}
		outs = append(outs, of{n, e.Name()})
	}
	if len(outs) == 0 {
		return "", 0
	}
	sort.Slice(outs, func(i, j int) bool { return outs[i].n < outs[j].n })
	return filepath.Join(dir, outs[len(outs)-1].name), len(outs)
}

// readTail reads at most max bytes from the end of path.
func readTail(path string, max int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	off := int64(0)
	if st.Size() > max {
		off = st.Size() - max
	}
	buf := make([]byte, st.Size()-off)
	n, _ := f.ReadAt(buf, off)
	return string(buf[:n])
}

var subagentSummaryRe = regexp.MustCompile(`(?s)<summary>(.*?)</summary>`)

func collectSubagentStatus(root, name string) subagentStatus {
	dir := filepath.Join(root, "temp", name)
	st := subagentStatus{Name: name, Dir: dir}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return st
	}
	st.Exists = true
	exists := func(fn string) bool {
		_, err := os.Stat(filepath.Join(dir, fn))
		return err == nil
	}
	st.HasReply = exists("reply.txt")
	st.HasExpected = exists("expected.md")
	st.StopRequested = exists("_stop")
	st.Intervened = exists("_intervene")
	latest, count := latestSubagentOutput(dir)
	st.Rounds = count
	if latest != "" {
		if ofi, err := os.Stat(latest); err == nil {
			st.UpdatedAt = ofi.ModTime().UnixMilli()
		}
		tail := readTail(latest, 64*1024)
		st.RoundEnded = strings.Contains(tail, "[ROUND END]")
		if sums := subagentSummaryRe.FindAllStringSubmatch(tail, -1); len(sums) > 0 {
			s := strings.TrimSpace(sums[len(sums)-1][1])
			if len(s) > 300 {
				s = s[:300]
			}
			st.LatestSummary = s
		}
	}
	return st
}

// chatSubagents serves GET /api/chat/subagents/{sid}: status of subagent
// task dirs referenced by the session transcript.
func (s *Server) chatSubagents(w http.ResponseWriter, r *http.Request, sid string) {
	cs, err := loadChatSession(s.CfgStore.Cfg, sid)
	if err != nil {
		bad(w, 404, "session not found")
		return
	}
	root := strings.TrimSpace(s.CfgStore.Cfg.GARoot)
	out := []subagentStatus{}
	if root != "" {
		for _, name := range extractSubagentTaskNames(cs.Messages) {
			st := collectSubagentStatus(root, name)
			if st.Exists {
				out = append(out, st)
			}
		}
	}
	writeJSON(w, map[string]interface{}{"subagents": out})
}
