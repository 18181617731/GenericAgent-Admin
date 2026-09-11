package ga

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	schedulerModelDispatchMarker = "GA_ADMIN_SCHEDULE_MODEL_DISPATCH_SCHEDULER"
	agentModelDispatchMarker     = "GA_ADMIN_SCHEDULE_MODEL_DISPATCH_AGENT"
	scheduleModelKeySeparator    = "\x1f"
)

type ScheduleModelDispatchResult struct {
	Updated []string `json:"updated,omitempty"`
}

func ScheduleTaskLLMNo(raw map[string]any) (int, bool, error) {
	value, exists := raw["llm_no"]
	if !exists || value == nil || value == "" {
		return 0, false, nil
	}
	number, err := parseScheduleLLMNo(value)
	if err != nil {
		return 0, false, err
	}
	return number, true, nil
}

// ScheduleModelKey is a non-secret, stable identity for a runtime model. The
// numeric llm_no is only a list position and can change when providers are
// reordered; model/name/base together identify the same backend across those
// list changes. The separator is intentionally not user-facing.
func ScheduleModelKey(model, name, apiBase string) string {
	return strings.Join([]string{
		normalizeScheduleModelPart(model, false),
		normalizeScheduleModelPart(name, false),
		normalizeScheduleModelPart(apiBase, true),
	}, scheduleModelKeySeparator)
}

func normalizeScheduleModelPart(value string, lower bool) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, scheduleModelKeySeparator, " ")
	if lower {
		value = strings.ToLower(strings.TrimRight(value, "/"))
	}
	return value
}

// ScheduleTaskModelKey returns the stable model identity persisted in a task,
// if present. Empty strings mean “follow the scheduler model”.
func ScheduleTaskModelKey(raw map[string]any) (string, bool, error) {
	if raw == nil {
		return "", false, nil
	}
	value, exists := raw["model_key"]
	if !exists || value == nil || value == "" {
		return "", false, nil
	}
	key, ok := value.(string)
	if !ok {
		return "", false, errors.New("model_key must be a string")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false, nil
	}
	if len(key) > 2048 || strings.ContainsAny(key, "\r\n") {
		return "", false, errors.New("model_key is invalid")
	}
	return key, true, nil
}

func parseScheduleLLMNo(value any) (int, error) {
	var number int
	switch typed := value.(type) {
	case int:
		number = typed
	case int64:
		number = int(typed)
	case float64:
		if typed != float64(int(typed)) {
			return 0, errors.New("llm_no must be a non-negative integer")
		}
		number = int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, errors.New("llm_no must be a non-negative integer")
		}
		number = int(parsed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, errors.New("llm_no must be a non-negative integer")
		}
		number = parsed
	default:
		return 0, errors.New("llm_no must be a non-negative integer")
	}
	if number < 0 {
		return 0, errors.New("llm_no must be a non-negative integer")
	}
	return number, nil
}

func EnsureScheduleModelDispatch(root string) (ScheduleModelDispatchResult, error) {
	schedulerPath := filepath.Join(root, "reflect", "scheduler.py")
	agentPath := filepath.Join(root, "agentmain.py")
	scheduler, err := readDispatchScript(schedulerPath)
	if err != nil {
		return ScheduleModelDispatchResult{}, err
	}
	agent, err := readDispatchScript(agentPath)
	if err != nil {
		return ScheduleModelDispatchResult{}, err
	}
	patchedScheduler, schedulerChanged, err := patchSchedulerModelDispatch(scheduler)
	if err != nil {
		return ScheduleModelDispatchResult{}, err
	}
	patchedAgent, agentChanged, err := patchAgentModelDispatch(agent)
	if err != nil {
		return ScheduleModelDispatchResult{}, err
	}
	if !schedulerChanged && !agentChanged {
		return ScheduleModelDispatchResult{}, nil
	}
	changes := []dispatchScriptChange{}
	if schedulerChanged {
		changes = append(changes, dispatchScriptChange{path: schedulerPath, before: scheduler, after: patchedScheduler})
	}
	if agentChanged {
		changes = append(changes, dispatchScriptChange{path: agentPath, before: agent, after: patchedAgent})
	}
	if err := writeDispatchChanges(changes); err != nil {
		return ScheduleModelDispatchResult{}, err
	}
	result := ScheduleModelDispatchResult{}
	for _, change := range changes {
		rel, _ := filepath.Rel(root, change.path)
		result.Updated = append(result.Updated, filepath.ToSlash(rel))
	}
	return result, nil
}

