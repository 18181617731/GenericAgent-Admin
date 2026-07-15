package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	maxModelProbeCount  = 50
	modelProbeWorkers   = 3
	modelProbeTimeout   = 45 * time.Second
	modelProbeBodyLimit = 1 << 20
)

var modelProbeNow = time.Now

var modelProbeTimePattern = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}\b`)

type modelProbeRequest struct {
	Protocol     string                       `json:"protocol"`
	BaseURL      string                       `json:"base_url"`
	APIKey       string                       `json:"api_key"`
	VarName      string                       `json:"var_name"`
	Models       []string                     `json:"models"`
	ModelOptions map[string]modelProbeOptions `json:"model_options"`
}

type modelProbeResult struct {
	ID        string `json:"id"`
	Available bool   `json:"available"`
	Status    string `json:"status"`
	Detail    string `json:"detail"`
	LatencyMS int64  `json:"latency_ms"`
}

type modelProbeResponse struct {
	Results   []modelProbeResult `json:"results"`
	Available int                `json:"available"`
	Failed    int                `json:"failed"`
	CheckedAt string             `json:"checked_at"`
}

func (s *Server) modelsProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input modelProbeRequest
	if err := decode(r, &input); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	isClaude, err := modelProbeProtocol(input.Protocol)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	models := uniqueProbeModels(input.Models)
	if len(models) == 0 || len(models) > maxModelProbeCount {
		bad(w, http.StatusBadRequest, "models must contain between 1 and 50 unique model IDs")
		return
	}
	if _, err := parseModelDiscoveryBaseURL(input.BaseURL); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	apiKey, err := s.resolveModelAPIKey(input.APIKey, input.VarName)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	options := s.resolveModelProbeOptions(input.VarName, input.ModelOptions)
	checkedAt := modelProbeNow().In(time.FixedZone("Asia/Shanghai", 8*60*60))
	results := runModelProbes(r.Context(), input.BaseURL, apiKey, models, options, isClaude, checkedAt)
	response := modelProbeResponse{Results: results, CheckedAt: checkedAt.Format(time.RFC3339)}
	for _, result := range results {
		if result.Available {
			response.Available++
		} else {
			response.Failed++
		}
	}
	writeJSON(w, response)
}

func runModelProbes(ctx context.Context, baseURL, apiKey string, models []string, options map[string]modelProbeOptions, isClaude bool, now time.Time) []modelProbeResult {
	results := make([]modelProbeResult, len(models))
	jobs := make(chan int)
	workers := modelProbeWorkers
	if len(models) < workers {
		workers = len(models)
	}
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				model := models[index]
				results[index] = probeModel(ctx, baseURL, apiKey, model, options[model], isClaude, now)
			}
		}()
	}
	for index := range models {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return results
}

func probeModel(ctx context.Context, baseURL, apiKey, model string, options modelProbeOptions, isClaude bool, now time.Time) (result modelProbeResult) {
	started := time.Now()
	result = modelProbeResult{ID: model, Status: "request_failed"}
	defer func() { result.LatencyMS = time.Since(started).Milliseconds() }()
	expected := now.Format("2006-01-02 15:04")
	endpoints, err := modelProbeEndpoints(baseURL, isClaude, options.APIMode)
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	body := modelProbePayload(model, expected, isClaude, options.APIMode)
	client := &http.Client{Timeout: modelProbeTimeout}
	for _, endpoint := range endpoints {
		for _, headers := range modelDiscoveryAuthHeaders(apiKey, isClaude) {
			headers = modelProbeRequestHeaders(headers, options, isClaude)
			reply, status, err := requestModelProbe(ctx, client, endpoint, headers, body, isClaude, options.APIMode)
			if err != nil {
				result.Detail = redactProbeDetail(err.Error(), apiKey)
				continue
			}
			if status < 200 || status >= 300 {
				result.Detail = redactProbeDetail(fmt.Sprintf("HTTP %d: %s", status, reply), apiKey)
				continue
			}
			if !validModelProbeAnswer(reply, expected) {
				result.Status = "invalid_answer"
				result.Detail = "模型已响应，但未正确回答北京时间"
				return result
			}
			result.Available = true
			result.Status = "available"
			result.Detail = "真实对话验证通过：" + expected
			return result
		}
	}
	if result.Detail == "" {
		result.Detail = "模型没有返回可用响应"
	}
	return result
}

func validModelProbeAnswer(reply, expected string) bool {
	matches := modelProbeTimePattern.FindAllString(reply, -1)
	if len(matches) == 0 {
		return false
	}
	for _, match := range matches {
		if strings.Join(strings.Fields(match), " ") != expected {
			return false
		}
	}
	return true
}

func requestModelProbe(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, payload []byte, isClaude bool, apiMode string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, modelProbeBodyLimit))
	if err != nil {
		return "", resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return probeErrorMessage(body), resp.StatusCode, nil
	}
	reply, err := decodeModelProbeReply(body, isClaude, apiMode)
	return reply, resp.StatusCode, err
}

func modelProbePayload(model, expected string, isClaude bool, apiMode string) []byte {
	system := "当前服务器提供的可信北京时间是 " + expected + "（Asia/Shanghai）。必须使用这个时间回答。"
	user := "现在北京时间几点了？只回答 YYYY-MM-DD HH:mm。"
	var payload map[string]interface{}
	if isClaude {
		payload = map[string]interface{}{"model": model, "max_tokens": 64, "stream": true, "system": system, "messages": []map[string]string{{"role": "user", "content": user}}}
	} else if normalizeModelProbeAPIMode(apiMode) == "responses" {
		payload = map[string]interface{}{"model": model, "stream": true, "instructions": system, "input": user, "max_output_tokens": 64}
	} else {
		payload = map[string]interface{}{"model": model, "stream": true, "messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}}}
		modelName := strings.ToLower(model)
		if strings.HasPrefix(modelName, "gpt-5") || strings.HasPrefix(modelName, "o1") || strings.HasPrefix(modelName, "o2") || strings.HasPrefix(modelName, "o3") || strings.HasPrefix(modelName, "o4") {
			payload["max_completion_tokens"] = 64
		} else {
			payload["max_tokens"] = 64
		}
	}
	body, _ := json.Marshal(payload)
	return body
}

func modelProbeProtocol(protocol string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "", "native_oai", "oai", "openai", "openai-compatible", "chatgpt":
		return false, nil
	case "native_claude", "claude":
		return true, nil
	default:
		return false, fmt.Errorf("model probe supports official OAI and Claude protocols only")
	}
}

func modelProbeEndpoints(baseURL string, isClaude bool, apiMode string) ([]string, error) {
	u, err := parseModelDiscoveryBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	suffix := "/chat/completions"
	if isClaude {
		suffix = "/messages"
	} else if normalizeModelProbeAPIMode(apiMode) == "responses" {
		suffix = "/responses"
	}
	path := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(path, suffix) {
		u.Path = path
		return []string{u.String()}, nil
	}
	primary := *u
	primary.Path = path + suffix
	endpoints := []string{primary.String()}
	if !strings.HasSuffix(path, "/v1") {
		fallback := *u
		fallback.Path = path + "/v1" + suffix
		endpoints = append(endpoints, fallback.String())
	}
	return endpoints, nil
}

func uniqueProbeModels(models []string) []string {
	result := make([]string, 0, len(models))
	seen := map[string]bool{}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model != "" && !seen[model] {
			seen[model] = true
			result = append(result, model)
		}
	}
	return result
}

func probeErrorMessage(body []byte) string {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil && strings.TrimSpace(payload.Error.Message) != "" {
		return payload.Error.Message
	}
	return strings.TrimSpace(string(body))
}

func redactProbeDetail(detail, apiKey string) string {
	if apiKey != "" {
		detail = strings.ReplaceAll(detail, apiKey, "[redacted]")
	}
	detail = strings.TrimSpace(detail)
	if len([]rune(detail)) > 300 {
		detail = string([]rune(detail)[:300]) + "..."
	}
	return detail
}
