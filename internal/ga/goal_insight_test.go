package ga

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGoalSessionLog(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "model_responses_123456.txt")
	content := "=== Prompt === 2026-07-27 08:31:32\n" +
		"{\"role\": \"user\", \"content\": \"[Goal Mode — persistent]\\n<objective>\\n分析代码\\n</objective>\\n第 1 次唤醒。\"}\n" +
		"=== Response === 2026-07-27 08:31:44 model=test'\n" +
		"<summary>启动：读取代码结构</summary>\n正文\n" +
		"=== Prompt === 2026-07-27 08:32:00\n" +
		"{\"role\": \"tool\", \"content\": \"no marker\"}\n" +
		"=== Response === 2026-07-27 08:32:10 model=test'\n" +
		"无摘要响应\n" +
		"=== Prompt === 2026-07-27 08:33:00\n" +
		"{\"role\": \"user\", \"content\": \"[Goal Mode — persistent] 第 2 次唤醒。\"}\n" +
		"=== Response === 2026-07-27 08:33:20 model=test'\n" +
		"<summary>完成分析</summary>\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parseGoalSessionLog(p, 200)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found || got.Total != 2 || got.Wakes != 2 || len(got.Entries) != 2 {
		t.Fatalf("unexpected: %+v", got)
	}
	e0, e1 := got.Entries[0], got.Entries[1]
	if e0.Turn != 1 || e1.Turn != 2 || e0.Time != "08:31:44" || !strings.Contains(e0.Text, "启动") || e1.Text != "完成分析" {
		t.Fatalf("entries: %+v", got.Entries)
	}
}

func TestParseGoalSessionLogLimit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "model_responses_654321.txt")
	var b strings.Builder
	for i := 0; i < 5; i++ {
		b.WriteString("=== Response === 2026-07-27 09:00:0" + string(rune('0'+i)) + " model=x'\n")
		b.WriteString("<summary>step" + string(rune('a'+i)) + "</summary>\n")
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parseGoalSessionLog(p, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 5 || !got.Truncated || len(got.Entries) != 2 || got.Entries[1].Text != "stepe" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestGoalObjectiveNeedle(t *testing.T) {
	if got := goalObjectiveNeedle("short\n这是一个比较长的目标行"); got != "这是一个比较长的目标行" {
		t.Fatalf("longest got %q", got)
	}
	if got := goalObjectiveNeedle("带\"引号\"目标"); got != `带\"引号\"目标` {
		t.Fatalf("escape got %q", got)
	}
	if goalObjectiveNeedle("   ") != "" {
		t.Fatal("blank objective should give empty needle")
	}
}
