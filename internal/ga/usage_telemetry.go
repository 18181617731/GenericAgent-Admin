package ga

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const usageTelemetryMarker = "GA_ADMIN_USAGE_TELEMETRY"

type UsageTelemetryResult struct {
	Updated []string `json:"updated,omitempty"`
}

func EnsureUsageTelemetry(root string) (UsageTelemetryResult, error) {
	path := filepath.Join(strings.TrimSpace(root), "llmcore.py")
	data, err := os.ReadFile(path)
	if err != nil {
		return UsageTelemetryResult{}, fmt.Errorf("read llmcore.py for usage telemetry: %w", err)
	}
	patched, changed, err := patchUsageTelemetry(string(data))
	if err != nil {
		return UsageTelemetryResult{}, err
	}
	if !changed {
		return UsageTelemetryResult{}, nil
	}
	backup := path + ".ga-admin.bak"
	if err := writeFileAtomic(backup, data, 0644); err != nil {
		return UsageTelemetryResult{}, fmt.Errorf("backup llmcore.py for usage telemetry: %w", err)
	}
	if err := writeFileAtomic(path, []byte(patched), 0644); err != nil {
		return UsageTelemetryResult{}, fmt.Errorf("update llmcore.py for usage telemetry: %w", err)
	}
	return UsageTelemetryResult{Updated: []string{"llmcore.py"}}, nil
}

