package api

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

var keychainMu sync.Mutex

func keychainPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "ga_keychain.enc"), nil
}

// keychainMask mirrors memory/keychain.py. Values are never exposed by the API.
func keychainMaskForUser(name string) []byte {
	sum := sha256.Sum256([]byte(name + "@ga_keychain"))
	return sum[:]
}

func keychainUserName() string {
	name := os.Getenv("USERNAME")
	if name == "" {
		name = os.Getenv("USER")
	}
	if name == "" {
		if home, err := os.UserHomeDir(); err == nil {
			name = filepath.Base(home)
		}
	}
	return name
}

func xorKeychainForUser(data []byte, name string) {
	mask := keychainMaskForUser(name)
	for i := range data {
		data[i] ^= mask[i%len(mask)]
	}
}

func readKeychainFile(path, name string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	xorKeychainForUser(data, name)
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("invalid keychain file: %w", err)
	}
	if values == nil {
		values = map[string]string{}
	}
	return values, nil
}

func readKeychain() (map[string]string, error) {
	path, err := keychainPath()
	if err != nil {
		return nil, err
	}
	return readKeychainFile(path, keychainUserName())
}

func writeKeychainFile(path, name string, values map[string]string) error {
	data, err := json.Marshal(values)
	if err != nil {
		return err
	}
	xorKeychainForUser(data, name)
	if runtime.GOOS == "windows" {
		return os.WriteFile(path, data, 0600)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func writeKeychain(values map[string]string) error {
	path, err := keychainPath()
	if err != nil {
		return err
	}
	return writeKeychainFile(path, keychainUserName(), values)
}

func validKeychainName(name string) bool {
	return name != "" && len(name) <= 128 && strings.TrimSpace(name) == name && !strings.ContainsAny(name, "\\/\r\n\t")
}

func (s *Server) keychainHandler(w http.ResponseWriter, r *http.Request) {
	keychainMu.Lock()
	defer keychainMu.Unlock()
	values, err := readKeychain()
	if err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	switch r.Method {
	case http.MethodGet:
		names := make([]string, 0, len(values))
		for name := range values {
			names = append(names, name)
		}
		sort.Strings(names)
		writeJSON(w, map[string]interface{}{"keys": names})
		return
	case http.MethodPut:
		var req struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		if err := decode(r, &req); err != nil {
			bad(w, http.StatusBadRequest, err.Error())
			return
		}
		if !validKeychainName(req.Name) {
			bad(w, http.StatusBadRequest, "invalid key name")
			return
		}
		if req.Value == "" {
			bad(w, http.StatusBadRequest, "value must not be empty")
			return
		}
		values[req.Name] = req.Value
	case http.MethodDelete:
		var req struct {
			Name string `json:"name"`
		}
		if err := decode(r, &req); err != nil {
			bad(w, http.StatusBadRequest, err.Error())
			return
		}
		if !validKeychainName(req.Name) {
			bad(w, http.StatusBadRequest, "invalid key name")
			return
		}
		if _, ok := values[req.Name]; !ok {
			bad(w, http.StatusNotFound, "key not found")
			return
		}
		delete(values, req.Name)
	default:
		bad(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := writeKeychain(values); err != nil {
		bad(w, http.StatusInternalServerError, err.Error())
		return
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	writeJSON(w, map[string]interface{}{"keys": names})
}
