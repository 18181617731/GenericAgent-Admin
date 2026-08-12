package api

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"genericagent-admin-go/internal/config"
	"genericagent-admin-go/internal/ga"
)

type instanceInstallTask struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func (s *Server) instanceInstallsAvailable() bool {
	if s == nil {
		return false
	}
	s.instanceInstallMu.Lock()
	defer s.instanceInstallMu.Unlock()
	return !s.instanceInstallsClosing
}

func (s *Server) startInstanceInstall(instance config.InstanceConfig) bool {
	if s == nil {
		return false
	}
	s.instanceInstallMu.Lock()
	defer s.instanceInstallMu.Unlock()
	if s.instanceInstallsClosing {
		return false
	}
	if s.instanceInstallTasks == nil {
		s.instanceInstallTasks = make(map[string]*instanceInstallTask)
	}
	if _, exists := s.instanceInstallTasks[instance.ID]; exists {
		return true
	}
	ctx, cancel := context.WithCancel(context.Background())
	task := &instanceInstallTask{cancel: cancel, done: make(chan struct{})}
	s.instanceInstallTasks[instance.ID] = task
	s.instanceInstallWG.Add(1)
	go s.runInstanceInstall(ctx, task, instance)
	return true
}

func (s *Server) runInstanceInstall(ctx context.Context, task *instanceInstallTask, instance config.InstanceConfig) {
	defer s.finishInstanceInstallTask(instance.ID, task)

	s.setInstanceInstallProgress(instance, "preparing", 15)
	err := ctx.Err()
	if err == nil {
		err = s.resetAutomaticInstanceRoot(instance)
	}
	archivePath := s.instanceTemplateArchivePath(instance.ID)
	usedTemplate := false
	if err == nil {
		if _, statErr := os.Stat(archivePath); statErr == nil {
			usedTemplate = true
			s.setInstanceInstallProgress(instance, "extracting", 35)
			err = extractGenericAgentTemplateZip(archivePath, instance.GARoot)
		} else if !os.IsNotExist(statErr) {
			err = statErr
		} else {
			s.setInstanceInstallProgress(instance, "downloading", 35)
			_, err = downloadAndExtractGenericAgentArchive(ctx, instance.GARoot)
		}
	}
	if err == nil {
		s.setInstanceInstallProgress(instance, "verifying", 85)
		health := ga.BuildHealth(instance.GARoot)
		if !health.OK {
			err = fmt.Errorf("downloaded GenericAgent is invalid: %s", strings.Join(health.Errors, ", "))
		}
	}
	if usedTemplate && ctx.Err() == nil {
		if removeErr := os.Remove(archivePath); removeErr != nil && !os.IsNotExist(removeErr) {
			log.Printf("remove instance template archive %q: %v", instance.ID, removeErr)
		}
	}
	if ctx.Err() != nil {
		if cleanupErr := s.resetAutomaticInstanceRoot(instance); cleanupErr != nil {
			log.Printf("clean up cancelled instance install %q: %v", instance.ID, cleanupErr)
		}
		return
	}
	if err != nil {
		message := "download GenericAgent archive: " + err.Error()
		if cleanupErr := s.resetAutomaticInstanceRoot(instance); cleanupErr != nil {
			message += "; clean up partial install: " + cleanupErr.Error()
		}
		s.failInstanceInstall(instance, message)
		return
	}
	s.setInstanceInstallProgress(instance, "finalizing", 95)
	s.setInstanceInstallState(instance, config.InstanceInitStatusReady, "")
}

func (s *Server) finishInstanceInstallTask(id string, task *instanceInstallTask) {
	s.instanceInstallMu.Lock()
	if current, ok := s.instanceInstallTasks[id]; ok && current == task {
		delete(s.instanceInstallTasks, id)
	}
	close(task.done)
	s.instanceInstallMu.Unlock()
	s.instanceInstallWG.Done()
}

func (s *Server) failInstanceInstall(instance config.InstanceConfig, message string) {
	s.setInstanceInstallState(instance, config.InstanceInitStatusFailed, strings.TrimSpace(message))
}

