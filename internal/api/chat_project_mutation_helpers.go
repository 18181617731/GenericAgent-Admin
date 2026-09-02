package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"genericagent-admin-go/internal/config"
)

var (
	errProjectNotFound      = errors.New("project not found")
	errProjectAlreadyExists = errors.New("project already exists")
	errProjectSameName      = errors.New("project name did not change")
	errProjectUnsafePath    = errors.New("project path is not a safe directory")
	errProjectRunning       = errors.New("project has a running chat")
)

type projectSessionFile struct {
	id       string
	path     string
	original []byte
	updated  []byte
}

type projectPrefsSnapshot struct {
	path   string
	data   []byte
	exists bool
}

func projectMutationStatus(err error) int {
	switch {
	case errors.Is(err, errProjectNameInvalid), errors.Is(err, errProjectGARootMissing):
		return http.StatusBadRequest
	case errors.Is(err, errProjectNotFound):
		return http.StatusNotFound
	case errors.Is(err, errProjectAlreadyExists), errors.Is(err, errProjectSameName),
		errors.Is(err, errProjectUnsafePath), errors.Is(err, errProjectRunning):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func projectProjectsDir(cfg config.AppConfig) (string, error) {
	if strings.TrimSpace(cfg.GARoot) == "" {
		return "", errProjectGARootMissing
	}
	dir := filepath.Join(cfg.GARoot, "temp", "projects")
	st, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errProjectNotFound
		}
		return "", fmt.Errorf("无法检查项目目录：%w", err)
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
		return "", fmt.Errorf("%w：`%s`", errProjectUnsafePath, dir)
	}
	return dir, nil
}

func projectDirectory(cfg config.AppConfig, name string) (string, error) {
	projectsDir, err := projectProjectsDir(cfg)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(projectsDir, name)
	st, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errProjectNotFound
		}
		return "", fmt.Errorf("无法检查项目路径：%w", err)
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
		return "", fmt.Errorf("%w：`%s`", errProjectUnsafePath, dir)
	}
	return dir, nil
}

func projectDestination(cfg config.AppConfig, name string) (string, error) {
	projectsDir, err := projectProjectsDir(cfg)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(projectsDir, name)
	if _, err := os.Lstat(dir); err == nil {
		return "", errProjectAlreadyExists
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("无法检查目标项目路径：%w", err)
	}
	return dir, nil
}

func collectProjectSessionFiles(cfg config.AppConfig, oldName, newName string) ([]projectSessionFile, error) {
	if err := ensureChatDataMigrated(cfg); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(chatSessionDir(cfg))
	if err != nil {
		if os.IsNotExist(err) {
			return []projectSessionFile{}, nil
		}
		return nil, err
	}
	changes := make([]projectSessionFile, 0)
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		change, ok, err := projectSessionChange(filepath.Join(chatSessionDir(cfg), entry.Name()), entry.Name(), oldName, newName)
		if err != nil {
			return nil, err
		}
		if ok {
			changes = append(changes, change)
		}
	}
	return changes, nil
}

func projectSessionChange(path, filename, oldName, newName string) (projectSessionFile, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return projectSessionFile{}, false, err
	}
	var session struct {
		ID          string `json:"id"`
		ProjectMode string `json:"project_mode"`
	}
	if err := json.Unmarshal(raw, &session); err != nil || session.ProjectMode != oldName {
		return projectSessionFile{}, false, nil
	}
	var document map[string]interface{}
	if err := json.Unmarshal(raw, &document); err != nil {
		return projectSessionFile{}, false, err
	}
	if newName == "" {
		delete(document, "project_mode")
	} else {
		document["project_mode"] = newName
	}
	updated, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return projectSessionFile{}, false, err
	}
	id := session.ID
	if id == "" {
		id = strings.TrimSuffix(filename, ".json")
	}
	return projectSessionFile{id: id, path: path, original: raw, updated: updated}, true, nil
}

func applyProjectSessionFiles(changes []projectSessionFile) error {
	for _, change := range changes {
		if err := writeChatFileAtomic(change.path, change.updated, 0644); err != nil {
			return err
		}
	}
	return nil
}

func restoreProjectSessionFiles(changes []projectSessionFile) error {
	var firstErr error
	for _, change := range changes {
		if err := writeChatFileAtomic(change.path, change.original, 0644); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func snapshotProjectPrefs(cfg config.AppConfig) (projectPrefsSnapshot, error) {
	snapshot := projectPrefsSnapshot{path: chatProjectPrefsPath(cfg)}
	data, err := os.ReadFile(snapshot.path)
	if err == nil {
		snapshot.data = data
		snapshot.exists = true
		return snapshot, nil
	}
	if os.IsNotExist(err) {
		return snapshot, nil
	}
	return snapshot, err
}

func restoreProjectPrefs(snapshot projectPrefsSnapshot) error {
	if snapshot.exists {
		return writeChatFileAtomic(snapshot.path, snapshot.data, 0644)
	}
	if err := os.Remove(snapshot.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func replacePinnedProject(cfg config.AppConfig, oldName, newName string) (bool, error) {
	current := loadPinnedProjects(cfg)
	pinned := false
	next := make([]string, 0, len(current)+1)
	for _, name := range current {
		if name == oldName {
			pinned = true
			next = append(next, newName)
			continue
		}
		next = append(next, name)
	}
	if !pinned {
		return false, nil
	}
	return true, savePinnedProjects(cfg, next)
}

func removePinnedProject(cfg config.AppConfig, name string) (bool, error) {
	current := loadPinnedProjects(cfg)
	next := make([]string, 0, len(current))
	removed := false
	for _, existing := range current {
		if existing == name {
			removed = true
			continue
		}
		next = append(next, existing)
	}
	if !removed {
		return false, nil
	}
	return true, savePinnedProjects(cfg, next)
}

func (s *Server) projectHasRunningSession(changes []projectSessionFile) bool {
	for _, change := range changes {
		if run := s.ChatRuns[safeChatID(change.id)]; run != nil && !run.Done {
			return true
		}
	}
	return false
}

func rollbackProjectRename(oldDir, newDir string, changes []projectSessionFile, prefs projectPrefsSnapshot) error {
	var firstErr error
	if err := restoreProjectSessionFiles(changes); err != nil {
		firstErr = err
	}
	if err := restoreProjectPrefs(prefs); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := os.Rename(newDir, oldDir); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func projectTrashPath(cfg config.AppConfig, name string) (string, string, error) {
	projectsDir, err := projectProjectsDir(cfg)
	if err != nil {
		return "", "", err
	}
	trashRoot := filepath.Join(filepath.Dir(projectsDir), ".ga-project-trash")
	if st, statErr := os.Lstat(trashRoot); statErr == nil {
		if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			return "", "", fmt.Errorf("%w：`%s`", errProjectUnsafePath, trashRoot)
		}
	} else if !os.IsNotExist(statErr) {
		return "", "", statErr
	}
	if err := os.MkdirAll(trashRoot, 0755); err != nil {
		return "", "", err
	}
	return filepath.Join(trashRoot, name+"-"+newChatID()), trashRoot, nil
}
