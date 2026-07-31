package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type modelProbeReply struct {
	Text  string
	Usage usageTotals
}

func decodeModelProbeReply(body []byte, isClaude bool, apiMode string) (modelProbeReply, error) {
	if bytes.Contains(body, []byte("data:")) {
		return decodeModelProbeStream(body, isClaude, apiMode)
	}
	if isClaude {
		var payload struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Usage map[string]interface{} `json:"usage"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return modelProbeReply{}, err
		}
		var parts []string
		for _, item := range payload.Content {
			parts = append(parts, item.Text)
		}
		return modelProbeReply{Text: strings.TrimSpace(strings.Join(parts, "\n")), Usage: modelProbeUsage(payload.Usage, true, apiMode)}, nil
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
		Usage map[string]interface{} `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return modelProbeReply{}, err
	}
	if len(payload.Choices) == 0 {
		return modelProbeReply{}, fmt.Errorf("response contains no choices")
	}
	return modelProbeReply{Text: strings.TrimSpace(payload.Choices[0].Message.Content + payload.Choices[0].Text), Usage: modelProbeUsage(payload.Usage, false, apiMode)}, nil
}

func decodeResponsesProbeReply(body []byte) (modelProbeReply, error) {
	var payload struct {
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage map[string]interface{} `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return modelProbeReply{}, err
	}
	var text strings.Builder
	for _, output := range payload.Output {
		for _, content := range output.Content {
			text.WriteString(content.Text)
		}
	}
	return modelProbeReply{Text: strings.TrimSpace(text.String()), Usage: modelProbeUsage(payload.Usage, false, "responses")}, nil
}

func decodeModelProbeStream(body []byte, isClaude bool, apiMode string) (modelProbeReply, error) {
	var text strings.Builder
	usage := usageTotals{}
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
		if raw, ok := event["usage"].(map[string]interface{}); ok {
			mergeUsageTotals(&usage, modelProbeUsage(raw, isClaude, apiMode))
		}
		if response, ok := event["response"].(map[string]interface{}); ok {
			if raw, ok := response["usage"].(map[string]interface{}); ok {
				mergeUsageTotals(&usage, modelProbeUsage(raw, isClaude, apiMode))
			}
		}
		if message, ok := event["message"].(map[string]interface{}); ok {
			if raw, ok := message["usage"].(map[string]interface{}); ok {
				mergeUsageTotals(&usage, modelProbeUsage(raw, isClaude, apiMode))
			}
		}
		if isClaude {
			appendClaudeProbeDelta(&text, event)
			continue
		}
		appendOpenAIProbeDelta(&text, event)
	}
	if strings.TrimSpace(text.String()) == "" {
		return modelProbeReply{}, fmt.Errorf("stream contains no text response")
	}
	return modelProbeReply{Text: strings.TrimSpace(text.String()), Usage: usage}, nil
}

func modelProbeUsage(raw map[string]interface{}, isClaude bool, apiMode string) usageTotals {
	read := func(key string) int {
		value, ok := raw[key]
		if !ok {
			return 0
		}
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case int:
			return typed
		default:
			return 0
		}
	}
	input := read("prompt_tokens")
	if input == 0 {
		input = read("input_tokens")
	}
	output := read("completion_tokens")
	if output == 0 {
		output = read("output_tokens")
	}
	cached := 0
	if details, ok := raw["prompt_tokens_details"].(map[string]interface{}); ok {
		cached = intValue(details["cached_tokens"])
	}
	if details, ok := raw["input_tokens_details"].(map[string]interface{}); ok && cached == 0 {
		cached = intValue(details["cached_tokens"])
	}
	if isClaude {
		input += read("cache_creation_input_tokens") + read("cache_read_input_tokens")
		cached = read("cache_read_input_tokens")
	}
	if normalizeModelProbeAPIMode(apiMode) == "responses" {
		input = read("input_tokens")
		output = read("output_tokens")
	}
	total := read("total_tokens")
	if total == 0 {
		total = input + output
	}
	result := usageTotals{InputTokens: input, OutputTokens: output, TotalTokens: total}
	if cached > 0 {
		result.Other = map[string]int{"cached_tokens": cached}
	}
	return result
}

func intValue(value interface{}) int {
	if number, ok := value.(float64); ok {
		return int(number)
	}
	if number, ok := value.(int); ok {
		return number
	}
	return 0
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
