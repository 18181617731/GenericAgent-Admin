package ga

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const (
	scheduleFoldersSchemaVersion = 1
	scheduleFoldersRelativePath  = "temp/ga-admin-schedule-folders.json"
	maxScheduleFolders           = 200
	maxScheduleFolderNameRunes   = 64
	maxScheduleFolderIDRunes     = 80
)

// ScheduleFolder is an Admin-side grouping. It deliberately lives in a
// separate metadata file so the upstream scheduler can continue scanning only
// sche_tasks/*.json and task JSON remains portable.
type ScheduleFolder struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Order int    `json:"order"`
}

// ScheduleFolders stores a single folder assignment per task. An empty folder
// id means the task is in the built-in “unassigned” inbox.
type ScheduleFolders struct {
	SchemaVersion int               `json:"schema_version"`
	Folders       []ScheduleFolder  `json:"folders"`
	Assignments   map[string]string `json:"assignments"`
	UpdatedAt     time.Time         `json:"updated_at,omitempty"`
}

func emptyScheduleFolders() ScheduleFolders {
	return ScheduleFolders{SchemaVersion: scheduleFoldersSchemaVersion, Folders: []ScheduleFolder{}, Assignments: map[string]string{}}
}

func scheduleFoldersPath(root string) string {
	return filepath.Join(root, filepath.FromSlash(scheduleFoldersRelativePath))
}

// LoadScheduleFolders reads the optional grouping index. A missing file is a
// valid empty state; malformed or unsafe state is surfaced to the API caller
// instead of silently moving cards to another folder.
func LoadScheduleFolders(root string) (ScheduleFolders, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return emptyScheduleFolders(), nil
	}
	data, err := os.ReadFile(scheduleFoldersPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return emptyScheduleFolders(), nil
	}
	if err != nil {
		return ScheduleFolders{}, fmt.Errorf("read schedule folders: %w", err)
	}
	state := emptyScheduleFolders()
	if err := json.Unmarshal(data, &state); err != nil {
		return ScheduleFolders{}, fmt.Errorf("parse schedule folders: %w", err)
	}
	if err := validateScheduleFolders(state); err != nil {
		return ScheduleFolders{}, err
	}
	return normalizeScheduleFolders(state), nil
}

// SaveScheduleFolders validates and atomically writes the grouping index. It
// never touches sche_tasks task files, so a failed write cannot lose a task.
func SaveScheduleFolders(root string, state ScheduleFolders) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return errors.New("ga_root is empty")
	}
	state = normalizeScheduleFolders(state)
	if err := validateScheduleFolders(state); err != nil {
		return err
	}
	state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode schedule folders: %w", err)
	}
	path := scheduleFoldersPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create schedule folder metadata directory: %w", err)
	}
	if old, err := os.ReadFile(path); err == nil {
		// Keep a recoverable copy for the same reason task edits create .bak
		// files. The timestamp makes concurrent edits non-destructive.
		bak := path + ".bak." + time.Now().UTC().Format("20060102_150405.000000000")
		if err := writeFileAtomic(bak, old, 0644); err != nil {
			return fmt.Errorf("back up schedule folders: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read existing schedule folders: %w", err)
	}
	if err := writeFileAtomic(path, data, 0644); err != nil {
		return fmt.Errorf("write schedule folders: %w", err)
	}
	return nil
}

func CreateScheduleFolder(root, name string) (ScheduleFolders, error) {
	state, err := LoadScheduleFolders(root)
	if err != nil {
		return ScheduleFolders{}, err
	}
	name, err = normalizeScheduleFolderName(name)
	if err != nil {
		return ScheduleFolders{}, err
	}
	for _, folder := range state.Folders {
		if strings.EqualFold(folder.Name, name) {
			return ScheduleFolders{}, errors.New("a schedule folder with this name already exists")
		}
	}
	if len(state.Folders) >= maxScheduleFolders {
		return ScheduleFolders{}, fmt.Errorf("schedule folder limit is %d", maxScheduleFolders)
	}
	id := newScheduleFolderID(state.Folders)
	state.Folders = append(state.Folders, ScheduleFolder{ID: id, Name: name, Order: len(state.Folders)})
	if err := SaveScheduleFolders(root, state); err != nil {
		return ScheduleFolders{}, err
	}
	return LoadScheduleFolders(root)
}

func RenameScheduleFolder(root, id, name string) (ScheduleFolders, error) {
	state, err := LoadScheduleFolders(root)
	if err != nil {
		return ScheduleFolders{}, err
	}
	id = strings.TrimSpace(id)
	name, err = normalizeScheduleFolderName(name)
	if err != nil {
		return ScheduleFolders{}, err
	}
	found := false
	for i := range state.Folders {
		if state.Folders[i].ID == id {
			found = true
			continue
		}
		if strings.EqualFold(state.Folders[i].Name, name) {
			return ScheduleFolders{}, errors.New("a schedule folder with this name already exists")
		}
	}
	if !found {
		return ScheduleFolders{}, errors.New("schedule folder not found")
	}
	for i := range state.Folders {
		if state.Folders[i].ID == id {
			state.Folders[i].Name = name
			break
		}
	}
	if err := SaveScheduleFolders(root, state); err != nil {
		return ScheduleFolders{}, err
	}
	return LoadScheduleFolders(root)
}

