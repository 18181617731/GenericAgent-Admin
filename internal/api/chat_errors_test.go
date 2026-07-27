package api

import (
	"strings"
	"testing"
)

func TestClassifyChatErrorIdentifiesProvider403WithoutHTMLDump(t *testing.T) {
	raw := `!!!Error: HTTP 403: <!DOCTYPE html><html><head><title>Attention Required! | Cloudflare</title></head><body>blocked</body></html>`
	info := classifyChatError(raw, "")
	if info.Source != chatErrorSourceModel || info.Code != "HTTP_403" {
		t.Fatalf("info=%#v want provider HTTP 403", info)
	}
	if !strings.Contains(info.Summary, "模型服务") || !strings.Contains(info.Summary, "403") {
		t.Fatalf("summary=%q want readable provider error", info.Summary)
	}
	if strings.Contains(strings.ToLower(info.Detail), "<html") || !strings.Contains(info.Detail, "Cloudflare") {
		t.Fatalf("detail=%q want compact HTML page title", info.Detail)
	}
}

func TestClassifyChatErrorSeparatesProjectAndNetworkFailures(t *testing.T) {
	project := classifyChatError("Traceback: ModuleNotFoundError: No module named 'requests'", "")
	if project.Source != chatErrorSourceProject || project.Code != "PROJECT_RUNTIME_ERROR" {
		t.Fatalf("project=%#v want project runtime classification", project)
	}
	network := classifyChatError("connection reset by peer", "")
	if network.Source != chatErrorSourceNetwork || !network.Retryable {
		t.Fatalf("network=%#v want retryable network classification", network)
	}
}

func TestNormalizeChatErrorMessageReplacesRawContentWithSummary(t *testing.T) {
	msg := chatMessage{Content: "!!!Error: HTTP 429: rate limited"}
	normalizeChatErrorMessage(&msg, "")
	if !msg.Error || msg.ErrorInfo == nil || msg.ErrorInfo.Code != "HTTP_429" {
		t.Fatalf("message=%#v want structured HTTP 429 error", msg)
	}
	if strings.Contains(msg.Content, "!!!Error") || !strings.Contains(msg.Content, "429") {
		t.Fatalf("content=%q want user-facing summary", msg.Content)
	}
}
