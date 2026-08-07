package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"genericagent-admin-go/internal/modelconfig"
)

type usageTotals struct {
	InputTokens  int            `json:"input_tokens"`
	OutputTokens int            `json:"output_tokens"`
	TotalTokens  int            `json:"total_tokens"`
	Other        map[string]int `json:"other,omitempty"`
}

type usageBreakdown struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	UpdatedAt        int64       `json:"updated_at,omitempty"`
	AssistantReplies int         `json:"assistant_replies"`
	Totals           usageTotals `json:"totals"`
}

type usageDaily struct {
	Date             string      `json:"date"`
	AssistantReplies int         `json:"assistant_replies"`
	Totals           usageTotals `json:"totals"`
}

type usageOverviewResponse struct {
	Totals            usageTotals      `json:"totals"`
	SessionCount      int              `json:"session_count"`
	SessionsWithUsage int              `json:"sessions_with_usage"`
	AssistantReplies  int              `json:"assistant_replies"`
	Models            []usageBreakdown `json:"models"`
	Sessions          []usageBreakdown `json:"sessions"`
	Daily             []usageDaily     `json:"daily"`
	SkippedSessions   int              `json:"skipped_sessions"`
	Records           []usageRecord    `json:"records"`
	RecordTotal       int              `json:"record_total"`
	RecordPage        int              `json:"record_page"`
	RecordPageSize    int              `json:"record_page_size"`
	RecordTotalPages  int              `json:"record_total_pages"`
	RecordProviders   []string         `json:"record_providers"`
	RecordModels      []string         `json:"record_models"`
}

