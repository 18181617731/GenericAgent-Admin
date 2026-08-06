package api

import (
	"context"
	"fmt"
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

	err := ctx.Err()
	if err == nil {
		err = s.resetAutomaticInstanceRoot(instance)
	}
	if err == nil {
		_, err = downloadAndExtractGenericAgentArchive(ctx, instance.GARoot)
	}
	if err == nil {
		health := ga.BuildHealth(instance.GARoot)
		if !health.OK {
			err = fmt.Errorf("downloaded GenericAgent is invalid: %s", strings.Join(health.Errors, ", "))
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
	dest := filepath.Join(s.CfgStore.Root, instance.ID)
	if !sameRuntimePath(dest, instance.GARoot) {
		return "", fmt.Errorf("refusing to modify unmanaged instance path %q", instance.GARoot)
	}
	return dest, nil
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
