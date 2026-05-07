package middlewares

import (
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/servers/api/shared"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RequestLogger logs incoming requests and outgoing responses with duration
// and status. Heartbeat probes (`/ping`) are skipped to keep the log signal
// useful — Cloud Run / monitoring hits these continuously.
func (m *Middlewares) RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ping" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()

		requestID, _ := r.Context().Value(shared.RequestID).(string)
		userID, _ := r.Context().Value(shared.UserID).(string)
		if userID == "" {
			userID = shared.AnonymousUserID
		}

		log := m.logger.With(
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("request_id", requestID),
			zap.String("user_id", userID),
		)

		log.Info("Incoming request")

		rw := newResponseWriter(w)
		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		responseLog := log.With(
			zap.Int("status", rw.statusCode),
			zap.Duration("duration", duration),
		)

		switch {
		case rw.statusCode >= 500:
			responseLog.Error("Response served")
		case rw.statusCode >= 400:
			responseLog.Warn("Response served")
		default:
			responseLog.Info("Response served")
		}
	})
}