type usageRecord struct {
	ID              string `json:"id"`
	Channel         string `json:"channel,omitempty"`
	Source          string `json:"source,omitempty"`
	SessionID       string `json:"session_id"`
	SessionName     string `json:"session_name,omitempty"`
	MessageID       string `json:"message_id,omitempty"`
	ModelID         string `json:"model_id,omitempty"`
	ModelName       string `json:"model_name,omitempty"`
	Provider        string `json:"provider,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	CreatedAt       int64  `json:"created_at"`
	CreatedAtMS     int64  `json:"created_at_ms"`
	ElapsedMS       int64  `json:"elapsed_ms,omitempty"`
	InputTokens     int    `json:"input_tokens"`
	OutputTokens    int    `json:"output_tokens"`
	CachedTokens    int    `json:"cached_tokens"`
	TotalTokens     int    `json:"total_tokens"`
}

type usageRecordQuery struct {
	From     int64
	To       int64
	Model    string
	Provider string
	Page     int
	PageSize int
}

type usageModelCatalogEntry struct {
	ModelID   string
	ModelName string
	Provider  string
	LLMNo     int
	Enabled   bool
	Order     int
	Sequence  int
}

type usageModelCatalog struct {
	Ordered []usageModelCatalogEntry
	ByModel map[string][]usageModelCatalogEntry
}

const (
	defaultUsageRecordPageSize = 20
	maxUsageRecordPageSize     = 100
	maxUsageExportRecords      = 10000
)

func addUsageValue(dst *usageTotals, key string, value int) {
	if value <= 0 {
		return
	}
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "input_tokens", "prompt_tokens", "input_token_count", "prompt_token_count":
		dst.InputTokens += value
	case "output_tokens", "completion_tokens", "output_token_count", "completion_token_count":
		dst.OutputTokens += value
	case "total_tokens", "total_token_count":
		dst.TotalTokens += value
	default:
		if dst.Other == nil {
			dst.Other = map[string]int{}
		}
		dst.Other[key] += value
	}
}

func normalizedMessageUsage(message chatMessage) (usageTotals, bool) {
	// Prefer usages (per-turn breakdown) when available; it is authoritative because
	// the legacy usage field can be overwritten by a terminal SSE event with all-zero values.
	var values map[string]int
	if len(message.Usages) > 0 {
		values = map[string]int{}
		for _, turn := range message.Usages {
			for key, value := range turn {
				values[key] += value
			}
		}
	} else {
		values = message.Usage
	}
	var totals usageTotals
	for key, value := range values {
		addUsageValue(&totals, key, value)
	}
	// Modern Claude usage reports uncached input, cache creation, and cache read
	// as disjoint categories. Keep the latter two in Other for breakdowns while
	// including them in the billed input total. Legacy cached_tokens is already
	// a subset of input_tokens and must not be added again.
	totals.InputTokens += totals.Other["cache_creation_tokens"] + totals.Other["cache_read_tokens"]
	if totals.TotalTokens == 0 {
		totals.TotalTokens = totals.InputTokens + totals.OutputTokens
	}
	return totals, len(values) > 0
}

func mergeUsageTotals(dst *usageTotals, src usageTotals) {
	dst.InputTokens += src.InputTokens
	dst.OutputTokens += src.OutputTokens
	dst.TotalTokens += src.TotalTokens
	for key, value := range src.Other {
		if dst.Other == nil {
			dst.Other = map[string]int{}
		}
		dst.Other[key] += value
	}
}

func normalizedUsageUnixSeconds(value int64) int64 {
	if value > 1_000_000_000_000 {
		return value / 1000
	}
	return value
}

func usageCachedTokens(totals usageTotals) int {
	cached := 0
	for key, value := range totals.Other {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "cached_tokens", "cache_read_tokens", "cache_read_input_tokens", "cache_creation_input_tokens":
			cached += value
		}
	}
	return cached
}

func usageModelConfigs(profile modelconfig.Profile) []modelconfig.ModelConfig {
	configs := append([]modelconfig.ModelConfig(nil), profile.ModelConfigs...)
	if len(configs) == 0 {
		models := append([]string(nil), profile.Models...)
		if len(models) == 0 && strings.TrimSpace(profile.Model) != "" {
			models = []string{profile.Model}
		}
		for _, model := range models {
			configs = append(configs, modelconfig.ModelConfig{Model: model})
		}
	}
	sort.SliceStable(configs, func(left, right int) bool {
		leftOrder, rightOrder := int(^uint(0)>>1), int(^uint(0)>>1)
		if configs[left].SortOrder != nil {
			leftOrder = *configs[left].SortOrder
		}
		if configs[right].SortOrder != nil {
			rightOrder = *configs[right].SortOrder
		}
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return false
	})
	return configs
}

func (s *Server) usageModelCatalog() usageModelCatalog {
	catalog := usageModelCatalog{Ordered: []usageModelCatalogEntry{}, ByModel: map[string][]usageModelCatalogEntry{}}
	if s == nil || s.CfgStore == nil {
		return catalog
	}
	draft, err := s.loadModelsFromOfficialMyKey(false)
	if err != nil {
		return catalog
	}
	allEntries := make([]usageModelCatalogEntry, 0)
	sequence := 0
	for _, profile := range draft.Profiles {
		provider := chatProviderDisplayName(profile)
		for _, config := range usageModelConfigs(profile) {
			modelID := strings.TrimSpace(config.Model)
			if modelID == "" {
				continue
			}
			modelName := strings.TrimSpace(config.Name)
			if modelName == "" {
				modelName = modelID
			}
			order := int(^uint(0) >> 1)
			if config.SortOrder != nil {
				order = *config.SortOrder
			}
			allEntries = append(allEntries, usageModelCatalogEntry{
				ModelID: modelID, ModelName: modelName, Provider: provider,
				LLMNo: -1, Enabled: modelconfig.ModelConfigEnabled(config), Order: order, Sequence: sequence,
			})
			sequence++
		}
	}
	activeEntries := make([]usageModelCatalogEntry, 0, len(allEntries))
	for _, entry := range allEntries {
		if entry.Enabled {
			activeEntries = append(activeEntries, entry)
		}
	}
	sort.SliceStable(activeEntries, func(left, right int) bool {
		if activeEntries[left].Order != activeEntries[right].Order {
			return activeEntries[left].Order < activeEntries[right].Order
		}
		return activeEntries[left].Sequence < activeEntries[right].Sequence
	})
	for index := range activeEntries {
		activeEntries[index].LLMNo = index
	}
	catalog.Ordered = activeEntries
	for _, entry := range activeEntries {
		catalog.ByModel[entry.ModelID] = append(catalog.ByModel[entry.ModelID], entry)
	}
	for _, entry := range allEntries {
		if !entry.Enabled {
			catalog.ByModel[entry.ModelID] = append(catalog.ByModel[entry.ModelID], entry)
		}
	}
	return catalog
}

func resolveUsageModel(entry usageLedgerEntry, catalog usageModelCatalog) (string, string) {
	modelID := strings.TrimSpace(entry.ModelID)
	if entry.LLMNo != nil && *entry.LLMNo >= 0 && *entry.LLMNo < len(catalog.Ordered) {
		candidate := catalog.Ordered[*entry.LLMNo]
		if modelID == "" || candidate.ModelID == modelID {
			return candidate.Provider, candidate.ModelName
		}
	}
	candidates := catalog.ByModel[modelID]
	if len(candidates) == 1 {
		return candidates[0].Provider, candidates[0].ModelName
	}
	if len(candidates) > 1 && entry.LLMNo != nil {
		for _, candidate := range candidates {
			if candidate.LLMNo == *entry.LLMNo {
				return candidate.Provider, candidate.ModelName
			}
		}
	}
	return "", modelID
}

func usageRecordFromEntry(entry usageLedgerEntry, catalog usageModelCatalog) usageRecord {
	createdAt := normalizedUsageUnixSeconds(entry.CreatedAt)
	provider, modelName := resolveUsageModel(entry, catalog)
	if strings.TrimSpace(entry.Provider) != "" {
		provider = entry.Provider
	}
	if modelName == "" {
		modelName = strings.TrimSpace(entry.ModelID)
	}
	return usageRecord{
		ID:              entry.Key,
		Channel:         strings.TrimSpace(entry.Channel),
		Source:          strings.TrimSpace(entry.Source),
		SessionID:       entry.SessionID,
		SessionName:     strings.TrimSpace(entry.Title),
		MessageID:       entry.MessageID,
		ModelID:         strings.TrimSpace(entry.ModelID),
		ModelName:       modelName,
		Provider:        provider,
		ReasoningEffort: strings.TrimSpace(entry.ReasoningEffort),
		CreatedAt:       createdAt,
		CreatedAtMS:     createdAt * 1000,
		ElapsedMS:       entry.ElapsedMS,
		InputTokens:     entry.Totals.InputTokens,
		OutputTokens:    entry.Totals.OutputTokens,
		CachedTokens:    usageCachedTokens(entry.Totals),
		TotalTokens:     entry.Totals.TotalTokens,
	}
}

func usageRecordsFromLedger(ledger usageLedger, catalog usageModelCatalog) []usageRecord {
	records := make([]usageRecord, 0, len(ledger.Entries))
	for _, entry := range ledger.Entries {
		records = append(records, usageRecordFromEntry(entry, catalog))
	}
	sort.SliceStable(records, func(left, right int) bool {
		if records[left].CreatedAt != records[right].CreatedAt {
			return records[left].CreatedAt > records[right].CreatedAt
		}
		return records[left].ID > records[right].ID
	})
	return records
}

func parseUsageDate(value string, endOfDay bool) (int64, error) {
	date := strings.TrimSpace(value)
	if date == "" {
		return 0, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return 0, fmt.Errorf("invalid date %q, expected YYYY-MM-DD", date)
	}
	if endOfDay {
		parsed = parsed.AddDate(0, 0, 1).Add(-time.Nanosecond)
	}
	return parsed.Unix(), nil
}

func parseUsageRecordQuery(r *http.Request) (usageRecordQuery, error) {
	query := r.URL.Query()
	from, err := parseUsageDate(query.Get("from"), false)
	if err != nil {
		return usageRecordQuery{}, err
	}
	to, err := parseUsageDate(query.Get("to"), true)
	if err != nil {
		return usageRecordQuery{}, err
	}
	if from > 0 && to > 0 && from > to {
		return usageRecordQuery{}, fmt.Errorf("from date must not be later than to date")
	}
	recordQuery := usageRecordQuery{From: from, To: to, Model: strings.TrimSpace(query.Get("model")), Provider: strings.TrimSpace(query.Get("provider")), Page: 1, PageSize: defaultUsageRecordPageSize}
	if rawPage := strings.TrimSpace(query.Get("page")); rawPage != "" {
		recordQuery.Page, err = strconv.Atoi(rawPage)
		if err != nil || recordQuery.Page < 1 {
			return usageRecordQuery{}, fmt.Errorf("page must be a positive integer")
		}
	}
	if rawPageSize := strings.TrimSpace(query.Get("page_size")); rawPageSize != "" {
		recordQuery.PageSize, err = strconv.Atoi(rawPageSize)
		if err != nil || recordQuery.PageSize < 1 || recordQuery.PageSize > maxUsageRecordPageSize {
			return usageRecordQuery{}, fmt.Errorf("page_size must be between 1 and %d", maxUsageRecordPageSize)
		}
	}
	return recordQuery, nil
}

func usageRecordMatches(record usageRecord, query usageRecordQuery) bool {
	if query.From > 0 && record.CreatedAt < query.From {
		return false
	}
	if query.To > 0 && record.CreatedAt > query.To {
		return false
	}
	if query.Provider != "" && !strings.EqualFold(record.Provider, query.Provider) {
		return false
	}
	if query.Model != "" {
		search := strings.ToLower(query.Model)
		fields := []string{record.ModelID, record.ModelName, record.Provider, record.SessionName, record.SessionID, record.Channel, record.Source, record.ReasoningEffort}
		matched := false
		for _, field := range fields {
			if strings.Contains(strings.ToLower(field), search) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func filterUsageRecords(records []usageRecord, query usageRecordQuery) []usageRecord {
	filtered := make([]usageRecord, 0, len(records))
	for _, record := range records {
		if usageRecordMatches(record, query) {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func uniqueUsageRecordValues(records []usageRecord, provider bool) []string {
	seen := map[string]bool{}
	values := make([]string, 0)
	for _, record := range records {
		value := record.ModelName
		if provider {
			value = record.Provider
		}
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func paginateUsageRecords(records []usageRecord, query usageRecordQuery) ([]usageRecord, int, int) {
	total := len(records)
	totalPages := 0
	if total > 0 {
		totalPages = (total + query.PageSize - 1) / query.PageSize
	}
	start := (query.Page - 1) * query.PageSize
	if start >= total {
		return []usageRecord{}, total, totalPages
	}
	end := start + query.PageSize
	if end > total {
		end = total
	}
	return records[start:end], total, totalPages
}

func (s *Server) usageOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	query, err := parseUsageRecordQuery(r)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	ledger, skipped, err := s.loadOrMigrateUsageLedger()
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	ledger = enrichUsageLedgerMetadata(s.CfgStore.Cfg, ledger)

	response := usageOverviewResponse{
		Models:          []usageBreakdown{},
		Sessions:        []usageBreakdown{},
		Daily:           []usageDaily{},
		Records:         []usageRecord{},
		RecordProviders: []string{},
		RecordModels:    []string{},
		SkippedSessions: skipped,
	}
	models := map[string]*usageBreakdown{}
	sessions := map[string]*usageBreakdown{}
	daily := map[string]*usageDaily{}
	for _, entry := range ledger.Entries {
		response.AssistantReplies++
		mergeUsageTotals(&response.Totals, entry.Totals)

		session := sessions[entry.SessionID]
		if session == nil {
			name := strings.TrimSpace(entry.Title)
			if name == "" {
				name = entry.SessionID
			}
			session = &usageBreakdown{ID: entry.SessionID, Name: name}
			sessions[entry.SessionID] = session
		}
		session.AssistantReplies++
		mergeUsageTotals(&session.Totals, entry.Totals)
		if entry.CreatedAt > session.UpdatedAt {
			session.UpdatedAt = entry.CreatedAt
		}

		if entry.CreatedAt > 0 {
			createdAt := entry.CreatedAt
			if createdAt > 1_000_000_000_000 {
				createdAt /= 1000
			}
			date := time.Unix(createdAt, 0).In(time.Local).Format("2006-01-02")
			day := daily[date]
			if day == nil {
				day = &usageDaily{Date: date}
				daily[date] = day
			}
			day.AssistantReplies++
			mergeUsageTotals(&day.Totals, entry.Totals)
		}

		modelID := strings.TrimSpace(entry.ModelID)
		if modelID != "" {
			model := models[modelID]
			if model == nil {
				model = &usageBreakdown{ID: modelID, Name: modelID}
				models[modelID] = model
			}
			model.AssistantReplies++
			mergeUsageTotals(&model.Totals, entry.Totals)
		}
	}
	response.SessionCount = len(sessions)
	response.SessionsWithUsage = len(sessions)
	for _, model := range models {
		response.Models = append(response.Models, *model)
	}
	for _, session := range sessions {
		response.Sessions = append(response.Sessions, *session)
	}
	for _, day := range daily {
		response.Daily = append(response.Daily, *day)
	}
	sort.Slice(response.Models, func(i, j int) bool {
		if response.Models[i].Totals.TotalTokens == response.Models[j].Totals.TotalTokens {
			return response.Models[i].Name < response.Models[j].Name
		}
		return response.Models[i].Totals.TotalTokens > response.Models[j].Totals.TotalTokens
	})
	sort.Slice(response.Sessions, func(i, j int) bool {
		return response.Sessions[i].UpdatedAt > response.Sessions[j].UpdatedAt
	})
	sort.Slice(response.Daily, func(i, j int) bool {
		return response.Daily[i].Date < response.Daily[j].Date
	})
	catalog := s.usageModelCatalog()
	allRecords := usageRecordsFromLedger(ledger, catalog)
	filteredRecords := filterUsageRecords(allRecords, query)
	response.Records, response.RecordTotal, response.RecordTotalPages = paginateUsageRecords(filteredRecords, query)
	response.RecordPage = query.Page
	response.RecordPageSize = query.PageSize
	response.RecordProviders = uniqueUsageRecordValues(allRecords, true)
	response.RecordModels = uniqueUsageRecordValues(allRecords, false)
	writeJSON(w, response)
}

func usageRecordCSVValue(value int) string {
	return strconv.Itoa(value)
}

func (s *Server) usageExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	query, err := parseUsageRecordQuery(r)
	if err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	ledger, err := s.readUsageLedgerOnly()
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	ledger = enrichUsageLedgerMetadata(s.CfgStore.Cfg, ledger)
	records := filterUsageRecords(usageRecordsFromLedger(ledger, s.usageModelCatalog()), query)
	truncated := len(records) > maxUsageExportRecords
	if truncated {
		records = records[:maxUsageExportRecords]
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="usage-records.csv"`)
	if truncated {
		w.Header().Set("X-Usage-Export-Truncated", "true")
	}
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	csvWriter := csv.NewWriter(w)
	_ = csvWriter.Write([]string{"时间", "调用渠道", "来源", "推理强度", "服务商", "模型", "会话", "输入 Token", "缓存 Token", "输出 Token", "总 Token", "耗时"})
	for _, record := range records {
		elapsed := ""
		if record.ElapsedMS > 0 {
			elapsed = strconv.FormatInt(record.ElapsedMS, 10) + " ms"
		}
		provider := record.Provider
		if strings.TrimSpace(provider) == "" {
			provider = "历史记录未保存服务商"
		}
		_ = csvWriter.Write([]string{
			time.Unix(record.CreatedAt, 0).In(time.Local).Format("2006-01-02 15:04:05"),
			record.Channel,
			record.Source,
			record.ReasoningEffort,
			provider,
			record.ModelName,
			record.SessionName,
			usageRecordCSVValue(record.InputTokens),
			usageRecordCSVValue(record.CachedTokens),
			usageRecordCSVValue(record.OutputTokens),
			usageRecordCSVValue(record.TotalTokens),
			elapsed,
		})
	}
	csvWriter.Flush()
}
