package api

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	chatErrorSourceModel   = "model_service"
	chatErrorSourceNetwork = "network"
	chatErrorSourceProject = "project_runtime"
	chatErrorSourceSystem  = "system"
)

type chatErrorInfo struct {
	Source      string `json:"source"`
	SourceLabel string `json:"source_label"`
	Code        string `json:"code"`
	Summary     string `json:"summary"`
	Hint        string `json:"hint"`
	Detail      string `json:"detail,omitempty"`
	Retryable   bool   `json:"retryable"`
}

var (
	chatHTTPStatusRE = regexp.MustCompile(`(?i)\bHTTP\s+(\d{3})\b`)
	chatHTMLTitleRE  = regexp.MustCompile(`(?is)<title[^>]*>\s*(.*?)\s*</title>`)
	chatHTMLTagRE    = regexp.MustCompile(`(?is)<[^>]+>`)
)

func classifyChatError(raw, sourceHint string) chatErrorInfo {
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "!!!Error:"))
	lower := strings.ToLower(text)
	status := chatHTTPStatus(text)
	if status > 0 {
		info := classifyChatHTTPError(status)
		info.Detail = compactChatErrorDetail(text, status)
		return info
	}
	if sourceHint != "" {
		return classifyChatHintedError(text, sourceHint)
	}
	if chatErrorContainsAny(lower, "modulenotfounderror", "importerror", "traceback", "agentmain", "llmcore", "requests", "worker exited", "no such file", "failed to start", "broken pipe") {
		return classifyChatHintedError(text, chatErrorSourceProject)
	}
	if chatErrorContainsAny(lower, "timeout", "timed out", "connection refused", "connection reset", "dns", "tls", "network", "eof") {
		return classifyChatHintedError(text, chatErrorSourceNetwork)
	}
	return classifyChatHintedError(text, chatErrorSourceModel)
}

func classifyChatHTTPError(status int) chatErrorInfo {
	info := chatErrorInfo{Source: chatErrorSourceModel, SourceLabel: "模型服务", Code: fmt.Sprintf("HTTP_%d", status)}
	switch {
	case status == 401:
		info.Summary = "模型服务拒绝访问（HTTP 401）"
		info.Hint = "请检查当前服务商的 API Key 是否正确、是否已过期。"
	case status == 403:
		info.Summary = "模型服务拒绝请求（HTTP 403）"
		info.Hint = "请检查 API Key 权限、模型权限、来源限制或服务商的安全防护配置。"
	case status == 404:
		info.Summary = "模型服务找不到请求地址或模型（HTTP 404）"
		info.Hint = "请检查服务商地址、API 路径和模型 ID 是否匹配。"
	case status == 408 || status == 504:
		info.Summary = fmt.Sprintf("模型服务响应超时（HTTP %d）", status)
		info.Hint = "可以稍后重试，也可以检查网络和服务商响应状态。"
		info.Retryable = true
	case status == 429:
		info.Summary = "模型服务请求过于频繁（HTTP 429）"
		info.Hint = "请稍后重试，或检查服务商的额度与限流策略。"
		info.Retryable = true
	case status >= 500:
		info.Summary = fmt.Sprintf("模型服务暂时异常（HTTP %d）", status)
		info.Hint = "这通常是服务商侧异常，可以稍后重试；若持续发生请检查服务商状态。"
		info.Retryable = true
	default:
		info.Summary = fmt.Sprintf("模型服务请求失败（HTTP %d）", status)
		info.Hint = "请检查服务商配置和当前模型是否可用。"
	}
	return info
}

func classifyChatHintedError(detail, source string) chatErrorInfo {
	info := chatErrorInfo{Source: source, Code: "CHAT_ERROR", Detail: compactChatErrorDetail(detail, 0)}
	switch source {
	case chatErrorSourceProject:
		info.SourceLabel = "项目运行环境"
		info.Code = "PROJECT_RUNTIME_ERROR"
		info.Summary = "GenericAgent 项目运行异常，本次对话未完成"
		info.Hint = "请到“总览”的系统状态检查中查看 Python、依赖和 GA 运行环境；修复后可重新发送。"
	case chatErrorSourceNetwork:
		info.SourceLabel = "网络连接"
		info.Code = "NETWORK_ERROR"
		info.Summary = "无法连接模型服务，本次对话未完成"
		info.Hint = "请检查网络、代理和服务商地址，确认后可重新发送。"
		info.Retryable = true
	default:
		info.Source = chatErrorSourceModel
		info.SourceLabel = "模型服务"
		info.Code = "MODEL_REQUEST_ERROR"
		info.Summary = "模型服务调用失败，本次对话未完成"
		info.Hint = "请检查模型配置和服务商状态，确认后可重新发送。"
		info.Retryable = true
	}
	return info
}

func normalizeChatErrorMessage(msg *chatMessage, sourceHint string) {
	if msg == nil {
		return
	}
	raw := msg.Content
	info := classifyChatError(raw, sourceHint)
	msg.Error = true
	msg.Content = info.Summary
	msg.ErrorInfo = &info
}

func newChatErrorMessage(id, content string, llmNo *int, sourceHint string, elapsedMS int64) chatMessage {
	msg := chatMessage{ID: id, Role: "assistant", Content: content, LLMNo: llmNo, CreatedAt: time.Now().Unix(), Error: true, ElapsedMS: elapsedMS}
	normalizeChatErrorMessage(&msg, sourceHint)
	return msg
}

func chatHTTPStatus(text string) int {
	match := chatHTTPStatusRE.FindStringSubmatch(text)
	if len(match) != 2 {
		return 0
	}
	status, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return status
}

func compactChatErrorDetail(raw string, status int) string {
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "!!!Error:"))
	if chatHTMLTagRE.MatchString(text) || strings.Contains(strings.ToLower(text), "<!doctype html") {
		title := "上游 HTML 错误页"
		if match := chatHTMLTitleRE.FindStringSubmatch(text); len(match) == 2 {
			cleanTitle := strings.Join(strings.Fields(html.UnescapeString(match[1])), " ")
			if cleanTitle != "" {
				title = title + "：" + cleanTitle
			}
		}
		if status > 0 {
			return fmt.Sprintf("HTTP %d；%s", status, title)
		}
		return title
	}
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) > 1200 {
		text = string([]rune(text)[:1200]) + "…"
	}
	return text
}

func chatErrorContainsAny(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}
