package middlewares

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	routeerrors "github.com/titouanfreville/copro-manager/api/src/servers/api/routes/errors"
)

const adminScheme = "AdminKey"

// RequireAdminKey gates the /admin/* subtree. It expects
// `Authorization: AdminKey <key>` matching middlewares.admin_api_key
// (constant-time compare). Empty configured key rejects every request —
// admin is opt-in.
func (m *Middlewares) RequireAdminKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := m.logger.With(zap.String("middleware", "RequireAdminKey"), zap.String("path", r.URL.Path))

		if m.config.AdminAPIKey == "" {
			log.Warn("admin endpoint hit but admin_api_key is empty")
			rest.Render().JSON(http.StatusUnauthorized, w, r, routeerrors.NewServErrors("UNAUTHORIZED", "admin disabled"))
			return
		}

		header := r.Header.Get(authorizationHeader)
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || parts[0] != adminScheme {
			log.Warn("malformed or missing AdminKey header")
			rest.Render().JSON(http.StatusUnauthorized, w, r, routeerrors.NewServErrors("UNAUTHORIZED", "unauthorized"))
			return
		}

		if subtle.ConstantTimeCompare([]byte(parts[1]), []byte(m.config.AdminAPIKey)) != 1 {
			log.Warn("AdminKey mismatch")
			rest.Render().JSON(http.StatusUnauthorized, w, r, routeerrors.NewServErrors("UNAUTHORIZED", "unauthorized"))
			return
		}

		next.ServeHTTP(w, r)
	})
}
