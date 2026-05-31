package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func bad(w http.ResponseWriter, code int, msg string) {
	if code == http.StatusBadRequest && msg == errRequestBodyTooLarge.Error() {
		code = http.StatusRequestEntityTooLarge
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": msg})
}

const maxJSONBodyBytes = 1 << 20

var errRequestBodyTooLarge = errors.New("request body too large")

func decode(r *http.Request, v interface{}) (err error) {
	defer func() {
		if closeErr := r.Body.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	data, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBodyBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxJSONBodyBytes {
		return errRequestBodyTooLarge
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain a single JSON value")
		}
		return err
	}
	return nil
}
