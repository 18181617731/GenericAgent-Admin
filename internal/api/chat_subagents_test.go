package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractSubagentTaskNames(t *testing.T) {
	msgs := []chatMessage{
		{Role: "assistant", Content: "run: python agentmain.py --task sup_c_test2 --input \"hi\""},
		{Role: "assistant", Content: "again --task sup_c_test2, and --task other_one now"},
		{Role: "user", Content: "please use --task manual.name"},
		{Role: "assistant", Content: "bad: --task ../evil and --task \"quoted_ok\""},
		{Role: "system", Content: "--task ignored_role"},
	}
	got := extractSubagentTaskNames(msgs)
	want := []string{"sup_c_test2", "other_one", "manual.name", "quoted_ok"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestCollectSubagentStatus(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "temp", "job1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("output.txt", "<summary>round one</summary>\nwork...\n[ROUND END]\n")
	write("output1.txt", "<summary>old</summary>\n...\n<summary>latest round two</summary>\nstill going\n")
	write("reply.txt", "reply body")
	write("expected.md", "# contract")

	st := collectSubagentStatus(root, "job1")
	if !st.Exists {
		t.Fatal("expected dir to exist")
	}
	if st.Rounds != 2 {
		t.Fatalf("rounds = %d, want 2", st.Rounds)
	}
	if st.RoundEnded {
		t.Fatal("latest output (output1.txt) has no [ROUND END], RoundEnded should be false")
	}
	if st.LatestSummary != "latest round two" {
		t.Fatalf("summary = %q", st.LatestSummary)
	}
	if !st.HasReply || !st.HasExpected {
		t.Fatalf("flags: reply=%v expected=%v", st.HasReply, st.HasExpected)
	}
	if st.StopRequested || st.Intervened {
		t.Fatal("no control files were written")
	}
	if st.UpdatedAt == 0 {
		t.Fatal("UpdatedAt should be set from output mtime")
	}

	// missing dir
	miss := collectSubagentStatus(root, "nope")
	if miss.Exists {
		t.Fatal("missing dir must report Exists=false")
	}
}
