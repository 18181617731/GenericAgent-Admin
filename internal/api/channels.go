package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type channelField struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Secret      bool   `json:"secret,omitempty"`
	Type        string `json:"type,omitempty"`
	Value       string `json:"value,omitempty"`
	HasValue    bool   `json:"has_value,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

type channelProfile struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Fields      []channelField `json:"fields"`
}

type channelsResponse struct {
	Path     string           `json:"path"`
	Exists   bool             `json:"exists"`
	Profiles []channelProfile `json:"profiles"`
}

var channelDefinitions = []channelProfile{
	{ID: "feishu", Name: "飞书 / Lark", Description: "机器人应用凭据、用户白名单与公开访问开关。", Fields: []channelField{
		{Name: "fs_app_id", Label: "App ID", Placeholder: "cli_xxx"},
		{Name: "fs_app_secret", Label: "App Secret", Secret: true, Placeholder: "留空则保留 mykey.py 已有值"},
		{Name: "fs_allowed_users", Label: "Allowed Users", Placeholder: "user1,user2 或留空", Type: "list"},
		{Name: "fs_public_access", Label: "Public Access", Type: "bool"},
	}},
	{ID: "wecom", Name: "企业微信", Description: "企业微信机器人/应用通道凭据与允许用户。", Fields: []channelField{
		{Name: "wecom_bot_id", Label: "Bot ID / Agent ID", Placeholder: "ww/openapi id"},
		{Name: "wecom_secret", Label: "Secret", Secret: true, Placeholder: "留空则保留 mykey.py 已有值"},
		{Name: "wecom_allowed_users", Label: "Allowed Users", Placeholder: "user1,user2 或留空", Type: "list"},
	}},
	{ID: "dingtalk", Name: "钉钉", Description: "钉钉机器人/应用通道 client 凭据与允许用户。", Fields: []channelField{
		{Name: "dingtalk_client_id", Label: "Client ID", Placeholder: "dingxxx"},
		{Name: "dingtalk_client_secret", Label: "Client Secret", Secret: true, Placeholder: "留空则保留 mykey.py 已有值"},
		{Name: "dingtalk_allowed_users", Label: "Allowed Users", Placeholder: "user1,user2 或留空", Type: "list"},
	}},
}

func (s *Server) channels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.loadChannelsResponse())
	case http.MethodPut:
		if r.Header.Get("X-GA-Confirm") != "dangerous" {
			bad(w, http.StatusPreconditionRequired, "dangerous operation requires X-GA-Confirm: dangerous")
			return
		}
		var req struct {
			Profiles []channelProfile `json:"profiles"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			bad(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.saveChannels(req.Profiles); err != nil {
			bad(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, s.loadChannelsResponse())
	default:
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) channelConfigPath() string {
	return filepath.Join(s.CfgStore.Cfg.GARoot, "mykey.py")
}

func (s *Server) loadChannelsResponse() channelsResponse {
	values := map[string]string{}
	exists := false
	if strings.TrimSpace(s.CfgStore.Cfg.GARoot) != "" {
		if content, err := os.ReadFile(s.channelConfigPath()); err == nil {
			exists = true
			values = parseChannelAssignments(string(content))
		}
	}
	profiles := cloneChannelDefinitions()
	for pi := range profiles {
		for fi := range profiles[pi].Fields {
			f := &profiles[pi].Fields[fi]
			if v, ok := values[f.Name]; ok {
				f.HasValue = strings.TrimSpace(v) != "" && v != "[]" && strings.ToLower(v) != "false"
				if !f.Secret {
					f.Value = normalizeChannelDisplayValue(v, f.Type)
				}
			}
		}
	}
	return channelsResponse{Path: s.channelConfigPath(), Exists: exists, Profiles: profiles}
}

func (s *Server) saveChannels(profiles []channelProfile) error {
	if strings.TrimSpace(s.CfgStore.Cfg.GARoot) == "" {
		return fmt.Errorf("GA root is not configured")
	}
	path := s.channelConfigPath()
	content := ""
	existing := map[string]string{}
	if b, err := os.ReadFile(path); err == nil {
		content = string(b)
		existing = parseChannelAssignments(content)
	} else if !os.IsNotExist(err) {
		return err
	}
	incoming := map[string]channelField{}
	for _, p := range profiles {
		for _, f := range p.Fields {
			incoming[f.Name] = f
		}
	}
	values := map[string]string{}
	for _, p := range channelDefinitions {
		for _, def := range p.Fields {
			f, ok := incoming[def.Name]
			if !ok {
				f = def
			}
			if def.Secret && f.Value == "" {
				values[def.Name] = existing[def.Name]
			} else {
				values[def.Name] = encodeChannelValue(f.Value, def.Type)
			}
		}
	}
	updated := upsertChannelAssignments(content, values)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(updated), 0600)
}

