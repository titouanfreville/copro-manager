package middlewares

import (
	"context"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	routeerrors "github.com/titouanfreville/copro-manager/api/src/servers/api/routes/errors"
	"github.com/titouanfreville/copro-manager/api/src/servers/api/shared"
)

const authorizationHeader = "Authorization"

// Authorize verifies the Authorization header and injects the authenticated user
// into the request context. Three schemes are recognized:
//
//   - Bearer <firebase-id-token>  — verified against Firebase Auth (production path)
//   - Bypasses <local-key>        — local-dev shortcut, only honored when
//     middlewares.allow_bypass is true and the key matches the configured one
//   - AdminKey <global-secret>    — passed through unverified here; the
//     admin.RequireAdminKey middleware validates it on the /admin/* subtree.
//     Requests authenticated only via AdminKey have no user context.
//
// Requests without an Authorization header are passed through as anonymous;
// downstream handlers can use RequireAuth to reject them.
func (m *Middlewares) Authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := m.logger.With(zap.String("middleware", "Authorize"))

		header := r.Header.Get(authorizationHeader)
		if header == "" {
			next.ServeHTTP(w, r.WithContext(withAnonymous(r.Context())))
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 {
			log.Warn("malformed Authorization header")
			rest.Render().JSON(http.StatusUnauthorized, w, r, routeerrors.NewServErrors("UNAUTHORIZED", "unauthorized"))
			return
		}

		scheme, token := parts[0], parts[1]

		switch scheme {
		case "Bearer":
			user, err := m.verifyFirebase(r.Context(), token)
			if err != nil {
				log.Warn("firebase token verification failed", zap.Error(err))
				rest.Render().JSON(http.StatusUnauthorized, w, r, routeerrors.NewServErrors("UNAUTHORIZED", "unauthorized"))
				return
			}

			next.ServeHTTP(w, r.WithContext(withUser(r.Context(), user)))

		case "Bypasses":
			if !m.config.AllowBypass || m.config.BypassAuthKey == "" || token != m.config.BypassAuthKey {
				log.Warn("bypass attempt rejected")
				rest.Render().JSON(http.StatusUnauthorized, w, r, routeerrors.NewServErrors("UNAUTHORIZED", "unauthorized"))
				return
			}

			impersonate := r.Header.Get("X-Bypass-User-ID")
			if impersonate == "" {
				impersonate = "bypass"
			}

			next.ServeHTTP(w, r.WithContext(withUser(r.Context(), shared.UserContext{UserID: impersonate})))

		case "AdminKey":
			next.ServeHTTP(w, r.WithContext(withAnonymous(r.Context())))

		default:
			log.Warn("unsupported Authorization scheme", zap.String("scheme", scheme))
			rest.Render().JSON(http.StatusUnauthorized, w, r, routeerrors.NewServErrors("UNAUTHORIZED", "unauthorized"))
		}
	})
}

// RequireAuth rejects requests that didn't go through Authorize successfully
// (i.e. anonymous requests). Mount it on routes that need a known user.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := r.Context().Value(shared.UserID).(string)
		if uid == "" || uid == shared.AnonymousUserID {
			rest.Render().JSON(http.StatusUnauthorized, w, r, routeerrors.NewServErrors("UNAUTHORIZED", "unauthorized"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *Middlewares) verifyFirebase(ctx context.Context, idToken string) (shared.UserContext, error) {
	tok, err := m.auth.VerifyIDToken(ctx, idToken)
	if err != nil {
		return shared.UserContext{}, err
	}

	user := shared.UserContext{UserID: tok.UID}

	// `email_verified` is intentionally NOT enforced: this project does not
	// run an email-verification flow. Accounts are seeded by the admin
	// (no self-signup, no invitation flow per FR6), so the email claim is
	// trusted as-is. Identity decisions still gate on UID, never on email.
	if email, ok := tok.Claims["email"].(string); ok {
		user.Email = email
	}

	if name, ok := tok.Claims["name"].(string); ok {
		user.Name = name
	}

	return user, nil
}

func withAnonymous(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, shared.UserID, shared.AnonymousUserID)
	ctx = context.WithValue(ctx, shared.User, shared.UserContext{UserID: shared.AnonymousUserID})

	return ctx
}

func withUser(ctx context.Context, user shared.UserContext) context.Context {
	ctx = context.WithValue(ctx, shared.UserID, user.UserID)
	ctx = context.WithValue(ctx, shared.User, user)

	return ctx
}
