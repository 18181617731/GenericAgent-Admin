package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	defaultAuthUser      = "admin"
	authStateFilename    = "auth.local.json"
	authHashIterations   = 210000
	authSaltBytes        = 16
	minimumAdminPassword = 8
	maximumAuthBodyBytes = 64 << 10
)

type authDiskState struct {
	Username   string `json:"username"`
	Salt       string `json:"salt"`
	Hash       string `json:"hash"`
	Iterations int    `json:"iterations"`
}

type authManager struct {
	mu          sync.RWMutex
	path        string
	username    string
	password    string
	salt        []byte
	hash        []byte
	iterations  int
	mustChange  bool
	initialized bool
	external    bool
}

func newAuthManager(appRoot, envUser, envPassword string) (*authManager, error) {
	manager := &authManager{path: filepath.Join(appRoot, authStateFilename)}
	if (envUser == "") != (envPassword == "") {
		return nil, errors.New("GA_ADMIN_AUTH_USER and GA_ADMIN_AUTH_PASSWORD must be set together")
	}
	if envUser != "" {
		manager.username = envUser
		manager.password = envPassword
		manager.initialized = true
		manager.external = true
		return manager, nil
	}

	data, err := os.ReadFile(manager.path)
	if err == nil {
		var state authDiskState
		if err := json.Unmarshal(data, &state); err != nil {
			return nil, fmt.Errorf("parse %s: %w", authStateFilename, err)
		}
		salt, saltErr := base64.RawStdEncoding.DecodeString(state.Salt)
		hash, hashErr := base64.RawStdEncoding.DecodeString(state.Hash)
		if saltErr != nil || hashErr != nil || state.Username == "" || len(salt) < authSaltBytes || len(hash) != sha256.Size || state.Iterations < 100000 {
			return nil, fmt.Errorf("invalid %s", authStateFilename)
		}
		manager.username = state.Username
		manager.salt = salt
		manager.hash = hash
		manager.iterations = state.Iterations
		manager.initialized = true
		return manager, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", authStateFilename, err)
	}
	manager.username = defaultAuthUser
	manager.mustChange = true
	return manager, nil
}

func (a *authManager) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isStatus := r.URL.Path == "/api/auth/status"
		isPasswordChange := r.URL.Path == "/api/auth/change-password"
		isAPI := r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/")

		if a.setupRequired() {
			switch {
			case isStatus:
				a.handleStatus(w, r)
				return
			case isPasswordChange:
				a.handleChangePassword(w, r)
				return
			case isAPI:
				writeAuthJSON(w, http.StatusPreconditionRequired, map[string]any{"error": "password_change_required", "mustChangePassword": true, "initialized": false})
				return
			default:
				next.ServeHTTP(w, r)
				return
			}
		}

		if !isIPv4LoopbackRemote(r.RemoteAddr) && !a.authenticateRequest(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="GA Admin", charset="UTF-8"`)
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}

		if isStatus {
			a.handleStatus(w, r)
			return
		}
		if isPasswordChange {
			a.handleChangePassword(w, r)
			return
		}
		if a.passwordChangeRequired() && isAPI {
			writeAuthJSON(w, http.StatusPreconditionRequired, map[string]any{"error": "password_change_required", "mustChangePassword": true, "initialized": true})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *authManager) authenticateRequest(r *http.Request) bool {
	user, password, ok := r.BasicAuth()
	if !ok {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if !a.initialized || !secureEqual(user, a.username) {
		return false
	}
	if a.password != "" {
		return secureEqual(password, a.password)
	}
	candidate := derivePasswordHash([]byte(password), a.salt, a.iterations)
	return subtle.ConstantTimeCompare(candidate, a.hash) == 1
}

func (a *authManager) setupRequired() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return !a.initialized
}

func (a *authManager) passwordChangeRequired() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.mustChange
}

func (a *authManager) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeAuthJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	writeAuthJSON(w, http.StatusOK, map[string]any{"username": a.username, "mustChangePassword": a.mustChange, "initialized": a.initialized, "managedByEnvironment": a.external})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
	ConfirmPassword string `json:"confirmPassword"`
}

func (a *authManager) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeAuthJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeAuthJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "json_required"})
		return
	}
	var req changePasswordRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, maximumAuthBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeAuthJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if req.NewPassword != req.ConfirmPassword {
		writeAuthJSON(w, http.StatusBadRequest, map[string]string{"error": "password_confirmation_mismatch"})
		return
	}
	if len(req.NewPassword) < minimumAdminPassword || len(req.NewPassword) > 256 {
		writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "weak_password", "minimumLength": minimumAdminPassword})
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.external {
		writeAuthJSON(w, http.StatusConflict, map[string]string{"error": "managed_by_environment"})
		return
	}
	if a.initialized && !a.passwordMatchesLocked(req.CurrentPassword) {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]string{"error": "current_password_incorrect"})
		return
	}
	salt := make([]byte, authSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "password_update_failed"})
		return
	}
	hash := derivePasswordHash([]byte(req.NewPassword), salt, authHashIterations)
	state := authDiskState{Username: a.username, Salt: base64.RawStdEncoding.EncodeToString(salt), Hash: base64.RawStdEncoding.EncodeToString(hash), Iterations: authHashIterations}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil || writePrivateFileAtomic(a.path, append(data, '\n')) != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]string{"error": "password_update_failed"})
		return
	}
	a.password = ""
	a.salt = salt
	a.hash = hash
	a.iterations = authHashIterations
	a.mustChange = false
	a.initialized = true
	writeAuthJSON(w, http.StatusOK, map[string]any{"ok": true, "mustChangePassword": false, "initialized": true})
}

func (a *authManager) passwordMatchesLocked(password string) bool {
	if a.password != "" {
		return secureEqual(password, a.password)
	}
	candidate := derivePasswordHash([]byte(password), a.salt, a.iterations)
	return subtle.ConstantTimeCompare(candidate, a.hash) == 1
}

func derivePasswordHash(password, salt []byte, iterations int) []byte {
	mac := hmac.New(sha256.New, password)
	mac.Write(salt)
	mac.Write([]byte{0, 0, 0, 1})
	u := mac.Sum(nil)
	result := append([]byte(nil), u...)
	for i := 1; i < iterations; i++ {
		mac = hmac.New(sha256.New, password)
		mac.Write(u)
		u = mac.Sum(nil)
		for j := range result {
			result[j] ^= u[j]
		}
	}
	return result
}

func writePrivateFileAtomic(path string, data []byte) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".auth-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		if err != nil {
			os.Remove(tmpName)
		}
	}()
	if err = tmp.Chmod(0600); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmpName, path); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

func secureEqual(got, expected string) bool {
	gotHash := sha256.Sum256([]byte(got))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(gotHash[:], expectedHash[:]) == 1
}

func writeAuthJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
