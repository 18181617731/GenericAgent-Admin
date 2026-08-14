package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxChatSearchQueryLength = 160
	defaultChatSearchLimit   = 40
	maxChatSearchLimit       = 100
)

var chatSearchScopes = map[string]bool{
	"all":     true,
	"title":   true,
	"content": true,
	"project": true,
}

type chatSearchResult struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	UpdatedAt   int64    `json:"updated_at"`
	Count       int      `json:"count"`
	Project     string   `json:"project,omitempty"`
	Workspace   string   `json:"workspace,omitempty"`
	ProjectMode string   `json:"project_mode,omitempty"`
	Archived    bool     `json:"archived"`
	Snippet     string   `json:"snippet,omitempty"`
	MatchType   string   `json:"match_type"`
	MatchTypes  []string `json:"match_types"`
}

func (s *Server) chatSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, map[string]interface{}{"results": []chatSearchResult{}, "query": "", "scope": "all"})
		return
	}
	if utf8.RuneCountInString(query) > maxChatSearchQueryLength {
		bad(w, http.StatusBadRequest, "search query is too long")
		return
	}
	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	if scope == "" {
		scope = "all"
	}
	if !chatSearchScopes[scope] {
		bad(w, http.StatusBadRequest, "invalid search scope")
		return
	}
	limit := defaultChatSearchLimit
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 {
			bad(w, http.StatusBadRequest, "invalid search limit")
			return
		}
		limit = parsed
		if limit > maxChatSearchLimit {
			limit = maxChatSearchLimit
		}
	}
	if err := ensureChatDataMigrated(s.CfgStore.Snapshot()); err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.MkdirAll(chatSessionDir(s.CfgStore.Snapshot()), 0755); err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	entries, err := os.ReadDir(chatSessionDir(s.CfgStore.Snapshot()))
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	terms := chatSearchTerms(query)
	results := make([]chatSearchResult, 0, limit)
	s.SessionMu.Lock()
	defer s.SessionMu.Unlock()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		cs, loadErr := loadChatSession(s.CfgStore.Snapshot(), strings.TrimSuffix(entry.Name(), ".json"))
		if loadErr != nil {
			continue
		}
		result, ok := matchChatSearchSession(cs, terms, scope)
		if ok {
			results = append(results, result)
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].UpdatedAt != results[j].UpdatedAt {
			return results[i].UpdatedAt > results[j].UpdatedAt
		}
		return strings.ToLower(results[i].Title) < strings.ToLower(results[j].Title)
	})
	if len(results) > limit {
		results = results[:limit]
	}
	writeJSON(w, map[string]interface{}{"results": results, "query": query, "scope": scope, "total": len(results)})
}

func chatSearchTerms(query string) []string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	terms := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		terms = append(terms, field)
	}
	return terms
}

func chatSearchMatches(value string, terms []string) bool {
	lower := strings.ToLower(value)
	for _, term := range terms {
		if !strings.Contains(lower, term) {
			return false
		}
	}
	return len(terms) > 0
}

func matchChatSearchSession(cs chatSession, terms []string, scope string) (chatSearchResult, bool) {
	titleMatch := scopeAllowsChatSearch(scope, "title") && chatSearchMatches(cs.Title, terms)
	projectText := strings.TrimSpace(strings.Join([]string{cs.ProjectMode, cs.Workspace}, " "))
	projectMatch := scopeAllowsChatSearch(scope, "project") && chatSearchMatches(projectText, terms)
	contentText := chatSearchContent(cs.Messages)
	contentMatch := scopeAllowsChatSearch(scope, "content") && chatSearchMatches(contentText, terms)
	if !titleMatch && !projectMatch && !contentMatch {
		return chatSearchResult{}, false
	}
	matchTypes := make([]string, 0, 3)
	if titleMatch {
		matchTypes = append(matchTypes, "title")
	}
	if contentMatch {
		matchTypes = append(matchTypes, "content")
	}
	if projectMatch {
		matchTypes = append(matchTypes, "project")
	}
	project := strings.TrimSpace(cs.ProjectMode)
	if project == "" {
		project = filepath.Base(strings.TrimRight(strings.TrimSpace(cs.Workspace), `/\\`))
		if project == "." {
			project = ""
		}
	}
	result := chatSearchResult{
		ID:          cs.ID,
		Title:       cs.Title,
		UpdatedAt:   cs.UpdatedAt,
		Count:       len(cs.Messages),
		Project:     project,
		Workspace:   cs.Workspace,
		ProjectMode: cs.ProjectMode,
		Archived:    cs.Archived,
		MatchType:   matchTypes[0],
		MatchTypes:  matchTypes,
	}
	if contentMatch {
		result.Snippet = chatSearchSnippet(contentText, terms)
	} else if projectMatch {
		result.Snippet = project
	}
	return result, true
}

func scopeAllowsChatSearch(scope, field string) bool {
	return scope == "all" || scope == field
}

func chatSearchContent(messages []chatMessage) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		if text := strings.TrimSpace(message.Content); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func chatSearchSnippet(content string, terms []string) string {
	compact := strings.Join(strings.Fields(content), " ")
	if compact == "" {
		return ""
	}
	lower := strings.ToLower(compact)
	runes := []rune(compact)
	matchAt := -1
	matchLength := 0
	for _, term := range terms {
		if index := strings.Index(lower, term); index >= 0 {
			runeIndex := utf8.RuneCountInString(lower[:index])
			if matchAt < 0 || runeIndex < matchAt {
				matchAt = runeIndex
				matchLength = utf8.RuneCountInString(term)
			}
		}
	}
	if matchAt < 0 {
		return chatSearchTrimRunes(compact, 180)
	}
	start := matchAt - 70
	if start < 0 {
		start = 0
	}
	end := matchAt + matchLength + 110
	if end > len(runes) {
		end = len(runes)
	}
	for start > 0 && runes[start] != ' ' {
		start--
	}
	for end < len(runes) && runes[end-1] != ' ' {
		end++
	}
	snippet := strings.TrimSpace(string(runes[start:end]))
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(runes) {
		snippet += "…"
	}
	return chatSearchTrimRunes(snippet, 220)
}

func chatSearchTrimRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit])) + "…"
}