func (s *Server) setInstanceInstallProgress(instance config.InstanceConfig, stage string, progress int) {
	if s == nil || s.ConfigMu == nil || s.CfgStore == nil {
		return
	}
	s.ConfigMu.Lock()
	defer s.ConfigMu.Unlock()
	cfg := cloneConfigWithInstances(s.CfgStore.Snapshot())
	for i := range cfg.Instances {
		current := cfg.Instances[i]
		if current.ID != instance.ID || !sameRuntimePath(current.GARoot, instance.GARoot) {
			continue
		}
		if strings.ToLower(strings.TrimSpace(current.InitStatus)) != config.InstanceInitStatusInitializing {
			return
		}
		cfg.Instances[i].InitStage = stage
		cfg.Instances[i].InitProgress = progress
		if err := s.saveConfigAndReconcile(cfg); err != nil {
			log.Printf("persist instance install progress %q=%q: %v", instance.ID, stage, err)
		}
		return
	}
}

func (s *Server) setInstanceInstallState(instance config.InstanceConfig, status, message string) {
	if s == nil || s.ConfigMu == nil || s.CfgStore == nil {
		return
	}
	s.ConfigMu.Lock()
	defer s.ConfigMu.Unlock()
	cfg := cloneConfigWithInstances(s.CfgStore.Snapshot())
	for i := range cfg.Instances {
		current := cfg.Instances[i]
		if current.ID != instance.ID || !sameRuntimePath(current.GARoot, instance.GARoot) {
			continue
		}
		if strings.ToLower(strings.TrimSpace(current.InitStatus)) != config.InstanceInitStatusInitializing {
			return
		}
		cfg.Instances[i].InitStatus = status
		cfg.Instances[i].InitError = message
		if status == config.InstanceInitStatusReady {
			cfg.Instances[i].InitStage = "complete"
			cfg.Instances[i].InitProgress = 100
		}
		if err := s.saveConfigAndReconcile(cfg); err != nil {
			log.Printf("persist instance install state %q=%q: %v", instance.ID, status, err)
		}
		return
	}
}

func (s *Server) resumeInstanceInstalls() {
	if s == nil || s.CfgStore == nil {
		return
	}
	for _, instance := range s.CfgStore.Snapshot().Instances {
		if strings.ToLower(strings.TrimSpace(instance.InitStatus)) == config.InstanceInitStatusInitializing {
			s.startInstanceInstall(instance)
		}
	}
}

func (s *Server) cancelInstanceInstall(id string) <-chan struct{} {
	if s == nil {
		return nil
	}
	s.instanceInstallMu.Lock()
	task := s.instanceInstallTasks[strings.TrimSpace(id)]
	if task != nil {
		task.cancel()
	}
	s.instanceInstallMu.Unlock()
	if task == nil {
		return nil
	}
	return task.done
}

func (s *Server) stopInstanceInstalls() {
	if s == nil {
		return
	}
	s.instanceInstallMu.Lock()
	s.instanceInstallsClosing = true
	for _, task := range s.instanceInstallTasks {
		task.cancel()
	}
	s.instanceInstallMu.Unlock()
	s.instanceInstallWG.Wait()
}

func (s *Server) reusableInstanceTemplatePath() string {
	return filepath.Join(s.CfgStore.Root, ".instance-templates", "genericagent.zip")
}

func (s *Server) reusableInstanceTemplateAvailable() bool {
	info, err := os.Stat(s.reusableInstanceTemplatePath())
	return err == nil && info.Mode().IsRegular()
}

func (s *Server) promoteInstanceTemplate(stagedPath string) error {
	dest := s.reusableInstanceTemplatePath()
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	backup := dest + ".previous"
	_ = os.Remove(backup)
	hadPrevious := false
	if err := os.Rename(dest, backup); err == nil {
		hadPrevious = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(stagedPath, dest); err != nil {
		if hadPrevious {
			if restoreErr := os.Rename(backup, dest); restoreErr != nil {
				return fmt.Errorf("replace reusable template: %v (restore previous template: %v)", err, restoreErr)
			}
		}
		return err
	}
	if hadPrevious {
		_ = os.Remove(backup)
	}
	return nil
}

func (s *Server) snapshotReusableInstanceTemplate(id string) (string, error) {
	srcPath := s.reusableInstanceTemplatePath()
	src, err := os.Open(srcPath)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer src.Close()
	dest := s.instanceTemplateArchivePath(id)
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), id+"-snapshot-*.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tmpPath)
		}
	}()
	_, copyErr := io.Copy(tmp, src)
	closeErr := tmp.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return "", err
	}
	keep = true
	return dest, nil
}

