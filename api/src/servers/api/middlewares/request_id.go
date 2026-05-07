package middlewares

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/titouanfreville/copro-manager/api/src/servers/api/shared"
)

const requestIDHeader = "X-Request-ID"

// RequestID extracts or generates a request ID and injects it into the context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = uuid.New().String()
		}

		ctx := context.WithValue(r.Context(), shared.RequestID, id)
		w.Header().Set(requestIDHeader, id)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
