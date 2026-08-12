package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
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
	Testable    bool           `json:"testable"`
	Fields      []channelField `json:"fields"`
}

type channelsResponse struct {
	Path     string           `json:"path"`
	Exists   bool             `json:"exists"`
	Profiles []channelProfile `json:"profiles"`
}

type channelTestRequest struct {
	ProfileID string           `json:"profile_id"`
	Fields    []channelField   `json:"fields"`
	Profiles  []channelProfile `json:"profiles,omitempty"`
}

type channelTestResponse struct {
	ProfileID string `json:"profile_id"`
	OK        bool   `json:"ok"`
	Message   string `json:"message"`
}

type channelTestEndpointSet struct {
	Feishu   string
	WeCom    string
	DingTalk string
	Discord  string
	QQ       string
	Telegram string
}

var channelTestEndpoints = channelTestEndpointSet{
	Feishu:   "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
	WeCom:    "https://qyapi.weixin.qq.com/cgi-bin/gettoken",
	DingTalk: "https://api.dingtalk.com/v1.0/oauth2/accessToken",
	Discord:  "https://discord.com/api/v10/users/@me",
	QQ:       "https://bots.qq.com/app/getAppAccessToken",
	Telegram: "https://api.telegram.org",
}

var channelTestHTTPClient = &http.Client{Timeout: 12 * time.Second}

const maxChannelTestResponseBytes = 1 << 20

