package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

func decodeModelProbeReply(body []byte, isClaude bool, apiMode string) (string, error) {
	if bytes.Contains(body, []byte("data:")) {
		return decodeModelProbeStream(body, isClaude)
	}
	if isClaude {
		var payload struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return "", err
		}
		var parts []string
		for _, item := range payload.Content {
			parts = append(parts, item.Text)
		}
		return strings.TrimSpace(strings.Join(parts, "\n")), nil
	}
	if normalizeModelProbeAPIMode(apiMode) == "responses" {
		return decodeResponsesProbeReply(body)
	}
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if len(payload.Choices) == 0 {
		return "", fmt.Errorf("response contains no choices")
	}
	return strings.TrimSpace(payload.Choices[0].Message.Content + payload.Choices[0].Text), nil
}

func decodeResponsesProbeReply(body []byte) (string, error) {
	var payload struct {
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	var text strings.Builder
	for _, output := range payload.Output {
		for _, content := range output.Content {
			text.WriteString(content.Text)
		}
	}
	return strings.TrimSpace(text.String()), nil
}

func decodeModelProbeStream(body []byte, isClaude bool) (string, error) {
	var text strings.Builder
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if bytes.Equal(data, []byte("[DONE]")) || len(data) == 0 {
			continue
		}
		var event map[string]interface{}
		if json.Unmarshal(data, &event) != nil {
			continue
		}
		if isClaude {
			appendClaudeProbeDelta(&text, event)
			continue
		}
		appendOpenAIProbeDelta(&text, event)
	}
	if strings.TrimSpace(text.String()) == "" {
		return "", fmt.Errorf("stream contains no text response")
	}
	return strings.TrimSpace(text.String()), nil
}

func appendClaudeProbeDelta(text *strings.Builder, event map[string]interface{}) {
	delta, _ := event["delta"].(map[string]interface{})
	if part, ok := delta["text"].(string); ok {
		text.WriteString(part)
	}
}

func appendOpenAIProbeDelta(text *strings.Builder, event map[string]interface{}) {
	if part, ok := event["delta"].(string); ok && event["type"] == "response.output_text.delta" {
		text.WriteString(part)
	}
	choices, _ := event["choices"].([]interface{})
	if len(choices) == 0 {
		return
	}
	choice, _ := choices[0].(map[string]interface{})
	delta, _ := choice["delta"].(map[string]interface{})
	if part, ok := delta["content"].(string); ok {
		text.WriteString(part)
	}
}
