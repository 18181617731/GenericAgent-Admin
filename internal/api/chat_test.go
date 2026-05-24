package api

import "testing"

func TestParseLLMJSONArrayFromMixedOutputIgnoresGAStartupLogs(t *testing.T) {
	out := []byte("[ContextGuard] installed\r\n[MemoryLauncher] native\r\n[Info] Load mykeys from E:\\AITools\\GenericAgent\\mykey.py\r\n" +
		`[{"index":0,"label":"NativeOAISession/gpt-5.5/cpa","name":"gpt-5.5/cpa","model":"cpa","active":true},{"index":1,"label":"NativeOAISession/deepseek-v4-pro/newapi","name":"deepseek-v4-pro/newapi","model":"newapi","active":false}]` +
		"\r\n[DelegationHintGuard] installed")

	llms, err := parseLLMJSONArrayFromMixedOutput(out)
	if err != nil {
		t.Fatalf("parse mixed GA output: %v", err)
	}
	if len(llms) != 2 {
		t.Fatalf("len(llms)=%d want=2: %#v", len(llms), llms)
	}
	if llms[0]["name"] != "gpt-5.5/cpa" || llms[1]["name"] != "deepseek-v4-pro/newapi" {
		t.Fatalf("unexpected llms: %#v", llms)
	}
}

func TestMarkChatLLMActiveUsesSessionLLMNo(t *testing.T) {
	llms := []map[string]interface{}{
		{"index": float64(0), "active": true},
		{"index": float64(3), "active": false},
	}

	markChatLLMActive(llms, 3)

	if llms[0]["active"] != false {
		t.Fatalf("llms[0].active=%v want false", llms[0]["active"])
	}
	if llms[1]["active"] != true {
		t.Fatalf("llms[1].active=%v want true", llms[1]["active"])
	}
}

func TestMarkChatLLMActiveAllowsIndexZero(t *testing.T) {
	llms := []map[string]interface{}{
		{"index": "0", "active": false},
		{"index": "3", "active": true},
	}

	markChatLLMActive(llms, 0)

	if llms[0]["active"] != true {
		t.Fatalf("llms[0].active=%v want true", llms[0]["active"])
	}
	if llms[1]["active"] != false {
		t.Fatalf("llms[1].active=%v want false", llms[1]["active"])
	}
}