func DeleteScheduleFolder(root, id string) (ScheduleFolders, error) {
	state, err := LoadScheduleFolders(root)
	if err != nil {
		return ScheduleFolders{}, err
	}
	id = strings.TrimSpace(id)
	if !scheduleFolderIDValid(id) {
		return ScheduleFolders{}, errors.New("invalid schedule folder id")
	}
	filtered := make([]ScheduleFolder, 0, len(state.Folders))
	found := false
	for _, folder := range state.Folders {
		if folder.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, folder)
	}
	if !found {
		return ScheduleFolders{}, errors.New("schedule folder not found")
	}
	state.Folders = filtered
	for taskID, folderID := range state.Assignments {
		if folderID == id {
			delete(state.Assignments, taskID)
		}
	}
	for i := range state.Folders {
		state.Folders[i].Order = i
	}
	if err := SaveScheduleFolders(root, state); err != nil {
		return ScheduleFolders{}, err
	}
	return LoadScheduleFolders(root)
}

func MoveScheduleTask(root, taskID, folderID string) (ScheduleFolders, error) {
	state, err := LoadScheduleFolders(root)
	if err != nil {
		return ScheduleFolders{}, err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || strings.Contains(taskID, "..") || strings.ContainsAny(taskID, `/\\`) || filepath.IsAbs(taskID) {
		return ScheduleFolders{}, errors.New("invalid schedule task id")
	}
	folderID = strings.TrimSpace(folderID)
	if folderID != "" {
		if !scheduleFolderIDValid(folderID) {
			return ScheduleFolders{}, errors.New("invalid schedule folder id")
		}
		found := false
		for _, folder := range state.Folders {
			if folder.ID == folderID {
				found = true
				break
			}
		}
		if !found {
			return ScheduleFolders{}, errors.New("schedule folder not found")
		}
		state.Assignments[taskID] = folderID
	} else {
		delete(state.Assignments, taskID)
	}
	if err := SaveScheduleFolders(root, state); err != nil {
		return ScheduleFolders{}, err
	}
	return LoadScheduleFolders(root)
}

func normalizeScheduleFolders(state ScheduleFolders) ScheduleFolders {
	if state.SchemaVersion == 0 {
		state.SchemaVersion = scheduleFoldersSchemaVersion
	}
	if state.Folders == nil {
		state.Folders = []ScheduleFolder{}
	}
	if state.Assignments == nil {
		state.Assignments = map[string]string{}
	}
	return state
}

func validateScheduleFolders(state ScheduleFolders) error {
	state = normalizeScheduleFolders(state)
	if state.SchemaVersion != scheduleFoldersSchemaVersion {
		return fmt.Errorf("unsupported schedule folders schema_version %d", state.SchemaVersion)
	}
	if len(state.Folders) > maxScheduleFolders {
		return fmt.Errorf("schedule folder limit is %d", maxScheduleFolders)
	}
	ids := make(map[string]struct{}, len(state.Folders))
	names := make(map[string]struct{}, len(state.Folders))
	for _, folder := range state.Folders {
		if !scheduleFolderIDValid(folder.ID) {
			return fmt.Errorf("invalid schedule folder id %q", folder.ID)
		}
		name, err := normalizeScheduleFolderName(folder.Name)
		if err != nil {
			return err
		}
		if name != folder.Name {
			return errors.New("schedule folder name must be trimmed")
		}
		if _, exists := ids[folder.ID]; exists {
			return fmt.Errorf("duplicate schedule folder id %q", folder.ID)
		}
		key := strings.ToLower(name)
		if _, exists := names[key]; exists {
			return fmt.Errorf("duplicate schedule folder name %q", name)
		}
		ids[folder.ID] = struct{}{}
		names[key] = struct{}{}
	}
	for taskID, folderID := range state.Assignments {
		if strings.TrimSpace(taskID) == "" || strings.Contains(taskID, "..") || strings.ContainsAny(taskID, `/\\`) || filepath.IsAbs(taskID) {
			return fmt.Errorf("invalid schedule task assignment %q", taskID)
		}
		if !scheduleFolderIDValid(folderID) {
			return fmt.Errorf("assignment for %q references invalid folder %q", taskID, folderID)
		}
		if _, exists := ids[folderID]; !exists {
			return fmt.Errorf("assignment for %q references missing folder %q", taskID, folderID)
		}
	}
	return nil
}

func normalizeScheduleFolderName(name string) (string, error) {
	name = strings.TrimSpace(name)
	runes := []rune(name)
	if len(runes) == 0 {
		return "", errors.New("schedule folder name is required")
	}
	if len(runes) > maxScheduleFolderNameRunes {
		return "", fmt.Errorf("schedule folder name cannot exceed %d characters", maxScheduleFolderNameRunes)
	}
	for _, r := range runes {
		if unicode.IsControl(r) {
			return "", errors.New("schedule folder name cannot contain control characters")
		}
	}
	return name, nil
}

func scheduleFolderIDValid(id string) bool {
	if id == "" || len([]rune(id)) > maxScheduleFolderIDRunes {
		return false
	}
	for i, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			if i == 0 && (r == '-' || r == '_') {
				return false
			}
			continue
		}
		return false
	}
	return true
}

func newScheduleFolderID(folders []ScheduleFolder) string {
	base := fmt.Sprintf("folder-%x", time.Now().UnixNano())
	id := base
	for suffix := 2; ; suffix++ {
		used := false
		for _, folder := range folders {
			if folder.ID == id {
				used = true
				break
			}
		}
		if !used {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, suffix)
	}
}
