package api

import (
	"errors"
	"net/http"

	"genericagent-admin-go/internal/modelconfig"
)

// withModelInstance scopes model-file operations to the selected GA instance.
// The derived config store is request-local; Admin-wide preferences remain on
// the base server and are deliberately not persisted from this scope.
func (s *Server) withModelInstance(next func(*Server, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modelServer, instanceID, err := s.chatServerForRequest(r)
		if err != nil {
			status := http.StatusInternalServerError
			var notFound *chatInstanceNotFoundError
			if errors.As(err, &notFound) {
				status = http.StatusNotFound
			}
			bad(w, status, err.Error())
			return
		}
		modelServer.Models = modelconfig.NewStore(modelServer.CfgStore.Snapshot().GARoot)
		setResolvedInstanceHeader(w, instanceID)
		next(modelServer, w, r)
	}
}
