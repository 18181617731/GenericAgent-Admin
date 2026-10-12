package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
)

// Keep only list-facing fields; full settings, prompts and records remain in
// the session detail response. Legacy list clients keep their existing schema.
type chatListConductor struct {
	Role            string `json:"role"`
	ParentSessionID string `json:"parent_session_id,omitempty"`
	DispatchID      string `json:"dispatch_id,omitempty"`
	Status          string `json:"status,omitempty"`
	Recovery        string `json:"recovery,omitempty"`
}

type chatListLoop struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
	Round   int    `json:"round"`
}

func compactChatListItems(items []map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		compact := make(map[string]interface{}, len(item))
		for key, value := range item {
			compact[key] = value
		}
		if c, ok := item["conductor"].(*chatConductorState); ok && c != nil {
			compact["conductor"] = &chatListConductor{Role: c.Role, ParentSessionID: c.ParentSessionID, DispatchID: c.DispatchID, Status: c.Status, Recovery: c.Recovery}
		}
		if l, ok := item["loop"].(chatLoopState); ok {
			compact["loop"] = chatListLoop{Enabled: l.Enabled, Status: l.Status, Round: l.Round}
		}
		out = append(out, compact)
	}
	return out
}

type chatListSyncSnapshot struct {
	revision string
	items    map[string][32]byte
	metadata [32]byte
	bytes    int
}

const chatListSyncMaxSnapshots = 8
const chatListSyncMaxBytes = 8 * 1024 * 1024

// Read-only derived cache: no client token can reference a different instance
// or data directory, and expired tokens safely receive a full reset snapshot.
func (s *Server) writeChatListSync(w http.ResponseWriter, r *http.Request, items []map[string]interface{}, metadata map[string]interface{}) {
	compact := compactChatListItems(items)
	metadataJSON, _ := json.Marshal(metadata)
	metadataHash := sha256.Sum256(metadataJSON)
	current := chatListSyncSnapshot{items: make(map[string][32]byte, len(compact)), metadata: metadataHash}
	digest := sha256.New()
	digest.Write(metadataHash[:])
	for _, item := range compact {
		data, _ := json.Marshal(item)
		hash := sha256.Sum256(data)
		id, _ := item["id"].(string)
		current.items[id] = hash
		digest.Write(hash[:]) // sorted list order is part of the revision
		current.bytes += len(id) + 32 + 64
	}
	current.revision = fmt.Sprintf("%x", digest.Sum(nil))
	current.bytes += len(current.revision) + 32
	since := r.URL.Query().Get("since")
	runtime := s.ChatRuntime
	runtime.listSyncMu.Lock()
	path := chatSessionDir(s.CfgStore.Snapshot())
	if runtime.listSyncPath != path {
		runtime.listSyncPath = path
		runtime.listSyncSnapshots = nil
		runtime.listSyncBytes = 0
	}
	var previous *chatListSyncSnapshot
	known := false
	for i := range runtime.listSyncSnapshots {
		snapshot := &runtime.listSyncSnapshots[i]
		if snapshot.revision == since {
			previous = snapshot
		}
		if snapshot.revision == current.revision {
			known = true
		}
	}
	payload := map[string]interface{}{"revision": current.revision, "reset": previous == nil}
	if previous == nil {
		payload["sessions"] = compact
		for key, value := range metadata {
			payload[key] = value
		}
	} else {
		upserts := make([]map[string]interface{}, 0)
		removed := make([]string, 0)
		for _, item := range compact {
			id, _ := item["id"].(string)
			if old, ok := previous.items[id]; !ok || old != current.items[id] {
				upserts = append(upserts, item)
			}
		}
		for id := range previous.items {
			if _, ok := current.items[id]; !ok {
				removed = append(removed, id)
			}
		}
		sort.Strings(removed)
		payload["base_revision"] = since
		payload["session_upserts"] = upserts
		payload["removed_session_ids"] = removed
		if previous.metadata != current.metadata {
			for key, value := range metadata {
				payload[key] = value
			}
		}
	}
	if !known && current.bytes <= chatListSyncMaxBytes {
		runtime.listSyncSnapshots = append(runtime.listSyncSnapshots, current)
		runtime.listSyncBytes += current.bytes
		for len(runtime.listSyncSnapshots) > chatListSyncMaxSnapshots || runtime.listSyncBytes > chatListSyncMaxBytes {
			runtime.listSyncBytes -= runtime.listSyncSnapshots[0].bytes
			runtime.listSyncSnapshots = runtime.listSyncSnapshots[1:]
		}
	}
	runtime.listSyncMu.Unlock()
	if since == current.revision {
		w.Header().Set("Cache-Control", "private, no-cache")
		w.Header().Set("Vary", "Accept-Encoding")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeCachedJSON(w, r, payload)
}