func (s *Server) instanceTemplateArchivePath(id string) string {
	return filepath.Join(s.CfgStore.Root, ".instance-install-archives", id+".zip")
}

const instanceTemplateMaxExtractedBytes int64 = 2 << 30
const instanceTemplateMaxEntries = 100000

func validateGenericAgentTemplateZip(zipPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()
	if len(r.File) == 0 {
		return fmt.Errorf("template zip is empty")
	}
	if len(r.File) > instanceTemplateMaxEntries {
		return fmt.Errorf("template zip has too many entries")
	}
	var total uint64
	for _, f := range r.File {
		if f.Mode()&os.ModeSymlink != 0 || !f.FileInfo().IsDir() && !f.Mode().IsRegular() {
			return fmt.Errorf("unsupported zip entry: %s", f.Name)
		}
		total += f.UncompressedSize64
		if total > uint64(instanceTemplateMaxExtractedBytes) {
			return fmt.Errorf("template expands beyond 2 GiB")
		}
	}
	return nil
}

func extractGenericAgentTemplateZip(zipPath, dest string) error {
	if err := validateGenericAgentTemplateZip(zipPath); err != nil {
		return err
	}
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	stripRoot := ""
	for _, f := range r.File {
		name := strings.Trim(strings.ReplaceAll(f.Name, "\\", "/"), "/")
		if name == "" {
			continue
		}
		parts := strings.Split(name, "/")
		if len(parts) < 2 {
			stripRoot = ""
			break
		}
		if stripRoot == "" {
			stripRoot = parts[0]
		} else if stripRoot != parts[0] {
			stripRoot = ""
			break
		}
	}
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destAbs, 0755); err != nil {
		return err
	}
	var written int64
	for _, f := range r.File {
		name := strings.Trim(strings.ReplaceAll(f.Name, "\\", "/"), "/")
		if name == "" {
			continue
		}
		if stripRoot != "" {
			if name == stripRoot {
				continue
			}
			name = strings.TrimPrefix(name, stripRoot+"/")
		}
		if name == "" {
			continue
		}
		target := filepath.Join(destAbs, filepath.FromSlash(name))
		rel, relErr := filepath.Rel(destAbs, target)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("zip entry escapes target directory: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode().Perm())
		if err != nil {
			rc.Close()
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(rc, instanceTemplateMaxExtractedBytes-written+1))
		written += n
		closeOutErr, closeInErr := out.Close(), rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOutErr != nil {
			return closeOutErr
		}
		if closeInErr != nil {
			return closeInErr
		}
		if written > instanceTemplateMaxExtractedBytes {
			return fmt.Errorf("template expands beyond 2 GiB")
		}
	}
	return nil
}

func (s *Server) resetAutomaticInstanceRoot(instance config.InstanceConfig) error {
	dest, err := s.automaticInstanceDestination(instance)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return os.Mkdir(dest, 0755)
}

func (s *Server) automaticInstanceDestination(instance config.InstanceConfig) (string, error) {
	if s == nil || s.CfgStore == nil || !isAutomaticInstanceID(instance.ID) {
		return "", fmt.Errorf("refusing to modify unmanaged instance path")
	}
	legacyDest := filepath.Join(s.CfgStore.Root, instance.ID)
	managedDest := filepath.Join(s.CfgStore.Root, automaticInstanceBaseID, "instances", instance.ID)
	if sameRuntimePath(managedDest, instance.GARoot) {
		return managedDest, nil
	}
	if sameRuntimePath(legacyDest, instance.GARoot) {
		return legacyDest, nil
	}
	return "", fmt.Errorf("refusing to modify unmanaged instance path %q", instance.GARoot)
}

func isAutomaticInstanceID(id string) bool {
	if id == "" || len(id) > 64 || id != strings.TrimSpace(id) {
		return false
	}
	for i, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || i > 0 && (r == '-' || r == '_' || r == '.') {
			continue
		}
		return false
	}
	return true
}
