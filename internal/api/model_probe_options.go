package api

import (
	"strings"

	"genericagent-admin-go/internal/modelconfig"
)

type modelProbeOptions struct {
	APIMode   string `json:"api_mode,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

func (s *Server) resolveModelProbeOptions(varName string, requested map[string]modelProbeOptions) map[string]modelProbeOptions {
	result := map[string]modelProbeOptions{}
	if s != nil && s.CfgStore != nil && strings.TrimSpace(varName) != "" {
		if draft, err := s.loadModelsFromOfficialMyKey(false); err == nil {
			for _, profile := range draft.Profiles {
				if profile.VarName != varName {
					continue
				}
				for _, config := range probeProfileModelConfigs(profile) {
					result[config.Model] = modelProbeOptions{
						APIMode:   firstProbeValue(config.APIMode, profile.APIMode),
						UserAgent: firstProbeValue(config.UserAgent, profile.UserAgent),
					}
				}
			}
		}
	}
	for model, options := range requested {
		current := result[model]
		if strings.TrimSpace(options.APIMode) != "" {
			current.APIMode = options.APIMode
		}
		if strings.TrimSpace(options.UserAgent) != "" {
			current.UserAgent = options.UserAgent
		}
		result[model] = current
	}
	return result
}

func probeProfileModelConfigs(profile modelconfig.Profile) []modelconfig.ModelConfig {
	if len(profile.ModelConfigs) > 0 {
		return profile.ModelConfigs
	}
	result := make([]modelconfig.ModelConfig, 0, len(profile.Models)+1)
	for _, model := range profile.Models {
		result = append(result, modelconfig.ModelConfig{Model: model})
	}
	if len(result) == 0 && strings.TrimSpace(profile.Model) != "" {
		result = append(result, modelconfig.ModelConfig{Model: profile.Model})
	}
	return result
}

func modelProbeRequestHeaders(base map[string]string, options modelProbeOptions, isClaude bool) map[string]string {
	headers := make(map[string]string, len(base)+3)
	for name, value := range base {
		headers[name] = value
	}
	userAgent := strings.TrimSpace(options.UserAgent)
	if userAgent == "" {
		userAgent = "claude-cli/2.1.152 (external, cli)"
		if isClaude {
			userAgent = "claude-cli/2.1.152 (native, cli)"
		}
	}
	headers["User-Agent"] = userAgent
	headers["x-app"] = "cli"
	if !isClaude {
		headers["originator"] = "codex_exec"
	}
	return headers
}

func normalizeModelProbeAPIMode(value string) string {
	value = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"))
	if value == "response" || value == "responses" {
		return "responses"
	}
	return "chat_completions"
}

func firstProbeValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
