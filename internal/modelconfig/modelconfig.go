package modelconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

type OptionalBool bool

func (b *OptionalBool) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch v := raw.(type) {
	case bool:
		*b = OptionalBool(v)
		return nil
	case string:
		parsed := parseOptionalBoolString(v)
		*b = OptionalBool(parsed)
		return nil
	default:
		return fmt.Errorf("fake_cc_system_prompt must be a boolean or boolean string")
	}
}

func parseOptionalBoolString(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "f", "no", "n", "off":
		return false
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		// Preserve the old string-field behavior for legacy non-empty values:
		// GA treats any non-empty fake_cc_system_prompt value as enabled.
		return true
	}
}

type Profile struct {
	VarName            string                 `json:"var_name"`
	Type               string                 `json:"type"`
	Name               string                 `json:"name"`
	APIBase            string                 `json:"apibase"`
	Model              string                 `json:"model"`
	Models             []string               `json:"models,omitempty"`
	APIKey             string                 `json:"apikey"`
	Stream             *bool                  `json:"stream,omitempty"`
	MaxRetries         *int                   `json:"max_retries,omitempty"`
	ReadTimeout        *int                   `json:"read_timeout,omitempty"`
	ConnectTimeout     *int                   `json:"connect_timeout,omitempty"`
	UserAgent          string                 `json:"user_agent,omitempty"`
	APIMode            string                 `json:"api_mode,omitempty"`
	ThinkingType       string                 `json:"thinking_type,omitempty"`
	ReasoningEffort    string                 `json:"reasoning_effort,omitempty"`
	FakeCCSystemPrompt *OptionalBool          `json:"fake_cc_system_prompt,omitempty"`
	Extra              map[string]interface{} `json:"extra,omitempty"`
}

type Draft struct {
	UpdatedAt string    `json:"updated_at,omitempty"`
	Profiles  []Profile `json:"profiles"`
}
type Store struct{ Root string }

var nameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func NewStore(root string) *Store { return &Store{Root: root} }
func (s *Store) path() string     { return filepath.Join(s.Root, "model_profiles.json") }

func unsafeGARoot(p string) bool {
	clean := filepath.Clean(strings.TrimSpace(p))
	if clean == "" || clean == "." {
		return true
	}
	vol := filepath.VolumeName(clean)
	rest := strings.TrimPrefix(clean, vol)
	rest = filepath.Clean(rest)
	return rest == "" || rest == "." || rest == string(filepath.Separator)
}

func validateExportRoot(gaRoot string) error {
	if unsafeGARoot(gaRoot) {
		return fmt.Errorf("ga_root must not be empty or a filesystem root")
	}
	return nil
}

func Defaults() []Profile {
	b := true
	mr := 3
	rt := 300
	return []Profile{{VarName: "native_oai_config1", Type: "native_oai", Name: "main", APIBase: "https://api.openai.com/v1", Model: "gpt-4.1", Models: []string{"gpt-4.1"}, Stream: &b, MaxRetries: &mr, ReadTimeout: &rt, Extra: map[string]interface{}{}}}
}

func (s *Store) Load(raw bool) (Draft, error) {
	data, err := os.ReadFile(s.path())
	if err != nil {
		d := Draft{Profiles: Defaults()}
		return d, nil
	}
	var d Draft
	if err := json.Unmarshal(data, &d); err != nil {
		return d, err
	}
	if len(d.Profiles) == 0 {
		d.Profiles = Defaults()
	}
	d.Profiles = normalizeProfiles(d.Profiles)
	if !raw {
		for i := range d.Profiles {
			if d.Profiles[i].APIKey != "" {
				d.Profiles[i].APIKey = "******"
			}
		}
	}
	return d, nil
}

func (s *Store) Save(profiles []Profile) (Draft, error) {
	merged, err := s.MergePreservedSecrets(profiles)
	if err != nil {
		return Draft{}, err
	}
	merged = normalizeProfiles(merged)
	if err := Validate(merged); err != nil {
		return Draft{}, err
	}
	d := Draft{UpdatedAt: time.Now().Format(time.RFC3339), Profiles: merged}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return d, err
	}
	return d, writeFileAtomic(s.path(), data, 0600)
}

