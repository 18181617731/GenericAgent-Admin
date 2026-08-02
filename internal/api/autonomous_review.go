package api

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/ga"
	"genericagent-admin-go/internal/modelconfig"
)

type autonomousReviewModel struct {
	LLMNo       int
	Provider    string
	Model       string
	DisplayName string
}

type autonomousModelReview struct {
	Decision   string
	Confidence string
	Reason     string
	Err        string
}

type autonomousReviewModelOption struct {
	autonomousReviewModel
	order    int
	sequence int
}

const maxAutonomousModelReviewReports = 8

const autonomousReviewQuickWait = 1200 * time.Millisecond

const autonomousReviewWorkerTimeout = 18 * time.Second

const autonomousReviewRetryDelay = 30 * time.Second

var (
	autonomousReviewCacheMu       sync.Mutex
	autonomousReviewCache         = map[string]autonomousModelReview{}
	autonomousReviewInFlight      = map[string]bool{}
	autonomousReviewRetryAfter    = map[string]time.Time{}
	runAutonomousReviewWorkerFunc = func(cfg config.AppConfig, sid string, req map[string]interface{}) (chatMessage, error) {
		return runOneShotBTWWorkerFunc(cfg, sid, req)
	}
)

func (s *Server) reviewAutonomousApprovals(overview *ga.AutonomousApprovalOverview) {
	if s == nil || overview == nil || s.CfgStore == nil {
		return
	}
	model, ok := s.autonomousReviewModel()
	if !ok {
		overview.ReviewStatus = "not_configured"
		return
	}
	modelNo := model.LLMNo
	overview.ReviewModelNo = &modelNo
	overview.ReviewModel = model.DisplayName
	if overview.ReviewModel == "" {
		overview.ReviewModel = model.Model
	}
	overview.ReviewProvider = model.Provider
	overview.ReviewStatus = "fallback"
	pending := make([]int, 0)
	for index := range overview.Items {
		item := &overview.Items[index]
		if item.State != "pending" {
			continue
		}
		item.ReviewModelNo = &modelNo
		item.ReviewModel = model.DisplayName
		if item.ReviewModel == "" {
			item.ReviewModel = model.Model
		}
		item.ReviewProvider = model.Provider
		if item.ReviewReport == nil {
			if item.ReviewStatus == "" {
				item.ReviewStatus = "rule_fallback"
			}
			continue
		}
		pending = append(pending, index)
	}
	if len(pending) > maxAutonomousModelReviewReports {
		for _, index := range pending[maxAutonomousModelReviewReports:] {
			overview.Items[index].ReviewStatus = "fallback"
			overview.Items[index].ReviewReason = strings.TrimSpace(strings.Join([]string{overview.Items[index].ReviewReason, "model review batch limit reached; conservative rule retained"}, "; "))
		}
		pending = pending[:maxAutonomousModelReviewReports]
	}
	results := s.autonomousModelReviews(model, overview.Items, pending)
	hadModelResult := false
	hadModelPending := false
	hadModelUnavailable := false
	for _, index := range pending {
		item := &overview.Items[index]
		result := results[item.ID]
		if result.Err != "" {
			if strings.Contains(result.Err, "scheduled") || strings.Contains(result.Err, "in progress") {
				hadModelPending = true
			} else if strings.Contains(result.Err, "unavailable") || strings.Contains(result.Err, "timed out") {
				hadModelUnavailable = true
			}
			item.ReviewStatus = "fallback"
			item.ReviewReason = strings.TrimSpace(strings.Join([]string{item.ReviewReason, "model review unavailable: " + result.Err}, "; "))
			continue
		}
		if result.Decision != "" {
			item.ReviewDecision = result.Decision
		}
		if result.Confidence != "" {
			item.ReviewConfidence = result.Confidence
		}
		if result.Reason != "" {
			item.ReviewReason = result.Reason
		}
		item.ReviewStatus = "model_reviewed"
		hadModelResult = true
	}
	if hadModelResult {
		overview.ReviewStatus = "model_reviewed"
	} else if hadModelPending {
		overview.ReviewStatus = "model_review_pending"
	} else if hadModelUnavailable {
		overview.ReviewStatus = "model_review_fallback"
	}
}

func (s *Server) autonomousReviewModel() (autonomousReviewModel, bool) {
	if s == nil || s.CfgStore == nil {
		return autonomousReviewModel{}, false
	}
	cfg := s.CfgStore.Cfg
	preferred := 0
	if cfg.ServiceModels != nil {
		if value, exists := cfg.ServiceModels["reflect/autonomous.py"]; exists && value >= 0 {
			preferred = value
		}
	}
	draft, err := s.loadModelsFromOfficialMyKey(false)
	if err != nil {
		return autonomousReviewModel{}, false
	}
	options := orderedAutonomousReviewModelOptions(draft.Profiles)
	if len(options) == 0 {
		return autonomousReviewModel{}, false
	}
	if preferred >= len(options) {
		preferred = 0
	}
	selected := options[preferred]
	selected.LLMNo = preferred
	return selected.autonomousReviewModel, true
}

