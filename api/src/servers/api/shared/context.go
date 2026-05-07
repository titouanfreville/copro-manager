package shared

type contextKey string

const (
	// RequestID is the context key for the request ID.
	RequestID contextKey = "request_id"

	// UserID is the context key for the authenticated user's UID.
	UserID contextKey = "user_id"

	// User is the context key for the full UserContext value.
	User contextKey = "user"

	// AnonymousUserID is the sentinel UID used for unauthenticated requests.
	AnonymousUserID = "anonymous"
)

// UserContext is the authenticated user payload propagated through context.
type UserContext struct {
	UserID string
	Email  string
	Name   string
}