func (s *Store) MergePreservedSecrets(profiles []Profile) ([]Profile, error) {
	old, err := s.Load(true)
	if err != nil {
		return nil, err
	}
	byVar := map[string]string{}
	for _, p := range old.Profiles {
		if p.VarName != "" && p.APIKey != "" && !IsMaskedSecret(p.APIKey) {
			byVar[p.VarName] = p.APIKey
		}
	}
	merged := make([]Profile, len(profiles))
	copy(merged, profiles)
	for i := range merged {
		if merged[i].APIKey == "" || IsMaskedSecret(merged[i].APIKey) {
			if oldKey := byVar[merged[i].VarName]; oldKey != "" {
				merged[i].APIKey = oldKey
			}
		}
	}
	return merged, nil
}


func normalizeProfiles(profiles []Profile) []Profile {
	out := make([]Profile, len(profiles))
	for i, p := range profiles {
		out[i] = normalizeProfile(p)
	}
	return out
}

func normalizeProfile(p Profile) Profile {
	models := profileModels(p)
	p.Models = models
	if len(models) > 0 {
		p.Model = models[0]
	} else {
		p.Model = strings.TrimSpace(p.Model)
	}
	return p
}

func profileModels(p Profile) []string {
	seen := map[string]bool{}
	models := []string{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		models = append(models, v)
	}
	add(p.Model)
	for _, m := range p.Models {
		add(m)
	}
	return models
}

func expandedVarName(base string, index int) string {
	if index == 0 {
		return base
	}
	return fmt.Sprintf("%s_%d", base, index+1)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}
func Validate(profiles []Profile) error {
	return validateProfiles(profiles, false)
}

func validateProfiles(profiles []Profile, allowMaskedSecrets bool) error {
	seen := map[string]bool{}
	for _, raw := range profiles {
		p := normalizeProfile(raw)
		if p.VarName == "" || !nameRe.MatchString(p.VarName) {
			return fmt.Errorf("invalid var_name: %s", p.VarName)
		}
		if !strings.Contains(strings.ToLower(p.VarName), "api") && !strings.Contains(strings.ToLower(p.VarName), "config") && !strings.Contains(strings.ToLower(p.VarName), "cookie") {
			return fmt.Errorf("var_name must contain api/config/cookie: %s", p.VarName)
		}
		models := profileModels(p)
		if len(models) == 0 {
			return errors.New("name, apibase and model are required")
		}
		for i := range models {
			varName := expandedVarName(p.VarName, i)
			if seen[varName] {
				return fmt.Errorf("duplicate var_name: %s", varName)
			}
			seen[varName] = true
		}
		if p.Name == "" || p.APIBase == "" {
			return errors.New("name, apibase and model are required")
		}
		if !allowMaskedSecrets && IsMaskedSecret(p.APIKey) {
			return fmt.Errorf("masked apikey cannot be saved or exported for %s; reveal/import with authorization or enter the full key", p.VarName)
		}
	}
	return nil
}

func IsMaskedSecret(s string) bool {
	if s == "******" {
		return true
	}
	return strings.Contains(s, "****")
}

func SourceStatus(gaRoot string) map[string]interface{} {
	mykey := filepath.Join(gaRoot, "mykey.py")
	jsonp := filepath.Join(gaRoot, "mykey.json")
	gen := filepath.Join(gaRoot, "mykey_admin.generated.py")
	return map[string]interface{}{
		"mykey_py_exists": exists(mykey), "mykey_json_exists": exists(jsonp), "generated_exists": exists(gen), "generated_path": gen,
		"safe_note": "mykey.py can be imported with explicit user authorization. Import uses Python AST parsing only and never executes mykey.py.",
	}
}
func exists(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }

func ImportMyKey(gaRoot string, reveal bool) (Draft, error) {
	return ImportMyKeyWithPython(gaRoot, "", reveal)
}