func SchedulerRunning(root, python string) (bool, error) {
	if strings.TrimSpace(python) == "" {
		python = "python"
	}
	code := "import json; from reflect import scheduler; print(json.dumps(scheduler.runtime_state()))"
	cmd := exec.Command(python, "-c", code)
	cmd.Dir = root
	hideChildWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("check scheduler runtime: %w: %s", err, strings.TrimSpace(string(out)))
	}
	var state struct {
		Running bool `json:"running"`
	}
	if err := json.Unmarshal(out, &state); err != nil {
		return false, fmt.Errorf("parse scheduler runtime: %w", err)
	}
	return state.Running, nil
}

type dispatchScriptChange struct {
	path   string
	before string
	after  string
}

func readDispatchScript(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read schedule model dispatch runtime %s: %w", filepath.Base(path), err)
	}
	return string(data), nil
}

func writeDispatchChanges(changes []dispatchScriptChange) error {
	for _, change := range changes {
		if err := writeFileAtomic(change.path+".ga-admin.bak", []byte(change.before), 0644); err != nil {
			return fmt.Errorf("back up %s: %w", filepath.Base(change.path), err)
		}
	}
	for index, change := range changes {
		if err := writeFileAtomic(change.path, []byte(change.after), 0644); err != nil {
			for rollback := 0; rollback < index; rollback++ {
				_ = writeFileAtomic(changes[rollback].path, []byte(changes[rollback].before), 0644)
			}
			return fmt.Errorf("update %s: %w", filepath.Base(change.path), err)
		}
	}
	return nil
}

func patchSchedulerModelDispatch(source string) (string, bool, error) {
	lineEnd := "\n"
	normalized := strings.ReplaceAll(source, "\r\n", "\n")
	if strings.Contains(source, "\r\n") {
		lineEnd = "\r\n"
	}
	start := strings.Index(normalized, "        # "+schedulerModelDispatchMarker)
	if start < 0 {
		start = strings.Index(normalized, "        # \u89e6\u53d1\n")
	}
	if start < 0 {
		return source, false, errors.New("unsupported scheduler.py: expected task trigger block was not found")
	}
	end := strings.Index(normalized[start:], "\n    return None")
	if end < 0 {
		return source, false, errors.New("unsupported scheduler.py: expected task trigger block was not found")
	}
	end += start
	block := normalized[start:end]
	if strings.Contains(block, "model_key") {
		return source, false, nil
	}
	returnIndex := strings.Index(block, "return {")
	// Inject validation immediately before the return while preserving every
	// upstream field (run_id, report_path, lease state, and future additions).
	validation := `        model_key = task.get('model_key')
        if model_key is not None:
            if not isinstance(model_key, str) or not model_key.strip():
                _logger.error(f'Invalid model_key for {tid}: {model_key!r}')
                continue
            model_key = model_key.strip()
`
	if returnIndex < 0 {
		// Older schedulers returned a bare prompt string. Wrap that expression in
		// the dict consumed by the agent so the stable identity can travel with
		// it, without discarding the original prompt expression.
		returnIndex = strings.Index(block, "return ")
		if returnIndex < 0 {
			return source, false, errors.New("unsupported scheduler.py: expected task return block was not found")
		}
		lineEndIndex := strings.Index(block[returnIndex:], "\n")
		if lineEndIndex < 0 {
			lineEndIndex = len(block) - returnIndex
		}
		lineEndIndex += returnIndex
		returnLine := block[returnIndex:lineEndIndex]
		expression := strings.TrimSpace(strings.TrimPrefix(returnLine, "return "))
		if expression == "" {
			return source, false, errors.New("unsupported scheduler.py: expected task return expression was not found")
		}
		lineStart := strings.LastIndex(block[:returnIndex], "\n") + 1
		block = block[:lineStart] + validation + "        return {'prompt': " + expression + ", 'model_key': model_key}" + block[lineEndIndex:]
		if !strings.Contains(block, "# "+schedulerModelDispatchMarker) {
			block = "        # " + schedulerModelDispatchMarker + "\n" + block
		}
		patched := normalized[:start] + block + normalized[end:]
		if patched == normalized {
			return source, false, nil
		}
		return strings.ReplaceAll(patched, "\n", lineEnd), true, nil
	}
	lineStart := strings.LastIndex(block[:returnIndex], "\n") + 1
	block = block[:lineStart] + validation + block[lineStart:]
	// Recompute the return location after inserting validation and add the
	// optional identity to the existing dictionary without rewriting it.
	returnIndex = strings.Index(block, "return {")
	closeIndex := strings.Index(block[returnIndex:], "}") + returnIndex
	block = block[:closeIndex] + ", 'model_key': model_key" + block[closeIndex:]
	patched := normalized[:start] + block + normalized[end:]
	if patched == normalized {
		return source, false, nil
	}
	return strings.ReplaceAll(patched, "\n", lineEnd), true, nil
}