func orderedAutonomousReviewModelOptions(profiles []modelconfig.Profile) []autonomousReviewModelOption {
	options := make([]autonomousReviewModelOption, 0)
	sequence := 0
	for _, profile := range profiles {
		configs := profile.ModelConfigs
		if len(configs) == 0 {
			models := profile.Models
			if len(models) == 0 && strings.TrimSpace(profile.Model) != "" {
				models = []string{profile.Model}
			}
			configs = make([]modelconfig.ModelConfig, 0, len(models))
			for _, model := range models {
				configs = append(configs, modelconfig.ModelConfig{Model: model})
			}
		}
		for _, config := range configs {
			if !modelconfig.ModelConfigEnabled(config) || strings.TrimSpace(config.Model) == "" {
				continue
			}
			order := math.MaxInt
			if config.SortOrder != nil {
				order = *config.SortOrder
			}
			model := strings.TrimSpace(config.Model)
			display := strings.TrimSpace(config.Name)
			if display == "" {
				display = model
			}
			options = append(options, autonomousReviewModelOption{
				autonomousReviewModel: autonomousReviewModel{
					Provider:    chatProviderDisplayName(profile),
					Model:       model,
					DisplayName: display,
				},
				order: order, sequence: sequence,
			})
			sequence++
		}
	}
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].order != options[j].order {
			return options[i].order < options[j].order
		}
		return options[i].sequence < options[j].sequence
	})
	return options
}

func (s *Server) autonomousModelReviews(model autonomousReviewModel, items []ga.AutonomousApproval, indexes []int) map[string]autonomousModelReview {
	results := make(map[string]autonomousModelReview, len(indexes))
	if s == nil || s.CfgStore == nil {
		return results
	}
	uncached := make([]int, 0, len(indexes))
	texts := make([]string, 0, len(indexes))
	keys := make(map[int]string, len(indexes))
	for _, index := range indexes {
		item := items[index]
		if item.ReviewReport == nil {
			results[item.ID] = autonomousModelReview{Err: "report context is unavailable"}
			continue
		}
		key := strings.Join([]string{s.CfgStore.Cfg.GARoot, fmt.Sprint(model.LLMNo), item.ReviewReport.Path, item.ReviewReport.ModTime.UTC().Format(time.RFC3339Nano)}, "|")
		keys[index] = key
		autonomousReviewCacheMu.Lock()
		cached, ok := autonomousReviewCache[key]
		inFlight := autonomousReviewInFlight[key]
		retryAfter := autonomousReviewRetryAfter[key]
		autonomousReviewCacheMu.Unlock()
		if ok {
			results[item.ID] = cached
			continue
		}
		if inFlight {
			results[item.ID] = autonomousModelReview{Err: "model review in progress; conservative rule retained"}
			continue
		}
		if !retryAfter.IsZero() && time.Now().Before(retryAfter) {
			results[item.ID] = autonomousModelReview{Err: "model review unavailable; retry scheduled; conservative rule retained"}
			continue
		}
		path, _, err := ga.SafeResolve(s.CfgStore.Cfg.GARoot, item.ReviewReport.Path)
		if err != nil {
			results[item.ID] = autonomousModelReview{Err: err.Error()}
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			results[item.ID] = autonomousModelReview{Err: err.Error()}
			continue
		}
		text := string(content)
		if len([]rune(text)) > 5000 {
			text = string([]rune(text)[:5000]) + "\n[report truncated]"
		}
		uncached = append(uncached, index)
		texts = append(texts, item.ID+"\nTITLE: "+item.Title+"\nREPORT:\n"+text)
	}
	if len(uncached) == 0 {
		return results
	}
	if _, err := os.Stat(filepath.Join(s.CfgStore.Cfg.GARoot, "agentmain.py")); err != nil {
		for _, index := range uncached {
			results[items[index].ID] = autonomousModelReview{Err: "GA runtime is not available"}
		}
		return results
	}
	batchKeys := make([]string, 0, len(uncached))
	for _, index := range uncached {
		batchKeys = append(batchKeys, keys[index])
	}
	batchKey := strings.Join(batchKeys, "\x00")
	autonomousReviewCacheMu.Lock()
	if autonomousReviewInFlight[batchKey] {
		autonomousReviewCacheMu.Unlock()
		for _, index := range uncached {
			results[items[index].ID] = autonomousModelReview{Err: "model review in progress; conservative rule retained"}
		}
		return results
	}
	autonomousReviewInFlight[batchKey] = true
	for _, key := range batchKeys {
		autonomousReviewInFlight[key] = true
	}
	autonomousReviewCacheMu.Unlock()

	prompt := "Review these autonomous-evolution reports for approval gating. Do not approve, reject, modify files, or execute anything. Determine only whether a human approval is still required. Return JSON only as an array of objects with id, decision (needs_approval|not_required|uncertain), confidence (high|medium|low), and a short reason. If approval evidence is missing, the task is blocked, or the source change is not confirmed, choose needs_approval.\n\n" + strings.Join(texts, "\n\n--- REPORT ---\n\n")
	done := make(chan map[string]autonomousModelReview, 1)
	go func() {
		batchResults := make(map[string]autonomousModelReview, len(uncached))
		workerResult := make(chan struct {
			message chatMessage
			err     error
		}, 1)
		go func() {
			message, err := runAutonomousReviewWorkerFunc(s.CfgStore.Cfg, "autonomous-review", map[string]interface{}{
				"op":         "btw",
				"prompt":     "/btw " + prompt,
				"llm_no":     model.LLMNo,
				"ga_root":    s.CfgStore.Cfg.GARoot,
				"timeout_ms": 15000,
			})
			workerResult <- struct {
				message chatMessage
				err     error
			}{message: message, err: err}
		}()
		var message chatMessage
		var err error
		select {
		case result := <-workerResult:
			message, err = result.message, result.err
		case <-time.After(autonomousReviewWorkerTimeout):
			err = fmt.Errorf("model review timed out after %s", autonomousReviewWorkerTimeout.Round(time.Second))
		}
		if err != nil {
			for _, index := range uncached {
				batchResults[items[index].ID] = autonomousModelReview{Err: err.Error()}
			}
		} else {
			parsed := parseAutonomousModelReviews(message.Content)
			for _, index := range uncached {
				item := items[index]
				result, ok := parsed[item.ID]
				if !ok {
					result = autonomousModelReview{Err: "model omitted this report"}
				}
				batchResults[item.ID] = result
			}
		}
		autonomousReviewCacheMu.Lock()
		for _, index := range uncached {
			item := items[index]
			result := batchResults[item.ID]
			if result.Err == "" {
				if key := keys[index]; key != "" {
					autonomousReviewCache[key] = result
					autonomousReviewRetryAfter = withoutAutonomousReviewMapKey(autonomousReviewRetryAfter, key)
				}
			} else if key := keys[index]; key != "" {
				autonomousReviewRetryAfter[key] = time.Now().Add(autonomousReviewRetryDelay)
			}
		}
		autonomousReviewInFlight = withoutAutonomousReviewMapKey(autonomousReviewInFlight, batchKey)
		for _, key := range batchKeys {
			autonomousReviewInFlight = withoutAutonomousReviewMapKey(autonomousReviewInFlight, key)
		}
		autonomousReviewCacheMu.Unlock()
		done <- batchResults
	}()
	timer := time.NewTimer(autonomousReviewQuickWait)
	defer timer.Stop()
	select {
	case batchResults := <-done:
		for _, index := range uncached {
			results[items[index].ID] = batchResults[items[index].ID]
		}
	case <-timer.C:
		for _, index := range uncached {
			results[items[index].ID] = autonomousModelReview{Err: "model review scheduled; conservative rule retained"}
		}
	}
	return results
}

