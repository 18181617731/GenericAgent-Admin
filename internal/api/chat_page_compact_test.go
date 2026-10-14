package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func compactFixture() chatSession {
	cs := pageFixture(2)
	cs.Messages[1].Role = "assistant"
	cs.Messages[1].Content = "**LLM Running (Turn 1)...**\n<summary>working</summary>\n" + strings.Repeat("process content\n", 1000) + "\n```\n[Info] Final response to user.\n```\n<summary>done</summary>\nFinal answer"
	cs.Messages[1].Outputs = []string{strings.Repeat("outputs", 1000)}
	cs.Messages[1].StructuredContent = []map[string]interface{}{{"type": "text", "text": cs.Messages[1].Content}}
	return cs
}

func TestChatPageLazyExecution(t *testing.T) {
	cs := compactFixture()
	before, _ := json.Marshal(cs)
	full, _, _ := chatSessionView(cs, httptest.NewRequest("GET", "/?view=page", nil))
	value, status, err := chatSessionView(cs, httptest.NewRequest("GET", "/?view=page&execution=lazy", nil))
	if err != nil || status != http.StatusOK {
		t.Fatal(status, err)
	}
	page := value.(map[string]interface{})
	m := page["messages"].([]chatPageMessage)[1]
	original := full.(map[string]interface{})["messages"].([]chatPageMessage)[1]
	if !m.ContentPartial || m.ExecutionCount != 1 || strings.TrimSpace(m.Content) != "<summary>done</summary>\nFinal answer" {
		t.Fatalf("bad compact message: %+v", m)
	}
	if m.ContentRevision != original.ContentRevision || m.Outputs != nil || m.StructuredContent != nil {
		t.Fatal("revision changed or process duplicated")
	}
	if !reflect.DeepEqual(page["message_index"], full.(map[string]interface{})["message_index"]) {
		t.Fatal("view changed index")
	}
	restored, status, err := chatSessionView(cs, httptest.NewRequest("GET", fmt.Sprintf("/?view=message&message_id=%s&revision=%s", m.ID, m.ContentRevision), nil))
	if err != nil || status != 200 || !reflect.DeepEqual(restored.(chatPageMessage), original) {
		t.Fatal("full body not restored", status, err)
	}
	serialized, _ := json.Marshal(m)
	if !strings.Contains(string(serialized), `"content_partial":true`) || !strings.Contains(string(serialized), `"execution_count":1`) {
		t.Fatal("page metadata lost")
	}
	after, _ := json.Marshal(cs)
	if string(before) != string(after) {
		t.Fatal("view mutated stored session")
	}
	_, status, _ = chatSessionView(cs, httptest.NewRequest("GET", fmt.Sprintf("/?view=message&message_id=%s&revision=old", m.ID), nil))
	if status != 409 {
		t.Fatal("old revision not rejected")
	}
}

func TestChatPageCompactConservativeMarkers(t *testing.T) {
	prefix := "LLM Running (Turn 1)\n" + strings.Repeat("process\n", 1500)
	for name, content := range map[string]string{
		"inline":         prefix + "```[Info] Final response to user.```\nAnswer",
		"crlf":           strings.ReplaceAll(prefix+"```\n[Info] Final response to user.\n```\nAnswer", "\n", "\r\n"),
		"fenced-example": prefix + "~~~example\nLLM Running (Turn 99)\n```[Info] Final response to user.```\n~~~\n```[Info] Final response to user.```\nAnswer",
	} {
		t.Run(name, func(t *testing.T) {
			m := compactChatPageMessage(chatPageMessage{chatMessage: chatMessage{Role: "assistant", Content: content}})
			if !m.ContentPartial || m.ExecutionCount != 1 || strings.TrimSpace(m.Content) != "Answer" {
				t.Fatalf("bad markers: %+v", m)
			}
		})
	}
	for name, content := range map[string]string{
		"no-final":      prefix,
		"empty-final":   prefix + "```[Info] Final response to user.```\n",
		"no-turn":       strings.Repeat("plain\n", 1500) + "```[Info] Final response to user.```\nAnswer",
		"unclosed-code": prefix + "~~~\n```[Info] Final response to user.```\nAnswer",
		"only-fenced":   "~~~\n" + prefix + "```[Info] Final response to user.```\n~~~",
	} {
		t.Run(name, func(t *testing.T) {
			m := chatPageMessage{chatMessage: chatMessage{Role: "assistant", Content: content}}
			if !reflect.DeepEqual(m, compactChatPageMessage(m)) {
				t.Fatal("ambiguous content compacted")
			}
		})
	}
	for _, role := range []string{"user", "system"} {
		m := chatPageMessage{chatMessage: chatMessage{Role: role, Content: prefix + "```[Info] Final response to user.```\nAnswer"}}
		if !reflect.DeepEqual(m, compactChatPageMessage(m)) {
			t.Fatal("non-assistant compacted")
		}
	}
}
