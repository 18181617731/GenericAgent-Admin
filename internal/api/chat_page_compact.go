package api

import (
	"regexp"
	"strings"
)

var chatCompactTurn = regexp.MustCompile(`(?i)^\s*(?:\*\*)?\s*LLM Running\s*\(Turn\s+(\d+)\)\s*(?:\.\.\.)?\s*(?:\*\*)?\s*$`)
var chatCompactFence = regexp.MustCompile("^( {0,3})(`{3,}|~{3,})(.*)$")
var chatCompactFinalInline = regexp.MustCompile("(?i)^\\s*`{3,}\\s*\\[Info\\]\\s*Final response to user\\.\\s*`{3,}\\s*$")
var chatCompactFinalFence = regexp.MustCompile("^\\s*`{3,}\\s*$")
var chatCompactFinalInfo = regexp.MustCompile(`(?i)^\s*\[Info\]\s*Final response to user\.\s*$`)

// Transport-only optimization. Mirror the frontend's top-level protocol markers;
// fenced examples are not execution history. Ambiguous/unfinished text stays whole.
func compactChatPageMessage(m chatPageMessage) chatPageMessage {
	if m.Role != "assistant" || m.Kind == "btw" || len(m.Content) < 8192 {
		return m
	}
	full := strings.ReplaceAll(strings.ReplaceAll(m.Content, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(full, "\n")
	count, offset := 0, 0
	fenceChar, fenceLength := byte(0), 0
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if fenceChar == 0 {
			if chatCompactTurn.MatchString(line) {
				count++
				offset += len(line) + 1
				continue
			}
			end := -1
			if chatCompactFinalInline.MatchString(line) {
				end = offset + len(line)
			}
			if chatCompactFinalFence.MatchString(line) && i+2 < len(lines) && chatCompactFinalInfo.MatchString(lines[i+1]) && chatCompactFinalFence.MatchString(lines[i+2]) {
				end = offset + len(line) + 1 + len(lines[i+1]) + 1 + len(lines[i+2])
			}
			if end >= 0 {
				if count == 0 || strings.TrimSpace(full[end:]) == "" {
					return m
				}
				final := full[end:]
				// Avoid expanding small messages into an extra request with little saving.
				if len(m.Content)-len(final) < 4096 {
					return m
				}
				m.Content = final
				m.Outputs = nil
				m.TaskOutputs = nil
				m.StructuredContent = nil
				m.ExecutionCount = count
				m.ContentPartial = true
				return m
			}
		}
		if match := chatCompactFence.FindStringSubmatch(line); match != nil {
			ticks, rest := match[2], match[3]
			if fenceChar == 0 {
				fenceChar, fenceLength = ticks[0], len(ticks)
			} else if ticks[0] == fenceChar && len(ticks) >= fenceLength && strings.TrimSpace(rest) == "" {
				fenceChar, fenceLength = 0, 0
			}
		}
		offset += len(line) + 1
	}
	return m
}
