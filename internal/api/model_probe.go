package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	maxModelProbeCount              = 50
	modelProbeWorkers               = 3
	modelProbeBodyLimit             = 1 << 20
	modelProbeDefaultMaxRetries     = 3
	modelProbeDefaultReadTimeout    = 300
	modelProbeDefaultConnectTimeout = 5
	modelProbeRetryBaseDelay        = 500 * time.Millisecond
	modelProbeRetryMaxDelay         = 30 * time.Second
)

var modelProbeNow = time.Now
var modelProbeRetryDelay = func(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 6 {
		attempt = 6
	}
	delay := modelProbeRetryBaseDelay * time.Duration(1<<attempt)
	if delay > modelProbeRetryMaxDelay {
		return modelProbeRetryMaxDelay
	}
	return delay
}
var modelProbeSleep = time.After

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
	usage     usageTotals
	effort    string
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
	if err := s.recordModelProbeUsage(input.VarName, results, checkedAt); err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
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

func (s *Server) recordModelProbeUsage(varName string, results []modelProbeResult, checkedAt time.Time) error {
	sessionID := fmt.Sprintf("model-probe-%d", time.Now().UnixNano())
	entries := make([]usageLedgerEntry, 0, len(results))
	for _, result := range results {
		if result.usage.TotalTokens <= 0 {
			continue
		}
		effort := strings.TrimSpace(result.effort)
		if effort == "" {
			effort = "off"
		}
		entries = append(entries, usageLedgerEntry{
			Key:             fmt.Sprintf("%s:%s", sessionID, result.ID),
			Channel:         "model_probe",
			Source:          strings.TrimSpace(varName),
			SessionID:       sessionID,
			Title:           "模型检测 · " + result.ID,
			ModelID:         result.ID,
			ReasoningEffort: effort,
			CreatedAt:       checkedAt.Unix(),
			ElapsedMS:       result.LatencyMS,
			Totals:          result.usage,
		})
	}
	return s.recordUsageEntries(entries)
}

func probeModel(ctx context.Context, baseURL, apiKey, model string, options modelProbeOptions, isClaude bool, now time.Time) (result modelProbeResult) {
	started := time.Now()
	result = modelProbeResult{ID: model, Status: "request_failed"}
	defer func() { result.LatencyMS = time.Since(started).Milliseconds() }()
	options = normalizeModelProbeOptions(options)
	expected := now.Format("2006-01-02 15:04")
	endpoints, err := modelProbeEndpoints(baseURL, isClaude, options.APIMode)
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	body := modelProbePayload(model, expected, isClaude, options)
	client := modelProbeHTTPClient(options)
	defer client.CloseIdleConnections()
	for _, endpoint := range endpoints {
		for _, headers := range modelDiscoveryAuthHeaders(apiKey, isClaude) {
			headers = modelProbeRequestHeaders(headers, options, isClaude)
			for attempt := 0; attempt <= options.MaxRetries; attempt++ {
				reply, status, requestErr := requestModelProbe(ctx, client, endpoint, headers, body, isClaude, options.APIMode)
				if requestErr != nil {
					result.Detail = redactProbeDetail(requestErr.Error(), apiKey)
					if attempt < options.MaxRetries && waitModelProbeRetry(ctx, attempt) {
						continue
					}
					break
				}
				if status < 200 || status >= 300 {
					result.Detail = redactProbeDetail(fmt.Sprintf("HTTP %d: %s", status, reply.Text), apiKey)
					if attempt < options.MaxRetries && retryableModelProbeStatus(status) && waitModelProbeRetry(ctx, attempt) {
						continue
					}
					break
				}
				result.usage = reply.Usage
				result.effort = options.ReasoningEffort
				if !validModelProbeAnswer(reply.Text, expected) {
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
	}
	if result.Detail == "" {
		result.Detail = "模型没有返回可用响应"
	}
	return result
}

func normalizeModelProbeOptions(options modelProbeOptions) modelProbeOptions {
	if options.MaxRetries < 0 {
		options.MaxRetries = 0
	}
	if !options.Configured && options.MaxRetries == 0 {
		options.MaxRetries = modelProbeDefaultMaxRetries
	}
	if options.ReadTimeout <= 0 {
		options.ReadTimeout = modelProbeDefaultReadTimeout
	}
	if options.ConnectTimeout <= 0 {
		options.ConnectTimeout = modelProbeDefaultConnectTimeout
	}
	return options
}

func modelProbeHTTPClient(options modelProbeOptions) *http.Client {
	options = normalizeModelProbeOptions(options)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: time.Duration(options.ConnectTimeout) * time.Second}).DialContext
	return &http.Client{Transport: transport, Timeout: time.Duration(options.ReadTimeout) * time.Second}
}

func retryableModelProbeStatus(status int) bool {
	return status == http.StatusConflict || status == http.StatusRequestTimeout || status == http.StatusTooEarly || status == http.StatusTooManyRequests || status >= 500
}

func waitModelProbeRetry(ctx context.Context, attempt int) bool {
	delay := modelProbeSleep(modelProbeRetryDelay(attempt))
	select {
	case <-ctx.Done():
		return false
	case <-delay:
		return true
	}
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

func requestModelProbe(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, payload []byte, isClaude bool, apiMode string) (modelProbeReply, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return modelProbeReply{}, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return modelProbeReply{}, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, modelProbeBodyLimit))
	if err != nil {
		return modelProbeReply{}, resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return modelProbeReply{Text: probeErrorMessage(body)}, resp.StatusCode, nil
	}
	reply, err := decodeModelProbeReply(body, isClaude, apiMode)
	return reply, resp.StatusCode, err
}

func modelProbePayload(model, expected string, isClaude bool, options modelProbeOptions) []byte {
	system := "当前服务器提供的可信北京时间是 " + expected + "（Asia/Shanghai）。必须使用这个时间回答。"
	user := "现在北京时间几点了？只回答 YYYY-MM-DD HH:mm。"
	var payload map[string]interface{}
	if isClaude {
		payload = map[string]interface{}{"model": model, "max_tokens": 64, "stream": true, "system": system, "messages": []map[string]string{{"role": "user", "content": user}}}
	} else if normalizeModelProbeAPIMode(options.APIMode) == "responses" {
		payload = map[string]interface{}{"model": model, "stream": true, "instructions": system, "input": user, "max_output_tokens": 64}
	} else {
		payload = map[string]interface{}{"model": model, "stream": true, "messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}}}
		if modelProbeUsesMaxCompletionTokens(model) {
			payload["max_completion_tokens"] = 64
		} else {
			payload["max_tokens"] = 64
		}
	}
	if effort := strings.TrimSpace(options.ReasoningEffort); effort != "" && effort != "off" {
		if isClaude {
			payload["output_config"] = map[string]string{"effort": effort}
		} else if normalizeModelProbeAPIMode(options.APIMode) == "responses" {
			payload["reasoning"] = map[string]string{"effort": effort}
		} else {
			payload["reasoning_effort"] = effort
		}
	}
	body, _ := json.Marshal(payload)
	return body
}

func modelProbeUsesMaxCompletionTokens(model string) bool {
	modelName := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(modelName, "gpt-5") || strings.HasPrefix(modelName, "o1") || strings.HasPrefix(modelName, "o2") || strings.HasPrefix(modelName, "o3") || strings.HasPrefix(modelName, "o4")
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
