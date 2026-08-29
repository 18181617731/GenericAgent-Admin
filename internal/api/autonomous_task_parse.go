package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type autonomousTaskDraft struct {
	Title     string `json:"title"`
	Objective string `json:"objective"`
	Priority  string `json:"priority"`
	Risk      string `json:"risk"`
	Project   string `json:"project"`
	NextStep  string `json:"next_step"`
}

func (s *Server) parseAutonomousTaskInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request struct {
		Input string `json:"input"`
	}
	if err := decode(r, &request); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	input := strings.TrimSpace(request.Input)
	if input == "" {
		bad(w, http.StatusBadRequest, "input is required")
		return
	}
	if len([]rune(input)) > 4000 {
		bad(w, http.StatusBadRequest, "input is too long")
		return
	}
	model, modelErr := s.resolveAutonomousReviewModel()
	if modelErr == nil {
		reply, requestErr := runAutonomousTaskParseRequest(context.Background(), model, input)
		if requestErr == nil {
			draft, parseErr := parseAutonomousTaskDraft(reply.Text, input)
			if parseErr == nil {
				writeJSON(w, map[string]interface{}{"parsed": draft, "model": autonomousReviewModelName(model), "fallback": false})
				return
			}
		}
	}
	writeJSON(w, map[string]interface{}{"parsed": fallbackAutonomousTaskDraft(input), "fallback": true})
}

func fallbackAutonomousTaskDraft(input string) autonomousTaskDraft {
	title := input
	if runes := []rune(title); len(runes) > 200 {
		title = string(runes[:200])
	}
	return autonomousTaskDraft{Title: title, Objective: input, Priority: "normal"}
}

func runAutonomousTaskParseRequest(ctx context.Context, model autonomousReviewModel, input string) (modelProbeReply, error) {
	options := normalizeModelProbeOptions(model.Options)
	isClaude := isClaudeModel(model.Profile.Type)
	endpoints, err := modelProbeEndpoints(model.Profile.APIBase, isClaude, options.APIMode)
	if err != nil {
		return modelProbeReply{}, err
	}
	payload, err := autonomousTaskParsePayload(model.Config.Model, isClaude, options, input)
	if err != nil {
		return modelProbeReply{}, err
	}
	client := modelProbeHTTPClient(options)
	defer client.CloseIdleConnections()
	var lastErr error
	for _, endpoint := range endpoints {
		for _, headers := range modelDiscoveryAuthHeaders(model.Profile.APIKey, isClaude) {
			headers = modelProbeRequestHeaders(headers, options, isClaude)
			for attempt := 0; attempt <= options.MaxRetries; attempt++ {
				reply, status, requestErr := requestModelProbe(ctx, client, endpoint, headers, payload, isClaude, options.APIMode)
				if requestErr == nil && status >= 200 && status < 300 && strings.TrimSpace(reply.Text) != "" {
					return reply, nil
				}
				if requestErr != nil {
					lastErr = requestErr
				} else {
					lastErr = fmt.Errorf("HTTP %d: %s", status, reply.Text)
				}
				if attempt < options.MaxRetries && (requestErr != nil || retryableModelProbeStatus(status)) && waitModelProbeRetry(ctx, attempt) {
					continue
				}
				break
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("解析模型没有返回内容")
	}
	return modelProbeReply{}, fmt.Errorf("解析模型请求失败：%s", redactProbeDetail(lastErr.Error(), model.Profile.APIKey))
}

func autonomousTaskParsePayload(model string, isClaude bool, options modelProbeOptions, input string) ([]byte, error) {
	system := "你是 GenericAgent Admin 的自主任务解析器。用户会用一句话描述想完成的任务，你把它整理成结构化任务草稿。只输出 JSON：{\"title\":\"不超过40字的动宾短语标题\",\"objective\":\"1到3句话说明任务目标\",\"priority\":\"low|normal|high\",\"risk\":\"一句话说明主要风险或空字符串\",\"project\":\"项目名或空字符串\",\"next_step\":\"建议的第一步行动\"}。所有字段用中文，不要输出 JSON 以外的任何内容。"
	user := "请解析下面这句任务描述：\n" + input
	var payload map[string]interface{}
	if isClaude {
		payload = map[string]interface{}{"model": model, "max_tokens": 512, "stream": false, "system": system, "messages": []map[string]string{{"role": "user", "content": user}}}
	} else if normalizeModelProbeAPIMode(options.APIMode) == "responses" {
		payload = map[string]interface{}{"model": model, "stream": false, "instructions": system, "input": user, "max_output_tokens": 512}
	} else {
		payload = map[string]interface{}{"model": model, "stream": false, "messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}}}
		if modelProbeUsesMaxCompletionTokens(model) {
			payload["max_completion_tokens"] = 512
		} else {
			payload["max_tokens"] = 512
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
	return json.Marshal(payload)
}

func parseAutonomousTaskDraft(text, input string) (autonomousTaskDraft, error) {
	text = strings.TrimSpace(text)
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return autonomousTaskDraft{}, fmt.Errorf("no JSON object found")
	}
	var draft autonomousTaskDraft
	if err := json.Unmarshal([]byte(text[start:end+1]), &draft); err != nil {
		return autonomousTaskDraft{}, err
	}
	draft.Title = strings.TrimSpace(draft.Title)
	if draft.Title == "" {
		return autonomousTaskDraft{}, fmt.Errorf("parsed title is empty")
	}
	if runes := []rune(draft.Title); len(runes) > 200 {
		draft.Title = string(runes[:200])
	}
	for field, value := range map[string]*string{
		"objective": &draft.Objective, "risk": &draft.Risk, "project": &draft.Project, "next_step": &draft.NextStep,
	} {
		trimmed := strings.Join(strings.Fields(*value), " ")
		if field == "objective" && len([]rune(trimmed)) > 4000 {
			trimmed = string([]rune(trimmed)[:4000])
		} else if field != "objective" && len([]rune(trimmed)) > 160 {
			trimmed = string([]rune(trimmed)[:160])
		}
		*value = redactProbeDetail(trimmed, "")
	}
	switch strings.ToLower(strings.TrimSpace(draft.Priority)) {
	case "low", "normal", "high":
		draft.Priority = strings.ToLower(strings.TrimSpace(draft.Priority))
	default:
		draft.Priority = "normal"
	}
	if strings.TrimSpace(draft.Objective) == "" {
		draft.Objective = input
	}
	return draft, nil
}