var channelDefinitions = []channelProfile{
	{ID: "feishu", Name: "飞书 / Lark", Description: "机器人应用凭据、用户白名单与兼容公开访问开关。", Testable: true, Fields: []channelField{
		{Name: "fs_app_id", Label: "App ID", Placeholder: "cli_xxx"},
		{Name: "fs_app_secret", Label: "App Secret", Secret: true, Placeholder: "留空则保留 mykey.py 中现有值"},
		{Name: "fs_allowed_users", Label: "允许用户", Type: "list", Placeholder: "ou_xxx,ou_yyy；留空或 * 为公开访问"},
		{Name: "fs_public_access", Label: "公开访问（兼容）", Type: "bool", Value: "false"},
	}},
	{ID: "wecom", Name: "企业微信", Description: "企业微信机器人/应用通道凭据、欢迎语与允许用户。", Testable: true, Fields: []channelField{
		{Name: "wecom_bot_id", Label: "Bot ID / Agent ID", Placeholder: "1000002"},
		{Name: "wecom_secret", Label: "Secret", Secret: true, Placeholder: "留空则保留 mykey.py 中现有值"},
		{Name: "wecom_welcome_message", Label: "欢迎语", Placeholder: "可选"},
		{Name: "wecom_allowed_users", Label: "允许用户", Type: "list", Placeholder: "userid1,userid2"},
	}},
	{ID: "dingtalk", Name: "钉钉", Description: "钉钉机器人/应用通道 client 凭据与允许用户。", Testable: true, Fields: []channelField{
		{Name: "dingtalk_client_id", Label: "Client ID"},
		{Name: "dingtalk_client_secret", Label: "Client Secret", Secret: true, Placeholder: "留空则保留 mykey.py 中现有值"},
		{Name: "dingtalk_allowed_users", Label: "允许用户", Type: "list", Placeholder: "userid1,userid2"},
	}},
	{ID: "discord", Name: "Discord", Description: "Discord Bot Token 与允许用户；可复用全局 proxy 配置。", Testable: true, Fields: []channelField{
		{Name: "discord_bot_token", Label: "Bot Token", Secret: true, Placeholder: "留空则保留 mykey.py 中现有值"},
		{Name: "discord_allowed_users", Label: "允许用户", Type: "list", Placeholder: "user_id1,user_id2"},
	}},
	{ID: "qq", Name: "QQ", Description: "QQ 开放平台机器人 App ID、App Secret 与允许用户。", Testable: true, Fields: []channelField{
		{Name: "qq_app_id", Label: "App ID"},
		{Name: "qq_app_secret", Label: "App Secret", Secret: true, Placeholder: "留空则保留 mykey.py 中现有值"},
		{Name: "qq_allowed_users", Label: "允许用户", Type: "list", Placeholder: "user_id1,user_id2"},
	}},
	{ID: "telegram", Name: "Telegram", Description: "Telegram Bot Token 与允许用户；可复用全局 proxy 配置。", Testable: true, Fields: []channelField{
		{Name: "tg_bot_token", Label: "Bot Token", Secret: true, Placeholder: "留空则保留 mykey.py 中现有值"},
		{Name: "tg_allowed_users", Label: "允许用户", Type: "list", Placeholder: "user_id1,user_id2"},
	}},
	{ID: "wechat", Name: "微信", Description: "无需 mykey 凭据；启动 wechatapp.py 后按日志中的二维码扫码登录。", Testable: false, Fields: []channelField{}},
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
		if err := decode(r, &req); err != nil {
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

func (s *Server) channelTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req channelTestRequest
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	profileID := strings.TrimSpace(req.ProfileID)
	if profileID == "" {
		bad(w, http.StatusBadRequest, "profile_id is required")
		return
	}
	fields := req.Fields
	if len(fields) == 0 {
		for _, p := range req.Profiles {
			if p.ID == profileID {
				fields = p.Fields
				break
			}
		}
	}
	values, err := s.channelTestValues(profileID, fields)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	ok, msg := testChannelCredentials(profileID, values)
	writeJSON(w, channelTestResponse{ProfileID: profileID, OK: ok, Message: msg})
}

func (s *Server) channelTestValues(profileID string, fields []channelField) (map[string]string, error) {
	defs := map[string]channelField{}
	known := false
	for _, p := range channelDefinitions {
		if p.ID != profileID {
			continue
		}
		known = true
		for _, f := range p.Fields {
			defs[f.Name] = f
		}
	}
	if !known {
		return nil, fmt.Errorf("unknown channel profile: %s", profileID)
	}
	existing := map[string]string{}
	if strings.TrimSpace(s.CfgStore.Snapshot().GARoot) != "" {
		if b, err := os.ReadFile(s.channelConfigPath()); err == nil {
			existing = parseChannelAssignments(string(b))
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	incoming := map[string]channelField{}
	for _, f := range fields {
		incoming[f.Name] = f
	}
	values := map[string]string{}
	for name, def := range defs {
		f, ok := incoming[name]
		if !ok {
			f = def
		}
		if def.Secret && strings.TrimSpace(f.Value) == "" {
			values[name] = existing[name]
		} else {
			value, err := encodeChannelValue(f.Value, def.Type)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			values[name] = value
		}
	}
	return values, nil
}

func testChannelCredentials(profileID string, values map[string]string) (bool, string) {
	switch profileID {
	case "feishu":
		return testFeishuCredentials(values["fs_app_id"], values["fs_app_secret"])
	case "wecom":
		return testWeComCredentials(values["wecom_bot_id"], values["wecom_secret"])
	case "dingtalk":
		return testDingTalkCredentials(values["dingtalk_client_id"], values["dingtalk_client_secret"])
	case "discord":
		return testDiscordCredentials(values["discord_bot_token"])
	case "qq":
		return testQQCredentials(values["qq_app_id"], values["qq_app_secret"])
	case "telegram":
		return testTelegramCredentials(values["tg_bot_token"])
	case "wechat":
		return false, "微信使用扫码登录，无需测试凭据"
	default:
		return false, "unknown channel profile"
	}
}

func testFeishuCredentials(appID, appSecret string) (bool, string) {
	appID, appSecret = strings.TrimSpace(appID), strings.TrimSpace(appSecret)
	if appID == "" || appSecret == "" {
		return false, "App ID 和 App Secret 不能为空"
	}
	body, _ := json.Marshal(map[string]string{"app_id": appID, "app_secret": appSecret})
	status, payload, err := channelTestRequestJSON(http.MethodPost, channelTestEndpoints.Feishu, body)
	if err != nil {
		return false, redactChannelTestMessage(err.Error(), appSecret)
	}
	if status < 200 || status >= 300 {
		return false, fmt.Sprintf("飞书接口返回 HTTP %d", status)
	}
	code := intFromJSON(payload, "code", "StatusCode", "errcode")
	if code == 0 {
		return true, "飞书凭据验证通过"
	}
	return false, redactChannelTestMessage(firstStringFromJSON(payload, "msg", "message", "StatusMessage", "errmsg"), appSecret)
}

func testWeComCredentials(botID, secret string) (bool, string) {
	botID, secret = strings.TrimSpace(botID), strings.TrimSpace(secret)
	if botID == "" || secret == "" {
		return false, "Bot ID / Agent ID 和 Secret 不能为空"
	}
	sep := "?"
	if strings.Contains(channelTestEndpoints.WeCom, "?") {
		sep = "&"
	}
	url := fmt.Sprintf("%s%scorpid=%s&corpsecret=%s", channelTestEndpoints.WeCom, sep, urlQueryEscape(botID), urlQueryEscape(secret))
	status, payload, err := channelTestRequestJSON(http.MethodGet, url, nil)
	if err != nil {
		return false, redactChannelTestMessage(err.Error(), secret)
	}
	if status < 200 || status >= 300 {
		return false, fmt.Sprintf("企业微信接口返回 HTTP %d", status)
	}
	if intFromJSON(payload, "errcode", "code") == 0 {
		return true, "企业微信凭据验证通过"
	}
	return false, redactChannelTestMessage(firstStringFromJSON(payload, "errmsg", "msg", "message"), secret)
}

func testDingTalkCredentials(clientID, clientSecret string) (bool, string) {
	clientID, clientSecret = strings.TrimSpace(clientID), strings.TrimSpace(clientSecret)
	if clientID == "" || clientSecret == "" {
		return false, "Client ID 和 Client Secret 不能为空"
	}
	body, _ := json.Marshal(map[string]string{"appKey": clientID, "appSecret": clientSecret})
	status, payload, err := channelTestRequestJSON(http.MethodPost, channelTestEndpoints.DingTalk, body)
	if err != nil {
		return false, redactChannelTestMessage(err.Error(), clientSecret)
	}
	if status < 200 || status >= 300 {
		return false, fmt.Sprintf("钉钉接口返回 HTTP %d", status)
	}
	if intFromJSON(payload, "errcode", "code") == 0 {
		return true, "钉钉凭据验证通过"
	}
	return false, redactChannelTestMessage(firstStringFromJSON(payload, "errmsg", "msg", "message"), clientSecret)
}

func testDiscordCredentials(token string) (bool, string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return false, "Bot Token 不能为空"
	}
	req, err := http.NewRequest(http.MethodGet, channelTestEndpoints.Discord, nil)
	if err != nil {
		return false, redactChannelTestMessage(err.Error(), token)
	}
	req.Header.Set("Authorization", "Bot "+token)
	status, payload, err := executeChannelTestRequest(req)
	if err != nil {
		return false, redactChannelTestMessage(err.Error(), token)
	}
	if status >= 200 && status < 300 && firstStringFromJSON(payload, "id") != "" {
		return true, "Discord 凭据验证通过"
	}
	return false, redactChannelTestMessage(firstStringFromJSON(payload, "message", "error"), token)
}

func testQQCredentials(appID, appSecret string) (bool, string) {
	appID, appSecret = strings.TrimSpace(appID), strings.TrimSpace(appSecret)
	if appID == "" || appSecret == "" {
		return false, "App ID 和 App Secret 不能为空"
	}
	body, _ := json.Marshal(map[string]string{"appId": appID, "clientSecret": appSecret})
	status, payload, err := channelTestRequestJSON(http.MethodPost, channelTestEndpoints.QQ, body)
	if err != nil {
		return false, redactChannelTestMessage(err.Error(), appSecret)
	}
	if status >= 200 && status < 300 && firstStringFromJSON(payload, "access_token") != "" {
		return true, "QQ 凭据验证通过"
	}
	return false, redactChannelTestMessage(firstStringFromJSON(payload, "message", "error", "msg"), appSecret)
}

func testTelegramCredentials(token string) (bool, string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return false, "Bot Token 不能为空"
	}
	endpoint := strings.TrimRight(channelTestEndpoints.Telegram, "/") + "/bot" + url.PathEscape(token) + "/getMe"
	status, payload, err := channelTestRequestJSON(http.MethodGet, endpoint, nil)
	if err != nil {
		return false, redactChannelTestMessage(err.Error(), token)
	}
	if status >= 200 && status < 300 {
		if ok, _ := payload["ok"].(bool); ok {
			return true, "Telegram 凭据验证通过"
		}
	}
	return false, redactChannelTestMessage(firstStringFromJSON(payload, "description", "message", "error"), token)
}

func executeChannelTestRequest(req *http.Request) (int, map[string]interface{}, error) {
	resp, err := channelTestHTTPClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxChannelTestResponseBytes+1))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if len(raw) > maxChannelTestResponseBytes {
		return resp.StatusCode, nil, fmt.Errorf("channel test response body too large")
	}
	payload := map[string]interface{}{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return resp.StatusCode, nil, err
		}
	}
	return resp.StatusCode, payload, nil
}