func patchAgentModelDispatch(source string) (string, bool, error) {
	lineEnd := "\n"
	normalized := strings.ReplaceAll(source, "\r\n", "\n")
	if strings.Contains(source, "\r\n") {
		lineEnd = "\r\n"
	}
	start := strings.Index(normalized, "            if task and task == '/exit': break\n")
	if start < 0 {
		return source, false, errors.New("unsupported agentmain.py: expected reflect task block was not found")
	}
	end := strings.Index(normalized[start:], "            time.sleep(getattr(mod, 'INTERVAL', 5))")
	if end < 0 {
		return source, false, errors.New("unsupported agentmain.py: expected reflect task block was not found")
	}
	end += start
	block := normalized[start:end]
	if strings.Contains(block, "task_model_key") {
		return source, false, nil
	}
	// Keep the runtime's existing execution, timeout, report, and cleanup
	// behavior. Only extend the task envelope and resolve a stable model key.
	needle := "            task_prompt, task_llm_no = task, None"
	if !strings.Contains(block, needle) {
		// A pre-envelope runtime only exposes `task` as a string. Keep support
		// for that contract by upgrading this whole small dispatch block; newer
		// runtimes take the minimal path below so their extra bookkeeping stays
		// untouched.
		legacy := `            if task and task == '/exit': break
            # GA_ADMIN_SCHEDULE_MODEL_DISPATCH_AGENT
            task_prompt, task_llm_no, task_model_key = task, None, None
            if isinstance(task, dict):
                task_prompt = task.get('prompt')
                task_llm_no = task.get('llm_no')
                task_model_key = task.get('model_key')
            if task_prompt:
                previous_llm_no, switched_llm = agent.llm_no, False
                try:
                    if task_model_key is not None:
                        if not isinstance(task_model_key, str) or not task_model_key.strip():
                            raise ValueError('invalid scheduled task model key')
                        def _ga_admin_schedule_model_key(client):
                            backend = client.get('backend') if isinstance(client, dict) else getattr(client, 'backend', None)
                            if backend is None:
                                return ''
                            config = getattr(backend, 'config', {})
                            if not isinstance(config, dict): config = {}
                            api_base = (getattr(backend, 'apibase', '') or getattr(backend, 'api_base', '') or getattr(backend, 'base_url', '') or config.get('apibase', '') or config.get('api_base', '') or config.get('base_url', ''))
                            clean = lambda value: str(value or '').strip().replace('\x1f', ' ')
                            return clean(getattr(backend, 'model', '')) + '\x1f' + clean(getattr(backend, 'name', '')) + '\x1f' + clean(api_base).lower().rstrip('/')
                        task_model_key = task_model_key.strip()
                        resolved_llm_no = None
                        for candidate_index, candidate in enumerate(getattr(agent, 'llmclients', []) or []):
                            if _ga_admin_schedule_model_key(candidate) == task_model_key:
                                resolved_llm_no = candidate_index
                                break
                        if resolved_llm_no is None:
                            raise ValueError('scheduled task model is unavailable')
                        task_llm_no = resolved_llm_no
                    if task_llm_no is not None:
                        task_llm_no = int(task_llm_no)
                        if task_llm_no < 0: raise ValueError('negative llm_no')
                        agent.next_llm(task_llm_no); switched_llm = True
                        print(f'[Reflect] switched to model #{agent.llm_no}')
                    print(f'[Reflect] triggered: {str(task_prompt)[:80]}')
                    dq = agent.put_task(task_prompt, source='reflect')
                    while 'done' not in (item := dq.get(timeout=2200)): pass
                    result = item['done']
                    print(result)
                except Exception as e:
                    if getattr(mod, 'ONCE', False): raise
                    print(f'[Reflect] drain error: {e}'); result = f'[ERROR] {e}'
                finally:
                    if switched_llm:
                        try: agent.next_llm(previous_llm_no)
                        except Exception as e: print(f'[Reflect] restore model error: {e}')
                log_dir = os.path.join(script_dir, 'temp/reflect_logs'); os.makedirs(log_dir, exist_ok=True)
                script_name = os.path.splitext(os.path.basename(args.reflect))[0]
                open(os.path.join(log_dir, f'{script_name}_{datetime.now():%Y-%m-%d}.log'), 'a', encoding='utf-8').write(f'[{datetime.now():%m-%d %H:%M}]\n{result}\n\n')
                if (on_done := getattr(mod, 'on_done', None)):
                    try: on_done(result)
                    except Exception as e: print(f'[Reflect] on_done error: {e}')
                if getattr(mod, 'ONCE', False): print('[Reflect] ONCE=True, exiting.'); break
`
		patched := normalized[:start] + legacy + normalized[end:]
		if patched == normalized {
			return source, false, nil
		}
		return strings.ReplaceAll(patched, "\n", lineEnd), true, nil
	}
	block = strings.Replace(block, needle, "            task_prompt, task_llm_no, task_model_key = task, None, None", 1)
	needle = "                task_llm_no = task.get('llm_no')"
	if !strings.Contains(block, needle) {
		return source, false, errors.New("unsupported agentmain.py: expected task model field was not found")
	}
	block = strings.Replace(block, needle, needle+"\n                task_model_key = task.get('model_key')", 1)
	needle = "                    if task_llm_no is not None:"
	if !strings.Contains(block, needle) {
		return source, false, errors.New("unsupported agentmain.py: expected model switch block was not found")
	}
	resolution := `                    if task_model_key is not None:
                        if not isinstance(task_model_key, str) or not task_model_key.strip():
                            raise ValueError('invalid scheduled task model key')
                        def _ga_admin_schedule_model_key(client):
                            backend = client.get('backend') if isinstance(client, dict) else getattr(client, 'backend', None)
                            if backend is None:
                                return ''
                            config = getattr(backend, 'config', {})
                            if not isinstance(config, dict): config = {}
                            api_base = (getattr(backend, 'apibase', '') or getattr(backend, 'api_base', '') or getattr(backend, 'base_url', '') or config.get('apibase', '') or config.get('api_base', '') or config.get('base_url', ''))
                            clean = lambda value: str(value or '').strip().replace('\x1f', ' ')
                            return clean(getattr(backend, 'model', '')) + '\x1f' + clean(getattr(backend, 'name', '')) + '\x1f' + clean(api_base).lower().rstrip('/')
                        task_model_key = task_model_key.strip()
                        resolved_llm_no = None
                        for candidate_index, candidate in enumerate(getattr(agent, 'llmclients', []) or []):
                            if _ga_admin_schedule_model_key(candidate) == task_model_key:
                                resolved_llm_no = candidate_index
                                break
                        if resolved_llm_no is None:
                            raise ValueError('scheduled task model is unavailable')
                        task_llm_no = resolved_llm_no
`
	block = strings.Replace(block, needle, resolution+needle, 1)
	patched := normalized[:start] + block + normalized[end:]
	if patched == normalized {
		return source, false, nil
	}
	return strings.ReplaceAll(patched, "\n", lineEnd), true, nil
}
