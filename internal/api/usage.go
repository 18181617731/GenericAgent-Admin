package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type usageTotals struct {
	InputTokens  int            `json:"input_tokens"`
	OutputTokens int            `json:"output_tokens"`
	TotalTokens  int            `json:"total_tokens"`
	Other        map[string]int `json:"other,omitempty"`
}

type usageBreakdown struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	UpdatedAt        int64       `json:"updated_at,omitempty"`
	AssistantReplies int         `json:"assistant_replies"`
	Totals           usageTotals `json:"totals"`
}

type usageOverviewResponse struct {
	Totals            usageTotals      `json:"totals"`
	SessionCount      int              `json:"session_count"`
	SessionsWithUsage int              `json:"sessions_with_usage"`
	AssistantReplies  int              `json:"assistant_replies"`
	Models            []usageBreakdown `json:"models"`
	Sessions          []usageBreakdown `json:"sessions"`
	SkippedSessions   int              `json:"skipped_sessions"`
}

func addUsageValue(dst *usageTotals, key string, value int) {
	if value <= 0 {
		return
	}
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "input_tokens", "prompt_tokens", "input_token_count", "prompt_token_count":
		dst.InputTokens += value
	case "output_tokens", "completion_tokens", "output_token_count", "completion_token_count":
		dst.OutputTokens += value
	case "total_tokens", "total_token_count":
		dst.TotalTokens += value
	default:
		if dst.Other == nil {
			dst.Other = map[string]int{}
		}
		dst.Other[key] += value
	}
}

func normalizedMessageUsage(message chatMessage) (usageTotals, bool) {
	values := message.Usage
	if len(values) == 0 && len(message.Usages) > 0 {
		values = map[string]int{}
		for _, turn := range message.Usages {
			for key, value := range turn {
				values[key] += value
			}
		}
	}
	var totals usageTotals
	for key, value := range values {
		addUsageValue(&totals, key, value)
	}
	if totals.TotalTokens == 0 {
		totals.TotalTokens = totals.InputTokens + totals.OutputTokens
	}
	return totals, len(values) > 0
}

func mergeUsageTotals(dst *usageTotals, src usageTotals) {
	dst.InputTokens += src.InputTokens
	dst.OutputTokens += src.OutputTokens
	dst.TotalTokens += src.TotalTokens
	for key, value := range src.Other {
		if dst.Other == nil {
			dst.Other = map[string]int{}
		}
		dst.Other[key] += value
	}
}

func (s *Server) usageOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	dir := chatSessionDir(s.CfgStore.Cfg)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, usageOverviewResponse{Models: []usageBreakdown{}, Sessions: []usageBreakdown{}})
			return
		}
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := usageOverviewResponse{Models: []usageBreakdown{}, Sessions: []usageBreakdown{}}
	models := map[string]*usageBreakdown{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		sid := strings.TrimSuffix(entry.Name(), ".json")
		cs := chatSession{}
		contents, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err == nil {
			err = json.Unmarshal(contents, &cs)
		}
		if err != nil {
			response.SkippedSessions++
			continue
		}
		if strings.TrimSpace(cs.ID) == "" {
			cs.ID = sid
		}
		response.SessionCount++
		session := usageBreakdown{ID: cs.ID, Name: cs.Title, UpdatedAt: cs.UpdatedAt}
		if strings.TrimSpace(session.Name) == "" {
			session.Name = cs.ID
		}
		for _, message := range cs.Messages {
			if message.Role != "assistant" {
				continue
			}
			totals, ok := normalizedMessageUsage(message)
			if !ok {
				continue
			}
			response.AssistantReplies++
			session.AssistantReplies++
			mergeUsageTotals(&response.Totals, totals)
			mergeUsageTotals(&session.Totals, totals)
			modelID := strings.TrimSpace(message.ModelID)
			if modelID == "" {
				modelID = "unknown"
			}
			model := models[modelID]
			if model == nil {
				model = &usageBreakdown{ID: modelID, Name: modelID}
				models[modelID] = model
			}
			model.AssistantReplies++
			mergeUsageTotals(&model.Totals, totals)
		}
		if session.AssistantReplies > 0 {
			response.SessionsWithUsage++
			response.Sessions = append(response.Sessions, session)
		}
	}
	for _, model := range models {
		response.Models = append(response.Models, *model)
	}
	sort.Slice(response.Models, func(i, j int) bool {
		if response.Models[i].Totals.TotalTokens == response.Models[j].Totals.TotalTokens {
			return response.Models[i].Name < response.Models[j].Name
		}
		return response.Models[i].Totals.TotalTokens > response.Models[j].Totals.TotalTokens
	})
	sort.Slice(response.Sessions, func(i, j int) bool {
		return response.Sessions[i].UpdatedAt > response.Sessions[j].UpdatedAt
	})
	writeJSON(w, response)
}
