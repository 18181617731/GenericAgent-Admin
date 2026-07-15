package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestModelsProbeRequiresSuccessfulChatInsteadOfListedModelID(t *testing.T) {
	fixed := time.Date(2026, time.July, 15, 14, 35, 0, 0, time.FixedZone("CST", 8*60*60))
	oldNow := modelProbeNow
	modelProbeNow = func() time.Time { return fixed }
	t.Cleanup(func() { modelProbeNow = oldNow })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%q want /v1/chat/completions", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-probe-test" {
			t.Errorf("Authorization=%q", r.Header.Get("Authorization"))
		}
		var payload struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(payload.Messages) < 2 || !strings.Contains(payload.Messages[0].Content, "2026-07-15 14:35") {
			t.Errorf("request did not provide trusted Beijing time: %#v", payload.Messages)
		}
		if !payload.Stream {
			t.Error("probe request must use streaming mode")
		}
		if payload.Model == "listed-but-broken" {
			http.Error(w, `{"error":{"message":"model unavailable"}}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"2026-07-15 \"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"14:35\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer upstream.Close()

	s := newModelTestServer(t, t.TempDir())
	rr := postModelProbe(t, s, map[string]interface{}{
		"protocol": "native_oai",
		"base_url": upstream.URL + "/v1",
		"api_key":  "sk-probe-test",
		"models":   []string{"listed-but-broken", "working-model"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	result := decodeModelProbeResponse(t, rr)
	if len(result.Results) != 2 {
		t.Fatalf("results=%#v", result.Results)
	}
	if result.Results[0].Available || result.Results[0].Status != "request_failed" {
		t.Fatalf("listed broken model was accepted: %#v", result.Results[0])
	}
	if !result.Results[1].Available || result.Results[1].Status != "available" {
		t.Fatalf("working model was rejected: %#v", result.Results[1])
	}
	if strings.Contains(rr.Body.String(), "sk-probe-test") {
		t.Fatalf("response leaked API key: %s", rr.Body.String())
	}
}

func TestModelsProbeRejectsSuccessfulResponseWithWrongBeijingTime(t *testing.T) {
	fixed := time.Date(2026, time.July, 15, 14, 35, 0, 0, time.FixedZone("CST", 8*60*60))
	oldNow := modelProbeNow
	modelProbeNow = func() time.Time { return fixed }
	t.Cleanup(func() { modelProbeNow = oldNow })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"2025-01-01 00:00"}}]}`))
	}))
	defer upstream.Close()
	s := newModelTestServer(t, t.TempDir())
	rr := postModelProbe(t, s, map[string]interface{}{
		"protocol": "native_oai", "base_url": upstream.URL, "models": []string{"wrong-clock"},
	})
	result := decodeModelProbeResponse(t, rr)
	if len(result.Results) != 1 || result.Results[0].Available || result.Results[0].Status != "invalid_answer" {
		t.Fatalf("wrong time response was accepted: %#v", result.Results)
	}
}

func TestModelsProbeRejectsAmbiguousAnswerContainingWrongTime(t *testing.T) {
	if validModelProbeAnswer("服务器提示 2026-07-15 14:35，但我认为现在是 2025-01-01 00:00", "2026-07-15 14:35") {
		t.Fatal("ambiguous answer containing a wrong timestamp was accepted")
	}
	if !validModelProbeAnswer("北京时间：2026-07-15  14:35。", "2026-07-15 14:35") {
		t.Fatal("correct answer with harmless formatting was rejected")
	}
}

func TestModelsProbeSupportsClaudeMessagesProtocol(t *testing.T) {
	fixed := time.Date(2026, time.July, 15, 14, 35, 0, 0, time.FixedZone("CST", 8*60*60))
	oldNow := modelProbeNow
	modelProbeNow = func() time.Time { return fixed }
	t.Cleanup(func() { modelProbeNow = oldNow })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "sk-ant-probe" {
			t.Errorf("path=%q x-api-key=%q", r.URL.Path, r.Header.Get("x-api-key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"2026-07-15 14:35"}]}`))
	}))
	defer upstream.Close()
	s := newModelTestServer(t, t.TempDir())
	rr := postModelProbe(t, s, map[string]interface{}{
		"protocol": "native_claude", "base_url": upstream.URL + "/v1", "api_key": "sk-ant-probe", "models": []string{"claude-working"},
	})
	result := decodeModelProbeResponse(t, rr)
	if len(result.Results) != 1 || !result.Results[0].Available {
		t.Fatalf("Claude model was rejected: status=%d results=%#v body=%s", rr.Code, result.Results, rr.Body.String())
	}
}

func TestModelsProbeUsesConfiguredOpenAIResponsesMode(t *testing.T) {
	fixed := time.Date(2026, time.July, 15, 14, 35, 0, 0, time.FixedZone("CST", 8*60*60))
	oldNow := modelProbeNow
	modelProbeNow = func() time.Time { return fixed }
	t.Cleanup(func() { modelProbeNow = oldNow })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path=%q want /v1/responses", r.URL.Path)
		}
		if r.Header.Get("originator") != "codex_exec" || r.Header.Get("User-Agent") != "probe-client" {
			t.Errorf("originator=%q user-agent=%q", r.Header.Get("originator"), r.Header.Get("User-Agent"))
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload["input"] != "现在北京时间几点了？只回答 YYYY-MM-DD HH:mm。" || payload["stream"] != true {
			t.Errorf("responses payload=%#v", payload)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"2026-07-15 14:35\"}\n\ndata: [DONE]\n\n"))
	}))
	defer upstream.Close()

	s := newModelTestServer(t, t.TempDir())
	rr := postModelProbe(t, s, map[string]interface{}{
		"protocol": "native_oai", "base_url": upstream.URL + "/v1", "models": []string{"responses-model"},
		"model_options": map[string]interface{}{"responses-model": map[string]string{"api_mode": "responses", "user_agent": "probe-client"}},
	})
	result := decodeModelProbeResponse(t, rr)
	if len(result.Results) != 1 || !result.Results[0].Available {
		t.Fatalf("Responses model was rejected: status=%d results=%#v body=%s", rr.Code, result.Results, rr.Body.String())
	}
}

func postModelProbe(t *testing.T, s *Server, payload map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/probe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.Routes().ServeHTTP(rr, req)
	return rr
}

func decodeModelProbeResponse(t *testing.T, rr *httptest.ResponseRecorder) modelProbeResponse {
	t.Helper()
	var response modelProbeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rr.Body.String())
	}
	return response
}
