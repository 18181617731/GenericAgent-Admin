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
	if strings.Contains(source, schedulerModelDispatchMarker) {
		return source, false, nil
	}
	lineEnd := "\n"
	normalized := strings.ReplaceAll(source, "\r\n", "\n")
	if strings.Contains(source, "\r\n") {
		lineEnd = "\r\n"
	}
	start := strings.Index(normalized, "        # \u89e6\u53d1\n")
	if start < 0 {
		return source, false, errors.New("unsupported scheduler.py: expected task trigger block was not found")
	}
	end := strings.Index(normalized[start:], "\n    return None")
	if end < 0 {
		return source, false, errors.New("unsupported scheduler.py: expected task trigger block was not found")
	}
	end += start
	block := `        # GA_ADMIN_SCHEDULE_MODEL_DISPATCH_SCHEDULER
        _logger.info(f'TRIGGER {tid} (repeat={repeat}, schedule={sched}, last_run={last})')
        ts = now.strftime('%Y-%m-%d_%H%M')
        rpt = os.path.join(DONE, f'{ts}_{tid}.md')
        task_prompt = (f'[\u5b9a\u65f6\u4efb\u52a1] {tid}\n'
                       f'[\u62a5\u544a\u8def\u5f84] {rpt}\n\n'
                       f'\u5148\u8bfb scheduled_task_sop \u4e86\u89e3\u6267\u884c\u6d41\u7a0b\uff0c\u7136\u540e\u6267\u884c\u4ee5\u4e0b\u4efb\u52a1\uff1a\n\n'
                       f'{task.get("prompt", "")}\n\n'
                       f'\u5b8c\u6210\u540e\u5c06\u6267\u884c\u62a5\u544a\u5199\u5165 {rpt}\u3002')
        llm_no = task.get('llm_no')
        if llm_no is not None:
            try:
                llm_no = int(llm_no)
                if llm_no < 0:
                    raise ValueError('negative llm_no')
            except (TypeError, ValueError):
                _logger.error(f'Invalid llm_no for {tid}: {llm_no!r}')
                continue
        return {'prompt': task_prompt, 'llm_no': llm_no, 'task_id': tid}
`
	return strings.ReplaceAll(normalized[:start]+block+normalized[end:], "\n", lineEnd), true, nil
}

func patchAgentModelDispatch(source string) (string, bool, error) {
	if strings.Contains(source, agentModelDispatchMarker) {
		return source, false, nil
	}
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
	block := `            if task and task == '/exit': break
            # GA_ADMIN_SCHEDULE_MODEL_DISPATCH_AGENT
            task_prompt, task_llm_no = task, None
            if isinstance(task, dict):
                task_prompt = task.get('prompt')
                task_llm_no = task.get('llm_no')
            if task_prompt:
                previous_llm_no, switched_llm = agent.llm_no, False
                try:
                    if task_llm_no is not None:
                        task_llm_no = int(task_llm_no)
                        if task_llm_no < 0: raise ValueError('negative llm_no')
                        agent.next_llm(task_llm_no); switched_llm = True
                        print(f'[Reflect] switched to model #{agent.llm_no}')
                    print(f'[Reflect] triggered: {str(task_prompt)[:80]}')
                    dq = agent.put_task(task_prompt, source='reflect')
                    while 'done' not in (item := dq.get(timeout=1200)): pass
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
	return strings.ReplaceAll(normalized[:start]+block+normalized[end:], "\n", lineEnd), true, nil
}
