package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"genericagent-admin-go/internal/config"
)

const usageLedgerVersion = 1

type usageLedgerEntry struct {
	Key             string      `json:"key"`
	Channel         string      `json:"channel,omitempty"`
	Source          string      `json:"source,omitempty"`
	SessionID       string      `json:"session_id"`
	Title           string      `json:"title,omitempty"`
	MessageID       string      `json:"message_id,omitempty"`
	ModelID         string      `json:"model_id,omitempty"`
	Provider        string      `json:"provider,omitempty"`
	LLMNo           *int        `json:"llm_no,omitempty"`
	ReasoningEffort string      `json:"reasoning_effort,omitempty"`
	CreatedAt       int64       `json:"created_at,omitempty"`
	ElapsedMS       int64       `json:"elapsed_ms,omitempty"`
	Totals          usageTotals `json:"totals"`
}

type usageEvent struct {
	ID              string         `json:"id"`
	Channel         string         `json:"channel"`
	Source          string         `json:"source"`
	SessionID       string         `json:"session_id"`
	SessionName     string         `json:"session_name"`
	ModelID         string         `json:"model_id"`
	LLMNo           *int           `json:"llm_no"`
	ReasoningEffort string         `json:"reasoning_effort"`
	CreatedAt       int64          `json:"created_at"`
	ElapsedMS       int64          `json:"elapsed_ms"`
	Totals          map[string]int `json:"totals"`
}

type usageLedger struct {
	Version int                `json:"version"`
	Entries []usageLedgerEntry `json:"entries"`
}

func usageLedgerPath(cfg config.AppConfig) string {
	return filepath.Join(chatDataDir(cfg), "usage_ledger.json")
}

func usageEventDir(cfg config.AppConfig) string {
	return filepath.Join(chatDataDir(cfg), "usage_events")
}

func usageEntryKey(sessionID, messageID string, index int) string {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		messageID = fmt.Sprintf("legacy-%d", index)
	}
	return sessionID + ":" + messageID
}

func usageEntriesFromSession(cs chatSession) []usageLedgerEntry {
	entries := make([]usageLedgerEntry, 0)
	for index, message := range cs.Messages {
		if message.Role != "assistant" {
			continue
		}
		totals, ok := normalizedMessageUsage(message)
		if !ok {
			continue
		}
		entries = append(entries, usageLedgerEntry{
			Key:             usageEntryKey(cs.ID, message.ID, index),
			Channel:         "chat",
			Source:          "chat",
			SessionID:       cs.ID,
			Title:           cs.Title,
			MessageID:       message.ID,
			ModelID:         message.ModelID,
			LLMNo:           message.LLMNo,
			ReasoningEffort: message.ReasoningEffort,
			CreatedAt:       message.CreatedAt,
			ElapsedMS:       message.ElapsedMS,
			Totals:          totals,
		})
	}
	return entries
}

func readUsageEvents(cfg config.AppConfig) ([]usageLedgerEntry, error) {
	files, err := os.ReadDir(usageEventDir(cfg))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entries := make([]usageLedgerEntry, 0)
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
			continue
		}
		contents, readErr := os.ReadFile(filepath.Join(usageEventDir(cfg), file.Name()))
		if readErr != nil {
			continue
		}
		for _, line := range strings.Split(string(contents), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var event usageEvent
			if json.Unmarshal([]byte(line), &event) != nil || strings.TrimSpace(event.ID) == "" {
				continue
			}
			totals := usageTotals{}
			for key, value := range event.Totals {
				addUsageValue(&totals, key, value)
			}
			if totals.TotalTokens == 0 {
				totals.TotalTokens = totals.InputTokens + totals.OutputTokens
			}
			if totals.TotalTokens <= 0 {
				continue
			}
			entries = append(entries, usageLedgerEntry{
				Key:             "ga-usage:" + event.ID,
				Channel:         strings.TrimSpace(event.Channel),
				Source:          strings.TrimSpace(event.Source),
				SessionID:       strings.TrimSpace(event.SessionID),
				Title:           strings.TrimSpace(event.SessionName),
				ModelID:         strings.TrimSpace(event.ModelID),
				LLMNo:           event.LLMNo,
				ReasoningEffort: strings.TrimSpace(event.ReasoningEffort),
				CreatedAt:       event.CreatedAt,
				ElapsedMS:       event.ElapsedMS,
				Totals:          totals,
			})
		}
	}
	return entries, nil
}

