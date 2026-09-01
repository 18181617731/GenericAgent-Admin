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
	Risk       string `json:"risk"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
	Problem    string `json:"problem"`
}

func (s *Server) reviewAutonomousApprovals(overview *ga.AutonomousApprovalOverview) {
	if s == nil || overview == nil || s.CfgStore == nil {
		return
	}
	model, err := s.resolveAutonomousReviewModel()
	if err != nil {
		overview.ReviewStatus = "not_configured"
		return
	}
	modelNo := model.LLMNo
	overview.ReviewModelNo = &modelNo
	overview.ReviewModel = autonomousReviewModelName(model)
	overview.ReviewProvider = model.Provider
	if overview.ReviewStatus == "" {
		overview.ReviewStatus = "configured"
	}
	for index := range overview.Items {
		item := &overview.Items[index]
		if item.State != "pending" {
			continue
		}
		if item.ReviewModelNo == nil {
			item.ReviewModelNo = &modelNo
		}
		if strings.TrimSpace(item.ReviewModel) == "" {
			item.ReviewModel = autonomousReviewModelName(model)
		}
		if strings.TrimSpace(item.ReviewProvider) == "" {
			item.ReviewProvider = model.Provider
		}
	}
}

func (s *Server) autonomousApprovalReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request struct {
		ID        string   `json:"id"`
		IDs       []string `json:"ids"`
		Automatic bool     `json:"automatic"`
	}
	if err := decode(r, &request); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	overview, err := ga.BuildAutonomousApprovals(s.CfgStore.Snapshot().GARoot)
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := autonomousReviewTargets(overview.Items, request.ID, request.IDs)
	if len(items) > maxAutonomousReviewItems {
		items = items[:maxAutonomousReviewItems]
	}
	results := make([]map[string]interface{}, 0, len(items))
	autoApproved := 0
	for _, item := range items {
		record, reviewErr := s.reviewAutonomousApproval(item, true)
		result := map[string]interface{}{"id": item.ID, "status": record.ReviewStatus, "attempts": record.Attempts, "decision": record.ReviewDecision, "risk": record.ReviewRisk, "confidence": record.ReviewConfidence}
		if reviewErr != nil {
			result["error"] = reviewErr.Error()
		} else {
			// The trigger can be automatic (when the approvals tab opens) or
			// explicit (when the user asks for another review), but the safety
			// policy is the same in both cases. A usable low/medium-risk model
			// result should not wait for a redundant human click.
			approved, autoErr := s.autoApproveAutonomousApproval(item, record)
			if autoErr != nil {
				result["error"] = autoErr.Error()
			} else if approved {
				result["auto_approved"] = true
				autoApproved++
			}
		}
		results = append(results, result)
	}
	updated, err := ga.BuildAutonomousApprovals(s.CfgStore.Snapshot().GARoot)
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "automatic": request.Automatic, "reviewed": len(results), "auto_approved": autoApproved, "results": results, "overview": updated})
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
	records, err := ga.LoadAutonomousReviews(s.CfgStore.Snapshot().GARoot)
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
	record.ReviewModel = autonomousReviewModelName(model)
	record.ReviewProvider = model.Provider
	reply, requestErr := runAutonomousReviewRequest(context.Background(), model, item)
	if requestErr != nil {
		return s.saveAutonomousReviewFailure(record, requestErr)
	}
	decision := parseAutonomousReviewDecision(reply.Text)
	record.ReviewStatus = "model"
	record.ReviewDecision = decision.Decision
	record.ReviewRisk = decision.Risk
	record.ReviewConfidence = decision.Confidence
	record.ReviewReason = decision.Reason
	if decision.Problem != "" {
		record.ReviewProblem = decision.Problem
	}
	record.NextRetryAt = time.Time{}
	record.UpdatedAt = time.Now()
	return record, ga.SaveAutonomousReview(s.CfgStore.Snapshot().GARoot, record)
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
	record.ReviewRisk = "unknown"
	record.ReviewConfidence = "high"
	record.ReviewReason = fmt.Sprintf("本轮审核调用失败，已按模型页面配置完成重试；这是第 %d 轮审核：%s；下次重新审核时会再次尝试", record.Attempts, redactProbeDetail(err.Error(), ""))
	record.NextRetryAt = time.Now().Add(autonomousReviewCooldown)
	record.UpdatedAt = time.Now()
	return record, ga.SaveAutonomousReview(s.CfgStore.Snapshot().GARoot, record)
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
	serviceModels := s.CfgStore.Snapshot().ServiceModels
	if serviceModels != nil {
		desired = serviceModels["reflect/autonomous.py"]
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

func autonomousReviewModelName(model autonomousReviewModel) string {
	if name := strings.TrimSpace(model.Config.Name); name != "" {
		return name
	}
	return strings.TrimSpace(model.Config.Model)
}

func runAutonomousReviewRequest(ctx context.Context, model autonomousReviewModel, item ga.AutonomousApproval) (modelProbeReply, error) {
	options := normalizeModelProbeOptions(model.Options)
	isClaude := isClaudeModel(model.Profile.Type)
	endpoints, err := modelProbeEndpoints(model.Profile.APIBase, isClaude, options.APIMode)
	if err != nil {
		return modelProbeReply{}, err
	}
	payload, err := autonomousReviewPayload(model.Config.Model, isClaude, options, item)
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
		lastErr = fmt.Errorf("审核模型没有返回内容")
	}
	return modelProbeReply{}, fmt.Errorf("审核模型请求失败：%s", redactProbeDetail(lastErr.Error(), model.Profile.APIKey))
}

func isClaudeModel(protocol string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(protocol)), "claude")
}

func autonomousReviewPayload(model string, isClaude bool, options modelProbeOptions, item ga.AutonomousApproval) ([]byte, error) {
	system := "你是 GenericAgent Admin 的自主任务审核器。你可以判断一项建议是否符合自动批准条件，但不能调用工具、修改文件或执行任务。只输出 JSON：{\"decision\":\"approved|rejected|needs_approval\",\"risk\":\"low|medium|high|unknown\",\"confidence\":\"low|medium|high\",\"reason\":\"用大白话说明理由\",\"problem\":\"用中文大白话概括这项任务具体要解决的问题，20至80字，不要只复述批准动作\"}。只有证据充分、没有阻塞、decision 为 approved、risk 为 low 或 medium 且 confidence 为 medium 或 high 时，系统才会自动批准；高风险、风险不明、证据不足、阻塞或低置信度必须返回 needs_approval。problem 必须结合任务标题、目标、状态、证据和下一步生成，不能使用“尚未落地或尚未确认的问题”这类泛化句。"
	input, err := json.Marshal(map[string]string{"title": item.Title, "source": item.Source, "candidate_source": item.CandidateSource, "problem": item.Problem, "target": item.Target, "status": item.Status, "risk": item.Risk, "evidence": item.Evidence, "next_step": item.NextStep, "expected_outcome": item.ExpectedOutcome})
	if err != nil {
		return nil, err
	}
	user := "请审核下面这项待审批建议，重点判断风险、证据是否充分、批准后是否可控，并明确给出 risk。不要调用工具，不要修改文件。\n" + string(input)
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
	decision := autonomousReviewDecision{Decision: "needs_approval", Risk: "unknown", Confidence: "medium", Reason: text}
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		var parsed autonomousReviewDecision
		if json.Unmarshal([]byte(text[start:end+1]), &parsed) == nil {
			decision = parsed
		}
	}
	decision.Decision = normalizeReviewDecision(decision.Decision)
	decision.Risk = normalizeReviewRisk(decision.Risk)
	decision.Confidence = strings.ToLower(strings.TrimSpace(decision.Confidence))
	if decision.Confidence != "low" && decision.Confidence != "medium" && decision.Confidence != "high" {
		decision.Confidence = "medium"
	}
	if decision.Reason == "" {
		decision.Reason = "模型未给出明确理由，仍需人工确认"
	}
	decision.Reason = redactProbeDetail(decision.Reason, "")
	if len([]rune(decision.Reason)) > 500 {
		decision.Reason = string([]rune(decision.Reason)[:500])
	}
	decision.Problem = normalizeAutonomousReviewProblem(decision.Problem)
	return decision
}

func normalizeReviewRisk(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	switch normalized {
	case "low", "低", "低风险", "low_risk":
		return "low"
	case "medium", "中", "中风险", "medium_risk":
		return "medium"
	case "high", "高", "高风险", "high_risk":
		return "high"
	default:
		return "unknown"
	}
}

func (s *Server) autoApproveAutonomousApproval(item ga.AutonomousApproval, record ga.AutonomousReviewRecord) (bool, error) {
	if !shouldAutoApproveAutonomousReview(item, record) {
		return false, nil
	}
	_, _, err := ga.DecideAutonomousApprovalWithSource(s.CfgStore.Snapshot().GARoot, item.ID, "approved", autonomousAutoApprovalNote(record), "model_auto")
	if err != nil {
		return false, err
	}
	return true, nil
}

func shouldAutoApproveAutonomousReview(item ga.AutonomousApproval, record ga.AutonomousReviewRecord) bool {
	if item.State != "pending" || strings.TrimSpace(item.Decision) != "" || record.ReviewStatus != "model" || normalizeReviewDecision(record.ReviewDecision) != "approved" {
		return false
	}
	if strings.TrimSpace(item.Evidence) == "" {
		return false
	}
	if risk := normalizeReviewRisk(record.ReviewRisk); risk != "low" && risk != "medium" {
		return false
	}
	confidence := strings.ToLower(strings.TrimSpace(record.ReviewConfidence))
	if confidence != "medium" && confidence != "high" {
		return false
	}
	text := strings.ToLower(strings.Join([]string{item.Status, item.Risk, item.Evidence, item.ReviewReason, item.NextStep}, " "))
	for _, negated := range []string{
		"no human review required", "no human approval required", "does not require human review", "does not require human approval",
		"无需人工复核", "不需要人工复核", "不需人工复核", "无需人工确认", "不需要人工确认", "不需人工确认", "无需人工审批", "不需要人工审批", "不需人工审批", "无需用户确认", "不需要用户确认",
	} {
		text = strings.ReplaceAll(text, negated, "")
	}
	for _, marker := range []string{
		"high risk", "high_risk", "高风险",
		"human review required", "report requires human approval", "requires human approval", "需要人工复核", "需人工复核", "需要人工审批", "需人工审批", "待人工确认", "待人工复核", "需人工确认",
		"evidence missing", "missing evidence", "insufficient evidence", "evidence unavailable", "cannot verify evidence", "approval evidence is missing", "approval evidence cannot be verified", "证据不足", "证据不充分", "证据缺失", "审批证据缺失", "审批证据无法核验",
		"blocked", "阻塞", "not implemented", "未实施", "未修改源码", "无法核验", "无法验证", "unverifiable",
	} {
		if strings.Contains(text, marker) {
			return false
		}
	}
	return true
}

func autonomousAutoApprovalNote(record ga.AutonomousReviewRecord) string {
	risk := map[string]string{"low": "低", "medium": "中"}[normalizeReviewRisk(record.ReviewRisk)]
	confidence := map[string]string{"medium": "中", "high": "高"}[strings.ToLower(strings.TrimSpace(record.ReviewConfidence))]
	note := fmt.Sprintf("模型自动批准：模型判断为%s风险，置信度%s。", risk, confidence)
	if reason := strings.TrimSpace(record.ReviewReason); reason != "" {
		note += " " + reason
	}
	if len([]rune(note)) > 1000 {
		note = string([]rune(note)[:1000])
	}
	return note
}

func normalizeAutonomousReviewProblem(value string) string {
	problem := strings.Join(strings.Fields(value), " ")
	problem = strings.TrimSpace(problem)
	for _, prefix := range []string{"问题：", "要解决的问题：", "问题:", "Problem:", "problem:"} {
		problem = strings.TrimSpace(strings.TrimPrefix(problem, prefix))
	}
	if len([]rune(problem)) > 160 {
		problem = string([]rune(problem)[:160])
	}
	return redactProbeDetail(problem, "")
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
