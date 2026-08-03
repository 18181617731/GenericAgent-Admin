package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"genericagent-admin-go/internal/ga"
	"genericagent-admin-go/internal/modelconfig"
)

const (
	maxAutonomousReviewItems = 30
	autonomousReviewCooldown = 30 * time.Second
)

type autonomousReviewModel struct {
	LLMNo    int
	Provider string
	Profile  modelconfig.Profile
	Config   modelconfig.ModelConfig
	Options  modelProbeOptions
}

type autonomousReviewDecision struct {
	Decision   string `json:"decision"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
}

func (s *Server) autonomousApprovalReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request struct {
		ID  string   `json:"id"`
		IDs []string `json:"ids"`
	}
	if err := decode(r, &request); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	overview, err := ga.BuildAutonomousApprovals(s.CfgStore.Cfg.GARoot)
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := autonomousReviewTargets(overview.Items, request.ID, request.IDs)
	if len(items) > maxAutonomousReviewItems {
		items = items[:maxAutonomousReviewItems]
	}
	results := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		record, reviewErr := s.reviewAutonomousApproval(item, true)
		result := map[string]interface{}{"id": item.ID, "status": record.ReviewStatus, "attempts": record.Attempts}
		if reviewErr != nil {
			result["error"] = reviewErr.Error()
		}
		results = append(results, result)
	}
	updated, err := ga.BuildAutonomousApprovals(s.CfgStore.Cfg.GARoot)
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "reviewed": len(results), "results": results, "overview": updated})
}

func autonomousReviewTargets(items []ga.AutonomousApproval, id string, ids []string) []ga.AutonomousApproval {
	selected := map[string]bool{}
	if strings.TrimSpace(id) != "" {
		selected[strings.TrimSpace(id)] = true
	}
	for _, value := range ids {
		if value = strings.TrimSpace(value); value != "" {
			selected[value] = true
		}
	}
	targets := make([]ga.AutonomousApproval, 0, len(items))
	for _, item := range items {
		if item.State != "pending" || (len(selected) > 0 && !selected[item.ID]) {
			continue
		}
		targets = append(targets, item)
	}
	return targets
}

func (s *Server) reviewAutonomousApproval(item ga.AutonomousApproval, force bool) (ga.AutonomousReviewRecord, error) {
	records, err := ga.LoadAutonomousReviews(s.CfgStore.Cfg.GARoot)
	if err != nil {
		return ga.AutonomousReviewRecord{}, err
	}
	record := findAutonomousReviewRecord(records, item.ID)
	if !force && !record.NextRetryAt.IsZero() && time.Now().Before(record.NextRetryAt) {
		return record, nil
	}
	record.ID = item.ID
	record.Fingerprint = ga.AutonomousApprovalFingerprint(item)
	record.Attempts++
	record.UpdatedAt = time.Now()
	model, modelErr := s.resolveAutonomousReviewModel()
	if modelErr != nil {
		return s.saveAutonomousReviewFailure(record, modelErr)
	}
	record.ReviewModelNo = model.LLMNo
	record.ReviewModel = model.Config.Model
	record.ReviewProvider = model.Provider
	reply, requestErr := runAutonomousReviewRequest(context.Background(), model, item)
	if requestErr != nil {
		return s.saveAutonomousReviewFailure(record, requestErr)
	}
	decision := parseAutonomousReviewDecision(reply.Text)
	record.ReviewStatus = "model"
	record.ReviewDecision = decision.Decision
	record.ReviewConfidence = decision.Confidence
	record.ReviewReason = decision.Reason
	record.NextRetryAt = time.Time{}
	record.UpdatedAt = time.Now()
	return record, ga.SaveAutonomousReview(s.CfgStore.Cfg.GARoot, record)
}

func findAutonomousReviewRecord(records []ga.AutonomousReviewRecord, id string) ga.AutonomousReviewRecord {
	for _, record := range records {
		if record.ID == id {
			return record
		}
	}
	return ga.AutonomousReviewRecord{}
}

func (s *Server) saveAutonomousReviewFailure(record ga.AutonomousReviewRecord, err error) (ga.AutonomousReviewRecord, error) {
	record.ReviewStatus = "fallback"
	record.ReviewDecision = "needs_approval"
	record.ReviewConfidence = "high"
	record.ReviewReason = fmt.Sprintf("本轮审核调用失败，已按模型页面配置完成重试；这是第 %d 轮审核：%s；下次重新审核时会再次尝试", record.Attempts, redactProbeDetail(err.Error(), ""))
	record.NextRetryAt = time.Now().Add(autonomousReviewCooldown)
	record.UpdatedAt = time.Now()
	return record, ga.SaveAutonomousReview(s.CfgStore.Cfg.GARoot, record)
}

func (s *Server) resolveAutonomousReviewModel() (autonomousReviewModel, error) {
	draft, err := s.loadModelsFromOfficialMyKey(true)
	if err != nil {
		return autonomousReviewModel{}, err
	}
	models := orderedAutonomousReviewModels(draft.Profiles)
	if len(models) == 0 {
		return autonomousReviewModel{}, fmt.Errorf("没有可用于审核的已启用模型")
	}
	desired := -1
	if s.CfgStore.Cfg.ServiceModels != nil {
		desired = s.CfgStore.Cfg.ServiceModels["reflect/autonomous.py"]
	}
	if desired >= 0 {
		for _, model := range models {
			if model.LLMNo == desired {
				return model, nil
			}
		}
	}
	return models[0], nil
}

func orderedAutonomousReviewModels(profiles []modelconfig.Profile) []autonomousReviewModel {
	models := make([]autonomousReviewModel, 0)
	sequence := 0
	for _, profile := range profiles {
		for _, rawConfig := range probeProfileModelConfigs(profile) {
			config := inheritAutonomousModelConfig(rawConfig, profile)
			if !modelconfig.ModelConfigEnabled(config) || strings.TrimSpace(config.Model) == "" {
				continue
			}
			order := sequence
			if config.SortOrder != nil {
				order = *config.SortOrder
			}
			options := modelProbeOptions{
				MaxRetries:     firstProbeIntAllowZero(config.MaxRetries, modelProbeDefaultMaxRetries),
				ReadTimeout:    firstProbeInt(config.ReadTimeout, modelProbeDefaultReadTimeout),
				ConnectTimeout: firstProbeInt(config.ConnectTimeout, modelProbeDefaultConnectTimeout),
				APIMode:        config.APIMode, UserAgent: config.UserAgent, ReasoningEffort: config.ReasoningEffort, Configured: true,
			}
			models = append(models, autonomousReviewModel{Provider: chatProviderDisplayName(profile), Profile: profile, Config: config, Options: options, LLMNo: order})
			sequence++
		}
	}
	sort.SliceStable(models, func(i, j int) bool { return models[i].LLMNo < models[j].LLMNo })
	for index := range models {
		models[index].LLMNo = index
	}
	return models
}

func inheritAutonomousModelConfig(config modelconfig.ModelConfig, profile modelconfig.Profile) modelconfig.ModelConfig {
	if config.MaxRetries == nil {
		config.MaxRetries = profile.MaxRetries
	}
	if config.ReadTimeout == nil {
		config.ReadTimeout = profile.ReadTimeout
	}
	if config.ConnectTimeout == nil {
		config.ConnectTimeout = profile.ConnectTimeout
	}
	if config.UserAgent == "" {
		config.UserAgent = profile.UserAgent
	}
	if config.APIMode == "" {
		config.APIMode = profile.APIMode
	}
	if config.ReasoningEffort == "" {
		config.ReasoningEffort = profile.ReasoningEffort
	}
	return config
}

func runAutonomousReviewRequest(ctx context.Context, model autonomousReviewModel, item ga.AutonomousApproval) (modelProbeReply, error) {
	options := normalizeModelProbeOptions(model.Options)
	endpoints, err := modelProbeEndpoints(model.Profile.APIBase, isClaudeModel(model.Profile.Type), options.APIMode)
	if err != nil {
		return modelProbeReply{}, err
	}
	payload, err := autonomousReviewPayload(model.Config.Model, isClaudeModel(model.Profile.Type), options, item)
	if err != nil {
		return modelProbeReply{}, err
	}
	client := modelProbeHTTPClient(options)
	defer client.CloseIdleConnections()
	var lastErr error
	for _, endpoint := range endpoints {
		for _, headers := range modelDiscoveryAuthHeaders(model.Profile.APIKey, isClaudeModel(model.Profile.Type)) {
			headers = modelProbeRequestHeaders(headers, options, isClaudeModel(model.Profile.Type))
			for attempt := 0; attempt <= options.MaxRetries; attempt++ {
				reply, status, requestErr := requestModelProbe(ctx, client, endpoint, headers, payload, isClaudeModel(model.Profile.Type), options.APIMode)
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
		lastErr = fmt.Errorf("审核模型没有返回内容")
	}
	return modelProbeReply{}, fmt.Errorf("审核模型请求失败：%s", redactProbeDetail(lastErr.Error(), model.Profile.APIKey))
}

func isClaudeModel(protocol string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(protocol)), "claude")
}

func autonomousReviewPayload(model string, isClaude bool, options modelProbeOptions, item ga.AutonomousApproval) ([]byte, error) {
	system := "你是 GenericAgent Admin 的自主任务审核器。你只能提供风险与执行建议，不能替用户批准或执行任务。只输出 JSON：{\"decision\":\"approved|rejected|needs_approval\",\"confidence\":\"low|medium|high\",\"reason\":\"用大白话说明理由\"}。"
	input, err := json.Marshal(map[string]string{"title": item.Title, "target": item.Target, "status": item.Status, "risk": item.Risk, "evidence": item.Evidence, "next_step": item.NextStep, "expected_outcome": item.ExpectedOutcome})
	if err != nil {
		return nil, err
	}
	user := "请审核下面这项待审批建议，重点判断风险、证据是否充分、批准后是否可控。不要调用工具，不要修改文件。\n" + string(input)
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

func parseAutonomousReviewDecision(text string) autonomousReviewDecision {
	text = strings.TrimSpace(text)
	decision := autonomousReviewDecision{Decision: "needs_approval", Confidence: "medium", Reason: text}
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		var parsed autonomousReviewDecision
		if json.Unmarshal([]byte(text[start:end+1]), &parsed) == nil {
			decision = parsed
		}
	}
	decision.Decision = normalizeReviewDecision(decision.Decision)
	if decision.Confidence == "" {
		decision.Confidence = "medium"
	}
	if decision.Reason == "" {
		decision.Reason = "模型未给出明确理由，仍需人工确认"
	}
	decision.Reason = redactProbeDetail(decision.Reason, "")
	return decision
}

func normalizeReviewDecision(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approved", "approve", "通过", "建议批准":
		return "approved"
	case "rejected", "reject", "拒绝", "建议拒绝":
		return "rejected"
	default:
		return "needs_approval"
	}
}