func mergeUsageEntries(ledger *usageLedger, entries []usageLedgerEntry) bool {
	if len(entries) == 0 {
		return false
	}
	byKey := make(map[string]int, len(ledger.Entries)+len(entries))
	for index := range ledger.Entries {
		byKey[ledger.Entries[index].Key] = index
	}
	changed := false
	for _, entry := range entries {
		if _, ok := byKey[entry.Key]; ok {
			continue
		}
		byKey[entry.Key] = len(ledger.Entries)
		ledger.Entries = append(ledger.Entries, entry)
		changed = true
	}
	return changed
}

func ingestUsageEvents(cfg config.AppConfig, ledger usageLedger) (usageLedger, bool, error) {
	events, err := readUsageEvents(cfg)
	if err != nil {
		return usageLedger{}, false, err
	}
	changed := mergeUsageEntries(&ledger, events)
	return ledger, changed, nil
}

func readUsageLedger(cfg config.AppConfig) (usageLedger, error) {
	contents, err := os.ReadFile(usageLedgerPath(cfg))
	if err != nil {
		return usageLedger{}, err
	}
	var ledger usageLedger
	if err := json.Unmarshal(contents, &ledger); err != nil {
		return usageLedger{}, err
	}
	if ledger.Version != usageLedgerVersion {
		return usageLedger{}, fmt.Errorf("unsupported usage ledger version %d", ledger.Version)
	}
	if ledger.Entries == nil {
		ledger.Entries = []usageLedgerEntry{}
	}
	return ledger, nil
}

func writeUsageLedger(cfg config.AppConfig, ledger usageLedger) error {
	ledger.Version = usageLedgerVersion
	sort.Slice(ledger.Entries, func(i, j int) bool { return ledger.Entries[i].Key < ledger.Entries[j].Key })
	contents, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	return writeChatFileAtomic(usageLedgerPath(cfg), contents, 0644)
}

func migrateUsageLedger(cfg config.AppConfig) (usageLedger, int, error) {
	ledger := usageLedger{Version: usageLedgerVersion, Entries: []usageLedgerEntry{}}
	dir := chatSessionDir(cfg)
	files, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return usageLedger{}, 0, err
	}
	skipped := 0
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		contents, readErr := os.ReadFile(filepath.Join(dir, file.Name()))
		var cs chatSession
		if readErr == nil {
			readErr = json.Unmarshal(contents, &cs)
		}
		if readErr != nil {
			skipped++
			continue
		}
		if strings.TrimSpace(cs.ID) == "" {
			cs.ID = strings.TrimSuffix(file.Name(), ".json")
		}
		ledger.Entries = append(ledger.Entries, usageEntriesFromSession(cs)...)
	}
	if err := writeUsageLedger(cfg, ledger); err != nil {
		return usageLedger{}, skipped, err
	}
	return ledger, skipped, nil
}

func (s *Server) loadOrMigrateUsageLedger() (usageLedger, int, error) {
	s.UsageMu.Lock()
	defer s.UsageMu.Unlock()
	ledger, err := readUsageLedger(s.CfgStore.Snapshot())
	if err == nil {
		ledger, changed, ingestErr := ingestUsageEvents(s.CfgStore.Cfg, ledger)
		if ingestErr != nil {
			return usageLedger{}, 0, ingestErr
		}
		if changed {
			if err := writeUsageLedger(s.CfgStore.Cfg, ledger); err != nil {
				return usageLedger{}, 0, err
			}
		}
		return ledger, 0, nil
	}
	if !os.IsNotExist(err) {
		return usageLedger{}, 0, err
	}
	ledger, skipped, err := migrateUsageLedger(s.CfgStore.Cfg)
	if err != nil {
		return usageLedger{}, 0, err
	}
	ledger, changed, err := ingestUsageEvents(s.CfgStore.Cfg, ledger)
	if err != nil {
		return usageLedger{}, 0, err
	}
	if changed {
		err = writeUsageLedger(s.CfgStore.Cfg, ledger)
	}
	return ledger, skipped, err
}

func (s *Server) readUsageLedgerOnly() (usageLedger, error) {
	s.UsageMu.Lock()
	defer s.UsageMu.Unlock()
	ledger, err := readUsageLedger(s.CfgStore.Cfg)
	if os.IsNotExist(err) {
		ledger = usageLedger{Version: usageLedgerVersion, Entries: []usageLedgerEntry{}}
		return ingestUsageEventsOnly(s.CfgStore.Cfg, ledger)
	}
	if err != nil {
		return usageLedger{}, err
	}
	return ingestUsageEventsOnly(s.CfgStore.Cfg, ledger)
}