func redactChannelTestMessage(message string, secrets ...string) string {
	redacted := message
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		redacted = strings.ReplaceAll(redacted, secret, "[REDACTED]")
		redacted = strings.ReplaceAll(redacted, urlQueryEscape(secret), "[REDACTED]")
	}
	return redacted
}

func channelTestRequestJSON(method, endpoint string, body []byte) (int, map[string]interface{}, error) {
	req, err := http.NewRequest(method, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := channelTestHTTPClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxChannelTestResponseBytes+1))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if len(raw) > maxChannelTestResponseBytes {
		return resp.StatusCode, nil, fmt.Errorf("channel test response body too large")
	}
	payload := map[string]interface{}{}
	if len(bytes.TrimSpace(raw)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(raw))
		if err := dec.Decode(&payload); err != nil {
			return resp.StatusCode, nil, fmt.Errorf("invalid channel test JSON response: %w", err)
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			if err == nil {
				return resp.StatusCode, nil, fmt.Errorf("channel test response must contain a single JSON value")
			}
			return resp.StatusCode, nil, fmt.Errorf("invalid channel test JSON response: %w", err)
		}
	}
	return resp.StatusCode, payload, nil
}

func firstStringFromJSON(payload map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := payload[key]; ok {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" {
				return s
			}
		}
	}
	return "凭据验证失败"
}

