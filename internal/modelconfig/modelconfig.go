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
	"sort"
	"strings"
	"time"
)

type Profile struct {
	VarName            string                 `json:"var_name"`
	Type               string                 `json:"type"`
	Name               string                 `json:"name"`
	APIBase            string                 `json:"apibase"`
	Model              string                 `json:"model"`
	APIKey             string                 `json:"apikey"`
	Stream             *bool                  `json:"stream,omitempty"`
	MaxRetries         *int                   `json:"max_retries,omitempty"`
	ReadTimeout        *int                   `json:"read_timeout,omitempty"`
	ConnectTimeout     *int                   `json:"connect_timeout,omitempty"`
	UserAgent          string                 `json:"user_agent,omitempty"`
	APIMode            string                 `json:"api_mode,omitempty"`
	ThinkingType       string                 `json:"thinking_type,omitempty"`
	ReasoningEffort    string                 `json:"reasoning_effort,omitempty"`
	FakeCCSystemPrompt string                 `json:"fake_cc_system_prompt,omitempty"`
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

func Defaults() []Profile {
	b := true
	mr := 3
	rt := 300
	return []Profile{{VarName: "api_config_main", Type: "openai", Name: "main", APIBase: "https://api.openai.com/v1", Model: "gpt-4.1", Stream: &b, MaxRetries: &mr, ReadTimeout: &rt, Extra: map[string]interface{}{}}}
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
	if err := Validate(profiles); err != nil {
		return Draft{}, err
	}
	d := Draft{UpdatedAt: time.Now().Format(time.RFC3339), Profiles: profiles}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return d, err
	}
	return d, os.WriteFile(s.path(), data, 0600)
}

func Validate(profiles []Profile) error {
	seen := map[string]bool{}
	for _, p := range profiles {
		if p.VarName == "" || !nameRe.MatchString(p.VarName) {
			return fmt.Errorf("invalid var_name: %s", p.VarName)
		}
		if !strings.Contains(strings.ToLower(p.VarName), "api") && !strings.Contains(strings.ToLower(p.VarName), "config") && !strings.Contains(strings.ToLower(p.VarName), "cookie") {
			return fmt.Errorf("var_name must contain api/config/cookie: %s", p.VarName)
		}
		if seen[p.VarName] {
			return fmt.Errorf("duplicate var_name: %s", p.VarName)
		}
		seen[p.VarName] = true
		if p.Name == "" || p.APIBase == "" || p.Model == "" {
			return errors.New("name, apibase and model are required")
		}
	}
	return nil
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
	mykey := filepath.Join(gaRoot, "mykey.py")
	if !exists(mykey) {
		return Draft{}, fmt.Errorf("mykey.py not found: %s", mykey)
	}
	py := pythonExe(gaRoot)
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
    if not any(x in low for x in ('api','config','cookie')): continue
    d=val(node.value)
    if not isinstance(d, dict): continue
    def pop_any(keys, default=''):
        for k in keys:
            if k in d: return d.pop(k)
        return default
    apikey=pop_any(['apikey','api_key','key','token','cookie'], '')
    typ='native_oai'
    if 'claude' in low or 'anthropic' in str(d).lower() or 'fake_cc_system_prompt' in d: typ='native_claude'
    if 'gemini' in low or 'generativelanguage' in str(d).lower(): typ='gemini'
    p={'var_name':var,'type':typ,'name':str(pop_any(['name'], var) or var),'apibase':str(pop_any(['apibase','api_base','base_url','baseURL'], '') or ''),'model':str(pop_any(['model','model_name'], '') or ''),'apikey':mask(str(apikey) if apikey is not None else '')}
    for src,dst in [('stream','stream'),('max_retries','max_retries'),('read_timeout','read_timeout'),('connect_timeout','connect_timeout'),('user_agent','user_agent'),('api_mode','api_mode'),('thinking_type','thinking_type'),('reasoning_effort','reasoning_effort'),('fake_cc_system_prompt','fake_cc_system_prompt')]:
        if src in d: p[dst]=d.pop(src)
    p['extra']=d
    profiles.append(p)
print(json.dumps({'updated_at':'','profiles':profiles}, ensure_ascii=False))`
	cmd := exec.Command(py, "-c", script, mykey, boolArg(reveal))
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
		return Draft{}, errors.New("no supported api/config/cookie dict assignments found in mykey.py")
	}
	return d, nil
}

func pythonExe(gaRoot string) string {
	candidates := []string{filepath.Join(gaRoot, ".venv", "Scripts", "python.exe"), filepath.Join(gaRoot, "venv", "Scripts", "python.exe"), "python"}
	for _, c := range candidates {
		if c == "python" || exists(c) {
			return c
		}
	}
	return "python"
}
func boolArg(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func Render(profiles []Profile) (string, error) {
	if err := Validate(profiles); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# Auto-generated by GenericAgent-Admin-Go.\n# Review before copying to mykey.py. Keep this file private.\n# GA discovers variables whose names contain api/config/cookie.\n\n")
	for _, p := range profiles {
		m := map[string]interface{}{}
		m["name"] = p.Name
		m["apikey"] = p.APIKey
		m["apibase"] = p.APIBase
		m["model"] = p.Model
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
		if p.FakeCCSystemPrompt != "" {
			m["fake_cc_system_prompt"] = p.FakeCCSystemPrompt
		}
		for k, v := range p.Extra {
			if _, ok := m[k]; !ok {
				m[k] = v
			}
		}
		b.WriteString(fmt.Sprintf("%s = %s\n\n", p.VarName, pyDict(m)))
	}
	return b.String(), nil
}

func pyDict(m map[string]interface{}) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%q: %s", k, pyVal(m[k])))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
func pyVal(v interface{}) string {
	switch x := v.(type) {
	case string:
		return fmt.Sprintf("%q", x)
	case bool:
		if x {
			return "True"
		}
		return "False"
	case float64:
		return fmt.Sprintf("%v", x)
	case int:
		return fmt.Sprintf("%d", x)
	case nil:
		return "None"
	default:
		data, _ := json.Marshal(x)
		return string(data)
	}
}

func Export(gaRoot string, profiles []Profile, overwriteActive bool) (map[string]interface{}, error) {
	text, err := Render(profiles)
	if err != nil {
		return nil, err
	}
	gen := filepath.Join(gaRoot, "mykey_admin.generated.py")
	if err := os.WriteFile(gen, []byte(text), 0600); err != nil {
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
			if err := os.WriteFile(bak, data, 0600); err != nil {
				return nil, err
			}
			res["backup_path"] = bak
		}
		if err := os.WriteFile(active, []byte(text), 0600); err != nil {
			return nil, err
		}
		res["activated"] = true
		res["active_path"] = active
	}
	return res, nil
}