func ingestUsageEventsOnly(cfg config.AppConfig, ledger usageLedger) (usageLedger, error) {
	merged, _, err := ingestUsageEvents(cfg, ledger)
	if err != nil {
		return usageLedger{}, err
	}
	return merged, nil
}

func enrichUsageLedgerMetadata(cfg config.AppConfig, ledger usageLedger) usageLedger {
	if len(ledger.Entries) == 0 {
		return ledger
	}
	byKey := make(map[string]int, len(ledger.Entries))
	for index := range ledger.Entries {
		byKey[ledger.Entries[index].Key] = index
	}
	files, err := os.ReadDir(chatSessionDir(cfg))
	if err != nil {
		return ledger
	}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		contents, readErr := os.ReadFile(filepath.Join(chatSessionDir(cfg), file.Name()))
		if readErr != nil {
			continue
		}
		var session chatSession
		if json.Unmarshal(contents, &session) != nil {
			continue
		}
		if strings.TrimSpace(session.ID) == "" {
			session.ID = strings.TrimSuffix(file.Name(), ".json")
		}
		for messageIndex, message := range session.Messages {
			if message.Role != "assistant" {
				continue
			}
			entryIndex, ok := byKey[usageEntryKey(session.ID, message.ID, messageIndex)]
			if !ok {
				continue
			}
			entry := &ledger.Entries[entryIndex]
			if strings.TrimSpace(session.Title) != "" {
				entry.Title = session.Title
			}
			if strings.TrimSpace(entry.ModelID) == "" {
				entry.ModelID = message.ModelID
			}
			if entry.LLMNo == nil && message.LLMNo != nil {
				value := *message.LLMNo
				entry.LLMNo = &value
			}
			if entry.CreatedAt == 0 {
				entry.CreatedAt = message.CreatedAt
			}
			if entry.ElapsedMS <= 0 && message.ElapsedMS > 0 {
				entry.ElapsedMS = message.ElapsedMS
			}
		}
	}
	return ledger
}

func (s *Server) recordSessionUsage(cs chatSession) error {
	s.UsageMu.Lock()
	defer s.UsageMu.Unlock()
	ledger, err := readUsageLedger(s.CfgStore.Snapshot())
	if os.IsNotExist(err) {
		ledger, _, err = migrateUsageLedger(s.CfgStore.Snapshot())
	}
	if err != nil {
		return err
	}
	var eventsChanged bool
	ledger, eventsChanged, err = ingestUsageEvents(s.CfgStore.Cfg, ledger)
	if err != nil {
		return err
	}
	byKey := make(map[string]int, len(ledger.Entries))
	for index := range ledger.Entries {
		byKey[ledger.Entries[index].Key] = index
	}
	changed := eventsChanged
	for _, entry := range usageEntriesFromSession(cs) {
		if index, ok := byKey[entry.Key]; ok {
			if ledger.Entries[index].Title != entry.Title {
				ledger.Entries[index].Title = entry.Title
				changed = true
			}
			// Always refresh Totals and ModelID so stale zero-values get corrected.
			ledger.Entries[index].Totals = entry.Totals
			ledger.Entries[index].ModelID = entry.ModelID
			ledger.Entries[index].Channel = entry.Channel
			ledger.Entries[index].Source = entry.Source
			ledger.Entries[index].Provider = entry.Provider
			ledger.Entries[index].LLMNo = entry.LLMNo
			ledger.Entries[index].ReasoningEffort = entry.ReasoningEffort
			ledger.Entries[index].CreatedAt = entry.CreatedAt
			ledger.Entries[index].ElapsedMS = entry.ElapsedMS
			changed = true
			continue
		}
		byKey[entry.Key] = len(ledger.Entries)
		ledger.Entries = append(ledger.Entries, entry)
		changed = true
	}
	if !changed {
		return nil
	}
	return writeUsageLedger(s.CfgStore.Snapshot(), ledger)
}

func (s *Server) recordUsageEntries(entries []usageLedgerEntry) error {
	if len(entries) == 0 {
		return nil
	}
	s.UsageMu.Lock()
	defer s.UsageMu.Unlock()
	ledger, err := readUsageLedger(s.CfgStore.Cfg)
	if os.IsNotExist(err) {
		ledger, _, err = migrateUsageLedger(s.CfgStore.Cfg)
	}
	if err != nil {
		return err
	}
	ledger, eventsChanged, err := ingestUsageEvents(s.CfgStore.Cfg, ledger)
	if err != nil {
		return err
	}
	changed := eventsChanged || mergeUsageEntries(&ledger, entries)
	if !changed {
		return nil
	}
	return writeUsageLedger(s.CfgStore.Cfg, ledger)
}