func withoutAutonomousReviewMapKey[K comparable, V any](values map[K]V, key K) map[K]V {
	if _, ok := values[key]; !ok {
		return values
	}
	result := make(map[K]V, len(values)-1)
	for candidate, value := range values {
		if candidate != key {
			result[candidate] = value
		}
	}
	return result
}

func parseAutonomousModelReviews(content string) map[string]autonomousModelReview {
	result := make(map[string]autonomousModelReview)
	content = strings.TrimSpace(content)
	for _, start := range []byte{'[', '{'} {
		if index := strings.IndexByte(content, start); index >= 0 {
			decoder := json.NewDecoder(strings.NewReader(content[index:]))
			if start == '[' {
				var list []struct {
					ID string `json:"id"`
					autonomousModelReview
				}
				if err := decoder.Decode(&list); err == nil && len(list) > 0 {
					for _, item := range list {
						if strings.TrimSpace(item.ID) != "" {
							result[item.ID] = normalizeAutonomousModelReview(item.autonomousModelReview)
						}
					}
					return result
				}
				continue
			}
			var envelope struct {
				ID      string `json:"id"`
				Reviews []struct {
					ID string `json:"id"`
					autonomousModelReview
				} `json:"reviews"`
				autonomousModelReview
			}
			if err := decoder.Decode(&envelope); err == nil {
				if len(envelope.Reviews) > 0 {
					for _, item := range envelope.Reviews {
						if strings.TrimSpace(item.ID) != "" {
							result[item.ID] = normalizeAutonomousModelReview(item.autonomousModelReview)
						}
					}
					return result
				}
				if strings.TrimSpace(envelope.ID) != "" {
					result[envelope.ID] = normalizeAutonomousModelReview(envelope.autonomousModelReview)
					return result
				}
			}
		}
	}
	return result
}

func normalizeAutonomousModelReview(result autonomousModelReview) autonomousModelReview {
	result.Decision = strings.ToLower(strings.TrimSpace(result.Decision))
	switch result.Decision {
	case "needs_approval", "not_required", "uncertain":
	default:
		result.Decision = "uncertain"
	}
	result.Confidence = strings.ToLower(strings.TrimSpace(result.Confidence))
	switch result.Confidence {
	case "high", "medium", "low":
	default:
		result.Confidence = "low"
	}
	result.Reason = strings.TrimSpace(result.Reason)
	if len([]rune(result.Reason)) > 500 {
		result.Reason = string([]rune(result.Reason)[:500])
	}
	return result
}
