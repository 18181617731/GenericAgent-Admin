package ga

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	adminContractSchemaVersion = 1
	contractDomainScheduleTask = "schedule_task"
	contractDomainGoalState    = "goal_state"
	contractDomainReportIndex  = "report_index"
)

type ContractMeta struct {
	SchemaVersion int    `json:"schema_version"`
	Domain        string `json:"domain"`
	Path          string `json:"path,omitempty"`
	Compatible    bool   `json:"compatible"`
	Legacy        bool   `json:"legacy,omitempty"`
	Warning       string `json:"warning,omitempty"`
}

type ReportIndex struct {
	SchemaVersion int       `json:"schema_version"`
	Domain        string    `json:"domain"`
	Generated     time.Time `json:"generated_at"`
	Roots         []string  `json:"roots"`
	Reports       []Entry   `json:"reports"`
	Count         int       `json:"count"`
}

func readContractJSON(path, domain string, dst any) (ContractMeta, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ContractMeta{}, err
	}
	var hdr struct {
		SchemaVersion int    `json:"schema_version"`
		Domain        string `json:"domain"`
	}
	if err := json.Unmarshal(b, &hdr); err != nil {
		return ContractMeta{}, err
	}
	meta := ContractMeta{SchemaVersion: hdr.SchemaVersion, Domain: hdr.Domain, Path: filepath.ToSlash(path), Compatible: true}
	if hdr.SchemaVersion == 0 {
		meta.SchemaVersion = adminContractSchemaVersion
		meta.Legacy = true
		meta.Domain = domain
	} else if hdr.SchemaVersion > adminContractSchemaVersion {
		return meta, fmt.Errorf("unsupported %s schema_version %d", domain, hdr.SchemaVersion)
	}
	if strings.TrimSpace(hdr.Domain) != "" && hdr.Domain != domain {
		return meta, fmt.Errorf("unexpected %s domain %q", domain, hdr.Domain)
	}
	meta.Domain = domain
	if err := json.Unmarshal(b, dst); err != nil {
		return meta, err
	}
	return meta, nil
}

func normalizeContractMap(raw map[string]any, domain string) map[string]any {
	if raw == nil {
		raw = map[string]any{}
	}
	out := make(map[string]any, len(raw)+2)
	for k, v := range raw {
		out[k] = v
	}
	out["schema_version"] = adminContractSchemaVersion
	out["domain"] = domain
	return out
}

func marshalContractJSON(raw map[string]any, domain string) ([]byte, error) {
	if raw == nil {
		return nil, errors.New("empty contract payload")
	}
	return json.MarshalIndent(normalizeContractMap(raw, domain), "", "  ")
}

func buildReportIndex(root string, reports []Entry) ReportIndex {
	roots := []string{"autonomous_reports", filepath.ToSlash(filepath.Join("temp", "autonomous_reports"))}
	return ReportIndex{SchemaVersion: adminContractSchemaVersion, Domain: contractDomainReportIndex, Generated: time.Now(), Roots: roots, Reports: reports, Count: len(reports)}
}