func intFromJSON(payload map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if v, ok := payload[key]; ok {
			switch n := v.(type) {
			case float64:
				return int(n)
			case int:
				return n
			case string:
				if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
					return i
				}
			}
		}
	}
	return -1
}

func urlQueryEscape(s string) string {
	return url.QueryEscape(s)
}

func (s *Server) channelConfigPath() string {
	return filepath.Join(s.CfgStore.Snapshot().GARoot, "mykey.py")
}

func unsafeChannelGARoot(p string) bool {
	clean := filepath.Clean(strings.TrimSpace(p))
	if clean == "" || clean == "." {
		return true
	}
	vol := filepath.VolumeName(clean)
	rest := strings.TrimPrefix(clean, vol)
	rest = filepath.Clean(rest)
	return rest == "" || rest == "." || rest == string(filepath.Separator)
}

func (s *Server) loadChannelsResponse() channelsResponse {
	values := map[string]string{}
	exists := false
	configPath := ""
	if !unsafeChannelGARoot(s.CfgStore.Snapshot().GARoot) {
		configPath = s.channelConfigPath()
		if content, err := os.ReadFile(configPath); err == nil {
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
	return channelsResponse{Path: configPath, Exists: exists, Profiles: profiles}
}

func (s *Server) saveChannels(profiles []channelProfile) error {
	gaRoot := strings.TrimSpace(s.CfgStore.Snapshot().GARoot)
	if unsafeChannelGARoot(gaRoot) {
		return fmt.Errorf("GA root is not configured or is a filesystem root")
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
				value, err := encodeChannelValue(f.Value, def.Type)
				if err != nil {
					return fmt.Errorf("%s: %w", def.Name, err)
				}
				values[def.Name] = value
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

var pyStrTokenRe = regexp.MustCompile(`'((?:[^'\\]|\\.)*)'|"((?:[^"\\]|\\.)*)"`)

// stripPyComment removes a trailing Python `#` comment that lies outside of any
// string literal. It returns the code portion of the line untouched.
func stripPyComment(s string) string {
	inStr := false
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				inStr = false
			}
			continue
		}
		switch c {
		case '\'', '"':
			inStr = true
			quote = c
		case '#':
			return s[:i]
		}
	}
	return s
}

// bracketDelta returns the net change in open-bracket depth for s, ignoring any
// brackets that appear inside string literals.
func bracketDelta(s string) int {
	depth := 0
	inStr := false
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				inStr = false
			}
			continue
		}
		switch c {
		case '\'', '"':
			inStr = true
			quote = c
		case '[', '{', '(':
			depth++
		case ']', '}', ')':
			depth--
		}
	}
	return depth
}

