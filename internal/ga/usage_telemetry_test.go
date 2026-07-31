package ga

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchUsageTelemetryAddsFlushAndMaxEffortSupport(t *testing.T) {
	patched, changed, err := patchUsageTelemetry(usageTelemetryFixture())
	if err != nil {
		t.Fatalf("patch usage telemetry: %v", err)
	}
	if !changed {
		t.Fatal("expected fixture to be patched")
	}
	for _, needle := range []string{
		usageTelemetryMarker,
		"_admin_usage_observe(usage, api_mode)",
		"_admin_usage_set_session(sess)",
		"_admin_usage_flush()",
		"'max': 'max'",
		"_record_usage(out_usage, \"messages\")",
	} {
		if !strings.Contains(patched, needle) {
			t.Fatalf("patched source is missing %q", needle)
		}
	}
}

func TestEnsureUsageTelemetryIsIdempotent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "llmcore.py")
	original := usageTelemetryFixture()
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	first, err := EnsureUsageTelemetry(root)
	if err != nil {
		t.Fatalf("first patch: %v", err)
	}
	if len(first.Updated) != 1 || first.Updated[0] != "llmcore.py" {
		t.Fatalf("first result=%+v", first)
	}
	patched, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".ga-admin.bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != original || !strings.Contains(string(patched), usageTelemetryMarker) {
		t.Fatal("patch did not preserve backup or install marker")
	}

	second, err := EnsureUsageTelemetry(root)
	if err != nil {
		t.Fatalf("second patch: %v", err)
	}
	if len(second.Updated) != 0 {
		t.Fatalf("second result=%+v, want no update", second)
	}
}

func usageTelemetryFixture() string {
	return `import os, json, re, time, threading, pathlib, uuid

def _parse_claude_sse(resp_lines):
    return []

def _record_usage(usage, api_mode):
    if not usage: return

def _stream_with_retry(sess, url, headers, payload, parse_fn):
    for attempt in range(sess.max_retries + 1):
        streamed = False
        try:
            gen = parse_fn(None)
            try:
                while True: chunk = next(gen); streamed = True; yield chunk
            except StopIteration as e:
                if not e.value and not streamed: raise requests.ConnectionError("empty response")
                return e.value or []
        except Exception:
            return []

def _apply_claude_thinking(self, payload):
    if self.reasoning_effort:
        effort = {'low': 'low', 'medium': 'medium', 'high': 'high', 'xhigh': 'max'}.get(self.reasoning_effort)
        if effort: payload["output_config"] = {"effort": effort}

def _parse_claude_output(out_usage, out_tokens, stop_reason):
    if out_tokens: print(f"[Output] tokens={out_tokens} stop_reason={stop_reason}")

class BaseSession:
    def __init__(self):
        self.reasoning_effort = _enum('reasoning_effort', {'none', 'minimal', 'low', 'medium', 'high', 'xhigh'})
`
}
