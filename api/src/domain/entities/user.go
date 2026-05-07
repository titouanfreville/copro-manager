package entities

// User is the logable entity in our own DB. Its ID is the Firebase Auth UID
// — Firebase is the auth source of truth, and we don't introduce a second
// identifier namespace. The User doc is our hook for app-level metadata
// that doesn't fit into the auth provider (display name overrides, future
// preferences, etc.).
type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}
