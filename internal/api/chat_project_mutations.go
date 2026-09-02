package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"genericagent-admin-go/internal/config"
)

func projectMutationMethod(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			bad(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		next(w, r)
	}
}

func (s *Server) renameProjectLocked(cfg config.AppConfig, oldName, newName string) (int, error) {
	oldDir, err := projectDirectory(cfg, oldName)
	if err != nil {
		return 0, err
	}
	newDir, err := projectDestination(cfg, newName)
	if err != nil {
		return 0, err
	}
	changes, err := collectProjectSessionFiles(cfg, oldName, newName)
	if err != nil {
		return 0, err
	}
	if s.projectHasRunningSession(changes) {
		return 0, errProjectRunning
	}
	prefs, err := snapshotProjectPrefs(cfg)
	if err != nil {
		return 0, err
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		return 0, fmt.Errorf("无法重命名项目目录：%w", err)
	}
	if err := applyProjectSessionFiles(changes); err != nil {
		return projectRenameRollback(oldDir, newDir, changes, prefs, err)
	}
	if _, err := replacePinnedProject(cfg, oldName, newName); err != nil {
		return projectRenameRollback(oldDir, newDir, changes, prefs, err)
	}
	return len(changes), nil
}

func projectRenameRollback(oldDir, newDir string, changes []projectSessionFile, prefs projectPrefsSnapshot, cause error) (int, error) {
	rollbackErr := rollbackProjectRename(oldDir, newDir, changes, prefs)
	if rollbackErr != nil {
		return 0, errors.Join(cause, fmt.Errorf("回滚项目重命名失败：%w", rollbackErr))
	}
	return 0, cause
}

func (s *Server) deleteProjectLocked(cfg config.AppConfig, name string) (int, error) {
	projectDir, err := projectDirectory(cfg, name)
	if err != nil {
		return 0, err
	}
	changes, err := collectProjectSessionFiles(cfg, name, "")
	if err != nil {
		return 0, err
	}
	if s.projectHasRunningSession(changes) {
		return 0, errProjectRunning
	}
	prefs, err := snapshotProjectPrefs(cfg)
	if err != nil {
		return 0, err
	}
	trashDir, trashRoot, err := projectTrashPath(cfg, name)
	if err != nil {
		return 0, err
	}
	if err := os.Rename(projectDir, trashDir); err != nil {
		return 0, fmt.Errorf("无法准备删除项目目录：%w", err)
	}
	rollback := func(cause error) (int, error) {
		return projectDeleteRollback(projectDir, trashDir, changes, prefs, cause)
	}
	if err := applyProjectSessionFiles(changes); err != nil {
		return rollback(err)
	}
	if _, err := removePinnedProject(cfg, name); err != nil {
		return rollback(err)
	}
	if err := os.RemoveAll(trashDir); err != nil {
		return rollback(err)
	}
	_ = os.Remove(trashRoot)
	return len(changes), nil
}

func projectDeleteRollback(projectDir, trashDir string, changes []projectSessionFile, prefs projectPrefsSnapshot, cause error) (int, error) {
	rollbackErr := restoreProjectSessionFiles(changes)
	if err := restoreProjectPrefs(prefs); err != nil && rollbackErr == nil {
		rollbackErr = err
	}
	if err := os.Rename(trashDir, projectDir); err != nil && rollbackErr == nil {
		rollbackErr = err
	}
	if rollbackErr != nil {
		return 0, errors.Join(cause, fmt.Errorf("回滚项目删除失败：%w", rollbackErr))
	}
	return 0, cause
}

func (s *Server) chatRenameProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !requireDangerousHeader(w, r) {
		return
	}
	var req struct {
		Name    string `json:"name"`
		NewName string `json:"new_name"`
	}
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	oldName, oldOK := validProjectModeName(req.Name)
	newName, newOK := validProjectModeName(req.NewName)
	if !oldOK || !newOK {
		bad(w, http.StatusBadRequest, errProjectNameInvalid.Error())
		return
	}
	if oldName == newName {
		bad(w, http.StatusConflict, errProjectSameName.Error())
		return
	}
	cfg := s.CfgStore.Snapshot()
	s.ChatMu.Lock()
	s.SessionMu.Lock()
	updated, err := s.renameProjectLocked(cfg, oldName, newName)
	s.SessionMu.Unlock()
	s.ChatMu.Unlock()
	if err != nil {
		bad(w, projectMutationStatus(err), err.Error())
		return
	}
	names, pinned := chatProjectNamesFor(cfg)
	writeJSON(w, map[string]interface{}{"ok": true, "old_name": oldName, "name": newName, "updated_sessions": updated, "projects": names, "pinned_projects": pinned})
}

func (s *Server) chatDeleteProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !requireDangerousHeader(w, r) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		bad(w, http.StatusBadRequest, err.Error())
		return
	}
	name, ok := validProjectModeName(req.Name)
	if !ok {
		bad(w, http.StatusBadRequest, errProjectNameInvalid.Error())
		return
	}
	cfg := s.CfgStore.Snapshot()
	s.ChatMu.Lock()
	s.SessionMu.Lock()
	detached, err := s.deleteProjectLocked(cfg, name)
	s.SessionMu.Unlock()
	s.ChatMu.Unlock()
	if err != nil {
		bad(w, projectMutationStatus(err), err.Error())
		return
	}
	names, pinned := chatProjectNamesFor(cfg)
	writeJSON(w, map[string]interface{}{"ok": true, "name": name, "detached_sessions": detached, "sessions_preserved": true, "projects": names, "pinned_projects": pinned})
}