func pyUnescape(s string) string {
	s = strings.ReplaceAll(s, `\\`, "\x00")
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\'`, `'`)
	s = strings.ReplaceAll(s, "\x00", `\`)
	return s
}

// collectAssignmentValue gathers the (possibly multi-line) right-hand side of an
// assignment whose first value fragment is first. It returns the comment-stripped
// joined value and the index of the last line consumed.
func collectAssignmentValue(lines []string, start int, first string) (string, int) {
	frag := strings.TrimSpace(stripPyComment(first))
	depth := bracketDelta(frag)
	parts := []string{frag}
	end := start
	for depth > 0 && end+1 < len(lines) {
		end++
		seg := strings.TrimSpace(stripPyComment(lines[end]))
		depth += bracketDelta(seg)
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	return strings.Join(parts, " "), end
}

func parseChannelAssignments(content string) map[string]string {
	out := map[string]string{}
	allowed := map[string]string{}
	for _, p := range channelDefinitions {
		for _, f := range p.Fields {
			allowed[f.Name] = f.Type
		}
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		m := assignRe.FindStringSubmatch(lines[i])
		if len(m) != 3 {
			continue
		}
		typ, ok := allowed[m[1]]
		if !ok {
			continue
		}
		value, end := collectAssignmentValue(lines, i, m[2])
		out[m[1]] = normalizeChannelDisplayValue(value, typ)
		i = end
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
	raw := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if strings.TrimSpace(content) == "" {
		raw = []string{}
	}
	lines := []string{}
	for i := 0; i < len(raw); i++ {
		m := assignRe.FindStringSubmatch(raw[i])
		if len(m) == 3 && allowed[m[1]] {
			_, end := collectAssignmentValue(raw, i, m[2])
			lines = append(lines, formatted[m[1]])
			seen[m[1]] = true
			i = end
			continue
		}
		lines = append(lines, raw[i])
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
		inner := raw
		if i := strings.Index(raw, "["); i >= 0 {
			if j := strings.LastIndex(raw, "]"); j > i {
				inner = raw[i+1 : j]
			}
		}
		if matches := pyStrTokenRe.FindAllStringSubmatch(inner, -1); len(matches) > 0 {
			items := make([]string, 0, len(matches))
			for _, mm := range matches {
				tok := mm[1]
				if tok == "" && mm[2] != "" {
					tok = mm[2]
				}
				items = append(items, pyUnescape(tok))
			}
			return strings.Join(items, ",")
		}
		return strings.Trim(strings.TrimSpace(inner), "'\"")
	default:
		if v, err := strconv.Unquote(raw); err == nil {
			return v
		}
		if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
			return pyUnescape(raw[1 : len(raw)-1])
		}
		return strings.Trim(raw, "'\"")
	}
}

func encodeChannelValue(v, typ string) (string, error) {
	v = strings.TrimSpace(v)
	if typ == "bool" {
		switch {
		case strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes") || strings.EqualFold(v, "on"):
			return "true", nil
		case strings.EqualFold(v, "false") || v == "0" || strings.EqualFold(v, "no") || strings.EqualFold(v, "off") || v == "":
			return "false", nil
		default:
			return "", fmt.Errorf("invalid boolean value %q", v)
		}
	}
	return v, nil
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