func cloneChannelDefinitions() []channelProfile {
	out := make([]channelProfile, len(channelDefinitions))
	for i, p := range channelDefinitions {
		out[i] = p
		out[i].Fields = append([]channelField{}, p.Fields...)
	}
	return out
}

var assignRe = regexp.MustCompile(`(?m)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.+?)\s*$`)

func parseChannelAssignments(content string) map[string]string {
	out := map[string]string{}
	allowed := map[string]string{}
	for _, p := range channelDefinitions {
		for _, f := range p.Fields {
			allowed[f.Name] = f.Type
		}
	}
	for _, m := range assignRe.FindAllStringSubmatch(content, -1) {
		typ, ok := allowed[m[1]]
		if !ok {
			continue
		}
		out[m[1]] = normalizeChannelDisplayValue(m[2], typ)
	}
	return out
}

func upsertChannelAssignments(content string, values map[string]string) string {
	allowed := map[string]bool{}
	formatted := map[string]string{}
	for _, p := range channelDefinitions {
		for _, f := range p.Fields {
			allowed[f.Name] = true
			formatted[f.Name] = fmt.Sprintf("%s = %s", f.Name, formatPythonLiteral(values[f.Name], f.Type))
		}
	}
	seen := map[string]bool{}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if strings.TrimSpace(content) == "" {
		lines = []string{}
	}
	for i, line := range lines {
		m := assignRe.FindStringSubmatch(line)
		if len(m) == 3 && allowed[m[1]] {
			lines[i] = formatted[m[1]]
			seen[m[1]] = true
		}
	}
	if strings.TrimSpace(content) != "" && (len(lines) == 0 || strings.TrimSpace(lines[len(lines)-1]) != "") {
		lines = append(lines, "")
	}
	missing := []string{}
	for _, p := range channelDefinitions {
		for _, f := range p.Fields {
			if !seen[f.Name] {
				missing = append(missing, formatted[f.Name])
			}
		}
	}
	if len(missing) > 0 {
		lines = append(lines, "# GA Admin channel configuration", "# Managed by GA Admin; secret values are masked in the UI.")
		lines = append(lines, missing...)
	}
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

func normalizeChannelDisplayValue(raw, typ string) string {
	raw = strings.TrimSpace(raw)
	switch typ {
	case "bool":
		return strings.ToLower(raw)
	case "list":
		if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
			var arr []string
			if err := json.Unmarshal([]byte(strings.ReplaceAll(raw, "'", "\"")), &arr); err == nil {
				return strings.Join(arr, ",")
			}
		}
		return strings.Trim(raw, "'\"")
	default:
		if v, err := strconv.Unquote(raw); err == nil {
			return v
		}
		return strings.Trim(raw, "'\"")
	}
}

func encodeChannelValue(v, typ string) string {
	v = strings.TrimSpace(v)
	if typ == "bool" {
		if strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes") || strings.EqualFold(v, "on") {
			return "true"
		}
		return "false"
	}
	return v
}

func formatPythonLiteral(v, typ string) string {
	switch typ {
	case "bool":
		if strings.EqualFold(v, "true") {
			return "True"
		}
		return "False"
	case "list":
		parts := []string{}
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				parts = append(parts, part)
			}
		}
		sort.Strings(parts)
		b, _ := json.Marshal(parts)
		return string(b)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