func ImportMyKeyWithPython(gaRoot, configuredPython string, reveal bool) (Draft, error) {
	mykey := filepath.Join(gaRoot, "mykey.py")
	if !exists(mykey) {
		return Draft{UpdatedAt: time.Now().Format(time.RFC3339), Profiles: Defaults()}, nil
	}
	py := pythonExe(gaRoot, configuredPython)
	script := `import ast, json, sys
path=sys.argv[1]
reveal=sys.argv[2]=='1'
text=open(path,'r',encoding='utf-8').read()
tree=ast.parse(text, filename=path)

def val(n):
    if isinstance(n, ast.Constant): return n.value
    if isinstance(n, ast.Dict): return {val(k): val(v) for k,v in zip(n.keys,n.values) if k is not None}
    if isinstance(n, (ast.List, ast.Tuple)): return [val(x) for x in n.elts]
    if isinstance(n, ast.UnaryOp) and isinstance(n.op, ast.USub) and isinstance(n.operand, ast.Constant) and isinstance(n.operand.value,(int,float)): return -n.operand.value
    return None

def mask(s):
    if not isinstance(s,str) or not s: return s
    if reveal: return s
    if len(s)<=8: return '******'
    return s[:3]+'****'+s[-4:]
profiles=[]
for node in tree.body:
    if not isinstance(node, ast.Assign): continue
    names=[t.id for t in node.targets if isinstance(t, ast.Name)]
    if not names: continue
    var=names[0]
    low=var.lower()
    if 'mixin' in low: continue
    official = any(x in low for x in ('native_claude','native_oai','claude','oai'))
    legacy = any(x in low for x in ('api','config','cookie'))
    if not (official or legacy): continue
    d=val(node.value)
    if not isinstance(d, dict): continue
    def pop_any(keys, default=''):
        for k in keys:
            if k in d: return d.pop(k)
        return default
    apikey=pop_any(['apikey','api_key','key','token','cookie'], '')
    if 'native' in low and 'claude' in low:
        typ='native_claude'
    elif 'native' in low and 'oai' in low:
        typ='native_oai'
    elif 'claude' in low:
        typ='claude'
    elif 'oai' in low:
        typ='oai'
    else:
        typ='native_oai'
    p={'var_name':var,'type':typ,'name':str(pop_any(['name'], var) or var),'apibase':str(pop_any(['apibase','api_base','base_url','baseURL'], '') or ''),'model':str(pop_any(['model','model_name'], '') or ''),'apikey':mask(str(apikey) if apikey is not None else '')}
    for src,dst in [('stream','stream'),('max_retries','max_retries'),('read_timeout','read_timeout'),('connect_timeout','connect_timeout'),('user_agent','user_agent'),('api_mode','api_mode'),('thinking_type','thinking_type'),('reasoning_effort','reasoning_effort'),('fake_cc_system_prompt','fake_cc_system_prompt')]:
        if src in d: p[dst]=d.pop(src)
    p['extra']=d
    profiles.append(p)
# Aggregate same source (protocol + API URL + API key) into one admin card while
# preserving ordinary mykey.py output on export.
groups=[]
index={}
for p in profiles:
    key=(p.get('type',''), p.get('apibase',''), p.get('apikey',''))
    model=p.get('model','')
    if key not in index:
        p['models']=[]
        index[key]=len(groups)
        groups.append(p)
    g=groups[index[key]]
    if model and model not in g['models']:
        g['models'].append(model)
    if not g.get('model') and model:
        g['model']=model
for g in groups:
    if not g.get('model') and g.get('models'):
        g['model']=g['models'][0]
print(json.dumps({'updated_at':'','profiles':groups}, ensure_ascii=False))`
	cmd := exec.Command(py, "-c", script, mykey, boolArg(reveal))
	hideChildWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return Draft{}, fmt.Errorf("parse mykey.py failed via %s: %v: %s", py, err, strings.TrimSpace(stderr.String()))
	}
	var d Draft
	if err := json.Unmarshal(out, &d); err != nil {
		return Draft{}, err
	}
	if len(d.Profiles) == 0 {
		return Draft{}, errors.New("no supported official model config dict assignments found in mykey.py")
	}
	return d, nil
}