func patchUsageTelemetry(source string) (string, bool, error) {
	if strings.Contains(source, usageTelemetryMarker) {
		return source, false, nil
	}
	normalized := strings.ReplaceAll(source, "\r\n", "\n")
	lineEnd := "\n"
	if strings.Contains(source, "\r\n") {
		lineEnd = "\r\n"
	}
	recordStart := strings.Index(normalized, "def _record_usage(usage, api_mode):\n")
	streamStart := strings.Index(normalized, "def _stream_with_retry(sess, url, headers, payload, parse_fn):\n")
	claudeOutput := "if out_tokens: print(f\"[Output] tokens={out_tokens} stop_reason={stop_reason}\")"
	if recordStart < 0 || streamStart < 0 || !strings.Contains(normalized, claudeOutput) {
		return source, false, errors.New("unsupported llmcore.py: usage telemetry hooks were not found")
	}
	helper := `# GA_ADMIN_USAGE_TELEMETRY_BEGIN
_ADMIN_USAGE_TLS = threading.local()
_ADMIN_USAGE_WRITE_LOCK = threading.Lock()

def _admin_usage_set_session(sess):
    model = getattr(sess, 'model', '')
    effort = getattr(sess, 'reasoning_effort', None)
    _ADMIN_USAGE_TLS.context = {
        'model_id': ' '.join(str(model or '').split())[:256],
        'reasoning_effort': str(effort or 'off').strip().lower() or 'off',
        'started': time.time(),
        'pending': {},
    }

def _admin_usage_numbers(usage, api_mode):
    if not isinstance(usage, dict):
        return {}
    if api_mode == 'responses':
        cached = (usage.get('input_tokens_details') or {}).get('cached_tokens', 0)
        inp = usage.get('input_tokens', 0)
        out = usage.get('output_tokens', 0)
    elif api_mode == 'messages':
        cached = usage.get('cache_read_input_tokens', 0)
        inp = (usage.get('input_tokens', 0) + usage.get('cache_creation_input_tokens', 0) + cached)
        out = usage.get('output_tokens', 0)
    else:
        cached = (usage.get('prompt_tokens_details') or {}).get('cached_tokens', 0)
        inp = usage.get('prompt_tokens', usage.get('input_tokens', 0))
        out = usage.get('completion_tokens', usage.get('output_tokens', 0))
    try:
        inp, out, cached = max(0, int(inp or 0)), max(0, int(out or 0)), max(0, int(cached or 0))
    except (TypeError, ValueError):
        return {}
    return {'input_tokens': inp, 'output_tokens': out, 'cached_tokens': cached, 'total_tokens': inp + out}

def _admin_usage_add(left, right):
    return {key: int(left.get(key, 0)) + int(right.get(key, 0)) for key in ('input_tokens', 'output_tokens', 'cached_tokens', 'total_tokens')}

def _admin_usage_emit(context, totals):
    directory = os.environ.get('GA_ADMIN_USAGE_DIR', '').strip()
    if not directory or totals.get('total_tokens', 0) <= 0:
        return
    channel = os.environ.get('GA_ADMIN_USAGE_CHANNEL', 'other').strip() or 'other'
    safe_channel = re.sub(r'[^A-Za-z0-9_.-]+', '_', channel)[:40] or 'other'
    llm_no = os.environ.get('GA_ADMIN_LLM_NO', '').strip()
    try:
        llm_no = int(llm_no)
    except (TypeError, ValueError):
        llm_no = None
    event = {
        'id': uuid.uuid4().hex,
        'created_at': int(time.time()),
        'elapsed_ms': max(1, int((time.time() - context.get('started', time.time())) * 1000)),
        'channel': channel,
        'source': os.environ.get('GA_ADMIN_USAGE_SOURCE', '').strip(),
        'session_id': os.environ.get('GA_ADMIN_USAGE_SESSION_ID', '').strip(),
        'session_name': os.environ.get('GA_ADMIN_USAGE_SESSION_NAME', '').strip(),
        'model_id': context.get('model_id', ''),
        'llm_no': llm_no,
        'reasoning_effort': context.get('reasoning_effort', 'off'),
        'totals': totals,
    }
    try:
        pathlib.Path(directory).mkdir(parents=True, exist_ok=True)
        path = pathlib.Path(directory) / ('ga-' + safe_channel + '-' + str(os.getpid()) + '.jsonl')
        with _ADMIN_USAGE_WRITE_LOCK:
            with open(path, 'a', encoding='utf-8') as handle:
                handle.write(json.dumps(event, ensure_ascii=False, separators=(',', ':')) + '\n')
    except Exception as error:
        print('[WARN] Admin usage telemetry write failed: %s' % error)

def _admin_usage_observe(usage, api_mode):
    context = getattr(_ADMIN_USAGE_TLS, 'context', None)
    if context is None:
        return
    current = _admin_usage_numbers(usage, api_mode)
    if not current:
        return
    if current.get('output_tokens', 0) <= 0:
        context['pending'] = _admin_usage_add(context.get('pending', {}), current)
        return
    totals = _admin_usage_add(context.get('pending', {}), current)
    context['pending'] = {}
    _admin_usage_emit(context, totals)

def _admin_usage_flush():
    context = getattr(_ADMIN_USAGE_TLS, 'context', None)
    if context is None:
        return
    pending = context.get('pending', {})
    context['pending'] = {}
    if pending:
        _admin_usage_emit(context, pending)
# GA_ADMIN_USAGE_TELEMETRY_END

`
	patched := normalized[:recordStart] + helper + normalized[recordStart:]
	recordSignature := "def _record_usage(usage, api_mode):\n"
	patched = strings.Replace(patched, recordSignature, recordSignature+"    _admin_usage_observe(usage, api_mode)\n", 1)
	streamSignature := "def _stream_with_retry(sess, url, headers, payload, parse_fn):\n"
	patched = strings.Replace(patched, streamSignature, streamSignature+"    _admin_usage_set_session(sess)\n", 1)
	streamReturn := "                    if not e.value and not streamed: raise requests.ConnectionError(\"empty response\")\n                    return e.value or []"
	patched = strings.Replace(patched, streamReturn, "                    if not e.value and not streamed: raise requests.ConnectionError(\"empty response\")\n                    _admin_usage_flush()\n                    return e.value or []", 1)
	patched = strings.Replace(patched, claudeOutput, "if out_tokens:\n                _record_usage(out_usage, \"messages\")\n                print(f\"[Output] tokens={out_tokens} stop_reason={stop_reason}\")", 1)
	patched = strings.Replace(patched, "{'none', 'minimal', 'low', 'medium', 'high', 'xhigh'}", "{'none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'}", 1)
	patched = strings.Replace(patched, "effort = {'low': 'low', 'medium': 'medium', 'high': 'high', 'xhigh': 'max'}.get(self.reasoning_effort)", "effort = {'low': 'low', 'medium': 'medium', 'high': 'high', 'xhigh': 'max', 'max': 'max'}.get(self.reasoning_effort)", 1)
	if !strings.Contains(patched, usageTelemetryMarker) || !strings.Contains(patched, "_admin_usage_observe(usage, api_mode)") || !strings.Contains(patched, "_admin_usage_set_session(sess)") || !strings.Contains(patched, "_admin_usage_flush()") {
		return source, false, errors.New("failed to install llmcore.py usage telemetry hooks")
	}
	return strings.ReplaceAll(patched, "\n", lineEnd), true, nil
}
