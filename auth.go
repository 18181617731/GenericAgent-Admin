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

	"genericagent-admin-go/internal/config"
)

const (
	defaultAuthUser      = "admin"
	defaultAuthPassword  = "admin"
	authStateFilename    = "auth.local.json"
	authHashIterations   = 210000
	authSaltBytes        = 16
	minimumAdminPassword = 12
	maximumAuthBodyBytes = 64 << 10
	authStatusPath       = "/api/auth/status"
	authPasswordPath     = "/api/auth/password"
)

type authDiskState struct {
	Username   string `json:"username"`
	Salt       string `json:"salt"`
	Hash       string `json:"hash"`
	Iterations int    `json:"iterations"`
}

// authManager guards non-loopback access to the admin server. Requests from
// this machine are always trusted: the admin server binds loopback unless the
// operator explicitly turns on remote access, so a local request already
// implies local control of the account.
type authManager struct {
	mu         sync.RWMutex
	path       string
	enabled    bool
	username   string
	password   string
	salt       []byte
	hash       []byte
	iterations int
	mustChange bool
	external   bool
}

func newAuthManager(appRoot, envUser, envPassword, envEnabled string) (*authManager, error) {
	manager := &authManager{path: filepath.Join(appRoot, authStateFilename)}
	if (envUser == "") != (envPassword == "") {
		return nil, errors.New("GA_ADMIN_AUTH_USER and GA_ADMIN_AUTH_PASSWORD must be set together")
	}
	manager.enabled = parseAuthEnabled(envEnabled) || envUser != ""
	if !manager.enabled {
		manager.username = defaultAuthUser
		return manager, nil
	}
	if envUser != "" {
		manager.username = envUser
		manager.password = envPassword
		manager.external = true
		return manager, nil
	}

	data, err := os.ReadFile(manager.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No password yet. That is the normal state for a loopback-only
			// install and must not block anything.
			return manager, nil
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
		return manager, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", authStateFilename, err)
	}
	manager.username = defaultAuthUser
	manager.password = defaultAuthPassword
	manager.mustChange = true
	return manager, nil
}

func parseAuthEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

// Authentication is intentionally disabled for the production server. Keep
// the two auth API endpoints available because the frontend checks auth status
// before rendering and older clients may still call the password endpoint.
func authDisabledMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/status":
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", http.MethodGet)
				writeAuthJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
				return
			}
			writeAuthJSON(w, http.StatusOK, map[string]any{
				"authEnabled":          false,
				"username":             defaultAuthUser,
				"mustChangePassword":   false,
				"managedByEnvironment": false,
			})
			return
		case "/api/auth/change-password":
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				writeAuthJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
				return
			}
			writeAuthJSON(w, http.StatusConflict, map[string]string{"error": "auth_disabled"})
			return
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func (a *authManager) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loopback := isIPv4LoopbackRemote(r.RemoteAddr)
		if a.enabled && !loopback && !a.authenticateRequest(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="GA Admin", charset="UTF-8"`)
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}

		if r.URL.Path == "/api/auth/status" {
			a.handleStatus(w, r)
			return
		}
		if r.URL.Path == "/api/auth/change-password" {
			a.handleChangePassword(w, r)
			return
		}
		if a.enabled && !loopback && a.passwordChangeRequired() && strings.HasPrefix(r.URL.Path, "/api/") {
			writeAuthJSON(w, http.StatusPreconditionRequired, map[string]any{"error": "password_change_required", "mustChangePassword": true})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authorize reports whether a non-loopback request may proceed, answering it
// with a challenge when it may not.
func (a *authManager) authorize(w http.ResponseWriter, r *http.Request) bool {
	if a.anonymousRemoteAllowed() {
		return true
	}
	if a.authenticateRequest(r) {
		return true
	}
	a.challenge(w)
	return false
}

func (a *authManager) challenge(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="GA Admin", charset="UTF-8"`)
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, "authentication required", http.StatusUnauthorized)
}

// anonymousRemoteAllowed reports whether remote clients may skip the password.
// When no password has been configured this stays false, so an unfinished
// remote setup fails closed with a challenge instead of opening the server.
func (a *authManager) anonymousRemoteAllowed() bool {
	a.mu.RLock()
	external := a.external
	a.mu.RUnlock()
	if external {
		return false
	}
	return a.cfg.Snapshot().RemoteAllowAnonymous
}

func (a *authManager) authenticateRequest(r *http.Request) bool {
	if !a.enabled {
		return true
	}
	user, password, ok := r.BasicAuth()
	if !ok {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if !secureEqual(user, a.username) {
		return false
	}
	return a.passwordMatchesLocked(password)
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
	mustChange := a.mustChange && !isIPv4LoopbackRemote(r.RemoteAddr)
	writeAuthJSON(w, http.StatusOK, map[string]any{"authEnabled": a.enabled, "username": a.username, "mustChangePassword": mustChange, "managedByEnvironment": a.external})
}

type setPasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
	ConfirmPassword string `json:"confirmPassword"`
}

func (a *authManager) handlePassword(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.handleSetPassword(w, r)
	case http.MethodDelete:
		a.handleRemovePassword(w)
	default:
		w.Header().Set("Allow", http.MethodPost+", "+http.MethodDelete)
		writeAuthJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
	if !a.enabled {
		writeAuthJSON(w, http.StatusConflict, map[string]string{"error": "auth_disabled"})
		return
	}
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeAuthJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "json_required"})
		return
	}
	var req setPasswordRequest
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
	if len(req.NewPassword) < minimumAdminPassword || len(req.NewPassword) > 256 || secureEqual(req.NewPassword, defaultAuthPassword) {
		writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "weak_password", "minimumLength": minimumAdminPassword})
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.external {
		writeAuthJSON(w, http.StatusConflict, map[string]string{"error": "managed_by_environment"})
		return
	}
	if !a.passwordMatchesLocked(req.CurrentPassword) {
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
	writeAuthJSON(w, http.StatusOK, map[string]any{"ok": true, "mustChangePassword": false})
}

func (a *authManager) passwordMatchesLocked(password string) bool {
	if a.password != "" {
		return secureEqual(password, a.password)
	}
	if len(a.hash) == 0 {
		return false
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