func pythonExe(gaRoot, configuredPython string) string {
	if py := strings.TrimSpace(configuredPython); py != "" {
		return py
	}
	candidates := []string{
		filepath.Join(gaRoot, ".venv", "Scripts", "python.exe"),
		filepath.Join(gaRoot, "venv", "Scripts", "python.exe"),
		filepath.Join(gaRoot, ".venv", "bin", "python"),
		filepath.Join(gaRoot, "venv", "bin", "python"),
	}
	for _, c := range candidates {
		if exists(c) {
			return c
		}
	}
	if runtime.GOOS == "windows" {
		return "python"
	}
	return "python3"
}
func boolArg(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func Render(profiles []Profile) (string, error) {
	return render(profiles, false)
}

func RenderPreview(profiles []Profile) (string, error) {
	return render(profiles, true)
}

func render(profiles []Profile, allowMaskedSecrets bool) (string, error) {
	if err := validateProfiles(profiles, allowMaskedSecrets); err != nil {
		return "", err
	}
	profiles = normalizeProfiles(profiles)
	var b strings.Builder
	b.WriteString("# Auto-generated by GenericAgent-Admin-Go.\n# Review before copying to mykey.py. Keep this file private.\n# GenericAgent discovers official config dicts by variable name: native_claude/native_oai/claude/oai or api/config/cookie.\n\n")
	for _, p := range profiles {
		models := profileModels(p)
		for i, model := range models {
			m := map[string]interface{}{}
			m["name"] = p.Name
			if len(models) > 1 {
				m["name"] = fmt.Sprintf("%s · %s", p.Name, model)
			}
			m["apikey"] = p.APIKey
			m["apibase"] = p.APIBase
			m["model"] = model
			if p.Stream != nil {
				m["stream"] = *p.Stream
			}
			if p.MaxRetries != nil {
				m["max_retries"] = *p.MaxRetries
			}
			if p.ReadTimeout != nil {
				m["read_timeout"] = *p.ReadTimeout
			}
			if p.ConnectTimeout != nil {
				m["connect_timeout"] = *p.ConnectTimeout
			}
			if p.UserAgent != "" {
				m["user_agent"] = p.UserAgent
			}
			if p.APIMode != "" {
				m["api_mode"] = p.APIMode
			}
			if p.ThinkingType != "" {
				m["thinking_type"] = p.ThinkingType
			}
			if p.ReasoningEffort != "" {
				m["reasoning_effort"] = p.ReasoningEffort
			}
			if p.FakeCCSystemPrompt != nil {
				m["fake_cc_system_prompt"] = bool(*p.FakeCCSystemPrompt)
			}
			for k, v := range p.Extra {
				if _, ok := m[k]; !ok {
					m[k] = v
				}
			}
			dict, err := pyDict(m)
			if err != nil {
				return "", err
			}
			b.WriteString(fmt.Sprintf("%s = %s\n\n", expandedVarName(p.VarName, i), dict))
		}
	}
	return b.String(), nil
}

func pyDict(m map[string]interface{}) (string, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, k := range keys {
		val, err := pyVal(m[k])
		if err != nil {
			return "", fmt.Errorf("render %q: %w", k, err)
		}
		parts = append(parts, fmt.Sprintf("%q: %s", k, val))
	}
	return "{" + strings.Join(parts, ", ") + "}", nil
}
func pyVal(v interface{}) (string, error) {
	switch x := v.(type) {
	case string:
		return fmt.Sprintf("%q", x), nil
	case bool:
		if x {
			return "True", nil
		}
		return "False", nil
	case float64:
		return fmt.Sprintf("%v", x), nil
	case int:
		return fmt.Sprintf("%d", x), nil
	case nil:
		return "None", nil
	default:
		data, err := json.Marshal(x)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}

func Export(gaRoot string, profiles []Profile, overwriteActive bool) (map[string]interface{}, error) {
	if err := validateExportRoot(gaRoot); err != nil {
		return nil, err
	}
	text, err := Render(profiles)
	if err != nil {
		return nil, err
	}
	gen := filepath.Join(gaRoot, "mykey_admin.generated.py")
	if err := writeFileAtomic(gen, []byte(text), 0600); err != nil {
		return nil, err
	}
	active := filepath.Join(gaRoot, "mykey.py")
	res := map[string]interface{}{"generated_path": gen, "activated": false, "active_path": nil, "backup_path": nil}
	if overwriteActive {
		if exists(active) {
			bak := filepath.Join(gaRoot, fmt.Sprintf("mykey.py.bak-%s", time.Now().Format("20060102-150405")))
			data, err := os.ReadFile(active)
			if err != nil {
				return nil, err
			}
			if err := writeFileAtomic(bak, data, 0600); err != nil {
				return nil, err
			}
			res["backup_path"] = bak
		}
		if err := writeFileAtomic(active, []byte(text), 0600); err != nil {
			return nil, err
		}
		res["activated"] = true
		res["active_path"] = active
	}
	return res, nil
}
